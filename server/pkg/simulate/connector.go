package simulate

import (
	"context"

	"github.com/shopspring/decimal"
)

// ConnectorAdapter provides a low-friction integration boundary for upper layers.
// It intentionally mirrors the most-used private trading methods.
type ConnectorAdapter struct {
	ex *SimExchange
}

func NewConnectorAdapter(ex *SimExchange) *ConnectorAdapter {
	return &ConnectorAdapter{ex: ex}
}

func (c *ConnectorAdapter) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (*PlaceOrderResult, error) {
	return c.ex.PlaceOrder(ctx, req)
}

func (c *ConnectorAdapter) CancelOrder(ctx context.Context, accountID string, sym Symbol, orderID string) error {
	return c.ex.CancelOrder(ctx, accountID, sym, orderID)
}

func (c *ConnectorAdapter) GetOrder(_ context.Context, accountID string, sym Symbol, orderID string) (SimOrder, bool) {
	return c.ex.GetOrder(accountID, sym, orderID)
}

func (c *ConnectorAdapter) GetOrders(_ context.Context, accountID string, sym Symbol) []SimOrder {
	return c.ex.ListOpenOrders(accountID, sym)
}

func (c *ConnectorAdapter) Balance(_ context.Context, accountID string) map[BalanceKey]decimal.Decimal {
	return c.ex.GetBalances(accountID)
}

func (c *ConnectorAdapter) Position(_ context.Context, accountID string, sym Symbol) (Position, bool) {
	return c.ex.GetPosition(accountID, sym)
}
