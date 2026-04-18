package simulate

import "context"

// DataLoader is implemented by the host to actively fetch order book data.
// Deltas() returns a read-only channel of incremental OrderBook events.
type DataLoader interface {
	PullSnapshot(ctx context.Context) (OrderBook, error)
	Deltas() <-chan OrderBook
}
