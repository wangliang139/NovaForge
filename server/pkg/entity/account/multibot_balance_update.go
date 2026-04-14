package account

import (
	"context"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

// ledgerReasonSplitToVirtualSubs P2 T7：可归因到 virtual_sub 的 BalanceUpdate 原因（与 docs/P2_T0_VIRTUAL_SUB_ATTRIBUTION.md §6 一致）。
func ledgerReasonSplitToVirtualSubs(r ctypes.LedgerReason) bool {
	switch r {
	case ctypes.LedgerReasonFundingFee,
		ctypes.LedgerReasonInterestDeduction,
		ctypes.LedgerReasonInsuranceClear:
		return true
	default:
		return false
	}
}

// fanoutMultiBotBalanceUpdateIfNeeded 父 multi_bot 在父侧增量落库成功后，将可归因的 BalanceUpdate 的可用余额增量按权重拆到各 virtual_sub（方案 1：父已持全额真值）。
// 不向子账户分摊冻结资金（Locked）；冻结变化仅在父侧体现。
// 子份额经合成 account_raw + handleAccountMessage（与 T4 一致），由 HandleAssetUpdates 统一写子 assets/ledger/Publish。
func (e *Entity) fanoutMultiBotBalanceUpdateIfNeeded(
	ctx context.Context,
	parentID string,
	exchange ctypes.Exchange,
	update *ctypes.BalanceUpdate,
	walletType ctypes.WalletType,
	assetCode string,
	totalDelta decimal.Decimal,
	ts time.Time,
) error {
	if update == nil || !ledgerReasonSplitToVirtualSubs(update.Reason) {
		return nil
	}
	if totalDelta.IsZero() {
		return nil
	}

	acct, err := e.GetAccount(ctx, parentID)
	if err != nil || acct == nil {
		return err
	}
	if acct.AccountType != ctypes.AccountTypeReal || !acct.MultiBotMode {
		return nil
	}

	subs, err := e.listVirtualSubsForParentFanoutAt(ctx, parentID, ts)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}

	weights, wUnalloc, err := e.computeSubWeightsAndUnalloc(ctx, parentID, exchange.String(), assetCode, walletType, subs, ts)
	if err != nil {
		return err
	}

	sharesTotal, _, err := SplitProportionalDelta(totalDelta, weights, wUnalloc)
	if err != nil {
		logBalanceFanoutZeroTotalWeight(ctx, parentID, exchange.String(), walletType, assetCode, string(update.Reason), wUnalloc, weights, ts)
		return nil
	}

	for _, sub := range subs {
		sid := sub.ID
		st := sharesTotal[sid]
		if st.IsZero() {
			continue
		}
		td := st
		childUpdate := &ctypes.BalanceUpdate{
			Type:   ctypes.UpdateTypeIncrement,
			Reason: update.Reason,
			Assets: []*ctypes.AssetEvent{
				{
					WalletType: walletType,
					Code:       assetCode,
					Balance:    &td,
					Locked:     nil,
					UpdatedTs:  ts,
				},
			},
			Detail: update.Detail,
		}
		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccountRaw,
			Account: lo.ToPtr(sid),
		}
		msg := ctypes.NewMessage(exchange, selector, childUpdate, ts)
		if err := e.PublishEvent(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func logBalanceFanoutZeroTotalWeight(
	ctx context.Context,
	parentID, exchangeStr string,
	walletType ctypes.WalletType,
	assetCode string,
	ledgerReason string,
	wUnalloc decimal.Decimal,
	weights []SubWeight,
	ts time.Time,
) {
	ev := logger.Ctx(ctx).Warn().
		Str("p2_obs", p2ObsBalanceFanoutZeroW).
		Str("parent_id", parentID).
		Str("exchange", exchangeStr).
		Str("asset", assetCode).
		Str("wallet_type", string(walletType)).
		Str("ledger_reason", ledgerReason).
		Str("w_unalloc", wUnalloc.String()).
		Str("sum_sub_w", sumSubWeightsForObs(weights).String()).
		Int("sub_count", len(weights))
	if !ts.IsZero() {
		ev = ev.Time("as_of", ts)
	}
	ev.Msg("multi_bot balance fanout skipped: zero total weight")
}
