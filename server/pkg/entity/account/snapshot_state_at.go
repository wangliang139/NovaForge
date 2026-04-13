package account

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/repos/acct_snapshot"
	"github.com/wangliang139/NovaForge/server/pkg/repos/positions"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/errors"
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
