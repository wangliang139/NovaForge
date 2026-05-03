package collectors

import (
	"math"
	"sync"

	"github.com/shopspring/decimal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// TradeCollector 成交记录收集器
// 从 Account 收集成交记录（通过订阅 FillEvent）
type TradeCollector struct {
	mu sync.RWMutex

	trades []*stypes.Trade

	winTrades  int
	lossTrades int
}

// NewTradeCollector 创建成交记录收集器
func NewTradeCollector() *TradeCollector {
	return &TradeCollector{
		trades: make([]*stypes.Trade, 0, 1024),
	}
}

// OnFill 处理成交事件
// 使用 FillSignal 中的 RealizedPnl/FeeInBase 字段（已由 gateway 计算并换算到 BaseCurrency）
func (c *TradeCollector) OnFill(fill *stypes.FillSignal, order *ctypes.Order) {
	if fill == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	clientOrderID := fill.OrderID
	if order != nil && order.ClientOrderID != "" {
		clientOrderID = order.ClientOrderID
	}

	realizedPnl := fill.RealizedPnl

	c.trades = append(c.trades, &stypes.Trade{
		ExSymbol:      ctypes.NewExSymbol(*fill.GetExchange(), *fill.GetSymbol()),
		OrderID:       fill.OrderID,
		ClientOrderID: clientOrderID,
		Side:          fill.Side,
		IsBuy:         fill.IsBuy,
		Qty:           fill.Qty,
		Price:         fill.Price,
		Fee:           fill.Fee,
		Asset:         fill.Asset,
		FeeInBase:     fill.FeeInBase,
		RealizedPnl:   realizedPnl,
		Ts:            fill.Ts,
	})

	// 修复 bug：原条件写反了（IsZero() 时才统计），应该是非零时统计
	if !realizedPnl.IsZero() {
		if realizedPnl.GreaterThan(decimal.Zero) {
			c.winTrades++
		} else if realizedPnl.LessThan(decimal.Zero) {
			c.lossTrades++
		}
	}
}

// GetTrades 获取所有成交记录
func (c *TradeCollector) GetTrades() []*stypes.Trade {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]*stypes.Trade, len(c.trades))
	copy(out, c.trades)
	return out
}

// GetStats 获取统计信息
func (c *TradeCollector) GetStats() (winTrades, lossTrades int, winRate float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := winTrades + lossTrades
	if total > 0 {
		winRate = float64(c.winTrades) / float64(total)
		// 处理 NaN 和 Inf
		if math.IsNaN(winRate) || math.IsInf(winRate, 0) {
			winRate = 0.0
		}
	}

	return c.winTrades, c.lossTrades, winRate
}
