package account

import (
	"testing"
	"time"

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

func TestAssetWeightPickMultiBot(t *testing.T) {
	ts := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	live := decimal.RequireFromString("100")
	snap := decimal.RequireFromString("80")

	t.Run("zero_asOf_ignores_snap", func(t *testing.T) {
		got := assetWeightPickMultiBot(time.Time{}, true, snap, live)
		if !got.Equal(live) {
			t.Fatalf("got %v want live %v", got, live)
		}
	})
	t.Run("nonzero_asOf_snap_found", func(t *testing.T) {
		got := assetWeightPickMultiBot(ts, true, snap, live)
		if !got.Equal(snap) {
			t.Fatalf("got %v want snap %v", got, snap)
		}
	})
	t.Run("nonzero_asOf_no_snap_falls_back_live", func(t *testing.T) {
		got := assetWeightPickMultiBot(ts, false, snap, live)
		if !got.Equal(live) {
			t.Fatalf("got %v want live %v", got, live)
		}
	})
	t.Run("snap_negative_clamped", func(t *testing.T) {
		got := assetWeightPickMultiBot(ts, true, decimal.NewFromInt(-1), live)
		if !got.IsZero() {
			t.Fatalf("got %v want 0", got)
		}
	})
	t.Run("live_negative_clamped_when_fallback", func(t *testing.T) {
		got := assetWeightPickMultiBot(ts, false, snap, decimal.NewFromInt(-2))
		if !got.IsZero() {
			t.Fatalf("got %v want 0", got)
		}
	})
}
