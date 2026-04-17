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
	total decimal.Decimal,
) {
	if total.LessThan(decimal.Zero) {
		total = decimal.Zero
	}

	ts := time.Now()

	// 根据 快照表 的最新记录比对是否需要新增一条记录
	latestRow, err := e.db.AcctSnapshotRepo.GetAccountAssetSnapshotAtOrBefore(ctx, acct_snapshot.GetAccountAssetSnapshotAtOrBeforeParams{
		AccountID:   accountID,
		Exchange:    exchange,
		WalletType:  acct_snapshot.WalletType(walletType),
		Asset:       asset,
		EffectiveTs: ts,
	})
	if err != nil {
		logger.Ctx(ctx).Err(err).
			Str("account_id", accountID).
			Str("exchange", exchange).
			Str("asset", asset).
			Str("wallet_type", string(walletType)).
			Msg("get latest account asset snapshot")
		return
	}
	if latestRow != nil {
		latestTotal := utils.Decimal.PgNumericToDecimal(latestRow.Total)
		if latestTotal.Equal(total) {
			return
		}
	}

	err = e.db.AcctSnapshotRepo.InsertAccountAssetSnapshot(ctx, acct_snapshot.InsertAccountAssetSnapshotParams{
		AccountID:   accountID,
		Exchange:    exchange,
		WalletType:  acct_snapshot.WalletType(walletType),
		Asset:       asset,
		Total:       precision.DecimalToPgNumeric(total),
		EffectiveTs: ts,
	})
	if err != nil {
		logger.Ctx(ctx).Err(err).
			Str("account_id", accountID).
			Str("exchange", exchange).
			Str("asset", asset).
			Str("wallet_type", string(walletType)).
			Msg("insert asset_snapshot")
		return
	}
}

func (e *Entity) recordPositionSnapshotIfChanged(ctx context.Context,
	accountID string,
	exchange ctypes.Exchange,
	symbol string,
	side positions.PositionSide,
	qty decimal.Decimal,
	entry decimal.Decimal,
	leverage int32,
) {
	if qty.LessThan(decimal.Zero) {
		qty = decimal.Zero
		entry = decimal.Zero
	}

	ts := time.Now()

	latestRow, err := e.db.AcctSnapshotRepo.GetAccountPositionSnapshotAtOrBefore(ctx, acct_snapshot.GetAccountPositionSnapshotAtOrBeforeParams{
		AccountID:   accountID,
		Exchange:    exchange.String(),
		Symbol:      symbol,
		Side:        acct_snapshot.PositionSide(side),
		EffectiveTs: ts,
	})
	if err != nil {
		logger.Ctx(ctx).Err(err).
			Str("account_id", accountID).
			Str("exchange", exchange.String()).
			Str("symbol", symbol).
			Str("side", string(side)).
			Msg("get latest account position snapshot")
	}
	if latestRow != nil {
		latestQty := utils.Decimal.PgNumericToDecimal(latestRow.Qty)
		latestEntry := utils.Decimal.PgNumericToDecimal(latestRow.EntryPrice)
		latestLeverage := latestRow.Leverage
		if latestQty.Equal(qty) && latestEntry.Equal(entry) && latestLeverage == leverage {
			return
		}
	}

	err = e.db.AcctSnapshotRepo.InsertAccountPositionSnapshot(ctx, acct_snapshot.InsertAccountPositionSnapshotParams{
		AccountID:   accountID,
		Exchange:    exchange.String(),
		Symbol:      symbol,
		Side:        acct_snapshot.PositionSide(side),
		Qty:         precision.DecimalToPgNumeric(qty),
		EntryPrice:  precision.DecimalToPgNumeric(entry),
		Leverage:    leverage,
		EffectiveTs: ts,
	})
	if err != nil {
		logger.Ctx(ctx).Err(err).
			Str("account_id", accountID).
			Str("exchange", exchange.String()).
			Str("symbol", symbol).
			Str("side", string(side)).
			Msg("insert position_snapshot")
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

	now := time.Now()
	latestRows, err := e.db.AcctSnapshotRepo.ListLatestAccountAssetSnapshotsAtOrBefore(
		ctx,
		acct_snapshot.ListLatestAccountAssetSnapshotsAtOrBeforeParams{
			AccountID:   accountID,
			Exchange:    exchange.String(),
			EffectiveTs: now,
		},
	)
	if err != nil {
		return err
	}

	current := make(map[string]decimal.Decimal, len(rows))

	for _, row := range rows {
		if row == nil {
			continue
		}

		key := row.Code + ":" + string(row.WalletType)
		current[key] = row.Balance
		if row.Balance.LessThan(decimal.Zero) {
			current[key] = decimal.Zero
		}

		e.recordAssetSnapshotIfChanged(ctx, accountID, exchange.String(), assets.WalletType(row.WalletType), row.Code, row.Balance)
	}

	for _, row := range latestRows {
		lastTotal := utils.Decimal.PgNumericToDecimal(row.Total)
		if lastTotal.Equal(decimal.Zero) {
			continue
		}
		key := row.Asset + ":" + string(row.WalletType)
		if _, ok := current[key]; ok {
			continue
		}
		if err := e.db.AcctSnapshotRepo.InsertAccountAssetSnapshot(ctx, acct_snapshot.InsertAccountAssetSnapshotParams{
			AccountID:   accountID,
			Exchange:    exchange.String(),
			WalletType:  row.WalletType,
			Asset:       row.Asset,
			Total:       precision.DecimalToPgNumeric(decimal.Zero),
			EffectiveTs: now,
		}); err != nil {
			logger.Ctx(ctx).Err(err).
				Str("account_id", accountID).
				Str("exchange", exchange.String()).
				Str("asset", row.Asset).
				Str("wallet_type", string(row.WalletType)).
				Msg("insert asset_snapshot")
		}
	}

	return nil
}

func (e *Entity) appendPositionSnapshotsAfterRefresh(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	rows, err := e.GetPositions(ctx, accountID)
	if err != nil {
		return err
	}

	now := time.Now()

	latestRows, err := e.db.AcctSnapshotRepo.ListLatestAccountPositionSnapshotsAtOrBefore(
		ctx,
		acct_snapshot.ListLatestAccountPositionSnapshotsAtOrBeforeParams{
			AccountID:   accountID,
			Exchange:    exchange.String(),
			EffectiveTs: now,
		},
	)
	if err != nil {
		return err
	}

	current := make(map[string]decimal.Decimal, len(rows))

	for _, row := range rows {
		if row == nil || row.Exchange != exchange || row.Symbol.Type != ctypes.MarketTypeFuture {
			continue
		}
		key := row.Symbol.String() + ":" + string(row.Side)
		current[key] = row.Amount
		e.recordPositionSnapshotIfChanged(ctx, accountID, exchange, row.Symbol.String(), positions.PositionSide(row.Side), row.Amount, row.EntryPrice, int32(row.Leverage))
	}

	for _, row := range latestRows {
		lastQty := utils.Decimal.PgNumericToDecimal(row.Qty)
		if lastQty.Equal(decimal.Zero) {
			continue
		}
		key := row.Symbol + ":" + string(row.Side)
		if _, ok := current[key]; ok {
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
			logger.Ctx(ctx).Err(err).
				Str("account_id", accountID).
				Str("exchange", exchange.String()).
				Str("symbol", row.Symbol).
				Str("side", string(row.Side)).
				Msg("insert position_snapshot")
		}
	}

	return nil
}
