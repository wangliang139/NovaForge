package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wangliang139/NovaForge/server/pkg/action/model"
	"github.com/wangliang139/NovaForge/server/pkg/action/resolver"
)

// 工具名 nf_* 与 GraphQL 根字段映射见各 Tool.Description。

type leverageQueryIn struct {
	AccountID string `json:"accountId" jsonschema:"交易账户 ID"`
	Symbol    string `json:"symbol" jsonschema:"交易对"`
}

type accountIDIn struct {
	ID string `json:"id" jsonschema:"交易账户 ID"`
}

type refreshSnapshotsIn struct {
	AccountID string `json:"accountId" jsonschema:"交易账户 ID"`
}

type setLeverageIn struct {
	AccountID string `json:"accountId" jsonschema:"交易账户 ID"`
	Symbol    string `json:"symbol" jsonschema:"交易对"`
	Leverage  int    `json:"leverage" jsonschema:"杠杆倍数"`
}

func RegisterAccountTools(s *mcp.Server, rsv *resolver.Resolver) {
	mcp.AddTool(s, &mcp.Tool{Name: "nf_accounts", Description: "Query Accounts：分页查询交易账户（GraphQL Accounts）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryAccountsInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Accounts", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPAccounts(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_equitys", Description: "Query Equitys：账户权益时间序列（GraphQL Equitys，range: 1d/7d/30d）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryEquitysInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Equitys", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPEquitys(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_account_leverage", Description: "Query Leverage：查询账户在某 symbol 上的杠杆（GraphQL Leverage）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in leverageQueryIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Leverage", false); err != nil {
				return nil, nil, err
			}
			v, err := rsv.MCPLeverage(ctx, in.AccountID, in.Symbol)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"leverage": v}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_account_metrics", Description: "Query AccountMetrics：账户绩效指标（GraphQL AccountMetrics）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryAccountMetricsInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "AccountMetrics", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPAccountMetrics(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_risk_events", Description: "Query RiskEvents：风控事件列表（GraphQL RiskEvents）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryRiskEventsInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "RiskEvents", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPRiskEvents(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_balance", Description: "Query Balance：账户余额（GraphQL Balance）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryBalanceInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Balance", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBalance(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_positions", Description: "Query Positions：账户持仓（GraphQL Positions）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryPositionsInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Positions", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPPositions(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_orders", Description: "Query Orders：账户订单分页（GraphQL Orders）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryOrdersInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Orders", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPOrders(ctx, in)
			return nil, out, err
		})

	// --- Mutations (账户) ---

	mcp.AddTool(s, &mcp.Tool{Name: "nf_online_account", Description: "Mutation OnlineAccount：上线账户"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in accountIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "OnlineAccount", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPOnlineAccount(ctx, in.ID)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_offline_account", Description: "Mutation OfflineAccount：下线账户"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in accountIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "OfflineAccount", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPOfflineAccount(ctx, in.ID)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_refresh_account_snapshots", Description: "Mutation RefreshAccountSnapshots：刷新账户快照"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in refreshSnapshotsIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "RefreshAccountSnapshots", true); err != nil {
				return nil, nil, err
			}
			ok, err := rsv.MCPRefreshAccountSnapshots(ctx, in.AccountID)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": ok}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_place_order", Description: "Mutation PlaceOrder：下单（高风险）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.PlaceOrderInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "PlaceOrder", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPPlaceOrder(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_cancel_order", Description: "Mutation CancelOrder：撤单"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.CancelOrderInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "CancelOrder", true); err != nil {
				return nil, nil, err
			}
			ok, err := rsv.MCPCancelOrder(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": ok}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_set_leverage", Description: "Mutation SetLeverage：设置杠杆"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in setLeverageIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "SetLeverage", true); err != nil {
				return nil, nil, err
			}
			v, err := rsv.MCPSetLeverage(ctx, in.AccountID, in.Symbol, in.Leverage)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"leverage": v}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "nf_update_account_risk_config", Description: "Mutation UpdateAccountRiskConfig：更新账户风控配置"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.UpdateAccountRiskConfigInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "UpdateAccountRiskConfig", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPUpdateAccountRiskConfig(ctx, in)
			return nil, out, err
		})
}
