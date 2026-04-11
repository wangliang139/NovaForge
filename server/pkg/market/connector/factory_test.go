package connector

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func Test_GetConnector(t *testing.T) {
	if os.Getenv("NOVAFORGE_INTEGRATION_TEST") == "" {
		t.Skip("skip integration test; set NOVAFORGE_INTEGRATION_TEST=1 to enable")
	}

	connector, err := GetConnector(ctypes.ExchangeOkx, nil)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	markets, err := connector.GetMarkets(context.Background(), []ctypes.MarketType{ctypes.MarketTypeSpot, ctypes.MarketTypeFuture})
	if err != nil {
		t.Fatalf("failed to get markets: %v", err)
	}
	log.Info().Int("markets", len(markets)).Send()

	symbol, err := ctypes.ParseSymbol("BTC/USDT")
	if err != nil {
		t.Fatalf("failed to parse symbol: %v", err)
	}

	handle, err := connector.Subscribe(context.Background(), ctypes.StreamSelector{Stream: ctypes.StreamTypeTicker, Symbol: &symbol})
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
	defer handle.Stop()

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case msg, ok := <-handle.C:
		if !ok {
			t.Fatalf("stream closed unexpectedly")
		}
		log.Info().Interface("msg", msg).Send()
	case <-timer.C:
		t.Fatalf("timeout waiting ticker message")
	}
}
