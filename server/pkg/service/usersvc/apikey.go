package usersvc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/gateway/auth"
	userapikey "github.com/wangliang139/NovaForge/server/pkg/repos/api_keys"
	"golang.org/x/crypto/bcrypt"
)

const (
	APIKeyPermissionQuery = "query"
	APIKeyPermissionTrade = "trade"
	// MaxActiveUserAPIKeys 单用户同时生效（未删除）的 API 密钥数量上限。
	MaxActiveUserAPIKeys = 10
	// CreateUserApiKey 使用 randomHexBytes(16)，hex 编码后 lookup 长度为 32。
	currentAPIKeyLookupLength = 32
	legacyAPIKeyLookupLength  = 16
)

var ErrInvalidAPIKey = errors.New("invalid api key")

// AuthenticateAPIKey 校验 X-API-Key 原始串，成功则返回注入 context 的 User（实现 auth.APIKeyAuthenticator）。
func (s *Service) AuthenticateAPIKey(ctx context.Context, rawKey string) (*auth.User, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, ErrInvalidAPIKey
	}
	lookup, secret, ok := parseAPIKeyRaw(rawKey)
	if !ok {
		return nil, ErrInvalidAPIKey
	}

	row, err := s.db.ApiKeysRepo.GetUserApiKeyByLookup(ctx, lookup)
	if err != nil {
		log.Warn().Err(err).Msg("api key lookup failed")
		return nil, ErrInvalidAPIKey
	}
	if row == nil {
		_ = bcrypt.CompareHashAndPassword(dummyAPIKeyBcryptHash(), []byte("invalid"))
		return nil, ErrInvalidAPIKey
	}

	if err := bcrypt.CompareHashAndPassword([]byte(row.SecretHash), []byte(secret)); err != nil {
		return nil, ErrInvalidAPIKey
	}

	u, err := s.db.UserRepo.GetUserByID(ctx, row.UserID)
	if err != nil || u == nil {
		log.Warn().Err(err).Int64("user_id", row.UserID).Msg("api key owner missing")
		return nil, ErrInvalidAPIKey
	}
	if u.Status != nil && *u.Status != "" && *u.Status != "active" {
		return nil, ErrInvalidAPIKey
	}

	access := "user"
	if u.Access != nil && *u.Access != "" {
		access = *u.Access
	}

	hasTrade := false
	for _, p := range row.Permissions {
		if p == APIKeyPermissionTrade {
			hasTrade = true
			break
		}
	}

	return &auth.User{
		ID:             u.ID,
		Name:           u.Name,
		Access:         access,
		Source:         auth.AuthSourceAPIKey,
		APIKeyHasTrade: hasTrade,
	}, nil
}

var (
	dummyAPIKeyBcryptOnce   sync.Once
	dummyAPIKeyBcryptCached []byte
)

func dummyAPIKeyBcryptHash() []byte {
	dummyAPIKeyBcryptOnce.Do(func() {
		h, err := bcrypt.GenerateFromPassword([]byte("invalid-api-key"), bcrypt.MinCost)
		if err != nil {
			dummyAPIKeyBcryptCached = []byte("$2a$04$invalidinvalidinvalidinvalidinvali")
			return
		}
		dummyAPIKeyBcryptCached = h
	})
	return dummyAPIKeyBcryptCached
}

// CreateUserApiKey 生成密钥；返回完整可展示 secret（仅此次）与行数据（不含 secret hash）。
func (s *Service) CreateUserApiKey(ctx context.Context, userID int64, name string, permissions []string) (plainKey string, row *userapikey.ApiKey, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, errors.New("name is required")
	}
	perms := normalizeAPIKeyPermissions(permissions)
	if len(perms) == 0 {
		return "", nil, errors.New("at least query permission is required")
	}

	taken, err := s.UserApiKeyNameTaken(ctx, userID, name)
	if err != nil {
		return "", nil, err
	}
	if taken {
		return "", nil, errors.New("api key name already exists")
	}

	activePtr, err := s.db.ApiKeysRepo.CountActiveUserApiKeysByUserID(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	var activeCount int64
	if activePtr != nil {
		activeCount = *activePtr
	}
	if activeCount >= MaxActiveUserAPIKeys {
		return "", nil, fmt.Errorf("每位用户最多同时保有 %d 个生效中的 API 密钥，请先删除不再使用的密钥", MaxActiveUserAPIKeys)
	}

	lookup, err := randomHexBytes(16)
	if err != nil {
		return "", nil, err
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", nil, err
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	plainKey = "nf_" + lookup + "_" + secret

	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, err
	}
	prefix := "nf_" + lookup[:minLookupPrefix(8, len(lookup))] + "…"

	created, err := s.db.ApiKeysRepo.CreateUserApiKey(ctx, userapikey.CreateUserApiKeyParams{
		UserID:      userID,
		Name:        name,
		KeyLookup:   lookup,
		SecretHash:  string(hash),
		KeyPrefix:   prefix,
		Permissions: perms,
	})
	if err != nil {
		return "", nil, err
	}
	if created == nil {
		return "", nil, errors.New("failed to create api key")
	}
	created.SecretHash = ""
	return plainKey, created, nil
}

func minLookupPrefix(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func normalizeAPIKeyPermissions(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != APIKeyPermissionQuery && p != APIKeyPermissionTrade {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if _, ok := seen[APIKeyPermissionQuery]; !ok {
		out = append([]string{APIKeyPermissionQuery}, out...)
	}
	return out
}

// UserApiKeyNameTaken 在「未删除」的密钥中是否已存在同名（同一用户下）。
func (s *Service) UserApiKeyNameTaken(ctx context.Context, userID int64, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, nil
	}
	nPtr, err := s.db.ApiKeysRepo.CountActiveUserApiKeysByUserIDAndName(ctx, userapikey.CountActiveUserApiKeysByUserIDAndNameParams{
		UserID: userID,
		Name:   name,
	})
	if err != nil {
		return false, err
	}
	var n int64
	if nPtr != nil {
		n = *nPtr
	}
	return n > 0, nil
}

// ListUserApiKeys 列出未删除的密钥（不含 hash）。
func (s *Service) ListUserApiKeys(ctx context.Context, userID int64) ([]userapikey.ApiKey, error) {
	rows, err := s.db.ApiKeysRepo.ListUserApiKeysByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].SecretHash = ""
	}
	return rows, nil
}

// DeleteUserApiKey 软删，且校验归属。
func (s *Service) DeleteUserApiKey(ctx context.Context, userID, keyID int64) (bool, error) {
	n, err := s.db.ApiKeysRepo.SoftDeleteUserApiKey(ctx, userapikey.SoftDeleteUserApiKeyParams{
		ID:     keyID,
		UserID: userID,
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func parseAPIKeyRaw(raw string) (lookup, secret string, ok bool) {
	if !strings.HasPrefix(raw, "nf_") {
		return "", "", false
	}
	rest := strings.TrimPrefix(raw, "nf_")
	idx := strings.IndexByte(rest, '_')
	if idx <= 0 || idx >= len(rest)-1 {
		return "", "", false
	}
	lookup = rest[:idx]
	secret = rest[idx+1:]
	if len(lookup) != currentAPIKeyLookupLength && len(lookup) != legacyAPIKeyLookupLength {
		return "", "", false
	}
	for _, c := range lookup {
		if !isHexRune(c) {
			return "", "", false
		}
	}
	if len(secret) < 20 {
		return "", "", false
	}
	return lookup, secret, true
}

func isHexRune(c rune) bool {
	switch {
	case c >= '0' && c <= '9':
		return true
	case c >= 'a' && c <= 'f':
		return true
	case c >= 'A' && c <= 'F':
		return true
	default:
		return false
	}
}

func randomHexBytes(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
