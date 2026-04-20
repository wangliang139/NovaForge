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

func testSpotInstrument(sym Symbol) *Instrument {
	return &Instrument{
		Symbol:      sym,
		Kind:        KindSpot,
		Exchange:    ctypes.ExchangeBinance,
		Market:      ctypes.MarketTypeSpot,
		Base:        "BTC",
		Quote:       "USDT",
		PriceTick:   dec("0.1"),
		QtyStep:     dec("0.001"),
		MinQty:      dec("0.001"),
		MinNotional: dec("5"),
		TakerFeeBps: 10,
		MakerFeeBps: 10,
	}
}

func seedDepth(t *testing.T, eng *Engine, ct ctypes.Symbol) {
	t.Helper()
	_, err := eng.ApplyDepthBook(&ctypes.OrderBook{
		Symbol: ct,
		SeqId:  1,
		Bids:   []ctypes.OrderBookLevel{{Price: dec("49900"), Size: dec("1")}},
		Asks:   []ctypes.OrderBookLevel{{Price: dec("50100"), Size: dec("1")}},
		Ts:     time.Now().UTC(),
	}, false)
	require.NoError(t, err)
}

// placeOrderForTest runs the same path as PlaceOrder but returns the result for assertions.
func placeOrderForTest(t *testing.T, eng *Engine, req PlaceOrderRequest) *PlaceOrderResult {
	t.Helper()
	eng.mu.Lock()
	defer eng.mu.Unlock()
	return eng.placeOrderMuLocked(req, eng.now())
}

func TestHedgeLongAndShortSimultaneously(t *testing.T) {
	eng := NewEngine()
	ct, sym := btcFutureSym()
	require.NoError(t, eng.RegisterInstrument(testPerpInstrument(sym)))
	seedDepth(t, eng, ct)

	acc := "trader1"
	eng.SetAccountPositionMode(acc, PositionModeHedge)
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": dec("100000")},
	})

	// Open long 0.1 @ market (buy long)
	res := placeOrderForTest(t, eng, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideBuy,
		PosSide: ctypes.PositionSideLong, ReduceOnly: false, Leverage: 10,
		Qty: dec("0.1"),
	})
	require.NotNil(t, res)
	require.Equal(t, OrderStatusFilled, res.Order.Status)

	slot, _ := eng.Ledger().GetPerpSlot(acc, sym)
	require.True(t, slot.Long.Qty.GreaterThan(decimal.Zero))
	require.True(t, slot.Short.Qty.IsZero())

	// Open short 0.05 (sell short)
	res2 := placeOrderForTest(t, eng, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideSell,
		PosSide: ctypes.PositionSideShort, ReduceOnly: false, Leverage: 10,
		Qty: dec("0.05"),
	})
	require.NotNil(t, res2)
	require.Equal(t, OrderStatusFilled, res2.Order.Status)

	slot, _ = eng.Ledger().GetPerpSlot(acc, sym)
	require.True(t, slot.Long.Qty.Equal(dec("0.1")))
	require.True(t, slot.Short.Qty.Equal(dec("0.05")))

	// Close long 0.1 (sell long, reduce-only style direction)
	res3 := placeOrderForTest(t, eng, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideSell,
		PosSide: ctypes.PositionSideLong, ReduceOnly: true, Leverage: 10,
		Qty: dec("0.1"),
	})
	require.NotNil(t, res3)
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
	seedDepth(t, eng, ct)

	acc := "u1"
	eng.SetAccountPositionMode(acc, PositionModeOneWay)
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": dec("100000")},
	})

	eng.PlaceOrder(ctx, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideBuy,
		Intent: IntentOpen, ReduceOnly: false, Leverage: 5, Qty: dec("0.02"),
	})
	pos, ok := eng.NetPosition(acc, sym)
	require.True(t, ok)
	require.True(t, pos.Qty.GreaterThan(decimal.Zero))
}

func TestSpotBuyFeeDeductedInBase(t *testing.T) {
	eng := NewEngine()
	ct := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	sym := Symbol(ct.String())
	require.NoError(t, eng.RegisterInstrument(testSpotInstrument(sym)))
	seedDepth(t, eng, ct)

	acc := "spot-buy-fee"
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeSpot: {"USDT": dec("100000"), "BTC": dec("0")},
	})

	res := placeOrderForTest(t, eng, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideBuy, Qty: dec("0.1"),
	})
	require.Equal(t, OrderStatusFilled, res.Order.Status)

	notional := dec("50100").Mul(dec("0.1"))
	feeQ := FeeNotional(notional, 10)
	wantFeeBase := SpotFeeBaseFromQuote(notional, dec("0.1"), feeQ)

	bals := eng.Ledger().Balances(acc)
	usdt := bals[BalanceKey{Wallet: ctypes.WalletTypeSpot, Asset: "USDT"}]
	require.True(t, dec("100000").Sub(usdt).Equal(notional), "USDT 仅扣成交价不含 quote 手续费")

	btc := bals[BalanceKey{Wallet: ctypes.WalletTypeSpot, Asset: "BTC"}]
	require.True(t, dec("0.1").Sub(btc).Equal(wantFeeBase), "BTC 到账 = 成交量 - base 手续费")

	stored, ok := eng.GetOrder(acc, sym, res.Order.ID)
	require.True(t, ok)
	require.Equal(t, "BTC", stored.FeeAsset)
	require.True(t, stored.FeePaid.Equal(wantFeeBase.Neg()))
}

func TestToTypesOrderSpotOmitsRealizedPnl(t *testing.T) {
	ct := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	od := &Order{Symbol: Symbol(ct.String()), RealizedPnl: dec("999")} // 即便内部误写，对外也不应带出（现货）
	out := toTypesOrder(ctypes.ExchangeBinance, od)
	require.Nil(t, out.RealizedPnl)
	require.Nil(t, out.PnlAsset)
}

func TestFeeNotionalFormula(t *testing.T) {
	n := decimal.NewFromInt(12345)
	want := n.Mul(decimal.NewFromInt(7)).Div(decimal.NewFromInt(10000))
	require.True(t, FeeNotional(n, 7).Equal(want), "got %s want %s", FeeNotional(n, 7), want)
}

// 合约开仓：USDT 钱包只扣手续费；初始保证金记在持仓 UsedMargin，不从钱包余额重复扣除。
func TestOneWayCloseOrderRealizedPnl(t *testing.T) {
	eng := NewEngine()
	ct := ctypes.NewSymbol("ETH", "USDT", ctypes.MarketTypeFuture)
	sym := Symbol(ct.String())
	require.NoError(t, eng.RegisterInstrument(testPerpInstrument(sym)))
	seedDepth(t, eng, ct)

	acc := "pnl1"
	eng.SetAccountPositionMode(acc, PositionModeOneWay)
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": dec("100000")},
	})

	open := placeOrderForTest(t, eng, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideBuy,
		Intent: IntentOpen, Leverage: 10, Qty: dec("0.1"),
	})
	require.Equal(t, OrderStatusFilled, open.Order.Status)

	closeRes := placeOrderForTest(t, eng, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideSell,
		Intent: IntentClose, ReduceOnly: true, Leverage: 10, Qty: dec("0.1"),
	})
	require.Equal(t, OrderStatusFilled, closeRes.Order.Status)

	storedClose, ok := eng.GetOrder(acc, sym, closeRes.Order.ID)
	require.True(t, ok)
	wantPnl := dec("49900").Sub(dec("50100")).Mul(dec("0.1"))
	require.True(t, storedClose.RealizedPnl.Equal(wantPnl), "got %s want %s", storedClose.RealizedPnl, wantPnl)

	co := toTypesOrder(ctypes.ExchangeBinance, &storedClose)
	require.NotNil(t, co.RealizedPnl)
	require.True(t, co.RealizedPnl.Equal(wantPnl))
	require.NotNil(t, co.PnlAsset)
	require.Equal(t, "USDT", *co.PnlAsset)
}

func TestOrderFillExposesFeeOnGetOrderAndTypes(t *testing.T) {
	eng := NewEngine()
	ct, sym := btcFutureSym()
	require.NoError(t, eng.RegisterInstrument(testPerpInstrument(sym)))
	seedDepth(t, eng, ct)

	acc := "fee-wire"
	eng.SetAccountPositionMode(acc, PositionModeHedge)
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": dec("100000")},
	})

	res := placeOrderForTest(t, eng, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideBuy,
		PosSide: ctypes.PositionSideLong, ReduceOnly: false, Leverage: 10,
		Qty: dec("0.1"),
	})
	require.NotNil(t, res)
	require.Equal(t, OrderStatusFilled, res.Order.Status)

	stored, ok := eng.GetOrder(acc, sym, res.Order.ID)
	require.True(t, ok)
	notional := dec("50100").Mul(dec("0.1"))
	wantFee := FeeNotional(notional, 10).Neg()
	require.True(t, stored.FeePaid.Equal(wantFee))
	require.Equal(t, "USDT", stored.FeeAsset)

	out := toTypesOrder(ctypes.ExchangeBinance, &stored)
	require.NotNil(t, out.Fee)
	require.True(t, out.Fee.Equal(wantFee))
	require.NotNil(t, out.FeeAsset)
	require.Equal(t, "USDT", *out.FeeAsset)
}

func TestPerpOpenDeductsQuoteFeeOnly(t *testing.T) {
	eng := NewEngine()
	ct, sym := btcFutureSym()
	require.NoError(t, eng.RegisterInstrument(testPerpInstrument(sym)))
	seedDepth(t, eng, ct)

	acc := "fee-check"
	eng.SetAccountPositionMode(acc, PositionModeHedge)
	start := dec("100000")
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": start},
	})

	res := placeOrderForTest(t, eng, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeMarket, Side: SideBuy,
		PosSide: ctypes.PositionSideLong, ReduceOnly: false, Leverage: 10,
		Qty: dec("0.1"),
	})
	require.NotNil(t, res)
	require.Equal(t, OrderStatusFilled, res.Order.Status)

	// 深度 ask 50100 → 名义 5010 U，手续费 10 bps = 5.01；保证金记在持仓侧。
	notional := dec("50100").Mul(dec("0.1"))
	wantFee := FeeNotional(notional, 10)
	wantIM := notional.Div(decimal.NewFromInt(10))

	slot, ok := eng.Ledger().GetPerpSlot(acc, sym)
	require.True(t, ok)
	require.True(t, slot.Long.UsedMargin.Equal(wantIM))

	bals := eng.Ledger().Balances(acc)
	key := BalanceKey{Wallet: ctypes.WalletTypeFuture, Asset: "USDT"}
	end := bals[key]
	require.True(t, start.Sub(wantFee).Equal(end), "wallet want %s got %s (fee %s)", start.Sub(wantFee), end, wantFee)
}

func TestApplyDepthBookMakerFill(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()
	ct, sym := btcFutureSym()
	require.NoError(t, eng.RegisterInstrument(testPerpInstrument(sym)))

	_, err := eng.ApplyDepthBook(&ctypes.OrderBook{
		Symbol: ct,
		SeqId:  1,
		Asks:   []ctypes.OrderBookLevel{{Price: dec("50000"), Size: dec("10")}},
		Bids:   []ctypes.OrderBookLevel{{Price: dec("49000"), Size: dec("10")}},
		Ts:     time.Now().UTC(),
	}, false)
	require.NoError(t, err)

	acc := "m1"
	eng.SetAccountPositionMode(acc, PositionModeOneWay)
	eng.InitBalances(acc, map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": dec("100000")},
	})

	// Resting buy below best ask — no immediate fill
	eng.PlaceOrder(ctx, PlaceOrderRequest{
		AccountID: acc, Symbol: sym, OrderType: OrderTypeLimit, Side: SideBuy,
		Intent: IntentOpen, ReduceOnly: false, Leverage: 10,
		Price: dec("49950"), Qty: dec("0.1"),
	})

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
