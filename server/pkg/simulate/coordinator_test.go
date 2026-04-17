package simulate

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

type sliceLoader struct {
	snap OrderBook
	ch   chan OrderBook
}

func (s *sliceLoader) PullSnapshot(ctx context.Context) (OrderBook, error) {
	return s.snap, nil
}

func (s *sliceLoader) Deltas() <-chan OrderBook {
	return s.ch
}

func TestCoordinatorResyncRetry(t *testing.T) {
	d := NewMarketDepth()
	p1 := decimal.NewFromInt(1)
	p2 := decimal.NewFromInt(2)
	loader := &sliceLoader{
		snap: OrderBook{
			SeqId: 100,
			Bids:  []OrderBookLevel{{Price: p1, Size: decimal.NewFromInt(5)}},
			Asks:  []OrderBookLevel{{Price: p2, Size: decimal.NewFromInt(5)}},
		},
		ch: make(chan OrderBook, 2),
	}
	c := NewCoordinator(d, loader)

	if err := c.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}

	// gap then valid delta after implicit resync in HandleDelta
	stale := OrderBook{
		PrevSeqId: 99,
		SeqId:     101,
		Bids:      []OrderBookLevel{{Price: p1, Size: decimal.NewFromInt(1)}},
	}
	if err := c.HandleDelta(context.Background(), &stale); err != nil {
		t.Fatal(err)
	}
	// stale delta should be discarded; depth still matches snapshot
	if d.LastSeqID() != 100 {
		t.Fatalf("last seq %d", d.LastSeqID())
	}

	good := OrderBook{
		PrevSeqId: 100,
		SeqId:     101,
		Bids:      []OrderBookLevel{{Price: p1, Size: decimal.NewFromInt(1)}},
	}
	if err := c.HandleDelta(context.Background(), &good); err != nil {
		t.Fatal(err)
	}
	if d.LastSeqID() != 101 {
		t.Fatalf("last seq %d", d.LastSeqID())
	}
	bp, _, ok := d.BestBid()
	if !ok || !bp.Equal(p1) {
		t.Fatal("unexpected bid")
	}
}
