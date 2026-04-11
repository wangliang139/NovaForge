package account

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/bytedance/sonic"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stumble/wpgx"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/repos/assets"
	"github.com/wangliang139/llt-trade/server/pkg/repos/ledgers"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/utils"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/logger"
)

var MinDelta = decimal.RequireFromString("0.00000001")

// GetAssets 查询账户资产快照
func (e *Entity) GetAssets(ctx context.Context, accountID string) ([]*types.Asset, error) {
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "accountID is required")
	}
	list, err := e.db.AssetsRepo.ListAssetsByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	result := make([]*types.Asset, 0, len(list))
	for _, item := range list {
		asset, err := convertAssetRepo2Types(item)
		if err != nil {
			return nil, err
		}
		if asset.IsEmpty() {
			continue
		}
		result = append(result, asset)
	}
	return result, nil
}

func (e *Entity) getAssetsByScope(ctx context.Context, accountID string, scope []ctypes.WalletType) ([]*types.Asset, error) {
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "accountID is required")
	}
	list, err := e.db.AssetsRepo.ListAssetsByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	result := make([]*types.Asset, 0, len(list))
	for _, item := range list {
		asset, err := convertAssetRepo2Types(item)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(scope, asset.WalletType) {
			continue
		}
		if asset.IsEmpty() {
			continue
		}
		result = append(result, asset)
	}
	return result, nil
}

// ApplyAccountBalance 更新账户全量资产快照
func (e *Entity) ApplyAccountBalance(ctx context.Context, accountID string, exchange ctypes.Exchange, scope []ctypes.WalletType, balance *ctypes.Balance) error {
	keyFn := func(code string, walletType ctypes.WalletType) string {
		return code + ":" + string(walletType)
	}

	assets, err := e.getAssetsByScope(ctx, accountID, scope)
	if err != nil {
		return fmt.Errorf("get assets: %w", err)
	}
	closedAssets := make(map[string]struct {
		code       string
		walletType ctypes.WalletType
	})
	for _, asset := range assets {
		closedAssets[keyFn(asset.Code, asset.WalletType)] = struct {
			code       string
			walletType ctypes.WalletType
		}{code: asset.Code, walletType: asset.WalletType}
	}

	exchangeStr := exchange.String()

	// 收集所有资产变更用于批量发布事件
	type assetDelta struct {
		walletType  ctypes.WalletType
		code        string
		total       decimal.Decimal
		frozen      decimal.Decimal
		totalDelta  decimal.Decimal
		frozenDelta decimal.Decimal
		ts          time.Time
	}
	var deltas []assetDelta

	for _, asset := range balance.Assets {
		if asset == nil {
			continue
		}
		walletType := asset.WalletType
		if !walletType.Valid() {
			walletType = ctypes.WalletTypeFund
		}

		balance := asset.Balance
		frozen := asset.Locked
		// 币安资产的冻结/解冻由订单快照事件推导而来，无需重复落库
		if exchange.Base() == ctypes.ExchangeBinance {
			frozen = decimal.Zero
		}

		delete(closedAssets, keyFn(asset.Code, walletType))
		row, err := e.ApplyAssetSnapshot(ctx, accountID, exchange, walletType, asset.Code, &balance, &frozen, asset.UpdatedTs)
		if err != nil {
			return fmt.Errorf("apply asset snapshot for %s: %w", asset.Code, err)
		}

		// 计算 delta
		if row != nil {
			prevTotal := utils.Decimal.PgNumericToDecimal(row.PrevTotal)
			prevFrozen := utils.Decimal.PgNumericToDecimal(row.PrevFrozen)
			total := utils.Decimal.PgNumericToDecimal(row.Total)
			frozen := utils.Decimal.PgNumericToDecimal(row.Frozen)
			totalDelta := total.Sub(prevTotal)
			frozenDelta := frozen.Sub(prevFrozen)
			_totalDelta := totalDelta.String()
			_frozenDelta := frozenDelta.String()
			_ = _totalDelta
			_ = _frozenDelta

			// 只有当 delta 不为零时才记录
			if totalDelta.Abs().LessThan(MinDelta) && frozenDelta.Abs().LessThan(MinDelta) {
				continue
			}
			deltas = append(deltas, assetDelta{
				walletType:  walletType,
				code:        asset.Code,
				total:       total,
				frozen:      frozen,
				totalDelta:  totalDelta,
				frozenDelta: frozenDelta,
				ts:          asset.UpdatedTs,
			})
		}
	}

	for _, asset := range closedAssets {
		row, err := e.ApplyAssetSnapshot(ctx, accountID, exchange, asset.walletType, asset.code, &decimal.Zero, &decimal.Zero, time.Now())
		if err != nil {
			return fmt.Errorf("apply asset snapshot for %s: %w", asset.code, err)
		}

		// 清零资产也需要记录 delta
		if row != nil {
			prevTotal := utils.Decimal.PgNumericToDecimal(row.PrevTotal)
			prevFrozen := utils.Decimal.PgNumericToDecimal(row.PrevFrozen)
			if prevTotal.Abs().LessThan(MinDelta) && prevFrozen.Abs().LessThan(MinDelta) {
				continue
			}
			deltas = append(deltas, assetDelta{
				walletType:  asset.walletType,
				code:        asset.code,
				total:       decimal.Zero,
				frozen:      decimal.Zero,
				totalDelta:  prevTotal.Neg(),
				frozenDelta: prevFrozen.Neg(),
				ts:          time.Now(),
			})
		}
	}

	// 批量发布资产变更事件（转换为增量语义）
	if len(deltas) > 0 {
		assetEvents := make([]*ctypes.AssetEvent, 0, len(deltas))
		maxTs := time.Time{}

		for _, d := range deltas {
			assetEvents = append(assetEvents, &ctypes.AssetEvent{
				WalletType: d.walletType,
				Code:       d.code,
				Balance:    lo.ToPtr(d.totalDelta),
				Locked:     lo.ToPtr(d.frozenDelta),
				UpdatedTs:  d.ts,
			})
			if d.ts.After(maxTs) {
				maxTs = d.ts
			}
		}

		if maxTs.IsZero() {
			maxTs = time.Now()
		}

		outUpdate := &ctypes.BalanceUpdate{
			Type:   ctypes.UpdateTypeIncrement,
			Reason: ctypes.LedgerReasonSnapshot,
			Assets: assetEvents,
		}
		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccount,
			Account: lo.ToPtr(accountID),
		}
		msg := ctypes.NewMessage(exchange, selector, outUpdate, maxTs)
		if err := e.engine.Publish(ctx, msg); err != nil {
			return err
		}

		// 写入 ledger（异步）
		go func() {
			ctx := context.WithoutCancel(ctx)
			for _, d := range deltas {
				ledgerParams := ledgers.CreateLedgerEntryParams{
					AccountID:   accountID,
					Exchange:    exchangeStr,
					Asset:       d.code,
					WalletType:  ledgers.WalletType(d.walletType),
					Type:        string(ctypes.LedgerReasonSnapshot),
					TotalDelta:  utils.Decimal.DecimalToPgNumeric(d.totalDelta),
					FrozenDelta: utils.Decimal.DecimalToPgNumeric(d.frozenDelta),
					Ts:          d.ts,
					IsEffective: true,
				}
				if err := e.AppendLedger(ctx, ledgerParams); err != nil {
					logger.Ctx(ctx).Err(err).
						Str("account_id", accountID).
						Str("exchange", exchangeStr).
						Str("asset", d.code).
						Msg("failed to append ledger entry for balance snapshot")
				}
			}
		}()
	}

	return nil
}

// ApplyAssetSnapshot 全量更新单个资产快照
func (e *Entity) ApplyAssetSnapshot(ctx context.Context, accountID string, exchange ctypes.Exchange, walletType ctypes.WalletType, asset string, total, frozen *decimal.Decimal, ts time.Time) (*assets.UpsertAssetRow, error) {
	if accountID == "" || exchange == "" || asset == "" {
		return nil, errors.New(errors.InvalidArgument, "accountID, exchange and asset are required")
	}
	if !walletType.Valid() {
		return nil, errors.New(errors.InvalidArgument, "invalid wallet type")
	}
	if total == nil && frozen == nil {
		return nil, errors.New(errors.InvalidArgument, "total or frozen is required")
	}

	var totalNum pgtype.Numeric
	if total != nil {
		totalNum = utils.Decimal.DecimalToPgNumeric(*total)
	}
	var frozenNum pgtype.Numeric
	if frozen != nil {
		frozenNum = utils.Decimal.DecimalToPgNumeric(*frozen)
	}
	result, err := e.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		row, err := e.db.AssetsRepo.WithTx(tx).UpsertAsset(ctx, assets.UpsertAssetParams{
			AccountID:     accountID,
			Exchange:      exchange.String(),
			Asset:         asset,
			WalletType:    assets.WalletType(walletType),
			Total:         totalNum,
			Frozen:        frozenNum,
			OrderOccupied: pgtype.Numeric{Valid: false}, // 不覆盖
			AvgPrice:      pgtype.Numeric{Valid: false}, // 不覆盖
			LastUpdatedTs: ts,
		})
		if err != nil {
			return nil, err
		}

		// 资产变多时更新 avg_price
		if row != nil {
			total := utils.Decimal.PgNumericToDecimal(row.Total)
			prevTotal := utils.Decimal.PgNumericToDecimal(row.PrevTotal)
			totalDelta := total.Sub(prevTotal)
			avgPrice := utils.Decimal.PgNumericToDecimal(row.AvgPrice)
			if prevTotal.GreaterThan(decimal.Zero) && avgPrice.IsZero() {
				err := e.fillMissingAvgPrice(ctx, tx, accountID, exchange, asset, walletType)
				if err != nil {
					return nil, err
				}
			} else if totalDelta.GreaterThan(MinDelta) {
				err := e.updateAssetAvgPriceOnIncrease(ctx, tx, accountID, exchange, walletType, asset, totalDelta)
				if err != nil {
					return nil, err
				}
			}
		}
		return row, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*assets.UpsertAssetRow), nil
}

// updateAssetAvgPriceOnIncrease 资产变多时按 WAC 更新 avg_price（内部方法，失败时仅记录日志）
func (e *Entity) updateAssetAvgPriceOnIncrease(ctx context.Context, tx *wpgx.WTx, accountID string, exchange ctypes.Exchange, walletType ctypes.WalletType, asset string, totalDelta decimal.Decimal) error {
	provider := e.engine.GetMarketProvider()
	priceUsdt, err := provider.GetLastPrice(ctx, exchange, ctypes.NewSymbol(asset, "USDT", ctypes.MarketTypeSpot))
	if err != nil || !priceUsdt.GreaterThan(decimal.Zero) {
		logger.Ctx(ctx).Err(err).Str("asset", asset).Msg("get asset/USDT price for avg_price update")
		return nil
	}
	querier := e.db.AssetsRepo
	if tx != nil {
		querier = e.db.AssetsRepo.WithTx(tx)
	}
	_, err = querier.UpdateAssetAvgPriceOnIncrease(ctx, assets.UpdateAssetAvgPriceOnIncreaseParams{
		AccountID:  accountID,
		Asset:      asset,
		WalletType: assets.WalletType(walletType),
		TotalDelta: utils.Decimal.DecimalToPgNumeric(totalDelta),
		PriceUsdt:  utils.Decimal.DecimalToPgNumeric(priceUsdt),
	})
	return err
}

// fillMissingAvgPrice 对 total > 0 且 avg_price 缺失的资产，用当前 asset/USDT 价格补全
func (e *Entity) fillMissingAvgPrice(ctx context.Context, tx *wpgx.WTx, accountID string, exchange ctypes.Exchange, asset string, walletType ctypes.WalletType) error {
	provider := e.engine.GetMarketProvider()
	priceUsdt, err := provider.GetLastPrice(ctx, exchange, ctypes.NewSymbol(asset, "USDT", ctypes.MarketTypeSpot))
	if err != nil {
		logger.Ctx(ctx).Err(err).Str("asset", asset).Msg("get asset/USDT price for fill missing avg_price")
		return nil
	}
	if priceUsdt.LessThanOrEqual(decimal.Zero) {
		logger.Ctx(ctx).Err(err).Str("asset", asset).Msg("priceUsdt is zero")
		return nil
	}
	querier := e.db.AssetsRepo
	if tx != nil {
		querier = e.db.AssetsRepo.WithTx(tx)
	}
	_, err = querier.SetAssetAvgPrice(ctx, assets.SetAssetAvgPriceParams{
		AccountID:  accountID,
		Asset:      asset,
		WalletType: assets.WalletType(walletType),
		AvgPrice:   utils.Decimal.DecimalToPgNumeric(priceUsdt),
	})
	if err != nil {
		return err
	}
	return nil
}

// ApplyAssetIncrement 增量更新单个资产
func (e *Entity) ApplyAssetIncrement(ctx context.Context, accountID string, exchange ctypes.Exchange, walletType ctypes.WalletType, asset string, total, frozen *decimal.Decimal, ts time.Time) (*assets.Asset, error) {
	if accountID == "" || asset == "" {
		return nil, errors.New(errors.InvalidArgument, "accountID and asset are required")
	}
	if !exchange.IsValid() {
		return nil, errors.New(errors.InvalidArgument, "exchange is invalid")
	}
	if !walletType.Valid() {
		return nil, errors.New(errors.InvalidArgument, "invalid wallet type")
	}
	if total == nil && frozen == nil {
		return nil, errors.New(errors.InvalidArgument, "total or frozen is required")
	}

	var (
		totalNum  = pgtype.Numeric{Valid: false}
		frozenNum = pgtype.Numeric{Valid: false}
	)
	if total != nil {
		totalNum = utils.Decimal.DecimalToPgNumeric(*total)
	}
	if frozen != nil {
		frozenNum = utils.Decimal.DecimalToPgNumeric(*frozen)
	}
	result, err := e.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		row, err := e.db.AssetsRepo.WithTx(tx).IncrementAsset(ctx, assets.IncrementAssetParams{
			AccountID:     accountID,
			Asset:         asset,
			WalletType:    assets.WalletType(walletType),
			Total:         totalNum,
			Frozen:        frozenNum,
			LastUpdatedTs: ts,
		})
		if err != nil {
			return nil, err
		}

		if row != nil {
			newTotal := utils.Decimal.PgNumericToDecimal(row.Total)
			avgPrice := utils.Decimal.PgNumericToDecimal(row.AvgPrice)
			if newTotal.GreaterThan(decimal.Zero) && avgPrice.IsZero() {
				err := e.fillMissingAvgPrice(ctx, tx, accountID, exchange, asset, walletType)
				if err != nil {
					return nil, err
				}
			} else if total != nil && total.GreaterThan(MinDelta) {
				err := e.updateAssetAvgPriceOnIncrease(ctx, tx, accountID, exchange, walletType, asset, *total)
				if err != nil {
					return nil, err
				}
			}
		}

		return row, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*assets.Asset), nil
}

func (e *Entity) CheckAndApplyAssetOrderOccupiedUpdate(ctx context.Context, accountID string, exchange ctypes.Exchange, asset *ctypes.AssetEvent, reason ctypes.LedgerReason, detail any) error {
	return e.CheckAndApplyAssetOrderOccupiedUpdateWithTx(ctx, nil, accountID, exchange, asset, reason, detail)
}

func (e *Entity) CheckAndApplyAssetOrderOccupiedUpdateWithTx(ctx context.Context, tx *wpgx.WTx, accountID string, exchange ctypes.Exchange, asset *ctypes.AssetEvent, reason ctypes.LedgerReason, detail any) error {
	if asset == nil || asset.Locked == nil || asset.Locked.IsZero() {
		return nil
	}
	// 解冻不需要校验资金
	if asset.Locked.LessThan(decimal.Zero) {
		return e.ApplyAssetOrderOccupiedUpdateWithTx(ctx, tx, accountID, exchange, asset, reason, detail)
	}

	fn := func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		assetPo, err := e.db.AssetsRepo.GetAssetWithLock(ctx, assets.GetAssetWithLockParams{
			AccountID:  accountID,
			Asset:      asset.Code,
			WalletType: assets.WalletType(asset.WalletType),
		})
		if err != nil {
			return nil, err
		}
		if assetPo == nil {
			return nil, errors.New(errors.InvalidArgument, "asset not found")
		}
		total := utils.Decimal.PgNumericToDecimal(assetPo.Total)
		frozen := utils.Decimal.PgNumericToDecimal(assetPo.Frozen)
		orderOccupied := utils.Decimal.PgNumericToDecimal(assetPo.OrderOccupied)
		free := total.Sub(frozen).Sub(orderOccupied)
		if free.LessThan(*asset.Locked) {
			return nil, errors.New(errors.InvalidArgument, "insufficient balance")
		}
		err = e.ApplyAssetOrderOccupiedUpdateWithTx(ctx, tx, accountID, exchange, asset, reason, detail)
		return nil, err
	}

	var err error
	if tx == nil {
		_, err = e.db.ConnPool.Transact(ctx, pgx.TxOptions{}, fn)
	} else {
		_, err = fn(ctx, tx)
	}
	return err
}

func (e *Entity) ApplyAssetOrderOccupiedUpdate(ctx context.Context, accountID string, exchange ctypes.Exchange, asset *ctypes.AssetEvent, reason ctypes.LedgerReason, detail any) error {
	return e.ApplyAssetOrderOccupiedUpdateWithTx(ctx, nil, accountID, exchange, asset, reason, detail)
}

func (e *Entity) ApplyAssetOrderOccupiedUpdateWithTx(ctx context.Context, tx *wpgx.WTx, accountID string, exchange ctypes.Exchange, asset *ctypes.AssetEvent, reason ctypes.LedgerReason, detail any) error {
	if asset == nil {
		return nil
	}

	exchangeStr := exchange.String()
	walletType := asset.WalletType
	if !walletType.Valid() {
		walletType = ctypes.WalletTypeFund
	}

	ts := time.Now()
	if !asset.UpdatedTs.IsZero() {
		ts = asset.UpdatedTs
	}

	if asset.Locked == nil || asset.Locked.IsZero() {
		return errors.New(errors.InvalidArgument, "locked is zero")
	}

	detailBytes, err := sonic.Marshal(detail)
	if err != nil {
		return errors.New(errors.Internal, "failed to marshal detail")
	}

	ledgerParams := ledgers.CreateLedgerEntryParams{
		AccountID:   accountID,
		Exchange:    exchangeStr,
		Asset:       asset.Code,
		WalletType:  ledgers.WalletType(walletType),
		FrozenDelta: utils.Decimal.DecimalToPgNumeric(*asset.Locked),
		Type:        string(reason),
		Detail:      detailBytes,
		Ts:          ts,
		IsEffective: false,
	}

	querier := e.db.AssetsRepo
	if tx == nil {
		querier = e.db.AssetsRepo
	} else {
		querier = e.db.AssetsRepo.WithTx(tx)
	}

	assetPo, err := querier.IncrementOrderOccupied(ctx, assets.IncrementOrderOccupiedParams{
		AccountID:     accountID,
		Asset:         asset.Code,
		WalletType:    assets.WalletType(walletType),
		OrderOccupied: utils.Decimal.DecimalToPgNumeric(*asset.Locked),
	})
	if err != nil {
		return err
	}

	if assetPo != nil {
		frozen := utils.Decimal.PgNumericToDecimal(assetPo.Frozen)
		orderOccupied := utils.Decimal.PgNumericToDecimal(assetPo.OrderOccupied)
		locked := frozen.Sub(orderOccupied)

		ledgerParams.Total = assetPo.Total
		ledgerParams.Frozen = utils.Decimal.DecimalToPgNumeric(locked)
		ledgerParams.IsEffective = true
	}
	// 增量语义直接对外发布（Balance/Locked 均为 delta）
	outUpdate := &ctypes.BalanceUpdate{
		Type:   ctypes.UpdateTypeIncrement,
		Reason: reason,
		Assets: []*ctypes.AssetEvent{
			{
				WalletType: walletType,
				Code:       asset.Code,
				Locked:     asset.Locked,
				UpdatedTs:  ts,
			},
		},
		Detail: detailBytes,
	}
	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}
	msg := ctypes.NewMessage(exchange, selector, outUpdate, ts)
	if e.engine != nil {
		if err := e.engine.Publish(ctx, msg); err != nil {
			return err
		}
	}

	go func() {
		ctx := context.WithoutCancel(ctx)
		if err := e.AppendLedger(ctx, ledgerParams); err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", accountID).
				Str("exchange", exchangeStr).
				Str("asset", asset.Code).
				Str("type", string(walletType)).
				Msg("failed to append ledger entry")
		}
	}()

	return nil
}

func convertAssetRepo2Types(item assets.Asset) (*types.Asset, error) {
	walletType := ctypes.WalletType(item.WalletType)
	if !walletType.Valid() {
		return nil, errors.New(errors.InvalidArgument, "invalid wallet type")
	}
	total := utils.Decimal.PgNumericToDecimal(item.Total)
	frozen := utils.Decimal.PgNumericToDecimal(item.Frozen)
	orderOccupied := utils.Decimal.PgNumericToDecimal(item.OrderOccupied)
	return &types.Asset{
		AccountID:     item.AccountID,
		Code:          item.Asset,
		WalletType:    walletType,
		Balance:       total,
		Frozened:      frozen,
		OrderOccupied: orderOccupied,
		UpdatedTs:     item.LastUpdatedTs,
	}, nil
}
