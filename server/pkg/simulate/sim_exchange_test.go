package simulate

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

func TestSimExchangeSpotMarketBuy(t *testing.T) {
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
	ex.Portfolio().SetBalance(Asset("USDT"), decimal.NewFromInt(10000))
	_ = ex.RegisterInstrument(ins)
	_ = ex.BindDepth(sym, depth)

	res, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
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
	bal := ex.GetBalances()
	if !bal[Asset("BTC")].Equal(decimal.NewFromInt(2)) {
		t.Fatalf("base %s", bal[Asset("BTC")])
	}
}

func TestSimExchangePerpOpenClose(t *testing.T) {
	sym := Symbol("BTC-PERP")
	ins := &Instrument{
		Symbol:        sym,
		Kind:          KindPerp,
		Base:          Asset("BTC"),
		Quote:         Asset("USDT"),
		PriceTick:     decimal.NewFromInt(1),
		QtyStep:       decimal.NewFromInt(1),
		MinQty:        decimal.NewFromInt(1),
		MinNotional:   decimal.NewFromInt(1),
		TakerFeeBps:   0,
		MakerFeeBps:   0,
		LeverageMax:   10,
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
	ex.Portfolio().SetBalance(Asset("USDT"), decimal.NewFromInt(100000))
	_ = ex.RegisterInstrument(ins)
	_ = ex.BindDepth(sym, depth)

	_, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
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
	pos, ok := ex.GetPosition(sym)
	if !ok || !pos.Qty.Equal(decimal.NewFromInt(2)) {
		t.Fatalf("position %+v", pos)
	}

	_, err = ex.PlaceOrder(context.Background(), PlaceOrderRequest{
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
	pos, ok = ex.GetPosition(sym)
	if !ok || !pos.Qty.IsZero() {
		t.Fatalf("position after close %+v", pos)
	}
}

func TestSimExchangeRestingLimitDepthTrigger(t *testing.T) {
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
	ex.Portfolio().SetBalance(Asset("USDT"), decimal.NewFromInt(1_000_000))
	_ = ex.RegisterInstrument(ins)
	_ = ex.BindDepth(sym, depth)

	_, err := ex.PlaceOrder(context.Background(), PlaceOrderRequest{
		Symbol:    sym,
		OrderType: OrderTypeLimit,
		Side:      SideBuy,
		Price:     decimal.NewFromInt(1990),
		Qty:       decimal.NewFromInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(ex.ListOpenOrders(sym)); n != 1 {
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
	if n := len(ex.ListOpenOrders(sym)); n != 0 {
		t.Fatalf("expected filled, open %d", n)
	}
}
