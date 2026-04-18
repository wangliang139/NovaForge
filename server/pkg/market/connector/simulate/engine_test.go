package simulate

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func dec(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}

func btcFutureSym() (ctypes.Symbol, Symbol) {
	ct := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)
	return ct, Symbol(ct.String())
}

func testPerpInstrument(sym Symbol) *Instrument {
	return &Instrument{
		Symbol:      sym,
		Kind:        KindPerp,
		Exchange:    ctypes.ExchangeBinance,
		Market:      ctypes.MarketTypeFuture,
		Base:        "BTC",
		Quote:       "USDT",
		PriceTick:   dec("0.1"),
		QtyStep:     dec("0.001"),
		MinQty:      dec("0.001"),
		MinNotional: dec("5"),
		TakerFeeBps: 10,
		MakerFeeBps: 10,
		LeverageMax: 125,
	}
}

func seedDepth(t *testing.T, eng *Engine, sym Symbol) {
	t.Helper()
	d := NewMarketDepth()
	require.NoError(t, d.ApplySnapshot(&OrderBook{
		Symbol: sym,
		SeqId:  1,
		Bids:   []OrderBookLevel{{Price: dec("49900"), Size: dec("1")}},
		Asks:   []OrderBookLevel{{Price: dec("50100"), Size: dec("1")}},
	}))
	require.NoError(t, eng.BindDepth(sym, d))
}

func TestHedgeLongAndShortSimultaneously(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()
	_, sym := btcFutureSym()
	require.NoError(t, eng.RegisterInstrument(testPerpInstrument(sym)))
	seedDepth(t, eng, sym)

	acc := "trader1"
	eng.SetAccountPositionMode(acc, PositionModeHedge)
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": dec("100000")},
	})

	// Open long 0.1 @ market (buy long)
	res, err := eng.PlaceOrder(ctx, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideBuy,
		PosSide: ctypes.PositionSideLong, ReduceOnly: false, Leverage: 10,
		Qty: dec("0.1"),
	})
	require.NoError(t, err)
	require.Equal(t, OrderStatusFilled, res.Order.Status)

	slot, _ := eng.Ledger().GetPerpSlot(acc, sym)
	require.True(t, slot.Long.Qty.GreaterThan(decimal.Zero))
	require.True(t, slot.Short.Qty.IsZero())

	// Open short 0.05 (sell short)
	res2, err := eng.PlaceOrder(ctx, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideSell,
		PosSide: ctypes.PositionSideShort, ReduceOnly: false, Leverage: 10,
		Qty: dec("0.05"),
	})
	require.NoError(t, err)
	require.Equal(t, OrderStatusFilled, res2.Order.Status)

	slot, _ = eng.Ledger().GetPerpSlot(acc, sym)
	require.True(t, slot.Long.Qty.Equal(dec("0.1")))
	require.True(t, slot.Short.Qty.Equal(dec("0.05")))

	// Close long 0.1 (sell long, reduce-only style direction)
	res3, err := eng.PlaceOrder(ctx, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideSell,
		PosSide: ctypes.PositionSideLong, ReduceOnly: true, Leverage: 10,
		Qty: dec("0.1"),
	})
	require.NoError(t, err)
	require.Equal(t, OrderStatusFilled, res3.Order.Status)

	slot, _ = eng.Ledger().GetPerpSlot(acc, sym)
	require.True(t, slot.Long.Qty.IsZero())
	require.True(t, slot.Short.Qty.Equal(dec("0.05")))
}

func TestOneWayNetPosition(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()
	ct := ctypes.NewSymbol("ETH", "USDT", ctypes.MarketTypeFuture)
	sym := Symbol(ct.String())
	require.NoError(t, eng.RegisterInstrument(testPerpInstrument(sym)))
	seedDepth(t, eng, sym)

	acc := "u1"
	eng.SetAccountPositionMode(acc, PositionModeOneWay)
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": dec("100000")},
	})

	_, err := eng.PlaceOrder(ctx, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideBuy,
		Intent: IntentOpen, ReduceOnly: false, Leverage: 5, Qty: dec("0.02"),
	})
	require.NoError(t, err)
	pos, ok := eng.NetPosition(acc, sym)
	require.True(t, ok)
	require.True(t, pos.Qty.GreaterThan(decimal.Zero))
}

func TestFeeNotionalFormula(t *testing.T) {
	n := decimal.NewFromInt(12345)
	want := n.Mul(decimal.NewFromInt(7)).Div(decimal.NewFromInt(10000))
	require.True(t, FeeNotional(n, 7).Equal(want), "got %s want %s", FeeNotional(n, 7), want)
}

func TestApplyDepthBookMakerFill(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()
	ct, sym := btcFutureSym()
	require.NoError(t, eng.RegisterInstrument(testPerpInstrument(sym)))

	d := NewMarketDepth()
	require.NoError(t, eng.BindDepth(sym, d))
	require.NoError(t, d.ApplySnapshot(&OrderBook{
		Symbol: sym, SeqId: 1,
		Asks: []OrderBookLevel{{Price: dec("50000"), Size: dec("10")}},
		Bids: []OrderBookLevel{{Price: dec("49000"), Size: dec("10")}},
	}))

	acc := "m1"
	eng.SetAccountPositionMode(acc, PositionModeOneWay)
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": dec("100000")},
	})

	// Resting buy below best ask — no immediate fill
	_, err := eng.PlaceOrder(ctx, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeLimit, Side: SideBuy,
		Intent: IntentOpen, ReduceOnly: false, Leverage: 10,
		Price: dec("49950"), Qty: dec("0.1"),
	})
	require.NoError(t, err)

	evs, err := eng.ApplyDepthBook(&ctypes.OrderBook{
		Symbol:    ct,
		SeqId:     2,
		PrevSeqId: 1,
		Asks: []ctypes.OrderBookLevel{
			{Price: dec("49950"), Size: dec("2")},
			{Price: dec("50000"), Size: dec("10")},
		},
		Ts: time.Now().UTC(),
	}, true)
	require.NoError(t, err)
	require.NotEmpty(t, evs)
}
