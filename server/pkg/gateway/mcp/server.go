package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wangliang139/NovaForge/server/pkg/action/resolver"
	"github.com/wangliang139/NovaForge/server/pkg/gateway/mcp/tools"
)

const (
	ServerName    = "novaforge"
	ServerVersion = "1.0.0"
)

func NewMCPServer(rsv *resolver.Resolver) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		Instructions: "NovaForge MCP：账户、市场只读（余额/持仓/订单）、策略与 Bot。",
	})
	tools.RegisterAccountTools(s, rsv)
	tools.RegisterStrategyTools(s, rsv)
	tools.RegisterBotTools(s, rsv)
	return s
}
