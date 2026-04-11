package auth

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

// CheckApiKeyPermission 限制 API Key 可调用的 GraphQL 操作（JWT 不受影响）。
func CheckApiKeyPermission(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	u, ok := GetUserFromContext(ctx)
	if !ok || u.Source != AuthSourceAPIKey {
		return next(ctx)
	}

	oc := graphql.GetOperationContext(ctx)
	if oc == nil || oc.Operation == nil {
		return next(ctx)
	}
	op := oc.Operation

	switch op.Operation {
	case ast.Subscription:
		return func(ctx context.Context) *graphql.Response {
			return graphql.ErrorResponse(ctx, "api key cannot access subscriptions")
		}
	case ast.Mutation:
		names := rootFieldNames(op)
		for _, n := range names {
			if _, banned := JWTOnlyMutations[n]; banned {
				return func(ctx context.Context) *graphql.Response {
					return graphql.ErrorResponse(ctx, "mutation not allowed for api key")
				}
			}
			if !u.APIKeyHasTrade {
				return func(ctx context.Context) *graphql.Response {
					return graphql.ErrorResponse(ctx, "api key does not have trade permission")
				}
			}
			if !TradeMutationAllowed(n) {
				return func(ctx context.Context) *graphql.Response {
					return graphql.ErrorResponse(ctx, "mutation not allowed for api key")
				}
			}
		}
	case ast.Query:
		for _, n := range rootFieldNames(op) {
			if _, banned := JWTOnlyQueries[n]; banned {
				return func(ctx context.Context) *graphql.Response {
					return graphql.ErrorResponse(ctx, "query not allowed for api key")
				}
			}
		}
	}
	return next(ctx)
}

func rootFieldNames(op *ast.OperationDefinition) []string {
	var names []string
	for _, sel := range op.SelectionSet {
		names = append(names, collectSelectionFieldNames(sel)...)
	}
	return names
}

func collectSelectionFieldNames(sel ast.Selection) []string {
	switch s := sel.(type) {
	case *ast.Field:
		return []string{s.Name}
	case *ast.InlineFragment:
		var out []string
		for _, inner := range s.SelectionSet {
			out = append(out, collectSelectionFieldNames(inner)...)
		}
		return out
	case *ast.FragmentSpread:
		return nil
	default:
		return nil
	}
}
