package account

import (
	"context"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/internal/consts"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type subShare struct {
	id string
	s  decimal.Decimal
}

type scaleFieldKind int

const (
	scaleFieldBaseQty scaleFieldKind = iota
	scaleFieldQuoteQty
	scaleFieldMoney
)

func marketLotStepBase(m *ctypes.Market) decimal.Decimal {
	if m != nil && m.Rules.LotSize.IsPositive() {
		return m.Rules.LotSize
	}
	bp := consts.DefaultAssetPrecision
	if m != nil && m.BaseAssetPrecision > 0 {
		bp = m.BaseAssetPrecision
	}
	return decimal.NewFromInt(1).Shift(-int32(bp))
}

func floorDecimalPlaces(d decimal.Decimal, places int32) decimal.Decimal {
	if places < 0 {
		places = 0
	}
	if places > consts.DefaultAssetPrecision {
		places = consts.DefaultAssetPrecision
	}
	return d.RoundDown(places)
}

func deriveQuoteFromBaseMap(
	baseMap map[string]decimal.Decimal,
	avgPrice decimal.Decimal,
) map[string]decimal.Decimal {
	out := make(map[string]decimal.Decimal, len(baseMap))
	for key, value := range baseMap {
		out[key] = value.Mul(avgPrice)
	}
	return out
}

// floorScaledPortion 对已按份额缩放后的量 t 做向下取整。若 t 为负，则对 |t| 按与正数相同规则 floor 后再取负，避免 Floor 向 -∞ 导致子合计比父更负。
func floorScaledPortion(t decimal.Decimal, isFuture bool, kind scaleFieldKind) decimal.Decimal {
	if t.IsNegative() {
		return floorScaledPortion(t.Abs(), isFuture, kind).Neg()
	}
	switch kind {
	case scaleFieldBaseQty:
		return floorDecimalPlaces(t, int32(consts.DefaultAssetPrecision))
	case scaleFieldQuoteQty:
		return floorDecimalPlaces(t, int32(consts.DefaultAssetPrecision))
	case scaleFieldMoney:
		return floorDecimalPlaces(t, int32(consts.DefaultAssetPrecision))
	default:
		return t
	}
}

func floorFieldShare(parentVal decimal.Decimal, row subShare, isFuture bool, kind scaleFieldKind) decimal.Decimal {
	return floorScaledPortion(parentVal.Mul(row.s), isFuture, kind)
}

// allocateFieldAmongSubs 将 parentVal 按 share 拆到各子；各子统一向下取整（合约 base 按 lot，其余按精度）。
// floor 后的剩余量不再补给任一子账户，统一由父侧吸收，避免子侧出现尾差超配。
func allocateFieldAmongSubs(parentVal decimal.Decimal, shares []subShare, isFuture bool, kind scaleFieldKind) map[string]decimal.Decimal {
	out := make(map[string]decimal.Decimal, len(shares))
	for _, row := range shares {
		out[row.id] = decimal.Zero
	}
	if len(shares) == 0 || parentVal.IsZero() {
		return out
	}
	sumShares := decimal.Zero
	for _, row := range shares {
		sumShares = sumShares.Add(row.s)
	}
	if sumShares.IsZero() {
		return out
	}
	if len(shares) == 1 {
		out[shares[0].id] = floorFieldShare(parentVal, shares[0], isFuture, kind)
		return out
	}
	for _, row := range shares {
		q := floorScaledPortion(parentVal.Mul(row.s), isFuture, kind)
		if parentVal.IsPositive() && q.IsNegative() {
			q = decimal.Zero
		}
		out[row.id] = q
	}
	return out
}

func sortedSubSharesFromDispatches(disp []SubRawDispatch) []subShare {
	seen := make(map[string]decimal.Decimal, len(disp))
	for _, d := range disp {
		seen[d.SubAccountID] = d.Share
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]subShare, 0, len(ids))
	for _, id := range ids {
		out = append(out, subShare{id: id, s: seen[id]})
	}
	return out
}

func (e *Entity) getMarket(ctx context.Context, ex ctypes.Exchange, sym ctypes.Symbol) *ctypes.Market {
	if e == nil || e.engine == nil {
		return nil
	}
	p := e.engine.GetMarketProvider()
	if p == nil {
		return nil
	}
	mkt, err := p.GetMarket(ctx, ex, sym)
	if err != nil || mkt == nil {
		return nil
	}
	return mkt
}

func normalizeOrderForAccountRaw(o ctypes.Order) ctypes.Order {
	o.OriginalQty = floorDecimalPlaces(o.OriginalQty, int32(consts.DefaultAssetPrecision))
	o.ExecutedQty = floorDecimalPlaces(o.ExecutedQty, int32(consts.DefaultAssetPrecision))
	o.OriginalQuoteQty = floorDecimalPlaces(o.OriginalQuoteQty, int32(consts.DefaultAssetPrecision))
	o.ExecutedQuoteQty = floorDecimalPlaces(o.ExecutedQuoteQty, int32(consts.DefaultAssetPrecision))
	if o.Fee != nil {
		v := floorDecimalPlaces(*o.Fee, int32(consts.DefaultAssetPrecision))
		o.Fee = &v
	}
	if o.RealizedPnl != nil {
		v := floorDecimalPlaces(*o.RealizedPnl, int32(consts.DefaultAssetPrecision))
		o.RealizedPnl = &v
	}
	return o
}

// buildScaledOrdersForMultiBotFanout 按 Share 与 Market 精度/lot 生成各子逻辑订单；各子独立向下取整，剩余量由父吸收。
func (e *Entity) buildScaledOrdersForMultiBotFanout(ctx context.Context, ex ctypes.Exchange, parent *ctypes.Order, disp []SubRawDispatch) (map[string]ctypes.Order, error) {
	if len(disp) == 0 || parent == nil {
		return map[string]ctypes.Order{}, nil
	}
	isFut := parent.Symbol.Type == ctypes.MarketTypeFuture
	shares := sortedSubSharesFromDispatches(disp)

	origMap := allocateFieldAmongSubs(parent.OriginalQty, shares, isFut, scaleFieldBaseQty)
	execMap := allocateFieldAmongSubs(parent.ExecutedQty, shares, isFut, scaleFieldBaseQty)
	origQuoteMap := allocateFieldAmongSubs(parent.OriginalQuoteQty, shares, isFut, scaleFieldQuoteQty)
	execQuoteMap := allocateFieldAmongSubs(parent.ExecutedQuoteQty, shares, isFut, scaleFieldQuoteQty)
	if parent.AvgPrice.IsPositive() {
		if parent.OriginalQty.IsPositive() {
			origQuoteMap = deriveQuoteFromBaseMap(origMap, parent.AvgPrice)
		}
		if parent.ExecutedQty.IsPositive() {
			execQuoteMap = deriveQuoteFromBaseMap(execMap, parent.AvgPrice)
		}
	}

	var feeMap, pnlMap map[string]decimal.Decimal
	if parent.Fee != nil {
		feeMap = allocateFieldAmongSubs(*parent.Fee, shares, false, scaleFieldMoney)
	}
	if parent.RealizedPnl != nil {
		pnlMap = allocateFieldAmongSubs(*parent.RealizedPnl, shares, false, scaleFieldMoney)
	}

	out := make(map[string]ctypes.Order, len(disp))
	for _, d := range disp {
		o := cloneOrderForSub(*parent, d.SubAccountID)
		sid := d.SubAccountID
		o.OriginalQty = origMap[sid]
		o.ExecutedQty = execMap[sid]
		o.OriginalQuoteQty = origQuoteMap[sid]
		o.ExecutedQuoteQty = execQuoteMap[sid]
		if feeMap != nil {
			f := feeMap[sid]
			o.Fee = &f
		} else {
			o.Fee = nil
		}
		if pnlMap != nil {
			p := pnlMap[sid]
			o.RealizedPnl = &p
		} else {
			o.RealizedPnl = nil
		}
		o.Locked = nil
		o = normalizeOrderForAccountRaw(o)

		if o.OriginalQty.LessThanOrEqual(decimal.Zero) {
			continue
		}

		out[sid] = o
	}
	return out, nil
}

// BuildFanoutScaledOrdersForAudit 稽核用：按父单与 fanout 份额映射生成各子期望订单（OriginalQty / ExecutedQty / 等），与 buildScaledOrdersForMultiBotFanout 一致。
// unitShares 为子账户 id -> 份额；engine 不可用时 mkt 为 nil，按默认精度做 floor，可能与线上略有差异。
func (e *Entity) BuildFanoutScaledOrdersForAudit(ctx context.Context, ex ctypes.Exchange, parent *ctypes.Order, unitShares map[string]decimal.Decimal) (map[string]ctypes.Order, error) {
	if parent == nil {
		return nil, fmt.Errorf("parent order is nil")
	}
	if len(unitShares) == 0 {
		return map[string]ctypes.Order{}, nil
	}
	disp := buildSubRawDispatchesFromUnitShares(*parent, unitShares)
	return e.buildScaledOrdersForMultiBotFanout(ctx, ex, parent, disp)
}

// FanoutSubOrderSkippedBelowMinStep 与 applyMultiBotParentOrderStage 一致：缩放并 normalize 后若 original 低于最小步长，则不会向该子派发订单流。
func (e *Entity) FanoutSubOrderSkippedBelowMinStep(ctx context.Context, ex ctypes.Exchange, scaled ctypes.Order) bool {
	return scaled.OriginalQty.LessThanOrEqual(decimal.Zero)
}
