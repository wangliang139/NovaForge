package account

import (
	"context"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/common/metrics"
	"github.com/wangliang139/llt-trade/server/pkg/repos/equity"
	"github.com/wangliang139/llt-trade/server/pkg/repos/orders"
	"github.com/wangliang139/llt-trade/server/pkg/repos/symbol_equity"
	"github.com/wangliang139/llt-trade/server/pkg/utils"
)

// AccountMetricsInput 账户指标查询输入
type AccountMetricsInput struct {
	AccountID string
	Symbol    *string
	StartTs   time.Time
	EndTs     time.Time
	Dimension int // 1=ACCOUNT, 2=SYMBOL (对应 metrics.v1.MetricsDimension)
}

// AccountMetricsResult 账户指标计算结果
type AccountMetricsResult struct {
	Cagr                  float64
	Sharpe                float64
	Sortino               float64
	MaxDrawdown           float64
	TimeUnderWaterSeconds int64
	Calmar                float64
	WinRate               float64
	ProfitFactor          float64
	RollingSharpe         float64
	AvgSlippageBps        float64
	FeeRatio              float64
	MaxConsecutiveLoss    int32
	StartTs               int64
	EndTs                 int64
	SymbolMetrics         []SymbolMetricsResult
}

// SymbolMetricsResult 标的级指标
type SymbolMetricsResult struct {
	Symbol                string
	Exchange              string
	Cagr                  float64
	Sharpe                float64
	Sortino               float64
	MaxDrawdown           float64
	TimeUnderWaterSeconds int64
	Calmar                float64
	WinRate               float64
	ProfitFactor          float64
	RollingSharpe         float64
	AvgSlippageBps        float64
	FeeRatio              float64
	MaxConsecutiveLoss    int32
}

// QueryAccountMetrics 查询账户指标
func (e *Entity) QueryAccountMetrics(ctx context.Context, input AccountMetricsInput) (*AccountMetricsResult, error) {
	// 1. 获取权益曲线
	equityRows, err := e.db.EquityRepo.ListEquityByAccountAndRange(ctx, equity.ListEquityByAccountAndRangeParams{
		AccountID: input.AccountID,
		Ts:        input.StartTs,
		Ts_2:      input.EndTs,
	})
	if err != nil {
		return nil, err
	}

	// 2. 获取订单（无 bot_id 过滤）
	orderRows, err := e.db.OrdersRepo.ListOrdersByAccountAndTimeRange(ctx, orders.ListOrdersByAccountAndTimeRangeParams{
		AccountID:   input.AccountID,
		CreatedTs:   input.StartTs,
		CreatedTs_2: input.EndTs,
		Limit:       10000,
		BotID:       nil,
		Symbol:      input.Symbol,
	})
	if err != nil {
		return nil, err
	}

	// 3. 构建权益点
	equityPoints := make([]metrics.EquityPoint, 0, len(equityRows))
	for _, row := range equityRows {
		n := utils.Decimal.PgNumericToDecimal(row.Notional)
		f, _ := n.Float64()
		equityPoints = append(equityPoints, metrics.EquityPoint{Ts: row.Ts.Unix(), Notional: f})
	}

	// 4. 构建订单（仅 DONE/PARTIAL_DONE 有 realized_pnl）
	ordersForMetrics := make([]metrics.OrderForMetrics, 0, len(orderRows))
	for _, row := range orderRows {
		if row.Status != "DONE" && row.Status != "PARTIAL_DONE" {
			continue
		}
		ordersForMetrics = append(ordersForMetrics, metrics.OrderForMetrics{
			RealizedPnl: utils.Decimal.PgNumericToDecimal(row.RealizedPnl),
			Fee:         utils.Decimal.PgNumericToDecimal(row.Fee),
			AvgPrice:    utils.Decimal.PgNumericToDecimal(row.AvgPrice),
			Price:       utils.Decimal.PgNumericToDecimal(row.Price),
			ExecutedQty: utils.Decimal.PgNumericToDecimal(row.ExecutedQty),
			OrderType:   string(row.OrderType),
			Symbol:      row.Symbol,
			Exchange:    row.Exchange,
		})
	}

	// 5. 计算账户级指标
	result := &AccountMetricsResult{
		StartTs: input.StartTs.Unix(),
		EndTs:   input.EndTs.Unix(),
	}
	if len(equityPoints) >= 2 {
		result.Cagr = metrics.CalculateCAGR(equityPoints)
		result.Sharpe = metrics.CalculateSharpeRatio(equityPoints)
		result.Sortino = metrics.CalculateSortino(equityPoints)
		result.MaxDrawdown = metrics.CalculateMaxDrawdown(equityPoints)
		result.TimeUnderWaterSeconds = metrics.CalculateTimeUnderWater(equityPoints)
		result.Calmar = metrics.CalculateCalmar(equityPoints)
		result.RollingSharpe = metrics.CalculateRollingSharpe(equityPoints, 20)
	}
	if len(ordersForMetrics) > 0 {
		result.WinRate = metrics.CalculateWinRate(ordersForMetrics)
		result.ProfitFactor = metrics.CalculateProfitFactor(ordersForMetrics)
		result.FeeRatio = metrics.CalculateFeeRatio(ordersForMetrics)
		result.MaxConsecutiveLoss = metrics.CalculateMaxConsecutiveLoss(ordersForMetrics)
		result.AvgSlippageBps = metrics.CalculateAvgSlippage(ordersForMetrics)
	}

	// 6. SYMBOL 维度：按 symbol 分组计算
	if input.Dimension == 2 {
		symbolEquityRows, err := e.db.SymbolEquityRepo.ListSymbolEquityByAccountAndRange(ctx, symbol_equity.ListSymbolEquityByAccountAndRangeParams{
			AccountID: input.AccountID,
			Ts:        input.StartTs,
			Ts_2:      input.EndTs,
			Exchange:  nil,
			Symbol:    input.Symbol,
		})
		if err != nil {
			return nil, err
		}

		// 按 (exchange, symbol) 分组
		equityBySymbol := make(map[string][]metrics.EquityPoint)
		for _, row := range symbolEquityRows {
			key := row.Exchange + "|" + row.Symbol
			n := utils.Decimal.PgNumericToDecimal(row.NetValue)
			f, _ := n.Float64()
			equityBySymbol[key] = append(equityBySymbol[key], metrics.EquityPoint{Ts: row.Ts.Unix(), Notional: f})
		}

		ordersBySymbol := make(map[string][]metrics.OrderForMetrics)
		for _, o := range ordersForMetrics {
			key := o.Exchange + "|" + o.Symbol
			ordersBySymbol[key] = append(ordersBySymbol[key], o)
		}

		for key, pts := range equityBySymbol {
			ex, sym := splitSymbolKey(key)
			ordList := ordersBySymbol[key]
			sm := SymbolMetricsResult{
				Symbol:   sym,
				Exchange: ex,
			}
			if len(pts) >= 2 {
				sm.Cagr = metrics.CalculateCAGR(pts)
				sm.Sharpe = metrics.CalculateSharpeRatio(pts)
				sm.Sortino = metrics.CalculateSortino(pts)
				sm.MaxDrawdown = metrics.CalculateMaxDrawdown(pts)
				sm.TimeUnderWaterSeconds = metrics.CalculateTimeUnderWater(pts)
				sm.Calmar = metrics.CalculateCalmar(pts)
				sm.RollingSharpe = metrics.CalculateRollingSharpe(pts, 20)
			}
			if len(ordList) > 0 {
				sm.WinRate = metrics.CalculateWinRate(ordList)
				sm.ProfitFactor = metrics.CalculateProfitFactor(ordList)
				sm.FeeRatio = metrics.CalculateFeeRatio(ordList)
				sm.MaxConsecutiveLoss = metrics.CalculateMaxConsecutiveLoss(ordList)
				sm.AvgSlippageBps = metrics.CalculateAvgSlippage(ordList)
			}
			result.SymbolMetrics = append(result.SymbolMetrics, sm)
		}
	}

	return result, nil
}

func splitSymbolKey(key string) (exchange, symbol string) {
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}
