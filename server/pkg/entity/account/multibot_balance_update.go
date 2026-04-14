package account

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/internal/push"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

// ledgerReasonSplitToVirtualSubs P2 T7：可归因到 virtual_sub 的 BalanceUpdate 原因（与 docs/P2_T0_VIRTUAL_SUB_ATTRIBUTION.md §6 一致）。
func ledgerReasonSplitToVirtualSubs(r ctypes.LedgerReason) bool {
	switch r {
	case ctypes.LedgerReasonFundingFee:
		return true
	default:
		return false
	}
}

// fanoutMultiBotBalanceUpdateIfNeeded 父 multi_bot 在父侧增量落库成功后，将可归因的 BalanceUpdate 派生到各 virtual_sub。
// FundingFee 分支采用“子账户独立理论计算并按 asset 聚合”发布 account_raw；其余原因保持按权重拆分。
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
	if update.Reason != ctypes.LedgerReasonFundingFee && totalDelta.IsZero() {
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

	if update.Reason == ctypes.LedgerReasonFundingFee {
		return e.fanoutFundingFeeBySubTheory(ctx, exchange, parentID, update, subs, ts)
	}
	var (
		weights  []SubWeight
		wUnalloc decimal.Decimal
	)
	weights, wUnalloc, err = e.computeSubWeightsAndUnalloc(ctx, parentID, exchange.String(), assetCode, walletType, subs, ts)
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

func (e *Entity) notifyFundingFeeFanoutError(ctx context.Context, parentID, subID string, exchange ctypes.Exchange, asOf time.Time, err error) {
	msg := fmt.Sprintf("multi_bot 资金费派生失败\nparent=%s\nsub=%s\nexchange=%s\nasOf=%s\nerr=%v", parentID, subID, exchange.String(), asOf.Format(time.RFC3339), err)
	go func() {
		sendCtx := context.WithoutCancel(ctx)
		if nerr := push.Notify(sendCtx, push.NotifyRequest{
			SceneKey: "alarm.multibot_funding_fee",
			Message:  msg,
		}); nerr != nil {
			logger.Ctx(sendCtx).Warn().Err(nerr).Msg("failed to send funding fee push")
		}
	}()
}

func (e *Entity) fanoutFundingFeeBySubTheory(
	ctx context.Context,
	exchange ctypes.Exchange,
	parentID string,
	update *ctypes.BalanceUpdate,
	subs []accountrepo.Account,
	ts time.Time,
) error {
	for _, sub := range subs {
		feeByAsset, err := e.fundingFeeByAssetAtOrBefore(ctx, sub.ID, exchange, ts)
		if err != nil {
			logger.Ctx(ctx).Err(err).
				Str("parent_id", parentID).
				Str("sub_id", sub.ID).
				Str("exchange", exchange.String()).
				Time("as_of", ts).
				Msg("multi_bot funding fee fanout skipped for sub")
			e.notifyFundingFeeFanoutError(ctx, parentID, sub.ID, exchange, ts, err)
			continue
		}
		assets := make([]*ctypes.AssetEvent, 0, len(feeByAsset))
		for asset, fee := range feeByAsset {
			amt := roundFundingFeeAmount(fee)
			if amt.IsZero() {
				continue
			}
			cp := amt
			assets = append(assets, &ctypes.AssetEvent{
				WalletType: ctypes.WalletTypeFuture,
				Code:       asset,
				Balance:    &cp,
				Locked:     nil,
				UpdatedTs:  ts,
			})
		}
		if len(assets) == 0 {
			continue
		}
		childUpdate := &ctypes.BalanceUpdate{
			Type:   ctypes.UpdateTypeIncrement,
			Reason: ctypes.LedgerReasonFundingFee,
			Assets: assets,
			Detail: update.Detail,
		}
		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccountRaw,
			Account: lo.ToPtr(sub.ID),
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
