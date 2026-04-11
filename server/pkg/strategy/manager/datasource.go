package manager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/jackc/pgx/v5"
	"github.com/samber/lo"
	"github.com/stumble/wpgx"
	converter "github.com/wangliang139/NovaForge/server/pkg/converter"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	repo_datasource "github.com/wangliang139/NovaForge/server/pkg/repos/datasource"
	"github.com/wangliang139/NovaForge/server/pkg/repos/ds_items"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type DatasourceManager interface {
	GetDatasource(ctx context.Context, id int32) (*types.DataSource, error)
	CreateDatasource(ctx context.Context, req *types.CreateDatasourceInput) (*types.DataSource, int64, error)
	ListDatasources(ctx context.Context, filter *types.DatasourceFilter) ([]*types.DataSource, int64, error)
	DeleteDatasource(ctx context.Context, id int32) error
}

type datasourceManager struct {
	db *repos.Entity
}

func NewDatasourceManager(db *repos.Entity) DatasourceManager {
	return &datasourceManager{
		db: db,
	}
}

func (m *datasourceManager) GetDatasource(ctx context.Context, id int32) (*types.DataSource, error) {
	if id <= 0 {
		return nil, fmt.Errorf("id is invalid")
	}
	ds, err := m.db.DataSourceRepo.GetById(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}
	return converter.DataSourceDb2Types(ds)
}

type klineItemPayload struct {
	Interval string `json:"interval"`
	Open     string `json:"open"`
	High     string `json:"high"`
	Low      string `json:"low"`
	Close    string `json:"close"`
	Volume   string `json:"volume"`
	OpenTs   int64  `json:"open_ts"`
	CloseTs  int64  `json:"close_ts"`
	IsClosed bool   `json:"is_closed"`
}

func (m *datasourceManager) CreateDatasource(ctx context.Context, req *types.CreateDatasourceInput) (*types.DataSource, int64, error) {
	if req == nil {
		return nil, 0, fmt.Errorf("request is required")
	}

	if req.Type != types.SignalTypeKline {
		return nil, 0, fmt.Errorf("unsupported datasource type: %s", req.Type)
	}

	if req.Exchange == nil {
		return nil, 0, fmt.Errorf("exchange is required")
	}
	if req.Symbol == nil {
		return nil, 0, fmt.Errorf("symbol is required")
	}
	if req.StartTs.IsZero() || req.EndTs.IsZero() {
		return nil, 0, fmt.Errorf("start_ts/end_ts is required")
	}
	if !req.StartTs.Before(req.EndTs) {
		return nil, 0, fmt.Errorf("start_ts must be before end_ts")
	}

	if _, ok := req.Props["interval"]; !ok {
		return nil, 0, fmt.Errorf("interval is required")
	}
	itv, ok := req.Props["interval"].(string)
	if !ok {
		return nil, 0, fmt.Errorf("interval is not a string")
	}
	interval := ctypes.Interval(strings.TrimSpace(itv))
	if !interval.Valid() {
		return nil, 0, fmt.Errorf("invalid interval: %s", itv)
	}

	// 1) 拉取历史K线（先拉取，避免占用 DB 事务太久）
	klines, err := m.fetchKlines(ctx, *req.Exchange, *req.Symbol, interval, req.StartTs, req.EndTs)
	if err != nil {
		return nil, 0, err
	}
	if len(klines) == 0 {
		return nil, 0, fmt.Errorf("no klines fetched")
	}

	// 2) 组装 datasource 数据
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("%s:%s:%s:%d-%d", *req.Exchange, *req.Symbol, interval.String(), req.StartTs.Unix(), req.EndTs.Unix())
	}
	description := strings.TrimSpace(req.Description)
	props := map[string]any{
		"interval": interval.String(),
	}
	propsBytes, err := sonic.Marshal(props)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal properties: %w", err)
	}

	var exchangeStr string
	var symbolStr string
	if req.Exchange != nil {
		exchangeStr = req.Exchange.String()
	}
	if req.Symbol != nil {
		symbolStr = req.Symbol.String()
	}

	// 3) 数据库写入（datasource + ds_items）
	var inserted int64
	createdAny, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		dsPo, err := m.db.DataSourceRepo.WithTx(tx).CreateDataSource(ctx, repo_datasource.CreateDataSourceParams{
			Name:        name,
			Description: description,
			Type:        repo_datasource.SignalTypeKline,
			Exchange:    exchangeStr,
			Symbol:      symbolStr,
			Props:       propsBytes,
			StartTs:     req.StartTs,
			EndTs:       req.EndTs,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create datasource: %w", err)
		}

		const chunkSize = 1000
		for start := 0; start < len(klines); start += chunkSize {
			end := start + chunkSize
			if end > len(klines) {
				end = len(klines)
			}
			chunk := klines[start:end]

			dsIDs := make([]int32, len(chunk))
			data := make([][]byte, len(chunk))
			tss := make([]time.Time, len(chunk))
			for i, k := range chunk {
				dsIDs[i] = dsPo.ID
				data[i], err = sonic.Marshal(klineItemPayload{
					Interval: interval.String(),
					Open:     k.Open.String(),
					High:     k.High.String(),
					Low:      k.Low.String(),
					Close:    k.Close.String(),
					Volume:   k.Volume.String(),
					OpenTs:   k.OpenTs.UnixMilli(),
					CloseTs:  k.CloseTs.UnixMilli(),
					IsClosed: k.IsClosed,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to marshal kline payload: %w", err)
				}
				// 以 openTs 作为数据源 item 的 ts，用于回测按时间回放
				tss[i] = k.OpenTs
			}

			n, err := m.db.DsItemsRepo.WithTx(tx).BatchInsert(ctx, ds_items.BatchInsertParams{
				DsID: dsIDs,
				Data: data,
				Ts:   tss,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to batch insert ds_items: %w", err)
			}
			inserted += n
		}

		return dsPo, nil
	})
	if err != nil {
		return nil, 0, err
	}

	dsPo := createdAny.(*repo_datasource.Datasource)
	out, err := converter.DataSourceDb2Types(dsPo)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to convert datasource: %w", err)
	}
	return out, inserted, nil
}

func (m *datasourceManager) fetchKlines(
	ctx context.Context,
	ex ctypes.Exchange,
	symbol ctypes.Symbol,
	interval ctypes.Interval,
	startTs time.Time,
	endTs time.Time,
) ([]*ctypes.Kline, error) {
	limit := 1000
	if ex == ctypes.ExchangeOkx || ex == ctypes.ExchangeOkxTest {
		limit = 300
	}

	// 用 startTs-1ms 作为初始游标，确保 closeTs == startTs 的K线不会被跳过
	cursor := startTs.Add(-time.Millisecond)
	out := make([]*ctypes.Kline, 0)
	seen := make(map[int64]struct{}, 1024) // closeTs milli

	// 避免极端情况下死循环（例如交易所接口返回重复数据）
	for iter := 0; iter < 100000; iter++ {
		if !cursor.Before(endTs) {
			break
		}

		var sArg, eArg *time.Time
		sArg = &cursor
		eArg = &endTs

		batch, err := proxy.GetHisKlines(ctx, ex, symbol, interval, sArg, eArg, &limit)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		// 统一按时间升序
		sort.Slice(batch, func(i, j int) bool {
			return batch[i].CloseTs.Before(batch[j].CloseTs)
		})

		added := 0
		var last time.Time
		for _, k := range batch {
			if k.CloseTs.Before(startTs) || k.CloseTs.After(endTs) {
				continue
			}
			if !k.CloseTs.After(cursor) {
				continue
			}
			ms := k.CloseTs.UnixMilli()
			if _, ok := seen[ms]; ok {
				continue
			}
			seen[ms] = struct{}{}
			out = append(out, k)
			added++
			last = k.CloseTs
		}

		if added == 0 {
			// 没有新增，避免死循环
			break
		}
		if !last.Before(endTs) {
			break
		}

		// 下一页从最后一个 closeTs + 1ms 开始
		cursor = last.Add(time.Millisecond)

		// 如果本次返回量明显小于limit，基本可以认为没有更多数据
		if len(batch) < limit {
			break
		}
	}

	return out, nil
}

func (m *datasourceManager) ListDatasources(ctx context.Context, filter *types.DatasourceFilter) ([]*types.DataSource, int64, error) {
	if filter == nil {
		filter = &types.DatasourceFilter{
			Offset: 0,
			Limit:  100,
		}
	}
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	var tp repo_datasource.NullSignalType
	if filter.Type != nil {
		tp.SignalType = repo_datasource.SignalType(*filter.Type)
		tp.Valid = true
	}

	var exchangeStr *string
	if filter.Exchange != nil {
		exchangeStr = lo.ToPtr(filter.Exchange.String())
	}
	var symbolStr *string
	if filter.Symbol != nil {
		symbolStr = lo.ToPtr(filter.Symbol.String())
	}

	count, err := m.db.DataSourceRepo.CountDatasources(ctx, repo_datasource.CountDatasourcesParams{
		Type:     tp,
		Exchange: exchangeStr,
		Symbol:   symbolStr,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count datasources: %w", err)
	}

	list, err := m.db.DataSourceRepo.ListDatasources(ctx, repo_datasource.ListDatasourcesParams{
		Type:     tp,
		Exchange: exchangeStr,
		Symbol:   symbolStr,
		Limit:    filter.Limit,
		Offset:   filter.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list datasources: %w", err)
	}

	result := make([]*types.DataSource, 0, len(list))
	for _, ds := range list {
		out, err := converter.DataSourceDb2Types(&ds)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to convert datasource: %w", err)
		}
		result = append(result, out)
	}

	return result, *count, nil
}

func (m *datasourceManager) DeleteDatasource(ctx context.Context, id int32) error {
	if id <= 0 {
		return fmt.Errorf("id is invalid")
	}
	if err := m.db.DataSourceRepo.DeleteDatasource(ctx, id); err != nil {
		return fmt.Errorf("failed to delete datasource: %w", err)
	}
	return nil
}
