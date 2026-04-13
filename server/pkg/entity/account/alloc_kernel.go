package account

import (
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/mow/errors"
)

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

// subAccountIDsInFirstAppearanceOrder 返回 subs 中首次出现的 SubAccountID 顺序（用于末子修正）。
func subAccountIDsInFirstAppearanceOrder(subs []SubWeight) []string {
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, s := range subs {
		if s.SubAccountID == "" {
			continue
		}
		if seen[s.SubAccountID] {
			continue
		}
		seen[s.SubAccountID] = true
		out = append(out, s.SubAccountID)
	}
	return out
}

// SplitProportionalDeltaRoundLastChild 先按 SplitProportionalDelta 得精确子份额；对前 n-1 个子（按首次出现顺序）
// 将金额四舍五入到 places 位；**最后一个子**承担子间舍入残差，使 sum(子) = delta - parentExact（与精确拆一致）。
// 父吸收 parentAbsorb = delta - sum(子最终)，在仅子间舍入修正时等于 parentExact。
func SplitProportionalDeltaRoundLastChild(delta decimal.Decimal, subs []SubWeight, wUnalloc decimal.Decimal, places int32) (toSub map[string]decimal.Decimal, parentAbsorb decimal.Decimal, err error) {
	exact, parentExact, err := SplitProportionalDelta(delta, subs, wUnalloc)
	if err != nil {
		return nil, decimal.Zero, err
	}
	if len(subs) == 0 {
		return make(map[string]decimal.Decimal), delta, nil
	}

	order := subAccountIDsInFirstAppearanceOrder(subs)
	if len(order) == 0 {
		return make(map[string]decimal.Decimal), delta, nil
	}

	childrenTotal := delta.Sub(parentExact)
	toSub = make(map[string]decimal.Decimal)
	if len(order) == 1 {
		id := order[0]
		toSub[id] = childrenTotal.Round(places)
		parentAbsorb = delta.Sub(toSub[id])
		return toSub, parentAbsorb, nil
	}

	var sumRoundedMid decimal.Decimal
	for i := 0; i < len(order)-1; i++ {
		id := order[i]
		ex := exact[id]
		r := ex.Round(places)
		toSub[id] = r
		sumRoundedMid = sumRoundedMid.Add(r)
	}
	lastID := order[len(order)-1]
	toSub[lastID] = childrenTotal.Sub(sumRoundedMid)

	parentAbsorb = delta.Sub(sumSubMap(toSub))
	return toSub, parentAbsorb, nil
}

func sumSubMap(m map[string]decimal.Decimal) decimal.Decimal {
	var s decimal.Decimal
	for _, v := range m {
		s = s.Add(v)
	}
	return s
}
