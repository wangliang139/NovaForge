package simulate

import (
	"context"
	"fmt"
	"log"
	"sync"

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

// publicStreamHub multiplexes exactly one publicConn Subscribe per StreamSelector.Key (exchange-wide).
type publicStreamHub struct {
	st *exchangeState

	stream ctypes.StreamType
	key    string

	// Stable selector snapshot (owned pointers for Symbol/Interval).
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

func newPublicStreamHub(st *exchangeState, sel ctypes.StreamSelector, key string) *publicStreamHub {
	h := &publicStreamHub{
		st:        st,
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

func (st *exchangeState) getOrCreateStreamHub(sel ctypes.StreamSelector) *publicStreamHub {
	key := sel.Key()
	st.streamHubMu.Lock()
	defer st.streamHubMu.Unlock()
	if st.streamHubs == nil {
		st.streamHubs = make(map[string]*publicStreamHub)
	}
	if h, ok := st.streamHubs[key]; ok {
		return h
	}
	h := newPublicStreamHub(st, sel, key)
	st.streamHubs[key] = h
	return h
}

func (st *exchangeState) removeStreamHub(key string) {
	st.streamHubMu.Lock()
	defer st.streamHubMu.Unlock()
	delete(st.streamHubs, key)
}

// removeStreamHubIfMatches deletes the hub entry only if it still points at h (avoids races with recreation).
func (st *exchangeState) removeStreamHubIfMatches(key string, h *publicStreamHub) {
	st.streamHubMu.Lock()
	defer st.streamHubMu.Unlock()
	if existing, ok := st.streamHubs[key]; ok && existing == h {
		delete(st.streamHubs, key)
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
	sel := h.selector()
	pub, err := h.st.publicConn.Subscribe(ctx, sel)
	if err != nil {
		cancel()
		h.mu.Unlock()
		log.Printf("simulate: publicConn.Subscribe failed stream=%s key=%s: %v", h.stream, h.key, err)
		h.closeAllListeners()
		h.st.removeStreamHubIfMatches(h.key, h)
		return
	}
	h.running = true
	h.cancel = cancel
	go h.run(ctx, pub)
	h.mu.Unlock()
}

// run pumps one public subscription; when the connection ends, pinned hubs close all listener
// channels — clients must Subscribe again (or rely on ensureDepthStreamStarted / Warm to restart ingest).
func (h *publicStreamHub) run(ctx context.Context, pub *mdtypes.StreamHandle) {
	defer pub.Stop()
	defer h.onRunFinished()

	for {
		select {
		case <-ctx.Done():
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
		h.st.dispatchDepthIngest(msg)
	case ctypes.StreamTypeTicker:
		h.st.dispatchTickerIngest(msg)
	case ctypes.StreamTypeMarkPrice:
		if msg.MarkPrice != nil {
			h.st.dispatchMarkPricePayload(msg.MarkPrice)
		}
	default:
		// Trade/Kline/Social : fan-out only for simulate UI / strategies.
	}
}

func (h *publicStreamHub) onRunFinished() {
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
		h.st.removeStreamHub(h.key)
	}
}

func (h *publicStreamHub) stopPublicLocked() {
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

// broadcastMsg is best-effort: slow consumers may drop updates when out channel blocks (see select default).
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

// broadcastErr is best-effort like broadcastMsg.
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

// --- dispatch: shared simulate state + per-connector account notifications ---

func (st *exchangeState) dispatchDepthIngest(msg *ctypes.Message) {
	if msg == nil || msg.Depth == nil || !msg.Depth.Symbol.IsValid() {
		return
	}
	sym := msg.Depth.Symbol

	st.connsMu.RLock()
	conns := make([]*Connector, 0, len(st.conns))
	for c := range st.conns {
		conns = append(conns, c)
	}
	st.connsMu.RUnlock()

	type snap struct {
		bal map[BalanceKey]decimal.Decimal
		pos Position
	}
	before := make([]snap, len(conns))
	for i, c := range conns {
		before[i].bal, before[i].pos = c.snapshotAccountState(sym)
	}

	st.mu.Lock()
	events := st.applyDepthBookLocked(msg.Depth, true)
	st.mu.Unlock()

	for i, c := range conns {
		afterBal, afterPos := c.snapshotAccountState(sym)
		for _, em := range c.buildMakerMatchMessages(sym, events, before[i].bal, afterBal, before[i].pos, afterPos) {
			c.publishAccountMessage(em)
		}
	}
}

// dispatchTickerIngest maps ticker last to both Last and Mark in TickerStore (simplified paper model;
// mark-index streams still override Mark via dispatchMarkPricePayload).
func (st *exchangeState) dispatchTickerIngest(msg *ctypes.Message) {
	if msg == nil || msg.Ticker == nil || !msg.Ticker.Symbol.IsValid() {
		return
	}
	st.mu.Lock()
	st.ticker.Update(Ticker{
		Symbol: toSimSymbol(msg.Ticker.Symbol),
		Last:   msg.Ticker.LastPrice,
		Mark:   msg.Ticker.LastPrice,
		Ts:     msg.Ticker.Ts,
	})
	st.mu.Unlock()
}

// dispatchMarkPricePayload updates shared ticker mark once, then runs liquidation per Connector.
// tryLiquidateAfterMark does not hold exchangeState.mu; ordering vs PlaceOrder relies on SimExchange.mu.
func (st *exchangeState) dispatchMarkPricePayload(mp *ctypes.MarkPrice) {
	if mp == nil || !mp.Symbol.IsValid() || !mp.MarkPrice.GreaterThan(decimal.Zero) {
		return
	}
	st.mu.Lock()
	sym := toSimSymbol(mp.Symbol)
	prev, _ := st.ticker.Get(sym)
	last := prev.Last
	if !last.GreaterThan(decimal.Zero) {
		last = mp.MarkPrice
	}
	st.ticker.Update(Ticker{
		Symbol: sym,
		Last:   last,
		Mark:   mp.MarkPrice,
		Ts:     mp.Ts,
	})
	st.mu.Unlock()

	st.connsMu.RLock()
	conns := make([]*Connector, 0, len(st.conns))
	for c := range st.conns {
		conns = append(conns, c)
	}
	st.connsMu.RUnlock()
	for _, c := range conns {
		c.tryLiquidateAfterMark(mp.Symbol, mp.MarkPrice)
	}
}
