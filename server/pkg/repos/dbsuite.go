package repos

import (
	"github.com/stumble/dcache"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/llt-trade/server/pkg/repos/account"
	"github.com/wangliang139/llt-trade/server/pkg/repos/alert"
	"github.com/wangliang139/llt-trade/server/pkg/repos/alert_trigger_event"
	"github.com/wangliang139/llt-trade/server/pkg/repos/assets"
	"github.com/wangliang139/llt-trade/server/pkg/repos/backtest"
	"github.com/wangliang139/llt-trade/server/pkg/repos/bot"
	"github.com/wangliang139/llt-trade/server/pkg/repos/calendar"
	"github.com/wangliang139/llt-trade/server/pkg/repos/datasource"
	"github.com/wangliang139/llt-trade/server/pkg/repos/document"
	"github.com/wangliang139/llt-trade/server/pkg/repos/ds_items"
	"github.com/wangliang139/llt-trade/server/pkg/repos/equity"
	"github.com/wangliang139/llt-trade/server/pkg/repos/kv"
	"github.com/wangliang139/llt-trade/server/pkg/repos/ledgers"
	"github.com/wangliang139/llt-trade/server/pkg/repos/llm_completion"
	"github.com/wangliang139/llt-trade/server/pkg/repos/llm_dialog"
	"github.com/wangliang139/llt-trade/server/pkg/repos/llm_prompt"
	"github.com/wangliang139/llt-trade/server/pkg/repos/llm_scene"
	"github.com/wangliang139/llt-trade/server/pkg/repos/llm_session"
	"github.com/wangliang139/llt-trade/server/pkg/repos/orders"
	"github.com/wangliang139/llt-trade/server/pkg/repos/positions"
	"github.com/wangliang139/llt-trade/server/pkg/repos/risk_event"
	"github.com/wangliang139/llt-trade/server/pkg/repos/snapshot"
	"github.com/wangliang139/llt-trade/server/pkg/repos/strategy"
	"github.com/wangliang139/llt-trade/server/pkg/repos/symbol_equity"
	"github.com/wangliang139/llt-trade/server/pkg/repos/tg_channel"
	"github.com/wangliang139/llt-trade/server/pkg/repos/user"
	"github.com/wangliang139/llt-trade/server/pkg/repos/user_api_key"
)

// Entity acts as a wrapper of db related stuffs.
type Entity struct {
	ConnPool *wpgx.Pool
	DCache   *dcache.DCache

	AccountRepo       *account.Queries
	DocumentRepo      *document.Queries
	CalendarRepo      *calendar.Queries
	LlmSceneRepo      *llm_scene.Queries
	LlmPromptRepo     *llm_prompt.Queries
	LlmCompletionRepo *llm_completion.Queries
	LlmSessionRepo    *llm_session.Queries
	LlmDialogRepo     *llm_dialog.Queries
	TgChannelRepo     *tg_channel.Queries
	UserRepo          *user.Queries
	UserApiKeyRepo    *user_api_key.Queries

	AssetsRepo            *assets.Queries
	PositionsRepo         *positions.Queries
	OrdersRepo            *orders.Queries
	LedgersRepo           *ledgers.Queries
	EquityRepo            *equity.Queries
	SymbolEquityRepo      *symbol_equity.Queries
	RiskEventRepo         *risk_event.Queries
	AlertRepo             *alert.Queries
	AlertTriggerEventRepo *alert_trigger_event.Queries

	StrategyRepo   *strategy.Queries
	SnapshotRepo   *snapshot.Queries
	DataSourceRepo *datasource.Queries
	DsItemsRepo    *ds_items.Queries
	BotRepo        *bot.Queries
	BacktestRepo   *backtest.Queries
	KvRepo         *kv.Queries
}

func New(connPool *wpgx.Pool, dCache *dcache.DCache) *Entity {
	return &Entity{
		DCache:            dCache,
		ConnPool:          connPool,
		AccountRepo:       account.New(connPool.WConn(), dCache),
		DocumentRepo:      document.New(connPool.WConn(), dCache),
		CalendarRepo:      calendar.New(connPool.WConn(), dCache),
		LlmSceneRepo:      llm_scene.New(connPool.WConn(), dCache),
		LlmPromptRepo:     llm_prompt.New(connPool.WConn(), dCache),
		LlmCompletionRepo: llm_completion.New(connPool.WConn(), dCache),
		LlmSessionRepo:    llm_session.New(connPool.WConn(), dCache),
		LlmDialogRepo:     llm_dialog.New(connPool.WConn(), dCache),
		TgChannelRepo:     tg_channel.New(connPool.WConn(), dCache),
		UserRepo:          user.New(connPool.WConn(), dCache),
		UserApiKeyRepo:    user_api_key.New(connPool.WConn(), dCache),

		AssetsRepo:            assets.New(connPool.WConn(), dCache),
		PositionsRepo:         positions.New(connPool.WConn(), dCache),
		OrdersRepo:            orders.New(connPool.WConn(), dCache),
		LedgersRepo:           ledgers.New(connPool.WConn(), dCache),
		EquityRepo:            equity.New(connPool.WConn(), dCache),
		SymbolEquityRepo:      symbol_equity.New(connPool.WConn(), dCache),
		RiskEventRepo:         risk_event.New(connPool.WConn(), dCache),
		AlertRepo:             alert.New(connPool.WConn(), dCache),
		AlertTriggerEventRepo: alert_trigger_event.New(connPool.WConn(), dCache),

		StrategyRepo:   strategy.New(connPool.WConn(), dCache),
		SnapshotRepo:   snapshot.New(connPool.WConn(), dCache),
		DataSourceRepo: datasource.New(connPool.WConn(), dCache),
		DsItemsRepo:    ds_items.New(connPool.WConn(), dCache),
		BotRepo:        bot.New(connPool.WConn(), dCache),
		BacktestRepo:   backtest.New(connPool.WConn(), dCache),
		KvRepo:         kv.New(connPool.WConn(), dCache),
	}
}
