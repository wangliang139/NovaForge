package account

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/repos/positions"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
)

func TestPositionUpsertMeaningfulChange(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	prevTs := ts.Add(-time.Hour)
	lev := int32(5)

	t.Run("nil_row", func(t *testing.T) {
		if positionUpsertMeaningfulChange(nil) {
			t.Fatal("expected false")
		}
	})
	t.Run("new_row", func(t *testing.T) {
		row := &positions.UpsertPositionRow{
			PrevUpdatedTs: nil,
			Qty:           utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(1)),
		}
		if !positionUpsertMeaningfulChange(row) {
			t.Fatal("expected true")
		}
	})
	t.Run("qty_change", func(t *testing.T) {
		row := &positions.UpsertPositionRow{
			PrevUpdatedTs: &prevTs,
			PrevQty:       utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(1)),
			Qty:           utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(2)),
			PrevEntryPrice: utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(10)),
			EntryPrice:     utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(10)),
			PrevLeverage:   &lev,
			Leverage:       lev,
			UpdatedTs:      ts,
		}
		if !positionUpsertMeaningfulChange(row) {
			t.Fatal("expected true")
		}
	})
	t.Run("no_change", func(t *testing.T) {
		q := utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(1))
		row := &positions.UpsertPositionRow{
			PrevUpdatedTs:  &ts,
			PrevQty:        q,
			Qty:            q,
			PrevEntryPrice: q,
			EntryPrice:     q,
			PrevLeverage:   &lev,
			Leverage:       lev,
			UpdatedTs:      ts,
		}
		if positionUpsertMeaningfulChange(row) {
			t.Fatal("expected false")
		}
	})
	t.Run("stale_prev_qty_scan", func(t *testing.T) {
		row := &positions.UpsertPositionRow{
			PrevUpdatedTs: &prevTs,
			PrevQty:       pgtype.Numeric{Valid: false},
			Qty:           utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(1)),
			PrevEntryPrice: utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(10)),
			EntryPrice:     utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(10)),
			PrevLeverage:   &lev,
			Leverage:       lev,
			UpdatedTs:      ts,
		}
		if !positionUpsertMeaningfulChange(row) {
			t.Fatal("expected true when prev qty invalid/zero vs new qty")
		}
	})
}
