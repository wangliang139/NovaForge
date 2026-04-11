package settings

import (
	"fmt"
	"strings"
)

const (
	KeyPushConfig = "settings.push.config"
	// KeyPushTemplatePrefix 推送模板前缀：push.template.{scene}.{provider}
	KeyPushTemplatePrefix = "push.template."

	// KeyTelegramNewsCollector MTProto 资讯采集配置（单条 JSON：app_id / app_hash / session）
	KeyNewsCollector = "settings.news.collector"
	// KeyNewsCollectorEnabled 资讯采集总开关（true/false）
	KeyNewsCollectorEnabled = "settings.news.collector.enabled"

	KeyLlmAPIKey = "settings.llm.or_api_key"

	// KeyTavilyAPIKey Tavily Web Search API Key（单条字符串）
	KeyTavilyAPIKey = "settings.llm.tavily_api_key"

	// KeyHttpProxyURL 出站 HTTP(S) 代理，由用户中心配置；值为完整 URL（http/https）或空表示直连。
	KeyHttpProxyURL = "settings.network.http_proxy"
)

// KeySettingsPrefix 用户自定义 KV 在 public.kv 中的 key 前缀（与系统 settings.telegram.* 等隔离）。
const KeySettingsPrefix = "settings."

// VerifyKey 校验 TrimSpace 后的逻辑 key：非空且以前缀开头。
func VerifyKey(key string) error {
	k := strings.TrimSpace(key)
	if k == "" {
		return fmt.Errorf("user setting key is empty")
	}
	if !strings.HasPrefix(k, KeySettingsPrefix) {
		return fmt.Errorf("user setting key must start with %q", KeySettingsPrefix)
	}
	return nil
}
