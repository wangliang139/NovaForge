package types

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type OrderId string

func (o OrderId) String() string {
	return string(o)
}

func NewOrderId() OrderId {
	id := uuid.NewString()
	id = strings.ReplaceAll(id, "-", "")
	return OrderId(id)
}

type OrderType string

const (
	OrderTypeMarket  OrderType = "MARKET"  // 市价单
	OrderTypeLimit   OrderType = "LIMIT"   // 限价单
	OrderTypeUnknown OrderType = "UNKNOWN" // 未知类型
)

func (o OrderType) String() string {
	return string(o)
}

func (o OrderType) Valid() bool {
	switch o {
	case OrderTypeMarket, OrderTypeLimit, OrderTypeUnknown:
		return true
	}
	return false
}

func AllOrderTypes() []OrderType {
	return []OrderType{
		OrderTypeMarket,
		OrderTypeLimit,
		OrderTypeUnknown,
	}
}

type OrderStatus string

const (
	OrderStatusNew         OrderStatus = "NEW"          // 新订单
	OrderStatusPending     OrderStatus = "PENDING"      // 待处理
	OrderStatusWorking     OrderStatus = "WORKING"      // 处理中（算法单）
	OrderStatusPartialDone OrderStatus = "PARTIAL_DONE" // 部分订单已被成交
	OrderStatusDone        OrderStatus = "DONE"         // 订单已完全成交
	OrderStatusCanceled    OrderStatus = "CANCELED"     // 用户撤销了订单
	OrderStatusRejected    OrderStatus = "REJECTED"     // 订单没有被交易引擎接受，也没被处理
	OrderStatusExpired     OrderStatus = "EXPIRED"      // 该订单根据订单类型的规则被取消
)

func (o OrderStatus) String() string {
	return string(o)
}

func (o OrderStatus) Valid() bool {
	switch o {
	case OrderStatusNew, OrderStatusPending, OrderStatusWorking, OrderStatusPartialDone, OrderStatusDone, OrderStatusCanceled, OrderStatusRejected, OrderStatusExpired:
		return true
	}
	return false
}

func (o OrderStatus) IsFinished() bool {
	switch o {
	case OrderStatusDone, OrderStatusCanceled, OrderStatusRejected, OrderStatusExpired:
		return true
	}
	return false
}

type TimeInForce string

const (
	TimeInForceGTC TimeInForce = "GTC" // 成交为止
	TimeInForceIOC TimeInForce = "IOC" // 立即成交并取消剩余
	TimeInForceFOK TimeInForce = "FOK" // 全部成交或立即取消
)

func (t TimeInForce) String() string {
	return string(t)
}

func (t TimeInForce) Valid() bool {
	switch t {
	case TimeInForceGTC, TimeInForceIOC, TimeInForceFOK:
		return true
	}
	return false
}

type PriceWorkingType string

const (
	PriceWorkingTypeLatest PriceWorkingType = "LATEST"
	PriceWorkingTypeMark   PriceWorkingType = "MARK"
	PriceWorkingTypeIndex  PriceWorkingType = "INDEX"
)

func (p PriceWorkingType) String() string {
	return string(p)
}

func (p PriceWorkingType) Valid() bool {
	switch p {
	case PriceWorkingTypeLatest, PriceWorkingTypeMark, PriceWorkingTypeIndex:
		return true
	}
	return false
}

type TriggerType string

const (
	TriggerNone       TriggerType = "NONE"
	TriggerStopLoss   TriggerType = "STOP_LOSS"
	TriggerTakeProfit TriggerType = "TAKE_PROFIT"
)

func (t TriggerType) String() string {
	return string(t)
}

func (t TriggerType) Valid() bool {
	switch t {
	case TriggerNone, TriggerStopLoss, TriggerTakeProfit:
		return true
	}
	return false
}

type OrderSource string

const (
	OrderSourceUser        OrderSource = "USER"
	OrderSourceStrategy    OrderSource = "STRATEGY"
	OrderSourceLiquidation OrderSource = "LIQUIDATION"
	OrderSourceADL         OrderSource = "ADL"
)

func (o OrderSource) String() string {
	return string(o)
}

func (o OrderSource) Valid() bool {
	switch o {
	case OrderSourceUser, OrderSourceStrategy, OrderSourceLiquidation, OrderSourceADL:
		return true
	}
	return false
}

type AlgoType string

const (
	AlgoTypeNone        AlgoType = "NONE"
	AlgoTypeConditional AlgoType = "CONDITIONAL"
	AlgoTypeTrailing    AlgoType = "TRAILING"
	AlgoTypeOCO         AlgoType = "OCO"
	AlgoTypeTWAP        AlgoType = "TWAP"
	AlgoTypeIceberg     AlgoType = "ICEBERG"
	AlgoTypeChase       AlgoType = "CHASE"
	AlgoTypeUnknown     AlgoType = "UNKNOWN"
)

type Order struct {
	// 归属信息
	AccountID string `json:"accountId,omitempty"` // 账户ID
	BotID     int64  `json:"botId,omitempty"`     // 机器人ID

	Exchange Exchange `json:"exchange,omitempty"` // 交易所
	Symbol   Symbol   `json:"symbol,omitempty"`   // 交易对

	// 本地与交易所 ID
	OrderID       OrderId `json:"orderId,omitempty"`       // 订单ID
	ClientOrderID OrderId `json:"clientOrderId,omitempty"` // 客户端订单ID
	DrivedOrderID OrderId `json:"drivedOrderId,omitempty"` // 衍生订单ID

	// 订单核心类型
	OrderType OrderType   `json:"orderType,omitempty"` // 订单类型
	AlgoType  AlgoType    `json:"algoType,omitempty"`  // 算法单类型
	Source    OrderSource `json:"source,omitempty"`    // 订单来源

	// 方向与仓位
	Side          PositionSide `json:"side,omitempty"`          // 方向
	IsBuy         bool         `json:"isBuy,omitempty"`         // 是否买入
	ReduceOnly    bool         `json:"reduceOnly,omitempty"`    // 是否只减仓
	ClosePosition bool         `json:"closePosition,omitempty"` // 是否平仓
	PostOnly      bool         `json:"postOnly,omitempty"`      // 是否只做 maker 单

	// 数量与价格
	Price            decimal.Decimal  `json:"price,omitempty"`            // 限价单价格
	OriginalQty      decimal.Decimal  `json:"originalQty,omitempty"`      // 下单数量
	ExecutedQty      decimal.Decimal  `json:"executedQty,omitempty"`      // 已成交数量
	OriginalQuoteQty decimal.Decimal  `json:"originalQuoteQty,omitempty"` // 下单金额（市价单时，为下单金额）
	ExecutedQuoteQty decimal.Decimal  `json:"executedQuoteQty,omitempty"` // 已成交金额
	AvgPrice         decimal.Decimal  `json:"avgPrice,omitempty"`         // 平均成交价格
	PriceWorkingType PriceWorkingType `json:"priceWorkingType,omitempty"` // 价格触发类型
	PriceMode        string           `json:"priceMode,omitempty"`        // 盘口价下单模式

	// 条件单（多条件统一）
	Conditions []OrderCondition `json:"conditions,omitempty"` // 算法单条件
	IsWorking  bool             `json:"isWorking,omitempty"`  // 算法单是否生效
	WorkingTs  *time.Time       `json:"workingTs,omitempty"`  // 算法单生效时间

	TimeInForce TimeInForce `json:"timeInForce,omitempty"` // 有效期类型

	Status OrderStatus `json:"status,omitempty"` // 订单状态

	// 费用与盈亏
	Locked      *decimal.Decimal `json:"locked,omitempty"`      // 锁定资产
	LockedAsset *string          `json:"lockedAsset,omitempty"` // 锁定资产类型
	Fee         *decimal.Decimal `json:"fee,omitempty"`         // 费用
	FeeAsset    *string          `json:"feeAsset,omitempty"`    // 费用资产
	RealizedPnl *decimal.Decimal `json:"realizedPnl,omitempty"` // 已实现盈亏（不含手续费）
	PnlAsset    *string          `json:"pnlAsset,omitempty"`    // 现货订单 realized_pnl 对应的资产；买入=quote，卖出=base

	RejectReason string `json:"rejectReason,omitempty"` // 订单被拒绝原因

	Raw string `json:"raw,omitempty"` // 交易所原始订单详情

	CreatedTs  time.Time  `json:"createdTs,omitempty"`  // 创建时间
	UpdatedTs  time.Time  `json:"updatedTs,omitempty"`  // 更新时间
	FinishedTs *time.Time `json:"finishedTs,omitempty"` // 完成时间
}

func (o *Order) Clone() *Order {
	return &Order{
		Exchange:         o.Exchange,
		Symbol:           o.Symbol,
		ClientOrderID:    o.ClientOrderID,
		OrderID:          o.OrderID,
		DrivedOrderID:    o.DrivedOrderID,
		Side:             o.Side,
		IsBuy:            o.IsBuy,
		OrderType:        o.OrderType,
		AlgoType:         o.AlgoType,
		Source:           o.Source,
		Price:            o.Price,
		OriginalQty:      o.OriginalQty,
		ExecutedQty:      o.ExecutedQty,
		OriginalQuoteQty: o.OriginalQuoteQty,
		ExecutedQuoteQty: o.ExecutedQuoteQty,
		AvgPrice:         o.AvgPrice,
		PriceWorkingType: o.PriceWorkingType,
		PriceMode:        o.PriceMode,
		Status:           o.Status,
		TimeInForce:      o.TimeInForce,
		ReduceOnly:       o.ReduceOnly,
		ClosePosition:    o.ClosePosition,
		PostOnly:         o.PostOnly,
		Conditions:       o.Conditions,
		IsWorking:        o.IsWorking,
		WorkingTs:        o.WorkingTs,
		CreatedTs:        o.CreatedTs,
		UpdatedTs:        o.UpdatedTs,
		FinishedTs:       o.FinishedTs,
	}
}

func (o *Order) IsReducePosition() bool {
	return IsReducePosition(o.Side, o.IsBuy)
}

func IsReducePosition(side PositionSide, isBuy bool) bool {
	if (side == PositionSideLong && !isBuy) || (side == PositionSideShort && isBuy) {
		return true
	}
	return false
}

type OrderAdvancedProps struct {
	TWAP    *TWAPProps    `json:"twap,omitempty"`
	Iceberg *IcebergProps `json:"iceberg,omitempty"`
	Chase   *ChaseProps   `json:"chase,omitempty"`
}

type TWAPProps struct {
	Duration time.Duration   `json:"duration,omitempty"`
	Size     decimal.Decimal `json:"size,omitempty"`
	Interval Interval        `json:"interval,omitempty"`
}

type IcebergProps struct {
	VisibleSize decimal.Decimal `json:"visibleSize,omitempty"`
}

type ChaseProps struct {
	Spread decimal.Decimal `json:"spread,omitempty"`
}

type OrderCondition struct {
	TriggerType      TriggerType      `json:"triggerType,omitempty"`
	ActivationPrice  decimal.Decimal  `json:"activationPrice,omitempty"`  // 激活价格
	OrderPrice       decimal.Decimal  `json:"orderPrice,omitempty"`       // 订单价格
	IsTrailing       bool             `json:"isTrailing,omitempty"`       // 是否追踪止损/止盈
	CallbackDistance decimal.Decimal  `json:"callbackDistance,omitempty"` // 回调距离（追踪止损）
	CallbackRate     decimal.Decimal  `json:"callbackRate,omitempty"`     // 回调比例（追踪止损）
	PriceWorkingType PriceWorkingType `json:"priceWorkingType,omitempty"` // 价格触发类型
	PriceMode        string           `json:"priceMode,omitempty"`        // 盘口价下单模式
	Activated        bool             `json:"activated,omitempty"`        // 是否已激活
	ActivatedTs      *time.Time       `json:"activatedTs,omitempty"`      // 激活时间
}

type PlaceOrderOutput struct {
	OrderId       OrderId     `json:"orderId"`
	ClientOrderId OrderId     `json:"clientOrderId"`
	Status        OrderStatus `json:"status"`
}

// ---- 订单服务 API 请求/响应（原 gRPC order 契约的纯类型形态）----

type GetOpenOrdersRequest struct {
	AccountID string `json:"accountId"`
	Symbol    string `json:"symbol,omitempty"`
}

type GetOpenOrdersResponse struct {
	Orders []*Order `json:"orders,omitempty"`
}

type QueryOrdersRequest struct {
	AccountID   string        `json:"accountId"`
	Symbol      string        `json:"symbol,omitempty"`
	Limit       int32         `json:"limit,omitempty"`
	Cursor      string        `json:"cursor,omitempty"`
	OrderType   *OrderType    `json:"orderType,omitempty"`
	OrderSource *OrderSource  `json:"orderSource,omitempty"`
	Statuses    []OrderStatus `json:"statuses,omitempty"`
	BotID       *int64        `json:"botId,omitempty"`
}

type QueryOrdersResponse struct {
	Orders  []*Order `json:"orders,omitempty"`
	HasMore bool     `json:"hasMore"`
	Next    string   `json:"next,omitempty"`
}

type QueryOrdersByPageRequest struct {
	AccountID   string        `json:"accountId"`
	Symbol      string        `json:"symbol,omitempty"`
	Page        int32         `json:"page"`
	Size        int32         `json:"size"`
	OrderType   *OrderType    `json:"orderType,omitempty"`
	OrderSource *OrderSource  `json:"orderSource,omitempty"`
	Statuses    []OrderStatus `json:"statuses,omitempty"`
	BotID       *int64        `json:"botId,omitempty"`
	StartTs     *int64        `json:"startTs,omitempty"`
	EndTs       *int64        `json:"endTs,omitempty"`
	// FinishedStartTsMs / FinishedEndTsMs 为 Unix 毫秒；同时设置时按 finished_ts 过滤（与 StartTs/EndTs 互斥，优先本字段）
	FinishedStartTsMs *int64 `json:"finishedStartTsMs,omitempty"`
	FinishedEndTsMs   *int64 `json:"finishedEndTsMs,omitempty"`
}

type QueryOrdersByPageResponse struct {
	Orders     []*Order `json:"orders,omitempty"`
	TotalCount int64    `json:"totalCount"`
}

// GetOrders / GetTotalCount 与 protobuf 生成代码风格对齐，便于调用方统一接口。
func (r *QueryOrdersByPageResponse) GetOrders() []*Order {
	if r == nil {
		return nil
	}
	return r.Orders
}

func (r *QueryOrdersByPageResponse) GetTotalCount() int64 {
	if r == nil {
		return 0
	}
	return r.TotalCount
}

type GetOrderRequest struct {
	AccountID       string `json:"accountId"`
	Symbol          string `json:"symbol"`
	ClientOrderID   string `json:"clientOrderId,omitempty"`
	ExchangeOrderID string `json:"exchangeOrderId,omitempty"`
}

type GetOrderResponse struct {
	Order *Order `json:"order,omitempty"`
}

type PlaceOrderRequest struct {
	AccountID     string       `json:"accountId"`
	Symbol        string       `json:"symbol"`
	Exchange      Exchange     `json:"exchange,omitempty"`
	OrderSource   *OrderSource `json:"orderSource,omitempty"`
	BotID         *int64       `json:"botId,omitempty"`
	ClientOrderID *string      `json:"clientOrderId,omitempty"`
	Side          PositionSide `json:"side"`
	IsBuy         bool         `json:"isBuy"`
	OrderType     OrderType    `json:"orderType"`
	Price         *string      `json:"price,omitempty"`
	Quantity      *string      `json:"quantity,omitempty"`
	QuoteQty      *string      `json:"quoteQty,omitempty"`
	TimeInForce   *string      `json:"timeInForce,omitempty"`
	ReduceOnly    *bool        `json:"reduceOnly,omitempty"`
	ClosePosition *bool        `json:"closePosition,omitempty"`
}

type PlaceOrderResponse struct {
	Error         *string     `json:"error,omitempty"`
	OrderId       string      `json:"orderId,omitempty"`
	ClientOrderId string      `json:"clientOrderId,omitempty"`
	Status        OrderStatus `json:"status,omitempty"`
}

type CancelOrderRequest struct {
	AccountID     string `json:"accountId"`
	Symbol        string `json:"symbol"`
	ClientOrderID string `json:"clientOrderId,omitempty"`
	OrderID       string `json:"orderId,omitempty"`
}

type CancelOrderResponse struct {
	Success bool    `json:"success"`
	Error   *string `json:"error,omitempty"`
}
