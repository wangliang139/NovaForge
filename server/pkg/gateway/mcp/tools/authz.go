package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/wangliang139/NovaForge/server/pkg/gateway/auth"
)

// CheckGQLAccess 对齐 GraphQL API Key 规则：[auth.CheckApiKeyPermission] + allowlist。
func CheckGQLAccess(ctx context.Context, gqlRootField string, isMutation bool) error {
	u, ok := auth.GetUserFromContext(ctx)
	if !ok || u == nil {
		return errors.New("unauthorized: send X-API-Key")
	}
	if u.Source != auth.AuthSourceAPIKey {
		return errors.New("unauthorized: mcp only supports X-API-Key")
	}
	if isMutation {
		if _, banned := auth.JWTOnlyMutations[gqlRootField]; banned {
			return fmt.Errorf("mutation %q not allowed for api key", gqlRootField)
		}
		if !u.APIKeyHasTrade {
			return errors.New("api key does not have trade permission")
		}
		if !auth.TradeMutationAllowed(gqlRootField) {
			return fmt.Errorf("mutation %q not allowed for api key", gqlRootField)
		}
		return nil
	}
	if _, banned := auth.JWTOnlyQueries[gqlRootField]; banned {
		return fmt.Errorf("query %q not allowed for api key", gqlRootField)
	}
	return nil
}
