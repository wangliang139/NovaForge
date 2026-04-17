package simulate

import (
	"context"
	"sort"
	"time"
)

type EventKind int

const (
	EventDepthSnapshot EventKind = iota + 1
	EventDepthDelta
	EventTicker
	EventOrderSubmit
	EventOrderCancel
)

type SimEvent struct {
	Kind EventKind
	At   time.Time
	Seq  int64

	Depth  *OrderBook
	Ticker *Ticker
	Order  *PlaceOrderRequest
	Cancel *CancelRequest
}

type CancelRequest struct {
	AccountID string
	Symbol    Symbol
	OrderID   string
}

// EventPump serializes replay-time events for deterministic simulation.
type EventPump struct {
	clock *ReplayClock
	ex    *SimExchange

	queue []SimEvent
	seq   int64

	orderLatency  time.Duration
	cancelLatency time.Duration
	tickers       *TickerStore
}

func NewEventPump(clock *ReplayClock, ex *SimExchange) *EventPump {
	return &EventPump{
		clock:   clock,
		ex:      ex,
		tickers: NewTickerStore(),
	}
}

func (p *EventPump) SetOrderLatency(latency time.Duration) {
	if latency < 0 {
		latency = 0
	}
	p.orderLatency = latency
}

func (p *EventPump) SetCancelLatency(latency time.Duration) {
	if latency < 0 {
		latency = 0
	}
	p.cancelLatency = latency
}

func (p *EventPump) SubmitDepthSnapshot(ob OrderBook) {
	cp := ob
	p.enqueue(SimEvent{Kind: EventDepthSnapshot, At: ob.Ts.UTC(), Depth: &cp})
}

func (p *EventPump) SubmitDepthDelta(ob OrderBook) {
	cp := ob
	p.enqueue(SimEvent{Kind: EventDepthDelta, At: ob.Ts.UTC(), Depth: &cp})
}

func (p *EventPump) SubmitTicker(t Ticker) {
	cp := t
	p.enqueue(SimEvent{Kind: EventTicker, At: t.Ts.UTC(), Ticker: &cp})
}

func (p *EventPump) SubmitOrder(req PlaceOrderRequest) {
	at := p.clock.Now().Add(p.orderLatency)
	cp := req
	p.enqueue(SimEvent{Kind: EventOrderSubmit, At: at, Order: &cp})
}

func (p *EventPump) SubmitCancel(accountID string, sym Symbol, orderID string) {
	at := p.clock.Now().Add(p.cancelLatency)
	p.enqueue(SimEvent{
		Kind: EventOrderCancel,
		At:   at,
		Cancel: &CancelRequest{
			AccountID: accountID,
			Symbol:    sym,
			OrderID:   orderID,
		},
	})
}

func (p *EventPump) RunUntil(ctx context.Context, until time.Time) error {
	until = until.UTC()
	for len(p.queue) > 0 {
		ev := p.queue[0]
		if ev.At.After(until) {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		p.queue = p.queue[1:]
		p.clock.AdvanceTo(ev.At)
		if err := p.apply(ctx, ev); err != nil {
			return err
		}
	}
	p.clock.AdvanceTo(until)
	return nil
}

func (p *EventPump) RunAll(ctx context.Context) error {
	for len(p.queue) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		ev := p.queue[0]
		p.queue = p.queue[1:]
		p.clock.AdvanceTo(ev.At)
		if err := p.apply(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

func (p *EventPump) LatestTicker(sym Symbol) (Ticker, bool) {
	return p.tickers.Get(sym)
}

func (p *EventPump) enqueue(ev SimEvent) {
	p.seq++
	ev.Seq = p.seq
	if ev.At.IsZero() {
		ev.At = p.clock.Now()
	}
	p.queue = append(p.queue, ev)
	sort.SliceStable(p.queue, func(i, j int) bool {
		if !p.queue[i].At.Equal(p.queue[j].At) {
			return p.queue[i].At.Before(p.queue[j].At)
		}
		return p.queue[i].Seq < p.queue[j].Seq
	})
}

func (p *EventPump) apply(ctx context.Context, ev SimEvent) error {
	switch ev.Kind {
	case EventDepthSnapshot:
		depth := p.ex.getDepth(ev.Depth.Symbol)
		if depth == nil {
			return ErrUnknownSymbol
		}
		if err := depth.ApplySnapshot(ev.Depth); err != nil {
			return err
		}
		_, err := p.ex.OnDepthUpdated(ev.Depth.Symbol)
		return err
	case EventDepthDelta:
		depth := p.ex.getDepth(ev.Depth.Symbol)
		if depth == nil {
			return ErrUnknownSymbol
		}
		if err := depth.ApplyDelta(ev.Depth); err != nil {
			return err
		}
		_, err := p.ex.OnDepthUpdated(ev.Depth.Symbol)
		return err
	case EventTicker:
		p.tickers.Update(*ev.Ticker)
		return nil
	case EventOrderSubmit:
		_, err := p.ex.PlaceOrderAt(ctx, *ev.Order, ev.At)
		return err
	case EventOrderCancel:
		return p.ex.CancelOrderAt(ctx, ev.Cancel.AccountID, ev.Cancel.Symbol, ev.Cancel.OrderID, ev.At)
	default:
		return nil
	}
}
