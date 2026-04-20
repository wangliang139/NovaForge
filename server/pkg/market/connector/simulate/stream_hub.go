package simulate

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type fanoutListener struct {
	out   chan *ctypes.Message
	errCh chan error
	once  sync.Once
}

func (l *fanoutListener) shutdown() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.out != nil {
			close(l.out)
		}
	})
}

// publicStreamHub multiplexes one public Subscribe per StreamSelector.Key.
type publicStreamHub struct {
	rt *VenueRuntime

	stream ctypes.StreamType
	key    string

	symOwned      ctypes.Symbol
	symPtr        *ctypes.Symbol
	intervalOwned ctypes.Interval
	intervalPtr   *ctypes.Interval

	mu sync.Mutex

	pinned    bool
	running   bool
	cancel    context.CancelFunc
	nextID    int64
	listeners map[int64]*fanoutListener
}

const (
	streamHubInitialBackoff = time.Second
	streamHubMaxBackoff     = 30 * time.Second
)

func newPublicStreamHub(rt *VenueRuntime, sel ctypes.StreamSelector, key string) *publicStreamHub {
	h := &publicStreamHub{
		rt:        rt,
		stream:    sel.Stream,
		key:       key,
		listeners: make(map[int64]*fanoutListener),
	}
	if sel.Symbol != nil && sel.Symbol.IsValid() {
		cp := *sel.Symbol
		h.symOwned = cp
		h.symPtr = &h.symOwned
	}
	if sel.Interval != nil && sel.Interval.Valid() {
		ip := *sel.Interval
		h.intervalOwned = ip
		h.intervalPtr = &h.intervalOwned
	}
	return h
}

func (h *publicStreamHub) selector() ctypes.StreamSelector {
	s := ctypes.StreamSelector{Stream: h.stream}
	if h.symPtr != nil {
		s.Symbol = h.symPtr
	}
	if h.intervalPtr != nil {
		s.Interval = h.intervalPtr
	}
	return s
}

func validateSimulatePublicSelector(sel ctypes.StreamSelector) error {
	switch sel.Stream {
	case ctypes.StreamTypeKline:
		return sel.Validate()
	default:
		if sel.Symbol == nil || !sel.Symbol.IsValid() {
			return fmt.Errorf("simulate: stream %s requires symbol", sel.Stream)
		}
		return nil
	}
}

func (h *publicStreamHub) setPinned() {
	h.mu.Lock()
	h.pinned = true
	h.mu.Unlock()
}

func (h *publicStreamHub) ensureRunning() {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.running = true
	h.cancel = cancel
	h.mu.Unlock()
	go h.supervisor(ctx)
}

func (h *publicStreamHub) supervisor(hubCtx context.Context) {
	defer h.onSupervisorFinished()

	sel := h.selector()
	backoff := streamHubInitialBackoff

	for {
		select {
		case <-hubCtx.Done():
			return
		default:
		}

		pub, err := h.rt.Public.Subscribe(hubCtx, sel)
		if err != nil {
			log.Printf("simulate: publicConn.Subscribe failed stream=%s key=%s: %v", h.stream, h.key, err)
			h.broadcastErr(err)
			if !sleepOrDone(hubCtx, backoff) {
				return
			}
			backoff = growBackoff(backoff)
			continue
		}
		backoff = streamHubInitialBackoff

		h.pumpSession(hubCtx, pub)

		pub.Stop()

		select {
		case <-hubCtx.Done():
			return
		default:
		}
		if !sleepOrDone(hubCtx, backoff) {
			return
		}
		backoff = growBackoff(backoff)
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func growBackoff(cur time.Duration) time.Duration {
	next := cur * 2
	if next > streamHubMaxBackoff {
		return streamHubMaxBackoff
	}
	return next
}

func (h *publicStreamHub) pumpSession(hubCtx context.Context, pub *mdtypes.StreamHandle) {
	for {
		select {
		case <-hubCtx.Done():
			return
		case err, ok := <-pub.ErrCh:
			if !ok {
				return
			}
			if err != nil {
				h.broadcastErr(err)
			}
		case msg, ok := <-pub.C:
			if !ok {
				return
			}
			if msg != nil {
				h.ingestOnce(msg)
				h.broadcastMsg(msg)
			}
		}
	}
}

func (h *publicStreamHub) ingestOnce(msg *ctypes.Message) {
	switch h.stream {
	case ctypes.StreamTypeDepth:
		h.rt.dispatchDepthIngest(msg)
	case ctypes.StreamTypeTicker:
		h.rt.dispatchTickerIngest(msg)
	case ctypes.StreamTypeMarkPrice:
		if msg.MarkPrice != nil {
			h.rt.dispatchMarkPricePayload(msg.MarkPrice)
		}
	default:
	}
}

func (h *publicStreamHub) onSupervisorFinished() {
	h.mu.Lock()
	h.running = false
	cancel := h.cancel
	h.cancel = nil
	pinned := h.pinned
	h.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	h.closeAllListeners()

	if !pinned {
		h.rt.removeStreamHub(h.key)
	}
}

func (h *publicStreamHub) stopPublicLocked() {
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

func (h *publicStreamHub) broadcastMsg(msg *ctypes.Message) {
	h.mu.Lock()
	ls := make([]*fanoutListener, 0, len(h.listeners))
	for _, l := range h.listeners {
		ls = append(ls, l)
	}
	h.mu.Unlock()

	for _, l := range ls {
		if l == nil {
			continue
		}
		select {
		case l.out <- msg:
		default:
		}
	}
}

func (h *publicStreamHub) broadcastErr(err error) {
	h.mu.Lock()
	ls := make([]*fanoutListener, 0, len(h.listeners))
	for _, l := range h.listeners {
		ls = append(ls, l)
	}
	h.mu.Unlock()

	for _, l := range ls {
		if l == nil {
			continue
		}
		select {
		case l.errCh <- err:
		default:
		}
	}
}

func (h *publicStreamHub) closeAllListeners() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, l := range h.listeners {
		l.shutdown()
	}
	h.listeners = make(map[int64]*fanoutListener)
}

func (h *publicStreamHub) registerListener(out chan *ctypes.Message, errCh chan error) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextID++
	id := h.nextID
	h.listeners[id] = &fanoutListener{out: out, errCh: errCh}
	return id
}

func (h *publicStreamHub) unregisterListener(id int64) {
	h.mu.Lock()
	l := h.listeners[id]
	delete(h.listeners, id)
	n := len(h.listeners)
	pinned := h.pinned
	running := h.running
	h.mu.Unlock()

	if l != nil {
		l.shutdown()
	}

	if n == 0 && !pinned && running {
		h.mu.Lock()
		h.stopPublicLocked()
		h.mu.Unlock()
	}
}

func (h *publicStreamHub) attachListener(ctx context.Context) (*mdtypes.StreamHandle, error) {
	out := make(chan *ctypes.Message, 256)
	errCh := make(chan error, 1)
	stopC := make(chan struct{})
	doneC := make(chan struct{})

	id := h.registerListener(out, errCh)

	go func() {
		defer close(doneC)
		select {
		case <-ctx.Done():
		case <-stopC:
		}
		h.unregisterListener(id)
	}()

	h.ensureRunning()
	return mdtypes.BuildHandle(ctx, out, errCh, stopC, doneC), nil
}

func (rt *VenueRuntime) dispatchDepthIngest(msg *ctypes.Message) {
	if msg == nil || msg.Depth == nil || !msg.Depth.Symbol.IsValid() {
		return
	}
	sym := msg.Depth.Symbol

	rt.connsMu.RLock()
	conns := make([]*Connector, 0, len(rt.conns))
	for _, c := range rt.conns {
		conns = append(conns, c...)
	}
	rt.connsMu.RUnlock()

	before := make([]AccountSnapshot, len(conns))
	for i, c := range conns {
		before[i] = rt.Engine.AccountSnapshot(c.accountID, Symbol(sym.String()))
	}

	matchResting := rt.SymbolSimReady(sym)
	_, err := rt.Engine.ApplyDepthBook(msg.Depth, matchResting)
	if err != nil {
		return
	}
}

func (rt *VenueRuntime) dispatchTickerIngest(msg *ctypes.Message) {
	if msg == nil || msg.Ticker == nil || !msg.Ticker.Symbol.IsValid() {
		return
	}
	rt.Quotes.Update(QuoteTick{
		Symbol: Symbol(msg.Ticker.Symbol.String()),
		Last:   msg.Ticker.LastPrice,
		Mark:   msg.Ticker.LastPrice,
		Ts:     msg.Ticker.Ts,
	})
}

func (rt *VenueRuntime) dispatchMarkPricePayload(mp *ctypes.MarkPrice) {
	if mp == nil || !mp.Symbol.IsValid() || !mp.MarkPrice.GreaterThan(decimal.Zero) {
		return
	}
	sym := Symbol(mp.Symbol.String())
	prev, _ := rt.Quotes.Get(sym)
	last := prev.Last
	if !last.GreaterThan(decimal.Zero) {
		last = mp.MarkPrice
	}
	rt.Quotes.Update(QuoteTick{
		Symbol: sym,
		Last:   last,
		Mark:   mp.MarkPrice,
		Ts:     mp.Ts,
	})

	if !rt.SymbolSimReady(mp.Symbol) {
		return
	}

	rt.connsMu.RLock()
	conns := make([]*Connector, 0, len(rt.conns))
	for _, c := range rt.conns {
		conns = append(conns, c...)
	}
	rt.connsMu.RUnlock()
	for _, c := range conns {
		c.tryLiquidateAfterMark(mp.Symbol, mp.MarkPrice)
	}
}

// onSymbolSimReady runs a one-off maker match against the current depth and a liquidation
// pass after the symbol first completes ensureSymbolInitialized (depth snapshots use matchResting=false).
func (rt *VenueRuntime) onSymbolSimReady(sym ctypes.Symbol) {
	if !sym.IsValid() {
		return
	}
	paperSym := Symbol(sym.String())

	rt.connsMu.RLock()
	conns := make([]*Connector, 0, len(rt.conns))
	for _, c := range rt.conns {
		conns = append(conns, c...)
	}
	rt.connsMu.RUnlock()

	rt.Engine.OnDepthUpdated(paperSym)

	if sym.Type != ctypes.MarketTypeFuture {
		return
	}
	q, ok := rt.Quotes.Get(paperSym)
	if !ok || !q.Mark.GreaterThan(decimal.Zero) {
		return
	}
	for _, c := range conns {
		c.tryLiquidateAfterMark(sym, q.Mark)
	}
}
