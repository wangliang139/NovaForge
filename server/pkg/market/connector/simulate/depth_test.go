package simulate

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestApplySnapshotAndDeltaPatch(t *testing.T) {
	d := NewMarketDepth()
	p100 := decimal.NewFromInt(100)
	p101 := decimal.NewFromInt(101)
	snap := OrderBook{
		SeqId: 10,
		Bids: []OrderBookLevel{
			{Price: p100, Size: decimal.NewFromInt(5)},
		},
		Asks: []OrderBookLevel{
			{Price: p101, Size: decimal.NewFromInt(3)},
		},
	}
	if err := d.ApplySnapshot(&snap); err != nil {
		t.Fatal(err)
	}
	if d.LastSeqID() != 10 {
		t.Fatalf("last seq %d", d.LastSeqID())
	}

	delta := OrderBook{
		PrevSeqId: 10,
		SeqId:     11,
		Asks: []OrderBookLevel{
			{Price: p101, Size: decimal.NewFromInt(1)},
		},
	}
	if err := d.ApplyDelta(&delta); err != nil {
		t.Fatal(err)
	}
	_, sz, ok := d.BestAsk()
	if !ok || !sz.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("best ask size got %v ok=%v", sz, ok)
	}

	// bids side omitted in Go zero value would be nil for JSON missing - simulate by not setting Bids field
	delta2 := OrderBook{
		PrevSeqId: 11,
		SeqId:     12,
		Bids: []OrderBookLevel{
			{Price: p100, Size: decimal.NewFromInt(2)},
		},
	}
	if err := d.ApplyDelta(&delta2); err != nil {
		t.Fatal(err)
	}
	bp, bsz, ok := d.BestBid()
	if !ok || !bp.Equal(p100) || !bsz.Equal(decimal.NewFromInt(2)) {
		t.Fatalf("best bid %v %v", bp, bsz)
	}
}

func TestDeltaDeleteLevel(t *testing.T) {
	d := NewMarketDepth()
	p := decimal.NewFromInt(50)
	snap := OrderBook{
		SeqId: 1,
		Bids:  []OrderBookLevel{{Price: p, Size: decimal.NewFromInt(10)}},
		Asks:  []OrderBookLevel{{Price: decimal.NewFromInt(51), Size: decimal.NewFromInt(1)}},
	}
	if err := d.ApplySnapshot(&snap); err != nil {
		t.Fatal(err)
	}
	del := OrderBook{
		PrevSeqId: 1,
		SeqId:     2,
		Bids:      []OrderBookLevel{{Price: p, Size: decimal.Zero}},
	}
	if err := d.ApplyDelta(&del); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := d.BestBid(); ok {
		t.Fatal("expected no bids")
	}
}

func TestDeltaNotInitialized(t *testing.T) {
	d := NewMarketDepth()
	err := d.ApplyDelta(&OrderBook{PrevSeqId: 0, SeqId: 1})
	if err != ErrNotInitialized {
		t.Fatalf("want ErrNotInitialized got %v", err)
	}
}

func TestSeqGap(t *testing.T) {
	d := NewMarketDepth()
	_ = d.ApplySnapshot(&OrderBook{
		SeqId: 5,
		Bids:  []OrderBookLevel{{Price: decimal.NewFromInt(1), Size: decimal.NewFromInt(1)}},
		Asks:  []OrderBookLevel{{Price: decimal.NewFromInt(2), Size: decimal.NewFromInt(1)}},
	})
	err := d.ApplyDelta(&OrderBook{
		PrevSeqId: 4,
		SeqId:     6,
		Bids:      []OrderBookLevel{{Price: decimal.NewFromInt(1), Size: decimal.NewFromInt(2)}},
	})
	if err != ErrSeqGap {
		t.Fatalf("want ErrSeqGap got %v", err)
	}
}

func TestConsumeAskFills(t *testing.T) {
	d := NewMarketDepth()
	_ = d.ApplySnapshot(&OrderBook{
		SeqId: 1,
		Asks: []OrderBookLevel{
			{Price: decimal.NewFromInt(10), Size: decimal.NewFromInt(5)},
		},
	})
	d.ConsumeAskFills([]Fill{{Price: decimal.NewFromInt(10), Size: decimal.NewFromInt(2)}})
	_, sz, ok := d.BestAsk()
	if !ok || !sz.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("ask size %v ok=%v", sz, ok)
	}
}

func TestSimulateMarketBuy(t *testing.T) {
	d := NewMarketDepth()
	_ = d.ApplySnapshot(&OrderBook{
		SeqId: 1,
		Asks: []OrderBookLevel{
			{Price: decimal.NewFromInt(100), Size: decimal.NewFromInt(1)},
			{Price: decimal.NewFromInt(101), Size: decimal.NewFromInt(2)},
		},
	})
	fills, left, notional := SimulateMarketBuy(d, decimal.NewFromInt(2))
	if !left.IsZero() {
		t.Fatalf("leftover %s", left)
	}
	if len(fills) != 2 {
		t.Fatalf("fills %d", len(fills))
	}
	filledQty := decimal.NewFromInt(2)
	avg := AveragePrice(notional, filledQty)
	if !avg.Equal(decimal.RequireFromString("100.5")) {
		t.Fatalf("avg %s", avg)
	}
}
