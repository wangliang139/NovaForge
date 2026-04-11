package stream

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/service/streamsvc"
	"github.com/wangliang139/llt-trade/server/pkg/types"
)

const (
	watcherDropThreshold = 500             // 累计丢帧超过此值则驱逐
	watcherLogInterval   = 5 * time.Second // warn 日志最小间隔
)

type FlowSpec struct {
	Stream   types.StreamType
	Account  *string
	Exchange *types.Exchange
	Symbol   *string
	Interval *types.Interval
}

func (s FlowSpec) key() string {
	stream := strings.ReplaceAll(s.Stream.String(), "@", "")
	account := ""
	if s.Account != nil {
		account = strings.ReplaceAll(*s.Account, "@", "")
	}
	exchange := ""
	if s.Exchange != nil {
		exchange = strings.ReplaceAll(s.Exchange.String(), "@", "")
	}
	symbol := ""
	if s.Symbol != nil {
		symbol = strings.ReplaceAll(*s.Symbol, "@", "")
	}
	interval := ""
	if s.Interval != nil {
		interval = strings.ReplaceAll(s.Interval.String(), "@", "")
	}
	return fmt.Sprintf("%s@%s@%s@%s@%s", stream, account, exchange, symbol, interval)
}

type watcherEntry struct {
	connID        string
	ch            chan *types.SubscribeStreamResponse
	droppedFrames atomic.Int64 // 累计丢帧数
	lastWarnAt    atomic.Int64 // 上次打印 warn 的 UnixNano
}

type Manager struct {
	mu sync.Mutex

	streamSvc *streamsvc.Service

	flows    map[string]*flowEntry
	connSubs map[string]map[string]int64
	nextID   atomic.Uint64
}

type flowEntry struct {
	spec     FlowSpec
	refs     int64
	watchers map[uint64]*watcherEntry
	cancel   context.CancelFunc
}

func NewManager(streamSvc *streamsvc.Service) *Manager {
	return &Manager{
		streamSvc: streamSvc,
		flows:     make(map[string]*flowEntry),
		connSubs:  make(map[string]map[string]int64),
	}
}

func (m *Manager) Acquire(ctx context.Context, connID string, spec FlowSpec) (<-chan *types.SubscribeStreamResponse, func(), error) {
	if connID == "" {
		return nil, nil, fmt.Errorf("connID is required")
	}
	if !spec.Stream.Valid() {
		return nil, nil, fmt.Errorf("stream is invalid")
	}

	key := spec.key()
	watcherID := m.nextID.Add(1)
	w := &watcherEntry{
		connID: connID,
		ch:     make(chan *types.SubscribeStreamResponse, 256),
	}

	m.mu.Lock()
	flow, ok := m.flows[key]
	if !ok {
		flowCtx, cancel := context.WithCancel(context.Background())
		flow = &flowEntry{
			spec:     spec,
			watchers: make(map[uint64]*watcherEntry),
			cancel:   cancel,
		}
		m.flows[key] = flow
		go m.runFlow(flowCtx, key, spec)
	}
	flow.refs++
	flow.watchers[watcherID] = w
	if _, ok := m.connSubs[connID]; !ok {
		m.connSubs[connID] = make(map[string]int64)
	}
	m.connSubs[connID][key]++
	m.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			m.release(connID, key, watcherID)
		})
	}
	return w.ch, release, nil
}

func (m *Manager) ReleaseAll(connID string) {
	m.mu.Lock()
	subs, ok := m.connSubs[connID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.connSubs, connID)

	for key, cnt := range subs {
		flow, ok := m.flows[key]
		if !ok {
			continue
		}
		flow.refs -= cnt
		if flow.refs <= 0 {
			delete(m.flows, key)
			if flow.cancel != nil {
				flow.cancel()
			}
			for watcherID, w := range flow.watchers {
				delete(flow.watchers, watcherID)
				close(w.ch)
			}
		}
	}
	m.mu.Unlock()
}

func (m *Manager) release(connID, key string, watcherID uint64) {
	var (
		closeCh chan *types.SubscribeStreamResponse
		stop    context.CancelFunc
	)

	m.mu.Lock()
	flow, ok := m.flows[key]
	if !ok {
		m.mu.Unlock()
		return
	}
	if w, ok := flow.watchers[watcherID]; ok {
		closeCh = w.ch
		delete(flow.watchers, watcherID)
	}
	flow.refs--
	if flow.refs <= 0 {
		stop = flow.cancel
		delete(m.flows, key)
		for id, w := range flow.watchers {
			delete(flow.watchers, id)
			close(w.ch)
		}
	}
	if connFlowRefs, ok := m.connSubs[connID]; ok {
		connFlowRefs[key]--
		if connFlowRefs[key] <= 0 {
			delete(connFlowRefs, key)
		}
		if len(connFlowRefs) == 0 {
			delete(m.connSubs, connID)
		}
	}
	m.mu.Unlock()

	if closeCh != nil {
		close(closeCh)
	}
	if stop != nil {
		stop()
	}
}

func (m *Manager) runFlow(ctx context.Context, key string, spec FlowSpec) {
	req := &types.SubscribeStreamRequest{
		StreamType: spec.Stream,
		AccountId:  spec.Account,
		Exchange:   spec.Exchange,
		Symbol:     lo.FromPtr(spec.Symbol),
		Interval:   spec.Interval,
	}

	stream, err := m.streamSvc.SubscribeStream(ctx, req)
	if err != nil {
		exStr, symStr := "", ""
		if spec.Exchange != nil {
			exStr = spec.Exchange.String()
		}
		if spec.Symbol != nil {
			symStr = *spec.Symbol
		}
		log.Error().
			Err(err).
			Str("key", key).
			Str("exchange", exStr).
			Str("symbol", symStr).
			Str("stream", spec.Stream.String()).
			Msg("subscribe stream failed")
		m.closeFlowWithError(key)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msg, ok := <-stream
		if !ok {
			return
		}
		if err != nil {
			exStr, symStr := "", ""
			if spec.Exchange != nil {
				exStr = spec.Exchange.String()
			}
			if spec.Symbol != nil {
				symStr = *spec.Symbol
			}
			log.Error().
				Err(err).
				Str("key", key).
				Str("exchange", exStr).
				Str("symbol", symStr).
				Str("stream", spec.Stream.String()).
				Msg("stream recv failed")
			m.closeFlowWithError(key)
			return
		}
		m.broadcast(key, msg)
	}
}

func (m *Manager) broadcast(key string, msg *types.SubscribeStreamResponse) {
	m.mu.Lock()
	flow, ok := m.flows[key]
	if !ok {
		m.mu.Unlock()
		return
	}

	var evictIDs []uint64
	for id, w := range flow.watchers {
		select {
		case w.ch <- msg:
		default:
			dropped := w.droppedFrames.Add(1)
			// 节流日志
			now := time.Now().UnixNano()
			last := w.lastWarnAt.Load()
			if now-last >= watcherLogInterval.Nanoseconds() &&
				w.lastWarnAt.CompareAndSwap(last, now) {
				log.Warn().Str("key", key).Uint64("watcherID", id).
					Int64("droppedFrames", dropped).
					Msg("market stream watcher is slow, dropping frames")
			}
			if dropped >= watcherDropThreshold {
				evictIDs = append(evictIDs, id)
			}
		}
	}

	// 驱逐慢 watcher（仍在锁内摘除，锁外 close）
	var evictChs []chan *types.SubscribeStreamResponse
	for _, id := range evictIDs {
		w, ok := flow.watchers[id]
		if !ok {
			continue
		}
		evictChs = append(evictChs, w.ch)
		delete(flow.watchers, id)
		flow.refs--
		// 同步更新 connSubs
		if subs, ok := m.connSubs[w.connID]; ok {
			subs[key]--
			if subs[key] <= 0 {
				delete(subs, key)
			}
			if len(subs) == 0 {
				delete(m.connSubs, w.connID)
			}
		}
	}
	m.mu.Unlock()

	for _, ch := range evictChs {
		log.Warn().Str("key", key).Msg("evicting slow market stream watcher")
		close(ch)
	}
}

func (m *Manager) closeFlowWithError(key string) {
	m.mu.Lock()
	flow, ok := m.flows[key]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.flows, key)
	for connID, connFlows := range m.connSubs {
		delete(connFlows, key)
		if len(connFlows) == 0 {
			delete(m.connSubs, connID)
		}
	}
	watchers := flow.watchers
	m.mu.Unlock()

	for id, w := range watchers {
		delete(watchers, id)
		close(w.ch)
	}
}
