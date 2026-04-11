package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/golang-jwt/jwt/v5"
)

var (
	// JWT 密钥，实际部署时应该从环境变量读取
	jwtSecret []byte

	// Token 有效期
	tokenExpiration = 7 * 24 * time.Hour // 7天

	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

// Claims JWT 声明
type Claims struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Access string `json:"access"` // user, admin
	jwt.RegisteredClaims
}

// InitJWT 初始化 JWT 配置
func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// 如果没有提供密钥，生成一个随机密钥（仅用于开发）
		randomBytes := make([]byte, 32)
		if _, err := rand.Read(randomBytes); err != nil {
			panic("failed to generate random JWT secret: " + err.Error())
		}
		secret = base64.StdEncoding.EncodeToString(randomBytes)
	}
	jwtSecret = []byte(secret)
}

// GenerateToken 生成 JWT token
func GenerateToken(userID int64, name, access string) (string, error) {
	if len(jwtSecret) == 0 {
		return "", errors.New("JWT secret not initialized")
	}

	now := time.Now()
	claims := Claims{
		UserID: userID,
		Name:   name,
		Access: access,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenExpiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateToken 验证 JWT token
func ValidateToken(tokenString string) (*Claims, error) {
	if len(jwtSecret) == 0 {
		return nil, errors.New("JWT secret not initialized")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}


func ExtractToken(payload transport.InitPayload) string {
	raw, ok := payload["Authorization"]
	if !ok {
		raw, ok = payload["authorization"]
	}
	if !ok {
		raw, ok = payload["token"]
	}
	if !ok {
		return ""
	}
	val, ok := raw.(string)
	if !ok {
		return ""
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(val), "bearer ") {
		return strings.TrimSpace(val[7:])
	}
	return val
}