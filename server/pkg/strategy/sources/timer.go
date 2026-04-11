package sources

import (
	"context"
	"errors"
	"time"

	ss "github.com/wangliang139/NovaForge/server/pkg/strategy/signal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	types "github.com/wangliang139/NovaForge/server/pkg/types"
)

// TimerSource 生成模拟时间轴上的 timer 事件（不使用系统时间）。
// 说明：该 source 用于回测/对齐等场景，输出 Type=SignalTypeTimer。
type TimerSource struct {
	spec      ss.TimerSignalSpec
	isDerived bool

	nextTs    time.Time
	nextSeq   uint64
	exhausted bool
	closed    bool
}

var _ stypes.Source = (*TimerSource)(nil)

func NewTimerSource(spec ss.TimerSignalSpec, isDerived bool) *TimerSource {
	return &TimerSource{
		spec:      spec,
		isDerived: isDerived,
		nextTs:    spec.GetStartTs(),
		nextSeq:   1,
		exhausted: false,
	}
}

func (s *TimerSource) ID() string {
	return s.spec.GetID()
}

func (s *TimerSource) Spec() stypes.SignalSpec {
	return &s.spec
}

func (s *TimerSource) IsDerived() bool {
	return s.isDerived
}

func (s *TimerSource) Datasource() *types.DataSource {
	return nil
}

// Fetch 以 cursor + batch 的方式生成 timer 事件，供 timeline.CursorFetchAdapter 使用。
func (s *TimerSource) Fetch(ctx context.Context, cursor stypes.Cursor, limit int) ([]*stypes.Message, stypes.Cursor, error) {
	_ = ctx
	if s == nil {
		return nil, cursor, nil
	}
	if limit <= 0 {
		return nil, cursor, nil
	}
	if s.spec.Interval <= 0 {
		return nil, cursor, errors.New("invalid interval")
	}

	// 约定：cursor.ID 表示已消费的条数（从 0 开始），下一条事件时间 = startTs + cursor.ID*interval
	startID := cursor.ID
	if startID < 0 {
		startID = 0
	}

	out := make([]*stypes.Message, 0, limit)
	nextTs := s.spec.GetStartTs().Add(time.Duration(startID) * s.spec.Interval)
	base := uint64(startID)
	for i := 0; i < limit; i++ {
		if nextTs.After(s.spec.GetEndTs()) {
			break
		}
		out = append(out, stypes.NewMessageWithSource(
			stypes.SignalSourceTimer,
			s.ID(),
			base+uint64(i)+1,
			&stypes.TimerSignal{
				BaseSignal: stypes.BaseSignal{
					Exchange: s.spec.GetExchange(),
					Symbol:   s.spec.GetSymbol(),
					Topic:    &s.spec.Topic,
				},
				Time: nextTs,
			},
			s.isDerived,
		))
		startID++
		nextTs = nextTs.Add(s.spec.Interval)
	}

	if len(out) > 0 {
		cursor.ID = startID
		cursor.Ts = out[len(out)-1].Ts
	}
	return out, cursor, nil
}

func (s *TimerSource) Peek(ctx context.Context) (*stypes.Message, bool, error) {
	_ = ctx
	if s == nil || s.closed {
		return nil, false, context.Canceled
	}
	if s.exhausted {
		return nil, false, nil
	}
	if s.spec.Interval <= 0 {
		return nil, false, &stypes.SourceError{SourceID: s.ID(), Op: "peek", Err: errors.New("invalid interval")}
	}
	if s.nextTs.After(s.spec.GetEndTs()) {
		s.exhausted = true
		return nil, false, nil
	}
	return stypes.NewMessageWithSource(
		stypes.SignalSourceTimer,
		s.ID(),
		s.nextSeq,
		&stypes.TimerSignal{
			BaseSignal: stypes.BaseSignal{
				Exchange: s.spec.GetExchange(),
				Symbol:   s.spec.GetSymbol(),
				Topic:    &s.spec.Topic,
				ID:       s.ID(),
				Ts:       s.nextTs,
			},
			Time: s.nextTs,
		},
		s.isDerived,
	), true, nil
}

func (s *TimerSource) Next(ctx context.Context) (*stypes.Message, bool, error) {
	ev, ok, err := s.Peek(ctx)
	if err != nil || !ok {
		return ev, ok, err
	}
	s.nextTs = s.nextTs.Add(s.spec.Interval)
	s.nextSeq++
	return ev, true, nil
}

func (s *TimerSource) Watermark(ctx context.Context) (ts time.Time, ok bool, err error) {
	_ = ctx
	return time.Time{}, false, nil
}

func (s *TimerSource) Close() error {
	if s == nil {
		return nil
	}
	s.closed = true
	return nil
}
