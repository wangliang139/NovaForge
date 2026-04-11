package entity

import (
	"github.com/redis/go-redis/v9"
	"github.com/wangliang139/llt-trade/server/pkg/entity/account"
	"github.com/wangliang139/llt-trade/server/pkg/entity/document"
	"github.com/wangliang139/llt-trade/server/pkg/entity/llm"
	"github.com/wangliang139/llt-trade/server/pkg/entity/market"
	"github.com/wangliang139/llt-trade/server/pkg/entity/order"
	"github.com/wangliang139/llt-trade/server/pkg/entity/risk"
	"github.com/wangliang139/llt-trade/server/pkg/entity/strategy"
	"github.com/wangliang139/llt-trade/server/pkg/entity/user"
	"github.com/wangliang139/llt-trade/server/pkg/internal/zai"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
	"github.com/wangliang139/mow/executors"
)

var (
	Account  *account.Entity
	Document *document.Entity
	Llm      *llm.Entity
	Market   *market.Entity
	Risk     *risk.RiskController
	Order    *order.Entity
	Strategy *strategy.Entity
	User     *user.Entity
)

func Init(
	cache redis.UniversalClient,
	db *repos.Entity,
	executor *executors.Executor,
) {
	zaiEngine := zai.NewEngine()

	var err error
	Market, err = market.New(cache, db)
	if err != nil {
		panic(err)
	}

	Risk = risk.NewRiskController(cache, Market.Engine())

	Account = account.New(db, Market.Engine(), cache, Risk)
	Order = order.New(db, cache, Account, Risk)

	Risk.SetAccountProvider(Account)
	Risk.SetOrderGateway(Order)

	Llm = llm.New(db, zaiEngine)
	Document = document.New(db, cache, zaiEngine, executor, Llm)
	User = user.New(db)
	Strategy, err = strategy.New(db)
	if err != nil {
		panic(err)
	}
}
