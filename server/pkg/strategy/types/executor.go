package types

import (
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// ExecutorStatus 执行器状态
type ExecutorStatus string

const (
	ExecutorStatusInit     ExecutorStatus = "init"
	ExecutorStatusFinished ExecutorStatus = "finished"
	ExecutorStatusRunning  ExecutorStatus = "running"
	ExecutorStatusCanceled ExecutorStatus = "canceled"
	ExecutorStatusError    ExecutorStatus = "error"
)

// State 风险控制状态（当前账户状态）
type RiskState struct {
	Exchange ctypes.Exchange

	// 当前持仓信息
	Positions []*ctypes.Position
	Assets    []*ctypes.AssetBo

	// 账户信息
	DailyPnL decimal.Decimal

	// 当前挂单
	OpenOrders map[ctypes.OrderId]*ctypes.Order
}

func (s *RiskState) GetPositionBySymbol(symbol *ctypes.Symbol) *ctypes.Position {
	if symbol == nil {
		return nil
	}
	for _, position := range s.Positions {
		if position.Symbol.Equal(*symbol) {
			return position
		}
	}
	return nil
}

func (s *RiskState) TotalEquity(symbol *ctypes.Symbol) decimal.Decimal {
	if symbol == nil {
		return decimal.Zero
	}
	walletType := ctypes.GetWalletType(s.Exchange, symbol.Type)
	totalEquity := decimal.Zero
	for _, asset := range s.Assets {
		if asset.WalletType == walletType && asset.Code == symbol.Quote {
			totalEquity = totalEquity.Add(asset.Notional)
		}
	}
	return totalEquity
}

// PlaceOrderCommand 下单命令
type PlaceOrderCommand struct {
	AccountID   string              `json:"accountId"`
	BotID       int64               `json:"botId"`
	Exchange    ctypes.Exchange     `json:"exchange"`
	Symbol      ctypes.Symbol       `json:"symbol"`
	Side        ctypes.PositionSide `json:"side"`
	IsBuy       bool                `json:"isBuy"`
	OrderType   ctypes.OrderType    `json:"orderType"`
	Price       *string             `json:"price"`    // 限价单价格，字符串格式
	Quantity    *string             `json:"quantity"` // 数量，字符串格式
	QuoteQty    *string             `json:"quoteQty"` // 金额（市价单），字符串格式
	TimeInForce *ctypes.TimeInForce `json:"timeInForce"`
	ReduceOnly  *bool               `json:"reduceOnly"`
	PostOnly    *bool               `json:"postOnly"`
}

// PlaceOrderResult 下单结果
type PlaceOrderResult struct {
	OrderID       ctypes.OrderId     `json:"orderId"`
	ExOrderID     ctypes.OrderId     `json:"-"`
	Status        ctypes.OrderStatus `json:"status"`
	ExecutedQty   string             `json:"executedQty"`
	ExecutedPrice string             `json:"executedPrice"`
	Error         string             `json:"error"`
}

// CancelOrderCommand 撤单命令
type CancelOrderCommand struct {
	AccountID string          `json:"accountId"`
	Exchange  ctypes.Exchange `json:"exchange"`
	Symbol    ctypes.Symbol   `json:"symbol"`
	OrderID   string          `json:"orderId"`
}

type ExecutorState struct {
	Portfolio           PortfolioSnapshot
	Status              ExecutorStatus
	RunErr              error
	JsRunnerStatus      string
	LastSignalTs        int64
	SignalAvgDurationMs int64
	SignalAvgLatencyMs  int64
}

type SignalStats struct {
	Ts         time.Time
	DurationMs int64 // duration in ms
	LatencyMs  int64 // latency in ms
}
