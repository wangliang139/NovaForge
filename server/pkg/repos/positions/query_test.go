package positions

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
)

func TestUpsertPosition(t *testing.T) {
	os.Setenv("POSTGRES_PASSWORD", "postgres")
	os.Setenv("POSTGRES_APPNAME", "novaforge")
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

	db := New(pool.WConn(), nil)

	qty := utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(100))
	entryPrice := utils.Decimal.DecimalToPgNumeric(decimal.NewFromInt(2000))
	
	row, err := db.UpsertPosition(context.Background(), UpsertPositionParams{
		AccountID: "123",
		Exchange: "binance",
		Symbol: "BTCUSDT",
		Side: PositionSideLONG,
		Qty: qty,
		EntryPrice: entryPrice,
		Leverage: lo.ToPtr(int32(3)),
		UpdatedTs: time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to upsert position: %v", err)
	}
	log.Info().Interface("row", row).Msg("upsert position")
}