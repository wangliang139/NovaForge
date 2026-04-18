package simulate

import (
	"context"
	"errors"
)

// Coordinator applies snapshots and deltas from a DataLoader, resynchronizing via
// PullSnapshot when a sequence gap is detected. After a successful snapshot, a
// delta whose PrevSeqId still does not match the new lastSeqId is discarded (stale).
type Coordinator struct {
	depth  *MarketDepth
	loader DataLoader
}

// NewCoordinator builds a coordinator bound to depth and loader.
func NewCoordinator(depth *MarketDepth, loader DataLoader) *Coordinator {
	return &Coordinator{depth: depth, loader: loader}
}

// Depth returns the underlying market depth.
func (c *Coordinator) Depth() *MarketDepth {
	return c.depth
}

// Bootstrap pulls an initial snapshot and applies it.
func (c *Coordinator) Bootstrap(ctx context.Context) error {
	snap, err := c.loader.PullSnapshot(ctx)
	if err != nil {
		return err
	}
	return c.depth.ApplySnapshot(&snap)
}

// HandleDelta applies one incremental event, resyncing once on sequence gap.
func (c *Coordinator) HandleDelta(ctx context.Context, ob *OrderBook) error {
	if ob == nil {
		return nil
	}
	err := c.depth.ApplyDelta(ob)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrSeqGap) {
		return err
	}
	if rerr := c.resync(ctx); rerr != nil {
		return rerr
	}
	// Retry once after snapshot; discard if still not contiguous.
	if err2 := c.depth.ApplyDelta(ob); err2 != nil {
		if errors.Is(err2, ErrSeqGap) {
			return nil
		}
		return err2
	}
	return nil
}

func (c *Coordinator) resync(ctx context.Context) error {
	snap, err := c.loader.PullSnapshot(ctx)
	if err != nil {
		return err
	}
	return c.depth.ApplySnapshot(&snap)
}

// Run blocks reading deltas from the loader until ctx is cancelled or the channel closes.
func (c *Coordinator) Run(ctx context.Context) error {
	ch := c.loader.Deltas()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ob, ok := <-ch:
			if !ok {
				return nil
			}
			cp := ob
			if err := c.HandleDelta(ctx, &cp); err != nil {
				return err
			}
		}
	}
}
