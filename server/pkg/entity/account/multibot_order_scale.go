package account

import (
	"context"
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

func effectiveBasePlaces(m *ctypes.Market) int32 {
	if m != nil && m.BaseAssetPrecision > 0 {
		return int32(m.BaseAssetPrecision)
	}
	return int32(consts.DefaultAssetPrecision)
}

func effectiveQuotePlaces(m *ctypes.Market) int32 {
	if m != nil && m.QuoteAssetPrecision > 0 {
		return int32(m.QuoteAssetPrecision)
	}
	return int32(consts.DefaultAssetPrecision)
}

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

func floorToStep(t, step decimal.Decimal) decimal.Decimal {
	if !step.IsPositive() {
		return t
	}
	return t.Div(step).Floor().Mul(step)
}

func deriveQuoteFromBaseMap(
	baseMap map[string]decimal.Decimal,
	avgPrice decimal.Decimal,
	m *ctypes.Market,
) map[string]decimal.Decimal {
	out := make(map[string]decimal.Decimal, len(baseMap))
	for key, value := range baseMap {
		out[key] = value.Mul(avgPrice)
	}
	return out
}

func floorFieldShare(parentVal decimal.Decimal, row subShare, isFuture bool, kind scaleFieldKind, m *ctypes.Market) decimal.Decimal {
	t := parentVal.Mul(row.s)
	switch kind {
	case scaleFieldBaseQty:
		if isFuture {
			return floorToStep(t, marketLotStepBase(m))
		}
		return floorDecimalPlaces(t, effectiveBasePlaces(m))
	case scaleFieldQuoteQty:
		return floorDecimalPlaces(t, effectiveQuotePlaces(m))
	case scaleFieldMoney:
		return floorDecimalPlaces(t, int32(consts.DefaultAssetPrecision))
	default:
		return t
	}
}

// allocateFieldAmongSubs 将 parentVal 按 share 拆到各子；非 max 子向下取整（合约 base 按 lot，其余按精度），余量归 maxSub，且 sum(子) = parentVal*sum(shares)。
func allocateFieldAmongSubs(parentVal decimal.Decimal, shares []subShare, maxSub string, isFuture bool, kind scaleFieldKind, m *ctypes.Market) map[string]decimal.Decimal {
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
		out[shares[0].id] = floorFieldShare(parentVal, shares[0], isFuture, kind, m)
		return out
	}
	if parentVal.IsNegative() {
		for _, row := range shares {
			out[row.id] = parentVal.Mul(row.s)
		}
		return out
	}
	sumTicks := parentVal.Mul(sumShares)
	sumOthers := decimal.Zero
	for _, row := range shares {
		if row.id == maxSub {
			continue
		}
		q := floorFieldShare(parentVal, row, isFuture, kind, m)
		if q.IsNegative() {
			q = decimal.Zero
		}
		out[row.id] = q
		sumOthers = sumOthers.Add(q)
	}
	rem := sumTicks.Sub(sumOthers)
	if rem.IsNegative() {
		rem = decimal.Zero
	}
	out[maxSub] = rem
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

func maxShareSubAccountID(disp []SubRawDispatch) string {
	if len(disp) == 0 {
		return ""
	}
	best := ""
	bestShare := decimal.NewFromInt(-2)
	for _, d := range disp {
		if d.Share.GreaterThan(bestShare) || (d.Share.Equal(bestShare) && d.SubAccountID > best) {
			bestShare = d.Share
			best = d.SubAccountID
		}
	}
	return best
}

func (e *Entity) getMarketForOrderFanout(ctx context.Context, ex ctypes.Exchange, sym ctypes.Symbol) *ctypes.Market {
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

// buildScaledOrdersForMultiBotFanout 按 Share 与 Market 精度/lot 生成各子逻辑订单；合约 base 数量按 lot 向下取整后余量归 share 最大子。
func (e *Entity) buildScaledOrdersForMultiBotFanout(ctx context.Context, ex ctypes.Exchange, parent *ctypes.Order, disp []SubRawDispatch) (map[string]ctypes.Order, error) {
	if len(disp) == 0 || parent == nil {
		return map[string]ctypes.Order{}, nil
	}
	mkt := e.getMarketForOrderFanout(ctx, ex, parent.Symbol)
	isFut := parent.Symbol.Type == ctypes.MarketTypeFuture
	shares := sortedSubSharesFromDispatches(disp)
	maxSub := maxShareSubAccountID(disp)

	origMap := allocateFieldAmongSubs(parent.OriginalQty, shares, maxSub, isFut, scaleFieldBaseQty, mkt)
	execMap := allocateFieldAmongSubs(parent.ExecutedQty, shares, maxSub, isFut, scaleFieldBaseQty, mkt)
	origQuoteMap := allocateFieldAmongSubs(parent.OriginalQuoteQty, shares, maxSub, isFut, scaleFieldQuoteQty, mkt)
	execQuoteMap := allocateFieldAmongSubs(parent.ExecutedQuoteQty, shares, maxSub, isFut, scaleFieldQuoteQty, mkt)
	if parent.AvgPrice.IsPositive() {
		if parent.OriginalQty.IsPositive() {
			origQuoteMap = deriveQuoteFromBaseMap(origMap, parent.AvgPrice, mkt)
		}
		if parent.ExecutedQty.IsPositive() {
			execQuoteMap = deriveQuoteFromBaseMap(execMap, parent.AvgPrice, mkt)
		}
	}

	var feeMap, pnlMap map[string]decimal.Decimal
	if parent.Fee != nil {
		feeMap = allocateFieldAmongSubs(*parent.Fee, shares, maxSub, false, scaleFieldMoney, mkt)
	}
	if parent.RealizedPnl != nil {
		pnlMap = allocateFieldAmongSubs(*parent.RealizedPnl, shares, maxSub, false, scaleFieldMoney, mkt)
	}

	out := make(map[string]ctypes.Order, len(disp))
	for _, d := range disp {
		o := d.Order
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
		out[sid] = o
	}
	return out, nil
}
