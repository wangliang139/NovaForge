package sources

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	types "github.com/wangliang139/llt-trade/server/pkg/types"
)

type TestSource struct {
	name  string
	limit int

	// ExternalSource support
	closed    bool
	exhausted bool
	failed    error
	cursor    stypes.Cursor
	buffer    []*stypes.Message

	batchSize int
}

var _ stypes.Source = (*TestSource)(nil)

func NewTestSource(name string, limit int) *TestSource {
	return &TestSource{
		name:  name,
		limit: limit,
		// 默认预取参数
		batchSize: 256,
	}
}

func (s *TestSource) ID() string {
	return fmt.Sprintf("test-%s", s.name)
}

func (s *TestSource) Spec() stypes.SignalSpec {
	return nil
}

func (s *TestSource) IsDerived() bool {
	return false
}

func (s *TestSource) Datasource() *types.DataSource {
	return nil
}

func (s *TestSource) Fetch(ctx context.Context, cursor stypes.Cursor, limit int) ([]*stypes.Message, stypes.Cursor, error) {
	events := make([]*stypes.Message, 0, limit)
	base := uint64(cursor.ID)
	exchange := ctypes.ExchangeBinance
	symbol := ctypes.Symbol{
		Base:  "BTC",
		Quote: "USDT",
		Type:  ctypes.MarketTypeSpot,
	}
	for i := 0; i < limit; i++ {
		if cursor.ID >= int64(s.limit) {
			break
		}
		cursor.ID++
		ts := time.Now()
		cursor.Ts = ts
		events = append(events, stypes.NewMessageWithSource(
			stypes.SignalSourceDatasource,
			s.ID(),
			base+uint64(i)+1,
			&stypes.KlineSignal{
				BaseSignal: stypes.BaseSignal{
					ID:       s.ID(),
					Exchange: &exchange,
					Symbol:   &symbol,
					Ts:       ts,
				},
				Interval: ctypes.Interval("1m"),
				Open:     decimal.NewFromInt(10000),
				High:     decimal.NewFromInt(10000),
				Low:      decimal.NewFromInt(10000),
				Close:    decimal.NewFromInt(10000),
				Volume:   decimal.NewFromInt(10000),
				IsClosed: true,
			},
			false,
		))
		time.Sleep(10 * time.Millisecond)
	}
	return events, cursor, nil
}

func (s *TestSource) ensureBuffer(ctx context.Context) error {
	if s.closed || s.exhausted || s.failed != nil {
		return s.failed
	}
	if len(s.buffer) > 0 {
		return nil
	}

	limit := s.batchSize
	if limit <= 0 {
		limit = 256
	}
	startCursor := s.cursor

	events, newCursor, err := s.Fetch(ctx, startCursor, limit)
	if err != nil {
		s.failed = err
		return err
	}
	if len(events) == 0 {
		s.exhausted = true
		return nil
	}

	s.buffer = append(s.buffer, events...)
	s.cursor = newCursor
	return nil
}

func (s *TestSource) Peek(ctx context.Context) (*stypes.Message, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	if s.closed {
		return nil, false, context.Canceled
	}
	if err := s.ensureBuffer(ctx); err != nil {
		return nil, false, &stypes.SourceError{SourceID: s.ID(), Op: "peek", Err: err}
	}
	if len(s.buffer) == 0 {
		return nil, false, nil
	}
	return s.buffer[0], true, nil
}

func (s *TestSource) Next(ctx context.Context) (*stypes.Message, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	if s.closed {
		return nil, false, context.Canceled
	}
	if err := s.ensureBuffer(ctx); err != nil {
		return nil, false, &stypes.SourceError{SourceID: s.ID(), Op: "next", Err: err}
	}
	if len(s.buffer) == 0 {
		return nil, false, nil
	}
	ev := s.buffer[0]
	s.buffer = s.buffer[1:]
	return ev, true, nil
}

func (s *TestSource) Watermark(ctx context.Context) (ts time.Time, ok bool, err error) {
	_ = ctx
	return time.Time{}, false, nil
}

func (s *TestSource) Close() error {
	if s == nil {
		return nil
	}
	s.closed = true
	return nil
}

type StaticExternalSource struct {
	id     string
	events []*stypes.Message
	idx    int
	closed bool
}

var _ stypes.Source = (*StaticExternalSource)(nil)

func NewStaticExternalSource(id string, events []*stypes.Message) *StaticExternalSource {
	return &StaticExternalSource{
		id:     id,
		events: events,
		idx:    0,
		closed: false,
	}
}

func (s *StaticExternalSource) ID() string { return s.id }

func (s *StaticExternalSource) Spec() stypes.SignalSpec {
	return nil
}

func (s *StaticExternalSource) IsDerived() bool {
	return false
}

func (s *StaticExternalSource) Datasource() *types.DataSource {
	return nil
}

func (s *StaticExternalSource) Peek(ctx context.Context) (*stypes.Message, bool, error) {
	_ = ctx
	if s.closed {
		return nil, false, context.Canceled
	}
	if s.idx >= len(s.events) {
		return nil, false, nil
	}
	return s.events[s.idx], true, nil
}

func (s *StaticExternalSource) Next(ctx context.Context) (*stypes.Message, bool, error) {
	ev, ok, err := s.Peek(ctx)
	if err != nil || !ok {
		return ev, ok, err
	}
	s.idx++
	return ev, true, nil
}

func (s *StaticExternalSource) Watermark(ctx context.Context) (time.Time, bool, error) {
	_ = ctx
	return time.Time{}, false, nil
}

func (s *StaticExternalSource) Close() error { s.closed = true; return nil }
