package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wangliang139/llt-trade/server/pkg/action/model"
	"github.com/wangliang139/llt-trade/server/pkg/action/resolver"
)

type strategyIDIn struct {
	ID string `json:"id" jsonschema:"策略 ID"`
}

func RegisterStrategyTools(s *mcp.Server, rsv *resolver.Resolver) {
	// --- Strategy / Datasource queries ---

	mcp.AddTool(s, &mcp.Tool{Name: "llt_strategies", Description: "Query Strategies：策略列表（GraphQL Strategies）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.QueryStrategiesInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Strategies", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPStrategies(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_strategy", Description: "Query Strategy：按 id 查策略（GraphQL Strategy）"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in strategyIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "Strategy", false); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPStrategy(ctx, in.ID)
			return nil, out, err
		})

	// --- Strategy / Bot mutations ---

	mcp.AddTool(s, &mcp.Tool{Name: "llt_create_strategy", Description: "Mutation CreateStrategy"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.CreateStrategyInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "CreateStrategy", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPCreateStrategy(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_update_strategy", Description: "Mutation UpdateStrategy"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.UpdateStrategyInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "UpdateStrategy", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPUpdateStrategy(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_generate_strategy", Description: "Mutation GenerateStrategy：AI 生成策略草稿"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.GenerateStrategyInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "GenerateStrategy", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPGenerateStrategy(ctx, in)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_active_strategy", Description: "Mutation ActiveStrategy"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in strategyIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "ActiveStrategy", true); err != nil {
				return nil, nil, err
			}
			ok, err := rsv.MCPActiveStrategy(ctx, in.ID)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": ok}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_inactive_strategy", Description: "Mutation InactiveStrategy"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in strategyIDIn) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "InactiveStrategy", true); err != nil {
				return nil, nil, err
			}
			ok, err := rsv.MCPInactiveStrategy(ctx, in.ID)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": ok}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "llt_run_backtest", Description: "Mutation RunBacktest"},
		func(ctx context.Context, _ *mcp.CallToolRequest, in model.RunBacktestInput) (*mcp.CallToolResult, any, error) {
			if err := CheckGQLAccess(ctx, "RunBacktest", true); err != nil {
				return nil, nil, err
			}
			out, err := rsv.MCPRunBacktest(ctx, in)
			return nil, out, err
		})
}
