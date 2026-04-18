package simulate

import (
	"context"

	"github.com/shopspring/decimal"
)

// TradingAdapter provides a low-friction integration boundary for upper layers.
// It intentionally mirrors the most-used private trading methods against SimExchange.
type TradingAdapter struct {
	ex *SimExchange
}

func NewTradingAdapter(ex *SimExchange) *TradingAdapter {
	return &TradingAdapter{ex: ex}
}

func (c *TradingAdapter) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (*PlaceOrderResult, error) {
	return c.ex.PlaceOrder(ctx, req)
}

func (c *TradingAdapter) CancelOrder(ctx context.Context, accountID string, sym Symbol, orderID string) error {
	return c.ex.CancelOrder(ctx, accountID, sym, orderID)
}

func (c *TradingAdapter) GetOrder(_ context.Context, accountID string, sym Symbol, orderID string) (SimOrder, bool) {
	return c.ex.GetOrder(accountID, sym, orderID)
}

func (c *TradingAdapter) GetOrders(_ context.Context, accountID string, sym Symbol) []SimOrder {
	return c.ex.ListOpenOrders(accountID, sym)
}

func (c *TradingAdapter) Balance(_ context.Context, accountID string) map[BalanceKey]decimal.Decimal {
	return c.ex.GetBalances(accountID)
}

func (c *TradingAdapter) Position(_ context.Context, accountID string, sym Symbol) (Position, bool) {
	return c.ex.GetPosition(accountID, sym)
}
