package manager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/jackc/pgx/v5"
	"github.com/samber/lo"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/llt-trade/server/pkg/converter"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
	"github.com/wangliang139/llt-trade/server/pkg/repos/bot"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/registry"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	"github.com/wangliang139/mow/logger"
)

// BotManager Bot管理器接口
type BotManager interface {
	CreateBot(ctx context.Context, req *stypes.CreateBotInput) (*stypes.Bot, error)
	UpdateBot(ctx context.Context, req *stypes.UpdateBotInput) (*stypes.Bot, error)
	StartBot(ctx context.Context, botID int32) error
	StopBot(ctx context.Context, botID int32) error
	UpdateBotStatus(ctx context.Context, botID int32, status stypes.BotStatus, errorMessage *string) error
	UpgradeBot(ctx context.Context, botID int32) (*stypes.Bot, bool, string, error)
	GetBot(ctx context.Context, botID int32) (*stypes.Bot, error)
	ListBots(ctx context.Context, filter *stypes.BotFilter) ([]*stypes.Bot, error)
	CountBots(ctx context.Context, filter *stypes.BotFilter) (int64, error)
	DeleteBot(ctx context.Context, botID int32) error
}

// botManager Bot管理器实现
type botManager struct {
	db               *repos.Entity
	strategyManager  StrategyManager
	executorRegistry *registry.ExecutorRegistry
}

// NewBotManager 创建Bot管理器
func NewBotManager(db *repos.Entity, strategyManager StrategyManager, executorRegistry *registry.ExecutorRegistry) BotManager {
	return &botManager{
		db:               db,
		strategyManager:  strategyManager,
		executorRegistry: executorRegistry,
	}
}

// CreateBot 创建Bot
func (m *botManager) CreateBot(ctx context.Context, req *stypes.CreateBotInput) (*stypes.Bot, error) {
	if req.StrategyID == "" {
		return nil, fmt.Errorf("strategy ID is required")
	}
	if !req.Mode.Valid() {
		return nil, fmt.Errorf("invalid bot mode: %s", req.Mode)
	}
	if len(req.Name) == 0 {
		return nil, fmt.Errorf("name is required")
	}
	if req.AccountID == "" {
		return nil, fmt.Errorf("account ID is required")
	}
	if len(req.Config.Signals) == 0 {
		return nil, fmt.Errorf("signals is required")
	}
	if len(req.Symbols) == 0 {
		return nil, fmt.Errorf("symbols is required")
	}
	if !req.Exchange.IsValid() {
		return nil, fmt.Errorf("invalid exchange: %s", req.Exchange)
	}

	// 验证策略存在
	strategy, err := m.strategyManager.GetStrategyByVersion(ctx, req.StrategyID, req.StrategyVer)
	if err != nil {
		return nil, fmt.Errorf("failed to get strategy: %w", err)
	}
	if strategy == nil {
		return nil, fmt.Errorf("strategy not found")
	}

	// 参数校验与补默认值
	if req.Config.Params == nil {
		req.Config.Params = make(map[string]any)
	}
	for _, p := range strategy.Params {
		if val, ok := req.Config.Params[p.Name]; ok {
			req.Config.Params[p.Name] = val
		} else if p.Required {
			return nil, fmt.Errorf("missing required param: %s", p.Name)
		} else if p.Default != nil {
			req.Config.Params[p.Name] = p.Default
		}
	}

	// 校验 account_id 唯一性
	existingBot, err := m.db.BotRepo.GetBotByAccountID(ctx, req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to check account binding: %w", err)
	}
	if existingBot != nil {
		return nil, fmt.Errorf("account id already bound")
	}

	symbols := make([]string, 0)
	for _, symbol := range req.Symbols {
		symbols = append(symbols, symbol.String())
	}

	// 保存到数据库
	botPo, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		// 构建 config JSONB：合并 config、params、signals
		configBytes := []byte("{}")
		configBytes, err = sonic.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}

		return m.db.BotRepo.WithTx(tx).CreateBot(ctx, bot.CreateBotParams{
			StrategyID:      req.StrategyID,
			StrategyVersion: req.StrategyVer,
			Exchange:        req.Exchange.String(),
			AccountID:       req.AccountID,
			Mode:            bot.RunMode(req.Mode),
			Name:            req.Name,
			Desc:            req.Description,
			Config:          configBytes,
			Symbols:         symbols,
			Status:          bot.BotStatus(stypes.BotStatusStopped),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	return converter.BotPo2Types(botPo.(*bot.Bot))
}

// UpdateBot 更新Bot（先停止 -> 更新DB -> 启动）
func (m *botManager) UpdateBot(ctx context.Context, req *stypes.UpdateBotInput) (*stypes.Bot, error) {
	if req.ID == 0 {
		return nil, fmt.Errorf("bot ID is required")
	}
	if len(req.Name) == 0 {
		return nil, fmt.Errorf("name is required")
	}

	// 获取当前 Bot 状态
	currentBot, err := m.GetBot(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bot: %w", err)
	}

	// 先停止 Bot（如果正在运行）
	wasRunning := currentBot.Status == stypes.BotStatusRunning
	if wasRunning {
		if err := m.StopBot(ctx, req.ID); err != nil {
			return nil, fmt.Errorf("failed to stop bot: %w", err)
		}
	}

	// 更新数据库
	configBytes := []byte(req.Config)
	botPo, err := m.db.BotRepo.UpdateBot(ctx, bot.UpdateBotParams{
		ID:      req.ID,
		Name:    req.Name,
		Desc:    req.Description,
		Symbols: req.Symbols,
		Config:  configBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update bot: %w", err)
	}

	updatedBot, err := converter.BotPo2Types(botPo)
	if err != nil {
		return nil, fmt.Errorf("failed to convert bot: %w", err)
	}

	// 如果之前是运行状态，重新启动
	if wasRunning {
		if err := m.StartBot(ctx, req.ID); err != nil {
			logger.Ctx(ctx).Warn().Int32("bot_id", req.ID).Err(err).Msg("failed to restart bot after update")
		}
	}

	logger.Ctx(ctx).Info().Int32("bot_id", req.ID).Bool("restarted", wasRunning).Msg("bot updated")
	return updatedBot, nil
}

// StartBot 启动Bot
func (m *botManager) StartBot(ctx context.Context, botID int32) error {
	bot, err := m.GetBot(ctx, botID)
	if err != nil {
		return err
	}

	if bot.Status == stypes.BotStatusRunning {
		return types.ErrBotAlreadyRunning
	}

	// 获取策略
	strategy, err := m.strategyManager.GetStrategyByVersion(ctx, bot.StrategyID, bot.StrategyVer)
	if err != nil {
		return fmt.Errorf("failed to get strategy: %w", err)
	}

	// 创建并启动执行器
	if m.executorRegistry == nil {
		return fmt.Errorf("executor registry is not initialized")
	}
	if err := m.executorRegistry.Start(ctx, bot, strategy); err != nil {
		if errors.Is(err, types.ErrBotAlreadyRunning) || errors.Is(err, types.ErrBotAlreadyStarting) {
			return err
		}
		return fmt.Errorf("failed to start executor: %w", err)
	}

	// 更新数据库状态为 running
	if err := m.UpdateBotStatus(ctx, botID, stypes.BotStatusRunning, nil); err != nil {
		// 更新失败，停止执行器
		if m.executorRegistry != nil {
			_ = m.executorRegistry.Stop(ctx, botID)
		}
		return fmt.Errorf("failed to update bot status: %w", err)
	}

	logger.Ctx(ctx).Info().Int32("bot_id", botID).Msg("bot started")
	return nil
}

// UpgradeBot 升级 Bot：停止 -> 更新策略版本为最新 -> 启动
//
// 返回：
// - bot：升级后的 bot（尽量返回最新状态）
// - started：是否启动成功（升级后启动失败时为 false，但不会返回 error）
// - message：启动失败等提示信息
func (m *botManager) UpgradeBot(ctx context.Context, botID int32) (*stypes.Bot, bool, string, error) {
	current, err := m.GetBot(ctx, botID)
	if err != nil {
		return nil, false, "", err
	}

	latest, err := m.strategyManager.GetStrategy(ctx, current.StrategyID)
	if err != nil {
		return current, false, "", fmt.Errorf("failed to get strategy: %w", err)
	}
	if latest == nil {
		return current, false, "", fmt.Errorf("strategy not found")
	}

	// 策略状态不是激活：不可升级
	if latest.Status != stypes.StrategyStatusActive {
		return current, true, "策略未激活，不可升级", nil
	}

	if latest.Version == current.StrategyVer {
		return current, true, "已是最新激活版本，无需升级", nil
	}

	// 先停止
	if err := m.StopBot(ctx, botID); err != nil {
		return current, false, "", fmt.Errorf("failed to stop bot: %w", err)
	}

	// 更新版本（并重置状态为 stopped/清空 error_message）
	updatedPo, err := m.db.BotRepo.UpdateBotStrategyVersion(ctx, bot.UpdateBotStrategyVersionParams{
		ID:              botID,
		StrategyVersion: latest.Version,
	})
	if err != nil {
		return current, false, "", fmt.Errorf("failed to update bot strategy version: %w", err)
	}
	updated, err := converter.BotPo2Types(updatedPo)
	if err != nil {
		return current, false, "", fmt.Errorf("failed to convert bot: %w", err)
	}

	// 尝试启动：启动失败不返回 error（前端已提前提示可能启动失败）
	if err := m.StartBot(ctx, botID); err != nil {
		after, getErr := m.GetBot(ctx, botID)
		if getErr == nil && after != nil {
			updated = after
		}
		return updated, false, err.Error(), nil
	}

	after, getErr := m.GetBot(ctx, botID)
	if getErr == nil && after != nil {
		updated = after
	}
	return updated, true, "", nil
}

// StopBot 停止Bot
func (m *botManager) StopBot(ctx context.Context, botID int32) error {
	bot, err := m.GetBot(ctx, botID)
	if err != nil {
		return err
	}
	if bot == nil || bot.Status != stypes.BotStatusRunning {
		return nil
	}

	// 停止执行器
	if m.executorRegistry != nil {
		if _, exists := m.executorRegistry.Get(botID); exists {
			if err := m.executorRegistry.Stop(ctx, botID); err != nil {
				return fmt.Errorf("failed to stop executor: %w", err)
			}
		}
	}

	// 更新数据库状态为 stopped
	if err := m.UpdateBotStatus(ctx, botID, stypes.BotStatusStopped, nil); err != nil {
		return fmt.Errorf("failed to update bot status: %w", err)
	}

	logger.Ctx(ctx).Info().Int32("bot_id", botID).Msg("bot stopped")
	return nil
}

// UpdateBotStatus 更新 Bot 状态
func (m *botManager) UpdateBotStatus(ctx context.Context, botID int32, status stypes.BotStatus, errorMessage *string) error {
	statusStr := string(status)
	_, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		return m.db.BotRepo.WithTx(tx).UpdateBotStatus(ctx, bot.UpdateBotStatusParams{
			ID:           botID,
			Status:       bot.BotStatus(statusStr),
			ErrorMessage: lo.FromPtr(errorMessage),
		})
	})
	return err
}

// GetBot 获取Bot
func (m *botManager) GetBot(ctx context.Context, botID int32) (*stypes.Bot, error) {
	botPo, err := m.db.BotRepo.GetBot(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bot: %w", err)
	}
	if botPo == nil {
		return nil, fmt.Errorf("bot not found")
	}
	return converter.BotPo2Types(botPo)
}

// ListBots 列出Bot
func (m *botManager) ListBots(ctx context.Context, filter *stypes.BotFilter) ([]*stypes.Bot, error) {
	var id *int32
	var strategyID *string
	var mode *bot.RunMode
	var status *bot.BotStatus
	var exchange *string
	var accountID *string
	var name *string
	var createdAtStart *time.Time
	var createdAtEnd *time.Time

	if filter != nil {
		if filter.ID != nil {
			idVal := int32(*filter.ID)
			id = &idVal
		}
		if filter.StrategyID != nil {
			strategyID = filter.StrategyID
		}
		if filter.Mode != nil {
			runMode := bot.RunMode(*filter.Mode)
			mode = &runMode
		}
		if filter.Status != nil {
			botStatus := bot.BotStatus(*filter.Status)
			status = &botStatus
		}
		if filter.Exchange != nil {
			exchange = filter.Exchange
		}
		if filter.AccountID != nil {
			accountID = filter.AccountID
		}
		if filter.Name != nil {
			name = filter.Name
		}
		if filter.CreatedAtStart != nil {
			t := time.Unix(*filter.CreatedAtStart, 0)
			createdAtStart = &t
		}
		if filter.CreatedAtEnd != nil {
			t := time.Unix(*filter.CreatedAtEnd, 0)
			createdAtEnd = &t
		}
	}

	botPos, err := m.db.BotRepo.ListBots(ctx, bot.ListBotsParams{
		ID:             id,
		StrategyID:     strategyID,
		Mode:           getNullRunMode(mode),
		Status:         getNullBotStatus(status),
		Exchange:       exchange,
		AccountID:      accountID,
		Name:           name,
		CreatedAtStart: createdAtStart,
		CreatedAtEnd:   createdAtEnd,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list bots: %w", err)
	}

	results := make([]*stypes.Bot, 0, len(botPos))
	for i := range botPos {
		bot, err := converter.BotPo2Types(&botPos[i])
		if err != nil {
			return nil, fmt.Errorf("failed to convert bot: %w", err)
		}
		results = append(results, bot)
	}
	return results, nil
}

// CountBots 统计 Bot 数量（支持按 status 过滤）
func (m *botManager) CountBots(ctx context.Context, filter *stypes.BotFilter) (int64, error) {
	var id *int32
	var strategyID *string
	var mode *bot.RunMode
	var status *bot.BotStatus
	var exchange *string
	var accountID *string
	var name *string
	var createdAtStart *time.Time
	var createdAtEnd *time.Time

	if filter != nil {
		if filter.ID != nil {
			idVal := int32(*filter.ID)
			id = &idVal
		}
		strategyID = filter.StrategyID
		if filter.Mode != nil {
			runMode := bot.RunMode(*filter.Mode)
			mode = &runMode
		}
		if filter.Status != nil {
			botStatus := bot.BotStatus(*filter.Status)
			status = &botStatus
		}
		if filter.Exchange != nil {
			exchange = filter.Exchange
		}
		if filter.AccountID != nil {
			accountID = filter.AccountID
		}
		if filter.Name != nil {
			name = filter.Name
		}
		if filter.CreatedAtStart != nil {
			t := time.UnixMilli(*filter.CreatedAtStart)
			createdAtStart = &t
		}
		if filter.CreatedAtEnd != nil {
			t := time.UnixMilli(*filter.CreatedAtEnd)
			createdAtEnd = &t
		}
	}
	count, err := m.db.BotRepo.CountBots(ctx, bot.CountBotsParams{
		ID:             id,
		StrategyID:     strategyID,
		Mode:           getNullRunMode(mode),
		Status:         getNullBotStatus(status),
		Exchange:       exchange,
		AccountID:      accountID,
		Name:           name,
		CreatedAtStart: createdAtStart,
		CreatedAtEnd:   createdAtEnd,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count bots: %w", err)
	}
	if count == nil {
		return 0, nil
	}
	return *count, nil
}

func getNullRunMode(m *bot.RunMode) bot.NullRunMode {
	if m == nil {
		return bot.NullRunMode{Valid: false}
	}
	return bot.NullRunMode{RunMode: *m, Valid: true}
}

func getNullBotStatus(s *bot.BotStatus) bot.NullBotStatus {
	if s == nil {
		return bot.NullBotStatus{Valid: false}
	}
	return bot.NullBotStatus{BotStatus: *s, Valid: true}
}

// DeleteBot 删除Bot
func (m *botManager) DeleteBot(ctx context.Context, botID int32) error {
	// 先停止Bot
	if err := m.StopBot(ctx, botID); err != nil {
		return fmt.Errorf("failed to stop bot before deletion: %w", err)
	}

	// 删除数据库记录
	_, err := m.db.BotRepo.DeleteBot(ctx, botID)
	if err != nil {
		return fmt.Errorf("failed to delete bot: %w", err)
	}

	logger.Ctx(ctx).Info().Int32("bot_id", botID).Msg("bot deleted")
	return nil
}
