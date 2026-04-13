package account

import (
	"context"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
)

// publishVirtualSubPositionsAfterOrderFill P2 T9 §3.1.5：virtual_sub 上订单出现成交增量后，从 DB 读取该账户在对应交易所上的非零持仓并发布 PositionsUpdate，供子维度 UI/策略与「仅看子 id」报表对齐。
func (e *Entity) publishVirtualSubPositionsAfterOrderFill(ctx context.Context, accountID string, exchange ctypes.Exchange, ord *ctypes.Order, prev *orders.Order) error {
	if ord == nil || e.engine == nil {
		return nil
	}
	prevExec := decimal.Zero
	if prev != nil {
		prevExec = utils.Decimal.PgNumericToDecimal(prev.ExecutedQty)
	}
	if !ord.ExecutedQty.GreaterThan(prevExec) {
		return nil
	}
	acct, err := e.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if acct == nil || acct.AccountType != ctypes.AccountTypeVirtualSub {
		return nil
	}
	if ord.Symbol.Type != ctypes.MarketTypeFuture {
		return nil
	}

	all, err := e.GetPositions(ctx, accountID)
	if err != nil {
		return err
	}
	onEx := filterPositionsByExchange(all, exchange)
	if len(onEx) == 0 {
		return nil
	}

	ts := ord.UpdatedTs
	if ts.IsZero() {
		ts = time.Now()
	}
	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}
	payload := &ctypes.PositionsUpdate{
		Type:      ctypes.UpdateTypeSnapshot,
		Reason:    "ORDER_DERIVED",
		Positions: onEx,
	}
	msg := ctypes.NewMessage(exchange, selector, payload, ts)
	if err := e.engine.Publish(ctx, msg); err != nil {
		return err
	}
	return nil
}

func filterPositionsByExchange(pos []*ctypes.Position, exchange ctypes.Exchange) []*ctypes.Position {
	out := make([]*ctypes.Position, 0, len(pos))
	for _, p := range pos {
		if p != nil && p.Exchange == exchange {
			out = append(out, p)
		}
	}
	return out
}
