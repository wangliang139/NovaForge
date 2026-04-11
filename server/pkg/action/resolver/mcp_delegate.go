package resolver

import (
	"context"

	"github.com/wangliang139/llt-trade/server/pkg/action/model"
)

// MCP* 方法供 MCP 网关复用与 GraphQL 相同的 resolver 逻辑（鉴权、参数校验、Service 调用）。

func (r *Resolver) MCPAccounts(ctx context.Context, in model.QueryAccountsInput) (*model.AccountsConnection, error) {
	return (&queryResolver{r}).Accounts(ctx, in)
}

func (r *Resolver) MCPEquitys(ctx context.Context, in model.QueryEquitysInput) ([]*model.Equity, error) {
	return (&queryResolver{r}).Equitys(ctx, in)
}

func (r *Resolver) MCPAccountEventFlow(ctx context.Context, in model.QueryAccountEventFlowInput) (*model.AccountEventFlowConnection, error) {
	return (&queryResolver{r}).AccountEventFlow(ctx, in)
}

func (r *Resolver) MCPLeverage(ctx context.Context, accountID, symbol string) (int, error) {
	return (&queryResolver{r}).Leverage(ctx, accountID, symbol)
}

func (r *Resolver) MCPAccountMetrics(ctx context.Context, in model.QueryAccountMetricsInput) (*model.AccountMetrics, error) {
	return (&queryResolver{r}).AccountMetrics(ctx, in)
}

func (r *Resolver) MCPRiskEvents(ctx context.Context, in model.QueryRiskEventsInput) ([]*model.RiskEvent, error) {
	return (&queryResolver{r}).RiskEvents(ctx, in)
}

func (r *Resolver) MCPBalance(ctx context.Context, in model.QueryBalanceInput) (*model.Balance, error) {
	return (&queryResolver{r}).Balance(ctx, in)
}

func (r *Resolver) MCPPositions(ctx context.Context, in model.QueryPositionsInput) ([]*model.Position, error) {
	return (&queryResolver{r}).Positions(ctx, in)
}

func (r *Resolver) MCPOrders(ctx context.Context, in model.QueryOrdersInput) (*model.OrdersConnection, error) {
	return (&queryResolver{r}).Orders(ctx, in)
}

func (r *Resolver) MCPEstimateOrder(ctx context.Context, in model.EstimateOrderInput) (*model.EstimateOrderResult, error) {
	return (&queryResolver{r}).EstimateOrder(ctx, in)
}

func (r *Resolver) MCPStrategies(ctx context.Context, in model.QueryStrategiesInput) (*model.StrategiesConnection, error) {
	return (&queryResolver{r}).Strategies(ctx, in)
}

func (r *Resolver) MCPStrategy(ctx context.Context, id string) (*model.Strategy, error) {
	return (&queryResolver{r}).Strategy(ctx, id)
}

func (r *Resolver) MCPDatasources(ctx context.Context, in model.QueryDatasourcesInput) (*model.DatasourcesConnection, error) {
	return (&queryResolver{r}).Datasources(ctx, in)
}

func (r *Resolver) MCPDatasource(ctx context.Context, id int) (*model.DataSource, error) {
	return (&queryResolver{r}).Datasource(ctx, id)
}

func (r *Resolver) MCPBots(ctx context.Context, in model.QueryBotsInput) (*model.BotsConnection, error) {
	return (&queryResolver{r}).Bots(ctx, in)
}

func (r *Resolver) MCPBot(ctx context.Context, id int) (*model.Bot, error) {
	return (&queryResolver{r}).Bot(ctx, id)
}

func (r *Resolver) MCPBotBalance(ctx context.Context, in model.QueryBotBalanceInput) (*model.Balance, error) {
	return (&queryResolver{r}).BotBalance(ctx, in)
}

func (r *Resolver) MCPBotPositions(ctx context.Context, in model.QueryBotPositionsInput) ([]*model.Position, error) {
	return (&queryResolver{r}).BotPositions(ctx, in)
}

func (r *Resolver) MCPBotState(ctx context.Context, in model.QueryBotStateInput) (*model.BotState, error) {
	return (&queryResolver{r}).BotState(ctx, in)
}

func (r *Resolver) MCPBotOrders(ctx context.Context, in model.QueryBotOrdersInput) (*model.BotOrdersConnection, error) {
	return (&queryResolver{r}).BotOrders(ctx, in)
}

func (r *Resolver) MCPBotLedger(ctx context.Context, in model.QueryBotLedgersInput) (*model.BotLedgerConnection, error) {
	return (&queryResolver{r}).BotLedger(ctx, in)
}

func (r *Resolver) MCPBotEquity(ctx context.Context, botID, startTs, endTs int) (*model.BotEquityConnection, error) {
	return (&queryResolver{r}).BotEquity(ctx, botID, startTs, endTs)
}

func (r *Resolver) MCPBotLogs(ctx context.Context, in model.QueryBotLogsInput) (*model.BotLogsConnection, error) {
	return (&queryResolver{r}).BotLogs(ctx, in)
}

func (r *Resolver) MCPBotSignalFlow(ctx context.Context, in model.QueryBotSignalFlowInput) (*model.BotSignalFlowConnection, error) {
	return (&queryResolver{r}).BotSignalFlow(ctx, in)
}

func (r *Resolver) MCPBotSignalStats(ctx context.Context, startTs, endTs int, botID *int) (*model.BotSignalStats, error) {
	return (&queryResolver{r}).BotSignalStats(ctx, startTs, endTs, botID)
}

func (r *Resolver) MCPBotMetrics(ctx context.Context, in model.QueryBotMetricsInput) (*model.BotMetrics, error) {
	return (&queryResolver{r}).BotMetrics(ctx, in)
}

func (r *Resolver) MCPCreateAccount(ctx context.Context, in model.MutationAccountInput) (*model.Account, error) {
	return (&mutationResolver{r}).CreateAccount(ctx, in)
}

func (r *Resolver) MCPUpdateAccount(ctx context.Context, in model.MutationAccountInput) (*model.Account, error) {
	return (&mutationResolver{r}).UpdateAccount(ctx, in)
}

func (r *Resolver) MCPOnlineAccount(ctx context.Context, id string) (*model.Account, error) {
	return (&mutationResolver{r}).OnlineAccount(ctx, id)
}

func (r *Resolver) MCPOfflineAccount(ctx context.Context, id string) (*model.Account, error) {
	return (&mutationResolver{r}).OfflineAccount(ctx, id)
}

func (r *Resolver) MCPDeleteAccount(ctx context.Context, id string) (bool, error) {
	return (&mutationResolver{r}).DeleteAccount(ctx, id)
}

func (r *Resolver) MCPRefreshAccountSnapshots(ctx context.Context, accountID string) (bool, error) {
	return (&mutationResolver{r}).RefreshAccountSnapshots(ctx, accountID)
}

func (r *Resolver) MCPPlaceOrder(ctx context.Context, in model.PlaceOrderInput) (*model.PlaceOrderResult, error) {
	return (&mutationResolver{r}).PlaceOrder(ctx, in)
}

func (r *Resolver) MCPCancelOrder(ctx context.Context, in model.CancelOrderInput) (bool, error) {
	return (&mutationResolver{r}).CancelOrder(ctx, in)
}

func (r *Resolver) MCPSetLeverage(ctx context.Context, accountID, symbol string, leverage int) (int, error) {
	return (&mutationResolver{r}).SetLeverage(ctx, accountID, symbol, leverage)
}

func (r *Resolver) MCPUpdateAccountRiskConfig(ctx context.Context, in model.UpdateAccountRiskConfigInput) (*model.Account, error) {
	return (&mutationResolver{r}).UpdateAccountRiskConfig(ctx, in)
}

func (r *Resolver) MCPCreateStrategy(ctx context.Context, in model.CreateStrategyInput) (*model.Strategy, error) {
	return (&mutationResolver{r}).CreateStrategy(ctx, in)
}

func (r *Resolver) MCPUpdateStrategy(ctx context.Context, in model.UpdateStrategyInput) (*model.Strategy, error) {
	return (&mutationResolver{r}).UpdateStrategy(ctx, in)
}

func (r *Resolver) MCPGenerateStrategy(ctx context.Context, in model.GenerateStrategyInput) (*model.GenerateStrategyResponse, error) {
	return (&mutationResolver{r}).GenerateStrategy(ctx, in)
}

func (r *Resolver) MCPDeleteStrategy(ctx context.Context, id string) (bool, error) {
	return (&mutationResolver{r}).DeleteStrategy(ctx, id)
}

func (r *Resolver) MCPActiveStrategy(ctx context.Context, id string) (bool, error) {
	return (&mutationResolver{r}).ActiveStrategy(ctx, id)
}

func (r *Resolver) MCPInactiveStrategy(ctx context.Context, id string) (bool, error) {
	return (&mutationResolver{r}).InactiveStrategy(ctx, id)
}

func (r *Resolver) MCPCreateDatasource(ctx context.Context, in model.CreateDatasourceInput) (*model.DataSource, error) {
	return (&mutationResolver{r}).CreateDatasource(ctx, in)
}

func (r *Resolver) MCPDeleteDatasource(ctx context.Context, id int) (bool, error) {
	return (&mutationResolver{r}).DeleteDatasource(ctx, id)
}

func (r *Resolver) MCPRunBacktest(ctx context.Context, in model.RunBacktestInput) (*model.RunBacktestResponse, error) {
	return (&mutationResolver{r}).RunBacktest(ctx, in)
}

func (r *Resolver) MCPCreateBot(ctx context.Context, in model.CreateBotInput) (*model.Bot, error) {
	return (&mutationResolver{r}).CreateBot(ctx, in)
}

func (r *Resolver) MCPUpdateBot(ctx context.Context, in model.UpdateBotInput) (*model.Bot, error) {
	return (&mutationResolver{r}).UpdateBot(ctx, in)
}

func (r *Resolver) MCPStartBot(ctx context.Context, id int) (bool, error) {
	return (&mutationResolver{r}).StartBot(ctx, id)
}

func (r *Resolver) MCPStopBot(ctx context.Context, id int) (bool, error) {
	return (&mutationResolver{r}).StopBot(ctx, id)
}

func (r *Resolver) MCPUpgradeBot(ctx context.Context, id int) (*model.UpgradeBotResult, error) {
	return (&mutationResolver{r}).UpgradeBot(ctx, id)
}

func (r *Resolver) MCPDeleteBot(ctx context.Context, id int) (bool, error) {
	return (&mutationResolver{r}).DeleteBot(ctx, id)
}
