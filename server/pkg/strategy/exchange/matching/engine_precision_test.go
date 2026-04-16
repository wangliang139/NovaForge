package matching

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestDefaultMatchingConfig_DecimalRates(t *testing.T) {
	c := DefaultMatchingConfig()
	if c.MakerCommissionRate.IsZero() || c.TakerCommissionRate.IsZero() {
		t.Fatalf("expected non-zero default commission decimals")
	}
	if !c.MakerRate().Equal(c.MakerCommissionRate) {
		t.Fatalf("maker rate fallback: %s", c.MakerRate())
	}
	if !c.TakerRate().Equal(c.TakerCommissionRate) {
		t.Fatalf("taker rate fallback: %s", c.TakerRate())
	}
	if c.SlippageRate.IsZero() != true {
		t.Fatalf("default slippage should be zero")
	}
}

func TestMatchingConfig_MakerRateFallbackToCommission(t *testing.T) {
	c := MatchingConfig{
		MakerCommissionRate: decimal.Zero,
		TakerCommissionRate: decimal.Zero,
		CommissionRate:      decimal.RequireFromString("0.002"),
	}
	if !c.MakerRate().Equal(decimal.RequireFromString("0.002")) {
		t.Fatalf("got %s", c.MakerRate())
	}
	if !c.TakerRate().Equal(decimal.RequireFromString("0.002")) {
		t.Fatalf("got %s", c.TakerRate())
	}
}
