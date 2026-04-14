package account

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestClampNonNegAssetTotal(t *testing.T) {
	if got := clampNonNegAssetTotal(decimal.NewFromInt(-3)); !got.IsZero() {
		t.Fatalf("negative clamp: %v", got)
	}
	x := decimal.RequireFromString("12.5")
	if got := clampNonNegAssetTotal(x); !got.Equal(x) {
		t.Fatalf("positive: %v", got)
	}
}
