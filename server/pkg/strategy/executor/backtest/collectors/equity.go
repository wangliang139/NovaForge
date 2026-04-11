package collectors

import (
	"sync"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// EquityCollector 权益曲线收集器
// 从 Portfolio 收集权益点数据
type EquityCollector struct {
	mu sync.RWMutex

	symbols []ctypes.ExSymbol

	equity  []*stypes.EquityPoint
	initial *stypes.EquityPoint
}

// NewEquityCollector 创建权益曲线收集器
func NewEquityCollector() *EquityCollector {
	return &EquityCollector{
		equity: make([]*stypes.EquityPoint, 0, 2048),
	}
}

// OnEquityPoint 记录权益点
func (c *EquityCollector) OnEquityPoint(point stypes.EquityPoint) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initial == nil {
		c.initial = &point
	}

	// 同一时间点多次记录：覆盖最后一个点，避免曲线膨胀
	if n := len(c.equity); n > 0 && c.equity[n-1].Ts.Equal(point.Ts) {
		c.equity[n-1] = &point
		return
	}

	c.equity = append(c.equity, &point)
}

// GetEquity 获取权益曲线
func (c *EquityCollector) GetEquity() []stypes.EquityPoint {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]stypes.EquityPoint, len(c.equity))
	for i, p := range c.equity {
		out[i] = *p
	}
	return out
}

func (c *EquityCollector) GetFinal() *stypes.EquityPoint {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.equity) == 0 {
		return nil
	}

	return c.equity[len(c.equity)-1]
}

func (c *EquityCollector) GetInitial() *stypes.EquityPoint {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.initial
}
