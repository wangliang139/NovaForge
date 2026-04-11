package settings

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NormalizeHttpProxyURL 校验并规范化代理 URL：空串表示不使用代理；非空须为含 host 的 http/https URL。
func NormalizeHttpProxyURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid proxy url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("proxy url scheme must be http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("proxy url must include host")
	}
	return s, nil
}

// GetHttpProxyURL 从 KV 读取当前代理 URL；无记录时返回空串。
func GetHttpProxyURL(ctx context.Context) (string, error) {
	row, err := kvRepo.GetByKey(ctx, KeyHttpProxyURL)
	if err != nil {
		return "", err
	}
	if row == nil {
		return "", nil
	}
	return strings.TrimSpace(row.Value), nil
}

// SetHttpProxyURL 写入代理 URL（可为空以清除）；会校验非空 URL。
func SetHttpProxyURL(ctx context.Context, raw string) error {
	normalized, err := NormalizeHttpProxyURL(raw)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return Set(ctx, KeyHttpProxyURL, normalized)
}
