package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/llt-trade/server/pkg/gateway/gql"
)

const (
	ContextKeyUser = "user"
)

// APIKeyAuthenticator 校验 X-API-Key（由 usersvc.Service 实现）。
type APIKeyAuthenticator interface {
	AuthenticateAPIKey(ctx context.Context, rawKey string) (*User, error)
}

// AuthMiddleware JWT 认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			gql.WrapGinError(c, http.StatusUnauthorized, "missing authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			gql.WrapGinError(c, http.StatusUnauthorized, "invalid authorization format")
			c.Abort()
			return
		}

		tokenString := parts[1]

		claims, err := ValidateToken(tokenString)
		if err != nil {
			log.Warn().Err(err).Msg("invalid token")
			gql.WrapGinError(c, http.StatusUnauthorized, "invalid or expired token")
			c.Abort()
			return
		}

		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, ContextKeyUser, &User{
			ID:     claims.UserID,
			Name:   claims.Name,
			Access: claims.Access,
			Source: AuthSourceJWT,
		})
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// OptionalAuthMiddleware 可选认证：合法 JWT 优先；否则尝试 X-API-Key。ak 可为 nil（仅 JWT）。
func OptionalAuthMiddleware(ak APIKeyAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		jwtOK := false

		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				claims, err := ValidateToken(parts[1])
				if err == nil {
					ctx = context.WithValue(ctx, ContextKeyUser, &User{
						ID:     claims.UserID,
						Name:   claims.Name,
						Access: claims.Access,
						Source: AuthSourceJWT,
					})
					c.Request = c.Request.WithContext(ctx)
					jwtOK = true
				}
			}
		}

		if !jwtOK && ak != nil {
			raw := strings.TrimSpace(c.GetHeader("X-API-Key"))
			if raw != "" {
				u, err := ak.AuthenticateAPIKey(ctx, raw)
				if err == nil && u != nil {
					ctx = context.WithValue(ctx, ContextKeyUser, u)
					c.Request = c.Request.WithContext(ctx)
				}
			}
		}
		c.Next()
	}
}

// APIKeyAuthMiddleware 仅允许通过 X-API-Key 访问。
func APIKeyAuthMiddleware(ak APIKeyAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ak == nil {
			gql.WrapGinError(c, http.StatusUnauthorized, "api key authentication is not configured")
			c.Abort()
			return
		}

		raw := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if raw == "" {
			gql.WrapGinError(c, http.StatusUnauthorized, "missing X-API-Key header")
			c.Abort()
			return
		}

		u, err := ak.AuthenticateAPIKey(c.Request.Context(), raw)
		if err != nil || u == nil {
			if err != nil {
				log.Warn().Err(err).Msg("invalid api key")
			}
			gql.WrapGinError(c, http.StatusUnauthorized, "invalid api key")
			c.Abort()
			return
		}

		ctx := context.WithValue(c.Request.Context(), ContextKeyUser, u)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// GetUserFromContext 从 context 获取用户。
func GetUserFromContext(ctx context.Context) (*User, bool) {
	v := ctx.Value(ContextKeyUser)
	switch u := v.(type) {
	case *User:
		return u, true
	case User:
		uu := u
		return &uu, true
	default:
		return nil, false
	}
}
