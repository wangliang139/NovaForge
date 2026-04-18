package simulate

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func seedUSDT(wallet ctypes.WalletType, amount decimal.Decimal) map[ctypes.WalletType]map[Asset]decimal.Decimal {
	return map[ctypes.WalletType]map[Asset]decimal.Decimal{
		wallet: {Asset("USDT"): amount},
	}
}

// TestOkxUnifiedTradeWalletSpotAndPerpShareUSDT documents types.GetWalletType(okx, ·): spot and futures
// both map to WalletTypeTrade — spot spend and perp margin debit the same USDT bucket.
func TestOkxUnifiedTradeWalletSpotAndPerpShareUSDT(t *testing.T) {
	ex := NewSimExchange()
	if err := ex.InitBalances("u", seedUSDT(ctypes.WalletTypeTrade, decimal.NewFromInt(10000))); err != nil {
		t.Fatal(err)
	}
	spotSym := Symbol("OKXBTCSPOT")
	perpSym := Symbol("OKXBTCPERP")
	spotIns := &Instrument{
		Symbol:      spotSym,
		Kind:        KindSpot,
		Exchange:    ctypes.ExchangeOkx,
		Market:      ctypes.MarketTypeSpot,
		Base:        Asset("BTC"),
		Quote:       Asset("USDT"),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(1),
		TakerFeeBps: 0,
	}
	perpIns := &Instrument{
		Symbol:             perpSym,
		Kind:               KindPerp,
		Exchange:           ctypes.ExchangeOkx,
		Market:             ctypes.MarketTypeFuture,
		Base:               Asset("BTC"),
		Quote:              Asset("USDT"),
		PriceTick:          decimal.NewFromInt(1),
		QtyStep:            decimal.NewFromInt(1),
		MinQty:             decimal.NewFromInt(1),
		MinNotional:        decimal.NewFromInt(1),
		TakerFeeBps:        0,
		LeverageMax:        10,
		ContractMultiplier: decimal.NewFromInt(1),
	}
	if wt := spotIns.WalletType(); wt != ctypes.WalletTypeTrade {
		t.Fatalf("spot wallet want trade got %s", wt)
	}
	if wt := perpIns.WalletType(); wt != ctypes.WalletTypeTrade {
		t.Fatalf("perp wallet want trade got %s", wt)
	}

	depthSpot := NewMarketDepth()
	_ = depthSpot.ApplySnapshot(&OrderBook{
		Symbol: spotSym,
		SeqId:  1,
		Asks:   []OrderBookLevel{{Price: decimal.NewFromInt(100), Size: decimal.NewFromInt(10)}},
		Bids:   []OrderBookLevel{{Price: decimal.NewFromInt(99), Size: decimal.NewFromInt(10)}},
	})
	depthPerp := NewMarketDepth()
	_ = depthPerp.ApplySnapshot(&OrderBook{
		Symbol: perpSym,
		SeqId:  1,
		Asks:   []OrderBookLevel{{Price: decimal.NewFromInt(1000), Size: decimal.NewFromInt(10)}},
		Bids:   []OrderBookLevel{{Price: decimal.NewFromInt(999), Size: decimal.NewFromInt(10)}},
	})
	_ = ex.RegisterInstrument(spotIns)
	_ = ex.RegisterInstrument(perpIns)
	_ = ex.BindDepth(spotSym, depthSpot)
	_ = ex.BindDepth(perpSym, depthPerp)

	if _, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "u",
		Symbol:    spotSym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Qty:       decimal.NewFromInt(1),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "u",
		Symbol:    perpSym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Intent:    IntentOpen,
		Leverage:  5,
		Qty:       decimal.NewFromInt(1),
	}); err != nil {
		t.Fatal(err)
	}

	tradeUSDT := BalanceKey{Wallet: ctypes.WalletTypeTrade, Asset: Asset("USDT")}
	bal := ex.GetBalances("u")
	// Spot 1 @ 100 + perp IM 1 @ 1000 / lev 5 = 100 + 200
	want := decimal.NewFromInt(9700)
	if !bal[tradeUSDT].Equal(want) {
		t.Fatalf("trade USDT=%s want %s", bal[tradeUSDT], want)
	}
}

// TestWalletBucketsIsolateSameAssetCode covers Binance-style separation: spot vs futures USDT buckets.
func TestWalletBucketsIsolateSameAssetCode(t *testing.T) {
	ex := NewSimExchange()
	if err := ex.InitBalances("u", map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeSpot:   {Asset("USDT"): decimal.NewFromInt(100)},
		ctypes.WalletTypeFuture: {Asset("USDT"): decimal.NewFromInt(500)},
	}); err != nil {
		t.Fatal(err)
	}

	spotSym := Symbol("BTCUSDT")
	perpSym := Symbol("BTC-PERP")
	spotIns := &Instrument{
		Symbol:      spotSym,
		Kind:        KindSpot,
		Exchange:    ctypes.ExchangeBinance,
		Market:      ctypes.MarketTypeSpot,
		Base:        Asset("BTC"),
		Quote:       Asset("USDT"),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(1),
		TakerFeeBps: 0,
	}
	perpIns := &Instrument{
		Symbol:             perpSym,
		Kind:               KindPerp,
		Exchange:           ctypes.ExchangeBinance,
		Market:             ctypes.MarketTypeFuture,
		Base:               Asset("BTC"),
		Quote:              Asset("USDT"),
		PriceTick:          decimal.NewFromInt(1),
		QtyStep:            decimal.NewFromInt(1),
		MinQty:             decimal.NewFromInt(1),
		MinNotional:        decimal.NewFromInt(1),
		TakerFeeBps:        0,
		LeverageMax:        10,
		ContractMultiplier: decimal.NewFromInt(1),
	}
	depthSpot := NewMarketDepth()
	_ = depthSpot.ApplySnapshot(&OrderBook{
		Symbol: spotSym,
		SeqId:  1,
		Asks:   []OrderBookLevel{{Price: decimal.NewFromInt(100), Size: decimal.NewFromInt(10)}},
		Bids:   []OrderBookLevel{{Price: decimal.NewFromInt(99), Size: decimal.NewFromInt(10)}},
	})
	depthPerp := NewMarketDepth()
	_ = depthPerp.ApplySnapshot(&OrderBook{
		Symbol: perpSym,
		SeqId:  1,
		Asks:   []OrderBookLevel{{Price: decimal.NewFromInt(1000), Size: decimal.NewFromInt(10)}},
		Bids:   []OrderBookLevel{{Price: decimal.NewFromInt(999), Size: decimal.NewFromInt(10)}},
	})

	_ = ex.RegisterInstrument(spotIns)
	_ = ex.RegisterInstrument(perpIns)
	_ = ex.BindDepth(spotSym, depthSpot)
	_ = ex.BindDepth(perpSym, depthPerp)

	if _, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "u",
		Symbol:    spotSym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Qty:       decimal.NewFromInt(1),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "u",
		Symbol:    perpSym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Intent:    IntentOpen,
		Leverage:  5,
		Qty:       decimal.NewFromInt(1),
	}); err != nil {
		t.Fatal(err)
	}

	bal := ex.GetBalances("u")
	spotUSDT := BalanceKey{Wallet: ctypes.WalletTypeSpot, Asset: Asset("USDT")}
	futUSDT := BalanceKey{Wallet: ctypes.WalletTypeFuture, Asset: Asset("USDT")}
	if !bal[spotUSDT].Equal(decimal.Zero) {
		t.Fatalf("spot USDT=%s want 0", bal[spotUSDT])
	}
	if !bal[futUSDT].LessThan(decimal.NewFromInt(500)) {
		t.Fatalf("future USDT=%s want < 500", bal[futUSDT])
	}
}

func TestSimExchangeSpotMarketBuy(t *testing.T) {
	sym := Symbol("BTCUSDT")
	ins := &Instrument{
		Symbol:      sym,
		Kind:        KindSpot,
		Exchange:    ctypes.ExchangeBinance,
		Market:      ctypes.MarketTypeSpot,
		Base:        Asset("BTC"),
		Quote:       Asset("USDT"),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(1),
		TakerFeeBps: 10,
		MakerFeeBps: 5,
	}
	depth := NewMarketDepth()
	_ = depth.ApplySnapshot(&OrderBook{
		SeqId: 1,
		Asks: []OrderBookLevel{
			{Price: decimal.NewFromInt(100), Size: decimal.NewFromInt(5)},
		},
		Bids: []OrderBookLevel{
			{Price: decimal.NewFromInt(99), Size: decimal.NewFromInt(1)},
		},
	})

	ex := NewSimExchange()
	ex.Portfolio().SetBalance("default", ctypes.WalletTypeSpot, Asset("USDT"), decimal.NewFromInt(10000))
	_ = ex.RegisterInstrument(ins)
	_ = ex.BindDepth(sym, depth)

	res, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "default",
		Symbol:    sym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Qty:       decimal.NewFromInt(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Order.Status != OrderStatusFilled {
		t.Fatalf("status %v", res.Order.Status)
	}
	if len(res.Fills) != 1 {
		t.Fatalf("fills %d", len(res.Fills))
	}
	bal := ex.GetBalances("default")
	k := BalanceKey{Wallet: ctypes.WalletTypeSpot, Asset: Asset("BTC")}
	if !bal[k].Equal(decimal.NewFromInt(2)) {
		t.Fatalf("base %s", bal[k])
	}
}

func TestSimExchangePerpOpenClose(t *testing.T) {
	sym := Symbol("BTC-PERP")
	ins := &Instrument{
		Symbol:             sym,
		Kind:               KindPerp,
		Exchange:           ctypes.ExchangeBinance,
		Market:             ctypes.MarketTypeFuture,
		Base:               Asset("BTC"),
		Quote:              Asset("USDT"),
		PriceTick:          decimal.NewFromInt(1),
		QtyStep:            decimal.NewFromInt(1),
		MinQty:             decimal.NewFromInt(1),
		MinNotional:        decimal.NewFromInt(1),
		TakerFeeBps:        0,
		MakerFeeBps:        0,
		LeverageMax:        10,
		ContractMultiplier: decimal.NewFromInt(1),
	}
	depth := NewMarketDepth()
	_ = depth.ApplySnapshot(&OrderBook{
		SeqId: 1,
		Asks: []OrderBookLevel{
			{Price: decimal.NewFromInt(1000), Size: decimal.NewFromInt(10)},
		},
		Bids: []OrderBookLevel{
			{Price: decimal.NewFromInt(999), Size: decimal.NewFromInt(10)},
		},
	})
	ex := NewSimExchange()
	ex.Portfolio().SetBalance("default", ctypes.WalletTypeFuture, Asset("USDT"), decimal.NewFromInt(100000))
	_ = ex.RegisterInstrument(ins)
	_ = ex.BindDepth(sym, depth)

	_, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "default",
		Symbol:    sym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Intent:    IntentOpen,
		Leverage:  5,
		Qty:       decimal.NewFromInt(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	pos, ok := ex.GetPosition("default", sym)
	if !ok || !pos.Qty.Equal(decimal.NewFromInt(2)) {
		t.Fatalf("position %+v", pos)
	}

	_, err = ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID:  "default",
		Symbol:     sym,
		OrderType:  OrderTypeMarket,
		Side:       SideSell,
		Intent:     IntentClose,
		ReduceOnly: true,
		Leverage:   5,
		Qty:        decimal.NewFromInt(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	pos, ok = ex.GetPosition("default", sym)
	if !ok || !pos.Qty.IsZero() {
		t.Fatalf("position after close %+v", pos)
	}
}

func TestSimExchangeRestingLimitDepthTrigger(t *testing.T) {
	sym := Symbol("ETHUSDT")
	ins := &Instrument{
		Symbol:      sym,
		Kind:        KindSpot,
		Exchange:    ctypes.ExchangeBinance,
		Market:      ctypes.MarketTypeSpot,
		Base:        Asset("ETH"),
		Quote:       Asset("USDT"),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(1),
		TakerFeeBps: 0,
		MakerFeeBps: 0,
	}
	depth := NewMarketDepth()
	_ = depth.ApplySnapshot(&OrderBook{
		SeqId: 1,
		Asks: []OrderBookLevel{
			{Price: decimal.NewFromInt(2000), Size: decimal.NewFromInt(10)},
		},
	})
	ex := NewSimExchange()
	ex.Portfolio().SetBalance("default", ctypes.WalletTypeSpot, Asset("USDT"), decimal.NewFromInt(1_000_000))
	_ = ex.RegisterInstrument(ins)
	_ = ex.BindDepth(sym, depth)

	_, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "default",
		Symbol:    sym,
		OrderType: OrderTypeLimit,
		Side:      SideBuy,
		Price:     decimal.NewFromInt(1990),
		Qty:       decimal.NewFromInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(ex.ListOpenOrders("default", sym)); n != 1 {
		t.Fatalf("open orders %d", n)
	}

	_ = depth.ApplyDelta(&OrderBook{
		PrevSeqId: 1,
		SeqId:     2,
		Asks: []OrderBookLevel{
			{Price: decimal.NewFromInt(2000), Size: decimal.NewFromInt(10)},
			{Price: decimal.NewFromInt(1980), Size: decimal.NewFromInt(5)},
		},
	})
	evs, err := ex.OnDepthUpdated(sym)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("events %d", len(evs))
	}
	if n := len(ex.ListOpenOrders("default", sym)); n != 0 {
		t.Fatalf("expected filled, open %d", n)
	}
}

func TestSimExchangeMultiAccountIsolation(t *testing.T) {
	sym := Symbol("XRPUSDT")
	ins := &Instrument{
		Symbol:      sym,
		Kind:        KindSpot,
		Exchange:    ctypes.ExchangeBinance,
		Market:      ctypes.MarketTypeSpot,
		Base:        Asset("XRP"),
		Quote:       Asset("USDT"),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(1),
		TakerFeeBps: 0,
		MakerFeeBps: 0,
	}
	depth := NewMarketDepth()
	_ = depth.ApplySnapshot(&OrderBook{
		SeqId: 1,
		Asks:  []OrderBookLevel{{Price: decimal.NewFromInt(10), Size: decimal.NewFromInt(100)}},
		Bids:  []OrderBookLevel{{Price: decimal.NewFromInt(9), Size: decimal.NewFromInt(100)}},
	})
	ex := NewSimExchange()
	_ = ex.InitBalances("a1", seedUSDT(ctypes.WalletTypeSpot, decimal.NewFromInt(100)))
	_ = ex.InitBalances("a2", seedUSDT(ctypes.WalletTypeSpot, decimal.NewFromInt(100)))
	_ = ex.RegisterInstrument(ins)
	_ = ex.BindDepth(sym, depth)

	_, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "a1",
		Symbol:    sym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Qty:       decimal.NewFromInt(5),
	})
	if err != nil {
		t.Fatal(err)
	}

	b1 := ex.GetBalances("a1")
	b2 := ex.GetBalances("a2")
	xrpKey := BalanceKey{Wallet: ctypes.WalletTypeSpot, Asset: Asset("XRP")}
	if !b1[xrpKey].Equal(decimal.NewFromInt(5)) {
		t.Fatalf("a1 xrp=%s", b1[xrpKey])
	}
	if !b2[xrpKey].IsZero() {
		t.Fatalf("a2 xrp=%s", b2[xrpKey])
	}
}
