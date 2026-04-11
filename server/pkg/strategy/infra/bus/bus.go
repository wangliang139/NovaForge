package bus

import (
	"context"

	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// Bus 消息总线，负责在模块之间进行消息订阅和转发
type Bus interface {
	// Subscribe 订阅事件
	// handler: 事件处理函数，接收事件并处理
	// priority: 数字越小优先级越高（0 最高）
	// filters: 可选的过滤器，用于过滤特定类型的事件
	// 返回订阅ID，可用于取消订阅
	Subscribe(handler Handler, priority int, filters ...Filter) (SubscriptionID, error)

	// Unsubscribe 取消订阅
	Unsubscribe(id SubscriptionID) error

	// Publish 发布事件
	Publish(ctx context.Context, sig stypes.Signal) error

	// Start 启动消息总线
	Start(ctx context.Context) error

	// Stop 停止消息总线
	Stop(ctx context.Context) error
}

// Handler 事件处理函数
type Handler func(ctx context.Context, sig stypes.Signal) error

// SubscriptionID 订阅ID
type SubscriptionID string

// Stage 定义事件处理阶段（用于确定性排序）
type Stage int

const (
	// StageMarketData 市场数据更新阶段（最先）
	StageMarketData Stage = 100
	// StageMatching 撮合引擎阶段
	StageMatching Stage = 200
	// StageStateUpdate 状态更新阶段（订单、账户、仓位）
	StageStateUpdate Stage = 300
	// StageStrategy 策略执行阶段
	StageStrategy Stage = 400
	// StageCollectors 数据收集阶段（最后）
	StageCollectors Stage = 500
	// StageDefault 默认优先级（用于未指定 priority 的订阅）
	StageDefault Stage = 1000
)

// subscription 订阅信息
type subscription struct {
	id       SubscriptionID
	handler  Handler
	filters  []Filter
	priority int    // 数字越小优先级越高
	seq      uint64 // 用于同优先级的稳定排序（按订阅顺序）
	ctx      context.Context
	cancel   context.CancelFunc
}

type signalMessage struct {
	ctx    context.Context
	signal stypes.Signal
}
