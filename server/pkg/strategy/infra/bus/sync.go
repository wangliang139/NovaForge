package bus

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/rs/zerolog/log"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/mow/logger"
)

// syncEventBus 同步消息总线实现（用于回测场景，保证确定性）
type syncEventBus struct {
	mu              sync.RWMutex
	subscriptions   map[SubscriptionID]*subscription // 用于快速查找
	orderedSubs     []*subscription                  // 保持稳定顺序
	ctx             context.Context
	cancel          context.CancelFunc
	nextID          uint64
	nextSeq         uint64 // 用于同优先级的稳定排序
	started         bool
	needsReordering bool // 标记是否需要重新排序
}

// NewSync 创建新的同步消息总线（用于回测场景）
func NewSync() Bus {
	ctx, cancel := context.WithCancel(context.Background())
	return &syncEventBus{
		subscriptions:   make(map[SubscriptionID]*subscription),
		orderedSubs:     make([]*subscription, 0),
		ctx:             ctx,
		cancel:          cancel,
		started:         false,
		needsReordering: false,
	}
}

// Subscribe 订阅事件并指定优先级（同步版本）
func (b *syncEventBus) Subscribe(handler Handler, priority int, filters ...Filter) (SubscriptionID, error) {
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
		seq:      b.nextSeq,
		ctx:      subCtx,
		cancel:   cancel,
	}
	b.nextSeq++

	b.subscriptions[id] = sub
	b.orderedSubs = append(b.orderedSubs, sub)
	b.needsReordering = true

	log.Debug().
		Str("subscription_id", string(id)).
		Int("priority", priority).
		Int("filters_count", len(filters)).
		Msg("sync event subscription created")

	return id, nil
}

// Unsubscribe 取消订阅（同步版本）
func (b *syncEventBus) Unsubscribe(id SubscriptionID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub, exists := b.subscriptions[id]
	if !exists {
		return fmt.Errorf("subscription not found: %s", id)
	}

	sub.cancel()
	delete(b.subscriptions, id)

	// 从有序列表中移除
	for i, s := range b.orderedSubs {
		if s.id == id {
			b.orderedSubs = append(b.orderedSubs[:i], b.orderedSubs[i+1:]...)
			break
		}
	}

	log.Debug().
		Str("subscription_id", string(id)).
		Msg("sync event subscription removed")

	return nil
}

// Publish 发布事件（同步版本，直接调用处理器，不经过 channel）
func (b *syncEventBus) Publish(ctx context.Context, sig stypes.Signal) error {
	if sig == nil {
		return fmt.Errorf("signal cannot be nil")
	}

	select {
	case <-b.ctx.Done():
		return fmt.Errorf("bus is stopped")
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// 同步分发事件，直接调用所有匹配的处理器
	b.dispatchEvent(ctx, sig)
	return nil
}

// Start 启动消息总线（同步版本，无需启动 goroutine）
func (b *syncEventBus) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ctx.Err() != nil {
		// 如果已经停止，重新创建 context
		b.ctx, b.cancel = context.WithCancel(ctx)
	}

	b.started = true
	log.Info().Msg("sync event bus started")
	return nil
}

// Stop 停止消息总线（同步版本）
func (b *syncEventBus) Stop(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 取消所有订阅
	for _, sub := range b.subscriptions {
		sub.cancel()
	}
	b.subscriptions = make(map[SubscriptionID]*subscription)

	// 停止总线
	b.cancel()
	b.started = false

	log.Info().Msg("sync event bus stopped")
	return nil
}

// dispatchEvent 分发单个事件（同步版本，直接调用处理器）
func (b *syncEventBus) dispatchEvent(ctx context.Context, sig stypes.Signal) {
	b.mu.RLock()
	// 如果需要重新排序，先排序
	if b.needsReordering {
		b.mu.RUnlock()
		b.mu.Lock()
		if b.needsReordering { // double-check
			b.reorderSubscriptions()
			b.needsReordering = false
		}
		b.mu.Unlock()
		b.mu.RLock()
	}

	// 复制有序列表（避免持有读锁期间订阅变更）
	subs := make([]*subscription, len(b.orderedSubs))
	copy(subs, b.orderedSubs)
	b.mu.RUnlock()

	// 按优先级顺序同步分发到所有匹配的订阅者
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

		// 再次检查订阅是否已取消
		select {
		case <-sub.ctx.Done():
			continue
		default:
		}

		// 同步调用处理器
		if err := sub.handler(ctx, sig); err != nil {
			logger.Ctx(ctx).Err(err).
				Str("subscription_id", string(sub.id)).
				Int("priority", sub.priority).
				Str("event_id", sig.GetID()).
				Msg("sync event handler error")
		}
	}
}

// reorderSubscriptions 按优先级和序号重新排序订阅（需持有写锁）
func (b *syncEventBus) reorderSubscriptions() {
	// 使用稳定排序：先按 priority，再按 seq
	// Go 的 sort.SliceStable 保证相同 priority 的元素保持原有顺序
	sort.SliceStable(b.orderedSubs, func(i, j int) bool {
		if b.orderedSubs[i].priority != b.orderedSubs[j].priority {
			return b.orderedSubs[i].priority < b.orderedSubs[j].priority
		}
		return b.orderedSubs[i].seq < b.orderedSubs[j].seq
	})
}

// matchFilters 检查事件是否匹配所有过滤器（同步版本）
func (b *syncEventBus) matchFilters(sig stypes.Signal, filters []Filter) bool {
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
