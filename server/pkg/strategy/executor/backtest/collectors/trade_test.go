package collectors

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestTradeCollectorRecordsFeeWithoutOrderSnapshot(t *testing.T) {
	exchange := ctypes.ExchangeBinance
	symbol := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	accountID := "binance"
	orderID := ctypes.OrderId("order-1")

	collector := NewTradeCollector()
	collector.OnFill(&stypes.FillSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange:  &exchange,
			Symbol:    &symbol,
			AccountID: &accountID,
			Ts:        time.Unix(100, 0),
		},
		OrderID:   orderID,
		Side:      ctypes.PositionSideLong,
		IsBuy:     true,
		Qty:       decimal.RequireFromString("0.1"),
		Price:     decimal.RequireFromString("10000"),
		Fee:       decimal.RequireFromString("0.0001"),
		Asset:     "BTC",
		FeeInBase: decimal.RequireFromString("1"),
	}, nil)

	trades := collector.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if !trades[0].Fee.Equal(decimal.RequireFromString("0.0001")) {
		t.Fatalf("expected fee 0.0001, got %s", trades[0].Fee)
	}
	if trades[0].Asset != "BTC" {
		t.Fatalf("expected fee asset BTC, got %s", trades[0].Asset)
	}
	if !trades[0].FeeInBase.Equal(decimal.RequireFromString("1")) {
		t.Fatalf("expected fee in base 1, got %s", trades[0].FeeInBase)
	}
	if trades[0].ClientOrderID != orderID {
		t.Fatalf("expected client order id %s, got %s", orderID, trades[0].ClientOrderID)
	}
}
