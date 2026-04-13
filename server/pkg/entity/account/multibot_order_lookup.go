package account

import (
	"context"
	"strings"

	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// multiBotOrderLookup 父 multi_bot 下 stream 账户对订单的 DB 命中（与 resolveEffectiveAccountIDForOrder 查询一致）。
type multiBotOrderLookup struct {
	ParentByOrderID            *orders.Order
	ChildByOrderID             *orders.Order
	ParentByClientOrderID      *orders.Order
	UnderParentByClientOrderID *orders.Order
}

func (e *Entity) loadMultiBotOrderLookup(ctx context.Context, streamAccountID string, exchange ctypes.Exchange, ord *ctypes.Order) (multiBotOrderLookup, error) {
	var out multiBotOrderLookup
	if ord == nil {
		return out, nil
	}
	exStr := exchange.String()
	oid := strings.TrimSpace(ord.OrderID.String())
	cid := strings.TrimSpace(ord.ClientOrderID.String())

	var err error

	if oid != "" {
		out.ParentByOrderID, err = e.db.OrdersRepo.GetOrderByOrderId(ctx, orders.GetOrderByOrderIdParams{
			AccountID: streamAccountID,
			OrderID:   oid,
		})
		if err != nil {
			return out, err
		}
		if out.ParentByOrderID == nil {
			pid := streamAccountID
			out.ChildByOrderID, err = e.db.OrdersRepo.GetOrderByOrderIdUnderVirtualSubs(ctx, orders.GetOrderByOrderIdUnderVirtualSubsParams{
				OrderID:         oid,
				Exchange:        exStr,
				ParentAccountID: &pid,
			})
			if err != nil {
				return out, err
			}
		}
	}

	if cid != "" {
		out.ParentByClientOrderID, err = e.db.OrdersRepo.GetOrderByClientOrderId(ctx, orders.GetOrderByClientOrderIdParams{
			AccountID:     streamAccountID,
			ClientOrderID: cid,
		})
		if err != nil {
			return out, err
		}
		if out.ParentByClientOrderID == nil {
			out.UnderParentByClientOrderID, err = e.db.OrdersRepo.GetOrderByClientOrderIdUnderParent(ctx, orders.GetOrderByClientOrderIdUnderParentParams{
				ClientOrderID: cid,
				Exchange:      exStr,
				AccountID:     streamAccountID,
			})
			if err != nil {
				return out, err
			}
		}
	}

	return out, nil
}
