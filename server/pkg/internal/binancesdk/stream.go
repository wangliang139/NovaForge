package binancesdk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	binance "github.com/adshao/go-binance/v2"
	"github.com/bitly/go-simplejson"
	"github.com/google/uuid"
)

func (c *Client) ListenOrderBookEvent(ctx context.Context, symbols []string, errHandler func(error), eventHandler func(*binance.WsDepthEvent)) (chan struct{}, chan struct{}, error) {
	if len(symbols) == 0 {
		return nil, nil, fmt.Errorf("symbols is empty")
	}
	doneCh, stopCh, err := c.client.WsCombinedDepthServe(symbols, eventHandler, errHandler)
	if err != nil {
		return nil, nil, err
	}
	return doneCh, stopCh, err
}

func (c *Client) ListenOrderBookEvent100Ms(ctx context.Context, symbols []string, errHandler func(error), eventHandler func(*binance.WsDepthEvent)) (chan struct{}, chan struct{}, error) {
	if len(symbols) == 0 {
		return nil, nil, fmt.Errorf("symbols is empty")
	}
	doneCh, stopCh, err := c.client.WsCombinedDepthServe100Ms(symbols, eventHandler, errHandler)
	if err != nil {
		return nil, nil, err
	}
	return doneCh, stopCh, err
}

func (c *Client) ListenTradeEvent(ctx context.Context, symbols []string, errHandler func(err error), eventHandler func(*binance.WsCombinedTradeEvent)) (chan struct{}, chan struct{}, error) {
	if len(symbols) == 0 {
		return nil, nil, fmt.Errorf("symbols is empty")
	}
	doneCh, stopCh, err := c.client.WsCombinedTradeServe(symbols, eventHandler, errHandler)
	if err != nil {
		return nil, nil, err
	}
	return doneCh, stopCh, err
}

func (c *Client) ListenAggTradeEvent(ctx context.Context, symbols []string, errHandler func(err error), eventHandler func(*binance.WsAggTradeEvent)) (chan struct{}, chan struct{}, error) {
	if len(symbols) == 0 {
		return nil, nil, fmt.Errorf("symbols is empty")
	}
	doneCh, stopCh, err := c.client.WsCombinedAggTradeServe(symbols, eventHandler, errHandler)
	if err != nil {
		return nil, nil, err
	}
	return doneCh, stopCh, err
}

func (c *Client) ListenSapiWsEvent(ctx context.Context, topic string, errHandler func(err error), eventHandler func(*WsSapiEvent)) (chan struct{}, chan struct{}, error) {
	randomID := uuid.New().String()
	timestamp := time.Now().UnixMilli()

	payload := fmt.Sprintf("random=%s&topic=%s&recvWindow=%d&timestamp=%d", randomID, topic, 60000, timestamp)

	// sign the payload
	sign := hmac.New(sha256.New, []byte(c.conf.ApiSecret))
	sign.Write([]byte(payload))
	signature := hex.EncodeToString(sign.Sum(nil))

	cfg := &wsConfig{
		ApiKey:      c.conf.ApiKey,
		ApiSecret:   c.conf.ApiSecret,
		Proxy:       c.conf.Proxy,
		Endpoint:    fmt.Sprintf("%s?%s&signature=%s", BaseWsSapiMainURL, payload, signature),
		Keepalive:   true,
		Timeout:     binance.WebsocketTimeout,
		PongTimeout: binance.WebsocketPongTimeout,
	}
	wsHandler := func(message []byte) {
		j, err := simplejson.NewJson(message)
		if err != nil {
			errHandler(err)
			return
		}
		event := new(WsSapiEvent)
		event.Type = j.Get("type").MustString()
		event.Topic = j.Get("topic").MustString()
		event.Data = j.Get("data").MustString()
		eventHandler(event)
	}
	return wsServe(cfg, wsHandler, errHandler)
}
