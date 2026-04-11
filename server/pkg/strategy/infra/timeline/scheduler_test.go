package timeline

import (
	"context"
	"testing"
	"time"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/sources"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

func TestTimelineScheduler_OrderAndFlushBarrier(t *testing.T) {
	ctx := context.Background()
	ts0 := time.Date(2025, 12, 18, 0, 0, 0, 0, time.UTC)
	ts1 := ts0.Add(time.Second)

	ext1 := sources.NewStaticExternalSource("ext", []*types.Message{
		stypes.NewMessageWithSource(types.SignalSourceDatasource, "ext", 1, &stypes.KlineSignal{
			BaseSignal: stypes.BaseSignal{Ts: ts1},
			IsClosed:   true,
		}, false),
	})
	merger := NewExternalMerger([]types.Source{ext1}, ExternalMergerConfig{Sort: DefaultSorterConfig()})

	q := NewInternalQueue()
	// internal before frame（Ts < frameTs）
	if err := q.Emit(ctx, stypes.NewMessageWithSource(types.SignalSourceInternal, "int", 1, &stypes.OrderLifecycleSignal{
		Status:     ctypes.OrderStatusNew,
		BaseSignal: stypes.BaseSignal{Ts: ts0},
	}, false)); err != nil {
		t.Fatalf("emit: %v", err)
	}

	// 注入 internal after（Ts == frameTs）
	q.Emit(ctx, stypes.NewMessageWithSource(types.SignalSourceInternal, "int", 2, &stypes.OrderLifecycleSignal{
		Status:     ctypes.OrderStatusNew,
		BaseSignal: stypes.BaseSignal{Ts: ts1},
	}, false))

	s := NewTimelineScheduler(SchedulerConfig{
		External: merger,
		Internal: q,
		Sorter:   DefaultSorterConfig(),
	})

	// event stream 模式：internal before -> external -> internal after
	ev1, ok, err := s.Next(ctx)
	if err != nil || !ok {
		t.Fatalf("expected ev1, err=%v ok=%v", err, ok)
	}
	if !ev1.Ts.Equal(ts0) || ev1.Source != types.SignalSourceInternal {
		t.Fatalf("unexpected ev1: %+v", ev1)
	}
	if ev1.GlobalSeq != 1 {
		t.Fatalf("expected global seq 1, got %d", ev1.GlobalSeq)
	}

	ev2, ok, err := s.Next(ctx)
	if err != nil || !ok {
		t.Fatalf("expected ev2, err=%v ok=%v", err, ok)
	}
	if !ev2.Ts.Equal(ts1) || ev2.Source != types.SignalSourceDatasource {
		t.Fatalf("unexpected ev2: %+v", ev2)
	}
	if ev2.GlobalSeq != 2 {
		t.Fatalf("expected global seq 2, got %d", ev2.GlobalSeq)
	}

	ev3, ok, err := s.Next(ctx)
	if err != nil || !ok {
		t.Fatalf("expected ev3, err=%v ok=%v", err, ok)
	}
	if !ev3.Ts.Equal(ts1) || ev3.Source != types.SignalSourceInternal {
		t.Fatalf("unexpected ev3: %+v", ev3)
	}
	if ev3.GlobalSeq != 3 {
		t.Fatalf("expected global seq 3, got %d", ev3.GlobalSeq)
	}

	_, ok, err = s.Next(ctx)
	if err != nil || ok {
		t.Fatalf("expected end, err=%v ok=%v", err, ok)
	}
}

func TestTimelineScheduler_NextMergedFrame_MergeAndSort(t *testing.T) {
	ctx := context.Background()
	ts0 := time.Date(2025, 12, 18, 0, 0, 0, 0, time.UTC)
	ts1 := ts0.Add(time.Second)

	ext := sources.NewStaticExternalSource("ext", []*types.Message{
		types.NewMessageWithSource(types.SignalSourceDatasource, "ext", 1, &stypes.KlineSignal{
			BaseSignal: stypes.BaseSignal{Ts: ts1},
			IsClosed:   true,
		}, false),
	})
	merger := NewExternalMerger([]types.Source{ext}, ExternalMergerConfig{Sort: DefaultSorterConfig()})

	q := NewInternalQueue()
	if err := q.Emit(ctx, types.NewMessageWithSource(types.SignalSourceInternal, "int", 1, &stypes.OrderLifecycleSignal{
		BaseSignal: stypes.BaseSignal{Ts: ts0},
	}, false)); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if err := q.Emit(ctx, types.NewMessageWithSource(types.SignalSourceInternal, "int", 2, &stypes.OrderLifecycleSignal{
		BaseSignal: stypes.BaseSignal{Ts: ts1},
		Status:     ctypes.OrderStatusNew,
	}, false)); err != nil {
		t.Fatalf("emit: %v", err)
	}

	s := NewTimelineScheduler(SchedulerConfig{
		External: merger,
		Internal: q,
		Sorter:   DefaultSorterConfig(),
	})

	mf1, ok, err := s.NextFrame(ctx)
	if err != nil || !ok {
		t.Fatalf("expected mf1, err=%v ok=%v", err, ok)
	}
	if !mf1.Ts.Equal(ts0) || len(mf1.Messages) != 1 || mf1.Messages[0].Source != types.SignalSourceInternal {
		t.Fatalf("unexpected mf1: %+v", mf1)
	}
	if mf1.Messages[0].GlobalSeq != 1 {
		t.Fatalf("expected global seq 1, got %d", mf1.Messages[0].GlobalSeq)
	}

	mf2, ok, err := s.NextFrame(ctx)
	if err != nil || !ok {
		t.Fatalf("expected mf2, err=%v ok=%v", err, ok)
	}
	if !mf2.Ts.Equal(ts1) || len(mf2.Messages) != 2 {
		t.Fatalf("unexpected mf2: %+v", mf2)
	}
	// 同 Ts 统一排序：kline(10) 应在 order(70) 之前
	if mf2.Messages[0].Source != types.SignalSourceDatasource || mf2.Messages[1].Source != types.SignalSourceInternal {
		t.Fatalf("unexpected order: %s then %s", mf2.Messages[0].Type(), mf2.Messages[1].Type())
	}
	if mf2.Messages[0].GlobalSeq != 2 || mf2.Messages[1].GlobalSeq != 3 {
		t.Fatalf("unexpected global seqs: %d %d", mf2.Messages[0].GlobalSeq, mf2.Messages[1].GlobalSeq)
	}

	_, ok, err = s.NextFrame(ctx)
	if err != nil || ok {
		t.Fatalf("expected end, err=%v ok=%v", err, ok)
	}
}
