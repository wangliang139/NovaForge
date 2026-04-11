package settings

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
)

func GetPushTemplate(ctx context.Context, sceneKey string, provider string) (string, bool, error) {
	s := strings.ToLower(strings.TrimSpace(sceneKey))
	p := strings.ToLower(strings.TrimSpace(provider))
	if s == "" || p == "" {
		return "", false, nil
	}
	key := KeyPushTemplatePrefix + s + "." + p
	row, err := kvRepo.GetByKey(ctx, key)
	if err != nil {
		return "", false, err
	}
	if row == nil || strings.TrimSpace(row.Value) == "" {
		return "", false, nil
	}
	return row.Value, true, nil
}

func GetPushConfig(ctx context.Context) (PushConfig, error) {
	conf := PushConfig{}
	row, err := kvRepo.GetByKey(ctx, KeyPushConfig)
	if err != nil {
		return conf, err
	}
	if row != nil {
		err = sonic.Unmarshal([]byte(row.Value), &conf)
		if err != nil {
			return conf, fmt.Errorf("failed to unmarshal telegram config: %w", err)
		}
	}
	if strings.TrimSpace(conf.Provider) == "" {
		conf.Provider = "telegram"
	}
	return conf, nil
}

func GetLlmProviderConfig(ctx context.Context) (LlmProviderConfig, error) {
	var out LlmProviderConfig
	row, err := kvRepo.GetByKey(ctx, KeyLlmAPIKey)
	if err != nil {
		return out, err
	}
	if row != nil {
		err = sonic.Unmarshal([]byte(row.Value), &out)
		if err != nil {
			return out, fmt.Errorf("failed to unmarshal llm provider config: %w", err)
		}
	}
	return out, nil
}

func GetNewsCollectConfig(ctx context.Context) (TelegramAppConfig, error) {
	conf := TelegramAppConfig{}
	row, err := kvRepo.GetByKey(ctx, KeyNewsCollector)
	if err != nil {
		return conf, err
	}
	if row != nil {
		err = sonic.Unmarshal([]byte(row.Value), &conf)
		if err != nil {
			return conf, fmt.Errorf("failed to unmarshal telegram news collector: %w", err)
		}
	}
	return conf, nil
}

func GetNewsCollectEnabled(ctx context.Context) (bool, error) {
	row, err := kvRepo.GetByKey(ctx, KeyNewsCollectorEnabled)
	if err != nil {
		return true, err
	}
	if row == nil {
		// 兼容历史行为：未配置开关时默认开启
		return true, nil
	}
	v := strings.ToLower(strings.TrimSpace(row.Value))
	switch v {
	case "", "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return true, nil
	}
}
