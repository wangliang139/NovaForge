package bus

import (
	"context"
	"fmt"

	"github.com/wangliang139/llt-trade/server/pkg/strategy/infra/timeline"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	"github.com/wangliang139/mow/logger"
)

// TimelineEventBus 时间线事件总线
// - Publish: 将事件写入 TimelineScheduler 的 InternalQueue
// - Send: 将 TimelineScheduler 排序后的事件分发到内部的 syncEventBus
type TimelineEventBus struct {
	scheduler *timeline.TimelineScheduler // TimelineScheduler 引用
	syncBus   Bus                         // 内部的 syncEventBus（用于订阅和分发）
}

var _ Bus = (*TimelineEventBus)(nil)

// NewTimeline 创建新的 TimelineEventBus
func NewTimeline(scheduler *timeline.TimelineScheduler) *TimelineEventBus {
	return &TimelineEventBus{
		scheduler: scheduler,
		syncBus:   NewSync(), // 内部使用 syncEventBus
	}
}

// Publish 发布事件：写入 TimelineScheduler 的 InternalQueue
func (b *TimelineEventBus) Publish(ctx context.Context, sig stypes.Signal) error {
	if sig == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// 获取 TimelineScheduler 的 InternalQueue 并写入
	internalQ := b.scheduler.GetInternalQueue()
	if internalQ == nil {
		return fmt.Errorf("internal queue not available")
	}

	return internalQ.Emit(ctx, stypes.NewMessage(sig, false))
}

// Send 发送 Frame 中的所有事件到内部的 syncEventBus
func (b *TimelineEventBus) Send(ctx context.Context, sig stypes.Signal) error {
	if sig == nil {
		return nil
	}
	if err := b.syncBus.Publish(ctx, sig); err != nil {
		logger.Ctx(ctx).Err(err).
			Str("event_id", sig.GetID()).
			Time("timestamp", sig.GetTimestamp()).
			Msg("failed to send event to sync bus")
		return fmt.Errorf("failed to send event: %w", err)
	}
	return nil
}

// SubscribeWithPriority 订阅事件并指定优先级（委托给内部的 syncEventBus）
func (b *TimelineEventBus) Subscribe(handler Handler, priority int, filters ...Filter) (SubscriptionID, error) {
	return b.syncBus.Subscribe(handler, priority, filters...)
}

// Unsubscribe 取消订阅（委托给内部的 syncEventBus）
func (b *TimelineEventBus) Unsubscribe(id SubscriptionID) error {
	return b.syncBus.Unsubscribe(id)
}

// Start 启动消息总线（委托给内部的 syncEventBus）
func (b *TimelineEventBus) Start(ctx context.Context) error {
	return b.syncBus.Start(ctx)
}

// Stop 停止消息总线（委托给内部的 syncEventBus）
func (b *TimelineEventBus) Stop(ctx context.Context) error {
	return b.syncBus.Stop(ctx)
}
