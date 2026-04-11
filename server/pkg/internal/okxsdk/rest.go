package okxsdk

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	okx "github.com/wangliang139/okx-connector-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type RestClient struct {
	conf   Config
	client *okx.Client
}

func NewRestClient(conf Config) *RestClient {
	client := okx.NewClient(func(c *okx.Client) {
		c.APIKey = conf.ApiKey
		c.SecretKey = conf.ApiSecret
		c.Passphrase = conf.Passphrase
		if conf.BaseUrl != "" {
			c.BaseURL = conf.BaseUrl
		}
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
		c.HTTPClient = &http.Client{
			Transport: otelhttp.NewTransport(transport),
		}
		if conf.IsDebug {
			c.Debug = true
		}
		c.IsTestNet = conf.IsTestNet
		c.TimeOffset = conf.TimeOffset
	})
	return &RestClient{conf: conf, client: client}
}

func (c *RestClient) GetSymbolInfo(ctx context.Context, instType string, symbol *string) ([]*okx.SymbolInfo, error) {
	service := c.client.NewSymbolInfoService().InstType(instType)
	if symbol != nil {
		service = service.InstId(*symbol)
	}
	response, err := service.Do(ctx)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetSymbolKlines returns the klines data for a symbol. Not support '1s' interval.
func (c *RestClient) GetSymbolKlines(ctx context.Context, symbol, interval string, startTime, endTime *int64, limit *int) ([]*okx.Kline, error) {
	if len(symbol) == 0 {
		return nil, errors.New("symbol is required")
	}
	if interval == "1s" {
		service := c.client.NewMarketKlinesHisService().InstId(symbol).Bar(interval)
		if startTime != nil {
			service.After(*startTime * 1000)
		}
		if endTime != nil {
			service.Before(*endTime * 1000)
		}
		if limit != nil {
			service.Limit(*limit)
		}
		response, err := service.Do(ctx)
		if err != nil {
			return nil, err
		}
		return response, err
	} else {
		service := c.client.NewMarketKlinesService().InstId(symbol).Bar(interval)
		if startTime != nil {
			service.After(*startTime * 1000)
		}
		if endTime != nil {
			service.Before(*endTime * 1000)
		}
		if limit != nil {
			service.Limit(*limit)
		}
		response, err := service.Do(ctx)
		if err != nil {
			return nil, err
		}
		return response, err
	}
}

func (c *RestClient) GetTickerPrice(ctx context.Context, symbol string) (*string, error) {
	if len(symbol) == 0 {
		return nil, errors.New("symbol is required")
	}
	response, err := c.client.NewSymbolQuotationService().InstId(symbol).Do(ctx)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("symbol not found")
	}
	return &response.Last, nil
}

func (c *RestClient) GetOrderBookDepth(ctx context.Context, symbol string, size int) (*okx.Depth, error) {
	if len(symbol) == 0 {
		return nil, errors.New("symbol is required")
	}
	var (
		response *okx.Depth
		err      error
	)
	if size <= 400 {
		response, err = c.client.NewMarketDepthService().InstId(symbol).Size(size).Do(ctx)
	} else {
		response, err = c.client.NewMarketDepthFullService().InstId(symbol).Size(size).Do(ctx)
	}
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (c *RestClient) GetAnnouncementTypes(ctx context.Context) ([]*okx.AnnouncementType, error) {
	response, err := c.client.NewAnnouncementTypeService().Do(ctx)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetAnnouncements returns the announcements.
func (c *RestClient) GetAnnouncements(ctx context.Context, annType *string, page *int) ([]*okx.Announcement, error) {
	service := c.client.NewAnnouncementService()
	if annType != nil {
		service = service.AnnType(*annType)
	}
	if page != nil {
		service = service.Page(*page)
	}
	response, err := service.Do(ctx)
	if err != nil {
		return nil, err
	}

	anns := make([]*okx.Announcement, 0)
	for _, r := range response {
		anns = append(anns, r.Details...)
	}
	return anns, nil
}

func (c *RestClient) GetAccountBalance(ctx context.Context, assets ...string) (*okx.AccountBalance, error) {
	response, err := c.client.NewAccountBalanceService().Assets(assets).Do(ctx)
	if err != nil {
		return nil, err
	}
	return response, nil
}
