package binance

import (
	"context"
	"os"
	"testing"

	spots "github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/common"
	futures "github.com/adshao/go-binance/v2/futures"
	portfolio "github.com/adshao/go-binance/v2/portfolio"
	"github.com/rs/zerolog/log"
	mdtypes "github.com/wangliang139/llt-trade/server/pkg/market/types"
)

func requireBinanceIntegration(t *testing.T) (apiKey, apiSecret string) {
	t.Helper()
	if os.Getenv("LLT_INTEGRATION_TEST") == "" {
		t.Skip("skip integration test; set LLT_INTEGRATION_TEST=1 to enable")
	}
	apiKey = os.Getenv("BINANCE_API_KEY")
	apiSecret = os.Getenv("BINANCE_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		t.Skip("skip integration test; BINANCE_API_KEY/BINANCE_API_SECRET not set")
	}
	return apiKey, apiSecret
}

func Test_Balance(t *testing.T) {
	ApiKey, ApiSecret := requireBinanceIntegration(t)
	connector, err := New(Config{}, &mdtypes.ApiAccount{
		ApiKey:    ApiKey,
		ApiSecret: ApiSecret,
	})
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}
	balance, err := connector.Balance(context.Background())
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}
	log.Info().Interface("balance", balance).Msg("balance")
}

func Test_GetFutureMarkets(t *testing.T) {
	ApiKey, ApiSecret := requireBinanceIntegration(t)

	futuresClient := futures.NewClient(ApiKey, ApiSecret)
	exchangeInfo, err := futuresClient.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		t.Fatalf("failed to get exchange info: %v", err)
	}
	log.Info().Interface("exchangeInfo", exchangeInfo).Msg("exchange info")
}

func Test_GetSpotMarkets(t *testing.T) {
	ApiKey, ApiSecret := requireBinanceIntegration(t)

	symbol := "ETHUSDT"

	spotClient := spots.NewClient(ApiKey, ApiSecret)
	exchangeInfo, err := spotClient.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		t.Fatalf("failed to get exchange info: %v", err)
	}

	for _, market := range exchangeInfo.Symbols {
		if market.Symbol == symbol {
			log.Info().Interface("market", market).Msg("market")
			break
		}
	}
}

func Test_GetUMAccountDetail(t *testing.T) {
	ApiKey, ApiSecret := requireBinanceIntegration(t)

	portfolioClient := portfolio.NewClient(ApiKey, ApiSecret)
	portfolioClient.KeyType = common.KeyTypeHmac
	portfolioClient.Debug = true
	orders, err := portfolioClient.NewGetUMAccountDetailService().Do(context.Background())
	if err != nil {
		t.Fatalf("failed to get orders: %v", err)
	}
	log.Info().Interface("orders", orders).Msg("orders")
}
