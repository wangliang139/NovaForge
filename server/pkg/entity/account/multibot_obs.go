package account

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

// P2 T6：multi_bot 归因 / 分摊「可检索」观测（日志事件码），便于检索与后续接 metric。
// 约定：字段名稳定；不记录密钥；金额以字符串落日志。

const (
	p2ObsBalanceFanoutZeroW   = "p2.multi_bot.balance_fanout_zero_total_weight"
	p2ObsOrderPropEmptyWeights = "p2.multi_bot.order_proportional_empty_weights"
	p2ObsOrderPropZeroDenom = "p2.multi_bot.order_proportional_zero_total_weight"
)

func sumSubWeightsForObs(ws []SubWeight) decimal.Decimal {
	var s decimal.Decimal
	for _, w := range ws {
		s = s.Add(w.W)
	}
	return s
}

func logP2T6BalanceFanoutZeroTotalWeight(
	ctx context.Context,
	parentID, exchangeStr string,
	walletType ctypes.WalletType,
	assetCode string,
	ledgerReason string,
	wUnalloc decimal.Decimal,
	weights []SubWeight,
	ts time.Time,
	frozenLeg bool,
) {
	ev := logger.Ctx(ctx).Warn().
		Str("p2_obs", p2ObsBalanceFanoutZeroW).
		Str("parent_id", parentID).
		Str("exchange", exchangeStr).
		Str("asset", assetCode).
		Str("wallet_type", string(walletType)).
		Str("ledger_reason", ledgerReason).
		Str("w_unalloc", wUnalloc.String()).
		Str("sum_sub_w", sumSubWeightsForObs(weights).String()).
		Int("sub_count", len(weights))
	if !ts.IsZero() {
		ev = ev.Time("as_of", ts)
	}
	if frozenLeg {
		ev.Msg("multi_bot balance fanout skipped: zero total weight (frozen leg)")
	} else {
		ev.Msg("multi_bot balance fanout skipped: zero total weight (total leg)")
	}
}

func logP2T6OrderProportionalEmptyWeights(ctx context.Context, parentID string, exchange ctypes.Exchange, ord *ctypes.Order) {
	if ord == nil {
		return
	}
	logger.Ctx(ctx).Warn().
		Str("p2_obs", p2ObsOrderPropEmptyWeights).
		Str("parent_id", parentID).
		Str("exchange", exchange.String()).
		Str("order_id", ord.OrderID.String()).
		Str("client_order_id", ord.ClientOrderID.String()).
		Int64("bot_id", ord.BotID).
		Str("symbol", ord.Symbol.String()).
		Msg("multi_bot order fanout: proportional branch has no weights (falls through to parent row)")
}

func logP2T6OrderProportionalZeroDenom(ctx context.Context, parentID string, exchange ctypes.Exchange, ord *ctypes.Order, weights []SubWeight, wUnalloc decimal.Decimal) {
	if ord == nil {
		return
	}
	logger.Ctx(ctx).Warn().
		Str("p2_obs", p2ObsOrderPropZeroDenom).
		Str("parent_id", parentID).
		Str("exchange", exchange.String()).
		Str("order_id", ord.OrderID.String()).
		Str("client_order_id", ord.ClientOrderID.String()).
		Int64("bot_id", ord.BotID).
		Str("symbol", ord.Symbol.String()).
		Str("w_unalloc", wUnalloc.String()).
		Str("sum_sub_w", sumSubWeightsForObs(weights).String()).
		Int("sub_count", len(weights)).
		Msg("multi_bot order fanout: proportional split W=0 (falls through to parent row)")
}
