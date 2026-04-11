package order

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	mb "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/marketdata"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// OrderEngineManager 路由层：单点订阅 bus，并按 accountId 分发到 per-account OrderEngine。
// 目的：
// - 避免多实例 OrderEngine 重复订阅导致事件重复处理
// - 让 orders/reserved/leverage 等状态严格按账户隔离
type OrderEngineManager struct {
	cfg Config

	clock           clock.Clock
	eventBus        mb.Bus
	accountEngine   strategy.AccountEngine
	marketProvider  marketdata.MarketProvider
	exchangeGateway strategy.Gateway

	mu      sync.RWMutex
	engines map[string]*orderEngine // accountId -> engine

	// 订单ID映射：用户态 clientOrderId <-> 撮合/交易所侧 exchangeOrderId
	clientToExchange map[ctypes.OrderId]ctypes.OrderId
	exchangeToClient map[ctypes.OrderId]ctypes.OrderId
	orderIDToAccount map[ctypes.OrderId]string // clientOrderID -> accountId
}

var _ strategy.OrderEngine = (*OrderEngineManager)(nil)

func NewOrderEngineManager(
	config Config,
	clk clock.Clock,
	eventBus mb.Bus,
	accountEngine strategy.AccountEngine,
	marketProvider marketdata.MarketProvider,
	exchangeGateway strategy.Gateway,
) (*OrderEngineManager, error) {
	if exchangeGateway == nil {
		return nil, fmt.Errorf("exchange gateway is required")
	}

	m := &OrderEngineManager{
		cfg:              config,
		clock:            clk,
		eventBus:         eventBus,
		accountEngine:    accountEngine,
		marketProvider:   marketProvider,
		exchangeGateway:  exchangeGateway,
		engines:          make(map[string]*orderEngine),
		clientToExchange: make(map[ctypes.OrderId]ctypes.OrderId),
		exchangeToClient: make(map[ctypes.OrderId]ctypes.OrderId),
		orderIDToAccount: make(map[ctypes.OrderId]string),
	}

	// 订单管理器在撮合阶段处理订单意图，在状态更新阶段处理订单和成交事件
	// 单点订阅：Order（包含 intent + order events）
	_, _ = eventBus.Subscribe(m.onOrderSignal, int(mb.StageMatching), mb.NewTypeFilter(stypes.SignalTypeOrder))
	// 单点订阅：Fill
	_, _ = eventBus.Subscribe(m.onFillSignal, int(mb.StageStateUpdate), mb.NewTypeFilter(stypes.SignalTypeFill))

	return m, nil
}

func (m *OrderEngineManager) getOrCreateEngine(accountID string) (*orderEngine, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if accountID == "" {
		return nil, fmt.Errorf("account id is required")
	}
	if eng := m.engines[accountID]; eng != nil {
		return eng, nil
	}

	eng, err := newOrderEngine(accountID, m.cfg, m.clock, m.eventBus, m.accountEngine, m.marketProvider, m.exchangeGateway)
	if err != nil {
		return nil, err
	}
	m.engines[accountID] = eng
	return eng, nil
}

func (m *OrderEngineManager) onOrderSignal(ctx context.Context, sig stypes.Signal) error {
	if sig == nil || sig.GetType() != stypes.SignalTypeOrder {
		return nil
	}

	aid := sig.GetAccountID()
	if aid == nil {
		return fmt.Errorf("account id not found")
	}

	eng, err := m.getOrCreateEngine(*aid)
	if err != nil {
		return err
	}
	if eng == nil {
		return fmt.Errorf("account engine not found")
	}

	// 处理订单事件并获取快照
	snapshot := eng.handleOrderEvent(ctx, sig)

	// 发布订单快照信号
	if snapshot != nil {
		m.publishOrderSnapshot(ctx, sig, snapshot)
	}

	// 订单完结时清理数据
	switch sig.GetKind() {
	case stypes.SignalKindOrderLifecycle:
		orderLifecycle, ok := sig.(*stypes.OrderLifecycleSignal)
		if !ok || orderLifecycle == nil {
			return nil
		}
		if orderLifecycle.Status != ctypes.OrderStatusDone && orderLifecycle.Status != ctypes.OrderStatusCanceled && orderLifecycle.Status != ctypes.OrderStatusExpired && orderLifecycle.Status != ctypes.OrderStatusRejected {
			return nil
		}
		m.mu.Lock()
		exOrderID, ok := m.clientToExchange[orderLifecycle.OrderID]
		if ok {
			delete(m.exchangeToClient, exOrderID)
		}
		delete(m.clientToExchange, orderLifecycle.OrderID)
		delete(m.orderIDToAccount, orderLifecycle.OrderID)
		m.mu.Unlock()
	}
	return nil
}

func (m *OrderEngineManager) onFillSignal(ctx context.Context, sig stypes.Signal) error {
	if sig == nil || sig.GetType() != stypes.SignalTypeFill {
		return nil
	}
	fe, ok := sig.(*stypes.FillSignal)
	if !ok || fe == nil {
		return nil
	}

	aid := fe.GetAccountID()
	if aid == nil {
		return fmt.Errorf("account id not found")
	}

	eng, err := m.getOrCreateEngine(*aid)
	if err != nil {
		return err
	}
	if eng == nil {
		return nil
	}

	// 处理成交事件并获取快照
	snapshot := eng.handleFillEvent(ctx, fe)

	// 发布订单快照信号
	if snapshot != nil {
		m.publishOrderSnapshot(ctx, sig, snapshot)
	}

	return nil
}

func (m *OrderEngineManager) PlaceOrder(ctx context.Context, req *stypes.PlaceOrderCommand, riskChecker strategy.RiskChecker) (*stypes.PlaceOrderResult, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if req.AccountID == "" {
		return nil, fmt.Errorf("account id is required")
	}
	eng, err := m.getOrCreateEngine(req.AccountID)
	if err != nil {
		return nil, err
	}
	if eng == nil {
		return nil, fmt.Errorf("account engine not found")
	}
	result, err := eng.PlaceOrder(ctx, req, riskChecker)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.clientToExchange[result.OrderID] = result.ExOrderID
	m.exchangeToClient[result.ExOrderID] = result.OrderID
	m.orderIDToAccount[result.OrderID] = req.AccountID
	m.mu.Unlock()

	// 获取订单快照并发布
	if snapshot, err := eng.GetOrder(ctx, req.AccountID, req.Symbol, result.OrderID); err == nil && snapshot != nil {
		ex := req.Exchange
		sym := req.Symbol
		aid := req.AccountID
		m.eventBus.Publish(ctx, &stypes.OrderSnapshotSignal{
			BaseSignal: stypes.BaseSignal{
				Exchange:  &ex,
				Symbol:    &sym,
				AccountID: &aid,
				Ts:        eng.clock.Now(),
			},
			OrderID:     result.OrderID,
			Order:       snapshot,
			TriggerKind: stypes.SignalKindOrderLifecycle,
		})
	}

	return result, nil
}

func (m *OrderEngineManager) CancelOrder(ctx context.Context, req *stypes.CancelOrderCommand) error {
	if req == nil {
		return fmt.Errorf("nil request")
	}
	if req.AccountID == "" {
		return fmt.Errorf("account id is required")
	}

	eng, err := m.getOrCreateEngine(req.AccountID)
	if err != nil {
		return err
	}
	if eng == nil {
		return fmt.Errorf("account engine not found")
	}
	return eng.CancelOrder(ctx, req)
}

func (m *OrderEngineManager) GetOrder(ctx context.Context, accountID string, symbol ctypes.Symbol, orderID ctypes.OrderId) (*ctypes.Order, error) {
	if orderID == "" {
		return nil, nil
	}

	m.mu.RLock()
	aid, ok := m.orderIDToAccount[orderID]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("account id not found")
	}
	m.mu.RUnlock()

	eng, err := m.getOrCreateEngine(aid)
	if err != nil {
		return nil, err
	}
	if eng == nil {
		return nil, nil
	}
	return eng.GetOrder(ctx, accountID, symbol, orderID)
}

func (m *OrderEngineManager) GetOrders(ctx context.Context, accountID string, symbol ctypes.Symbol) ([]*ctypes.Order, error) {
	eng, err := m.getOrCreateEngine(accountID)
	if err != nil {
		return nil, err
	}
	if eng == nil {
		return nil, errors.New("account engine not found")
	}
	return eng.GetOrders(ctx, accountID, symbol)
}

func (m *OrderEngineManager) GetAllOrders(ctx context.Context, accountID string) ([]*ctypes.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*ctypes.Order, 0)
	for _, eng := range m.engines {
		orders, err := eng.GetAllOrders(ctx, accountID)
		if err != nil {
			return nil, err
		}
		out = append(out, orders...)
	}
	return out, nil
}

// publishOrderSnapshot 发布订单快照信号
func (m *OrderEngineManager) publishOrderSnapshot(ctx context.Context, triggerSignal stypes.Signal, snapshot *ctypes.Order) {
	if snapshot == nil || triggerSignal == nil {
		return
	}

	// 构建快照信号
	snapshotSignal := &stypes.OrderSnapshotSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange:  triggerSignal.GetExchange(),
			Symbol:    triggerSignal.GetSymbol(),
			AccountID: triggerSignal.GetAccountID(),
			Ts:        triggerSignal.GetTimestamp(),
		},
		OrderID:     snapshot.ClientOrderID,
		Order:       snapshot,
		TriggerKind: triggerSignal.GetKind(),
	}

	// 发布快照信号
	_ = m.eventBus.Publish(ctx, snapshotSignal)
}
