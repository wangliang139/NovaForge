package account

import (
	"context"
	"slices"
	"time"

	"github.com/samber/lo"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// publishVsAcctSnapshotsFromDB 从子账户 DB 投影发布 BalanceSnapshot；可选发布合约 PositionSnapshot（现货不落仓位类事件）。
func (e *Entity) publishVsAcctSnapshotsFromDB(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	if e == nil || accountID == "" || !exchange.IsValid() {
		return nil
	}

	if err := e.publishVsAcctAssetSnapshots(ctx, accountID, exchange); err != nil {
		return err
	}

	if err := e.publishVsAcctPositionSnapshots(ctx, accountID, exchange); err != nil {
		return err
	}

	return nil
}

func (e *Entity) publishVsAcctAssetSnapshots(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	if e == nil || accountID == "" || !exchange.IsValid() {
		return nil
	}

	scope := []ctypes.WalletType{}
	switch exchange {
	case ctypes.ExchangeBinance, ctypes.ExchangeBinanceTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeFund, ctypes.WalletTypeSpot, ctypes.WalletTypeFuture, ctypes.WalletTypeMargin}
	case ctypes.ExchangeOkx, ctypes.ExchangeOkxTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeFund, ctypes.WalletTypeTrade}
	}

	var rows []*ctypes.Asset
	var err error
	if len(scope) > 0 {
		rows, err = e.getAssetsByScope(ctx, accountID, scope)
	} else {
		rows, err = e.GetAssets(ctx, accountID)
		if err != nil {
			return err
		}
		for _, a := range rows {
			if a == nil {
				continue
			}
			if !slices.Contains(scope, a.WalletType) {
				scope = append(scope, a.WalletType)
			}
		}
	}
	if err != nil {
		return err
	}

	ts := time.Now()
	snap := &ctypes.BalanceSnapshot{Scope: scope, Assets: make([]*ctypes.AssetEvent, 0, len(rows))}
	for _, a := range rows {
		if a == nil {
			continue
		}
		bal := a.Balance
		locked := a.Locked()
		snap.Assets = append(snap.Assets, &ctypes.AssetEvent{
			WalletType: a.WalletType,
			Code:       a.Code,
			Balance:    &bal,
			Locked:     &locked,
			UpdatedTs:  a.UpdatedTs,
		})
		if !a.UpdatedTs.IsZero() && a.UpdatedTs.After(ts) {
			ts = a.UpdatedTs
		}
	}

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}
	if err := e.PublishEvent(ctx, ctypes.NewMessage(exchange, selector, snap, ts)); err != nil {
		return err
	}

	return nil
}

func (e *Entity) publishVsAcctPositionSnapshots(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	if e == nil || accountID == "" || !exchange.IsValid() {
		return nil
	}

	ts := time.Now()

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}

	allPos, err := e.GetPositions(ctx, accountID)
	if err != nil {
		return err
	}
	fut := make([]*ctypes.Position, 0, len(allPos))
	for _, p := range allPos {
		if p != nil && p.Exchange == exchange && p.Symbol.Type == ctypes.MarketTypeFuture {
			fut = append(fut, p)
		}
	}
	posTs := ts
	for _, p := range fut {
		if p != nil && !p.UpdatedTs.IsZero() && p.UpdatedTs.After(posTs) {
			posTs = p.UpdatedTs
		}
	}
	posSnap := ctypes.PositionSnapshot{Positions: fut}
	if err := e.PublishEvent(ctx, ctypes.NewMessage(exchange, selector, posSnap, posTs)); err != nil {
		return err
	}
	return nil
}
