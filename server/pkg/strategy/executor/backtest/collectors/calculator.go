package collectors

import (
	"math"

	"github.com/shopspring/decimal"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

// CalculateMaxDrawdown 计算最大回撤
func CalculateMaxDrawdown(equity []stypes.EquityPoint) float64 {
	if len(equity) < 2 {
		return 0
	}

	peak := decimal.Zero
	maxDD := decimal.Zero

	for _, p := range equity {
		nv, err := decimal.NewFromString(p.TotalNetValue.String())
		if err != nil || nv.LessThanOrEqual(decimal.Zero) {
			continue
		}
		if peak.IsZero() || nv.GreaterThan(peak) {
			peak = nv
			continue
		}
		dd := peak.Sub(nv).Div(peak) // 0..1
		if dd.GreaterThan(maxDD) {
			maxDD = dd
		}
	}

	f, _ := maxDD.Float64()
	return f
}

// CalculateSharpeRatio 计算夏普比率
func CalculateSharpeRatio(equity []stypes.EquityPoint) float64 {
	// 简化：用相邻点收益率计算，年化因子不引入（返回 per-series Sharpe）
	if len(equity) < 3 {
		return 0
	}

	rets := make([]float64, 0, len(equity)-1)

	var prev decimal.Decimal
	for _, p := range equity {
		nv, err := decimal.NewFromString(p.TotalNetValue.String())
		if err != nil || nv.LessThanOrEqual(decimal.Zero) {
			continue
		}
		if prev.IsZero() {
			prev = nv
			continue
		}
		r := nv.Div(prev).Sub(decimal.NewFromInt(1))
		rf, _ := r.Float64()
		rets = append(rets, rf)
		prev = nv
	}

	if len(rets) < 2 {
		return 0
	}

	mean := 0.0
	for _, r := range rets {
		mean += r
	}
	mean /= float64(len(rets))

	variance := 0.0
	for _, r := range rets {
		d := r - mean
		variance += d * d
	}
	variance /= float64(len(rets) - 1)
	if variance <= 0 {
		return 0
	}

	std := math.Sqrt(variance)
	return mean / std * math.Sqrt(float64(len(rets)))
}
