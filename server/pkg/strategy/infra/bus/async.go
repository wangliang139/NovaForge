package bus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/mow/logger"
)

// asyncEventBus 异步消息总线实现
type asyncEventBus struct {
	mu            sync.RWMutex
	subscriptions map[SubscriptionID]*subscription
	eventCh       chan signalMessage
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	nextID        uint64
	stopped       bool // 防止 Stop 重复关闭 channel（如 Start 失败时 cleanup 与 registry 均会调 Stop）

	// 背压统计指标
	droppedWeakEvents   atomic.Uint64 // 被丢弃的弱一致事件数
	blockedStrongEvents atomic.Uint64 // 因队列满而阻塞的强一致事件数
}

// NewAsync 创建新的异步消息总线（用于生产环境）
func NewAsync() Bus {
	ctx, cancel := context.WithCancel(context.Background())
	return &asyncEventBus{
		subscriptions: make(map[SubscriptionID]*subscription),
		eventCh:       make(chan signalMessage, 1000), // 缓冲通道，避免阻塞
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SubscribeWithPriority 订阅事件并指定优先级
func (b *asyncEventBus) Subscribe(handler Handler, priority int, filters ...Filter) (SubscriptionID, error) {
	if handler == nil {
		return "", fmt.Errorf("handler cannot be nil")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	id := SubscriptionID(fmt.Sprintf("sub_%d", b.nextID))
	b.nextID++

	subCtx, cancel := context.WithCancel(b.ctx)
	sub := &subscription{
		id:       id,
		handler:  handler,
		filters:  filters,
		priority: priority,
		seq:      0, // async 不需要严格顺序，保留字段以保持结构一致
		ctx:      subCtx,
		cancel:   cancel,
	}

	b.subscriptions[id] = sub

	log.Debug().
		Str("subscription_id", string(id)).
		Int("priority", priority).
		Int("filters_count", len(filters)).
		Msg("event subscription created")

	return id, nil
}

// Unsubscribe 取消订阅
func (b *asyncEventBus) Unsubscribe(id SubscriptionID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub, exists := b.subscriptions[id]
	if !exists {
		return fmt.Errorf("subscription not found: %s", id)
	}

	sub.cancel()
	delete(b.subscriptions, id)

	log.Debug().
		Str("subscription_id", string(id)).
		Msg("event subscription removed")

	return nil
}

// Publish 发布事件（带一致性级别的背压策略）
func (b *asyncEventBus) Publish(ctx context.Context, signal stypes.Signal) error {
	if signal == nil {
		return fmt.Errorf("event cannot be nil")
	}

	select {
	case <-b.ctx.Done():
		return fmt.Errorf("bus is stopped")
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// 获取事件一致性级别
	consistency := stypes.GetSignalConsistency(signal.GetType())

	// 尝试非阻塞发送
	select {
	case b.eventCh <- signalMessage{ctx: ctx, signal: signal}:
		return nil
	default:
		// 通道满了，根据一致性级别决定策略
		switch consistency {
		case stypes.ConsistencyStrong:
			// 强一致事件：阻塞等待（不能丢）
			b.blockedStrongEvents.Add(1)
			logger.Ctx(ctx).Warn().
				Str("event_id", signal.GetID()).
				Str("event_type", string(signal.GetType())).
				Msg("event channel full, blocking for strong consistency event")

			select {
			case b.eventCh <- signalMessage{ctx: ctx, signal: signal}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-b.ctx.Done():
				return fmt.Errorf("bus is stopped")
			}

		case stypes.ConsistencyWeak:
			// 弱一致事件：直接丢弃（允许降采样）
			b.droppedWeakEvents.Add(1)
			logger.Ctx(ctx).Debug().
				Str("event_id", signal.GetID()).
				Str("event_type", string(signal.GetType())).
				Msg("event channel full, dropping weak consistency event")
			return fmt.Errorf("event channel full, dropped weak consistency event")

		default:
			// 未分类事件：默认采用强一致策略（保守）
			b.blockedStrongEvents.Add(1)
			logger.Ctx(ctx).Warn().
				Str("event_id", signal.GetID()).
				Str("event_type", string(signal.GetType())).
				Msg("event channel full, blocking for default consistency event")

			select {
			case b.eventCh <- signalMessage{ctx: ctx, signal: signal}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-b.ctx.Done():
				return fmt.Errorf("bus is stopped")
			}
		}
	}
}

// Start 启动消息总线
func (b *asyncEventBus) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ctx.Err() != nil {
		// 如果已经停止，重新创建 context
		b.ctx, b.cancel = context.WithCancel(ctx)
	}

	b.wg.Add(1)
	go b.dispatchLoop()

	log.Info().Msg("event bus started")
	return nil
}

// Stop 停止消息总线（幂等：多次调用仅实际执行一次）
func (b *asyncEventBus) Stop(ctx context.Context) error {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return nil
	}
	b.stopped = true

	// 取消所有订阅
	for _, sub := range b.subscriptions {
		sub.cancel()
	}
	b.subscriptions = make(map[SubscriptionID]*subscription)

	// 停止分发循环
	b.cancel()

	// 关闭事件通道
	close(b.eventCh)

	// 等待分发循环结束
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	b.mu.Unlock()

	select {
	case <-done:
		log.Info().Msg("event bus stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// dispatchLoop 事件分发循环
func (b *asyncEventBus) dispatchLoop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			// 处理剩余的事件
			b.drainEvents()
			return
		case msg, ok := <-b.eventCh:
			if !ok {
				// 通道已关闭
				return
			}
			b.dispatchEvent(msg.ctx, msg.signal)
		}
	}
}

// drainEvents 排空剩余事件
func (b *asyncEventBus) drainEvents() {
	for {
		select {
		case msg, ok := <-b.eventCh:
			if !ok {
				return
			}
			b.dispatchEvent(msg.ctx, msg.signal)
		default:
			return
		}
	}
}

// dispatchEvent 分发单个事件
func (b *asyncEventBus) dispatchEvent(ctx context.Context, sig stypes.Signal) {
	b.mu.RLock()
	subs := make([]*subscription, 0, len(b.subscriptions))
	for _, sub := range b.subscriptions {
		subs = append(subs, sub)
	}
	b.mu.RUnlock()

	// 并发分发到所有匹配的订阅者
	var wg sync.WaitGroup
	for _, sub := range subs {
		// 检查订阅是否已取消
		select {
		case <-sub.ctx.Done():
			continue
		default:
		}

		// 检查过滤器
		if !b.matchFilters(sig, sub.filters) {
			continue
		}

		wg.Add(1)
		go func(s *subscription) {
			defer wg.Done()

			// 再次检查订阅是否已取消
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			// 调用处理器
			if err := s.handler(ctx, sig); err != nil {
				logger.Ctx(ctx).Err(err).
					Str("subscription_id", string(s.id)).
					Str("event_id", sig.GetID()).
					Msg("event handler error")
			}
		}(sub)
	}

	wg.Wait()
}

// matchFilters 检查事件是否匹配所有过滤器
func (b *asyncEventBus) matchFilters(sig stypes.Signal, filters []Filter) bool {
	if len(filters) == 0 {
		// 没有过滤器，匹配所有事件
		return true
	}

	for _, filter := range filters {
		if filter == nil {
			continue
		}
		if !filter.Match(sig) {
			return false
		}
	}

	return true
}

// GetMetrics 获取背压统计指标
func (b *asyncEventBus) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"dropped_weak_events":   b.droppedWeakEvents.Load(),
		"blocked_strong_events": b.blockedStrongEvents.Load(),
		"queue_capacity":        uint64(cap(b.eventCh)),
		"queue_length":          uint64(len(b.eventCh)),
	}
}
