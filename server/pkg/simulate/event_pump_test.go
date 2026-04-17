package simulate

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestEventPumpReplayTimeAndTicker(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewReplayClock(start)
	ex := NewSimExchange(WithNowFn(clock.Now))
	sym := Symbol("BTCUSDT")
	ins := &Instrument{
		Symbol:      sym,
		Kind:        KindSpot,
		Base:        Asset("BTC"),
		Quote:       Asset("USDT"),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(1),
	}
	depth := NewMarketDepth()
	if err := ex.InitBalances("acct1", map[Asset]decimal.Decimal{Asset("USDT"): decimal.NewFromInt(10000)}); err != nil {
		t.Fatal(err)
	}
	if err := ex.RegisterInstrument(ins); err != nil {
		t.Fatal(err)
	}
	if err := ex.BindDepth(sym, depth); err != nil {
		t.Fatal(err)
	}

	pump := NewEventPump(clock, ex)
	pump.SetOrderLatency(100 * time.Millisecond)

	pump.SubmitDepthSnapshot(OrderBook{
		Symbol: sym,
		Ts:     start,
		SeqId:  1,
		Asks: []OrderBookLevel{
			{Price: decimal.NewFromInt(100), Size: decimal.NewFromInt(5)},
		},
		Bids: []OrderBookLevel{
			{Price: decimal.NewFromInt(99), Size: decimal.NewFromInt(5)},
		},
	})
	pump.SubmitTicker(Ticker{
		Symbol: sym,
		Last:   decimal.NewFromInt(100),
		Ts:     start.Add(10 * time.Millisecond),
	})
	pump.SubmitOrder(PlaceOrderRequest{
		AccountID: "acct1",
		Symbol:    sym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Qty:       decimal.NewFromInt(1),
	})

	if err := pump.RunAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !clock.Now().Equal(start.Add(100 * time.Millisecond)) {
		t.Fatalf("unexpected replay clock: %s", clock.Now())
	}
	tk, ok := pump.LatestTicker(sym)
	if !ok || !tk.Last.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("unexpected ticker: %+v", tk)
	}
	orderList := ex.ListOpenOrders("acct1", sym)
	if len(orderList) != 0 {
		t.Fatalf("market order should not rest, got %d", len(orderList))
	}
	bal := ex.GetBalances("acct1")
	if !bal[Asset("BTC")].Equal(decimal.NewFromInt(1)) {
		t.Fatalf("unexpected BTC balance: %s", bal[Asset("BTC")])
	}
}

func TestEventPumpDelayedCancel(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewReplayClock(start)
	ex := NewSimExchange(WithNowFn(clock.Now))
	sym := Symbol("ETHUSDT")
	ins := &Instrument{
		Symbol:      sym,
		Kind:        KindSpot,
		Base:        Asset("ETH"),
		Quote:       Asset("USDT"),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(1),
	}
	depth := NewMarketDepth()
	_ = depth.ApplySnapshot(&OrderBook{
		Symbol: sym,
		Ts:     start,
		SeqId:  1,
		Asks: []OrderBookLevel{
			{Price: decimal.NewFromInt(200), Size: decimal.NewFromInt(5)},
		},
		Bids: []OrderBookLevel{
			{Price: decimal.NewFromInt(199), Size: decimal.NewFromInt(5)},
		},
	})
	if err := ex.InitBalances("acct1", map[Asset]decimal.Decimal{Asset("USDT"): decimal.NewFromInt(10000)}); err != nil {
		t.Fatal(err)
	}
	if err := ex.RegisterInstrument(ins); err != nil {
		t.Fatal(err)
	}
	if err := ex.BindDepth(sym, depth); err != nil {
		t.Fatal(err)
	}

	res, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "acct1",
		Symbol:    sym,
		OrderType: OrderTypeLimit,
		Side:      SideBuy,
		Price:     decimal.NewFromInt(190),
		Qty:       decimal.NewFromInt(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	pump := NewEventPump(clock, ex)
	pump.SetCancelLatency(200 * time.Millisecond)
	pump.SubmitCancel("acct1", sym, res.Order.ID)

	if err := pump.RunUntil(context.Background(), start.Add(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(ex.ListOpenOrders("acct1", sym)) != 1 {
		t.Fatal("order should still be open before cancel latency")
	}
	if err := pump.RunAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(ex.ListOpenOrders("acct1", sym)) != 0 {
		t.Fatal("order should be cancelled after cancel latency")
	}
}
