package simulate

import (
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func (c *Connector) buildDiffAndMakerMessages(symbol ctypes.Symbol, events []MatchEvent, before, after AccountSnapshot) []*ctypes.Message {
	if len(events) == 0 {
		return nil
	}
	out := make([]*ctypes.Message, 0)
	matched := false
	for _, ev := range events {
		if ev.Order == nil || ev.Order.AccountID != c.accountID {
			continue
		}
		matched = true
		for _, f := range ev.Fills {
			out = append(out, c.newOrderFillMessage(symbol, *ev.Order, f))
		}
	}
	if matched {
		out = append(out, c.buildSnapshotDiffMessages(symbol, before, after)...)
	}
	return out
}

// publishOrderAcceptedNew emits an order lifecycle message with status NEW right after the engine accepts the order (before matching).
func (c *Connector) publishOrderAcceptedNew(symbol ctypes.Symbol, o Order) {
	to := toTypesOrder(c.exchange, &o)
	to.Symbol = symbol
	to.Status = ctypes.OrderStatusNew
	if m := c.newOrderLifecycleMessage(to); m != nil {
		c.publishAccountMessage(m)
	}
}

// publishPlaceOrderOutcome publishes taker outcomes: per-fill trade events, then order lifecycle if there were no fills, then balance/position snapshots.
func (c *Connector) publishPlaceOrderOutcome(symbol ctypes.Symbol, before, after AccountSnapshot, res *PlaceOrderResult) {
	if res == nil {
		return
	}
	for _, f := range res.Fills {
		if m := c.newOrderFillMessage(symbol, res.Order, f); m != nil {
			c.publishAccountMessage(m)
		}
	}
	if len(res.Fills) == 0 && res.Order.Status != OrderStatusNew {
		if m := c.newOrderLifecycleMessage(toTypesOrder(c.exchange, &res.Order)); m != nil {
			c.publishAccountMessage(m)
		}
	}
	for _, m := range c.buildSnapshotDiffMessages(symbol, before, after) {
		if m != nil {
			c.publishAccountMessage(m)
		}
	}
}

func (c *Connector) buildSnapshotDiffMessages(symbol ctypes.Symbol, before, after AccountSnapshot) []*ctypes.Message {
	now := time.Now().UTC()
	out := make([]*ctypes.Message, 0)
	changedAssets := make([]*ctypes.AssetEvent, 0)
	assetSeen := map[BalanceKey]struct{}{}
	for k, v := range before.Bal {
		assetSeen[k] = struct{}{}
		av := after.Bal[k]
		if !av.Equal(v) {
			b := av
			changedAssets = append(changedAssets, &ctypes.AssetEvent{
				WalletType: k.Wallet,
				Code:       string(k.Asset),
				Balance:    &b,
				Locked:     lo.ToPtr(decimal.Zero),
				UpdatedTs:  now,
			})
		}
	}
	for k, v := range after.Bal {
		if _, ok := assetSeen[k]; ok {
			continue
		}
		if !v.IsZero() {
			b := v
			changedAssets = append(changedAssets, &ctypes.AssetEvent{
				WalletType: k.Wallet,
				Code:       string(k.Asset),
				Balance:    &b,
				Locked:     lo.ToPtr(decimal.Zero),
				UpdatedTs:  now,
			})
		}
	}
	if len(changedAssets) > 0 {
		out = append(out, ctypes.NewMessage(c.exchange, ctypes.StreamSelector{Stream: ctypes.StreamTypeAccountRaw, Account: lo.ToPtr(c.accountID)}, ctypes.BalanceUpdate{
			EventID: c.nextEventID(),
			Type:    ctypes.UpdateTypeSnapshot,
			Reason:  ctypes.LedgerReasonFill,
			Assets:  changedAssets,
		}, now))
	}
	out = append(out, c.buildPositionDiffMessages(symbol, before, after)...)
	return out
}

func (c *Connector) buildPositionDiffMessages(symbol ctypes.Symbol, before, after AccountSnapshot) []*ctypes.Message {
	now := time.Now().UTC()
	var out []*ctypes.Message
	if after.Mode == PositionModeHedge {
		msgs := c.diffHedgeLeg(symbol, ctypes.PositionSideLong, before.Slot.Long, after.Slot.Long, now)
		out = append(out, msgs...)
		msgs = c.diffHedgeLeg(symbol, ctypes.PositionSideShort, before.Slot.Short, after.Slot.Short, now)
		out = append(out, msgs...)
		return out
	}
	posChanged := !before.Slot.Net.Qty.Equal(after.Slot.Net.Qty) ||
		!before.Slot.Net.EntryPrice.Equal(after.Slot.Net.EntryPrice) ||
		before.Slot.Net.Leverage != after.Slot.Net.Leverage
	if !posChanged {
		return out
	}
	side := ctypes.PositionSideLong
	amount := after.Slot.Net.Qty
	if after.Slot.Net.Qty.Sign() < 0 {
		side = ctypes.PositionSideShort
		amount = amount.Abs()
	}
	out = append(out, ctypes.NewMessage(c.exchange, ctypes.StreamSelector{Stream: ctypes.StreamTypeAccountRaw, Account: lo.ToPtr(c.accountID)}, ctypes.PositionsUpdate{
		EventID: c.nextEventID(),
		Type:    ctypes.UpdateTypeSnapshot,
		Positions: []*ctypes.Position{
			{
				AccountID:     c.accountID,
				Exchange:      c.exchange,
				Symbol:        symbol,
				Side:          side,
				Amount:        amount,
				EntryPrice:    after.Slot.Net.EntryPrice,
				InitialMargin: after.Slot.Net.UsedMargin,
				Leverage:      int(after.Slot.Net.Leverage),
				UpdatedTs:     now,
			},
		},
	}, now))
	return out
}

func (c *Connector) diffHedgeLeg(symbol ctypes.Symbol, side ctypes.PositionSide, bLeg, aLeg PerpLeg, now time.Time) []*ctypes.Message {
	changed := !bLeg.Qty.Equal(aLeg.Qty) || !bLeg.EntryPrice.Equal(aLeg.EntryPrice) || bLeg.Leverage != aLeg.Leverage
	if !changed {
		return nil
	}
	if aLeg.Qty.IsZero() {
		return nil
	}
	return []*ctypes.Message{ctypes.NewMessage(c.exchange, ctypes.StreamSelector{Stream: ctypes.StreamTypeAccountRaw, Account: lo.ToPtr(c.accountID)}, ctypes.PositionsUpdate{
		EventID: c.nextEventID(),
		Type:    ctypes.UpdateTypeSnapshot,
		Positions: []*ctypes.Position{
			{
				AccountID:     c.accountID,
				Exchange:      c.exchange,
				Symbol:        symbol,
				Side:          side,
				Amount:        aLeg.Qty,
				EntryPrice:    aLeg.EntryPrice,
				InitialMargin: aLeg.UsedMargin,
				Leverage:      int(aLeg.Leverage),
				UpdatedTs:     now,
			},
		},
	}, now)}
}

func (c *Connector) newOrderLifecycleMessage(order *ctypes.Order) *ctypes.Message {
	if order == nil {
		return nil
	}
	if order.UpdatedTs.IsZero() {
		order.UpdatedTs = time.Now().UTC()
	}
	order.Raw = c.mustEventMetaJSON(order.UpdatedTs)
	return ctypes.NewMessage(c.exchange, ctypes.StreamSelector{Stream: ctypes.StreamTypeAccountRaw, Account: lo.ToPtr(c.accountID)}, order, order.UpdatedTs)
}

func (c *Connector) newOrderFillMessage(symbol ctypes.Symbol, od Order, fill Fill) *ctypes.Message {
	ts := time.Now().UTC()
	order := toTypesOrder(c.exchange, &od)
	order.Symbol = symbol
	order.ExecutedQty = fill.Size
	order.ExecutedQuoteQty = fill.Price.Mul(fill.Size)
	order.AvgPrice = fill.Price
	order.Price = fill.Price
	order.Status = ctypes.OrderStatusPartialDone
	if od.QtyRemaining.IsZero() {
		order.Status = ctypes.OrderStatusDone
	}
	order.UpdatedTs = ts
	order.Raw = c.mustEventMetaJSON(ts)
	return ctypes.NewMessage(c.exchange, ctypes.StreamSelector{Stream: ctypes.StreamTypeAccountRaw, Account: lo.ToPtr(c.accountID)}, order, ts)
}
