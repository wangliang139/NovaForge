package timeline

import (
	"context"
	"sort"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/strategy/infra/timeline/internal"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

// TimelineScheduler 将 external frame 推进与 internal 事件仲裁合并到同一输出序列。
//
// 输出两种模式：
// - Next: 逐条事件流
// - NextFrame: 按时间点分帧（外部 + 内部 before/after）
//
// 注意：scheduler 本身单线程输出；InternalQueue 允许多线程 Emit。
type TimelineScheduler struct {
	external ExternalMerger
	internal *internal.InternalQueue
	sortCfg  SorterConfig

	globalSeq uint64

	// 用于 Next(event stream) 的展开缓存
	curPoint *Frame
	curIdx   int
}

type SchedulerConfig struct {
	External ExternalMerger
	Internal *internal.InternalQueue
	Sorter   SorterConfig
}

func NewTimelineScheduler(cfg SchedulerConfig) *TimelineScheduler {
	sortCfg := cfg.Sorter
	if sortCfg.SignalTypePriority == nil && sortCfg.ScopePriority == nil {
		sortCfg = DefaultSorterConfig()
	}
	internal := cfg.Internal
	if internal == nil {
		internal = NewInternalQueue()
	}
	return &TimelineScheduler{
		external: cfg.External,
		internal: internal,
		sortCfg:  sortCfg,
	}
}

// NextFrame 返回“下一时间点”的所有事件（external + internal）并做统一排序。
//
// 归并规则：
// - 若下一 internalTs < next external frameTs：返回该 internalTs 的所有 internal event。
// - 若下一 external frameTs < internalTs：返回该 external frameTs 的所有 external event。
// - 若两者相等：返回该 Ts 下的 external + internal 合并集合。
func (s *TimelineScheduler) NextFrame(ctx context.Context) (*Frame, bool, error) {
	if s == nil {
		return nil, false, nil
	}

	// 取 external 的下一帧（peek，不消费）
	var extTs time.Time
	extOK := false
	if s.external != nil {
		f, ok, err := s.external.PeekFrame(ctx)
		if err != nil {
			return nil, false, err
		}
		if ok && f != nil {
			extTs = f.Ts
			extOK = true
		}
	}

	// 取 internal 的下一条（peek，不消费）
	if s.internal == nil {
		// 兼容：允许 scheduler 在未设置 internal queue 时仅输出 external。
		s.internal = NewInternalQueue()
	}
	intHead := s.internal.Peek()
	intOK := intHead != nil

	if !extOK && !intOK {
		return nil, false, nil
	}

	// 决定下一时间点 ts
	var ts time.Time
	switch {
	case extOK && !intOK:
		ts = extTs
	case !extOK && intOK:
		ts = intHead.Ts
	default:
		// both ok
		if intHead.Ts.Before(extTs) {
			ts = intHead.Ts
		} else {
			// tie or external earlier
			ts = extTs
		}
	}

	events := make([]*types.Message, 0, 16)

	// 消费 external frame（若本次 ts 选择了 external）
	for {
		head, ok, err := s.external.PeekFrame(ctx)
		if err != nil {
			return nil, false, err
		}
		if !ok || head == nil || !head.Ts.Equal(ts) {
			break
		}
		f, ok, err := s.external.NextFrame(ctx)
		if err != nil {
			return nil, false, err
		}
		if ok && f != nil {
			events = append(events, f.Messages...)
		}
	}

	// drain internal（若本次 ts 选择了 internal）
	for {
		head := s.internal.Peek()
		if head == nil || !head.Ts.Equal(ts) {
			break
		}
		events = append(events, s.internal.Pop())
	}

	// 统一排序（同 Ts 内按 sorter 的 total order）
	sort.SliceStable(events, func(i, j int) bool {
		return s.sortCfg.Compare(events[i], events[j]) < 0
	})

	// 统一赋 global seq（按最终输出顺序）
	for _, ev := range events {
		s.assignGlobalSeq(ev)
	}

	return &Frame{Ts: ts, Messages: events}, true, nil
}

// Next 逐条输出（按 NextPointFrame 的顺序展开）。
func (s *TimelineScheduler) Next(ctx context.Context) (*types.Message, bool, error) {
	if s == nil {
		return nil, false, nil
	}

	for {
		if s.curPoint == nil {
			pf, ok, err := s.NextFrame(ctx)
			if err != nil || !ok {
				return nil, ok, err
			}
			s.curPoint = pf
			s.curIdx = 0
		}

		curSlice := s.curPoint.Messages
		if s.curIdx < len(curSlice) {
			ev := curSlice[s.curIdx]
			s.curIdx++
			return ev, true, nil
		}

		s.curPoint = nil
		s.curIdx = 0
	}
}

func (s *TimelineScheduler) assignGlobalSeq(ev *types.Message) {
	if ev == nil {
		return
	}
	s.globalSeq++
	ev.GlobalSeq = s.globalSeq
}

func (s *TimelineScheduler) assignGlobalSeqPointFrame(pf *Frame) {
	if pf == nil {
		return
	}
	for _, ev := range pf.Messages {
		if ev == nil {
			continue
		}
		s.assignGlobalSeq(ev)
	}
}

// GetInternalQueue 获取内部事件队列（用于 TimelineEventBus）
func (s *TimelineScheduler) GetInternalQueue() *internal.InternalQueue {
	if s == nil {
		return nil
	}
	return s.internal
}
