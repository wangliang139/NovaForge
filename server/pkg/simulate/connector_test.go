package simulate

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

func TestConnectorAdapterFullAccountingSpot(t *testing.T) {
	sym := Symbol("SOLUSDT")
	ex := NewSimExchange()
	if err := ex.InitBalances("u1", map[Asset]decimal.Decimal{
		Asset("USDT"): decimal.NewFromInt(1000),
	}); err != nil {
		t.Fatal(err)
	}
	ins := &Instrument{
		Symbol:      sym,
		Kind:        KindSpot,
		Base:        Asset("SOL"),
		Quote:       Asset("USDT"),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(1),
	}
	depth := NewMarketDepth()
	_ = depth.ApplySnapshot(&OrderBook{
		Symbol: sym,
		SeqId:  1,
		Asks: []OrderBookLevel{
			{Price: decimal.NewFromInt(100), Size: decimal.NewFromInt(10)},
		},
		Bids: []OrderBookLevel{
			{Price: decimal.NewFromInt(99), Size: decimal.NewFromInt(10)},
		},
	})
	if err := ex.RegisterInstrument(ins); err != nil {
		t.Fatal(err)
	}
	if err := ex.BindDepth(sym, depth); err != nil {
		t.Fatal(err)
	}
	adapter := NewConnectorAdapter(ex)

	res, err := adapter.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "u1",
		Symbol:    sym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Qty:       decimal.NewFromInt(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := adapter.GetOrder(context.Background(), "u1", sym, res.Order.ID)
	if !ok || got.Status != OrderStatusFilled {
		t.Fatalf("unexpected order: %+v", got)
	}
	bal := adapter.Balance(context.Background(), "u1")
	if !bal[Asset("SOL")].Equal(decimal.NewFromInt(2)) {
		t.Fatalf("unexpected SOL balance: %s", bal[Asset("SOL")])
	}
}

func TestConnectorAdapterMultiAccountBalance(t *testing.T) {
	sym := Symbol("ADAUSDT")
	ex := NewSimExchange()
	_ = ex.InitBalances("u1", map[Asset]decimal.Decimal{Asset("USDT"): decimal.NewFromInt(1000)})
	_ = ex.InitBalances("u2", map[Asset]decimal.Decimal{Asset("USDT"): decimal.NewFromInt(1000)})
	ins := &Instrument{
		Symbol:      sym,
		Kind:        KindSpot,
		Base:        Asset("ADA"),
		Quote:       Asset("USDT"),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(1),
	}
	depth := NewMarketDepth()
	_ = depth.ApplySnapshot(&OrderBook{
		Symbol: sym,
		SeqId:  1,
		Asks:   []OrderBookLevel{{Price: decimal.NewFromInt(10), Size: decimal.NewFromInt(100)}},
		Bids:   []OrderBookLevel{{Price: decimal.NewFromInt(9), Size: decimal.NewFromInt(100)}},
	})
	_ = ex.RegisterInstrument(ins)
	_ = ex.BindDepth(sym, depth)
	adapter := NewConnectorAdapter(ex)

	_, err := adapter.PlaceOrder(context.Background(), PlaceOrderRequest{
		AccountID: "u1",
		Symbol:    sym,
		OrderType: OrderTypeMarket,
		Side:      SideBuy,
		Qty:       decimal.NewFromInt(3),
	})
	if err != nil {
		t.Fatal(err)
	}
	b1 := adapter.Balance(context.Background(), "u1")
	b2 := adapter.Balance(context.Background(), "u2")
	if !b1[Asset("ADA")].Equal(decimal.NewFromInt(3)) {
		t.Fatalf("u1 ada=%s", b1[Asset("ADA")])
	}
	if !b2[Asset("ADA")].IsZero() {
		t.Fatalf("u2 ada=%s", b2[Asset("ADA")])
	}
}
