package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wangliang139/llt-trade/server/pkg/action/model"
	"github.com/wangliang139/llt-trade/server/pkg/action/resolver"
)

type botIDIn struct {
	ID int `json:"id" jsonschema:"Bot 数字 ID"`
}

type botEquityIn struct {
	BotID   int `json:"botId"`
	StartTs int `json:"startTs"`
	EndTs   int `json:"endTs"`
}

func RegisterBotTools(s *mcp.Server, rsv *resolver.Resolver) {
	// --- Bot queries ---

	mcp.AddTool(s, &mcp.Tool{Name: "llt_bots", Description: "Query Bots：Bot 列表（GraphQL Bots）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryBotsInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Bots", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBots(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_bot", Description: "Query Bot：单个 Bot（GraphQL Bot）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in botIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Bot", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBot(ctx, in.ID)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_bot_balance", Description: "Query BotBalance：查询Bot关联账户的资产信息"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryBotBalanceInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "BotBalance", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBotBalance(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_bot_positions", Description: "Query BotPositions：查询Bot关联账户的仓位信息"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryBotPositionsInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "BotPositions", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBotPositions(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_bot_state", Description: "Query BotState：查询Bot运行状态"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryBotStateInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "BotState", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBotState(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_bot_orders", Description: "Query BotOrders：查询Bot所属的订单列表"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryBotOrdersInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "BotOrders", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBotOrders(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_bot_ledger", Description: "Query BotLedger：查询Bot的资金流水"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryBotLedgersInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "BotLedger", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBotLedger(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_bot_equity", Description: "Query BotEquity：查询Bot的净值曲线"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in botEquityIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "BotEquity", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBotEquity(ctx, in.BotID, in.StartTs, in.EndTs)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_bot_metrics", Description: "Query BotMetrics：查询Bot的指标"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryBotMetricsInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "BotMetrics", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPBotMetrics(ctx, in)
			return nil, out, err
		})

	// --- Strategy / Bot mutations ---

	mcp.AddTool(s, &mcp.Tool{Name: "llt_create_bot", Description: "Mutation CreateBot"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.CreateBotInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "CreateBot", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPCreateBot(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_update_bot", Description: "Mutation UpdateBot"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.UpdateBotInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "UpdateBot", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPUpdateBot(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_start_bot", Description: "Mutation StartBot"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in botIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "StartBot", true); err != nil {
				return nil, nil, err
			}
			ok, err := rsv.MCPStartBot(ctx, in.ID)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": ok}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_stop_bot", Description: "Mutation StopBot"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in botIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "StopBot", true); err != nil {
				return nil, nil, err
			}
			ok, err := rsv.MCPStopBot(ctx, in.ID)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": ok}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_upgrade_bot", Description: "Mutation UpgradeBot"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in botIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "UpgradeBot", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPUpgradeBot(ctx, in.ID)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_delete_bot", Description: "Mutation DeleteBot"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in botIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "DeleteBot", true); err != nil {
				return nil, nil, err
			}
			ok, err := rsv.MCPDeleteBot(ctx, in.ID)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": ok}, nil
		})
}
