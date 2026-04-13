package account

import (
	"context"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// applyMultiBotParentOrderStage P2 T4：在父账户已完成与交易所对齐的订单落库之后调用。
// 父 real+multi_bot 且 T1 归因产生子派发时，对每个子合成 account_raw（Synthetic + source_parent_id）并走 handleAccountMessage（§3.3）。
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
		env := newSyntheticAccountRawOrderEnvelope(parentID, exchange, d.SubAccountID, o)
		if err := e.handleAccountMessage(ctx, env); err != nil {
			return true, err
		}
	}
	return true, nil
}
