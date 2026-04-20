package order

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/precision"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	mb "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/marketdata"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/misc"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type Config struct {
	AllowedSymbols []ctypes.ExSymbolKey

	TakerCommissionRate decimal.Decimal
	MakerCommissionRate decimal.Decimal

	FutureTakerCommissionRate decimal.Decimal
	FutureMakerCommissionRate decimal.Decimal

	MarketOrderFreezeFactor decimal.Decimal
}

// orderEngine 订单引擎
type orderEngine struct {
	// accountID 绑定到该引擎实例（per-account 模式下使用）。为空表示未绑定（旧行为/兼容模式）。
	accountID string

	config Config

	clock    clock.Clock
	eventBus mb.Bus

	accountEngine  strategy.AccountEngine    // 用于获取持仓信息计算已实现盈亏 + 资金校验
	marketProvider marketdata.MarketProvider // 用于价格相关校验
	exGateway      strategy.Gateway          // 同步下单/撤单（撮合所）

	mu       sync.RWMutex
	orders   map[ctypes.OrderId]*ctypes.Order // clientOrderID -> order
	reserved map[ctypes.OrderId]reservation   // clientOrderID -> reserved funds/qty (for unlock on cancel)
}

var _ strategy.OrderEngine = (*orderEngine)(nil)

func newOrderEngine(
	accountID string,
	config Config,
	clock clock.Clock,
	eventBus mb.Bus,
	accountEngine strategy.AccountEngine,
	marketProvider marketdata.MarketProvider,
	exchangeGateway strategy.Gateway,
) (*orderEngine, error) {
	if exchangeGateway == nil {
		return nil, fmt.Errorf("exchange gateway is required")
	}

	om := &orderEngine{
		accountID:      accountID,
		config:         config,
		clock:          clock,
		eventBus:       eventBus,
		accountEngine:  accountEngine,
		marketProvider: marketProvider,
		exGateway:      exchangeGateway,
		orders:         make(map[ctypes.OrderId]*ctypes.Order),
		reserved:       make(map[ctypes.OrderId]reservation),
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
	accountID := req.AccountID
	if m.accountID != "" && accountID != m.accountID {
		return &stypes.PlaceOrderResult{
			Status: ctypes.OrderStatusRejected,
			Error:  fmt.Sprintf("account id mismatch: engine=%s req=%s", m.accountID, accountID),
		}, nil
	}

	exSymbol := ctypes.NewExSymbol(req.Exchange, req.Symbol)

	// 1. 基础校验（交易对是否允许）
	if !slices.Contains(m.config.AllowedSymbols, exSymbol.Key()) {
		return &stypes.PlaceOrderResult{
			Status: ctypes.OrderStatusRejected,
			Error:  "trading symbol is not allowed",
		}, nil
	}

	price := decimal.Zero
	quantity := decimal.Zero
	quoteQty := decimal.Zero
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
		quantity = q
	}
	if req.QuoteQty != nil {
		q, err := decimal.NewFromString(*req.QuoteQty)
		if err != nil {
			return &stypes.PlaceOrderResult{
				Status: ctypes.OrderStatusRejected,
				Error:  fmt.Sprintf("invalid quote quantity: %v", err),
			}, nil
		}
		quoteQty = q
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
		Quantity:      &quantity,
		QuoteQty:      &quoteQty,
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

	// log.Info().Interface("preparedOrder", preparedOrder).Interface("reserve", reserve).Msg("prepared order")

	// 5. 预留资金/仓位（账户侧 & 内部记录）
	reserve, err := m.computeReservation(ctx, intent)
	if err != nil {
		return &stypes.PlaceOrderResult{
			Status: ctypes.OrderStatusRejected,
			Error:  fmt.Sprintf("failed to compute reservation: %v", err),
		}, nil
	}
	if m.accountEngine != nil && reserve.Amount.GreaterThan(decimal.Zero) {
		if reserve.IsFuture && reserve.IsOpenPosition {
			// 期货开仓：冻结保证金（计入 Locked；手续费不冻结）
			if err := m.accountEngine.FreezeFunds(ctx, accountID, exSymbol.Symbol, reserve.Asset, reserve.MarginAmount, intent.ToOrder()); err != nil {
				return &stypes.PlaceOrderResult{
					Status: ctypes.OrderStatusRejected,
					Error:  fmt.Sprintf("failed to freeze margin: %v", err),
				}, nil
			}
		} else {
			// 现货或期货平仓：冻结资金（若 Amount 为 0 则不会进入）
			if err := m.accountEngine.FreezeFunds(ctx, accountID, exSymbol.Symbol, reserve.Asset, reserve.Amount, intent.ToOrder()); err != nil {
				return &stypes.PlaceOrderResult{
					Status: ctypes.OrderStatusRejected,
					Error:  fmt.Sprintf("failed to freeze funds: %v", err),
				}, nil
			}
		}
		m.ReserveFundsWithDetails(intent.ClientOrderID, reserve)
	}

	// 6. 先写入内部订单存储，方便风险/查询（使用 ClientOrderID 作为 key）
	m.mu.Lock()
	m.orders[intent.ClientOrderID] = intent.ToOrder()
	m.mu.Unlock()

	// log.Info().Interface("intent", intent).Msg("placing order")

	exchangeOrderID, err := m.exGateway.PlaceOrder(ctx, *intent)
	if err != nil {
		return &stypes.PlaceOrderResult{
			Status: ctypes.OrderStatusRejected,
			Error:  fmt.Sprintf("failed to place order via exchange: %v", err),
		}, nil
	}

	// 更新订单ID
	m.mu.Lock()
	m.orders[intent.ClientOrderID].OrderID = exchangeOrderID
	m.mu.Unlock()

	return &stypes.PlaceOrderResult{
		OrderID:   cid,
		ExOrderID: exchangeOrderID,
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

	// 4. 数量解析：Quantity 与 QuoteQty 只能二选一，如果同时提供以 intent.Quantity 为准
	if intent.Quantity != nil && intent.Quantity.GreaterThan(decimal.Zero) && intent.QuoteQty != nil && intent.QuoteQty.GreaterThan(decimal.Zero) {
		intent.QuoteQty = nil
	}

	var qty decimal.Decimal
	if intent.Quantity != nil && intent.Quantity.GreaterThan(decimal.Zero) {
		qty = *intent.Quantity
	} else if intent.QuoteQty != nil && intent.QuoteQty.GreaterThan(decimal.Zero) {
		quoteQty := *intent.QuoteQty
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
		qty = quoteQty.Div(px)
		intent.QuoteQty = nil
	} else {
		return nil, fmt.Errorf("quantity or quoteQty required")
	}

	if qty.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("invalid quantity")
	}

	// 使用 LotSize 归一化数量
	qty = misc.NormalizeBaseAssetQty(qty, intent.OrderType, marketInfo)
	if qty.IsZero() {
		return nil, fmt.Errorf("quantity adjusted to zero")
	}
	intent.Quantity = &qty

	// 5. 使用市场过滤器做最终校验
	if err := misc.ValidateMarketFilters(marketInfo, intent.OrderType, intent.Price, *intent.Quantity, 0); err != nil {
		return nil, err
	}

	return intent, nil
}

// computeReservation 根据 intent 计算资金/仓位预留，并校验余额是否充足。
// 调用前需保证 intent 已通过 normalizeIntent 归一化（Price/Quantity 已填）。
func (m *orderEngine) computeReservation(
	ctx context.Context,
	intent *stypes.OrderPlaceIntent,
) (reservation, error) {
	if intent == nil || intent.Exchange == nil || intent.Symbol == nil {
		return reservation{}, fmt.Errorf("intent exchange and symbol required")
	}
	accountID := lo.FromPtrOr(intent.GetAccountID(), "")
	if accountID == "" {
		return reservation{}, fmt.Errorf("intent account id required")
	}
	if intent.Quantity == nil || intent.Quantity.LessThanOrEqual(decimal.Zero) {
		return reservation{}, fmt.Errorf("intent quantity required")
	}

	exSymbol := ctypes.NewExSymbol(*intent.Exchange, *intent.Symbol)
	qty := *intent.Quantity

	// 价格：限价用 intent.Price，市价用最新价
	var px decimal.Decimal
	if intent.Price != nil {
		px = *intent.Price
	} else {
		last, err := m.marketProvider.GetLastPrice(ctx, exSymbol.Exchange, exSymbol.Symbol)
		if err != nil || last.LessThanOrEqual(decimal.Zero) {
			return reservation{}, fmt.Errorf("price required for reservation (market price unavailable)")
		}
		px = last
	}

	isOpenPosition := func(side ctypes.PositionSide, isBuy bool) bool {
		return (side == ctypes.PositionSideLong && isBuy) || (side == ctypes.PositionSideShort && !isBuy)
	}

	if exSymbol.Symbol.Type == ctypes.MarketTypeFuture {
		leverage, err := m.accountEngine.GetLeverage(ctx, accountID, exSymbol.Symbol)
		if err != nil {
			return reservation{}, fmt.Errorf("failed to get leverage: %w", err)
		}
		lev := decimal.NewFromInt(int64(leverage))

		notional := px.Mul(qty)
		margin := notional.Div(lev)
		feeBuf := precision.FeeFromNotional(notional, m.config.TakerCommissionRate)
		openPos := isOpenPosition(intent.Side, intent.IsBuy)

		if openPos {
			asset, err := m.accountEngine.GetAsset(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Quote)
			if err != nil || asset.Free().LessThan(margin.Add(feeBuf)) {
				return reservation{}, fmt.Errorf("insufficient collateral balance")
			}
			return reservation{
				Asset:          exSymbol.Symbol.Quote,
				Amount:         margin,
				IsFuture:       true,
				IsOpenPosition: true,
				MarginAmount:   margin,
			}, nil
		}
		asset, err := m.accountEngine.GetAsset(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Quote)
		if err != nil || asset.Free().LessThan(feeBuf) {
			return reservation{}, fmt.Errorf("insufficient collateral balance for fee")
		}
		return reservation{
			Asset:          exSymbol.Symbol.Quote,
			Amount:         decimal.Zero,
			IsFuture:       true,
			IsOpenPosition: false,
			MarginAmount:   decimal.Zero,
		}, nil
	}

	if intent.IsBuy {
		reserveAmt := px.Mul(qty)
		if intent.OrderType == ctypes.OrderTypeMarket {
			reserveAmt = reserveAmt.Mul(m.config.MarketOrderFreezeFactor)
		}
		asset, err := m.accountEngine.GetAsset(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Quote)
		if err != nil || asset.Free().LessThan(reserveAmt) {
			return reservation{}, fmt.Errorf("insufficient quote balance")
		}
		return reservation{
			Asset:          exSymbol.Symbol.Quote,
			Amount:         reserveAmt,
			IsFuture:       false,
			IsOpenPosition: false,
			MarginAmount:   decimal.Zero,
		}, nil
	}

	asset, err := m.accountEngine.GetAsset(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Base)
	if err != nil || asset.Free().LessThan(qty) {
		return reservation{}, fmt.Errorf("insufficient base balance")
	}
	return reservation{
		Asset:          exSymbol.Symbol.Base,
		Amount:         qty,
		IsFuture:       false,
		IsOpenPosition: false,
		MarginAmount:   decimal.Zero,
	}, nil
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
	if m.accountID != "" && accountID != m.accountID {
		return fmt.Errorf("account id mismatch: engine=%s req=%s", m.accountID, accountID)
	}

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
	m.mu.RLock()
	defer m.mu.RUnlock()
	order := m.orders[orderID]
	if order != nil {
		return order.Clone(), nil
	}
	return nil, nil
}

// GetOrders 查询订单列表（只返回未完结订单）
func (m *orderEngine) GetOrders(ctx context.Context, accountID string, symbol ctypes.Symbol) ([]*ctypes.Order, error) {
	if m.accountID != "" && accountID != "" && accountID != m.accountID {
		return nil, fmt.Errorf("account id mismatch: engine=%s req=%s", m.accountID, accountID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ctypes.Order, 0, len(m.orders))
	for _, order := range m.orders {
		if order == nil {
			continue
		}
		if symbol.IsValid() && !order.Symbol.Equal(symbol) {
			continue
		}
		// 只返回未完结订单
		if !isOrderFinal(order.Status) {
			out = append(out, order.Clone())
		}
	}
	return out, nil
}

// ReserveFunds 预留资金/数量（使用 ClientOrderID）
func (m *orderEngine) ReserveFunds(clientOrderID ctypes.OrderId, asset string, amount decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 保持向后兼容，如果没有设置其他字段，使用默认值
	if res, ok := m.reserved[clientOrderID]; ok {
		res.Asset = asset
		res.Amount = amount
		m.reserved[clientOrderID] = res
	} else {
		m.reserved[clientOrderID] = reservation{Asset: asset, Amount: amount}
	}
}

// ReserveFundsWithDetails 预留资金/数量（带详细信息）
func (m *orderEngine) ReserveFundsWithDetails(clientOrderID ctypes.OrderId, res reservation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reserved[clientOrderID] = res
}

// GetReservation 获取预留信息（支持 OrderID 或 ClientOrderID）
func (m *orderEngine) GetReservation(orderID ctypes.OrderId) (reservation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 先尝试作为 ClientOrderID 查找
	res, ok := m.reserved[orderID]
	if ok {
		return res, ok
	}
	return reservation{}, false
}

// ReleaseReservation 释放预留（支持 OrderID 或 ClientOrderID）
func (m *orderEngine) ReleaseReservation(orderID ctypes.OrderId) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 先尝试作为 ClientOrderID 删除
	if _, ok := m.reserved[orderID]; ok {
		delete(m.reserved, orderID)
		return
	}
}

// UpdateReservation 更新预留金额（支持 OrderID 或 ClientOrderID）
func (m *orderEngine) UpdateReservation(orderID ctypes.OrderId, amount decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 先尝试作为 ClientOrderID 查找
	if res, ok := m.reserved[orderID]; ok {
		res.Amount = amount
		m.reserved[orderID] = res
		return
	}
}

// isOrderFinal 判断订单是否已完结（需要从内存中删除）
func isOrderFinal(status ctypes.OrderStatus) bool {
	switch status {
	case ctypes.OrderStatusDone, ctypes.OrderStatusCanceled, ctypes.OrderStatusRejected, ctypes.OrderStatusExpired:
		return true
	default:
		return false
	}
}

// GetAllOrders 返回引擎 map 中的订单克隆。完结订单在生命周期处理中已从 map 删除，
// 故此处等价于当前尚未完结的订单；完整历史订单由 OrderCollector 持有。
func (m *orderEngine) GetAllOrders(ctx context.Context, accountID string) ([]*ctypes.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ctypes.Order, 0, len(m.orders))
	for _, order := range m.orders {
		if order == nil {
			continue
		}
		out = append(out, order.Clone())
	}
	return out, nil
}

// handleOrderEvent 处理订单事件（更新订单状态和预留资金）
// 返回更新后的订单快照（用于发布 OrderSnapshotSignal）
func (m *orderEngine) handleOrderEvent(ctx context.Context, signal stypes.Signal) *ctypes.Order {
	if signal == nil || signal.GetType() != stypes.SignalTypeOrder {
		return nil
	}

	if signal.GetKind() != stypes.SignalKindOrderLifecycle {
		return nil
	}
	orderLifecycle, ok := signal.(*stypes.OrderLifecycleSignal)
	if !ok || orderLifecycle == nil {
		return nil
	}

	switch orderLifecycle.Status {
	case ctypes.OrderStatusNew:
		return m.handleOrderAccepted(ctx, signal)
	case ctypes.OrderStatusRejected:
		return m.handleOrderRejected(ctx, signal)
	case ctypes.OrderStatusCanceled:
		return m.handleOrderCanceled(ctx, signal)
	case ctypes.OrderStatusExpired:
		return m.handleOrderExpired(ctx, signal)
	case ctypes.OrderStatusDone:
		return m.handleOrderDone(ctx, signal)
	}
	return nil
}

func (m *orderEngine) handleOrderAccepted(_ context.Context, signal stypes.Signal) *ctypes.Order {
	if signal == nil || signal.GetType() != stypes.SignalTypeOrder {
		return nil
	}
	orderLifecycle, ok := signal.(*stypes.OrderLifecycleSignal)
	if !ok || orderLifecycle == nil || orderLifecycle.Status != ctypes.OrderStatusNew {
		return nil
	}

	clientOrderID := orderLifecycle.OrderID
	orderID := ctypes.OrderId(orderLifecycle.OrderID)
	ts := signal.GetTimestamp()

	m.mu.Lock()
	defer m.mu.Unlock()

	order, ok := m.orders[clientOrderID]
	if !ok || order == nil {
		// 只关心由本地 PlaceOrder 创建的订单；外部订单忽略
		return nil
	}

	// 更新订单的 OrderID（由 MatchingEngine 生成）
	order.OrderID = orderID

	// 撮合引擎已接受订单：进入待处理状态，仅更新状态和时间戳
	order.Status = ctypes.OrderStatusPending
	order.UpdatedTs = ts

	// 返回订单快照
	return order.Clone()
}

func (m *orderEngine) handleOrderRejected(ctx context.Context, signal stypes.Signal) *ctypes.Order {
	if signal == nil || signal.GetType() != stypes.SignalTypeOrder {
		return nil
	}
	if signal.GetAccountID() == nil {
		return nil
	}
	accountID := *signal.GetAccountID()
	orderLifecycle, ok := signal.(*stypes.OrderLifecycleSignal)
	if !ok || orderLifecycle == nil || orderLifecycle.Status != ctypes.OrderStatusRejected {
		return nil
	}

	clientOrderID := orderLifecycle.OrderID
	ts := signal.GetTimestamp()

	m.mu.Lock()
	defer m.mu.Unlock()

	var snapshot *ctypes.Order
	order, ok := m.orders[clientOrderID]
	if ok && order != nil {
		order.Status = ctypes.OrderStatusRejected
		order.UpdatedTs = ts
		// 在删除前克隆快照
		snapshot = order.Clone()
		// 订单已完结，从内存中删除
		delete(m.orders, clientOrderID)
	}

	// 拒单：释放剩余预留资金/保证金
	if res, ok := m.reserved[clientOrderID]; ok {
		if m.accountEngine != nil && res.Amount.GreaterThan(decimal.Zero) && res.Asset != "" {
			symbol := *signal.GetSymbol()
			if res.IsFuture && res.IsOpenPosition {
				// 期货开仓：解冻保证金
				_ = m.accountEngine.UnfreezeFunds(ctx, accountID, symbol, res.Asset, res.MarginAmount, order)
			} else {
				// 现货或期货平仓：解冻冻结资金
				_ = m.accountEngine.UnfreezeFunds(ctx, accountID, symbol, res.Asset, res.Amount, order)
			}
		}
		delete(m.reserved, clientOrderID)
	}

	return snapshot
}

func (m *orderEngine) handleOrderCanceled(ctx context.Context, signal stypes.Signal) *ctypes.Order {
	if signal == nil || signal.GetType() != stypes.SignalTypeOrder {
		return nil
	}
	if signal.GetAccountID() == nil {
		return nil
	}
	accountID := *signal.GetAccountID()
	orderLifecycle, ok := signal.(*stypes.OrderLifecycleSignal)
	if !ok || orderLifecycle == nil || orderLifecycle.Status != ctypes.OrderStatusCanceled {
		return nil
	}

	ts := signal.GetTimestamp()

	m.mu.Lock()
	defer m.mu.Unlock()

	clientOrderID := orderLifecycle.OrderID
	var snapshot *ctypes.Order
	order, ok := m.orders[clientOrderID]
	if ok && order != nil {
		order.Status = ctypes.OrderStatusCanceled
		order.UpdatedTs = ts
		// 在删除前克隆快照
		snapshot = order.Clone()
		// 订单已完结，从内存中删除
		delete(m.orders, clientOrderID)
	}

	// 撤单：释放剩余预留资金/保证金
	if res, ok := m.reserved[clientOrderID]; ok {
		if m.accountEngine != nil && res.Amount.GreaterThan(decimal.Zero) && res.Asset != "" {
			symbol := *signal.GetSymbol()
			if res.IsFuture && res.IsOpenPosition {
				// 期货开仓：解冻保证金
				_ = m.accountEngine.UnfreezeFunds(ctx, accountID, symbol, res.Asset, res.MarginAmount, order)
			} else {
				// 现货或期货平仓：解冻冻结资金
				_ = m.accountEngine.UnfreezeFunds(ctx, accountID, symbol, res.Asset, res.Amount, order)
			}
		}
		delete(m.reserved, clientOrderID)
	}

	return snapshot
}

func (m *orderEngine) handleOrderExpired(ctx context.Context, signal stypes.Signal) *ctypes.Order {
	if signal == nil || signal.GetType() != stypes.SignalTypeOrder {
		return nil
	}
	if signal.GetAccountID() == nil {
		return nil
	}
	accountID := *signal.GetAccountID()
	orderLifecycle, ok := signal.(*stypes.OrderLifecycleSignal)
	if !ok || orderLifecycle == nil || orderLifecycle.Status != ctypes.OrderStatusExpired {
		return nil
	}

	ts := signal.GetTimestamp()

	m.mu.Lock()
	defer m.mu.Unlock()

	clientOrderID := orderLifecycle.OrderID
	var snapshot *ctypes.Order
	order, ok := m.orders[clientOrderID]
	if ok && order != nil {
		order.Status = ctypes.OrderStatusExpired
		order.UpdatedTs = ts
		// 在删除前克隆快照
		snapshot = order.Clone()
		// 订单已完结，从内存中删除
		delete(m.orders, clientOrderID)
	}

	// 过期：释放剩余预留资金/保证金
	if res, ok := m.reserved[clientOrderID]; ok {
		if m.accountEngine != nil && res.Amount.GreaterThan(decimal.Zero) && res.Asset != "" {
			symbol := *signal.GetSymbol()
			if res.IsFuture && res.IsOpenPosition {
				// 期货开仓：解冻保证金
				_ = m.accountEngine.UnfreezeFunds(ctx, accountID, symbol, res.Asset, res.MarginAmount, order)
			} else {
				// 现货或期货平仓：解冻冻结资金
				_ = m.accountEngine.UnfreezeFunds(ctx, accountID, symbol, res.Asset, res.Amount, order)
			}
		}
		delete(m.reserved, clientOrderID)
	}

	return snapshot
}

func (m *orderEngine) handleOrderDone(ctx context.Context, signal stypes.Signal) *ctypes.Order {
	if signal == nil || signal.GetType() != stypes.SignalTypeOrder {
		return nil
	}
	if signal.GetAccountID() == nil {
		return nil
	}
	accountID := *signal.GetAccountID()
	orderLifecycle, ok := signal.(*stypes.OrderLifecycleSignal)
	if !ok || orderLifecycle == nil || orderLifecycle.Status != ctypes.OrderStatusDone {
		return nil
	}

	ts := signal.GetTimestamp()

	m.mu.Lock()
	defer m.mu.Unlock()

	clientOrderID := orderLifecycle.OrderID
	var snapshot *ctypes.Order
	order, ok := m.orders[clientOrderID]
	if ok && order != nil {
		order.Status = ctypes.OrderStatusDone
		order.UpdatedTs = ts
		// 在删除前克隆快照
		snapshot = order.Clone()
		// 订单已完结，从内存中删除
		delete(m.orders, clientOrderID)
	}

	// 订单完成：计算并释放剩余冻结资金/保证金
	if res, ok := m.reserved[clientOrderID]; ok {
		if m.accountEngine != nil && res.Amount.GreaterThan(decimal.Zero) && res.Asset != "" {
			symbol := *signal.GetSymbol()

			// 计算剩余金额
			// reserved 中的金额已经在 handleFillEvent 中按成交比例减少
			// 如果还有剩余，说明有未消耗的资金需要释放
			remainingAmount := res.Amount

			if res.IsFuture && res.IsOpenPosition {
				// 期货开仓：计算剩余保证金
				if order != nil && remainingAmount.GreaterThan(decimal.Zero) {
					originalQty := order.OriginalQty
					executedQty := order.ExecutedQty
					if originalQty.GreaterThan(decimal.Zero) && executedQty.LessThan(originalQty) {
						remainingRatio := originalQty.Sub(executedQty).Div(originalQty)
						remainingMargin := res.MarginAmount.Mul(remainingRatio)
						if remainingMargin.GreaterThan(decimal.Zero) {
							_ = m.accountEngine.UnfreezeFunds(ctx, accountID, symbol, res.Asset, remainingMargin, order)
						}
					}
				}
			} else {
				// 现货或期货平仓：解冻剩余冻结资金
				if remainingAmount.GreaterThan(decimal.Zero) {
					_ = m.accountEngine.UnfreezeFunds(ctx, accountID, symbol, res.Asset, remainingAmount, order)
				}
			}
		}
		delete(m.reserved, clientOrderID)
	}

	return snapshot
}

// handleFillEvent 处理成交事件（更新订单成交状态和预留资金）
// 返回更新后的订单快照（用于发布 OrderSnapshotSignal）
func (m *orderEngine) handleFillEvent(_ context.Context, event *stypes.FillSignal) *ctypes.Order {
	if event == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	clientOrderID := event.OrderID
	if clientOrderID == "" {
		return nil
	}

	// 获取订单
	order, ok := m.orders[clientOrderID]
	if !ok || order == nil {
		return nil
	}

	// 更新订单成交数量（从事件中获取）
	// 注意：FillEvent 中的 Qty 是本次成交数量，需要累加
	order.ExecutedQty = order.ExecutedQty.Add(event.Qty)
	order.ExecutedQuoteQty = order.ExecutedQuoteQty.Add(event.Qty.Mul(event.Price))
	if order.ExecutedQty.GreaterThan(decimal.Zero) {
		order.AvgPrice = order.ExecutedQuoteQty.Div(order.ExecutedQty)
	}
	order.UpdatedTs = event.Ts

	// 更新订单状态（部分成交）
	// 注意：订单完成由 OrderDoneSignal 处理，这里只处理部分成交的情况
	if order.ExecutedQty.GreaterThan(decimal.Zero) && order.ExecutedQty.LessThan(order.OriginalQty) {
		order.Status = ctypes.OrderStatusPartialDone
	}

	// 更新预留资金（减少已成交部分对应的预留）
	if res, ok := m.reserved[clientOrderID]; ok && res.Amount.GreaterThan(decimal.Zero) {
		switch {
		case res.IsFuture && res.IsOpenPosition:
			// 期货开仓：预留仅包含保证金，按成交进度在订单完成时释放，无需在 fill 时递减
			m.reserved[clientOrderID] = res
		case !res.IsFuture && order.IsBuy:
			// 现货买入：冻结 quote，按成交支付额递减
			consumed := event.Price.Mul(event.Qty)
			res.Amount = res.Amount.Sub(consumed)
			if res.Amount.LessThan(decimal.Zero) {
				res.Amount = decimal.Zero
			}
			if res.Amount.LessThan(precision.ReservationReleaseDust) {
				delete(m.reserved, clientOrderID)
			} else {
				m.reserved[clientOrderID] = res
			}
		default:
			// 现货卖出（冻结 base）：按成交数量递减
			consumed := event.Qty
			res.Amount = res.Amount.Sub(consumed)
			if res.Amount.LessThan(decimal.Zero) {
				res.Amount = decimal.Zero
			}
			if res.Amount.LessThan(precision.ReservationReleaseDust) {
				delete(m.reserved, clientOrderID)
			} else {
				m.reserved[clientOrderID] = res
			}
		}
	}

	// 返回订单快照
	return order.Clone()
}
