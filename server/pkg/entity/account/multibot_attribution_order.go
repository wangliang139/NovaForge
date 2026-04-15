package account

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	ordersrepo "github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	"github.com/wangliang139/NovaForge/server/pkg/repos/positions"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/logger"
)

// SubRawDispatch P2 T1/T4：归因模块对单条父事件的子派发单元（未分配份额由父吸收，不进入本列表）。
// Share 为 [0,1] 内对本条事件量的归因比例；BotId / DB 精确命中时 Share=1。
type SubRawDispatch struct {
	SubAccountID string
	Share        decimal.Decimal
	Order        ctypes.Order
}

func cloneOrderForSub(ord ctypes.Order, subID string) ctypes.Order {
	cp := ord
	cp.AccountID = subID
	return cp
}

// absPositionWeightForFanout 平仓归因权重：仅使用 position_snapshot AtOrBefore(asOf)；无快照行则权重为 0，不回读实时 positions。
func (e *Entity) absPositionWeightForFanout(ctx context.Context, accountID, exchangeStr, sym string, side positions.PositionSide, asOf time.Time) (decimal.Decimal, error) {
	key := AccountStateAtPositionKey{
		Exchange: exchangeStr,
		Symbol:   sym,
		Side:     side,
	}
	snap, err := e.GetAccountPositionSnapshotAtOrBefore(ctx, accountID, key, asOf)
	if err != nil {
		return decimal.Zero, err
	}
	if snap != nil && snap.Found {
		return snap.Qty.Abs(), nil
	}
	return decimal.Zero, nil
}

func (e *Entity) accountIsVirtualSubOfParent(ctx context.Context, parentID, accountID string) bool {
	if accountID == "" || accountID == parentID {
		return false
	}
	a, err := e.GetAccount(ctx, accountID)
	if err != nil || a == nil {
		return false
	}
	if a.AccountType != ctypes.AccountTypeVirtualSub {
		return false
	}
	if a.ParentAccountID == nil || *a.ParentAccountID != parentID {
		return false
	}
	return true
}

// orderMatchedWeightsToSubFanoutShares 将子单 quantity 聚合权重按父单原始量 P 做比例拆分，与 SplitProportionalDelta 语义一致：
// 子 i 份额为 w_i/(sum(w)+max(0,P-sum(w)))，未镜像到子单的量 max(0,P-T) 由父吸收（份额不进入子派发）。
func orderMatchedWeightsToSubFanoutShares(weights map[string]decimal.Decimal, parentOriginalQty decimal.Decimal) (map[string]decimal.Decimal, error) {
	if len(weights) == 0 {
		return nil, nil
	}
	subs := make([]SubWeight, 0, len(weights))
	var total decimal.Decimal
	for sid, w := range weights {
		if w.LessThanOrEqual(decimal.Zero) {
			continue
		}
		subs = append(subs, SubWeight{SubAccountID: sid, W: w})
		total = total.Add(w)
	}
	if len(subs) == 0 {
		return nil, nil
	}
	wUnalloc := decimal.Zero
	if parentOriginalQty.IsPositive() && total.LessThan(parentOriginalQty) {
		wUnalloc = parentOriginalQty.Sub(total)
	}
	toSub, _, err := SplitProportionalDelta(decimal.NewFromInt(1), subs, wUnalloc)
	return toSub, err
}

func buildSubRawDispatchesFromUnitShares(ord ctypes.Order, unitShares map[string]decimal.Decimal) []SubRawDispatch {
	ids := make([]string, 0, len(unitShares))
	for sid, sh := range unitShares {
		if sh.IsZero() {
			continue
		}
		ids = append(ids, sid)
	}
	sort.Strings(ids)
	out := make([]SubRawDispatch, 0, len(ids))
	for _, sid := range ids {
		sh := unitShares[sid]
		out = append(out, SubRawDispatch{
			SubAccountID: sid,
			Share:        sh,
			Order:        cloneOrderForSub(ord, sid),
		})
	}
	return out
}

// computeOrderProportionalWeights 无 BotId / 无 DB 子命中时的比例权重
func (e *Entity) computeOrderProportionalWeights(ctx context.Context, parentID string, exchange ctypes.Exchange, ord ctypes.Order, subs []accountrepo.Account, ts time.Time) ([]SubWeight, decimal.Decimal, error) {
	wt := ctypes.GetWalletType(exchange, ord.Symbol.Type)

	switch ord.Symbol.Type {
	case ctypes.MarketTypeSpot:
		if ord.IsBuy {
			asset := strings.ToUpper(ord.Symbol.Quote)
			return e.computeSubWeightsAndUnalloc(ctx, parentID, exchange.String(), asset, wt, subs, ts)
		}
		asset := strings.ToUpper(ord.Symbol.Base)
		return e.computeSubWeightsAndUnalloc(ctx, parentID, exchange.String(), asset, wt, subs, ts)
	case ctypes.MarketTypeFuture:
		if !(ord.Side == ctypes.PositionSideLong && ord.IsBuy) ||
			!(ord.Side == ctypes.PositionSideShort && !ord.IsBuy) {
			asset := strings.ToUpper(ord.Symbol.Quote)
			fw := ctypes.GetWalletType(exchange, ctypes.MarketTypeFuture)
			return e.computeSubWeightsAndUnalloc(ctx, parentID, exchange.String(), asset, fw, subs, ts)
		}
		return e.computeFutureClosePositionWeights(ctx, parentID, exchange, ord, subs, ts)
	default:
		return nil, decimal.Zero, nil
	}
}

func (e *Entity) computeFutureClosePositionWeights(ctx context.Context, parentID string, exchange ctypes.Exchange, ord ctypes.Order, subs []accountrepo.Account, ts time.Time) ([]SubWeight, decimal.Decimal, error) {
	if !ord.Symbol.IsValid() {
		return nil, decimal.Zero, nil
	}
	exStr := exchange.String()
	sym := ord.Symbol.String()
	side := positions.PositionSide(ord.Side.String())

	parentAbs, err := e.absPositionWeightForFanout(ctx, parentID, exStr, sym, side, ts)
	if err != nil {
		return nil, decimal.Zero, err
	}

	var sumChild decimal.Decimal
	weights := make([]SubWeight, 0, len(subs))
	for i := range subs {
		sid := subs[i].ID
		w, err := e.absPositionWeightForFanout(ctx, sid, exStr, sym, side, ts)
		if err != nil {
			return nil, decimal.Zero, err
		}
		weights = append(weights, SubWeight{SubAccountID: sid, W: w})
		sumChild = sumChild.Add(w)
	}

	U := parentAbs.Sub(sumChild)
	if U.IsNegative() {
		U = decimal.Zero
	}
	return weights, U, nil
}

// AttributeMultiBotOrderForFanout：父 multi_bot 下将交易所 Order 归因到 0/1/N 个 virtual_sub（BotId → DB 子行 → 比例；比例与分摊内核一致）。
// 非 multi_bot 父、无子、或无可分摊权重时返回 (nil, nil)。T4 由 applyMultiBotParentOrderStage 在父行落库之后经 PublishEvent 入队，由账户消费者 handleAccountMessage。
func (e *Entity) AttributeMultiBotOrderForFanout(ctx context.Context, parentID string, exchange ctypes.Exchange, ord *ctypes.Order) ([]SubRawDispatch, error) {
	if ord == nil || parentID == "" {
		return nil, nil
	}

	acct, err := e.GetAccount(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if acct == nil || acct.AccountType != ctypes.AccountTypeReal || !acct.MultiBotMode {
		return nil, nil
	}

	subs, err := e.listVirtualSubsForParentFanoutAt(ctx, parentID, ord.CreatedTs)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, nil
	}

	ordCopy := *ord

	// 1) BotId 优先（用于从子账户策略发起的订单）
	if ord.BotID > 0 {
		bot, err := e.db.BotRepo.GetBot(ctx, int32(ord.BotID))
		if err != nil {
			return nil, err
		}
		if bot != nil && strings.TrimSpace(bot.AccountID) != "" {
			if e.accountIsVirtualSubOfParent(ctx, parentID, bot.AccountID) {
				logger.Ctx(ctx).Info().
					Str("parent_id", parentID).
					Str("exchange", exchange.String()).
					Str("order_id", ord.OrderID.String()).
					Str("client_order_id", ord.ClientOrderID.String()).
					Int64("bot_id", ord.BotID).
					Str("symbol", ord.Symbol.String()).
					Msg("multi_bot order fanout: bot_id hit")
				return []SubRawDispatch{{
					SubAccountID: bot.AccountID,
					Share:        decimal.NewFromInt(1),
					Order:        cloneOrderForSub(ordCopy, bot.AccountID),
				}}, nil
			}
		}
		return nil, nil
	}

	// 2) 按 client_order_id 命中子账户订单（用于从子账户手动发起的订单）
	subOrders, err := e.db.OrdersRepo.ListOrdersByClientOrderIdUnderVirtualSubs(ctx, ordersrepo.ListOrdersByClientOrderIdUnderVirtualSubsParams{
		ClientOrderID:   ord.ClientOrderID.String(),
		Exchange:        exchange.String(),
		ParentAccountID: &parentID,
	})
	if err != nil {
		return nil, err
	}
	if len(subOrders) == 1 {
		logger.Ctx(ctx).Info().
			Str("parent_id", parentID).
			Str("exchange", exchange.String()).
			Str("order_id", ord.OrderID.String()).
			Str("client_order_id", ord.ClientOrderID.String()).
			Int64("bot_id", ord.BotID).
			Str("symbol", ord.Symbol.String()).
			Msg("multi_bot order fanout: client_order_id hit")
		return []SubRawDispatch{{
			SubAccountID: subOrders[0].AccountID,
			Share:        decimal.NewFromInt(1),
			Order:        cloneOrderForSub(ordCopy, subOrders[0].AccountID),
		}}, nil
	}

	// 3) 比例：无单子命中时的 N 路分摊（父侧权威行已由上游先落库，不再因 ParentByOrderID 阻断 fanout），以订单创建时间作为分摊的时间点，保证分摊效果的稳定性
	weights, wUnalloc, err := e.computeOrderProportionalWeights(ctx, parentID, exchange, ordCopy, subs, ord.CreatedTs)
	if err != nil {
		return nil, err
	}
	if len(weights) == 0 {
		logger.Ctx(ctx).Warn().
			Str("p2_obs", p2ObsOrderPropEmptyWeights).
			Str("parent_id", parentID).
			Str("exchange", exchange.String()).
			Str("order_id", ord.OrderID.String()).
			Str("client_order_id", ord.ClientOrderID.String()).
			Int64("bot_id", ord.BotID).
			Str("symbol", ord.Symbol.String()).
			Msg("multi_bot order fanout: proportional branch has no weights (falls through to parent row)")
		return nil, nil
	}

	unit := decimal.NewFromInt(1)
	shares, _, err := SplitProportionalDelta(unit, weights, wUnalloc)
	if err != nil {
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
		return nil, nil
	}
	logger.Ctx(ctx).Info().
		Str("shares", formatAnyToJson(shares)).
		Str("parent_id", parentID).
		Str("exchange", exchange.String()).
		Str("order_id", ord.OrderID.String()).
		Str("client_order_id", ord.ClientOrderID.String()).
		Int64("bot_id", ord.BotID).
		Str("symbol", ord.Symbol.String()).
		Msg("multi_bot order fanout: proportional split")
	return buildSubRawDispatchesFromUnitShares(ordCopy, shares), nil
}

// AttributeOrdersFromParent 将父 connector 拉到的在途订单按 multi_bot 归因到本 virtual_sub（含份额缩放）。
// 供 connector.VirtualSubAccountReader 实现，与 WS/Cron 侧 AttributeMultiBotOrderForFanout 语义一致。
func (e *Entity) AttributeOrdersFromParent(ctx context.Context, parentID, subID string, exchange ctypes.Exchange, symbol *ctypes.Symbol, parentOrders []*ctypes.Order) ([]*ctypes.Order, error) {
	_ = symbol // 父订单列表已由交易所按 symbol 过滤
	out := make([]*ctypes.Order, 0)
	for _, po := range parentOrders {
		if po == nil {
			continue
		}
		disp, err := e.AttributeMultiBotOrderForFanout(ctx, parentID, exchange, po)
		if err != nil {
			return nil, err
		}
		scaled, err := e.buildScaledOrdersForMultiBotFanout(ctx, exchange, po, disp)
		if err != nil {
			return nil, err
		}
		for _, d := range disp {
			if d.SubAccountID != subID {
				continue
			}
			o, ok := scaled[d.SubAccountID]
			if !ok {
				o = d.Order
			}
			cp := o
			out = append(out, &cp)
		}
	}
	return out, nil
}

// AttributeOrderFromParent 将父侧单笔订单归因到本 virtual_sub；无派发至本子账户时返回 (nil, nil)。
func (e *Entity) AttributeOrderFromParent(ctx context.Context, parentID, subID string, exchange ctypes.Exchange, symbol ctypes.Symbol, parentOrder *ctypes.Order) (*ctypes.Order, error) {
	_ = symbol
	if parentOrder == nil {
		return nil, nil
	}
	disp, err := e.AttributeMultiBotOrderForFanout(ctx, parentID, exchange, parentOrder)
	if err != nil {
		return nil, err
	}
	scaled, err := e.buildScaledOrdersForMultiBotFanout(ctx, exchange, parentOrder, disp)
	if err != nil {
		return nil, err
	}
	for _, d := range disp {
		if d.SubAccountID != subID {
			continue
		}
		o, ok := scaled[d.SubAccountID]
		if !ok {
			o = d.Order
		}
		cp := o
		return &cp, nil
	}
	return nil, nil
}

func formatAnyToJson(shares map[string]decimal.Decimal) string {
	raw, _ := sonic.Marshal(shares)
	return string(raw)
}
