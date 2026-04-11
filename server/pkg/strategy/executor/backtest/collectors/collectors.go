package collectors

import (
	"context"

	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	mb "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging/store"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// Collectors 收集器集合
type Collectors struct {
	Equity *EquityCollector
	Trade  *TradeCollector
	Order  *OrderCollector
	Log    *LogCollector
}

func NewCollectors(consoleLogger *store.BufferStorage, eventBus mb.Bus, orderEngine strategy.OrderEngine) *Collectors {
	c := &Collectors{
		Equity: NewEquityCollector(),
		Trade:  NewTradeCollector(),
		Order:  NewOrderCollector(orderEngine),
		Log:    NewLogCollector(consoleLogger),
	}

	// Collectors 在最后阶段处理事件（仅用于记录，不影响业务逻辑）
	eventBus.Subscribe(func(ctx context.Context, event stypes.Signal) error {
		return c.Order.OnOrderSnapshot(ctx, event)
	}, int(mb.StageCollectors), mb.NewKindFilter(stypes.SignalKindOrderSnapshot))

	// TradeCollector: 订阅 FillEvent
	// FillSignal 已包含 BaseCurrency 计价的 RealizedPnl/FeeInBase（由 gateway 计算）
	eventBus.Subscribe(func(ctx context.Context, event stypes.Signal) error {
		if fillEvent, ok := event.(*stypes.FillSignal); ok {
			// 从 OrderCollector 获取订单信息
			allOrders := c.Order.GetAllOrders()
			for _, o := range allOrders {
				if o != nil && o.OrderID == fillEvent.OrderID {
					c.Trade.OnFill(fillEvent, o)
					break
				}
			}
		}
		return nil
	}, int(mb.StageCollectors), mb.NewTypeFilter(stypes.SignalTypeFill))

	return c
}
