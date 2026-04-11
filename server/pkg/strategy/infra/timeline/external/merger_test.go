package external

import (
	"context"
	"testing"
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/timeline/sorter"
	timeline "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/timeline/types"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/sources"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

func TestHeapExternalMerger_FramesAndOrder(t *testing.T) {
	ctx := context.Background()
	ts1 := time.Date(2025, 12, 18, 0, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(time.Second)

	// 两个 source 在同 Ts 产出不同 Type，frame 内应按 TotalOrder 排序（kline < trade）
	s1 := sources.NewStaticExternalSource("s1", []*types.Message{
		types.NewMessageWithSource(types.SignalSourceDatasource, "s1", 1, &types.KlineSignal{
			BaseSignal: types.BaseSignal{Ts: ts1},
			IsClosed:   false,
		}, false),
		types.NewMessageWithSource(types.SignalSourceDatasource, "s1", 2, &types.KlineSignal{
			BaseSignal: types.BaseSignal{Ts: ts2},
			IsClosed:   true,
		}, false),
	})
	s2 := sources.NewStaticExternalSource("s2", []*types.Message{
		types.NewMessageWithSource(types.SignalSourceDatasource, "s2", 1, &types.TradeSignal{
			BaseSignal: types.BaseSignal{Ts: ts1},
		}, false),
	})

	m := NewExternalMerger([]types.Source{s1, s2}, ExternalMergerConfig{
		Sort:   sorter.DefaultSorterConfig(),
		Policy: timeline.ErrorPolicyFailFast,
	})

	f1, ok, err := m.NextFrame(ctx)
	if err != nil || !ok {
		t.Fatalf("expected frame1 ok, err=%v ok=%v", err, ok)
	}
	if !f1.Ts.Equal(ts1) {
		t.Fatalf("expected ts1, got %v", f1.Ts)
	}
	if len(f1.Messages) != 2 {
		t.Fatalf("expected 2 events, got %d", len(f1.Messages))
	}
	if f1.Messages[0].Source != types.SignalSourceDatasource || f1.Messages[1].Source != types.SignalSourceDatasource {
		t.Fatalf("unexpected frame1 order: %s then %s", f1.Messages[0].Type(), f1.Messages[1].Type())
	}

	f2, ok, err := m.NextFrame(ctx)
	if err != nil || !ok {
		t.Fatalf("expected frame2 ok, err=%v ok=%v", err, ok)
	}
	if !f2.Ts.Equal(ts2) || len(f2.Messages) != 1 {
		t.Fatalf("unexpected frame2: ts=%v len=%d", f2.Ts, len(f2.Messages))
	}
	if f2.Messages[0].SourceID != "s1" {
		t.Fatalf("expected event from s1, got %s", f2.Messages[0].SourceID)
	}

	_, ok, err = m.NextFrame(ctx)
	if err != nil || ok {
		t.Fatalf("expected end, err=%v ok=%v", err, ok)
	}
}
