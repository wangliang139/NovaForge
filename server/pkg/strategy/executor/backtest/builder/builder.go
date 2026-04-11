package builder

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/strategy"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/executor/backtest/collectors"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/marketdata"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

// ResultBuilder 结果构建器
// 汇总所有数据生成 BacktestResult
type ResultBuilder struct {
	collectors        *collectors.Collectors
	orderEngine       strategy.OrderEngine
	accountEngine     strategy.AccountEngine
	marketProvider    marketdata.MarketProvider
	baseCurrency      string
	baseExchange      ctypes.Exchange
	accountIDProvider strategy.AccountIDProvider
}

// NewResultBuilder 创建结果构建器
func NewResultBuilder(
	collectors *collectors.Collectors,
	orderEngine strategy.OrderEngine,
	accountEngine strategy.AccountEngine,
	marketProvider marketdata.MarketProvider,
	baseCurrency string,
	baseExchange ctypes.Exchange,
	accountIDProvider strategy.AccountIDProvider,
) *ResultBuilder {
	return &ResultBuilder{
		collectors:        collectors,
		orderEngine:       orderEngine,
		accountEngine:     accountEngine,
		marketProvider:    marketProvider,
		baseCurrency:      baseCurrency,
		baseExchange:      baseExchange,
		accountIDProvider: accountIDProvider,
	}
}

// BuildResult 构建回测结果
func (b *ResultBuilder) BuildResult(
	ctx context.Context,
	btCtx stypes.BacktestContext,
	config stypes.BacktestConfig,
	startTime, endTime time.Time,
	timeCost int64,
) (*stypes.BacktestResult, error) {
	// 1. 获取权益曲线
	equity := b.collectors.Equity.GetEquity()

	// 2. 计算指标
	sharpe := collectors.CalculateSharpeRatio(equity)
	maxDD := collectors.CalculateMaxDrawdown(equity)

	// 3. 获取初始和最终余额（使用 NetValueCalculator 计算）
	initial := b.collectors.Equity.GetInitial()
	if initial == nil {
		return nil, fmt.Errorf("initial equity point is nil")
	}
	initialBal := initial.TotalNetValue

	final := b.collectors.Equity.GetFinal()
	if final == nil {
		return nil, fmt.Errorf("final equity point is nil")
	}
	finalBal := final.TotalNetValue

	totalPnl := finalBal.Sub(initialBal)

	// 4. 获取成交记录（从 TradeCollector）并统计胜负
	tradeRecords := b.collectors.Trade.GetTrades()
	winTrades := 0
	lossTrades := 0
	for _, trade := range tradeRecords {
		if trade == nil {
			continue
		}
		// 统计胜负
		if trade.RealizedPnl.GreaterThan(decimal.Zero) {
			winTrades++
		} else if trade.RealizedPnl.LessThan(decimal.Zero) {
			lossTrades++
		}
	}

	// 5. 计算胜率
	winRate := 0.0
	if winTrades+lossTrades > 0 {
		winRate = float64(winTrades) / float64(winTrades+lossTrades)
	}

	_allOrders, err := b.orderEngine.GetAllOrders(ctx, "")
	_ = _allOrders

	// 6. 获取所有订单（从 OrderCollector，包含所有已完结订单）
	allOrders := b.collectors.Order.GetAllOrders()

	// 7. 获取按标的汇总（在 ResultBuilder 中计算）
	symbols, err := b.calculateSummaryBySymbols(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate summary by symbols: %w", err)
	}

	// 8. 获取日志
	logs := b.collectors.Log.GetLogs()
	logTotal, logMaxCache := b.collectors.Log.GetLogStats()

	// 9. 构建结果数据
	data := &stypes.BacktestResultData{
		Symbols:     symbols,
		Equity:      equity,
		Orders:      allOrders,
		Trades:      tradeRecords,
		ConsoleLogs: logs,
		Meta:        make(map[string]any),
	}

	if logTotal > 0 {
		data.Meta["console_logs_total"] = logTotal
		data.Meta["console_logs_max_cache"] = logMaxCache
	}

	// 10. 构建最终结果
	result := &stypes.BacktestResult{
		ID:             btCtx.ID,
		JobID:          btCtx.ID,
		StrategyID:     btCtx.StrategyID,
		StrategyVer:    btCtx.StrategyVer,
		StartTime:      startTime,
		EndTime:        endTime,
		TimeCost:       timeCost,
		InitialBalance: initialBal.String(),
		FinalBalance:   finalBal.String(),
		TotalPnl:       totalPnl.String(),
		TotalTrades:    len(tradeRecords),
		WinTrades:      winTrades,
		LossTrades:     lossTrades,
		WinRate:        winRate,
		SharpeRatio:    sharpe,
		MaxDrawdown:    maxDD,
		Data:           data,
		CreatedAt:      time.Now(),
	}

	return result, nil
}

// calculateSummaryBySymbols 计算每个 symbol 的摘要（按 BaseCurrency 计价）
func (b *ResultBuilder) calculateSummaryBySymbols(ctx context.Context, config stypes.BacktestConfig) ([]*stypes.SymbolSummary, error) {
	// 1. 获取所有成交记录，按 ExSymbol 分组计算已实现盈亏（区分多空方向）
	trades := b.collectors.Trade.GetTrades()
	realizedByKey := make(map[ctypes.ExSymbolKey]decimal.Decimal)

	// 按方向统计
	type DirectionStats struct {
		RealizedPnl decimal.Decimal
		TradeCount  int
	}
	longStatsByKey := make(map[ctypes.ExSymbolKey]*DirectionStats)
	shortStatsByKey := make(map[ctypes.ExSymbolKey]*DirectionStats)

	for _, trade := range trades {
		if trade == nil {
			continue
		}
		key := trade.ExSymbol.Key()
		realizedByKey[key] = realizedByKey[key].Add(trade.RealizedPnl)

		// 按方向统计
		switch trade.Side {
		case ctypes.PositionSideLong:
			if longStatsByKey[key] == nil {
				longStatsByKey[key] = &DirectionStats{}
			}
			longStatsByKey[key].RealizedPnl = longStatsByKey[key].RealizedPnl.Add(trade.RealizedPnl)
			longStatsByKey[key].TradeCount++
		case ctypes.PositionSideShort:
			if shortStatsByKey[key] == nil {
				shortStatsByKey[key] = &DirectionStats{}
			}
			shortStatsByKey[key].RealizedPnl = shortStatsByKey[key].RealizedPnl.Add(trade.RealizedPnl)
			shortStatsByKey[key].TradeCount++
		}
	}

	// 2. 获取初始和最终快照（按 BaseCurrency 计价）
	// 注意：initialSnapshots 和 finalSnapshots 在 BuildResult 中已经获取，这里可以复用
	// 但为了保持 calculateSummaryBySymbols 的独立性，重新获取
	initialSnapshots := b.collectors.Equity.GetInitial()
	if initialSnapshots == nil {
		return nil, fmt.Errorf("initial equity point is nil")
	}
	initialSymbolMap := make(map[ctypes.ExSymbolKey]stypes.SymbolEquityPoint)
	for _, p := range initialSnapshots.Symbols {
		initialSymbolMap[p.ExSymbol.Key()] = p
	}

	finalSnapshots := b.collectors.Equity.GetFinal()
	if finalSnapshots == nil {
		return nil, fmt.Errorf("final equity point is nil")
	}
	finalSymbolMap := make(map[ctypes.ExSymbolKey]stypes.SymbolEquityPoint)
	for _, p := range finalSnapshots.Symbols {
		finalSymbolMap[p.ExSymbol.Key()] = p
	}

	// 3. 遍历 config.Symbols，为每个 symbol 构建 SymbolSummary
	symbols := make([]*stypes.SymbolSummary, 0, len(config.Symbols))
	for _, symCfg := range config.Symbols {
		exSymbol := ctypes.NewExSymbol(symCfg.Exchange, symCfg.Symbol)
		exSymKey := exSymbol.Key()

		// 获取初始余额
		initBase := decimal.Zero
		initQuote := decimal.Zero
		if init, ok := initialSymbolMap[exSymKey]; ok {
			initBase = init.BaseQty
			initQuote = init.QuoteQty
		}

		// 获取最后价格
		lastPx, _ := b.marketProvider.GetLastPrice(ctx, exSymbol.Exchange, exSymbol.Symbol)
		if lastPx.IsZero() {
			lastPx = decimal.Zero
		}

		// 获取已实现盈亏
		realized := realizedByKey[exSymKey]

		// 使用 BaseCurrency 计价的净值
		initialNet := decimal.Zero
		finalNet := decimal.Zero
		if snap, ok := initialSymbolMap[exSymKey]; ok {
			initialNet = snap.BaseNetValue.Add(snap.QuoteNetValue)
		}
		if snap, ok := finalSymbolMap[exSymKey]; ok {
			finalNet = snap.BaseNetValue.Add(snap.QuoteNetValue)
		}

		accountID := b.accountIDProvider(exSymbol.Exchange, exSymbol.Symbol)
		if accountID == nil {
			return nil, fmt.Errorf("accountID is nil")
		}

		// 从 accountProvider 获取当前余额和持仓
		baseAsset, _ := b.accountEngine.GetAsset(ctx, *accountID, exSymbol.Symbol, exSymbol.GetBase())
		quoteAsset, _ := b.accountEngine.GetAsset(ctx, *accountID, exSymbol.Symbol, exSymbol.GetQuote())

		// 对于合约，需要分别查询多空持仓
		longPosQty := decimal.Zero
		shortPosQty := decimal.Zero
		longAvgPx := decimal.Zero
		shortAvgPx := decimal.Zero

		if exSymbol.Symbol.Type == ctypes.MarketTypeFuture {
			// 查询多头持仓
			position, _ := b.accountEngine.GetPosition(ctx, *accountID, exSymbol.Symbol, ctypes.PositionSideLong)
			if position != nil {
				longPosQty = position.Amount
				longAvgPx = position.EntryPrice
			}
			// 查询空头持仓
			position, _ = b.accountEngine.GetPosition(ctx, *accountID, exSymbol.Symbol, ctypes.PositionSideShort)
			if position != nil {
				shortPosQty = position.Amount
				shortAvgPx = position.EntryPrice
			}
		} else {
			// 现货市场，使用 PositionSideLong 作为默认值
			position, _ := b.accountEngine.GetPosition(ctx, *accountID, exSymbol.Symbol, ctypes.PositionSideLong)
			if position != nil {
				longPosQty = position.Amount
				longAvgPx = position.EntryPrice
			}
		}

		// 计算净持仓（多仓为正，空仓为负）
		posQty := longPosQty.Add(shortPosQty)

		// 计算平均价格（加权平均）
		avgPx := decimal.Zero
		if !posQty.IsZero() {
			totalNotional := longPosQty.Mul(longAvgPx).Add(shortPosQty.Abs().Mul(shortAvgPx))
			totalQty := longPosQty.Add(shortPosQty.Abs())
			if !totalQty.IsZero() {
				avgPx = totalNotional.Div(totalQty)
			}
		}

		finalBase := decimal.Zero
		finalQuote := decimal.Zero
		if baseAsset != nil {
			finalBase = baseAsset.Balance
		}
		if quoteAsset != nil {
			finalQuote = quoteAsset.Balance
		}

		// 计算未实现盈亏（基于持仓*（当前价格-成本价）），而不是残差
		unrealized := b.calculateUnrealizedPnL(ctx, exSymbol, posQty, avgPx, lastPx)

		// 获取按方向统计的数据
		longRealized := decimal.Zero
		longTrades := 0
		if longStats, ok := longStatsByKey[exSymKey]; ok {
			longRealized = longStats.RealizedPnl
			longTrades = longStats.TradeCount
		}

		shortRealized := decimal.Zero
		shortTrades := 0
		if shortStats, ok := shortStatsByKey[exSymKey]; ok {
			shortRealized = shortStats.RealizedPnl
			shortTrades = shortStats.TradeCount
		}

		// 未实现盈亏按方向分配
		// 如果当前持仓量为正，表示多仓持仓；为负表示空仓持仓
		longUnrealized := decimal.Zero
		shortUnrealized := decimal.Zero
		if posQty.GreaterThan(decimal.Zero) {
			// 多仓持仓，未实现盈亏归属于多仓
			longUnrealized = unrealized
		} else if posQty.LessThan(decimal.Zero) {
			// 空仓持仓，未实现盈亏归属于空仓
			shortUnrealized = unrealized
		}

		// 计算净盈亏
		longNetPnl := longRealized.Add(longUnrealized)
		shortNetPnl := shortRealized.Add(shortUnrealized)

		symbols = append(symbols, &stypes.SymbolSummary{
			ExSymbol: exSymbol,

			InitialBase:  initBase,
			InitialQuote: initQuote,
			FinalBase:    finalBase,
			FinalQuote:   finalQuote,

			PositionQty: posQty,
			AvgPrice:    avgPx,
			LastPrice:   lastPx,

			InitialNet: initialNet,
			FinalNet:   finalNet,

			RealizedPnl:   realized,
			UnrealizedPnl: unrealized,
			NetPnl:        finalNet.Sub(initialNet),

			// 按方向区分的盈亏统计
			LongRealizedPnl:    longRealized,
			ShortRealizedPnl:   shortRealized,
			LongUnrealizedPnl:  longUnrealized,
			ShortUnrealizedPnl: shortUnrealized,
			LongNetPnl:         longNetPnl,
			ShortNetPnl:        shortNetPnl,
			LongTrades:         longTrades,
			ShortTrades:        shortTrades,
		})
	}

	return symbols, nil
}

// calculateUnrealizedPnL 计算未实现盈亏（以 BaseCurrency 计价）
// 基于持仓*（当前价格-成本价），而不是残差
func (b *ResultBuilder) calculateUnrealizedPnL(ctx context.Context, exSymbol ctypes.ExSymbol, posQty, avgPx, lastPx decimal.Decimal) decimal.Decimal {
	if posQty.IsZero() || avgPx.IsZero() || lastPx.IsZero() {
		return decimal.Zero
	}

	// 未实现盈亏（quote 计价）
	unrealizedQuote := decimal.Zero

	switch exSymbol.GetType() {
	case ctypes.MarketTypeSpot:
		// 现货：未实现盈亏 = 持仓量 * (当前价 - 成本价)
		unrealizedQuote = posQty.Mul(lastPx.Sub(avgPx))
	case ctypes.MarketTypeFuture:
		// 合约：未实现盈亏取决于持仓方向
		// posQty 已经带符号（正数为多，负数为空）
		unrealizedQuote = posQty.Mul(lastPx.Sub(avgPx))
	}

	// 换算到 BaseCurrency
	quoteAsset := exSymbol.GetQuote()
	quotePrice, err := b.marketProvider.GetPriceInBaseCurrency(ctx, quoteAsset, b.baseCurrency)
	if err != nil {
		return decimal.Zero
	}

	return unrealizedQuote.Mul(quotePrice)
}
