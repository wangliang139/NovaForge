package collectors

import (
	"context"
	"errors"
	"sync"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// OrderCollector 订单收集器
// 从 OrderManager 收集订单记录（通过订阅 OrderEvent）
type OrderCollector struct {
	mu           sync.RWMutex
	orders       map[ctypes.OrderId]*ctypes.Order
	orderEngine strategy.OrderEngine
}

// NewOrderCollector 创建订单收集器
func NewOrderCollector(orderEngine strategy.OrderEngine) *OrderCollector {
	return &OrderCollector{
		orders:       make(map[ctypes.OrderId]*ctypes.Order),
		orderEngine: orderEngine,
	}
}

// OnOrderSnapshot 处理订单快照信号（新方法，替代 OnOrderEvent）
func (c *OrderCollector) OnOrderSnapshot(ctx context.Context, event stypes.Signal) error {
	if event == nil {
		return errors.New("event is nil")
	}

	snapshot, ok := event.(*stypes.OrderSnapshotSignal)
	if !ok || snapshot == nil {
		return nil
	}

	// 快照信号已包含完整订单信息，直接记录
	if snapshot.Order == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 使用 ClientOrderID 作为 key（与原来保持一致）
	orderID := snapshot.OrderID
	if orderID != "" {
		c.orders[orderID] = snapshot.Order.Clone()
		// log.Info().Str("orderID", string(orderID)).
		// 	Str("triggerKind", string(snapshot.TriggerKind)).
		// 	Str("executedQty", snapshot.Order.ExecutedQty.String()).
		// 	Str("avgPrice", snapshot.Order.AvgPrice.String()).
		// 	Str("status", string(snapshot.Order.Status)).
		// 	Msg("order snapshot")
	}

	return nil
}

// GetAllOrders 获取所有订单
func (c *OrderCollector) GetAllOrders() []*ctypes.Order {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]*ctypes.Order, 0, len(c.orders))
	for _, order := range c.orders {
		if order != nil {
			out = append(out, order.Clone())
		}
	}
	return out
}
