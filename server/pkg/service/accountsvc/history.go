package accountsvc

import (
	"context"
	"strings"
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/repos/acct_snapshot"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/errors"
)

// 允许的最长查询区间（略大于自然月滚动窗口）
const maxAcctSnapshotHistorySpan = 35 * 24 * time.Hour

// AssetSnapshotHistoryPoint 资产快照曲线单点。
type AssetSnapshotHistoryPoint struct {
	TsMs  int
	Total string
}

// PositionSnapshotHistoryPoint 仓位快照曲线单点。
type PositionSnapshotHistoryPoint struct {
	TsMs       int
	Qty        string
	EntryPrice string
}

func validateSnapshotHistoryRange(start, end time.Time) error {
	if start.IsZero() || end.IsZero() {
		return errors.New(errors.InvalidArgument, "time range is required")
	}
	if end.Before(start) {
		return errors.New(errors.InvalidArgument, "endTsMs must be >= startTsMs")
	}
	if end.Sub(start) > maxAcctSnapshotHistorySpan {
		return errors.New(errors.InvalidArgument, "time range too large")
	}
	return nil
}

func positionSideToAcctSnap(s types.PositionSide) acct_snapshot.PositionSide {
	if s == types.PositionSideShort {
		return acct_snapshot.PositionSideSHORT
	}
	return acct_snapshot.PositionSideLONG
}

// ListAssetSnapshotHistory 读取 asset_snapshot 时间序列（闭区间）。
func (s *Service) ListAssetSnapshotHistory(
	ctx context.Context,
	accountID string,
	walletType types.WalletType,
	asset string,
	start, end time.Time,
) ([]AssetSnapshotHistoryPoint, error) {
	if len(accountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if !walletType.Valid() {
		return nil, errors.New(errors.InvalidArgument, "walletType is required")
	}
	asset = strings.TrimSpace(asset)
	if asset == "" {
		return nil, errors.New(errors.InvalidArgument, "asset is required")
	}
	if err := validateSnapshotHistoryRange(start, end); err != nil {
		return nil, err
	}
	acct, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.AcctSnapshotRepo.ListAccountAssetSnapshotsInRange(ctx, acct_snapshot.ListAccountAssetSnapshotsInRangeParams{
		AccountID:     accountID,
		Exchange:      acct.Exchange.String(),
		Asset:         asset,
		WalletType:    acct_snapshot.WalletType(walletType),
		EffectiveTs:   start,
		EffectiveTs_2: end,
	})
	if err != nil {
		return nil, err
	}
	out := make([]AssetSnapshotHistoryPoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, AssetSnapshotHistoryPoint{
			TsMs:  int(row.EffectiveTs.UnixMilli()),
			Total: utils.Decimal.PgNumericToDecimal(row.Total).String(),
		})
	}
	return out, nil
}

// ListPositionSnapshotHistory 读取 position_snapshot 时间序列（闭区间）。
func (s *Service) ListPositionSnapshotHistory(
	ctx context.Context,
	accountID string,
	symbol string,
	side types.PositionSide,
	start, end time.Time,
) ([]PositionSnapshotHistoryPoint, error) {
	if len(accountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}
	if !side.Valid() {
		return nil, errors.New(errors.InvalidArgument, "side is required")
	}
	if err := validateSnapshotHistoryRange(start, end); err != nil {
		return nil, err
	}
	acct, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.AcctSnapshotRepo.ListAccountPositionSnapshotsInRange(ctx, acct_snapshot.ListAccountPositionSnapshotsInRangeParams{
		AccountID:     accountID,
		Exchange:      acct.Exchange.String(),
		Symbol:        symbol,
		Side:          positionSideToAcctSnap(side),
		EffectiveTs:   start,
		EffectiveTs_2: end,
	})
	if err != nil {
		return nil, err
	}
	out := make([]PositionSnapshotHistoryPoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, PositionSnapshotHistoryPoint{
			TsMs:       int(row.EffectiveTs.UnixMilli()),
			Qty:        utils.Decimal.PgNumericToDecimal(row.Qty).String(),
			EntryPrice: utils.Decimal.PgNumericToDecimal(row.EntryPrice).String(),
		})
	}
	return out, nil
}
