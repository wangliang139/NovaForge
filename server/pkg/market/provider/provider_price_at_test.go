package provider

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

func TestAlignToIntervalStart(t *testing.T) {
	ts := time.Date(2026, 4, 10, 12, 34, 56, 789000000, time.UTC)
	got := alignToIntervalStart(ts, time.Minute)
	want := time.Date(2026, 4, 10, 12, 34, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("unexpected aligned time: got=%s want=%s", got, want)
	}
}

func TestPriceAtFromKlines(t *testing.T) {
	ts := time.Date(2026, 4, 10, 12, 34, 30, 0, time.UTC)
	bars := []*ctypes.Kline{
		{
			Interval: ctypes.Interval1m,
			Open:     decimal.NewFromInt(100),
			OpenTs:   time.Date(2026, 4, 10, 12, 34, 0, 0, time.UTC),
		},
	}
	price, ok := priceAtFromKlines(bars, ts, time.Minute)
	if !ok {
		t.Fatalf("expected to find price")
	}
	if !price.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("unexpected price: %s", price)
	}
}
