package account

import (
	"context"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
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

func clampNonNegAssetTotal(d decimal.Decimal) decimal.Decimal {
	if d.IsNegative() {
		return decimal.Zero
	}
	return d
}

// assetWeightTotalForFanout P2 T12：资金费等分摊权重；仅 asset_snapshot AtOrBefore(asOf)；无快照行则权重为 0，不回读实时 assets。
func (e *Entity) assetWeightTotalForFanout(ctx context.Context, accountID, exchangeStr, assetCode string, walletType ctypes.WalletType, asOf time.Time) (decimal.Decimal, error) {
	snap, err := e.GetAccountAssetSnapshotAtOrBefore(ctx, accountID, AccountStateAtAssetKey{
		Exchange:   exchangeStr,
		WalletType: walletType,
		Asset:      assetCode,
	}, asOf)
	if err != nil {
		return decimal.Zero, err
	}
	st := decimal.Zero
	if snap != nil && snap.Found {
		st = snap.Total
	}
	return clampNonNegAssetTotal(st), nil
}

// computeSubWeightsAndUnalloc 按 P2 T0 §3：w_子i / 父 P 均来自 asset_snapshot；w_unalloc = max(0, 父 − Σ子)。
func (e *Entity) computeSubWeightsAndUnalloc(ctx context.Context, parentID, exchangeStr, assetCode string, walletType ctypes.WalletType, subs []accountrepo.Account, asOf time.Time) ([]SubWeight, decimal.Decimal, error) {
	var sumSubs decimal.Decimal
	weights := make([]SubWeight, 0, len(subs))
	for i := range subs {
		sid := subs[i].ID
		w, err := e.assetWeightTotalForFanout(ctx, sid, exchangeStr, assetCode, walletType, asOf)
		if err != nil {
			return nil, decimal.Zero, err
		}
		weights = append(weights, SubWeight{SubAccountID: sid, W: w})
		sumSubs = sumSubs.Add(w)
	}

	P, err := e.assetWeightTotalForFanout(ctx, parentID, exchangeStr, assetCode, walletType, asOf)
	if err != nil {
		return nil, decimal.Zero, err
	}
	U := P.Sub(sumSubs)
	if U.IsNegative() {
		U = decimal.Zero
	}
	return weights, U, nil
}

// fanoutMultiBotBalanceUpdateIfNeeded 父 multi_bot 在父侧增量落库成功后，将可归因的 BalanceUpdate 按权重拆到各 virtual_sub（方案 1：父已持全额真值）。
// 子份额经合成 account_raw + handleAccountMessage（与 T4 一致），由 HandleAssetUpdates 统一写子 assets/ledger/Publish。
func (e *Entity) fanoutMultiBotBalanceUpdateIfNeeded(
	ctx context.Context,
	parentID string,
	exchange ctypes.Exchange,
	update *ctypes.BalanceUpdate,
	walletType ctypes.WalletType,
	assetCode string,
	totalDelta, frozenDelta decimal.Decimal,
	ts time.Time,
) error {
	if update == nil || !ledgerReasonSplitToVirtualSubs(update.Reason) {
		return nil
	}
	if totalDelta.IsZero() && frozenDelta.IsZero() {
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
		logBalanceFanoutZeroTotalWeight(ctx, parentID, exchange.String(), walletType, assetCode, string(update.Reason), wUnalloc, weights, ts, false)
		return nil
	}
	sharesFrozen, _, err := SplitProportionalDelta(frozenDelta, weights, wUnalloc)
	if err != nil {
		logBalanceFanoutZeroTotalWeight(ctx, parentID, exchange.String(), walletType, assetCode, string(update.Reason), wUnalloc, weights, ts, true)
		return nil
	}

	for _, sub := range subs {
		sid := sub.ID
		st := sharesTotal[sid]
		sf := sharesFrozen[sid]
		var td *decimal.Decimal
		var fd *decimal.Decimal
		if !st.IsZero() {
			x := st
			td = &x
		}
		if !sf.IsZero() {
			y := sf
			fd = &y
		}
		if td == nil && fd == nil {
			continue
		}
		childUpdate := &ctypes.BalanceUpdate{
			Type:   ctypes.UpdateTypeIncrement,
			Reason: update.Reason,
			Assets: []*ctypes.AssetEvent{
				{
					WalletType: walletType,
					Code:       assetCode,
					Balance:    td,
					Locked:     fd,
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
	frozenLeg bool,
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
	if frozenLeg {
		ev.Msg("multi_bot balance fanout skipped: zero total weight (frozen leg)")
	} else {
		ev.Msg("multi_bot balance fanout skipped: zero total weight (total leg)")
	}
}