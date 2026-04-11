package order

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/llt-trade/server/pkg/strategy"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/exchange"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/infra/clock"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/marketdata"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/misc"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/proxy"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

// orderEngine 订单引擎
type orderEngine struct {
	clock clock.Clock

	marketProvider marketdata.MarketProvider // 市场数据提供器
	exGateway      strategy.Gateway          // 用于下单/撤单
}

var _ strategy.OrderEngine = (*orderEngine)(nil)

func NewOrderEngine(marketProvider marketdata.MarketProvider) (*orderEngine, error) {
	om := &orderEngine{
		clock:          clock.DefaultRealClock,
		exGateway:      exchange.NewLiveGateway(),
		marketProvider: marketProvider,
	}
	return om, nil
}

// PlaceOrder 下单
func (m *orderEngine) PlaceOrder(ctx context.Context, req *stypes.PlaceOrderCommand, riskChecker strategy.RiskChecker) (*stypes.PlaceOrderResult, error) {
	if req == nil {
		return &stypes.PlaceOrderResult{
			Status: ctypes.OrderStatusRejected,
			Error:  "nil request",
		}, nil
	}
	if req.Exchange == "" {
		return &stypes.PlaceOrderResult{
			Status: ctypes.OrderStatusRejected,
			Error:  "exchange is required",
		}, nil
	}
	if req.AccountID == "" {
		return &stypes.PlaceOrderResult{
			Status: ctypes.OrderStatusRejected,
			Error:  "account id is required",
		}, nil
	}

	price := decimal.Zero
	var quantity *decimal.Decimal
	var quoteQty *decimal.Decimal
	if req.Price != nil {
		p, err := decimal.NewFromString(*req.Price)
		if err != nil {
			return &stypes.PlaceOrderResult{
				Status: ctypes.OrderStatusRejected,
				Error:  fmt.Sprintf("invalid price: %v", err),
			}, nil
		}
		price = p
	}
	if req.Quantity != nil {
		q, err := decimal.NewFromString(*req.Quantity)
		if err != nil {
			return &stypes.PlaceOrderResult{
				Status: ctypes.OrderStatusRejected,
				Error:  fmt.Sprintf("invalid quantity: %v", err),
			}, nil
		}
		quantity = &q
	}
	if req.QuoteQty != nil {
		q, err := decimal.NewFromString(*req.QuoteQty)
		if err != nil {
			return &stypes.PlaceOrderResult{
				Status: ctypes.OrderStatusRejected,
				Error:  fmt.Sprintf("invalid quote quantity: %v", err),
			}, nil
		}
		quoteQty = &q
	}

	// 生成订单ID
	cid := ctypes.NewOrderId()

	intent := &stypes.OrderPlaceIntent{
		BaseSignal: stypes.BaseSignal{
			Exchange:  &req.Exchange,
			Symbol:    &req.Symbol,
			AccountID: &req.AccountID,
			Ts:        m.clock.Now(),
		},
		ClientOrderID: cid,
		BotID:         req.BotID,
		IsBuy:         req.IsBuy,
		Side:          req.Side,
		OrderType:     req.OrderType,
		Price:         &price,
		Quantity:      quantity,
		QuoteQty:      quoteQty,
		TimeInForce:   req.TimeInForce,
		ReduceOnly:    req.ReduceOnly,
		PostOnly:      req.PostOnly,
	}

	// 1. 预处理：价格/数量归一化、交易所规则校验
	intent, err := m.normalizeIntent(ctx, intent)
	if err != nil {
		return &stypes.PlaceOrderResult{
			Status: ctypes.OrderStatusRejected,
			Error:  err.Error(),
		}, nil
	}

	// 2. 业务风控校验
	if riskChecker != nil {
		if err := riskChecker(ctx, *intent); err != nil {
			return &stypes.PlaceOrderResult{
				Status: ctypes.OrderStatusRejected,
				Error:  fmt.Sprintf("risk control failed: %v", err),
			}, nil
		}
	}

	exOrderID, err := m.exGateway.PlaceOrder(ctx, *intent)
	if err != nil {
		return &stypes.PlaceOrderResult{
			Status: ctypes.OrderStatusRejected,
			Error:  fmt.Sprintf("failed to place order: %v", err),
		}, nil
	}

	return &stypes.PlaceOrderResult{
		OrderID:   cid,
		ExOrderID: exOrderID,
		Status:    ctypes.OrderStatusNew,
	}, nil
}

// normalizeIntent 对下单请求进行价格/数量归一化以及交易所规则校验，并返回规范化后的订单意图。
func (m *orderEngine) normalizeIntent(ctx context.Context, intent *stypes.OrderPlaceIntent) (*stypes.OrderPlaceIntent, error) {
	if intent == nil {
		return nil, nil
	}

	if m.marketProvider == nil {
		return nil, fmt.Errorf("market provider not set")
	}

	exchange := *intent.Exchange
	symbol := *intent.Symbol

	// 1. 获取市场信息
	marketInfo, err := proxy.GetMarket(ctx, exchange, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get market data: %w", err)
	}
	if marketInfo == nil {
		return nil, fmt.Errorf("market not found")
	}

	// 2. 基础校验：市场状态 & 订单类型
	if err := misc.ValidateMarketStatus(marketInfo); err != nil {
		return nil, err
	}
	if err := misc.ValidateOrderType(marketInfo, intent.OrderType); err != nil {
		return nil, err
	}

	// 3. 计算价格：限价直接取 req.Price，市价使用 lastPrice 估算
	if intent.OrderType == ctypes.OrderTypeLimit {
		if intent.Price == nil {
			return nil, fmt.Errorf("price is required for limit order")
		}
		if intent.Price == nil || intent.Price.LessThanOrEqual(decimal.Zero) {
			return nil, fmt.Errorf("invalid price")
		}
		// 归一化价格
		px := misc.NormalizeSymbolPrice(*intent.Price, intent.OrderType, marketInfo)
		if px.IsZero() {
			return nil, fmt.Errorf("price adjusted to zero")
		}
		intent.Price = &px
	}

	// 4. 数量解析：Quantity 与 QuoteQty 只能二选一
	if (intent.Quantity == nil || intent.Quantity.LessThanOrEqual(decimal.Zero)) && (intent.QuoteQty == nil || intent.QuoteQty.LessThanOrEqual(decimal.Zero)) {
		return nil, fmt.Errorf("quantity and quoteQty cannot be provided together")
	}

	var qty decimal.Decimal
	if intent.Quantity != nil && intent.Quantity.GreaterThan(decimal.Zero) {
		qty = *intent.Quantity
		if qty.LessThanOrEqual(decimal.Zero) {
			return nil, fmt.Errorf("invalid quantity")
		}
		// 使用 LotSize 归一化数量
		qty = misc.NormalizeBaseAssetQty(qty, intent.OrderType, marketInfo)
		if qty.IsZero() {
			return nil, fmt.Errorf("quantity adjusted to zero")
		}
		intent.Quantity = &qty
	} else if intent.QuoteQty != nil && intent.QuoteQty.GreaterThan(decimal.Zero) {
		quoteQty := *intent.QuoteQty
		if quoteQty.LessThanOrEqual(decimal.Zero) {
			return nil, fmt.Errorf("invalid quote quantity")
		}
		// 按市价将报价资产数量转换为基础资产数量
		px := decimal.Zero
		if intent.OrderType == ctypes.OrderTypeLimit {
			px = *intent.Price
		} else {
			px, err = m.marketProvider.GetLastPrice(ctx, exchange, symbol)
			if err != nil {
				return nil, fmt.Errorf("failed to get last price: %w", err)
			}
			if px.LessThanOrEqual(decimal.Zero) {
				return nil, fmt.Errorf("last price is zero")
			}
		}
		qty := quoteQty.Div(px)
		qty = misc.NormalizeBaseAssetQty(qty, intent.OrderType, marketInfo)
		if qty.IsZero() {
			return nil, fmt.Errorf("quantity adjusted to zero")
		}
		intent.Quantity = &qty
		intent.QuoteQty = nil
	} else {
		return nil, fmt.Errorf("quantity or quoteQty required")
	}

	// 5. 使用市场过滤器做最终校验
	if err := misc.ValidateMarketFilters(marketInfo, intent.OrderType, intent.Price, *intent.Quantity, 0); err != nil {
		return nil, err
	}

	return intent, nil
}

// CancelOrder 撤单
func (m *orderEngine) CancelOrder(ctx context.Context, req *stypes.CancelOrderCommand) error {
	if m.exGateway == nil {
		return fmt.Errorf("matching engine is nil")
	}
	if req == nil {
		return fmt.Errorf("nil request")
	}
	if req.Exchange == "" {
		return fmt.Errorf("exchange is required")
	}
	if req.AccountID == "" {
		return fmt.Errorf("account id is required")
	}
	accountID := req.AccountID
	clientOrderID := ctypes.OrderId(req.OrderID)

	intent := stypes.OrderCancelIntent{
		BaseSignal: stypes.BaseSignal{
			Exchange:  &req.Exchange,
			Symbol:    &req.Symbol,
			AccountID: &accountID,
			Ts:        m.clock.Now(),
		},
		ClientOrderID: clientOrderID,
	}

	return m.exGateway.CancelOrder(ctx, intent)
}

// GetOrder 查询单个订单（支持 OrderID 或 ClientOrderID）
func (m *orderEngine) GetOrder(ctx context.Context, accountID string, symbol ctypes.Symbol, orderID ctypes.OrderId) (*ctypes.Order, error) {
	return proxy.GetOrder(ctx, accountID, symbol, orderID.String())
}

// GetOrders 查询订单列表（只返回未完结订单）
func (m *orderEngine) GetOrders(ctx context.Context, accountID string, symbol ctypes.Symbol) ([]*ctypes.Order, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account id is required")
	}
	if !symbol.IsValid() {
		return nil, fmt.Errorf("symbol is required")
	}
	return proxy.GetOrders(ctx, accountID, &symbol)
}

// GetAllOrders 返回所有未完结订单（只保留 NEW、PENDING、PARTIAL_DONE 状态的订单）
func (m *orderEngine) GetAllOrders(ctx context.Context, accountID string) ([]*ctypes.Order, error) {
	// TODO: implement
	return nil, nil
}
