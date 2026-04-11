package metrics

import (
	"math"

	"github.com/shopspring/decimal"
)

// EquityPoint 权益曲线点，用于指标计算
type EquityPoint struct {
	Ts       int64   // Unix 秒
	Notional float64 // 名义价值（BaseCurrency 计价）
}

// OrderForMetrics 订单简化结构，用于指标计算
type OrderForMetrics struct {
	RealizedPnl decimal.Decimal
	Fee         decimal.Decimal
	AvgPrice    decimal.Decimal
	Price       decimal.Decimal
	ExecutedQty decimal.Decimal
	OrderType   string // "LIMIT" | "MARKET"
	Symbol      string
	Exchange    string
}

// CalculateCAGR 年化复合收益率: (V_end/V_start)^(1/T)-1, T 为年数
func CalculateCAGR(equity []EquityPoint) float64 {
	if len(equity) < 2 {
		return 0
	}
	start := equity[0].Notional
	end := equity[len(equity)-1].Notional
	if start <= 0 {
		return 0
	}
	years := float64(equity[len(equity)-1].Ts-equity[0].Ts) / (365.25 * 24 * 3600)
	if years <= 0 {
		return 0
	}
	return math.Pow(end/start, 1/years) - 1
}

// CalculateSharpeRatio 夏普比率: mean(r)/std(r)*sqrt(N)
func CalculateSharpeRatio(equity []EquityPoint) float64 {
	if len(equity) < 3 {
		return 0
	}
	rets := make([]float64, 0, len(equity)-1)
	var prev float64
	for i, p := range equity {
		if p.Notional <= 0 {
			continue
		}
		if i == 0 {
			prev = p.Notional
			continue
		}
		r := (p.Notional/prev - 1)
		rets = append(rets, r)
		prev = p.Notional
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
	sharp := mean / std * math.Sqrt(float64(len(rets)))
	if math.IsNaN(sharp) {
		return -1
	}
	return sharp
}

// CalculateSortino 索提诺比率: mean(r)/std_down(r)，仅负收益标准差
func CalculateSortino(equity []EquityPoint) float64 {
	if len(equity) < 3 {
		return 0
	}
	rets := make([]float64, 0, len(equity)-1)
	var prev float64
	for i, p := range equity {
		if p.Notional <= 0 {
			continue
		}
		if i == 0 {
			prev = p.Notional
			continue
		}
		r := (p.Notional/prev - 1)
		rets = append(rets, r)
		prev = p.Notional
	}
	if len(rets) < 2 {
		return 0
	}
	mean := 0.0
	for _, r := range rets {
		mean += r
	}
	mean /= float64(len(rets))
	var sumSq float64
	var count int
	for _, r := range rets {
		if r < 0 {
			sumSq += r * r
			count++
		}
	}
	if count < 2 {
		return 0
	}
	stdDown := math.Sqrt(sumSq / float64(count-1))
	if stdDown <= 0 {
		return 0
	}
	sortino := mean / stdDown
	if math.IsInf(sortino, 1) {
		return -1
	}
	return sortino
}

// CalculateMaxDrawdown 最大回撤: max((Peak-NV)/Peak)
func CalculateMaxDrawdown(equity []EquityPoint) float64 {
	if len(equity) < 2 {
		return 0
	}
	peak := 0.0
	maxDD := 0.0
	for _, p := range equity {
		if p.Notional <= 0 {
			continue
		}
		if peak == 0 || p.Notional > peak {
			peak = p.Notional
			continue
		}
		dd := (peak - p.Notional) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}

// CalculateTimeUnderWater 回撤持续时间（秒）: max(t_recovery - t_peak)
func CalculateTimeUnderWater(equity []EquityPoint) int64 {
	if len(equity) < 2 {
		return 0
	}
	peak := 0.0
	peakTs := int64(0)
	maxTUW := int64(0)
	for _, p := range equity {
		if p.Notional <= 0 {
			continue
		}
		if p.Notional > peak {
			peak = p.Notional
			peakTs = p.Ts
			continue
		}
		if p.Notional < peak {
			tuw := p.Ts - peakTs
			if tuw > maxTUW {
				maxTUW = tuw
			}
		} else {
			peak = p.Notional
			peakTs = p.Ts
		}
	}
	return maxTUW
}

// CalculateCalmar 卡玛比率: CAGR / MDD，MDD=0 返回 0
func CalculateCalmar(equity []EquityPoint) float64 {
	cagr := CalculateCAGR(equity)
	mdd := CalculateMaxDrawdown(equity)
	if mdd <= 0 {
		return 0
	}
	return cagr / mdd
}

// CalculateRollingSharpe 滚动夏普：窗口内子序列的 Sharpe，取中位数
func CalculateRollingSharpe(equity []EquityPoint, window int) float64 {
	if len(equity) < window+2 {
		return CalculateSharpeRatio(equity)
	}
	sharps := make([]float64, 0, len(equity)-window)
	for i := 0; i <= len(equity)-window; i++ {
		sub := equity[i : i+window]
		s := CalculateSharpeRatio(sub)
		sharps = append(sharps, s)
	}
	if len(sharps) == 0 {
		return 0
	}
	sortFloats(sharps)
	return sharps[len(sharps)/2]
}

func sortFloats(a []float64) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[i] > a[j] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

// CalculateWinRate 胜率: WinTrades/(WinTrades+LossTrades)
func CalculateWinRate(orders []OrderForMetrics) float64 {
	if len(orders) == 0 {
		return 0
	}
	wins, losses := 0, 0
	for _, o := range orders {
		pnl, _ := o.RealizedPnl.Float64()
		if pnl > 0 {
			wins++
		} else if pnl < 0 {
			losses++
		}
	}
	total := wins + losses
	if total == 0 {
		return 0
	}
	return float64(wins) / float64(total)
}

// CalculateProfitFactor 盈亏比因子: GrossProfit / GrossLoss，GrossLoss=0 返回 999
func CalculateProfitFactor(orders []OrderForMetrics) float64 {
	grossProfit := 0.0
	grossLoss := 0.0
	for _, o := range orders {
		pnl, _ := o.RealizedPnl.Float64()
		if pnl > 0 {
			grossProfit += pnl
		} else if pnl < 0 {
			grossLoss += -pnl
		}
	}
	if grossLoss <= 0 {
		return 999
	}
	return grossProfit / grossLoss
}

// CalculateFeeRatio 手续费占比: TotalFee / GrossPnL
func CalculateFeeRatio(orders []OrderForMetrics) float64 {
	totalFee := 0.0
	grossPnl := 0.0
	for _, o := range orders {
		f, _ := o.Fee.Float64()
		totalFee += f
		p, _ := o.RealizedPnl.Float64()
		grossPnl += p
	}
	if grossPnl <= 0 {
		return 0
	}
	return totalFee / grossPnl
}

// CalculateMaxConsecutiveLoss 最大连续亏损笔数
func CalculateMaxConsecutiveLoss(orders []OrderForMetrics) int32 {
	max := 0
	cur := 0
	for _, o := range orders {
		pnl, _ := o.RealizedPnl.Float64()
		if pnl < 0 {
			cur++
			if cur > max {
				max = cur
			}
		} else {
			cur = 0
		}
	}
	return int32(max)
}

// CalculateAvgSlippage 平均滑点: 限价单 (avgPrice-price)/price*10000 bps，市价单不参与
func CalculateAvgSlippage(orders []OrderForMetrics) float64 {
	var sumSlippage float64
	var sumNotional float64
	for _, o := range orders {
		if o.OrderType != "LIMIT" {
			continue
		}
		if o.AvgPrice.IsZero() && o.Price.IsZero() {
			continue
		}
		if o.Price.IsZero() {
			continue
		}
		avg, _ := o.AvgPrice.Float64()
		price, _ := o.Price.Float64()
		qty, _ := o.ExecutedQty.Float64()
		if qty <= 0 {
			continue
		}
		notional := avg * qty
		slippageBps := (avg - price) / price * 10000
		sumSlippage += slippageBps * notional
		sumNotional += notional
	}
	if sumNotional <= 0 {
		return 0
	}
	return sumSlippage / sumNotional
}
