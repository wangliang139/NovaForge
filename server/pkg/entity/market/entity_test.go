package market

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/stumble/wpgx"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	converter "github.com/wangliang139/llt-trade/server/pkg/converter"
	"github.com/wangliang139/llt-trade/server/pkg/market/connector"
	"github.com/wangliang139/llt-trade/server/pkg/market/types"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
)

func Test_connector(t *testing.T) {
	conn, err := connector.GetConnector(ctypes.ExchangeOkx, nil)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	symbol := ctypes.Symbol{Base: "ETH", Quote: "USDT", Type: ctypes.MarketTypeSpot}

	ticker, err := conn.Ticker(context.Background(), symbol)
	if err != nil {
		t.Fatalf("failed to get ticker: %v", err)
	}
	log.Info().Interface("ticker", ticker).Msg("ticker")

	trades, err := conn.Trades(context.Background(), symbol, 10)
	if err != nil {
		t.Fatalf("failed to get trades: %v", err)
	}
	log.Info().Interface("trades", trades).Msg("trades")

	depth, err := conn.Depth(context.Background(), symbol, 10)
	if err != nil {
		t.Fatalf("failed to get depth: %v", err)
	}
	log.Info().Interface("depth", depth).Msg("depth")

	endTs := time.Now()
	startTs := endTs.Add(-10 * time.Minute)

	klines, err := conn.Klines(context.Background(), symbol, ctypes.Interval1m, 10)
	if err != nil {
		t.Fatalf("failed to get klines: %v", err)
	}
	log.Info().Interface("klines", klines).Msg("klines")

	hisKlines, err := conn.HisKlines(context.Background(), symbol, ctypes.Interval1m, &startTs, &endTs, lo.ToPtr(10))
	if err != nil {
		t.Fatalf("failed to get his klines: %v", err)
	}
	log.Info().Interface("hisKlines", hisKlines).Msg("hisKlines")
}

func Test_engine(t *testing.T) {
	os.Setenv("POSTGRES_PASSWORD", "postgres")
	os.Setenv("POSTGRES_APPNAME", "llt-data")
	os.Setenv("POSTGRES_DBNAME", "llt_data_db")
	os.Setenv("POSTGRES_PASSWORD", "my-secret")

	var wpgxConfig wpgx.Config
	envconfig.MustProcess("postgres", &wpgxConfig)
	log.Info().Msgf("wpgx config: %+v", &wpgxConfig)

	pool, err := wpgx.NewPool(context.Background(), &wpgxConfig)
	if err != nil {
		t.Fatalf("failed to create wpgx pool: %v", err)
	}

	defer pool.Close()

	db := repos.New(pool, nil)

	acct, err := db.AccountRepo.GetById(context.Background(), "2014196413501628416")
	if err != nil {
		t.Fatalf("failed to get account: %v", err)
	}
	if acct == nil {
		t.Fatalf("account not found")
	}

	account := converter.AccountRepo2Types(acct)
	apiAccount := types.NewSecretApiAccount(account.ID, account.Exchange, account.ApiKey, account.ApiSecret, account.Passphrase, string(account.Algorithm))
	conn, err := connector.GetConnector(account.Exchange, apiAccount)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	// markets, err := conn.GetMarkets(context.Background(), []types.MarketType{types.MarketTypeFuture})
	// if err != nil {
	// 	t.Fatalf("failed to get markets: %v", err)
	// }
	// log.Info().Interface("markets", markets).Msg("markets")

	// exAcct, err := conn.Account(context.Background())
	// if err != nil {
	// 	t.Fatalf("failed to get exchange account: %v", err)
	// }
	// log.Info().Interface("account", exAcct).Msg("exchange account info")

	// balance, err := conn.Balance(context.Background())
	// if err != nil {
	// 	t.Fatalf("failed to get balance: %v", err)
	// }
	// log.Info().Interface("balance", balance).Msg("balance")

	// positions, err := conn.Positions(context.Background(), lo.ToPtr(mdtypes.MarketTypeFuture))
	// if err != nil {
	// 	t.Fatalf("failed to get positions: %v", err)
	// }
	// log.Info().Interface("positions", positions).Msg("positions")

	// symbolConfig, err := conn.SymbolConfig(context.Background(), mdtypes.Symbol{Base: "ETH", Quote: "USDT", Type: mdtypes.MarketTypeFuture})
	// if err != nil {
	// 	t.Fatalf("failed to get symbol config: %v", err)
	// }
	// log.Info().Interface("symbolConfig", symbolConfig).Msg("symbolConfig")

	// symbol := mdtypes.Symbol{Base: "ETH", Quote: "USDT", Type: mdtypes.MarketTypeFuture}
	// orders, err := conn.GetOrders(context.Background(), &symbol)
	// if err != nil {
	// 	t.Fatalf("failed to get orders: %v", err)
	// }
	// log.Info().Interface("orders", orders).Msg("open orders")

	order, err := conn.GetOrder(context.Background(), ctypes.Symbol{Base: "ETH", Quote: "USDT", Type: ctypes.MarketTypeFuture}, "8389766084865745584")
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}
	log.Info().Interface("order", order).Msg("order")
}
