package simulate

import (
	"container/heap"
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

const (
	fundingRetryDelay  = 2 * time.Second
	fundingMinTimerDur = 5 * time.Millisecond
	fundingFetchTries  = 30
)

type fundingItem struct {
	at  time.Time
	sym Symbol
}

// fundingHeap is a min-heap by next settlement time (then symbol) across perp symbols on one venue.
type fundingHeap []fundingItem

func (h fundingHeap) Len() int { return len(h) }
func (h fundingHeap) Less(i, j int) bool {
	if !h[i].at.Equal(h[j].at) {
		return h[i].at.Before(h[j].at)
	}
	return h[i].sym < h[j].sym
}
func (h fundingHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *fundingHeap) Push(x any) {
	*h = append(*h, x.(fundingItem))
}

func (h *fundingHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = fundingItem{}
	*h = old[:n-1]
	return x
}

func (rt *VenueRuntime) engineNow() time.Time {
	if rt == nil || rt.Engine == nil {
		return time.Now().UTC()
	}
	return rt.Engine.now()
}

// registerFundingSymbol schedules perp funding settlement for sym (paper venue). Idempotent per symbol readiness.
func (rt *VenueRuntime) registerFundingSymbol(sym ctypes.Symbol) {
	if rt == nil || sym.Type != ctypes.MarketTypeFuture || !sym.IsValid() {
		return
	}
	go rt.bootstrapFundingForTypesSymbol(sym)
}

func (rt *VenueRuntime) bootstrapFundingForTypesSymbol(sym ctypes.Symbol) {
	ctx := context.Background()
	fr := rt.fetchFundingRateWithRetries(ctx, sym)
	paper := Symbol(sym.String())
	now := rt.engineNow().UTC()
	next := now.Add(fundingRetryDelay)
	if fr != nil && !fr.NextFundingTime.IsZero() {
		next = fr.NextFundingTime.UTC()
		if !next.After(now) {
			next = now
		}
	}
	rt.fundingMu.Lock()
	defer rt.fundingMu.Unlock()
	rt.fundingUpsertLocked(paper, next)
	rt.pruneStaleHeapLocked()
	rt.armFundingTimerLocked()
}

func (rt *VenueRuntime) fetchFundingRateWithRetries(ctx context.Context, sym ctypes.Symbol) *ctypes.FundingRate {
	for i := 0; i < fundingFetchTries; i++ {
		fr, err := rt.Public.FundingRate(ctx, sym)
		if err == nil && fr != nil && !fr.NextFundingTime.IsZero() {
			return fr
		}
		if err != nil {
			log.Debug().Err(err).Str("symbol", sym.String()).Msg("simulate: funding rate fetch retry")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(fundingRetryDelay):
		}
	}
	return nil
}

func (rt *VenueRuntime) fundingUpsertLocked(sym Symbol, at time.Time) {
	if rt.fundingNext == nil {
		rt.fundingNext = make(map[Symbol]time.Time)
	}
	at = at.UTC()
	rt.fundingNext[sym] = at
	if rt.fundingHeap == nil {
		h := make(fundingHeap, 0, 4)
		rt.fundingHeap = &h
	}
	heap.Push(rt.fundingHeap, fundingItem{at: at, sym: sym})
}

func (rt *VenueRuntime) pruneStaleHeapLocked() {
	for rt.fundingHeap != nil && rt.fundingHeap.Len() > 0 {
		top := (*rt.fundingHeap)[0]
		want, ok := rt.fundingNext[top.sym]
		if ok && want.Equal(top.at) {
			return
		}
		_ = heap.Pop(rt.fundingHeap)
	}
}

func (rt *VenueRuntime) armFundingTimerLocked() {
	if rt.fundingTimer != nil {
		rt.fundingTimer.Stop()
		rt.fundingTimer = nil
	}
	rt.pruneStaleHeapLocked()
	if rt.fundingHeap == nil || rt.fundingHeap.Len() == 0 {
		return
	}
	top := (*rt.fundingHeap)[0]
	d := top.at.Sub(rt.engineNow())
	if d < 0 {
		d = 0
	}
	if d < fundingMinTimerDur {
		d = fundingMinTimerDur
	}
	rt.fundingTimer = time.AfterFunc(d, func() { rt.fundingWake() })
}

func (rt *VenueRuntime) fundingWake() {
	rt.fundingWakeMu.Lock()
	defer rt.fundingWakeMu.Unlock()

	now := rt.engineNow().UTC()
	var due []Symbol

	rt.fundingMu.Lock()
	for rt.fundingHeap != nil && rt.fundingHeap.Len() > 0 && !(*rt.fundingHeap)[0].at.After(now) {
		it := heap.Pop(rt.fundingHeap).(fundingItem)
		want, ok := rt.fundingNext[it.sym]
		if !ok || !want.Equal(it.at) {
			continue
		}
		delete(rt.fundingNext, it.sym)
		due = append(due, it.sym)
	}
	rt.fundingMu.Unlock()

	for _, sym := range due {
		rt.processFundingTick(sym)
	}

	rt.fundingMu.Lock()
	rt.pruneStaleHeapLocked()
	rt.armFundingTimerLocked()
	rt.fundingMu.Unlock()
}

func (rt *VenueRuntime) processFundingTick(sym Symbol) {
	ctx := context.Background()
	typesSym := toTypesSymbol(sym)
	fr, err := rt.Public.FundingRate(ctx, typesSym)
	now := rt.engineNow().UTC()
	if err != nil || fr == nil || fr.NextFundingTime.IsZero() {
		if err != nil {
			log.Debug().Err(err).Str("symbol", string(sym)).Msg("simulate: funding settlement fetch failed")
		}
		rt.fundingMu.Lock()
		rt.fundingUpsertLocked(sym, now.Add(fundingRetryDelay))
		rt.fundingMu.Unlock()
		return
	}

	mark := decimal.Zero
	if q, ok := rt.Quotes.Get(sym); ok && q.Mark.GreaterThan(decimal.Zero) {
		mark = q.Mark
	}
	if rt.Engine != nil {
		rt.Engine.settleFunding(sym, mark, fr.FundingRate)
	}

	next := fr.NextFundingTime.UTC()
	if !next.After(now) {
		next = now.Add(fundingMinTimerDur)
	}
	rt.fundingMu.Lock()
	rt.fundingUpsertLocked(sym, next)
	rt.fundingMu.Unlock()
}
