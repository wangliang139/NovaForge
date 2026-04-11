package mcp

import (
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wangliang139/NovaForge/server/pkg/action/resolver"
)

// HTTPHandler Streamable HTTP MCP 端点；与 /query 相同应在 OptionalAuthMiddleware 之后挂载。
func HTTPHandler(rsv *resolver.Resolver) http.Handler {
	srv := NewMCPServer(rsv)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return srv
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}
