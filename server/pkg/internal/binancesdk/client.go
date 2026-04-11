package binancesdk

import (
	"log"
	"net/http"
	"net/url"

	binance "github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/common"
	zlog "github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Client struct {
	conf   Config
	client *binance.Client
}

func NewClient(conf Config) *Client {
	transport := http.DefaultTransport
	if conf.Proxy != nil && len(*conf.Proxy) > 0 {
		u, err := url.Parse(*conf.Proxy)
		if err != nil {
			panic(err)
		}
		transport = &http.Transport{
			Proxy: http.ProxyURL(u),
		}
	}
	// trace the request and response
	transport = otelhttp.NewTransport(transport)

	client := &binance.Client{
		APIKey:     conf.ApiKey,
		SecretKey:  conf.ApiSecret,
		KeyType:    common.KeyTypeHmac,
		UserAgent:  "Binance/golang",
		HTTPClient: &http.Client{Transport: transport},
		Logger:     log.New(zlog.Logger, "", 0),
		TimeOffset: conf.TimeOffset,
		Debug:      conf.IsDebug,
	}
	return &Client{conf: conf, client: client}
}
