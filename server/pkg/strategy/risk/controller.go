package risk

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/marketdata"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/portfolio"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// Config 风险控制配置
type Config struct {
	MaxPositionPerSymbol decimal.Decimal // 单标的最大持仓（名义价值）
	MaxTotalPosition     decimal.Decimal // 总持仓限额（名义价值）
	MaxOrderSize         decimal.Decimal // 单笔订单限额（名义价值）
	MaxDailyLoss         decimal.Decimal // 日亏损限额（绝对值）
	MaxLeverage          decimal.Decimal // 最大杠杆
	MaxConcentration     float64         // 最大持仓集中度（0-1，单个标的持仓占总持仓的比例）
}

// DefaultConfig 返回默认风险控制配置
func DefaultConfig() Config {
	return Config{
		MaxPositionPerSymbol: decimal.Zero, // 0 表示不限制
		MaxTotalPosition:     decimal.Zero, // 0 表示不限制
		MaxOrderSize:         decimal.Zero, // 0 表示不限制
		MaxDailyLoss:         decimal.Zero, // 0 表示不限制
		MaxLeverage:          decimal.Zero, // 0 表示不限制
		MaxConcentration:     0,            // 0 表示不限制
	}
}

// BacktestRiskController 回测风险控制器
type RiskController struct {
	config         Config
	portfolio      *portfolio.Portfolio
	marketProvider marketdata.MarketProvider
}

// NewBacktestRiskController 创建回测风险控制器
func NewRiskController(config Config, portfolio *portfolio.Portfolio, marketProvider marketdata.MarketProvider) *RiskController {
	return &RiskController{
		config:         config,
		portfolio:      portfolio,
		marketProvider: marketProvider,
	}
}

// Check 校验订单是否符合业务风控规则
func (c *RiskController) Check(ctx context.Context, intent stypes.OrderPlaceIntent) error {
	// 1. 基础校验（交易对是否允许） TODO
	// if !slices.Contains(m.config.AllowedSymbols, exSymbol.Key()) {
	// 	return &stypes.PlaceOrderResult{
	// 		Status: ctypes.OrderStatusRejected,
	// 		Error:  "trading symbol is not allowed",
	// 	}, nil
	// }

	state := c.portfolio.BuildRiskState()

	// 计算订单名义价值（需要价格）
	var orderNotional decimal.Decimal
	if intent.QuoteQty != nil {
		orderNotional = *intent.QuoteQty
	}
	if intent.Quantity == nil {
		return fmt.Errorf("quantity is required")
	}

	var price decimal.Decimal
	if intent.Price != nil {
		price = *intent.Price
	} else if intent.QuoteQty != nil {
		var err error
		price, err = c.marketProvider.GetLastPrice(ctx, *intent.Exchange, *intent.Symbol)
		if err != nil {
			return fmt.Errorf("failed to get last price: %w", err)
		}
	}
	orderNotional = price.Mul(*intent.Quantity)

	// 1. 单笔订单限额检查
	if !c.config.MaxOrderSize.IsZero() && orderNotional.GreaterThan(c.config.MaxOrderSize) {
		return fmt.Errorf("order size %s exceeds max order size %s", orderNotional.String(), c.config.MaxOrderSize.String())
	}

	// 2. 日亏损限额检查
	if !c.config.MaxDailyLoss.IsZero() && state.DailyPnL.LessThan(c.config.MaxDailyLoss.Neg()) {
		return fmt.Errorf("daily loss %s exceeds max daily loss %s", state.DailyPnL.String(), c.config.MaxDailyLoss.String())
	}

	// 3. 单标的最大持仓检查

	// 4. 总持仓限额检查

	// 5. 持仓集中度检查

	// 6. 合约（期货）风控：以该 symbol 当前杠杆为准做保证金与杠杆校验

	return nil
}
