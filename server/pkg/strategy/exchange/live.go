package exchange

import (
	"context"
	"fmt"

	"github.com/wangliang139/llt-trade/server/pkg/strategy"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/proxy"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

// LiveGateway 实盘交易所网关
// 负责接收策略发送的请求，并通过 market 代理调用交易所接口
type LiveGateway struct{}

var _ strategy.Gateway = (*LiveGateway)(nil)

// NewLiveGateway 创建实盘交易所网关
func NewLiveGateway() *LiveGateway {
	return &LiveGateway{}
}

// PlaceOrder 下单
func (g *LiveGateway) PlaceOrder(ctx context.Context, intent stypes.OrderPlaceIntent) (ctypes.OrderId, error) {
	// 参数校验
	if intent.GetAccountID() == nil {
		return "", fmt.Errorf("account id is required")
	}
	if intent.GetExchange() == nil {
		return "", fmt.Errorf("exchange is required")
	}
	if intent.GetSymbol() == nil {
		return "", fmt.Errorf("symbol is required")
	}

	// 调用 market 代理下单（核心业务逻辑在 llt-data-api）
	orderID, clientOrderID, err := proxy.PlaceOrder(ctx, intent)
	if err != nil {
		return "", err
	}

	// 返回交易所订单 ID，如果交易所未返回则使用 clientOrderID
	if orderID != "" {
		return ctypes.OrderId(orderID), nil
	}
	return ctypes.OrderId(clientOrderID), nil
}

// CancelOrder 撤单
func (g *LiveGateway) CancelOrder(ctx context.Context, intent stypes.OrderCancelIntent) error {
	// 参数校验
	if intent.GetAccountID() == nil {
		return fmt.Errorf("account id is required")
	}
	if intent.GetExchange() == nil {
		return fmt.Errorf("exchange is required")
	}
	if intent.GetSymbol() == nil {
		return fmt.Errorf("symbol is required")
	}
	if intent.ClientOrderID == "" {
		return fmt.Errorf("order id is required")
	}

	accountID := *intent.GetAccountID()
	symbol := *intent.GetSymbol()
	clientOrderID := string(intent.ClientOrderID)

	// 调用 market 代理撤单
	return proxy.CancelOrder(ctx, accountID, symbol, clientOrderID)
}

// SetLeverage 设置杠杆
func (g *LiveGateway) SetLeverage(ctx context.Context, accountID string, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error {
	// 参数校验
	if accountID == "" {
		return fmt.Errorf("account id is required")
	}
	if !exchange.IsValid() {
		return fmt.Errorf("invalid exchange")
	}
	if !symbol.IsValid() {
		return fmt.Errorf("invalid symbol")
	}
	if leverage <= 0 {
		return fmt.Errorf("invalid leverage: %d", leverage)
	}

	// 调用 market 代理设置杠杆
	_, err := proxy.SetLeverage(ctx, accountID, symbol, leverage)
	return err
}

// GetLeverage 获取杠杆
func (g *LiveGateway) GetLeverage(ctx context.Context, accountID string, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error) {
	// 参数校验
	if accountID == "" {
		return 0, fmt.Errorf("account id is required")
	}
	if !exchange.IsValid() {
		return 0, fmt.Errorf("invalid exchange")
	}
	if !symbol.IsValid() {
		return 0, fmt.Errorf("invalid symbol")
	}

	// 调用 market 代理获取杠杆
	return proxy.GetLeverage(ctx, accountID, symbol)
}
