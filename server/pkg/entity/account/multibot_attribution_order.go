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

// computeOrderProportionalWeights 无 BotId / 无 DB 子命中时的比例权重（与 P2 T0 §3/§4 对齐）。
func (e *Entity) computeOrderProportionalWeights(ctx context.Context, parentID string, exchange ctypes.Exchange, ord ctypes.Order, subs []accountrepo.Account) ([]SubWeight, decimal.Decimal, error) {
	wt := ctypes.GetWalletType(exchange, ord.Symbol.Type)

	switch ord.Symbol.Type {
	case ctypes.MarketTypeSpot:
		// P2 T0: spot dimension see docs/P2_T0_VIRTUAL_SUB_ATTRIBUTION.md §4
		if ord.IsBuy {
			asset := strings.ToUpper(ord.Symbol.Quote)
			return e.computeSubWeightsAndUnalloc(ctx, parentID, exchange.String(), asset, wt, subs, time.Time{})
		}
		asset := strings.ToUpper(ord.Symbol.Base)
		return e.computeSubWeightsAndUnalloc(ctx, parentID, exchange.String(), asset, wt, subs, time.Time{})

	case ctypes.MarketTypeFuture:
		if futureOpenPositionLikeDeriveOrderLocked(ord) {
			asset := strings.ToUpper(ord.Symbol.Quote)
			fw := ctypes.GetWalletType(exchange, ctypes.MarketTypeFuture)
			return e.computeSubWeightsAndUnalloc(ctx, parentID, exchange.String(), asset, fw, subs, time.Time{})
		}
		return e.computeFutureClosePositionWeights(ctx, parentID, exchange, ord, subs)

	default:
		return nil, decimal.Zero, nil
	}
}

func (e *Entity) computeFutureClosePositionWeights(ctx context.Context, parentID string, exchange ctypes.Exchange, ord ctypes.Order, subs []accountrepo.Account) ([]SubWeight, decimal.Decimal, error) {
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
// 非 multi_bot 父、无子、或无可分摊权重时返回 (nil, nil)。T4 由 applyMultiBotParentOrderStage 在父行落库之后合成 account_raw 并调用 handleAccountMessage。
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

	pid := parentID
	subs, err := e.db.AccountRepo.ListVirtualSubByParent(ctx, &pid)
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
	}

	lookup, err := e.loadMultiBotOrderLookup(ctx, parentID, exchange, ord)
	if err != nil {
		return nil, err
	}

	// 2) DB：子账户上已有 order_id 行
	if lookup.ChildByOrderID != nil && e.accountIsVirtualSubOfParent(ctx, parentID, lookup.ChildByOrderID.AccountID) {
		sid := lookup.ChildByOrderID.AccountID
		return []SubRawDispatch{{
			SubAccountID: sid,
			Share:        decimal.NewFromInt(1),
			Order:        cloneOrderForSub(ordCopy, sid),
		}}, nil
	}

	// 3) DB：父树下 client_order_id 命中子账户行
	if lookup.UnderParentByClientOrderID != nil {
		sid := lookup.UnderParentByClientOrderID.AccountID
		if sid != parentID && e.accountIsVirtualSubOfParent(ctx, parentID, sid) {
			return []SubRawDispatch{{
				SubAccountID: sid,
				Share:        decimal.NewFromInt(1),
				Order:        cloneOrderForSub(ordCopy, sid),
			}}, nil
		}
	}

	// 4) 比例：无单子命中时的 N 路分摊（父侧权威行已由上游先落库，不再因 ParentByOrderID 阻断 fanout）
	weights, wUnalloc, err := e.computeOrderProportionalWeights(ctx, parentID, exchange, ordCopy, subs)
	if err != nil {
		return nil, err
	}
	if len(weights) == 0 {
		logP2T6OrderProportionalEmptyWeights(ctx, parentID, exchange, &ordCopy)
		return nil, nil
	}

	unit := decimal.NewFromInt(1)
	shares, _, err := SplitProportionalDelta(unit, weights, wUnalloc)
	if err != nil {
		logP2T6OrderProportionalZeroDenom(ctx, parentID, exchange, &ordCopy, weights, wUnalloc)
		return nil, nil
	}
	return buildSubRawDispatchesFromUnitShares(ordCopy, shares), nil
}
