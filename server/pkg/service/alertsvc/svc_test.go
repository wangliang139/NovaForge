package alertsvc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/llt-trade/server/pkg/types"
)

type stubPriceLookup struct {
	price    decimal.Decimal
	err      error
	lastEx   types.Exchange
	lastSym  types.Symbol
	lastTs   time.Time
	lastIntv types.Interval
}

func (s *stubPriceLookup) GetPriceAt(_ context.Context, ex types.Exchange, symbol types.Symbol, ts time.Time, interval types.Interval) (decimal.Decimal, error) {
	s.lastEx = ex
	s.lastSym = symbol
	s.lastTs = ts
	s.lastIntv = interval
	if s.err != nil {
		return decimal.Zero, s.err
	}
	return s.price, nil
}

func TestValidateAddInput(t *testing.T) {
	price := "100"
	window := "1h"
	percent := "5"
	if err := validateAddInput(AddAlertInput{
		Exchange:  types.ExchangeBinance,
		Symbol:    "BTC/USDT",
		Type:      "price_rise_to",
		Frequency: "once",
		Price:     &price,
	}); err != nil {
		t.Fatalf("expected valid price alert, got error: %v", err)
	}
	if err := validateAddInput(AddAlertInput{
		Exchange:  types.ExchangeBinance,
		Symbol:    "BTC/USDT",
		Type:      "price_rise_pct_over",
		Frequency: "repeat",
		Window:    &window,
		Percent:   &percent,
	}); err != nil {
		t.Fatalf("expected valid pct alert, got error: %v", err)
	}
	if err := validateAddInput(AddAlertInput{
		Exchange:  types.ExchangeBinance,
		Symbol:    "BTC/USDT",
		Type:      "price_rise_to",
		Frequency: "repeat",
		Window:    &window,
		Percent:   &percent,
	}); err == nil {
		t.Fatalf("expected invalid combination to fail")
	}
}

func TestRuntimeAlertPriceReachCross(t *testing.T) {
	target := decimal.NewFromInt(100)
	now := time.Now()
	ra := newRuntimeAlert(AlertItem{
		Type:  "price_reach",
		Price: &target,
	})
	ra.lastPrice = loPtrDecimal(decimal.NewFromInt(99))
	met, _, evaluated := ra.evaluate(context.Background(), nil, now, decimal.NewFromInt(101))
	if !evaluated {
		t.Fatalf("expected evaluation to run")
	}
	if !met {
		t.Fatalf("expected cross to trigger")
	}
}

func TestRuntimeAlertPriceRiseToRequiresCross(t *testing.T) {
	target := decimal.NewFromInt(100)
	now := time.Now()
	ra := newRuntimeAlert(AlertItem{
		Type:  "price_rise_to",
		Price: &target,
	})
	ra.lastPrice = loPtrDecimal(decimal.NewFromInt(99))
	met, _, evaluated := ra.evaluate(context.Background(), nil, now, decimal.NewFromInt(101))
	if !evaluated {
		t.Fatalf("expected evaluation to run")
	}
	if !met {
		t.Fatalf("expected upward cross to trigger")
	}
}

func TestRuntimeAlertPriceRiseToDoesNotTriggerWithoutCross(t *testing.T) {
	target := decimal.NewFromInt(100)
	now := time.Now()
	ra := newRuntimeAlert(AlertItem{
		Type:  "price_rise_to",
		Price: &target,
	})
	ra.lastPrice = loPtrDecimal(decimal.NewFromInt(101))
	met, _, evaluated := ra.evaluate(context.Background(), nil, now, decimal.NewFromInt(102))
	if !evaluated {
		t.Fatalf("expected evaluation to run")
	}
	if met {
		t.Fatalf("did not expect rise_to to trigger without cross")
	}
}

func TestRuntimeAlertPriceFallToRequiresCross(t *testing.T) {
	target := decimal.NewFromInt(100)
	now := time.Now()
	ra := newRuntimeAlert(AlertItem{
		Type:  "price_fall_to",
		Price: &target,
	})
	ra.lastPrice = loPtrDecimal(decimal.NewFromInt(101))
	met, _, evaluated := ra.evaluate(context.Background(), nil, now, decimal.NewFromInt(99))
	if !evaluated {
		t.Fatalf("expected evaluation to run")
	}
	if !met {
		t.Fatalf("expected downward cross to trigger")
	}
}

func TestRuntimeAlertPriceFallToDoesNotTriggerWithoutCross(t *testing.T) {
	target := decimal.NewFromInt(100)
	now := time.Now()
	ra := newRuntimeAlert(AlertItem{
		Type:  "price_fall_to",
		Price: &target,
	})
	ra.lastPrice = loPtrDecimal(decimal.NewFromInt(99))
	met, _, evaluated := ra.evaluate(context.Background(), nil, now, decimal.NewFromInt(98))
	if !evaluated {
		t.Fatalf("expected evaluation to run")
	}
	if met {
		t.Fatalf("did not expect fall_to to trigger without cross")
	}
}

func TestRuntimeAlertPctRiseOverUsesLocalFiveMinuteSample(t *testing.T) {
	percent := decimal.NewFromInt(5)
	window := "5m"
	now := time.Now().Truncate(time.Second)
	ra := newRuntimeAlert(AlertItem{
		Type:    "price_rise_pct_over",
		Percent: &percent,
		Window:  &window,
	})
	ra.pctState.record(now.Add(-5*time.Minute), decimal.NewFromInt(100))
	met, baseline, evaluated := ra.evaluate(context.Background(), nil, now, decimal.NewFromInt(106))
	if !evaluated {
		t.Fatalf("expected evaluation to run")
	}
	if !met {
		t.Fatalf("expected pct rise to trigger")
	}
	if baseline == nil || !baseline.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("unexpected baseline: %v", baseline)
	}
	if ra.pctState == nil {
		t.Fatalf("expected 5m pct alert to keep local sample state")
	}
}

func TestRuntimeAlertPctRiseOverFallsBackToPriceLookup(t *testing.T) {
	percent := decimal.NewFromInt(5)
	window := "5m"
	now := time.Now().Truncate(time.Second)
	lookup := &stubPriceLookup{price: decimal.NewFromInt(100)}
	ra := newRuntimeAlert(AlertItem{
		Exchange: types.ExchangeBinance,
		Symbol:   "BTC/USDT",
		Type:     "price_rise_pct_over",
		Percent:  &percent,
		Window:   &window,
	})
	met, baseline, evaluated := ra.evaluate(context.Background(), lookup, now, decimal.NewFromInt(106))
	if !evaluated {
		t.Fatalf("expected evaluation to run")
	}
	if !met {
		t.Fatalf("expected pct rise to trigger")
	}
	if baseline == nil || !baseline.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("unexpected baseline: %v", baseline)
	}
	if lookup.lastIntv != types.Interval1s {
		t.Fatalf("expected 1s baseline lookup, got %s", lookup.lastIntv)
	}
	if !lookup.lastTs.Equal(now.Add(-5 * time.Minute)) {
		t.Fatalf("unexpected lookup ts: %s", lookup.lastTs)
	}
}

func TestRuntimeAlertPctFallOverUsesMinuteBaselineLookup(t *testing.T) {
	percent := decimal.NewFromInt(5)
	window := "1h"
	now := time.Now().Truncate(time.Second)
	lookup := &stubPriceLookup{price: decimal.NewFromInt(100)}
	ra := newRuntimeAlert(AlertItem{
		Exchange: types.ExchangeBinance,
		Symbol:   "BTC/USDT",
		Type:     "price_fall_pct_over",
		Percent:  &percent,
		Window:   &window,
	})
	if ra.pctState != nil {
		t.Fatalf("expected non-5m pct alert to skip local sample state")
	}
	met, baseline, evaluated := ra.evaluate(context.Background(), lookup, now, decimal.NewFromInt(94))
	if !evaluated {
		t.Fatalf("expected evaluation to run")
	}
	if !met {
		t.Fatalf("expected pct fall to trigger")
	}
	if baseline == nil || !baseline.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("unexpected baseline: %v", baseline)
	}
	if lookup.lastIntv != types.Interval1m {
		t.Fatalf("expected 1m baseline lookup, got %s", lookup.lastIntv)
	}
}

func TestRuntimeAlertPctSkipWhenLookupUnavailable(t *testing.T) {
	percent := decimal.NewFromInt(5)
	window := "1h"
	now := time.Now().Truncate(time.Second)
	lookup := &stubPriceLookup{err: context.DeadlineExceeded}
	ra := newRuntimeAlert(AlertItem{
		Exchange: types.ExchangeBinance,
		Symbol:   "BTC/USDT",
		Type:     "price_rise_pct_over",
		Percent:  &percent,
		Window:   &window,
	})
	met, baseline, evaluated := ra.evaluate(context.Background(), lookup, now, decimal.NewFromInt(106))
	if evaluated {
		t.Fatalf("expected evaluation to be skipped when baseline lookup fails")
	}
	if met || baseline != nil {
		t.Fatalf("unexpected result: met=%v baseline=%v", met, baseline)
	}
}

func TestBuildAlertPushMessage(t *testing.T) {
	price := decimal.NewFromInt(120)
	percent := decimal.NewFromInt(5)
	window := "1h"
	remark := "测试提醒"
	now := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	msg := buildAlertPushMessage(AlertItem{
		Exchange: types.ExchangeBinance,
		Symbol:   "BTC/USDT",
		Type:     "price_rise_pct_over",
		Price:    &price,
		Window:   &window,
		Percent:  &percent,
		Remark:   &remark,
	}, decimal.NewFromInt(126), loPtrDecimal(decimal.NewFromInt(120)), now)
	if !strings.Contains(msg, "🔔 BINANCE:BTC/USDT 价格预警") {
		t.Fatalf("unexpected message: %s", msg)
	}
	if !strings.Contains(msg, "价格 1h内 涨幅达 5%") {
		t.Fatalf("summary missing: %s", msg)
	}
	if !strings.Contains(msg, "备注: 测试提醒") {
		t.Fatalf("remark missing: %s", msg)
	}
}

func TestBuildAlertPushMessageFormatsPriceWithThousandsSeparator(t *testing.T) {
	price := decimal.RequireFromString("1234567.89")
	msg := buildAlertPushMessage(AlertItem{
		Exchange: types.ExchangeBinance,
		Symbol:   "BTC/USDT",
		Type:     "price_rise_to",
		Price:    &price,
	}, decimal.Zero, nil, time.Now())
	if !strings.Contains(msg, "价格上涨至 1,234,567.89") {
		t.Fatalf("price formatting missing: %s", msg)
	}
}

func loPtrDecimal(v decimal.Decimal) *decimal.Decimal {
	return &v
}
