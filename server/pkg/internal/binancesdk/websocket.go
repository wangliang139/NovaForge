package binancesdk

import (
	"net/http"
	"net/url"
	"time"

	binance "github.com/adshao/go-binance/v2"
	"github.com/gorilla/websocket"
)

type wsConfig struct {
	ApiKey      string
	ApiSecret   string
	Endpoint    string
	Proxy       *string
	Keepalive   bool
	Timeout     time.Duration
	PongTimeout time.Duration
}

var wsServe = func(cfg *wsConfig, handler binance.WsHandler, errHandler binance.ErrHandler) (doneC, stopC chan struct{}, err error) {
	proxy := http.ProxyFromEnvironment
	if cfg.Proxy != nil && len(*cfg.Proxy) > 0 {
		u, err := url.Parse(*cfg.Proxy)
		if err != nil {
			return nil, nil, err
		}
		proxy = http.ProxyURL(u)
	}
	dailer := websocket.Dialer{
		Proxy:             proxy,
		HandshakeTimeout:  45 * time.Second,
		EnableCompression: true,
	}

	requestHeader := http.Header{}
	if len(cfg.ApiKey) > 0 {
		requestHeader.Set("X-MBX-APIKEY", cfg.ApiKey)
	}
	c, _, err := dailer.Dial(cfg.Endpoint, requestHeader)
	if err != nil {
		return nil, nil, err
	}
	c.SetReadLimit(655350)
	doneC = make(chan struct{})
	stopC = make(chan struct{})
	go func() {
		// This function will exit either on error from
		// websocket.Conn.ReadMessage or when the stopC channel is
		// closed by the client.

		defer close(doneC)
		if cfg.Keepalive {
			// This function overwrites the default ping frame handler
			// sent by the websocket API server
			keepAlive(c, cfg.Timeout, cfg.PongTimeout)
		}

		// Wait for the stopC channel to be closed.  We do that in a
		// separate goroutine because ReadMessage is a blocking
		// operation.
		silent := false
		go func() {
			select {
			case <-stopC:
				silent = true
			case <-doneC:
			}
			c.Close()
		}()
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				if !silent {
					errHandler(err)
				}
				return
			}
			handler(message)
		}
	}()
	return
}

func keepAlive(c *websocket.Conn, heartbeatTimeout time.Duration, perRequestTimeout time.Duration) {
	ticker := time.NewTicker(heartbeatTimeout)
	pingTicker := time.NewTicker(15 * time.Second)

	lastPing := time.Now()

	c.SetPingHandler(func(pingData string) error {
		// Respond with Pong using the server's PING payload
		return c.WriteControl(
			websocket.PongMessage,
			[]byte(pingData),
			time.Now().Add(perRequestTimeout), // Short deadline to ensure timely response
		)
	})

	c.SetPongHandler(func(pongData string) error {
		lastPing = time.Now()
		return nil
	})

	go func() {
		defer ticker.Stop()
		defer pingTicker.Stop()
		for {
			select {
			case <-ticker.C:
				if time.Since(lastPing) > heartbeatTimeout {
					c.Close()
					return
				}
			case <-pingTicker.C:
				c.WriteControl(websocket.PingMessage, []byte(""), time.Now().Add(perRequestTimeout))
			}
		}
	}()
}
