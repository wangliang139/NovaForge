package auth

// AuthSource 区分 JWT 与 API Key，零值表示未设置（按非 API Key 处理）。
type AuthSource int8

const (
	AuthSourceUnset AuthSource = iota
	AuthSourceJWT
	AuthSourceAPIKey
)

// User 注入 request context，供 GraphQL resolver 使用。
type User struct {
	ID     int64
	Name   string
	Access string

	Source AuthSource
	// APIKeyHasTrade 仅当 Source==AuthSourceAPIKey 时有意义；JWT 不受 Mutation 白名单限制。
	APIKeyHasTrade bool
}
