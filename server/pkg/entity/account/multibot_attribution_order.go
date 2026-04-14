package account

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
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

// scaleOrderForShare 将订单数量/成交额/费用等按归因份额缩放（同一 exchange order_id，多子各记逻辑份额）。
func scaleOrderForShare(ord ctypes.Order, share decimal.Decimal) ctypes.Order {
	if share.LessThanOrEqual(decimal.Zero) || share.GreaterThan(decimal.NewFromInt(1)) {
		return ord
	}
	if share.Equal(decimal.NewFromInt(1)) {
		return ord
	}
	out := ord
	out.OriginalQty = ord.OriginalQty.Mul(share)
	out.ExecutedQty = ord.ExecutedQty.Mul(share)
	out.OriginalQuoteQty = ord.OriginalQuoteQty.Mul(share)
	out.ExecutedQuoteQty = ord.ExecutedQuoteQty.Mul(share)
	if ord.Fee != nil {
		f := ord.Fee.Mul(share)
		out.Fee = &f
	}
	if ord.RealizedPnl != nil {
		p := ord.RealizedPnl.Mul(share)
		out.RealizedPnl = &p
	}
	return out
}

// futureOpenPositionLikeDeriveOrderLocked 与 deriveOrderLocked 中合约「开仓」判定一致。
func futureOpenPositionLikeDeriveOrderLocked(o ctypes.Order) bool {
	if o.Symbol.Type != ctypes.MarketTypeFuture {
		return false
	}
	return (o.Side == ctypes.PositionSideLong && o.IsBuy) ||
		(o.Side == ctypes.PositionSideShort && !o.IsBuy)
}

func absPositionQty(p *positions.Position) decimal.Decimal {
	if p == nil {
		return decimal.Zero
	}
	q := utils.Decimal.PgNumericToDecimal(p.Qty)
	return q.Abs()
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
		if futureOpenPositionLikeDeriveOrderLocked(ord) {
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

	parentPos, err := e.db.PositionsRepo.GetPosition(ctx, positions.GetPositionParams{
		AccountID: parentID,
		Exchange:  exStr,
		Symbol:    sym,
		Side:      side,
	})
	if err != nil {
		return nil, decimal.Zero, err
	}
	parentAbs := absPositionQty(parentPos)

	var sumChild decimal.Decimal
	weights := make([]SubWeight, 0, len(subs))
	for i := range subs {
		sid := subs[i].ID
		pos, err := e.db.PositionsRepo.GetPosition(ctx, positions.GetPositionParams{
			AccountID: sid,
			Exchange:  exStr,
			Symbol:    sym,
			Side:      side,
		})
		if err != nil {
			return nil, decimal.Zero, err
		}
		w := absPositionQty(pos)
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

	subs, err := e.db.AccountRepo.ListVirtualSubByParent(ctx, &parentID)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, nil
	}

	ordCopy := *ord

	// 1) BotId 优先（§9.3：<=0 视为无效）
	if ord.BotID > 0 {
		bot, err := e.db.BotRepo.GetBot(ctx, int32(ord.BotID))
		if err != nil {
			return nil, err
		}
		if bot != nil && strings.TrimSpace(bot.AccountID) != "" {
			if e.accountIsVirtualSubOfParent(ctx, parentID, bot.AccountID) {
				return []SubRawDispatch{{
					SubAccountID: bot.AccountID,
					Share:        decimal.NewFromInt(1),
					Order:        cloneOrderForSub(ordCopy, bot.AccountID),
				}}, nil
			}
		}
		return nil, nil
	}

	// 2) 比例：无单子命中时的 N 路分摊（父侧权威行已由上游先落库，不再因 ParentByOrderID 阻断 fanout），以订单创建时间作为分摊的时间点，保证分摊效果的稳定性
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
		for _, d := range disp {
			if d.SubAccountID != subID {
				continue
			}
			o := d.Order
			if !d.Share.Equal(decimal.NewFromInt(1)) {
				o = scaleOrderForShare(o, d.Share)
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
	for _, d := range disp {
		if d.SubAccountID != subID {
			continue
		}
		o := d.Order
		if !d.Share.Equal(decimal.NewFromInt(1)) {
			o = scaleOrderForShare(o, d.Share)
		}
		cp := o
		return &cp, nil
	}
	return nil, nil
}
