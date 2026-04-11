package gateio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/kelseyhightower/envconfig"
	"github.com/wangliang139/mow/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Client struct {
	conf   Config
	client *http.Client
}

// NewClient 使用 GATEIO_* 环境变量中的 ApiHost；httpProxyURL 为非空时使用该 HTTP(S) 代理。
func NewClient(httpProxyURL string) *Client {
	var conf Config
	envconfig.MustProcess("GATEIO", &conf)
	transport := http.DefaultTransport
	if p := strings.TrimSpace(httpProxyURL); p != "" {
		u, err := url.Parse(p)
		if err != nil {
			panic(err)
		}
		transport = &http.Transport{
			Proxy: http.ProxyURL(u),
		}
	}
	return &Client{
		conf:   conf,
		client: &http.Client{
			Transport: otelhttp.NewTransport(transport),
		},
	}
}

func (c *Client) GetFutureList(ctx context.Context, page, size int, startTime, endTime int64) (*Response[ListFutureResponse], error) {
	logger.Ctx(ctx).Info().Int("page", page).Int("size", size).Int64("start_time", startTime).Int64("end_time", endTime).Msg("get future list request")

	queryUrl := fmt.Sprintf("%s/future-event-api/api/v2/future/getList?page=%d&limit=%d&begin_time=%d&end_time=%d", c.conf.ApiHost, page, size, startTime, endTime)
	req, err := http.NewRequestWithContext(ctx, "GET", queryUrl, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("accept-language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("lang", "zh")

	rsp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}
	defer func() {
		cerr := rsp.Body.Close()
		// Only overwrite the retured error if the original error was nil and an
		// error occurred while closing the body.
		if err == nil && cerr != nil {
			err = cerr
		}
	}()

	// logger.Ctx(ctx).Info().Str("data", string(data)).Msg("get future list response")

	var response Response[ListFutureResponse]
	if err := sonic.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
