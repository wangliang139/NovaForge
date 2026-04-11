package strategy

import (
	"context"

	"github.com/shopspring/decimal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// Executor 执行器接口
type Executor interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetSignalChannel() chan stypes.Signal
	GetBotID() int32
	GetState() *stypes.ExecutorState
}

// OrderEngine 订单引擎接口
type OrderEngine interface {
	// PlaceOrder 下单：集成 Risk Controller 和路由到 MatchingEngine
	PlaceOrder(ctx context.Context, req *stypes.PlaceOrderCommand, riskChecker RiskChecker) (*stypes.PlaceOrderResult, error)

	// CancelOrder 撤单
	CancelOrder(ctx context.Context, req *stypes.CancelOrderCommand) error

	// GetOrder 查询单个订单
	GetOrder(ctx context.Context, accountID string, symbol ctypes.Symbol, orderID ctypes.OrderId) (*ctypes.Order, error)

	// GetOrders 查询订单列表
	GetOrders(ctx context.Context, accountID string, symbol ctypes.Symbol) ([]*ctypes.Order, error)

	// GetAllOrders 查询所有订单
	GetAllOrders(ctx context.Context, accountID string) ([]*ctypes.Order, error)
}

type AccountIDProvider func(exchange ctypes.Exchange, symbol ctypes.Symbol) *string

// AccountEngine 账户提供器接口（供 MatchingEngine/OrderManager/Portfolio 使用）
//
// 说明：
// - exchange 用于 AccountManager 路由到具体 Account
// - accountId 不放在接口参数中（由 ctx 传递），避免污染业务接口；同时 signal 必须携带 BaseSignal.AccountID 用于共享 bus 的事件隔离
type AccountEngine interface {
	GetAsset(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string) (*ctypes.AssetBo, error)

	// GetBalance 批量获取账户下所有资产余额（面向 Portfolio 初始化使用）
	// - 返回的切片内每个 Asset 至少包含 Code/Balance/Locked 字段
	// - 如果账户不存在或无余额，返回空切片而不是 nil，以便调用方安全遍历
	GetBalance(ctx context.Context, accountID string) ([]*ctypes.AssetBo, error)

	FreezeFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error
	UnfreezeFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error

	GetPositions(ctx context.Context, accountID string) ([]*ctypes.Position, error)
	GetPosition(ctx context.Context, accountID string, symbol ctypes.Symbol, side ctypes.PositionSide) (*ctypes.Position, error)

	// 杠杆配置（按 accountId + 标的 + 仓位方向）
	SetLeverage(ctx context.Context, accountID string, symbol ctypes.Symbol, leverage int) error
	GetLeverage(ctx context.Context, accountID string, symbol ctypes.Symbol) (int, error)

	GetSymbolConfig(ctx context.Context, accountID string, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error)
}

// Gateway 交易所网关接口
type Gateway interface {
	PlaceOrder(ctx context.Context, intent stypes.OrderPlaceIntent) (ctypes.OrderId, error)
	CancelOrder(ctx context.Context, intent stypes.OrderCancelIntent) error
}

type RiskChecker func(ctx context.Context, intent stypes.OrderPlaceIntent) error
