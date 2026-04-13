package account

import (
	"context"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
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
		env := newSyntheticAccountRawSymbolLeverageEnvelope(parentID, exchange, sub.ID, &cp)
		if env == nil {
			continue
		}
		if err := e.handleAccountMessage(ctx, env); err != nil {
			return err
		}
	}
	return nil
}
