package simulate_test

import (
	"context"
	"os"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/NovaForge/server/pkg/entity"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/simulate"
	"github.com/wangliang139/NovaForge/server/pkg/entity/account"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	"github.com/wangliang139/NovaForge/server/pkg/service/accountsvc"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func openIntegrationRepo(t *testing.T) *repos.Entity {
	t.Helper()
	if os.Getenv("NOVAFORGE_INTEGRATION_TEST") == "" {
		t.Skip("skip integration test; set NOVAFORGE_INTEGRATION_TEST=1 to enable")
	}

	var wpgxConfig wpgx.Config
	envconfig.MustProcess("postgres", &wpgxConfig)
	pool, err := wpgx.NewPool(context.Background(), &wpgxConfig)
	if err != nil {
		t.Fatalf("create pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return repos.New(pool, nil)
}

func setupIntegrationEntityAndProxy(t *testing.T, db *repos.Entity, acctSvc *accountsvc.Service) {
	t.Helper()
	entity.Account = account.New(db, nil, nil, nil)
	proxy.AssignStub(
		func(context.Context, *ctypes.GetMarketsRequest) (*ctypes.GetMarketsResponse, error) { return &ctypes.GetMarketsResponse{}, nil },
		func(context.Context, *ctypes.GetMarketRequest) (*ctypes.GetMarketResponse, error) { return &ctypes.GetMarketResponse{}, nil },
		func(context.Context, *ctypes.GetPriceRequest) (*ctypes.GetPriceResponse, error) { return &ctypes.GetPriceResponse{}, nil },
		func(context.Context, *ctypes.GetBookPriceRequest) (*ctypes.GetBookPriceResponse, error) { return &ctypes.GetBookPriceResponse{}, nil },
		func(context.Context, *ctypes.GetMarkPriceRequest) (*ctypes.GetMarkPriceResponse, error) { return &ctypes.GetMarkPriceResponse{}, nil },
		func(context.Context, *ctypes.GetIndexPriceRequest) (*ctypes.GetIndexPriceResponse, error) { return &ctypes.GetIndexPriceResponse{}, nil },
		func(context.Context, *ctypes.GetTickerRequest) (*ctypes.GetTickerResponse, error) { return &ctypes.GetTickerResponse{}, nil },
		func(context.Context, *ctypes.GetTradesRequest) (*ctypes.GetTradesResponse, error) { return &ctypes.GetTradesResponse{}, nil },
		func(context.Context, *ctypes.GetOrderBookRequest) (*ctypes.GetOrderBookResponse, error) { return &ctypes.GetOrderBookResponse{}, nil },
		func(context.Context, *ctypes.GetKlinesRequest) (*ctypes.GetKlinesResponse, error) { return &ctypes.GetKlinesResponse{}, nil },
		func(context.Context, *ctypes.GetHisKlinesRequest) (*ctypes.GetHisKlinesResponse, error) { return &ctypes.GetHisKlinesResponse{}, nil },
		func(context.Context, *ctypes.GetFundingRateRequest) (*ctypes.GetFundingRateResponse, error) { return &ctypes.GetFundingRateResponse{}, nil },
		func(context.Context, *ctypes.GetHisFundingRatesRequest) (*ctypes.GetHisFundingRatesResponse, error) {
			return &ctypes.GetHisFundingRatesResponse{}, nil
		},
		func(context.Context, *ctypes.GetOpenInterestRequest) (*ctypes.GetOpenInterestResponse, error) {
			return &ctypes.GetOpenInterestResponse{}, nil
		},
		acctSvc.GetSymbolConfig,
		func(context.Context, *ctypes.GetBalanceRequest) (*ctypes.GetBalanceResponse, error) { return &ctypes.GetBalanceResponse{}, nil },
		func(context.Context, *ctypes.GetPositionsRequest) (*ctypes.GetPositionsResponse, error) { return &ctypes.GetPositionsResponse{}, nil },
		func(context.Context, *ctypes.GetOpenOrdersRequest) (*ctypes.GetOpenOrdersResponse, error) { return &ctypes.GetOpenOrdersResponse{}, nil },
		func(context.Context, *ctypes.GetOrderRequest) (*ctypes.GetOrderResponse, error) { return &ctypes.GetOrderResponse{}, nil },
		func(context.Context, *ctypes.PlaceOrderRequest) (*ctypes.PlaceOrderResponse, error) { return &ctypes.PlaceOrderResponse{}, nil },
		func(context.Context, *ctypes.CancelOrderRequest) (*ctypes.CancelOrderResponse, error) {
			return &ctypes.CancelOrderResponse{Success: true}, nil
		},
		func(context.Context, *ctypes.GetLeverageRequest) (*ctypes.GetLeverageResponse, error) { return &ctypes.GetLeverageResponse{Leverage: 1}, nil },
		func(context.Context, *ctypes.SetLeverageRequest) (*ctypes.SetLeverageResponse, error) { return &ctypes.SetLeverageResponse{Leverage: 1}, nil },
		func(context.Context, *ctypes.FundsFreezeRequest) (*ctypes.FundsFreezeResponse, error) { return &ctypes.FundsFreezeResponse{Success: true}, nil },
		func(context.Context, *ctypes.FundsUnfreezeRequest) (*ctypes.FundsUnfreezeResponse, error) { return &ctypes.FundsUnfreezeResponse{Success: true}, nil },
		func(context.Context, *ctypes.SubscribeStreamRequest) (<-chan *ctypes.SubscribeStreamResponse, error) {
			ch := make(chan *ctypes.SubscribeStreamResponse)
			close(ch)
			return ch, nil
		},
	)
}

func TestPaperProxyServiceEntitySimulate_DBFixtureIntegration(t *testing.T) {
	db := openIntegrationRepo(t)
	_ = os.Setenv("ACCOUNT_SVC_DECRYPT_KEY_BASE64", "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY=")

	symbol, err := ctypes.ParseSymbol("BTC/USDT:SPOT")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}

	accountID := "it-sim-fixture-001"
	accountName := "it-sim-fixture-001"
	_, _ = db.AccountRepo.DeleteAccount(context.Background(), accountID)
	_, err = db.AccountRepo.Create(context.Background(), accountrepo.CreateParams{
		ID:          accountID,
		Name:        accountName,
		Exchange:    accountrepo.ExchangeBinance,
		Config:      []byte(`{}`),
		ApiKey:      "",
		ApiSecret:   "",
		Passphrase:  "",
		Algorithm:   accountrepo.AlgorithmHmac,
		Tags:        []string{"it"},
		Status:      accountrepo.AccountStatusOnline,
		AccountType: accountrepo.AccountTypeVirtual,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("insert fixture account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.AccountRepo.DeleteAccount(context.Background(), accountID)
	})

	acctSvc, err := accountsvc.New(db)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	setupIntegrationEntityAndProxy(t, db, acctSvc)

	// 验证 entity 层能够为 virtual account 返回 simulate connector。
	conn, err := entity.Account.GetConnector(context.Background(), ctypes.ExchangeBinance, accountID)
	if err != nil {
		t.Fatalf("entity get connector failed: %v", err)
	}
	if _, ok := conn.(*simulate.Connector); !ok {
		t.Fatalf("expected simulate connector, got %T", conn)
	}

	// 端到端链路：proxy -> accountsvc -> entity -> simulate connector
	cfg, err := proxy.GetSymbolConfig(context.Background(), accountID, symbol)
	if err != nil {
		t.Fatalf("proxy get symbol config failed: %v", err)
	}
	if cfg == nil {
		t.Fatalf("symbol config should not be nil")
	}
	if cfg.Exchange != ctypes.ExchangeBinance {
		t.Fatalf("unexpected exchange: %s", cfg.Exchange)
	}
	if cfg.Market.PricePrecision <= 0 {
		t.Fatalf("unexpected price precision: %d", cfg.Market.PricePrecision)
	}
}

