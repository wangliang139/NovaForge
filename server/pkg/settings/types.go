package settings

// SettingEntry 用户自定义 KV
type SettingEntry struct {
	Key   string
	Value string
}

// PushConfig 实例级消息推送配置（KV）。
type PushConfig struct {
	PushDocumentEnabled bool   `json:"pushDocumentEnabled"`
	PushAlarmEnabled    bool   `json:"pushAlarmEnabled"`
	PushTradeEnabled    bool   `json:"pushTradeEnabled"`
	Provider            string `json:"provider"`
	TelegramBotToken    string `json:"telegramBotToken"`
	TelegramChannelID   int    `json:"telegramChannelID"`
	FeishuWebhookURL    string `json:"feishuWebhookURL"`
	FeishuSecret        string `json:"feishuSecret"`
	FeishuKeyword       string `json:"feishuKeyword"`
}

const (
	PushProviderTelegram = "telegram"
	PushProviderFeishu   = "feishu"
)

// TelegramNewsCollectorStored KV 中单 key JSON 的序列化形态。
type TelegramAppConfig struct {
	AppID   string `json:"app_id"`
	AppHash string `json:"app_hash"`
	Session string `json:"session"`
}

// LlmProviderConfig 大模型网关 API Key（存于 kv；与 ZAI_* 环境变量叠加时以 kv 为准）。
type LlmProviderConfig struct {
	OpenRouterAPIKey  string `json:"openRouterApiKey"`
	SiliconFlowAPIKey string `json:"siliconFlowApiKey,omitempty"`
	DefaultModel      string `json:"defaultModel"`
}
