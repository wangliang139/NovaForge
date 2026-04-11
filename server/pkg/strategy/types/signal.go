package types

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// SignalDefinition 描述策略需求的信号定义（用于 Strategy.Signals / Create/UpdateStrategyRequest.Signals）。
// 说明：
// - 该结构用于"需求声明"，不等同于运行时的 Signal（后者是实际事件）。
// - exchange/symbol 可选（策略可不预设具体标的）。
// - props 用于承载 interval/levels/topic 等不同 type 的特定参数（值统一用 string，便于跨语言/跨存储）。
// - scope 用于定义信号的作用域：symbol（每个交易对独立）、exchange（每个交易所独立）、strategy（整个策略共享）。
type SignalDefinition struct {
	ID       string             `json:"id,omitempty"`
	Type     types.SignalType   `json:"type,omitempty"`
	Scope    ctypes.SignalScope `json:"scope,omitempty"` // 信号作用域
	Exchange *ctypes.Exchange   `json:"exchange,omitempty"`
	Symbol   *ctypes.Symbol     `json:"symbol,omitempty"`
	Props    map[string]any     `json:"props,omitempty"`
}

func (s SignalDefinition) Clone() *SignalDefinition {
	return &SignalDefinition{
		ID:       s.ID,
		Type:     s.Type,
		Scope:    s.Scope,
		Exchange: s.Exchange,
		Symbol:   s.Symbol,
		Props:    s.Props,
	}
}

type SignalBinding struct {
	SignalID     string           `json:"signalId,omitempty"`
	DatasourceID int32            `json:"datasourceId,omitempty"`
	Exchange     *ctypes.Exchange `json:"exchange,omitempty"`
	Symbol       *ctypes.Symbol   `json:"symbol,omitempty"`
}

type SignalSpec interface {
	GetID() string
	GetSignalID() string
	GetType() types.SignalType
	GetScope() types.SignalScope
	GetExchange() *ctypes.Exchange
	GetSymbol() *ctypes.Symbol
	GetProps() map[string]any
	GetStartTs() time.Time
	GetEndTs() time.Time
	MatchProps(props map[string]any) bool
}

// SignalSource 表示事件来源分类（用于诊断/过滤，不参与排序语义）。
type SignalSource string

const (
	SignalSourceDatasource SignalSource = "datasource"
	SignalSourceTimer      SignalSource = "timer"
	SignalSourceInternal   SignalSource = "internal"
	SignalSourceSystem     SignalSource = "system"
)

// SignalConsistency 表示事件的一致性级别（用于背压策略）
type SignalConsistency int

const (
	// ConsistencyStrong 强一致事件：订单、成交、账户/仓位变更、风控事件
	// 这些事件对交易状态机至关重要，必须可靠投递，不得丢失
	ConsistencyStrong SignalConsistency = 1

	// ConsistencyWeak 弱一致事件：高频行情数据（ticker、depth等）
	// 这些事件允许降采样或丢弃，但需要监控指标
	ConsistencyWeak SignalConsistency = 2

	// ConsistencyDefault 默认一致性级别（未分类的事件）
	ConsistencyDefault SignalConsistency = 0
)

// GetSignalConsistency 获取信号类型的一致性级别
func GetSignalConsistency(sigType types.SignalType) SignalConsistency {
	switch sigType {
	// 强一致事件
	case types.SignalTypeOrder, types.SignalTypeFill,
		types.SignalTypeBalance, types.SignalTypePosition,
		types.SignalTypeLeverage, types.SignalTypeRisk:
		return ConsistencyStrong

	// 弱一致事件（高频行情）
	case types.SignalTypeKline, types.SignalTypeTicker,
		types.SignalTypeDepth, types.SignalTypeTrade, types.SignalTypeMarkPrice:
		return ConsistencyWeak

	// 其他事件默认为强一致（保守策略）
	default:
		return ConsistencyDefault
	}
}

// GetSignalConsistencyBySignal 根据运行时 Signal 获取一致性级别
func GetSignalConsistencyBySignal(sig Signal) SignalConsistency {
	if sig == nil {
		return ConsistencyDefault
	}
	return GetSignalConsistency(sig.GetType())
}

// SignalScope 用于表达事件的作用域（可选）。
// 说明：排序时是否参与、如何参与，由 timeline.SortConfig.ScopePriority 决定。
type SignalScope struct {
	Exchange *ctypes.Exchange `json:"exchange,omitempty"`
	Symbol   *ctypes.Symbol   `json:"symbol,omitempty"`
	Topic    *string          `json:"topic,omitempty"`
}

// Message 是 TimelineScheduler 的统一事件模型。
//
// 关键字段含义：
// - Ts: 事件发布时间（主排序维度）
// - Source/SourceID: 事件来源分类 + 具体来源标识
// - SourceSeq: 由 source 自身生成且稳定的单调序号（用于同 Ts 的稳定排序）
// - GlobalSeq: 由 timeline 输出端按最终输出顺序赋值（用于回放与 debug）
type Message struct {
	ID    string    `json:"id,omitempty"`
	Ts    time.Time `json:"ts"`     // 事件时间
	PubAt time.Time `json:"pub_at"` // 事件发布时间

	Source    SignalSource `json:"-"`
	SourceID  string       `json:"-"`
	SourceSeq uint64       `json:"-"`
	GlobalSeq uint64       `json:"-"`
	IsDerived bool         `json:"-"`

	Signal Signal `json:"signal,omitempty"`
}

func NewMessage(signal Signal, isDerived bool) *Message {
	if signal == nil {
		return nil
	}
	return &Message{
		ID:        uuid.New().String(),
		Ts:        signal.GetTimestamp(),
		PubAt:     time.Now(),
		Signal:    signal,
		IsDerived: isDerived,
		Source:    SignalSourceInternal,
	}
}

func NewMessageWithSource(source SignalSource, sourceID string, sourceSeq uint64, signal Signal, isDerived bool) *Message {
	if signal == nil {
		return nil
	}
	msg := NewMessage(signal, isDerived)
	msg.Source = source
	msg.SourceID = sourceID
	msg.SourceSeq = sourceSeq
	return msg
}

// Scope 获取信号作用域
func (e *Message) Scope() *SignalScope {
	if e == nil || e.Signal == nil {
		return &SignalScope{}
	}
	return &SignalScope{
		Exchange: e.Signal.GetExchange(),
		Symbol:   e.Signal.GetSymbol(),
		Topic:    e.Signal.GetTopic(),
	}
}

func (e *Message) Exchange() *ctypes.Exchange {
	if e == nil || e.Signal == nil {
		return nil
	}
	return e.Signal.GetExchange()
}

func (e *Message) Symbol() *ctypes.Symbol {
	if e == nil || e.Signal == nil {
		return nil
	}
	return e.Signal.GetSymbol()
}

func (e *Message) Topic() *string {
	if e == nil || e.Signal == nil {
		return nil
	}
	return e.Signal.GetTopic()
}

func (e *Message) Type() types.SignalType {
	if e == nil || e.Signal == nil {
		return ""
	}
	return e.Signal.GetType()
}

func (e *Message) Kind() types.SignalKind {
	if e == nil || e.Signal == nil {
		return ""
	}
	return e.Signal.GetKind()
}

// Signal 信号
type Signal interface {
	GetID() string
	GetType() types.SignalType
	GetKind() types.SignalKind
	GetAccountID() *string
	GetExchange() *ctypes.Exchange
	GetSymbol() *ctypes.Symbol
	GetTopic() *string
	GetTimestamp() time.Time
	GetInboundAt() time.Time
	GetOutboundAt() time.Time
	GetReceiveAt() time.Time
}

type BaseSignal struct {
	ID         string           `json:"id,omitempty"`
	Exchange   *ctypes.Exchange `json:"exchange,omitempty"`
	Symbol     *ctypes.Symbol   `json:"symbol,omitempty"`
	Topic      *string          `json:"topic,omitempty"`
	AccountID  *string          `json:"accountId,omitempty"`
	Ts         time.Time        `json:"ts,omitempty"`         // 事件时间
	InboundAt  time.Time        `json:"inboundAt,omitempty"`  // 上游接收时间
	OutboundAt time.Time        `json:"outboundAt,omitempty"` // 上游发布时间
	ReceiveAt  time.Time        `json:"receiveAt,omitempty"`  // 本地接收时间
}

func (e *BaseSignal) GetID() string {
	return e.ID
}

func (e *BaseSignal) GetAccountID() *string {
	return e.AccountID
}

func (e *BaseSignal) GetExchange() *ctypes.Exchange {
	return e.Exchange
}

func (e *BaseSignal) GetSymbol() *ctypes.Symbol {
	return e.Symbol
}

func (e *BaseSignal) GetTopic() *string {
	return e.Topic
}

func (e *BaseSignal) GetTimestamp() time.Time {
	return e.Ts
}

func (e *BaseSignal) GetInboundAt() time.Time {
	return e.InboundAt
}

func (e *BaseSignal) GetOutboundAt() time.Time {
	return e.OutboundAt
}

func (e *BaseSignal) GetReceiveAt() time.Time {
	return e.ReceiveAt
}

/******** Market ********/
type TickerSignal struct {
	BaseSignal
	LastPrice     decimal.Decimal `json:"lastPrice,omitempty"`
	Open24        decimal.Decimal `json:"open24H,omitempty"`
	High24        decimal.Decimal `json:"high24H,omitempty"`
	Low24         decimal.Decimal `json:"low24H,omitempty"`
	Avg24         decimal.Decimal `json:"avg24H,omitempty"`
	Volume24      decimal.Decimal `json:"volume24H,omitempty"`
	QuoteVolume24 decimal.Decimal `json:"quoteVolume24H,omitempty"`
}

var _ Signal = (*TickerSignal)(nil)

func (e TickerSignal) GetType() types.SignalType { return types.SignalTypeTicker }
func (e TickerSignal) GetKind() types.SignalKind { return types.SignalKindTicker }

func (e TickerSignal) MarshalJSON() ([]byte, error) {
	type Alias TickerSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

type KlineSignal struct {
	BaseSignal
	Interval ctypes.Interval `json:"interval,omitempty"`
	Open     decimal.Decimal `json:"open,omitempty"`
	High     decimal.Decimal `json:"high,omitempty"`
	Low      decimal.Decimal `json:"low,omitempty"`
	Close    decimal.Decimal `json:"close,omitempty"`
	Volume   decimal.Decimal `json:"volume,omitempty"`
	OpenTs   int64           `json:"openTs,omitempty"`
	IsClosed bool            `json:"isClosed"`
}

var _ Signal = (*KlineSignal)(nil)

func (e KlineSignal) GetType() types.SignalType { return types.SignalTypeKline }

// KlineSignal 在时间线上代表 bar_close（Ts=CloseTs），用于表达该根 bar 完整落地后的 OHLCV。
func (e KlineSignal) GetKind() types.SignalKind { return types.SignalKindKline }

func (e KlineSignal) MarshalJSON() ([]byte, error) {
	type Alias KlineSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

type DepthSignal struct {
	BaseSignal
	OrderBook *ctypes.OrderBook `json:"orderBook,omitempty"`
}

var _ Signal = (*DepthSignal)(nil)

func (e DepthSignal) GetType() types.SignalType { return types.SignalTypeDepth }
func (e DepthSignal) GetKind() types.SignalKind { return types.SignalKindDepth }

func (e DepthSignal) MarshalJSON() ([]byte, error) {
	type Alias DepthSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

type MarkPriceSignal struct {
	BaseSignal
	Price decimal.Decimal `json:"price,omitempty"`
}

func (e MarkPriceSignal) GetType() types.SignalType { return types.SignalTypeMarkPrice }
func (e MarkPriceSignal) GetKind() types.SignalKind { return types.SignalKindMarkPrice }

func (e MarkPriceSignal) MarshalJSON() ([]byte, error) {
	type Alias MarkPriceSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

/******** Trade ********/

type TradeSignal struct {
	BaseSignal
	TradeID string          `json:"tradeId,omitempty"`
	Price   decimal.Decimal `json:"price,omitempty"`
	Size    decimal.Decimal `json:"size,omitempty"`
	IsBuy   bool            `json:"isBuy,omitempty"`
}

func (e TradeSignal) GetType() types.SignalType { return types.SignalTypeTrade }
func (e TradeSignal) GetKind() types.SignalKind { return types.SignalKindTrade }

func (e TradeSignal) MarshalJSON() ([]byte, error) {
	type Alias TradeSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

/******** Order ********/

type OrderIntent interface {
	Signal
	GetIntentKind() types.SignalKind
}

type OrderPlaceIntent struct {
	BaseSignal

	BotID         int64          `json:"botId,omitempty"`
	ClientOrderID ctypes.OrderId `json:"clientOrderId,omitempty"`

	IsBuy       bool                `json:"isBuy,omitempty"`
	Side        ctypes.PositionSide `json:"side,omitempty"`
	OrderType   ctypes.OrderType    `json:"orderType,omitempty"`
	Price       *decimal.Decimal    `json:"price,omitempty"`
	Quantity    *decimal.Decimal    `json:"quantity,omitempty"`
	QuoteQty    *decimal.Decimal    `json:"quoteQty,omitempty"`
	TimeInForce *ctypes.TimeInForce `json:"timeInForce,omitempty"`
	ReduceOnly  *bool               `json:"reduceOnly,omitempty"`
	PostOnly    *bool               `json:"postOnly,omitempty"`
}

var _ Signal = (*OrderPlaceIntent)(nil)

func (e OrderPlaceIntent) GetType() types.SignalType       { return types.SignalTypeOrder }
func (e OrderPlaceIntent) GetKind() types.SignalKind       { return types.SignalKindPlaceIntent }
func (e OrderPlaceIntent) GetIntentKind() types.SignalKind { return types.SignalKindPlaceIntent }

func (e OrderPlaceIntent) MarshalJSON() ([]byte, error) {
	type Alias OrderPlaceIntent
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

func (e OrderPlaceIntent) ToOrder() *ctypes.Order {
	return &ctypes.Order{
		ClientOrderID:    e.ClientOrderID,
		Exchange:         *e.Exchange,
		Symbol:           *e.Symbol,
		Side:             e.Side,
		IsBuy:            e.IsBuy,
		OrderType:        e.OrderType,
		Price:            lo.FromPtrOr(e.Price, decimal.Zero),
		OriginalQty:      lo.FromPtrOr(e.Quantity, decimal.Zero),
		OriginalQuoteQty: lo.FromPtrOr(e.QuoteQty, decimal.Zero),
		TimeInForce:      lo.FromPtrOr(e.TimeInForce, ctypes.TimeInForceGTC),
		ReduceOnly:       lo.FromPtrOr(e.ReduceOnly, false),
		PostOnly:         lo.FromPtrOr(e.PostOnly, false),
		Status:           ctypes.OrderStatusNew,
		CreatedTs:        e.Ts,
		UpdatedTs:        e.Ts,
	}
}

type OrderCancelIntent struct {
	BaseSignal

	ClientOrderID ctypes.OrderId `json:"orderId,omitempty"`
}

var _ Signal = (*OrderCancelIntent)(nil)

func (e OrderCancelIntent) GetType() types.SignalType       { return types.SignalTypeOrder }
func (e OrderCancelIntent) GetKind() types.SignalKind       { return types.SignalKindCancelIntent }
func (e OrderCancelIntent) GetIntentKind() types.SignalKind { return types.SignalKindCancelIntent }

func (e OrderCancelIntent) MarshalJSON() ([]byte, error) {
	type Alias OrderCancelIntent
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

// 订单生命周期信号
type OrderLifecycleSignal struct {
	BaseSignal
	OrderID ctypes.OrderId     `json:"orderId,omitempty"`
	Status  ctypes.OrderStatus `json:"status,omitempty"`
	Code    string             `json:"code,omitempty"`   // 交易所错误码（可选）
	Reason  string             `json:"reason,omitempty"` // 失败原因（可选）
}

func (e OrderLifecycleSignal) GetType() types.SignalType { return types.SignalTypeOrder }
func (e OrderLifecycleSignal) GetKind() types.SignalKind { return types.SignalKindOrderLifecycle }

func (e OrderLifecycleSignal) MarshalJSON() ([]byte, error) {
	type Alias OrderLifecycleSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

// OrderSnapshotSignal 订单快照信号（订单状态更新后的完整快照）
//
// 用途：
// - 由 OrderEngineManager 在订单状态变化时发布，携带订单完整信息
// - 下游（如 OrderCollector）订阅此信号即可获取最新订单状态，无需再调用 GetOrder() 或做增量计算
// - 覆盖的订单生命周期：accepted, fill, done, canceled, expired, rejected
type OrderSnapshotSignal struct {
	BaseSignal
	OrderID     ctypes.OrderId   `json:"orderId,omitempty"`
	TriggerKind types.SignalKind `json:"triggerKind,omitempty"` // 触发此快照的事件类型（如 fill, order_done）
	Order       *ctypes.Order    `json:"order,omitempty"`       // 订单完整快照
}

var _ Signal = (*OrderSnapshotSignal)(nil)

func (e OrderSnapshotSignal) GetType() types.SignalType { return types.SignalTypeOrder }
func (e OrderSnapshotSignal) GetKind() types.SignalKind { return types.SignalKindOrderSnapshot }

func (e OrderSnapshotSignal) MarshalJSON() ([]byte, error) {
	type Alias OrderSnapshotSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

/******** Balance ********/

// BalanceSignal 资金快照事件
//
// 语义：
// - Free: 可用余额
// - Frozen: 冻结余额
// - Free/Frozen 为快照值（绝对值，非增量）
// - 由 exchange gateway 或回测初始化阶段发布，供 Account/Portfolio/Risk/Strategy 只读消费
type BalanceSignal struct {
	BaseSignal
	WalletType ctypes.WalletType `json:"walletType,omitempty"`
	Asset      string            `json:"asset,omitempty"`
	Free       decimal.Decimal   `json:"free,omitempty"`   // 可用余额快照值（absolute）
	Frozen     decimal.Decimal   `json:"frozen,omitempty"` // 冻结余额快照值（absolute）
}

var _ Signal = (*BalanceSignal)(nil)

func (e BalanceSignal) GetType() types.SignalType { return types.SignalTypeBalance }
func (e BalanceSignal) GetKind() types.SignalKind { return types.SignalKindBalanceSnapshot }

func (e BalanceSignal) MarshalJSON() ([]byte, error) {
	type Alias BalanceSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

// BalanceDeltaSignal 资金变更事件（增量 delta 语义）
type BalanceDeltaSignal struct {
	BaseSignal
	WalletType ctypes.WalletType `json:"walletType,omitempty"`
	Asset      string            `json:"asset,omitempty"`
	Free       decimal.Decimal   `json:"free,omitempty"`   // 可用余额增量（delta）
	Frozen     decimal.Decimal   `json:"frozen,omitempty"` // 冻结余额增量（delta）
}

var _ Signal = (*BalanceDeltaSignal)(nil)

func (e BalanceDeltaSignal) GetType() types.SignalType { return types.SignalTypeBalance }
func (e BalanceDeltaSignal) GetKind() types.SignalKind { return types.SignalKindBalanceChanged }

func (e BalanceDeltaSignal) MarshalJSON() ([]byte, error) {
	type Alias BalanceDeltaSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

/******** Position ********/

// PositionSignal 持仓快照信号（snapshot）
//
// 语义：
// - Qty: signed net position 的“当前值”（LONG 为正，SHORT 为负）
// - EntryPrice: 当前持仓均价（Qty=0 时为 0）
// - Side: 方向提示字段（通常由 Qty 的符号推导得到；当 Qty=0 时可忽略）
//
// 说明：
// - PositionSignal 是强一致链路：下游应当以“覆盖/校准”的方式更新持仓视图，而不是做增量累加。
// - 对于 snapshot 更新，缺失的 symbol 视为仓位归零（由上游/订阅转换器负责补齐清仓快照）。
type PositionSignal struct {
	BaseSignal
	Side       ctypes.PositionSide `json:"side,omitempty"`
	Qty        decimal.Decimal     `json:"qty,omitempty"`
	EntryPrice decimal.Decimal     `json:"entryPrice,omitempty"`
}

var _ Signal = (*PositionSignal)(nil)

func (e PositionSignal) GetType() types.SignalType { return types.SignalTypePosition }
func (e PositionSignal) GetKind() types.SignalKind { return types.SignalKindPositionSnapshot }

func (e PositionSignal) MarshalJSON() ([]byte, error) {
	type Alias PositionSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

/******** Fill ********/

type FillSignal struct {
	BaseSignal
	// OrderID: exchange order id（撮合引擎/交易所侧订单ID）
	OrderID ctypes.OrderId      `json:"orderId,omitempty"`
	Side    ctypes.PositionSide `json:"side,omitempty"`
	IsBuy   bool                `json:"isBuy,omitempty"`
	Qty     decimal.Decimal     `json:"qty,omitempty"`
	Price   decimal.Decimal     `json:"price,omitempty"`
	Fee     decimal.Decimal     `json:"fee,omitempty"`
	Asset   string              `json:"asset,omitempty"` // 手续费扣除资产

	// 以 BaseCurrency 计价的收益拆分
	RealizedPnl decimal.Decimal `json:"realizedPnl,omitempty"` // 已实现盈亏（不含手续费/资金费）
}

func (e FillSignal) GetType() types.SignalType { return types.SignalTypeFill }
func (e FillSignal) GetKind() types.SignalKind { return types.SignalKindFill }

func (e FillSignal) MarshalJSON() ([]byte, error) {
	type Alias FillSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

/******** Leverage ********/

// LeverageChangedSignal 杠杆变更事件（账户配置变更）
//
// 维度：
// - AccountID：账户
// - Exchange + Symbol：标的（交易所维度下的 symbol）
type LeverageChangedSignal struct {
	BaseSignal
	Side     ctypes.PositionSide `json:"side,omitempty"`
	Leverage int                 `json:"leverage,omitempty"`
}

var _ Signal = (*LeverageChangedSignal)(nil)

func (e LeverageChangedSignal) GetType() types.SignalType { return types.SignalTypeLeverage }
func (e LeverageChangedSignal) GetKind() types.SignalKind { return types.SignalKindLeverageChanged }

func (e LeverageChangedSignal) MarshalJSON() ([]byte, error) {
	type Alias LeverageChangedSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

/******** Timer ********/

type TimerSignal struct {
	BaseSignal
	Time time.Time `json:"time,omitempty"`
}

var _ Signal = (*TimerSignal)(nil)

func (e TimerSignal) GetType() types.SignalType { return types.SignalTypeTimer }
func (e TimerSignal) GetKind() types.SignalKind { return types.SignalKindTimer }

func (e TimerSignal) MarshalJSON() ([]byte, error) {
	type Alias TimerSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}

// TestSignal 测试事件
type TestSignal struct {
	BaseSignal
}

func (e TestSignal) GetType() types.SignalType { return types.SignalTypeTest }
func (e TestSignal) GetKind() types.SignalKind { return types.SignalKindTest }

func (e TestSignal) MarshalJSON() ([]byte, error) {
	type Alias TestSignal
	return sonic.Marshal(struct {
		Alias
		Type types.SignalType `json:"type"`
		Kind types.SignalKind `json:"kind"`
	}{
		Alias: Alias(e),
		Type:  e.GetType(),
		Kind:  e.GetKind(),
	})
}
