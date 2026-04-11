package sources

import (
	"context"
	"fmt"
	"testing"
	"time"

	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

type deterministicCursorSource struct {
	id    string
	start time.Time
	step  time.Duration
	total int64
}

func (s *deterministicCursorSource) ID() string { return s.id }

func (s *deterministicCursorSource) Spec() stypes.SignalSpec {
	return nil
}

func (s *deterministicCursorSource) Fetch(ctx context.Context, cursor stypes.Cursor, limit int) ([]*stypes.Message, stypes.Cursor, error) {
	_ = ctx
	if limit <= 0 {
		return nil, cursor, nil
	}
	out := make([]*stypes.Message, 0, limit)
	base := uint64(cursor.ID)
	for i := 0; i < limit; i++ {
		if cursor.ID >= s.total {
			break
		}
		cursor.ID++
		ts := s.start.Add(time.Duration(cursor.ID) * s.step)
		cursor.Ts = ts
		out = append(out, stypes.NewMessageWithSource(
			stypes.SignalSourceDatasource,
			s.id,
			base+uint64(i)+1,
			&stypes.TestSignal{BaseSignal: stypes.BaseSignal{ID: fmt.Sprintf("%s-%d", s.id, cursor.ID), Ts: ts}},
			false,
		))
	}
	return out, cursor, nil
}

func TestAdaptCursorSources_WireThroughMerger(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2025, 12, 18, 0, 0, 0, 0, time.UTC)

	srcA := &deterministicCursorSource{id: "a", start: base, step: time.Second, total: 3}

	fetcherA := NewCursorFetchSource(srcA, stypes.Cursor{}, 2, 0, 8, false)

	var got int
	for {
		_, ok, err := fetcherA.Next(ctx)
		if err != nil {
			t.Fatalf("next frame err: %v", err)
		}
		if !ok {
			break
		}
		got++
	}
	expected := int(srcA.total)
	if got != expected {
		t.Fatalf("expected %d events total, got %d", expected, got)
	}
}
