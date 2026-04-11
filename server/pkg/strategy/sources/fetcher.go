package sources

import (
	"context"
	"sync"
	"time"

	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	types "github.com/wangliang139/llt-trade/server/pkg/types"
)

// CursorFetchSource 将现有的 types.Source（cursor + batch Fetch）适配为 ExternalSource。
//
// 设计目标：
// - 对外提供 Peek/Next head 语义
// - 内部异步预取 + 有界 buffer
// - 保持确定性：同输入必然生成同 (SourceSeq)（依赖底层 Fetch 返回顺序稳定）
type CursorFetchSource struct {
	fetcher   stypes.Fetcher
	isDerived bool

	cursor       stypes.Cursor
	batchSize    int
	lowWatermark int
	maxBuffer    int

	mu        sync.Mutex
	cond      *sync.Cond
	fetching  bool
	exhausted bool
	closed    bool
	failed    error
	buffer    []*stypes.Message
}

// var _ types.Source = (*CursorFetchSource)(nil)

func NewCursorFetchSource(fetcher stypes.Fetcher, cursor stypes.Cursor, batchSize int, lowWatermark int, maxBuffer int, isDerived bool) *CursorFetchSource {
	if batchSize <= 0 {
		batchSize = 256
	}
	if lowWatermark < 0 {
		lowWatermark = 0
	}
	if maxBuffer <= 0 {
		maxBuffer = batchSize * 4
	}
	a := &CursorFetchSource{
		fetcher:      fetcher,
		isDerived:    isDerived,
		cursor:       cursor,
		batchSize:    batchSize,
		lowWatermark: lowWatermark,
		maxBuffer:    maxBuffer,
	}
	a.cond = sync.NewCond(&a.mu)
	return a
}

func (a *CursorFetchSource) ID() string {
	if a == nil || a.fetcher == nil {
		return ""
	}
	return a.fetcher.ID()
}

func (a *CursorFetchSource) Spec() stypes.SignalSpec {
	if a == nil || a.fetcher == nil {
		return nil
	}
	return a.fetcher.Spec()
}

func (a *CursorFetchSource) IsDerived() bool {
	return a.isDerived
}

func (a *CursorFetchSource) Datasource() *types.DataSource {
	return nil
}

// triggerFetch 必须在已持有 a.mu 的情况下调用。
func (a *CursorFetchSource) triggerFetch(ctx context.Context) {
	if a.closed || a.exhausted || a.fetching || a.failed != nil {
		return
	}
	if len(a.buffer) > a.lowWatermark {
		return
	}
	if len(a.buffer) >= a.maxBuffer {
		return
	}

	a.fetching = true
	startCursor := a.cursor
	limit := a.batchSize
	src := a.fetcher

	go func() {
		events, newCursor, err := src.Fetch(ctx, startCursor, limit)

		a.mu.Lock()
		defer func() {
			a.fetching = false
			a.cond.Broadcast()
			a.mu.Unlock()
		}()

		if err != nil {
			a.failed = err
			return
		}
		if len(events) == 0 {
			a.exhausted = true
			return
		}

		// 如果 Fetcher 返回的 EventEnvelope 还没有设置 SourceSeq，需要补充
		base := uint64(startCursor.ID)
		for i, ev := range events {
			if ev.SourceSeq == 0 {
				ev.SourceSeq = base + uint64(i) + 1
			}
			if ev.SourceID == "" {
				ev.SourceID = src.ID()
			}
			ev.IsDerived = a.isDerived
			// 确保 Source 字段已设置
			if ev.Source == "" {
				if ev.Type() == types.SignalTypeTimer {
					ev.Source = stypes.SignalSourceTimer
				} else {
					ev.Source = stypes.SignalSourceDatasource
				}
			}
		}

		a.buffer = append(a.buffer, events...)
		a.cursor = newCursor
	}()
}

func (a *CursorFetchSource) Peek(ctx context.Context) (*stypes.Message, bool, error) {
	if a == nil {
		return nil, false, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for {
		if a.closed {
			return nil, false, context.Canceled
		}
		if a.failed != nil {
			return nil, false, &stypes.SourceError{SourceID: a.ID(), Op: "peek", Err: a.failed}
		}
		if len(a.buffer) > 0 {
			return a.buffer[0], true, nil
		}
		if a.exhausted {
			return nil, false, nil
		}

		a.triggerFetch(ctx)
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		a.cond.Wait()
	}
}

func (a *CursorFetchSource) Next(ctx context.Context) (*stypes.Message, bool, error) {
	if a == nil {
		return nil, false, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for {
		if a.closed {
			return nil, false, context.Canceled
		}
		if a.failed != nil {
			return nil, false, &stypes.SourceError{SourceID: a.ID(), Op: "next", Err: a.failed}
		}
		if len(a.buffer) > 0 {
			ev := a.buffer[0]
			a.buffer = a.buffer[1:]
			if len(a.buffer) <= a.lowWatermark {
				a.triggerFetch(ctx)
			}
			return ev, true, nil
		}
		if a.exhausted {
			return nil, false, nil
		}

		a.triggerFetch(ctx)
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		a.cond.Wait()
	}
}

func (a *CursorFetchSource) Watermark(ctx context.Context) (ts time.Time, ok bool, err error) {
	_ = ctx
	return time.Time{}, false, nil
}

func (a *CursorFetchSource) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	a.closed = true
	a.cond.Broadcast()
	a.mu.Unlock()
	return nil
}
