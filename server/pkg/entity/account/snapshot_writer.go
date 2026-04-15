package account

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/repos/acct_snapshot"
	"github.com/wangliang139/NovaForge/server/pkg/repos/assets"
	"github.com/wangliang139/NovaForge/server/pkg/repos/positions"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/logger"
)

// positionUpsertMeaningfulChange 与 ApplyAccountPositions 中「是否对外广播」判定一致，用于决定是否写入仓位历史快照。
func positionUpsertMeaningfulChange(row *positions.UpsertPositionRow) bool {
	if row == nil {
		return false
	}
	if row.PrevUpdatedTs == nil {
		return true
	}
	prevQty := utils.Decimal.PgNumericToDecimal(row.PrevQty)
	qty := utils.Decimal.PgNumericToDecimal(row.Qty)
	prevEntry := utils.Decimal.PgNumericToDecimal(row.PrevEntryPrice)
	entry := utils.Decimal.PgNumericToDecimal(row.EntryPrice)

	if !qty.Equal(prevQty) || !entry.Equal(prevEntry) {
		return true
	}
	if !row.UpdatedTs.Equal(*row.PrevUpdatedTs) {
		return true
	}
	return false
}

func (e *Entity) recordAssetSnapshotIfChanged(
	ctx context.Context,
	accountID, exchange string,
	walletType assets.WalletType,
	asset string,
	prevTotal, prevFrozen, newTotal, newFrozen decimal.Decimal,
	effectiveTs time.Time,
) {
	d := func(a, b decimal.Decimal) bool { return a.Sub(b).Abs().LessThan(MinDelta) }
	if d(prevTotal, newTotal) && d(prevFrozen, newFrozen) {
		return
	}
	ts := effectiveTs
	if ts.IsZero() {
		ts = time.Now()
	}
	err := e.db.AcctSnapshotRepo.InsertAccountAssetSnapshot(ctx, acct_snapshot.InsertAccountAssetSnapshotParams{
		AccountID:   accountID,
		Exchange:    exchange,
		WalletType:  acct_snapshot.WalletType(walletType),
		Asset:       asset,
		Total:       utils.Decimal.DecimalToPgNumeric(newTotal),
		Frozen:      utils.Decimal.DecimalToPgNumeric(newFrozen),
		EffectiveTs: ts,
	})
	if err != nil {
		logger.Ctx(ctx).Err(err).
			Str("account_id", accountID).
			Str("exchange", exchange).
			Str("asset", asset).
			Str("wallet_type", string(walletType)).
			Msg("insert asset_snapshot")
	}
}

func (e *Entity) recordAssetSnapshotFromUpsertRow(ctx context.Context, row *assets.UpsertAssetRow) {
	if row == nil {
		return
	}
	prevT := utils.Decimal.PgNumericToDecimal(row.PrevTotal)
	prevF := utils.Decimal.PgNumericToDecimal(row.PrevFrozen)
	newT := utils.Decimal.PgNumericToDecimal(row.Total)
	newF := utils.Decimal.PgNumericToDecimal(row.Frozen)
	e.recordAssetSnapshotIfChanged(ctx, row.AccountID, row.Exchange, row.WalletType, row.Asset,
		prevT, prevF, newT, newF, row.LastUpdatedTs)
}

func (e *Entity) recordPositionSnapshotFromUpsertRow(ctx context.Context, row *positions.UpsertPositionRow) {
	if row == nil {
		return
	}
	ts := row.UpdatedTs
	if ts.IsZero() {
		ts = time.Now()
	}
	qty := utils.Decimal.PgNumericToDecimal(row.Qty)
	entry := utils.Decimal.PgNumericToDecimal(row.EntryPrice)
	err := e.db.AcctSnapshotRepo.InsertAccountPositionSnapshot(ctx, acct_snapshot.InsertAccountPositionSnapshotParams{
		AccountID:   row.AccountID,
		Exchange:    row.Exchange,
		Symbol:      row.Symbol,
		Side:        acct_snapshot.PositionSide(row.Side),
		Qty:         utils.Decimal.DecimalToPgNumeric(qty),
		EntryPrice:  utils.Decimal.DecimalToPgNumeric(entry),
		Leverage:    row.Leverage,
		EffectiveTs: ts,
	})
	if err != nil {
		logger.Ctx(ctx).Err(err).
			Str("account_id", row.AccountID).
			Str("symbol", row.Symbol).
			Str("side", string(row.Side)).
			Msg("insert position_snapshot")
	}
}

// recordPositionSnapshotFromPositionsRow 在 UpsertSymbolLeverage 等仅更新 positions 行但未返回 UpsertPositionRow 时使用。
func (e *Entity) recordPositionSnapshotFromPositionsRow(ctx context.Context, row *positions.Position, effectiveTs time.Time) {
	if row == nil {
		return
	}
	ts := effectiveTs
	if ts.IsZero() {
		ts = row.UpdatedTs
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	qty := utils.Decimal.PgNumericToDecimal(row.Qty)
	entry := utils.Decimal.PgNumericToDecimal(row.EntryPrice)
	err := e.db.AcctSnapshotRepo.InsertAccountPositionSnapshot(ctx, acct_snapshot.InsertAccountPositionSnapshotParams{
		AccountID:   row.AccountID,
		Exchange:    row.Exchange,
		Symbol:      row.Symbol,
		Side:        acct_snapshot.PositionSide(row.Side),
		Qty:         utils.Decimal.DecimalToPgNumeric(qty),
		EntryPrice:  utils.Decimal.DecimalToPgNumeric(entry),
		Leverage:    row.Leverage,
		EffectiveTs: ts,
	})
	if err != nil {
		logger.Ctx(ctx).Err(err).
			Str("account_id", row.AccountID).
			Str("symbol", row.Symbol).
			Str("side", string(row.Side)).
			Msg("insert position_snapshot from positions row")
	}
}
