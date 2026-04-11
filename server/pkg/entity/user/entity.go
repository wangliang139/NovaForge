package user

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/wangliang139/llt-trade/server/pkg/internal/tgbot"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
	"github.com/wangliang139/llt-trade/server/pkg/settings"
)

type Entity struct {
	db *repos.Entity
}

func New(db *repos.Entity) *Entity {
	return &Entity{db: db}
}

func (s *Entity) Start() error {
	ctx := context.Background()
	tgConfig, err := settings.GetPushConfig(ctx)
	if err != nil {
		return err
	}
	if tgConfig.Provider == settings.PushProviderTelegram && len(tgConfig.TelegramBotToken) > 0 {
		httpProxyURL, err := settings.GetHttpProxyURL(ctx)
		if err != nil {
			return err
		}
		return tgbot.SetupGlobalClient(ctx, &tgbot.Config{
			BotToken: tgConfig.TelegramBotToken,
			ProxyUrl: httpProxyURL,
		})
	}
	return nil
}

func (s *Entity) UpdatePushConfig(ctx context.Context, st settings.PushConfig) error {
	value, err := sonic.MarshalString(st)
	if err != nil {
		return err
	}
	if err := settings.Set(ctx, settings.KeyPushConfig, value); err != nil {
		return err
	}

	httpProxyURL, err := settings.GetHttpProxyURL(ctx)
	if err != nil {
		return err
	}

	botToken := st.TelegramBotToken
	if st.Provider == settings.PushProviderFeishu {
		botToken = ""
	}
	tgbot.SetupGlobalClient(ctx, &tgbot.Config{
		BotToken: botToken,
		ProxyUrl: httpProxyURL,
	})

	return nil
}

func (s *Entity) UpdateLlmProviderConfig(ctx context.Context, st settings.LlmProviderConfig) error {
	value, err := sonic.MarshalString(st)
	if err != nil {
		return err
	}
	if err := settings.Set(ctx, settings.KeyLlmAPIKey, value); err != nil {
		return err
	}
	return nil
}
