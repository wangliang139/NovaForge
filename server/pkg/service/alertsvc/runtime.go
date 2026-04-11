package alertsvc

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/types"
)

const maxFiveMinuteSamples = 300

type marketPriceLookup interface {
	GetPriceAt(ctx context.Context, ex types.Exchange, symbol types.Symbol, ts time.Time, interval types.Interval) (decimal.Decimal, error)
}

type runtimeAlert struct {
	item        AlertItem
	inThreshold bool
	rearmReady  bool
	lastPrice   *decimal.Decimal
	pctState    *pctAlertState
}

type pctAlertState struct {
	samples             []priceSample
	lastEvaluatedSecond time.Time
}

type priceSample struct {
	ts    time.Time
	price decimal.Decimal
}

func newRuntimeAlert(item AlertItem) *runtimeAlert {
	ra := &runtimeAlert{
		item:       item,
		rearmReady: true,
	}
	if isPctAlertType(item.Type) && usesLocalPctSamples(item.Window) {
		ra.pctState = &pctAlertState{
			samples: make([]priceSample, 0, maxFiveMinuteSamples),
		}
	}
	return ra
}

func (ra *runtimeAlert) evaluate(ctx context.Context, lookup marketPriceLookup, now time.Time, price decimal.Decimal) (bool, *decimal.Decimal, bool) {
	switch ra.item.Type {
	case "price_reach":
		return ra.evaluatePriceReach(price)
	case "price_rise_to":
		return ra.evaluatePriceRiseTo(price)
	case "price_fall_to":
		return ra.evaluatePriceFallTo(price)
	case "price_rise_pct_over", "price_fall_pct_over":
		return ra.evaluatePctChange(ctx, lookup, now, price)
	default:
		return false, nil, false
	}
}

func (ra *runtimeAlert) evaluatePriceReach(price decimal.Decimal) (bool, *decimal.Decimal, bool) {
	if ra.item.Price == nil {
		return false, nil, false
	}
	target := *ra.item.Price
	lastPrice := ra.lastPrice
	tmp := price
	ra.lastPrice = &tmp
	if lastPrice == nil {
		return price.Equal(target), nil, true
	}
	if price.Equal(target) {
		return true, nil, true
	}
	upCross := lastPrice.LessThan(target) && (price.GreaterThan(target) || price.Equal(target))
	downCross := lastPrice.GreaterThan(target) && (price.LessThan(target) || price.Equal(target))
	return upCross || downCross, nil, true
}

func (ra *runtimeAlert) evaluatePriceRiseTo(price decimal.Decimal) (bool, *decimal.Decimal, bool) {
	if ra.item.Price == nil {
		return false, nil, false
	}
	target := *ra.item.Price
	lastPrice := ra.lastPrice
	tmp := price
	ra.lastPrice = &tmp
	if lastPrice == nil {
		return price.Equal(target), nil, true
	}
	return lastPrice.LessThan(target) && (price.GreaterThan(target) || price.Equal(target)), nil, true
}

func (ra *runtimeAlert) evaluatePriceFallTo(price decimal.Decimal) (bool, *decimal.Decimal, bool) {
	if ra.item.Price == nil {
		return false, nil, false
	}
	target := *ra.item.Price
	lastPrice := ra.lastPrice
	tmp := price
	ra.lastPrice = &tmp
	if lastPrice == nil {
		return price.Equal(target), nil, true
	}
	return lastPrice.GreaterThan(target) && (price.LessThan(target) || price.Equal(target)), nil, true
}

func (ra *runtimeAlert) evaluatePctChange(ctx context.Context, lookup marketPriceLookup, now time.Time, price decimal.Decimal) (bool, *decimal.Decimal, bool) {
	if ra.item.Percent == nil || ra.item.Window == nil {
		return false, nil, false
	}

	second := now.Truncate(time.Second)
	if usesLocalPctSamples(ra.item.Window) {
		if ra.pctState == nil {
			ra.pctState = &pctAlertState{
				samples: make([]priceSample, 0, maxFiveMinuteSamples),
			}
		}
		ra.pctState.record(second, price)
		if ra.pctState.lastEvaluatedSecond.Equal(second) {
			return false, nil, false
		}
		ra.pctState.lastEvaluatedSecond = second
	}

	baseline, ok := ra.resolvePctBaseline(ctx, lookup, second)
	if !ok || baseline == nil || baseline.IsZero() {
		return false, nil, false
	}

	pct := price.Sub(*baseline).Div(*baseline).Mul(decimal.NewFromInt(100))
	if ra.item.Type == "price_rise_pct_over" {
		return pct.GreaterThanOrEqual(*ra.item.Percent), baseline, true
	}
	return pct.LessThanOrEqual(ra.item.Percent.Neg()), baseline, true
}

func (ra *runtimeAlert) resolvePctBaseline(ctx context.Context, lookup marketPriceLookup, second time.Time) (*decimal.Decimal, bool) {
	if ra.item.Window == nil {
		return nil, false
	}
	dur, ok := alertWindowToDuration(*ra.item.Window)
	if !ok {
		return nil, false
	}
	cutoff := second.Add(-dur)

	if usesLocalPctSamples(ra.item.Window) && ra.pctState != nil {
		if baseline := ra.pctState.sampleAt(cutoff); baseline != nil {
			return baseline, true
		}
	}
	if lookup == nil {
		return nil, false
	}

	interval, ok := alertWindowToBaselineInterval(*ra.item.Window)
	if !ok {
		return nil, false
	}
	symbol, err := types.ParseSymbol(ra.item.Symbol)
	if err != nil {
		return nil, false
	}
	price, err := lookup.GetPriceAt(ctx, ra.item.Exchange, symbol, cutoff, interval)
	if err != nil || price.IsZero() {
		return nil, false
	}
	return &price, true
}

func (s *pctAlertState) record(second time.Time, price decimal.Decimal) {
	if len(s.samples) > 0 && s.samples[len(s.samples)-1].ts.Equal(second) {
		s.samples[len(s.samples)-1].price = price
	} else {
		s.samples = append(s.samples, priceSample{ts: second, price: price})
	}

	cutoff := second.Add(-5 * time.Minute)
	for len(s.samples) > 0 && s.samples[0].ts.Before(cutoff) {
		s.samples = s.samples[1:]
	}
	if len(s.samples) > maxFiveMinuteSamples {
		s.samples = s.samples[len(s.samples)-maxFiveMinuteSamples:]
	}
}

func (s *pctAlertState) sampleAt(second time.Time) *decimal.Decimal {
	for i := len(s.samples) - 1; i >= 0; i-- {
		if s.samples[i].ts.Equal(second) {
			price := s.samples[i].price
			return &price
		}
		if s.samples[i].ts.Before(second) {
			break
		}
	}
	return nil
}

func isPctAlertType(tp string) bool {
	return tp == "price_rise_pct_over" || tp == "price_fall_pct_over"
}

func usesLocalPctSamples(window *string) bool {
	return window != nil && *window == "5m"
}

func alertWindowToBaselineInterval(window string) (types.Interval, bool) {
	switch window {
	case "5m":
		return types.Interval1s, true
	case "1h", "4h", "24h":
		return types.Interval1m, true
	default:
		return "", false
	}
}
