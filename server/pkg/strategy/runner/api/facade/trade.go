package facade

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/executor/backtest/collectors"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/misc"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/portfolio"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

// TradeFacade 交易外观模式，统一回测和实盘的交易能力
type TradeFacade struct {
	// 回测场景：使用 collector
	tradeCollector *collectors.TradeCollector

	portfolio *portfolio.Portfolio

	// 下单函数（回测/实盘不同实现）
	placeOrderFn  func(ctx context.Context, req *stypes.PlaceOrderCommand) (*stypes.PlaceOrderResult, error)
	cancelOrderFn func(ctx context.Context, req *stypes.CancelOrderCommand) error
	getOrdersFn   func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) ([]*ctypes.Order, error)
	getOrderFn    func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error)

	// 账户相关
	getPositionsFn func(ctx context.Context, exchange ctypes.Exchange, symbol *ctypes.Symbol, side *ctypes.PositionSide) ([]*ctypes.Position, error)
	getLeverageFn  func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error)
	setLeverageFn  func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error

	// AccountID 提供器（用于回测和实盘的 accountID 映射）
	accountIDProvider func(exchange ctypes.Exchange, symbol ctypes.Symbol) *string

	// 是否为回测模式
	isBacktest bool
}

// TradeFacadeConfig 配置
type TradeFacadeConfig struct {
	TradeCollector    *collectors.TradeCollector
	Portfolio         *portfolio.Portfolio
	PlaceOrderFn      func(ctx context.Context, req *stypes.PlaceOrderCommand) (*stypes.PlaceOrderResult, error)
	CancelOrderFn     func(ctx context.Context, req *stypes.CancelOrderCommand) error
	GetOrdersFn       func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) ([]*ctypes.Order, error)
	GetOrderFn        func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error)
	GetPositionsFn    func(ctx context.Context, exchange ctypes.Exchange, symbol *ctypes.Symbol, side *ctypes.PositionSide) ([]*ctypes.Position, error)
	GetLeverageFn     func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error)
	SetLeverageFn     func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error
	AccountIDProvider func(exchange ctypes.Exchange, symbol ctypes.Symbol) *string
	IsBacktest        bool
}

// NewTradeFacade 创建 TradeFacade
func NewTradeFacade(cfg TradeFacadeConfig) *TradeFacade {
	return &TradeFacade{
		tradeCollector:    cfg.TradeCollector,
		portfolio:         cfg.Portfolio,
		placeOrderFn:      cfg.PlaceOrderFn,
		cancelOrderFn:     cfg.CancelOrderFn,
		getOrdersFn:       cfg.GetOrdersFn,
		getOrderFn:        cfg.GetOrderFn,
		getPositionsFn:    cfg.GetPositionsFn,
		getLeverageFn:     cfg.GetLeverageFn,
		setLeverageFn:     cfg.SetLeverageFn,
		accountIDProvider: cfg.AccountIDProvider,
		isBacktest:        cfg.IsBacktest,
	}
}

// Buy 买入
func (f *TradeFacade) Buy(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, opts map[string]any) (map[string]any, error) {
	// 构建下单请求
	req := &stypes.PlaceOrderCommand{
		Exchange: exchange,
		Symbol:   symbol,
		Side:     ctypes.PositionSideLong,
		IsBuy:    true,
	}

	// 允许 opts 覆盖 side/isBuy（主要用于 FUTURE：平仓需要显式 side+方向）
	// - side: "LONG" | "SHORT"
	// - isBuy: true/false
	if sideStr, ok := opts["side"].(string); ok && sideStr != "" {
		side := ctypes.ParsePositionSide(sideStr)
		if !side.Valid() {
			return nil, fmt.Errorf("invalid side: %s", sideStr)
		}
		req.Side = side
	}
	if symbol.Type == ctypes.MarketTypeSpot && req.Side != ctypes.PositionSideLong {
		return nil, fmt.Errorf("invalid side: %s for spot", req.Side)
	}

	// 设置 accountID
	if f.accountIDProvider != nil {
		accountID := f.accountIDProvider(exchange, symbol)
		if accountID != nil {
			req.AccountID = *accountID
		}
	}

	// 解析 opts
	orderType := ctypes.OrderTypeMarket
	if orderTypeStr, ok := opts["type"].(string); ok {
		switch orderTypeStr {
		case "market":
			orderType = ctypes.OrderTypeMarket
		case "limit":
			orderType = ctypes.OrderTypeLimit
		}
	}
	req.OrderType = orderType

	req.Price, _ = misc.AnyToString(opts["price"])
	req.Quantity, _ = misc.AnyToString(opts["quantity"])
	req.QuoteQty, _ = misc.AnyToString(opts["quoteQty"])

	if tif, ok := opts["timeInForce"].(string); ok {
		req.TimeInForce = lo.ToPtr(ctypes.TimeInForce(tif))
	} else {
		req.TimeInForce = lo.ToPtr(ctypes.TimeInForceGTC)
	}

	// 下单
	if f.placeOrderFn == nil {
		return nil, fmt.Errorf("placeOrderFn not configured")
	}

	result, err := f.placeOrderFn(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to place buy order: %w", err)
	}

	return map[string]any{
		"orderId":       result.OrderID,
		"status":        result.Status.String(),
		"executedQty":   result.ExecutedQty,
		"executedPrice": result.ExecutedPrice,
		"error":         result.Error,
	}, nil
}

// Sell 卖出
func (f *TradeFacade) Sell(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, opts map[string]any) (map[string]any, error) {
	// 构建下单请求
	req := &stypes.PlaceOrderCommand{
		Exchange: exchange,
		Symbol:   symbol,
		Side:     ctypes.PositionSideLong,
		IsBuy:    false,
	}

	// 允许 opts 覆盖 side/isBuy（主要用于 FUTURE：平仓需要显式 side+方向）
	// - side: "LONG" | "SHORT"
	// - isBuy: true/false
	if sideStr, ok := opts["side"].(string); ok && sideStr != "" {
		side := ctypes.ParsePositionSide(sideStr)
		if !side.Valid() {
			return nil, fmt.Errorf("invalid side: %s", sideStr)
		}
		req.Side = side
	}
	if symbol.Type == ctypes.MarketTypeSpot && req.Side != ctypes.PositionSideLong {
		return nil, fmt.Errorf("invalid side: %s for spot", req.Side)
	}

	// 设置 accountID
	if f.accountIDProvider != nil {
		accountID := f.accountIDProvider(exchange, symbol)
		if accountID != nil {
			req.AccountID = *accountID
		}
	}

	// 解析 opts
	orderType := ctypes.OrderTypeMarket
	if orderTypeStr, ok := opts["type"].(string); ok {
		switch orderTypeStr {
		case "market":
			orderType = ctypes.OrderTypeMarket
		case "limit":
			orderType = ctypes.OrderTypeLimit
		}
	}
	req.OrderType = orderType

	req.Price, _ = misc.AnyToString(opts["price"])
	req.Quantity, _ = misc.AnyToString(opts["quantity"])
	req.QuoteQty, _ = misc.AnyToString(opts["quoteQty"])

	if tif, ok := opts["timeInForce"].(string); ok {
		req.TimeInForce = lo.ToPtr(ctypes.TimeInForce(tif))
	} else {
		req.TimeInForce = lo.ToPtr(ctypes.TimeInForceGTC)
	}

	// 下单
	if f.placeOrderFn == nil {
		return nil, fmt.Errorf("placeOrderFn not configured")
	}

	result, err := f.placeOrderFn(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to place sell order: %w", err)
	}

	return map[string]any{
		"orderId":       result.OrderID,
		"status":        result.Status.String(),
		"executedQty":   result.ExecutedQty,
		"executedPrice": result.ExecutedPrice,
		"error":         result.Error,
	}, nil
}

// CancelOrder 撤单
func (f *TradeFacade) CancelOrder(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) error {
	req := &stypes.CancelOrderCommand{
		Exchange: exchange,
		Symbol:   symbol,
		OrderID:  orderId,
	}

	// 设置 accountID
	if f.accountIDProvider != nil {
		accountID := f.accountIDProvider(exchange, symbol)
		if accountID != nil {
			req.AccountID = *accountID
		}
	}

	if f.cancelOrderFn == nil {
		return fmt.Errorf("cancelOrderFn not configured")
	}

	return f.cancelOrderFn(ctx, req)
}

// GetOrders 获取订单列表
func (f *TradeFacade) GetOrders(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) ([]map[string]any, error) {
	if f.getOrdersFn == nil {
		return nil, fmt.Errorf("getOrdersFn not configured")
	}

	orders, err := f.getOrdersFn(ctx, exchange, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}

	result := make([]map[string]any, 0, len(orders))
	for _, order := range orders {
		result = append(result, map[string]any{
			"orderId":          order.OrderID.String(),
			"clientOrderId":    order.ClientOrderID.String(),
			"symbol":           order.Symbol.String(),
			"side":             order.Side.String(),
			"orderType":        order.OrderType.String(),
			"price":            order.Price.String(),
			"originalQty":      order.OriginalQty.String(),
			"executedQty":      order.ExecutedQty.String(),
			"executedQuoteQty": order.ExecutedQuoteQty.String(),
			"avgPrice":         order.AvgPrice.String(),
			"status":           order.Status.String(),
			"createdTs":        order.CreatedTs.UnixMilli(),
			"updatedTs":        order.UpdatedTs.UnixMilli(),
		})
	}

	return result, nil
}

// GetOrder 获取单个订单
func (f *TradeFacade) GetOrder(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) (map[string]any, error) {
	if f.getOrderFn == nil {
		return nil, fmt.Errorf("getOrderFn not configured")
	}

	order, err := f.getOrderFn(ctx, exchange, symbol, orderId)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	if order == nil {
		return nil, nil
	}

	return map[string]any{
		"orderId":          order.OrderID.String(),
		"clientOrderId":    order.ClientOrderID.String(),
		"symbol":           order.Symbol.String(),
		"side":             order.Side.String(),
		"orderType":        order.OrderType.String(),
		"price":            order.Price.String(),
		"originalQty":      order.OriginalQty.String(),
		"executedQty":      order.ExecutedQty.String(),
		"executedQuoteQty": order.ExecutedQuoteQty.String(),
		"avgPrice":         order.AvgPrice.String(),
		"status":           order.Status.String(),
		"createdTs":        order.CreatedTs.UnixMilli(),
		"updatedTs":        order.UpdatedTs.UnixMilli(),
	}, nil
}

// GetFills 获取成交记录
func (f *TradeFacade) GetFills(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) ([]map[string]any, error) {
	// 回测场景：从 collector 获取
	if f.isBacktest && f.tradeCollector != nil {
		trades := f.tradeCollector.GetTrades()
		result := make([]map[string]any, 0)

		now := time.Now()
		cutoff := now.Add(-period)

		for _, trade := range trades {
			// 过滤 exchange 和 symbol
			if trade.ExSymbol.Exchange != exchange || !trade.ExSymbol.Symbol.Equal(symbol) {
				continue
			}

			// 时间窗口过滤
			if period > 0 && trade.Ts.Before(cutoff) {
				continue
			}

			result = append(result, map[string]any{
				"fillId":          "", // 回测没有 fillId
				"orderId":         trade.OrderID.String(),
				"symbol":          trade.ExSymbol.Symbol.String(),
				"side":            trade.Side.String(),
				"price":           trade.Price.String(),
				"quantity":        trade.Qty.String(),
				"commission":      trade.Fee.String(),
				"commissionAsset": trade.Asset,
				"isMaker":         false, // 回测暂不区分 maker/taker
				"ts":              trade.Ts.UnixMilli(),
			})
		}

		return result, nil
	}

	// 实盘场景：调用 RPC（需要实现）
	// TODO: 调用 market.GetFills(ctx, exchange, symbol, period)
	return nil, fmt.Errorf("GetFills not implemented for live mode")
}

// GetPositions 获取仓位
func (f *TradeFacade) GetPositions(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, side *ctypes.PositionSide) ([]map[string]any, error) {
	if f.getPositionsFn == nil {
		return nil, fmt.Errorf("getPositionsFn not configured")
	}

	positions, err := f.getPositionsFn(ctx, exchange, &symbol, side)
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	result := make([]map[string]any, 0, len(positions))
	for _, pos := range positions {
		// 如果指定了 side，过滤
		if side != nil && pos.Side != *side {
			continue
		}

		result = append(result, map[string]any{
			"symbol":           pos.Symbol.String(),
			"side":             pos.Side.String(),
			"amount":           pos.Amount.String(),
			"entryPrice":       pos.EntryPrice.String(),
			"markPrice":        pos.MarkPrice.String(),
			"liquidationPrice": pos.LiquidationPrice.String(),
			"leverage":         pos.Leverage,
			"unrealizedProfit": pos.UnRealizedProfit.String(),
			"updatedTs":        pos.UpdatedTs.UnixMilli(),
		})
	}

	return result, nil
}

// GetLeverage 获取杠杆
func (f *TradeFacade) GetLeverage(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error) {
	if f.getLeverageFn == nil {
		return 0, fmt.Errorf("getLeverageFn not configured")
	}

	return f.getLeverageFn(ctx, exchange, symbol)
}

// SetLeverage 设置杠杆
func (f *TradeFacade) SetLeverage(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error {
	if f.setLeverageFn == nil {
		return fmt.Errorf("setLeverageFn not configured")
	}

	return f.setLeverageFn(ctx, exchange, symbol, leverage)
}

// GetFundings 获取资金费率（暂时返回空，待实现）
func (f *TradeFacade) GetFundings(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) ([]map[string]any, error) {
	// TODO: 实现资金费率查询
	// 实盘：调用 market.GetFundings(ctx, exchange, symbol, start, end)
	// 回测：暂不支持
	return []map[string]any{}, nil
}

// GetAccount 获取账户信息（暂时返回空，待实现）
func (f *TradeFacade) GetAccount(ctx context.Context, exchange ctypes.Exchange) (map[string]any, error) {
	_ = ctx
	if f.portfolio == nil {
		return nil, fmt.Errorf("portfolio not configured")
	}

	snap := f.portfolio.Snapshot()
	assets := make([]map[string]any, 0, len(snap.Balances))
	for key, b := range snap.Balances {
		total := b.Free.Add(b.Frozen)
		assets = append(assets, map[string]any{
			"code":    key.Asset,
			"balance": total.String(),
			"locked":  b.Frozen.String(),
			"net":     b.Free.String(),
			"ts":      time.Unix(0, b.UpdateAt).UnixMilli(),
		})
	}

	return map[string]any{
		"exchange": exchange.String(),
		"assets":   assets,
		"ts":       time.Now().UnixMilli(),
	}, nil
}

// GetBalance 获取资产信息
func (f *TradeFacade) GetAsset(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, asset string) (*ctypes.AssetBo, error) {
	return f.portfolio.GetAsset(exchange, symbol, asset)
}
