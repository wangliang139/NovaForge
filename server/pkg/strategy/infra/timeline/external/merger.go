package external

import (
	"container/heap"
	"context"
	"sort"
	"sync"

	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/timeline/sorter"
	timeline "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/timeline/types"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

type sourceState struct {
	src       types.Source
	id        string
	head      *types.Message
	exhausted bool
	failed    error
}

type headItem struct {
	id string
	ev *types.Message
	s  *sourceState
}

type headHeap []*headItem

func (h headHeap) Len() int { return len(h) }

func (h headHeap) Less(i, j int) bool {
	ai := h[i]
	aj := h[j]

	if ai.ev == nil && aj.ev == nil {
		return ai.id < aj.id
	}
	if ai.ev == nil {
		return true
	}
	if aj.ev == nil {
		return false
	}

	if !ai.ev.Ts.Equal(aj.ev.Ts) {
		return ai.ev.Ts.Before(aj.ev.Ts)
	}
	// heap 只用于“最小 Ts”选择；同 Ts 采用 SourceID 做稳定 tie-break。
	return ai.id < aj.id
}

func (h headHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *headHeap) Push(x any) { *h = append(*h, x.(*headItem)) }

func (h *headHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// ExternalMerger 是 ExternalMerger 的第一版实现（回测优先）：
// - head 屏障：每个未 exhausted 的 source 必须具备 head 才能输出
// - heap 仅维护每个 source 的 head，进行 k-way merge
type ExternalMerger struct {
	sort   sorter.SorterConfig
	policy timeline.ErrorPolicy

	mu     sync.Mutex
	closed bool

	sources map[string]*sourceState
	heap    headHeap

	peekCache *timeline.Frame
	peekIDs   []string // 本次 peek frame 参与的 source id 列表（用于 NextFrame 消费）
}

var _ timeline.ExternalMerger = (*ExternalMerger)(nil)

type ExternalMergerConfig struct {
	Sort   sorter.SorterConfig
	Policy timeline.ErrorPolicy
}

func NewExternalMerger(srcs []types.Source, cfg ExternalMergerConfig) *ExternalMerger {
	sortCfg := cfg.Sort
	if sortCfg.SignalTypePriority == nil && sortCfg.ScopePriority == nil {
		sortCfg = sorter.DefaultSorterConfig()
	}
	policy := cfg.Policy
	if policy == "" {
		policy = timeline.ErrorPolicyFailFast
	}

	m := &ExternalMerger{
		sort:    sortCfg,
		policy:  policy,
		sources: make(map[string]*sourceState, len(srcs)),
		heap:    headHeap{},
	}
	for _, s := range srcs {
		if s == nil {
			continue
		}
		id := s.ID()
		m.sources[id] = &sourceState{src: s, id: id}
	}
	heap.Init(&m.heap)
	return m
}

func (m *ExternalMerger) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	var firstErr error
	for _, s := range m.sources {
		if s == nil || s.src == nil {
			continue
		}
		if err := s.src.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *ExternalMerger) handleSourceErr(s *sourceState, op string, err error) error {
	if err == nil {
		return nil
	}
	se := &types.SourceError{SourceID: s.id, Op: op, Err: err}
	if m.policy == timeline.ErrorPolicyDegrade {
		s.failed = se
		s.head = nil
		return nil
	}
	return se
}

// ensureHeads 会为每个 active source 填充 head，并维护 heap。
// 说明：这里的“active”指未 exhausted 且未 failed 的 source。
func (m *ExternalMerger) ensureHeads(ctx context.Context) error {
	// 先把 heap 重建，避免“某个 source head 变更但 heap 未更新”的隐患。
	m.heap = headHeap{}
	heap.Init(&m.heap)

	for _, s := range m.sources {
		if s == nil || s.src == nil {
			continue
		}
		if s.exhausted || s.failed != nil {
			continue
		}

		ev, ok, err := s.src.Peek(ctx)
		if err != nil {
			if herr := m.handleSourceErr(s, "peek", err); herr != nil {
				return herr
			}
			continue
		}
		if !ok {
			s.exhausted = true
			s.head = nil
			continue
		}
		s.head = ev
		heap.Push(&m.heap, &headItem{id: s.id, ev: ev, s: s})
	}

	return nil
}

func (m *ExternalMerger) PeekFrame(ctx context.Context) (*timeline.Frame, bool, error) {
	defer m.mu.Unlock()
	m.mu.Lock()

	if m.closed {
		return nil, false, context.Canceled
	}
	if m.peekCache != nil {
		return m.peekCache, true, nil
	}

	if err := m.ensureHeads(ctx); err != nil {
		return nil, false, err
	}
	if m.heap.Len() == 0 {
		return nil, false, nil
	}

	minTs := m.heap[0].ev.Ts

	messages := make([]*types.Message, 0, 8)
	ids := make([]string, 0, 4)
	for _, it := range m.heap {
		if it == nil || it.ev == nil {
			continue
		}
		if it.ev.Ts.Equal(minTs) {
			messages = append(messages, it.ev)
			ids = append(ids, it.id)
		}
	}

	sort.SliceStable(messages, func(i, j int) bool {
		return m.sort.Compare(messages[i], messages[j]) < 0
	})

	m.peekCache = &timeline.Frame{Ts: minTs, Messages: messages}
	m.peekIDs = ids
	return m.peekCache, true, nil
}

func (m *ExternalMerger) NextFrame(ctx context.Context) (*timeline.Frame, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, false, context.Canceled
	}

	// 直接在持锁状态下走 PeekFrame 的“缓存路径”；
	// 若缓存为空，这里需要先计算一次 frame。
	if m.peekCache == nil {
		// ensureHeads + cache 生成
		if err := m.ensureHeads(ctx); err != nil {
			return nil, false, err
		}
		if m.heap.Len() == 0 {
			return nil, false, nil
		}
		minTs := m.heap[0].ev.Ts

		messages := make([]*types.Message, 0, 8)
		ids := make([]string, 0, 4)
		for _, it := range m.heap {
			if it == nil || it.ev == nil {
				continue
			}
			if it.ev.Ts.Equal(minTs) {
				messages = append(messages, it.ev)
				ids = append(ids, it.id)
			}
		}
		sort.SliceStable(messages, func(i, j int) bool {
			return m.sort.Compare(messages[i], messages[j]) < 0
		})
		m.peekCache = &timeline.Frame{Ts: minTs, Messages: messages}
		m.peekIDs = ids
	}

	frame := m.peekCache
	if frame == nil {
		return nil, false, nil
	}

	// 消费本次 frame 参与的 sources（推进其 head）。
	for _, id := range m.peekIDs {
		s := m.sources[id]
		if s == nil || s.src == nil || s.exhausted || s.failed != nil {
			continue
		}
		_, ok2, err2 := s.src.Next(ctx)
		if err2 != nil {
			if herr := m.handleSourceErr(s, "next", err2); herr != nil {
				return nil, false, herr
			}
			continue
		}
		if !ok2 {
			s.exhausted = true
			s.head = nil
			continue
		}
		// 不依赖 Next 返回值作为“新 head”；统一由 Peek 重新读取，避免不同 source 实现差异。
		ev, ok3, err3 := s.src.Peek(ctx)
		if err3 != nil {
			if herr := m.handleSourceErr(s, "peek", err3); herr != nil {
				return nil, false, herr
			}
			continue
		}
		if !ok3 {
			s.exhausted = true
			s.head = nil
			continue
		}
		s.head = ev
	}

	// 清理 peek cache，下一次重新计算。
	m.peekCache = nil
	m.peekIDs = nil

	return frame, true, nil
}
