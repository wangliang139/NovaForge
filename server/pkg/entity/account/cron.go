package account

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

// RefreshAccountSnapshotsInput 刷新账户快照的输入参数
type RefreshAccountSnapshotsInput struct {
	AccountID string `json:"account_id,omitempty"` // 如果为空，则刷新所有在线账户
}

// RefreshAccountSnapshotsOutput 刷新账户快照的输出结果
type RefreshAccountSnapshotsOutput struct {
	AccountID      string `json:"account_id"`
	AssetsCount    int    `json:"assets_count"`
	PositionsCount int    `json:"positions_count"`
	OrdersCount    int    `json:"orders_count"`
	Success        bool   `json:"success"`
	Error          string `json:"error,omitempty"`
}

// RefreshAccountEquityInput 刷新账户权益快照的输入参数
type RefreshAccountEquityInput struct {
	AccountID string `json:"account_id,omitempty"` // 如果为空，则刷新所有在线账户
}

// RefreshAccountEquityOutput 刷新账户权益快照的输出结果
type RefreshAccountEquityOutput struct {
	AccountID        string `json:"account_id"`
	Notional         string `json:"notional"`
	UnRealizedProfit string `json:"unrealized_profit"`
	Success          bool   `json:"success"`
	Error            string `json:"error,omitempty"`
}

// AccountRiskCheckInput 定时账户风控检查的输入
type AccountRiskCheckInput struct {
	AccountID string `json:"account_id,omitempty"` // 如果为空，则检查所有在线账户
}

// AccountRiskCheckOutput 定时账户风控检查的输出
type AccountRiskCheckOutput struct {
	AccountID      string   `json:"account_id"`
	TriggeredRules []string `json:"triggered_rules,omitempty"`
	RiskIndex      string   `json:"risk_index,omitempty"`
	Success        bool     `json:"success"`
	Error          string   `json:"error,omitempty"`
	Action         *string  `json:"action,omitempty"`
}

// RefreshAccountSnapshots 定时刷新账户快照数据（资产/仓位/在途订单）
func (e *Entity) RefreshAccountSnapshots(ctx context.Context, input RefreshAccountSnapshotsInput) (*RefreshAccountSnapshotsOutput, error) {
	logger.Ctx(ctx).Info().Msgf("start refresh account snapshots, input: %+v", input)

	// 获取要刷新的账户列表
	var accounts []accountrepo.Account
	var err error
	if input.AccountID != "" {
		accountID := input.AccountID
		account, err := e.db.AccountRepo.GetById(ctx, accountID)
		if err != nil {
			return nil, fmt.Errorf("get account by id: %w", err)
		}
		if account == nil {
			return nil, fmt.Errorf("account not found: %s", accountID)
		}
		if account.Status != accountrepo.AccountStatusOnline {
			logger.Ctx(ctx).Info().Msgf("account %s is not online, skip", accountID)
			return &RefreshAccountSnapshotsOutput{
				AccountID: accountID,
				Success:   false,
				Error:     "account is not online",
			}, nil
		}
		accounts = []accountrepo.Account{*account}
	} else {
		// 获取所有在线账户
		accounts, err = e.db.AccountRepo.ListAccounts(ctx, accountrepo.AccountStatusOnline)
		if err != nil {
			return nil, fmt.Errorf("list online accounts: %w", err)
		}
	}

	if len(accounts) == 0 {
		logger.Ctx(ctx).Info().Msg("no online accounts found")
		return &RefreshAccountSnapshotsOutput{
			Success: true,
		}, nil
	}

	logger.Ctx(ctx).Info().Msgf("found %d online accounts to refresh", len(accounts))

	// 遍历每个账户，刷新快照
	for _, acc := range accounts {
		if acc.AccountType != accountrepo.AccountTypeReal {
			continue
		}
		time.Sleep(5 * time.Second)
		err := e.RefreshSingleAccountSnapshots(ctx, acc.ID)
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", acc.ID).Msg("failed to refresh account snapshots")
			continue
		}
	}

	return &RefreshAccountSnapshotsOutput{
		Success: true,
	}, nil
}

// RefreshAccountEquity 定时刷新账户权益快照（每小时）
func (e *Entity) RefreshAccountEquity(ctx context.Context, input RefreshAccountEquityInput) (*RefreshAccountEquityOutput, error) {
	logger.Ctx(ctx).Info().Msgf("start refresh account equity, input: %+v", input)

	var accounts []accountrepo.Account
	if input.AccountID != "" {
		account, err := e.db.AccountRepo.GetById(ctx, input.AccountID)
		if err != nil {
			return nil, fmt.Errorf("get account by id: %w", err)
		}
		if account == nil {
			return nil, fmt.Errorf("account not found: %s", input.AccountID)
		}
		if account.Status != accountrepo.AccountStatusOnline {
			logger.Ctx(ctx).Info().Msgf("account %s is not online, skip", input.AccountID)
			return &RefreshAccountEquityOutput{
				AccountID: input.AccountID,
				Success:   false,
				Error:     "account is not online",
			}, nil
		}
		accounts = []accountrepo.Account{*account}
	} else {
		var err error
		accounts, err = e.db.AccountRepo.ListAccounts(ctx, accountrepo.AccountStatusOnline)
		if err != nil {
			return nil, fmt.Errorf("list online accounts: %w", err)
		}
	}

	if len(accounts) == 0 {
		logger.Ctx(ctx).Info().Msg("no online accounts found")
		return &RefreshAccountEquityOutput{Success: true}, nil
	}

	ts := time.Now().Truncate(time.Hour)
	for _, acc := range accounts {
		exchange, err := ctypes.ParseExchange(string(acc.Exchange))
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", acc.ID).Msg("parse exchange failed")
			continue
		}
		notional, unrealized, err := e.CalculateAccountEquity(ctx, acc.ID, exchange)
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", acc.ID).Msg("calculate account equity failed")
			continue
		}
		_, err = e.CreateEquity(ctx, acc.ID, ts, notional, unrealized)
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", acc.ID).Msg("create equity failed")
			continue
		}
		// 同步刷新 symbol 级权益快照（与 equity 一起触发）
		if err := e.refreshSymbolEquity(ctx, acc.ID, string(acc.Exchange), ts); err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", acc.ID).Msg("refresh symbol equity failed")
		}
	}

	return &RefreshAccountEquityOutput{Success: true}, nil
}

// AccountRiskCheck 定时扫描在线账户，执行账户级风控检查（规则 3/4/6/7/8 + 风险指数）
func (e *Entity) AccountRiskCheck(ctx context.Context, input AccountRiskCheckInput) (*AccountRiskCheckOutput, error) {
	logger.Ctx(ctx).Info().Msgf("start account risk check, input: %+v", input)

	var accounts []*types.Account
	if input.AccountID != "" {
		account, err := e.GetAccount(ctx, input.AccountID)
		if err != nil {
			return nil, fmt.Errorf("get account by id: %w", err)
		}
		if account == nil {
			return nil, fmt.Errorf("account not found: %s", input.AccountID)
		}
		if account.Status != types.AccountStatusOnline {
			logger.Ctx(ctx).Info().Msgf("account %s is not online, skip", input.AccountID)
			return &AccountRiskCheckOutput{
				AccountID: input.AccountID,
				Success:   false,
				Error:     "account is not online",
			}, nil
		}
		accounts = []*types.Account{account}
	} else {
		var err error
		accounts, err = e.ListAccounts(ctx, types.AccountStatusOnline)
		if err != nil {
			return nil, fmt.Errorf("list online accounts: %w", err)
		}
	}

	if len(accounts) == 0 {
		logger.Ctx(ctx).Info().Msg("no online accounts found")
		return &AccountRiskCheckOutput{Success: true}, nil
	}

	var lastOutput *types.RiskCheckResult
	for _, acct := range accounts {
		out, err := e.riskChecker.CheckAccountRisk(ctx, acct)
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", acct.ID).Msg("account risk check failed")
			continue
		}
		if len(out.TriggeredRules) > 0 {
			logger.Ctx(ctx).Info().Msgf("account %s risk triggered: rules=%v riskIndex=%s", out.AccountID, out.TriggeredRules, out.RiskIndex)
		}
		lastOutput = out
	}

	if lastOutput == nil {
		return &AccountRiskCheckOutput{Success: true}, nil
	}
	return &AccountRiskCheckOutput{
		AccountID:      lastOutput.AccountID,
		TriggeredRules: lastOutput.TriggeredRules,
		RiskIndex:      lastOutput.RiskIndex,
		Success:        lastOutput.Success,
		Error:          lastOutput.Error,
		Action:         lastOutput.Action,
	}, nil
}

// RefreshSingleAccountSnapshots 刷新单个账户的快照数据
func (e *Entity) RefreshSingleAccountSnapshots(ctx context.Context, accountId string) error {
	acct, err := e.GetAccount(ctx, accountId)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}
	if acct == nil {
		return fmt.Errorf("account not found")
	}
	if acct.AccountType == types.AccountTypeVirtual {
		return fmt.Errorf("account type not supported")
	}

	conn, err := e.GetConnector(ctx, acct.Exchange, acct.ID)
	if err != nil {
		return fmt.Errorf("get connector: %w", err)
	}

	// 1. 刷新资产快照
	if acct.AccountType == types.AccountTypeReal {
		err = e.refreshAssets(ctx, conn, acct.ID, acct.Exchange)
		if err != nil {
			return fmt.Errorf("refresh assets failed: %v", err)
		}
	}

	// 2. 刷新持仓快照
	if acct.AccountType == types.AccountTypeReal {
		err = e.refreshPositions(ctx, conn, acct.ID, acct.Exchange)
		if err != nil {
			return fmt.Errorf("refresh positions failed: %v", err)
		}
	}

	// 3. 刷新在途订单快照
	_, err = e.refreshOrders(ctx, conn, acct)
	if err != nil {
		return fmt.Errorf("refresh orders failed: %v", err)
	}

	return nil
}

// refreshAssets 刷新资产快照
func (e *Entity) refreshAssets(ctx context.Context, conn mdtypes.Connector, accountID string, exchange ctypes.Exchange) error {
	balance, err := conn.Balance(ctx)
	if err != nil {
		return fmt.Errorf("get balance: %w", err)
	}

	if balance == nil {
		return nil
	}

	scope := []ctypes.WalletType{}
	switch exchange {
	case ctypes.ExchangeBinance, ctypes.ExchangeBinanceTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeFund, ctypes.WalletTypeSpot, ctypes.WalletTypeFuture, ctypes.WalletTypeMargin}
	case ctypes.ExchangeOkx, ctypes.ExchangeOkxTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeTrade, ctypes.WalletTypeFund}
	}
	if err := e.ApplyAccountBalance(ctx, accountID, exchange, scope, balance); err != nil {
		return err
	}

	var ts time.Time
	snapshot := ctypes.BalanceSnapshot{}
	switch exchange {
	case ctypes.ExchangeBinance, ctypes.ExchangeBinanceTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeSpot, ctypes.WalletTypeFuture, ctypes.WalletTypeMargin}
	case ctypes.ExchangeOkx, ctypes.ExchangeOkxTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeTrade}
	}
	for _, asset := range balance.Assets {
		snapshot.Assets = append(snapshot.Assets, &ctypes.AssetEvent{
			WalletType: asset.WalletType,
			Code:       asset.Code,
			Balance:    lo.ToPtr(asset.Balance),
			Locked:     lo.ToPtr(asset.Locked),
			UpdatedTs:  asset.UpdatedTs,
		})
		if asset.UpdatedTs.After(ts) {
			ts = asset.UpdatedTs
		}
	}

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}

	if err := e.PublishEvent(ctx, ctypes.NewMessage(exchange, selector, snapshot, ts)); err != nil {
		logger.Ctx(ctx).Err(err).Str("account_id", accountID).Str("exchange", exchange.String()).Msg("failed to publish balance snapshot")
	}

	return nil
}

// refreshPositions 刷新持仓快照
func (e *Entity) refreshPositions(ctx context.Context, conn mdtypes.Connector, accountID string, exchange ctypes.Exchange) error {
	// 刷新期货持仓（包括永续合约）
	futuresPositions, err := conn.Positions(ctx, lo.ToPtr(ctypes.MarketTypeFuture))
	if err != nil {
		return fmt.Errorf("get futures positions: %w", err)
	}

	err = e.ApplyAccountPositions(ctx, accountID, exchange, futuresPositions, true)
	if err != nil {
		return fmt.Errorf("apply account positions: %w", err)
	}

	var ts time.Time
	for _, pos := range futuresPositions {
		if pos.UpdatedTs.After(ts) {
			ts = pos.UpdatedTs
		}
	}
	snapshot := ctypes.PositionSnapshot{
		Positions: futuresPositions,
	}
	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}
	if err := e.PublishEvent(ctx, ctypes.NewMessage(exchange, selector, snapshot, ts)); err != nil {
		logger.Ctx(ctx).Err(err).Str("account_id", accountID).Str("exchange", exchange.String()).Msg("failed to publish position snapshot")
	}

	return nil
}

// refreshOrders 刷新订单快照（不传 symbol，直接拉取账户当前所有订单）。
// P2 T5：仅由 RefreshSingleAccountSnapshots 对父 real 调用；与 WS 同源。
// multi_bot：先对 ordersList 全量仅父落库，再对成功项逐个执行子归因派发（与 handleOrderUpdate 顺序一致，仅批量摊平 RPC/锁竞争）。
func (e *Entity) refreshOrders(ctx context.Context, conn mdtypes.Connector, acct *types.Account) ([]*ctypes.Order, error) {
	if acct == nil {
		return nil, fmt.Errorf("account is nil")
	}
	accountID := acct.ID
	exchange := acct.Exchange

	var (
		ordersList []*ctypes.Order
		err        error
	)
	switch acct.AccountType {
	case types.AccountTypeReal:
		// GetOrders 支持 symbol 传 nil，表示获取当前账户下所有相关订单
		ordersList, err = conn.GetOrders(ctx, nil)
	case types.AccountTypeVirtualSub:
		// virtual_sub 无独立交易所连接：与 WS 一致，对父 real 拉取并落库（含 multi_bot 派发链）
		if acct.ParentAccountID == nil || *acct.ParentAccountID == "" {
			return nil, fmt.Errorf("virtual_sub account missing parent_account_id")
		}
		parent, err := e.GetAccount(ctx, *acct.ParentAccountID)
		if err != nil {
			return nil, fmt.Errorf("get parent account: %w", err)
		}
		if parent == nil {
			return nil, fmt.Errorf("parent account not found")
		}
		if parent.AccountType != types.AccountTypeReal {
			return nil, fmt.Errorf("virtual_sub parent must be a real account")
		}
		// 先同步一遍父账户
		parentOrders, err := e.refreshOrders(ctx, conn, parent)
		if err != nil {
			return nil, fmt.Errorf("refresh parent orders: %w", err)
		}
		if len(parentOrders) == 0 {
			return nil, nil
		}
		for _, ord := range parentOrders {
			if ord == nil {
				continue
			}
			dispatches, err := e.AttributeMultiBotOrderForFanout(ctx, parent.ID, parent.Exchange, ord)
			if err != nil {
				logger.Ctx(ctx).Err(err).
					Str("parent_account_id", parent.ID).
					Str("sub_account_id", accountID).
					Str("order_id", ord.OrderID.String()).
					Msg("attribute parent order for virtual_sub failed")
				continue
			}
			for _, d := range dispatches {
				if d.SubAccountID != accountID {
					continue
				}
				o := d.Order
				ordersList = append(ordersList, &o)
			}
		}
	default:
		return nil, fmt.Errorf("account type not supported")
	}

	existingOpenOrders := make(map[string]bool)
	for _, ord := range ordersList {
		if ord == nil {
			continue
		}

		existingOpenOrders[ord.OrderID.String()] = true

		if err := e.applyOrderPipeline(ctx, accountID, exchange, ord, true); err != nil {
			logger.Ctx(ctx).Err(err).
				Str("order_id", ord.OrderID.String()).
				Str("symbol", ord.Symbol.String()).
				Msg("failed to save parent order snapshot")
			continue
		}
	}

	pendingOrders, err := e.db.OrdersRepo.GetPendingOrders(ctx, orders.GetPendingOrdersParams{
		AccountID: accountID,
	})
	if err != nil {
		return nil, fmt.Errorf("get pending orders: %w", err)
	}
	for _, ord := range pendingOrders {
		if ord.UpdatedAt.After(time.Now().Add(-1*time.Minute)) || existingOpenOrders[ord.OrderID] {
			continue
		}
		symbol, err := ctypes.ParseSymbol(ord.Symbol)
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("order_id", ord.OrderID).Msg("failed to parse symbol")
			continue
		}
		order, err := conn.GetOrder(ctx, symbol, ord.OrderID)
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("order_id", ord.OrderID).Msg("failed to get order")
			continue
		}
		if order == nil {
			_, err := e.db.OrdersRepo.CancelOrderStatusWithReason(ctx, orders.CancelOrderStatusWithReasonParams{
				AccountID:    accountID,
				OrderID:      ord.OrderID,
				RejectReason: lo.ToPtr("not found in exchange"),
				FinishedTs:   lo.ToPtr(time.Now()),
				UpdatedTs:    time.Now(),
			})
			if err != nil {
				logger.Ctx(ctx).Err(err).Str("order_id", ord.OrderID).Msg("failed to cancel order status")
			}
		} else {
			if err := e.applyOrderPipeline(ctx, accountID, exchange, order, true); err != nil {
				logger.Ctx(ctx).Err(err).Str("order_id", ord.OrderID).Msg("failed to save order snapshot")
				continue
			}
		}
	}

	// 同步订单结束后：汇总当前所有在途订单的 locked，重置 assets.order_occupied
	// 以 DB 的订单状态为准（NEW/PENDING/WORKING/PARTIAL_DONE）
	if _, err := e.db.AssetsRepo.ResetOrderOccupiedByPendingOrders(ctx, accountID); err != nil {
		return nil, fmt.Errorf("reset order occupied by pending orders: %w", err)
	}

	return ordersList, nil
}
