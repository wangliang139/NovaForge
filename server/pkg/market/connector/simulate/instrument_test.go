package simulate

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestFloorToTickStep(t *testing.T) {
	tick := decimal.RequireFromString("0.1")
	p := decimal.RequireFromString("100.15")
	got := FloorToTick(p, tick)
	want := decimal.RequireFromString("100.1")
	if !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
	step := decimal.RequireFromString("0.001")
	q := decimal.RequireFromString("1.2345")
	gotq := FloorToStep(q, step)
	wantq := decimal.RequireFromString("1.234")
	if !gotq.Equal(wantq) {
		t.Fatalf("got %s want %s", gotq, wantq)
	}
}

func TestValidateOrderParamsLimit(t *testing.T) {
	ins := &Instrument{
		MinQty:      decimal.NewFromInt(1),
		MinNotional: decimal.NewFromInt(10),
		PriceTick:   decimal.NewFromInt(1),
		QtyStep:     decimal.NewFromInt(1),
	}
	if err := ins.ValidateOrderParams(decimal.NewFromInt(5), decimal.NewFromInt(1), true); err != ErrBelowMinNotional {
		t.Fatalf("expected ErrBelowMinNotional got %v", err)
	}
}
