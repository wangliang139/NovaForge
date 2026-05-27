package collectors

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

func TestCalculateMaxDrawdownHitsZero(t *testing.T) {
	equity := []stypes.EquityPoint{
		{Ts: time.Unix(1, 0), TotalNetValue: decimal.RequireFromString("100"), Symbols: nil},
		{Ts: time.Unix(2, 0), TotalNetValue: decimal.RequireFromString("50"), Symbols: nil},
		{Ts: time.Unix(3, 0), TotalNetValue: decimal.Zero, Symbols: nil},
	}
	dd := CalculateMaxDrawdown(equity)
	if dd < 0.99 || dd > 1.01 {
		t.Fatalf("expected max drawdown ~1.0 from peak to zero, got %v", dd)
	}
}
