package tgbot

import (
	"context"
	"errors"

	"github.com/mymmrac/telego"
)

var global *Client

func SetupGlobalClient(ctx context.Context, cfg *Config) error {
	if len(cfg.BotToken) == 0 {
		global = nil
		return nil
	}
	tgClient, err := NewBot(ctx, cfg)
	if err != nil {
		return err
	}
	global = tgClient
	return nil
}

func HasGlobalClient() bool {
	return global != nil
}

func SendMessage(ctx context.Context, params *telego.SendMessageParams) (*telego.Message, error) {
	if global == nil {
		return nil, errors.New("telegram bot client not initialized")
	}
	return global.SendMessage(ctx, params)
}
