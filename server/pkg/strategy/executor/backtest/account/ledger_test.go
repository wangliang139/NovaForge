package account

import (
	"testing"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

func TestSpotCostBasis_BuyAndSell(t *testing.T) {
	ledger := NewAssetLedger("USDT")
	exSymbol := ctypes.NewExSymbol(ctypes.ExchangeBinance, ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot))

	// 买入 1 BTC @ 10000 USDT
	realizedPnl := ledger.UpdateSpotCostBasis(exSymbol, true, decimal.NewFromInt(1), decimal.NewFromInt(10000))
	if !realizedPnl.IsZero() {
		t.Errorf("买入不应产生已实现盈亏，got %s", realizedPnl)
	}

	cb := ledger.GetSpotCostBasis(exSymbol)
	if !cb.Qty.Equal(decimal.NewFromInt(1)) {
		t.Errorf("持仓量应为 1，got %s", cb.Qty)
	}
	if !cb.AvgCostQuote.Equal(decimal.NewFromInt(10000)) {
		t.Errorf("平均成本应为 10000，got %s", cb.AvgCostQuote)
	}

	// 再买入 1 BTC @ 12000 USDT
	realizedPnl = ledger.UpdateSpotCostBasis(exSymbol, true, decimal.NewFromInt(1), decimal.NewFromInt(12000))
	if !realizedPnl.IsZero() {
		t.Errorf("买入不应产生已实现盈亏，got %s", realizedPnl)
	}

	cb = ledger.GetSpotCostBasis(exSymbol)
	if !cb.Qty.Equal(decimal.NewFromInt(2)) {
		t.Errorf("持仓量应为 2，got %s", cb.Qty)
	}
	expectedAvg := decimal.NewFromInt(11000) // (10000 + 12000) / 2
	if !cb.AvgCostQuote.Equal(expectedAvg) {
		t.Errorf("平均成本应为 11000，got %s", cb.AvgCostQuote)
	}

	// 卖出 1 BTC @ 13000 USDT
	realizedPnl = ledger.UpdateSpotCostBasis(exSymbol, false, decimal.NewFromInt(1), decimal.NewFromInt(13000))
	expectedPnl := decimal.NewFromInt(2000) // 1 * (13000 - 11000)
	if !realizedPnl.Equal(expectedPnl) {
		t.Errorf("已实现盈亏应为 2000，got %s", realizedPnl)
	}

	cb = ledger.GetSpotCostBasis(exSymbol)
	if !cb.Qty.Equal(decimal.NewFromInt(1)) {
		t.Errorf("持仓量应为 1，got %s", cb.Qty)
	}
	if !cb.AvgCostQuote.Equal(decimal.NewFromInt(11000)) {
		t.Errorf("平均成本应保持 11000，got %s", cb.AvgCostQuote)
	}

	// 卖出剩余 1 BTC @ 12000 USDT
	realizedPnl = ledger.UpdateSpotCostBasis(exSymbol, false, decimal.NewFromInt(1), decimal.NewFromInt(12000))
	expectedPnl = decimal.NewFromInt(1000) // 1 * (12000 - 11000)
	if !realizedPnl.Equal(expectedPnl) {
		t.Errorf("已实现盈亏应为 1000，got %s", realizedPnl)
	}

	cb = ledger.GetSpotCostBasis(exSymbol)
	if !cb.Qty.IsZero() {
		t.Errorf("持仓量应为 0，got %s", cb.Qty)
	}
	if !cb.AvgCostQuote.IsZero() {
		t.Errorf("平均成本应为 0，got %s", cb.AvgCostQuote)
	}
}

func TestSpotCostBasis_FeeSeparate(t *testing.T) {
	// 验证手续费不影响成本价
	ledger := NewAssetLedger("USDT")
	exSymbol := ctypes.NewExSymbol(ctypes.ExchangeBinance, ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot))

	// 买入 1 BTC @ 10000 USDT（手续费不在这里处理，由 gateway 单独扣除）
	ledger.UpdateSpotCostBasis(exSymbol, true, decimal.NewFromInt(1), decimal.NewFromInt(10000))

	cb := ledger.GetSpotCostBasis(exSymbol)
	// 成本价应该就是成交价，不包含手续费
	if !cb.AvgCostQuote.Equal(decimal.NewFromInt(10000)) {
		t.Errorf("成本价应为 10000（不含手续费），got %s", cb.AvgCostQuote)
	}
}
