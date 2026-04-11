package internal

import (
	"container/heap"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

type internalItem struct {
	ev *types.Message
}

// InternalQueue 是线程安全的内部事件堆。
// - Emit 可从任意 goroutine 调用
// - Peek/Pop 由 scheduler 单线程消费
//
// 约束：
// - ev.Source 必须为 Internal
// - ev.SourceSeq 用于同 Ts 的稳定顺序；若为 0，会在队列内自动分配（确定性：同执行路径必然一致）
type InternalQueue struct {
	mu   sync.Mutex
	heap internalHeap

	seq atomic.Uint64
}

func NewInternalQueue() *InternalQueue {
	q := &InternalQueue{heap: internalHeap{}}
	heap.Init(&q.heap)
	return q
}

// var _ types.SignalEmitter = (*InternalQueue)(nil)

func (q *InternalQueue) Emit(ctx context.Context, ev *types.Message) error {
	if q == nil {
		return types.ErrInvalidInternalEvent
	}
	if ev == nil {
		return types.ErrInvalidInternalEvent
	}
	if ev.Source != types.SignalSourceInternal {
		return types.ErrInvalidInternalEvent
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if ev.SourceSeq == 0 {
		ev.SourceSeq = q.seq.Add(1)
	} else {
		// 保持 seq 单调不降，避免后续自动分配出现“回退”。
		for {
			cur := q.seq.Load()
			if ev.SourceSeq <= cur {
				break
			}
			if q.seq.CompareAndSwap(cur, ev.SourceSeq) {
				break
			}
		}
	}

	heap.Push(&q.heap, &internalItem{ev: ev})
	return nil
}

func (q *InternalQueue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.heap.Len()
}

func (q *InternalQueue) Peek() *types.Message {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.heap.Len() == 0 {
		return nil
	}
	it := q.heap[0]
	if it == nil {
		return nil
	}
	return it.ev
}

func (q *InternalQueue) Pop() *types.Message {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.heap.Len() == 0 {
		return nil
	}
	it := heap.Pop(&q.heap).(*internalItem)
	if it == nil {
		return nil
	}
	return it.ev
}

func (q *InternalQueue) PeekTs() (ts time.Time, ok bool) {
	ev := q.Peek()
	if ev == nil {
		return time.Time{}, false
	}
	return ev.Ts, true
}
