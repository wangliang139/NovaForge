package account

import (
	"context"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

// fanoutMultiBotSymbolLeverageIfNeeded P2 T8：父 real+multi_bot 在父侧 UpsertSymbolLeverage 并发布后，对每个 virtual_sub 合成 account_raw 再走 handleAccountMessage（子表落库与 account 流发布）。
func (e *Entity) fanoutMultiBotSymbolLeverageIfNeeded(ctx context.Context, parentID string, exchange ctypes.Exchange, update *ctypes.SymbolLeverage) error {
	if update == nil {
		return nil
	}
	acct, err := e.GetAccount(ctx, parentID)
	if err != nil || acct == nil {
		return err
	}
	if acct.AccountType != ctypes.AccountTypeReal || !acct.MultiBotMode {
		return nil
	}
	pid := parentID
	subs, err := e.db.AccountRepo.ListVirtualSubByParent(ctx, &pid)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		cp := *update
		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccountRaw,
			Account: lo.ToPtr(sub.ID),
		}
		ts := time.Now()
		if !cp.UpdatedTs.IsZero() {
			ts = cp.UpdatedTs
		}
		msg := ctypes.NewMessage(exchange, selector, &cp, ts)
		if err := e.PublishEvent(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// applyMultiBotParentOrderStage P2 T4：在父账户已完成与交易所对齐的订单落库之后调用。
// 父 real+multi_bot 且 T1 归因产生子派发时，对每个子经 PublishEvent 写入账户原始流（Synthetic + source_parent_id），由 ListenAccountEvent 消费并 handleAccountMessage（P2 3.3），与父侧 WS 主路径解耦。
func (e *Entity) applyMultiBotParentOrderStage(ctx context.Context, parentID string, exchange ctypes.Exchange, ord *ctypes.Order) (handled bool, err error) {
	if ord == nil {
		return false, nil
	}
	acct, err := e.GetAccount(ctx, parentID)
	if err != nil {
		return false, err
	}
	if acct == nil || acct.AccountType != ctypes.AccountTypeReal || !acct.MultiBotMode {
		return false, nil
	}
	disp, err := e.AttributeMultiBotOrderForFanout(ctx, parentID, exchange, ord)
	if err != nil {
		return false, err
	}
	if len(disp) == 0 {
		return false, nil
	}
	for _, d := range disp {
		o := d.Order
		if !d.Share.Equal(decimal.NewFromInt(1)) {
			o = scaleOrderForShare(o, d.Share)
		}
		ts := time.Now()
		if !ord.UpdatedTs.IsZero() {
			ts = ord.UpdatedTs
		}
		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccountRaw,
			Account: lo.ToPtr(d.SubAccountID),
		}
		msg := ctypes.NewMessage(exchange, selector, o, ts)
		if err := e.PublishEvent(ctx, msg); err != nil {
			logger.Ctx(ctx).Err(err).
				Str("parent_account_id", parentID).
				Str("sub_account_id", d.SubAccountID).
				Str("order_id", o.OrderID.String()).
				Msg("multi_bot parent order stage publish failed")
			return true, err
		}
	}
	return true, nil
}
