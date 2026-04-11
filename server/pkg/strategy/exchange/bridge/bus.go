package bridge

import (
	"context"
	"fmt"
	"sync"
)

// Handler 交易所语义事件处理函数（engine -> gateway）。
type Handler func(ctx context.Context, ev ExchangeEvent) error

type SubscriptionID string

// Bus 用于撮合引擎向 gateway 单向投递交易所语义事件。
// 这里提供同步实现，保证回测确定性，并避免额外 channel。
type Bus interface {
	Subscribe(handler Handler) (SubscriptionID, error)
	Unsubscribe(id SubscriptionID) error
	Publish(ctx context.Context, ev ExchangeEvent) error
}

type syncBus struct {
	mu     sync.RWMutex
	nextID uint64
	subs   map[SubscriptionID]Handler
}

func NewSyncBus() Bus {
	return &syncBus{
		subs: make(map[SubscriptionID]Handler),
	}
}

func (b *syncBus) Subscribe(handler Handler) (SubscriptionID, error) {
	if handler == nil {
		return "", fmt.Errorf("handler cannot be nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	id := SubscriptionID(fmt.Sprintf("sub_%d", b.nextID))
	b.nextID++
	b.subs[id] = handler
	return id, nil
}

func (b *syncBus) Unsubscribe(id SubscriptionID) error {
	if id == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, id)
	return nil
}

func (b *syncBus) Publish(ctx context.Context, ev ExchangeEvent) error {
	if ev == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	b.mu.RLock()
	handlers := make([]Handler, 0, len(b.subs))
	for _, h := range b.subs {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()

	for _, h := range handlers {
		if h == nil {
			continue
		}
		if err := h(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}
