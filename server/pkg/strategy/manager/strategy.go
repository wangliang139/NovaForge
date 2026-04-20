package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/jackc/pgx/v5"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/repos/bot"
	"github.com/wangliang139/NovaForge/server/pkg/repos/snapshot"
	repo "github.com/wangliang139/NovaForge/server/pkg/repos/strategy"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/mow/snowflake"
)

// 策略状态流转：draft -> active -> inactive -> active -> overridden -> deleted

// StrategyManager 策略管理器接口
type StrategyManager interface {
	CreateStrategy(ctx context.Context, req *stypes.CreateStrategyRequest) (*stypes.Strategy, error)
	UpdateStrategy(ctx context.Context, req *stypes.UpdateStrategyRequest) (*stypes.Strategy, error)
	GetStrategy(ctx context.Context, id string) (*stypes.Strategy, error)
	GetStrategyByVersion(ctx context.Context, id, version string) (*stypes.Strategy, error)
	ListStrategies(ctx context.Context, filter *stypes.StrategyFilter) ([]*stypes.Strategy, error)
	CountStrategies(ctx context.Context) (int64, error)
	DeleteStrategy(ctx context.Context, id string) error
	ActiveStrategy(ctx context.Context, id string) error
	InactiveStrategy(ctx context.Context, id string) error
}

// strategyManager 策略管理器实现
type strategyManager struct {
	db *repos.Entity
}

// NewStrategyManager 创建策略管理器
func NewStrategyManager(db *repos.Entity) StrategyManager {
	return &strategyManager{
		db: db,
	}
}

// CreateStrategy 创建策略
func (m *strategyManager) CreateStrategy(ctx context.Context, req *stypes.CreateStrategyRequest) (*stypes.Strategy, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("strategy name required")
	}
	if req.Code == "" {
		return nil, fmt.Errorf("strategy code required")
	}

	id := snowflake.Generate().String()
	createdAt := time.Now()

	paramsBytes, paramsHash, err := canonicalizeParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	sigBytes, sigHash, err := canonicalizeSignals(req.Signals)
	if err != nil {
		return nil, fmt.Errorf("invalid signals: %w", err)
	}
	version := getStrategyVersion(id, req.Code, -1, paramsHash, sigHash)

	// check if strategy already exists
	existingStrategy, err := m.db.StrategyRepo.GetByName(ctx, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get strategy: %w", err)
	}
	if existingStrategy != nil {
		return nil, fmt.Errorf("strategy already exists")
	}

	// 保存到数据库
	result, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		po, err := m.db.StrategyRepo.WithTx(tx).CreateStrategy(ctx, repo.CreateStrategyParams{
			ID:          id,
			Name:        req.Name,
			Description: req.Description,
			Status:      repo.StrategyStatusDraft,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create strategy: %w", err)
		}

		snapshotPo, err := m.db.SnapshotRepo.WithTx(tx).CreateSnapshot(ctx, snapshot.CreateSnapshotParams{
			StrategyID: id,
			ParentID:   -1,
			Version:    version,
			Code:       req.Code,
			Params:     paramsBytes,
			Signals:    sigBytes,
			IsActive:   true,
			CreatedAt:  createdAt,
			UpdatedAt:  createdAt,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create snapshot: %w", err)
		}
		return strategyToTypes(po, snapshotPo)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create strategy: %w", err)
	}
	return result.(*stypes.Strategy), nil
}

// UpdateStrategy 更新策略
func (m *strategyManager) UpdateStrategy(ctx context.Context, req *stypes.UpdateStrategyRequest) (*stypes.Strategy, error) {
	if req.Id == "" {
		return nil, fmt.Errorf("strategy id required")
	}
	if req.Version == "" {
		return nil, fmt.Errorf("strategy version required")
	}

	result, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		var err error
		strategyPo, err := m.db.StrategyRepo.WithTx(tx).LockStrategy(ctx, req.Id)
		if err != nil {
			return nil, fmt.Errorf("failed to lock strategy: %w", err)
		}
		if strategyPo == nil {
			return nil, fmt.Errorf("strategy not found")
		}

		snapshotPo, err := m.db.SnapshotRepo.GetByStrategyIdAndVersion(ctx, snapshot.GetByStrategyIdAndVersionParams{
			StrategyID: req.Id,
			Version:    req.Version,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get snapshot: %w", err)
		}
		if snapshotPo == nil {
			return nil, fmt.Errorf("strategy version not found")
		}
		if !snapshotPo.IsActive {
			return nil, fmt.Errorf("strategy version is not active")
		}

		// 全量覆盖：始终使用请求里的 code/params/signals 构建新的内容版本。
		// 若内容未变化，则不创建新 snapshot（避免版本膨胀），但仍会更新策略 name/description。
		paramsBytes, paramsHash, err := canonicalizeParams(req.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal strategy params: %w", err)
		}

		sigBytes, sigHash, err := canonicalizeSignals(req.Signals)
		if err != nil {
			return nil, fmt.Errorf("invalid signals: %w", err)
		}

		snapshotUpdated := false
		version := getStrategyVersion(req.Id, req.Code, snapshotPo.ParentID, paramsHash, sigHash)
		if version != snapshotPo.Version {
			snapshotUpdated = true

			// 将老版本设置为非活跃
			count, err := m.db.SnapshotRepo.WithTx(tx).InactivateSnapshot(ctx, snapshotPo.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to inactivate snapshot: %w", err)
			}
			if count == 0 {
				return nil, fmt.Errorf("failed to inactivate snapshot")
			}

			// 创建新版本
			createdAt := time.Now()
			newVersion := getStrategyVersion(req.Id, req.Code, snapshotPo.ID, paramsHash, sigHash)
			snapshotPo, err = m.db.SnapshotRepo.WithTx(tx).CreateSnapshot(ctx, snapshot.CreateSnapshotParams{
				StrategyID: req.Id,
				ParentID:   snapshotPo.ID,
				Version:    newVersion,
				Code:       req.Code,
				Params:     paramsBytes,
				Signals:    sigBytes,
				IsActive:   true,
				CreatedAt:  createdAt,
				UpdatedAt:  createdAt,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create snapshot: %w", err)
			}
		} else if string(snapshotPo.Params) != string(paramsBytes) {
			snapshotUpdated = true
			snapshotPo, err = m.db.SnapshotRepo.WithTx(tx).UpdateParams(ctx, snapshot.UpdateParamsParams{
				Params: paramsBytes,
				ID:     snapshotPo.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to update snapshot: %w", err)
			}
		}

		// 更新策略
		if req.Name != strategyPo.Name || req.Description != strategyPo.Description || snapshotUpdated {
			sts := repo.NullStrategyStatus{}
			if snapshotUpdated && strategyPo.Status != repo.StrategyStatusDraft {
				sts.StrategyStatus = repo.StrategyStatusDraft
				sts.Valid = true
			}
			strategyPo, err = m.db.StrategyRepo.WithTx(tx).UpdateStrategy(ctx, repo.UpdateStrategyParams{
				ID:          req.Id,
				Name:        &req.Name,
				Description: &req.Description,
				Status:      sts,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to update strategy: %w", err)
			}
		}

		return strategyToTypes(strategyPo, snapshotPo)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update strategy: %w", err)
	}
	return result.(*stypes.Strategy), nil
}

// GetStrategy 获取策略
func (m *strategyManager) GetStrategy(ctx context.Context, id string) (*stypes.Strategy, error) {
	strategyPo, err := m.db.StrategyRepo.GetById(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get strategy: %w", err)
	}
	if strategyPo == nil {
		return nil, fmt.Errorf("strategy not found")
	}

	snapshotPo, err := m.db.SnapshotRepo.GetActicedSnapshot(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}
	if snapshotPo == nil {
		return nil, fmt.Errorf("snapshot not found")
	}

	return strategyToTypes(strategyPo, snapshotPo)
}

// GetStrategyByNameVersion 根据名称和版本获取策略
func (m *strategyManager) GetStrategyByVersion(ctx context.Context, id, version string) (*stypes.Strategy, error) {
	strategyPo, err := m.db.StrategyRepo.GetById(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get strategy: %w", err)
	}
	if strategyPo == nil {
		return nil, fmt.Errorf("strategy not found")
	}

	var snapshotPo *snapshot.Snapshot
	switch version {
	case "Latest":
		snapshotPo, err = m.db.SnapshotRepo.GetLatestSnapshot(ctx, id)
	case "Active":
		snapshotPo, err = m.db.SnapshotRepo.GetActicedSnapshot(ctx, id)
	default:
		snapshotPo, err = m.db.SnapshotRepo.GetByStrategyIdAndVersion(ctx, snapshot.GetByStrategyIdAndVersionParams{
			StrategyID: id,
			Version:    version,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}
	if snapshotPo == nil {
		return nil, fmt.Errorf("snapshot not found")
	}

	return strategyToTypes(strategyPo, snapshotPo)
}

// ListStrategies 列出策略
func (m *strategyManager) ListStrategies(ctx context.Context, filter *stypes.StrategyFilter) ([]*stypes.Strategy, error) {
	var repoFilter repo.ListStrategiesParams
	if filter != nil {
		if filter.Id != nil {
			repoFilter.ID = filter.Id
		}
		if filter.Status != nil {
			status := repo.NullStrategyStatus{
				Valid:          true,
				StrategyStatus: repo.StrategyStatus(*filter.Status),
			}
			repoFilter.Status = status
		}
		if filter.Name != nil {
			repoFilter.Name = filter.Name
		}
		if filter.CreatedAtStart != nil {
			t := time.Unix(*filter.CreatedAtStart, 0)
			repoFilter.CreatedAtStart = &t
		}
		if filter.CreatedAtEnd != nil {
			t := time.Unix(*filter.CreatedAtEnd, 0)
			repoFilter.CreatedAtEnd = &t
		}
	}

	strategyPos, err := m.db.StrategyRepo.ListStrategies(ctx, repoFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list strategies: %w", err)
	}

	results := make([]*stypes.Strategy, 0, len(strategyPos))
	for _, strategyPo := range strategyPos {
		var snapshotPo *snapshot.Snapshot
		var err error

		// 根据 version 参数选择不同的版本
		if filter != nil && filter.Version != nil {
			version := *filter.Version
			switch version {
			case "Latest":
				// 获取最新版本（按 created_at DESC）
				snapshotPo, err = m.db.SnapshotRepo.GetLatestSnapshot(ctx, strategyPo.ID)
			case "Active":
				// 获取激活版本（is_active = TRUE）
				snapshotPo, err = m.db.SnapshotRepo.GetActicedSnapshot(ctx, strategyPo.ID)
			default:
				// 具体版本号
				snapshotPo, err = m.db.SnapshotRepo.GetByStrategyIdAndVersion(ctx, snapshot.GetByStrategyIdAndVersionParams{
					StrategyID: strategyPo.ID,
					Version:    version,
				})
			}
		} else {
			// 默认获取激活版本
			snapshotPo, err = m.db.SnapshotRepo.GetActicedSnapshot(ctx, strategyPo.ID)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to get snapshot: %w", err)
		}
		if snapshotPo == nil {
			// 如果找不到对应的版本，跳过该策略
			continue
		}
		strategy, err := strategyToTypes(&strategyPo, snapshotPo)
		if err != nil {
			return nil, fmt.Errorf("failed to convert strategy to types: %w", err)
		}
		results = append(results, strategy)
	}
	return results, nil
}

// CountStrategies 统计策略总数（仅统计未删除的）
func (m *strategyManager) CountStrategies(ctx context.Context) (int64, error) {
	count, err := m.db.StrategyRepo.CountStrategies(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count strategies: %w", err)
	}
	if count == nil {
		return 0, nil
	}
	return *count, nil
}

// DeleteStrategy 删除策略
func (m *strategyManager) DeleteStrategy(ctx context.Context, id string) error {
	// 校验是否存在关联的 Bot 实例，如有则不允许删除
	bots, err := m.db.BotRepo.ListBots(ctx, bot.ListBotsParams{
		StrategyID: &id,
		Mode:       bot.NullRunMode{},
		Status:     bot.NullBotStatus{},
	})
	if err != nil {
		return fmt.Errorf("failed to list bots for strategy: %w", err)
	}
	if len(bots) > 0 {
		return fmt.Errorf("该策略下存在 %d 个 Bot 实例，不允许删除。请先删除相关 Bot", len(bots))
	}

	_, err = m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		strategyPo, err := m.db.StrategyRepo.WithTx(tx).LockStrategy(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to lock strategy: %w", err)
		}
		if strategyPo == nil {
			return nil, fmt.Errorf("strategy not found")
		}

		_, err = m.db.SnapshotRepo.WithTx(tx).DeleteByStrategyId(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to delete snapshot: %w", err)
		}

		return m.db.StrategyRepo.WithTx(tx).DeleteStrategy(ctx, id)
	})
	if err != nil {
		return fmt.Errorf("failed to delete strategy: %w", err)
	}
	return nil
}

func (m *strategyManager) ActiveStrategy(ctx context.Context, id string) error {
	_, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		strategyPo, err := m.db.StrategyRepo.WithTx(tx).LockStrategy(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to lock strategy: %w", err)
		}
		if strategyPo == nil {
			return nil, fmt.Errorf("strategy not found")
		}

		return m.db.StrategyRepo.WithTx(tx).UpdateStrategyStatus(ctx, repo.UpdateStrategyStatusParams{
			ID:     id,
			Status: repo.StrategyStatusActive,
		})
	})
	return err
}

func (m *strategyManager) InactiveStrategy(ctx context.Context, id string) error {
	_, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		strategyPo, err := m.db.StrategyRepo.WithTx(tx).LockStrategy(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to lock strategy: %w", err)
		}
		if strategyPo == nil {
			return nil, fmt.Errorf("strategy not found")
		}

		return m.db.StrategyRepo.WithTx(tx).UpdateStrategyStatus(ctx, repo.UpdateStrategyStatusParams{
			ID:     id,
			Status: repo.StrategyStatusInactive,
		})
	})
	return err
}

func getStrategyVersion(id string, code string, parentId int32, paramsVersion string, sigHash string) string {
	raw := fmt.Sprintf("%s:%s:%d:%s:%s", id, code, parentId, paramsVersion, sigHash)
	hash := sha256.Sum256([]byte(raw))
	v := hex.EncodeToString(hash[:])
	return v[:8]
}

func canonicalizeParams(params []*stypes.StrategyParam) ([]byte, string, error) {
	// 将参数按字典序排序
	sort.SliceStable(params, func(i, j int) bool {
		return params[i].Name < params[j].Name
	})

	sb := strings.Builder{}
	for _, param := range params {
		defaultValue := ""
		if param.Default != nil {
			str, err := sonic.Marshal(param.Default)
			if err != nil {
				return nil, "", err
			}
			defaultValue = string(str)
		}
		sb.WriteString(fmt.Sprintf("%s:%s:%t:%s", param.Name, param.Type, param.Required, defaultValue))
	}

	h := sha256.Sum256([]byte(sb.String()))
	hash := hex.EncodeToString(h[:])
	if len(hash) > 10 {
		hash = hash[:10]
	}

	jsParams, err := sonic.Marshal(params)
	if err != nil {
		return nil, "", err
	}

	return jsParams, hash, nil
}

func canonicalizeSignals(signals []*stypes.SignalDefinition) ([]byte, string, error) {
	// normalize + stable sort
	normalized := make([]json.RawMessage, len(signals))
	for i, s := range signals {
		// normalize empty props to nil for stable encoding
		if len(s.Props) == 0 {
			s.Props = nil
		}
		// normalize empty id to ""
		s.ID = strings.TrimSpace(s.ID)
		// normalize empty exchange/symbol to ""
		v := map[string]any{
			"type": s.Type.String(),
		}
		if s.ID != "" {
			v["id"] = s.ID
		}
		if s.Exchange != nil {
			v["exchange"] = s.Exchange.String()
		}
		if s.Symbol != nil {
			v["symbol"] = s.Symbol.String()
		}
		if len(s.Props) > 0 {
			v["props"] = s.Props
		}
		b, err := sonic.Marshal(v)
		if err != nil {
			return nil, "", err
		}
		normalized[i] = b
	}
	sort.SliceStable(signals, func(i, j int) bool {
		return string(normalized[i]) < string(normalized[j])
	})

	signalsBytes, err := sonic.Marshal(signals)
	if err != nil {
		return nil, "", err
	}

	h := sha256.Sum256(signalsBytes)
	hash := hex.EncodeToString(h[:])
	if len(hash) > 10 {
		hash = hash[:10]
	}

	return signalsBytes, hash, nil
}

func strategyToTypes(po *repo.Strategy, snapshotPo *snapshot.Snapshot) (*stypes.Strategy, error) {
	var sp []stypes.StrategyParam
	err := sonic.Unmarshal(snapshotPo.Params, &sp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal params: %w", err)
	}

	var sd []stypes.SignalDefinition
	err = sonic.Unmarshal(snapshotPo.Signals, &sd)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal signals: %w", err)
	}
	return &stypes.Strategy{
		ID:          po.ID,
		Name:        po.Name,
		Description: po.Description,
		Code:        snapshotPo.Code,
		Version:     snapshotPo.Version,
		Params:      sp,
		Signals:     sd,
		Status:      stypes.StrategyStatus(po.Status),
		CreatedAt:   po.CreatedAt,
		UpdatedAt:   snapshotPo.UpdatedAt,
	}, nil
}
