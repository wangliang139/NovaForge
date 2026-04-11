package okxsdk

import (
	"context"
	"fmt"

	okx "github.com/wangliang139/okx-connector-go"
)

type StreamClient struct {
	client *okx.WebsocketStreamClient
}

func NewWsPublicStreamClient(conf Config) *StreamClient {
	client := okx.NewWsStreamClient(okx.WithWsAPIAuth(conf.ApiKey, conf.ApiSecret, conf.Passphrase))
	if conf.IsDebug {
		client.Debug = true
	}
	return &StreamClient{client: client}
}

func (c *StreamClient) ListenOrderBookEvent(ctx context.Context, channel okx.DepthChannel, symbols []string, errHandler func(error), eventHandler func(*okx.WsDepthEvent)) (chan struct{}, chan struct{}, error) {
	if len(symbols) == 0 {
		return nil, nil, fmt.Errorf("symbols is empty")
	}
	doneCh, stopCh, err := c.client.WsDepthServe(ctx, symbols, channel, eventHandler, errHandler)
	if err != nil {
		return nil, nil, err
	}
	return doneCh, stopCh, err
}

func (c *StreamClient) ListenKlineEvent(ctx context.Context, channel okx.KlineChannel, symbols []string, errHandler func(err error), eventHandler func(*okx.WsKlineEvent)) (chan struct{}, chan struct{}, error) {
	if len(symbols) == 0 {
		return nil, nil, fmt.Errorf("symbols is empty")
	}
	doneCh, stopCh, err := c.client.WsKlineServe(ctx, symbols, channel, eventHandler, errHandler)
	if err != nil {
		return nil, nil, err
	}
	return doneCh, stopCh, err
}

func (c *StreamClient) ListenTradeEvent(ctx context.Context, symbols []string, errHandler func(err error), eventHandler func(event *okx.WsTradeEvent)) (chan struct{}, chan struct{}, error) {
	if len(symbols) == 0 {
		return nil, nil, fmt.Errorf("symbols is empty")
	}
	doneCh, stopCh, err := c.client.WsTradeServe(ctx, symbols, eventHandler, errHandler)
	if err != nil {
		return nil, nil, err
	}
	return doneCh, stopCh, err
}

func (c *StreamClient) ListenTickerEvent(ctx context.Context, symbols []string, errHandler func(err error), eventHandler func(*okx.WsTickerEvent)) (chan struct{}, chan struct{}, error) {
	if len(symbols) == 0 {
		return nil, nil, fmt.Errorf("symbols is empty")
	}
	doneCh, stopCh, err := c.client.WsTickerServe(ctx, symbols, eventHandler, errHandler)
	if err != nil {
		return nil, nil, err
	}
	return doneCh, stopCh, err
}
