package account

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	"github.com/wangliang139/NovaForge/server/pkg/repos/assets"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
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

// assetTotalFromAssetsRow 当前 assets 表 total（非负钳制），与 GetBalance 总量口径一致。
func (e *Entity) assetTotalFromAssetsRow(ctx context.Context, accountID, assetCode string, walletType ctypes.WalletType) (decimal.Decimal, error) {
	code := ctypes.ParseAssetCode(assetCode)
	wt := assets.WalletType(walletType)
	row, err := e.db.AssetsRepo.GetAsset(ctx, assets.GetAssetParams{
		AccountID:  accountID,
		Asset:      code,
		WalletType: wt,
	})
	if err != nil {
		return decimal.Zero, err
	}
	w := decimal.Zero
	if row != nil {
		w = utils.Decimal.PgNumericToDecimal(row.Total)
	}
	return clampNonNegAssetTotal(w), nil
}

// assetWeightPickMultiBot P2 T12：asOf 非零且快照命中时用 snapTotal，否则用 liveTotal（均非负钳制）。与 assetWeightTotalForFanout 择路一致，供单测覆盖边界。
func assetWeightPickMultiBot(asOf time.Time, snapFound bool, snapTotal, liveTotal decimal.Decimal) decimal.Decimal {
	if !asOf.IsZero() && snapFound {
		return clampNonNegAssetTotal(snapTotal)
	}
	return clampNonNegAssetTotal(liveTotal)
}

// assetWeightTotalForFanout P2 T12：资金费等分摊权重；在 update.UpdatedTs 非零时优先读 asset_snapshot AtOrBefore(asOf)，无行则降级为当前 assets 行（与 docs/P2_T0_VIRTUAL_SUB_ATTRIBUTION.md §3「同一键」时序一致）。
func (e *Entity) assetWeightTotalForFanout(ctx context.Context, accountID, exchangeStr, assetCode string, walletType ctypes.WalletType, asOf time.Time) (decimal.Decimal, error) {
	live, err := e.assetTotalFromAssetsRow(ctx, accountID, assetCode, walletType)
	if err != nil {
		return decimal.Zero, err
	}
	if asOf.IsZero() {
		return live, nil
	}
	snap, err := e.GetAccountAssetSnapshotAtOrBefore(ctx, accountID, AccountStateAtAssetKey{
		Exchange:   exchangeStr,
		WalletType: walletType,
		Asset:      assetCode,
	}, asOf)
	if err != nil {
		return decimal.Zero, err
	}
	found := snap != nil && snap.Found
	st := decimal.Zero
	if found {
		st = snap.Total
	}
	return assetWeightPickMultiBot(asOf, found, st, live), nil
}

// computeSubWeightsAndUnalloc 按 P2 T0 §3：w_子i / 父 P 在 asOf 非零时优先取快照 floor，否则取当前 assets；w_unalloc = max(0, 父 − Σ子)。
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

// fanoutMultiBotAttributedBalanceUpdateIfNeeded 父 multi_bot 在父侧增量落库成功后，将可归因的 BalanceUpdate 按权重拆到各 virtual_sub（方案 1：父已持全额真值）。
// 子份额经合成 account_raw + handleAccountMessage（与 T4 一致），由 HandleAssetUpdates 统一写子 assets/ledger/Publish。
func (e *Entity) fanoutMultiBotAttributedBalanceUpdateIfNeeded(
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

	pid := parentID
	subs, err := e.db.AccountRepo.ListVirtualSubByParent(ctx, &pid)
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
		logP2T6BalanceFanoutZeroTotalWeight(ctx, parentID, exchange.String(), walletType, assetCode, string(update.Reason), wUnalloc, weights, ts, false)
		return nil
	}
	sharesFrozen, _, err := SplitProportionalDelta(frozenDelta, weights, wUnalloc)
	if err != nil {
		logP2T6BalanceFanoutZeroTotalWeight(ctx, parentID, exchange.String(), walletType, assetCode, string(update.Reason), wUnalloc, weights, ts, true)
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
		env := newSyntheticAccountRawBalanceUpdateEnvelope(parentID, exchange, sid, childUpdate)
		if env == nil {
			continue
		}
		if err := e.handleAccountMessage(ctx, env); err != nil {
			return err
		}
	}
	return nil
}
