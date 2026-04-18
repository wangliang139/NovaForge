package account

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	simconnector "github.com/wangliang139/NovaForge/server/pkg/market/connector/simulate"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

func (e *Entity) syncAllSimulateAccounts(ctx context.Context) {
	accts, err := e.db.AccountRepo.ListAccounts(ctx, accountrepo.AccountStatusOnline)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("list online accounts for simulate sync failed")
		return
	}
	for _, acct := range accts {
		if acct.AccountType != accountrepo.AccountTypeVirtual {
			continue
		}
		if err := e.syncOneSimulateAccount(ctx, acct.ID, false); err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", acct.ID).Msg("sync simulate account failed")
		}
	}
}

func (e *Entity) syncOneSimulateAccount(ctx context.Context, accountID string, remove bool) error {
	acct, err := e.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if acct == nil || acct.AccountType != ctypes.AccountTypeVirtual {
		return nil
	}
	conn, err := e.GetConnector(ctx, acct.Exchange, acct.ID)
	if err != nil {
		return err
	}
	simConn, ok := conn.(*simconnector.Connector)
	if !ok {
		return nil
	}
	if remove || acct.Status == ctypes.AccountStatusOffline {
		simConn.Close()
		return nil
	}

	assets, err := e.GetAssets(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("get assets: %w", err)
	}
	// Allowed DB wallet tags for simulate seeding: derived from spot vs futures market types.
	// On OKX, GetWalletType returns trade for both — this map collapses to a single entry.
	walletTypes := map[ctypes.WalletType]struct{}{
		ctypes.GetWalletType(acct.Exchange, ctypes.MarketTypeSpot):   {},
		ctypes.GetWalletType(acct.Exchange, ctypes.MarketTypeFuture): {},
	}
	bals := make(map[ctypes.WalletType]map[simconnector.Asset]decimal.Decimal)
	for _, a := range assets {
		if a == nil {
			continue
		}
		if _, ok := walletTypes[a.WalletType]; !ok {
			continue
		}
		m := bals[a.WalletType]
		if m == nil {
			m = make(map[simconnector.Asset]decimal.Decimal)
			bals[a.WalletType] = m
		}
		code := simconnector.Asset(a.Code)
		m[code] = m[code].Add(a.Balance)
	}
	if err := simConn.SeedAccountBalances(bals); err != nil {
		return fmt.Errorf("seed balances: %w", err)
	}

	posList, err := e.GetPositions(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("get positions: %w", err)
	}
	if err := simConn.SeedAccountPositions(posList); err != nil {
		return fmt.Errorf("seed positions: %w", err)
	}

	orders, err := e.GetOpenOrders(ctx, acct.ID, nil)
	if err != nil {
		return fmt.Errorf("get open orders: %w", err)
	}
	if err := simConn.SeedOpenOrders(orders); err != nil {
		return fmt.Errorf("seed orders: %w", err)
	}

	return nil
}
