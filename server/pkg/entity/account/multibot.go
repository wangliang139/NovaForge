package account

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/logger"
)

const (
	p2ObsBalanceFanoutZeroW    = "p2.multi_bot.balance_fanout_zero_total_weight"
	p2ObsOrderPropEmptyWeights = "p2.multi_bot.order_proportional_empty_weights"
	p2ObsOrderPropZeroDenom    = "p2.multi_bot.order_proportional_zero_total_weight"
)

func sumSubWeightsForObs(ws []SubWeight) decimal.Decimal {
	var s decimal.Decimal
	for _, w := range ws {
		s = s.Add(w.W)
	}
	return s
}

// listVirtualSubsForParentFanoutAt 返回在 asOf 时点挂在父下的子账户（含 asOf 之后才软删的），用于 multi_bot 分摊/归因与「当前 ListVirtualSubByParent」解耦。
// asOf 为零时退化为仅未删除子账户（与历史行为一致）。
func (e *Entity) listVirtualSubsForParentFanoutAt(ctx context.Context, parentID string, asOf time.Time) ([]accountrepo.Account, error) {
	if parentID == "" {
		return nil, nil
	}
	pid := parentID
	if asOf.IsZero() {
		return e.db.AccountRepo.ListVirtualSubByParent(ctx, &pid)
	}
	return e.db.AccountRepo.ListVirtualSubByParentAsOf(ctx, accountrepo.ListVirtualSubByParentAsOfParams{
		ParentAccountID: &pid,
		CreatedAt:       asOf,
	})
}

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
	// 获取当前有效的虚拟子账户
	subs, err := e.db.AccountRepo.ListVirtualSubByParent(ctx, &parentID)
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
	scaled, err := e.buildScaledOrdersForMultiBotFanout(ctx, exchange, ord, disp)
	if err != nil {
		return false, err
	}
	for _, d := range disp {
		if d.Share.LessThanOrEqual(decimal.Zero) {
			continue
		}
		o, ok := scaled[d.SubAccountID]
		if !ok {
			o = d.Order
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

// SubWeight 单个子账户在分摊中的正权重（与 P2 T0 文档中 w_子 同量纲）。
type SubWeight struct {
	SubAccountID string
	W            decimal.Decimal
}

// SplitProportionalDelta 将父侧一条事件量 delta 按权重拆给各子；未分配份额由父吸收（不进入 toSub）。
// 权重分母 W = sum(subs.W) + wUnalloc；子 i 得到 delta * subs[i].W / W，父保留 delta * wUnalloc / W。
// 相同 SubAccountID 出现多次时，子份额相加合并。
// 若 W==0，返回错误（计划：不派发子）。
func SplitProportionalDelta(delta decimal.Decimal, subs []SubWeight, wUnalloc decimal.Decimal) (toSub map[string]decimal.Decimal, parentAbsorb decimal.Decimal, err error) {
	var sumW decimal.Decimal
	for _, s := range subs {
		if s.W.IsNegative() {
			return nil, decimal.Zero, errors.New(errors.InvalidArgument, fmt.Sprintf("negative weight for sub %q", s.SubAccountID))
		}
		sumW = sumW.Add(s.W)
	}
	if wUnalloc.IsNegative() {
		return nil, decimal.Zero, errors.New(errors.InvalidArgument, "w_unalloc must be non-negative")
	}
	W := sumW.Add(wUnalloc)
	if W.IsZero() {
		return nil, decimal.Zero, errors.New(errors.InvalidArgument, "total weight is zero: no proportional split")
	}

	toSub = make(map[string]decimal.Decimal)
	for _, s := range subs {
		if s.SubAccountID == "" {
			return nil, decimal.Zero, errors.New(errors.InvalidArgument, "sub_account_id is required")
		}
		part := delta.Mul(s.W).Div(W)
		toSub[s.SubAccountID] = toSub[s.SubAccountID].Add(part)
	}
	parentAbsorb = delta.Mul(wUnalloc).Div(W)
	return toSub, parentAbsorb, nil
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
	if st.IsNegative() {
		st = decimal.Zero
	}
	return st, nil
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
