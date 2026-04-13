package account

import (
	"context"
	"strings"

	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

// classifyMultiBotOrderAccount 在已确认 stream 账户为「父 real + multi_bot」且各查询已执行的前提下，决定订单应写入的 account_id。
// 优先级：父 order_id → 子 order_id → 父 client_order_id → 父树下任意 client_order_id（含子）。
func classifyMultiBotOrderAccount(parentStreamID string,
	parentByOID, childByOID, parentByCID, underParentByCID *orders.Order,
) string {
	if parentByOID != nil {
		return parentStreamID
	}
	if childByOID != nil {
		return childByOID.AccountID
	}
	if parentByCID != nil {
		return parentStreamID
	}
	if underParentByCID != nil {
		return underParentByCID.AccountID
	}
	return parentStreamID
}

// resolveEffectiveAccountIDForOrder 将 WS / 轮询侧「连接账户」映射为实际落库账户。
// 非父 multi_bot：原样返回 streamAccountID；父 multi_bot：按 DB 中已有订单行归因到父或 virtual_sub。
func (e *Entity) resolveEffectiveAccountIDForOrder(ctx context.Context, streamAccountID string, exchange ctypes.Exchange, ord *ctypes.Order) (string, error) {
	if ord == nil || streamAccountID == "" {
		return streamAccountID, nil
	}

	acct, err := e.GetAccount(ctx, streamAccountID)
	if err != nil {
		return "", err
	}
	if acct == nil {
		return streamAccountID, nil
	}
	if acct.AccountType != ctypes.AccountTypeReal || !acct.MultiBotMode {
		return streamAccountID, nil
	}

	exStr := exchange.String()
	oid := strings.TrimSpace(ord.OrderID.String())
	cid := strings.TrimSpace(ord.ClientOrderID.String())

	var parentByOID, childByOID, parentByCID, underParentByCID *orders.Order

	if oid != "" {
		parentByOID, err = e.db.OrdersRepo.GetOrderByOrderId(ctx, orders.GetOrderByOrderIdParams{
			AccountID: streamAccountID,
			OrderID:   oid,
		})
		if err != nil {
			return "", err
		}
		if parentByOID == nil {
			pid := streamAccountID
			childByOID, err = e.db.OrdersRepo.GetOrderByOrderIdUnderVirtualSubs(ctx, orders.GetOrderByOrderIdUnderVirtualSubsParams{
				OrderID:         oid,
				Exchange:        exStr,
				ParentAccountID: &pid,
			})
			if err != nil {
				return "", err
			}
		}
	}

	if cid != "" {
		parentByCID, err = e.db.OrdersRepo.GetOrderByClientOrderId(ctx, orders.GetOrderByClientOrderIdParams{
			AccountID:     streamAccountID,
			ClientOrderID: cid,
		})
		if err != nil {
			return "", err
		}
		if parentByCID == nil {
			underParentByCID, err = e.db.OrdersRepo.GetOrderByClientOrderIdUnderParent(ctx, orders.GetOrderByClientOrderIdUnderParentParams{
				ClientOrderID: cid,
				Exchange:      exStr,
				AccountID:     streamAccountID,
			})
			if err != nil {
				return "", err
			}
		}
	}

	effective := classifyMultiBotOrderAccount(streamAccountID, parentByOID, childByOID, parentByCID, underParentByCID)
	if effective != streamAccountID {
		logger.Ctx(ctx).Info().
			Str("stream_account_id", streamAccountID).
			Str("effective_account_id", effective).
			Str("order_id", oid).
			Str("client_order_id", cid).
			Str("exchange", exStr).
			Msg("multi_bot order update attributed to sub-account")
	} else if acct.MultiBotMode && (oid != "" || cid != "") {
		logger.Ctx(ctx).Debug().
			Str("stream_account_id", streamAccountID).
			Str("order_id", oid).
			Str("client_order_id", cid).
			Str("exchange", exStr).
			Msg("multi_bot order update: no existing row under parent tree, using parent account")
	}

	return effective, nil
}
