package account

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/precision"
	"github.com/wangliang139/NovaForge/server/pkg/repos/acct_snapshot"
	"github.com/wangliang139/NovaForge/server/pkg/repos/assets"
	"github.com/wangliang139/NovaForge/server/pkg/repos/positions"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/logger"
)

// AccountStateAtAssetKey 指定一条资产历史 floor 查询键（与 asset_snapshot 维度一致）。
type AccountStateAtAssetKey struct {
	Exchange   string
	WalletType ctypes.WalletType
	Asset      string
}

// AccountStateAtPositionKey 指定一条仓位历史 floor 查询键。
type AccountStateAtPositionKey struct {
	Exchange string
	Symbol   string
	Side     positions.PositionSide
}

// AccountStateAtFilter 在 asOf 时刻需要组装的键集合；空切片表示不查该类。
type AccountStateAtFilter struct {
	Assets    []AccountStateAtAssetKey
	Positions []AccountStateAtPositionKey
}

// AssetSnapshotAt 单键在 effective_ts <= asOf 下的最近一条快照；无行时 Found=false。
type AssetSnapshotAt struct {
	Key           AccountStateAtAssetKey
	Found         bool
	Total         decimal.Decimal
	Frozen        decimal.Decimal
	EffectiveTs   time.Time
	SnapshotRowID int64
}

// PositionSnapshotAt 单键在 effective_ts <= asOf 下的最近一条快照；无行时 Found=false。
type PositionSnapshotAt struct {
	Key           AccountStateAtPositionKey
	Found         bool
	Qty           decimal.Decimal
	EntryPrice    decimal.Decimal
	Leverage      int32
	EffectiveTs   time.Time
	SnapshotRowID int64
}

// AccountStateAtResult BuildAccountStateAt 的输出；Partial 表示任一请求键无历史行。
type AccountStateAtResult struct {
	AccountID string
	AsOf      time.Time
	Assets    []AssetSnapshotAt
	Positions []PositionSnapshotAt
	Partial   bool
}

// GetAccountAssetSnapshotAtOrBefore 对单 (exchange, wallet_type, asset) 做 floor 读取（§8.4）。
func (e *Entity) GetAccountAssetSnapshotAtOrBefore(ctx context.Context, accountID string, key AccountStateAtAssetKey, asOf time.Time) (*AssetSnapshotAt, error) {
	if accountID == "" || key.Exchange == "" || key.Asset == "" || !key.WalletType.Valid() {
		return nil, errors.New(errors.InvalidArgument, "account_id, exchange, asset and wallet_type are required")
	}
	row, err := e.db.AcctSnapshotRepo.GetAccountAssetSnapshotAtOrBefore(ctx, acct_snapshot.GetAccountAssetSnapshotAtOrBeforeParams{
		AccountID:   accountID,
		Exchange:    key.Exchange,
		Asset:       key.Asset,
		WalletType:  acct_snapshot.WalletType(key.WalletType),
		EffectiveTs: asOf,
	})
	if err != nil {
		return nil, err
	}
	out := &AssetSnapshotAt{Key: key, Found: false}
	if row == nil {
		return out, nil
	}
	out.Found = true
	out.Total = utils.Decimal.PgNumericToDecimal(row.Total)
	out.Frozen = utils.Decimal.PgNumericToDecimal(row.Frozen)
	out.EffectiveTs = row.EffectiveTs
	out.SnapshotRowID = row.ID
	return out, nil
}

// GetAccountPositionSnapshotAtOrBefore 对单 (exchange, symbol, side) 做 floor 读取（§8.4）。
func (e *Entity) GetAccountPositionSnapshotAtOrBefore(ctx context.Context, accountID string, key AccountStateAtPositionKey, asOf time.Time) (*PositionSnapshotAt, error) {
	if accountID == "" || key.Exchange == "" || key.Symbol == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id, exchange and symbol are required")
	}
	row, err := e.db.AcctSnapshotRepo.GetAccountPositionSnapshotAtOrBefore(ctx, acct_snapshot.GetAccountPositionSnapshotAtOrBeforeParams{
		AccountID:   accountID,
		Exchange:    key.Exchange,
		Symbol:      key.Symbol,
		Side:        acct_snapshot.PositionSide(key.Side),
		EffectiveTs: asOf,
	})
	if err != nil {
		return nil, err
	}
	out := &PositionSnapshotAt{Key: key, Found: false}
	if row == nil {
		return out, nil
	}
	out.Found = true
	out.Qty = utils.Decimal.PgNumericToDecimal(row.Qty)
	out.EntryPrice = utils.Decimal.PgNumericToDecimal(row.EntryPrice)
	out.Leverage = row.Leverage
	out.EffectiveTs = row.EffectiveTs
	out.SnapshotRowID = row.ID
	return out, nil
}

// BuildAccountStateAt 按 filter 中各键分别 AtOrBefore(asOf) 组装截面（§8.4）；任一键无行则 Partial=true。
func (e *Entity) BuildAccountStateAt(ctx context.Context, accountID string, asOf time.Time, filter AccountStateAtFilter) (*AccountStateAtResult, error) {
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if asOf.IsZero() {
		return nil, errors.New(errors.InvalidArgument, "asOf is required")
	}

	res := &AccountStateAtResult{
		AccountID: accountID,
		AsOf:      asOf,
		Assets:    make([]AssetSnapshotAt, 0, len(filter.Assets)),
		Positions: make([]PositionSnapshotAt, 0, len(filter.Positions)),
	}

	for _, k := range filter.Assets {
		row, err := e.GetAccountAssetSnapshotAtOrBefore(ctx, accountID, k, asOf)
		if err != nil {
			return nil, err
		}
		if row != nil && !row.Found {
			res.Partial = true
		}
		if row != nil {
			res.Assets = append(res.Assets, *row)
		}
	}

	for _, k := range filter.Positions {
		row, err := e.GetAccountPositionSnapshotAtOrBefore(ctx, accountID, k, asOf)
		if err != nil {
			return nil, err
		}
		if row != nil && !row.Found {
			res.Partial = true
		}
		if row != nil {
			res.Positions = append(res.Positions, *row)
		}
	}

	return res, nil
}



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
		Total:       precision.DecimalToPgNumeric(newTotal),
		Frozen:      precision.DecimalToPgNumeric(newFrozen),
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
		Qty:         precision.DecimalToPgNumeric(qty),
		EntryPrice:  precision.DecimalToPgNumeric(entry),
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
		Qty:         precision.DecimalToPgNumeric(qty),
		EntryPrice:  precision.DecimalToPgNumeric(entry),
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

// appendSnapshotRecordsAfterRefresh 在定时全量刷新后，强制追加一条资金/仓位快照历史（即使值未变化）。
func (e *Entity) appendSnapshotRecordsAfterRefresh(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	if err := e.appendAssetSnapshotsAfterRefresh(ctx, accountID, exchange); err != nil {
		return err
	}
	if err := e.appendPositionSnapshotsAfterRefresh(ctx, accountID, exchange); err != nil {
		return err
	}
	if err := e.patchZeroSnapshotsAfterRefresh(ctx, accountID, exchange); err != nil {
		return err
	}
	return nil
}

func (e *Entity) appendAssetSnapshotsAfterRefresh(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	scope := []ctypes.WalletType{}
	switch exchange {
	case ctypes.ExchangeBinance, ctypes.ExchangeBinanceTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeFund, ctypes.WalletTypeSpot, ctypes.WalletTypeFuture, ctypes.WalletTypeMargin}
	case ctypes.ExchangeOkx, ctypes.ExchangeOkxTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeTrade, ctypes.WalletTypeFund}
	}

	var (
		rows []*ctypes.Asset
		err  error
	)
	if len(scope) > 0 {
		rows, err = e.getAssetsByScope(ctx, accountID, scope)
	} else {
		rows, err = e.GetAssets(ctx, accountID)
	}
	if err != nil {
		return err
	}

	for _, a := range rows {
		if a == nil {
			continue
		}
		ts := a.UpdatedTs
		if ts.IsZero() {
			ts = time.Now()
		}
		total := precision.DecimalToPgNumeric(a.Balance)
		frozen := precision.DecimalToPgNumeric(a.Locked())
		if err := e.db.AcctSnapshotRepo.InsertAccountAssetSnapshot(ctx, acct_snapshot.InsertAccountAssetSnapshotParams{
			AccountID:   accountID,
			Exchange:    exchange.String(),
			WalletType:  acct_snapshot.WalletType(a.WalletType),
			Asset:       a.Code,
			Total:       total,
			Frozen:      frozen,
			EffectiveTs: ts,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Entity) appendPositionSnapshotsAfterRefresh(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	rows, err := e.GetPositions(ctx, accountID)
	if err != nil {
		return err
	}
	for _, p := range rows {
		if p == nil || p.Exchange != exchange || p.Symbol.Type != ctypes.MarketTypeFuture {
			continue
		}
		ts := p.UpdatedTs
		if ts.IsZero() {
			ts = time.Now()
		}
		if err := e.db.AcctSnapshotRepo.InsertAccountPositionSnapshot(ctx, acct_snapshot.InsertAccountPositionSnapshotParams{
			AccountID:   accountID,
			Exchange:    exchange.String(),
			Symbol:      p.Symbol.String(),
			Side:        acct_snapshot.PositionSide(p.Side),
			Qty:         precision.DecimalToPgNumeric(p.Amount),
			EntryPrice:  precision.DecimalToPgNumeric(p.EntryPrice),
			Leverage:    int32(p.Leverage),
			EffectiveTs: ts,
		}); err != nil {
			return err
		}
	}
	return nil
}

// patchZeroSnapshotsAfterRefresh 根据快照末态与当前表状态补全归零点，避免历史曲线悬挂。
func (e *Entity) patchZeroSnapshotsAfterRefresh(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	if err := e.patchZeroAssetSnapshotsAfterRefresh(ctx, accountID, exchange); err != nil {
		return err
	}
	if err := e.patchZeroPositionSnapshotsAfterRefresh(ctx, accountID, exchange); err != nil {
		return err
	}
	return nil
}

func (e *Entity) patchZeroAssetSnapshotsAfterRefresh(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	latestRows, err := e.db.AcctSnapshotRepo.ListLatestAccountAssetSnapshotsAtOrBefore(
		ctx,
		acct_snapshot.ListLatestAccountAssetSnapshotsAtOrBeforeParams{
			AccountID:   accountID,
			Exchange:    exchange.String(),
			EffectiveTs: time.Now(),
		},
	)
	if err != nil {
		return err
	}

	currentRows, err := e.db.AssetsRepo.ListAssetsByAccount(ctx, accountID)
	if err != nil {
		return err
	}
	current := make(map[string]decimal.Decimal, len(currentRows))
	for _, row := range currentRows {
		if row.Exchange != exchange.String() {
			continue
		}
		key := row.Asset + ":" + string(row.WalletType)
		current[key] = utils.Decimal.PgNumericToDecimal(row.Total)
	}

	now := time.Now()
	for _, row := range latestRows {
		lastTotal := utils.Decimal.PgNumericToDecimal(row.Total)
		if !lastTotal.GreaterThan(decimal.Zero) {
			continue
		}
		key := row.Asset + ":" + string(row.WalletType)
		curTotal, ok := current[key]
		if ok && curTotal.GreaterThan(decimal.Zero) {
			continue
		}
		if err := e.db.AcctSnapshotRepo.InsertAccountAssetSnapshot(ctx, acct_snapshot.InsertAccountAssetSnapshotParams{
			AccountID:   accountID,
			Exchange:    exchange.String(),
			WalletType:  row.WalletType,
			Asset:       row.Asset,
			Total:       precision.DecimalToPgNumeric(decimal.Zero),
			Frozen:      precision.DecimalToPgNumeric(decimal.Zero),
			EffectiveTs: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Entity) patchZeroPositionSnapshotsAfterRefresh(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	latestRows, err := e.db.AcctSnapshotRepo.ListLatestAccountPositionSnapshotsAtOrBefore(
		ctx,
		acct_snapshot.ListLatestAccountPositionSnapshotsAtOrBeforeParams{
			AccountID:   accountID,
			Exchange:    exchange.String(),
			EffectiveTs: time.Now(),
		},
	)
	if err != nil {
		return err
	}

	currentRows, err := e.db.PositionsRepo.ListPositionsByAccountAndExchange(ctx, positions.ListPositionsByAccountAndExchangeParams{
		AccountID: accountID,
		Exchange:  exchange.String(),
	})
	if err != nil {
		return err
	}
	current := make(map[string]decimal.Decimal, len(currentRows))
	for _, row := range currentRows {
		key := row.Symbol + ":" + string(row.Side)
		current[key] = utils.Decimal.PgNumericToDecimal(row.Qty)
	}

	now := time.Now()
	for _, row := range latestRows {
		lastQty := utils.Decimal.PgNumericToDecimal(row.Qty)
		if !lastQty.GreaterThan(decimal.Zero) {
			continue
		}
		key := row.Symbol + ":" + string(row.Side)
		curQty, ok := current[key]
		if ok && curQty.GreaterThan(decimal.Zero) {
			continue
		}
		if err := e.db.AcctSnapshotRepo.InsertAccountPositionSnapshot(ctx, acct_snapshot.InsertAccountPositionSnapshotParams{
			AccountID:   accountID,
			Exchange:    exchange.String(),
			Symbol:      row.Symbol,
			Side:        row.Side,
			Qty:         precision.DecimalToPgNumeric(decimal.Zero),
			EntryPrice:  precision.DecimalToPgNumeric(decimal.Zero),
			Leverage:    row.Leverage,
			EffectiveTs: now,
		}); err != nil {
			return err
		}
	}
	return nil
}
