package connector

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/binance"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/okx"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	"github.com/wangliang139/NovaForge/server/pkg/settings"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

var (
	mu           sync.RWMutex
	once         sync.Once
	httpProxyURL string
	connectors   = map[string]mdtypes.Connector{}
)

func key(exchange ctypes.Exchange, account *mdtypes.ApiAccount) string {
	if account == nil {
		return exchange.String()
	}
	return fmt.Sprintf("%s:%s", exchange.String(), account.ID)
}

func GetConnector(exchange ctypes.Exchange, account *mdtypes.ApiAccount) (mdtypes.Connector, error) {
	k := key(exchange, account)

	once.Do(func() {
		var err error
		httpProxyURL, err = settings.GetHttpProxyURL(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("failed to get http proxy url")
		}
	})

	mu.RLock()
	existing, ok := connectors[k]
	mu.RUnlock()
	if ok {
		return existing, nil
	}

	mu.Lock()
	defer mu.Unlock()
	if connector, ok := connectors[k]; ok {
		return connector, nil
	}

	var err error
	var connector mdtypes.Connector
	switch exchange {
	case ctypes.ExchangeBinance:
		connector, err = binance.New(binance.Config{ProxyURL: httpProxyURL}, account)
	case ctypes.ExchangeBinanceTest:
		connector, err = binance.New(binance.Config{UseDemo: true, ProxyURL: httpProxyURL}, account)
	case ctypes.ExchangeOkx:
		connector, err = okx.New(okx.Config{ProxyURL: httpProxyURL}, account)
	case ctypes.ExchangeOkxTest:
		connector, err = okx.New(okx.Config{UseTestnet: true, ProxyURL: httpProxyURL}, account)
	default:
		return nil, fmt.Errorf("unsupported exchange: %s", exchange)
	}
	if err != nil {
		return nil, err
	}
	connectors[k] = connector
	return connector, nil
}
