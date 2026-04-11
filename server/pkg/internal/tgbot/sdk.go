package tgbot

import (
	"context"
	"net/http"
	"net/url"

	"github.com/mymmrac/telego"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Config struct {
	BotToken string
	ProxyUrl string
}

type Client struct {
	*telego.Bot

	conf Config
}

func NewBot(ctx context.Context, cfg *Config) (*Client, error) {
	transport := http.DefaultTransport
	if cfg.ProxyUrl != "" {
		u, err := url.Parse(cfg.ProxyUrl)
		if err != nil {
			return nil, err
		}
		transport = &http.Transport{
			Proxy: http.ProxyURL(u),
		}
	}
	bot, err := telego.NewBot(cfg.BotToken, telego.WithHTTPClient(&http.Client{
		Transport: otelhttp.NewTransport(transport),
	}))
	if err != nil {
		return nil, err
	}
	return &Client{
		Bot:  bot,
		conf: *cfg,
	}, nil
}
