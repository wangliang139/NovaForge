package converter

import (
	"github.com/shopspring/decimal"
	"github.com/wangliang139/llt-trade/server/pkg/action/model"
	"github.com/wangliang139/llt-trade/server/pkg/types"
)

func streamEnvelopeEventTs(env *types.Envelope) int {
	if env == nil {
		return 0
	}
	return int(env.Ts)
}

func decimalPtrString(d *decimal.Decimal) string {
	if d == nil {
		return "0"
	}
	return d.String()
}

func assetEventsToStreamAssets(events []*types.AssetEvent) []*model.AccountStreamAsset {
	out := make([]*model.AccountStreamAsset, 0, len(events))
	for _, a := range events {
		if a == nil {
			continue
		}
		out = append(out, &model.AccountStreamAsset{
			WalletType: WalletTypeTypes2Gql(a.WalletType),
			Code:       a.Code,
			Balance:    decimalPtrString(a.Balance),
			Locked:     decimalPtrString(a.Locked),
			UpdatedTs:  int(a.UpdatedTs.UnixMilli()),
		})
	}
	return out
}

func accountStreamUpdateType(t types.UpdateType) model.AccountStreamUpdateType {
	switch t {
	case types.UpdateTypeIncrement:
		return model.AccountStreamUpdateTypeIncrement
	default:
		return model.AccountStreamUpdateTypeSnapshot
	}
}

func positionsToGqlList(in []*types.Position) []*model.Position {
	out := make([]*model.Position, 0, len(in))
	for _, p := range in {
		if p == nil {
			continue
		}
		if g := PositionTypes2Gql(p); g != nil {
			out = append(out, g)
		}
	}
	return out
}

func ConvertStreamEvent(stype model.StreamType, resp *types.SubscribeStreamResponse) *model.StreamEvent {
	if resp == nil || resp.Envelope == nil {
		return nil
	}
	ets := streamEnvelopeEventTs(resp.Envelope)
	msg := resp.Envelope.Payload
	switch {
	case msg.Ticker != nil:
		if t := TickerTypes2Gql(msg.Ticker); t != nil {
			return &model.StreamEvent{Type: stype, EventTs: ets, Ticker: t}
		}
	case msg.Trade != nil:
		if t := TradeTypes2Gql(msg.Trade); t != nil {
			return &model.StreamEvent{Type: stype, EventTs: ets, Trade: t}
		}
	case msg.Depth != nil:
		if d := OrderBookTypes2Gql(msg.Depth); d != nil {
			return &model.StreamEvent{Type: stype, EventTs: ets, Depth: d}
		}
	case msg.Kline != nil:
		if k := KlineTypes2Gql(msg.Kline); k != nil {
			return &model.StreamEvent{Type: stype, EventTs: ets, Kline: k}
		}
	case msg.MarkPrice != nil:
		if mp := MarkPriceTypes2Gql(msg.MarkPrice); mp != nil {
			return &model.StreamEvent{Type: stype, EventTs: ets, MarkPrice: mp}
		}
	case msg.Social != nil:
		if doc := DocumentTypes2Gql(msg.Social); doc != nil {
			return &model.StreamEvent{Type: stype, EventTs: ets, Social: doc}
		}
	case msg.BalanceSnapshot != nil:
		bs := msg.BalanceSnapshot
		scope := make([]model.WalletType, 0, len(bs.Scope))
		for _, w := range bs.Scope {
			scope = append(scope, WalletTypeTypes2Gql(w))
		}
		assets := assetEventsToStreamAssets(bs.Assets)
		if len(scope) == 0 && len(assets) == 0 {
			return nil
		}
		return &model.StreamEvent{
			Type:    stype,
			EventTs: ets,
			BalanceSnapshot: &model.AccountBalanceSnapshot{
				Scope:  scope,
				Assets: assets,
			},
		}
	case msg.BalanceUpdate != nil:
		bu := msg.BalanceUpdate
		assets := assetEventsToStreamAssets(bu.Assets)
		if len(assets) == 0 {
			return nil
		}
		return &model.StreamEvent{
			Type:    stype,
			EventTs: ets,
			BalanceUpdate: &model.AccountBalanceUpdate{
				EventID: bu.EventID,
				Type:    accountStreamUpdateType(bu.Type),
				Reason:  string(bu.Reason),
				Assets:  assets,
			},
		}
	case msg.PositionSnapshot != nil:
		ps := msg.PositionSnapshot
		positions := positionsToGqlList(ps.Positions)
		if len(positions) == 0 {
			return nil
		}
		return &model.StreamEvent{
			Type:    stype,
			EventTs: ets,
			PositionSnapshot: &model.AccountPositionSnapshot{
				Positions: positions,
			},
		}
	case msg.PositionsUpdate != nil:
		pu := msg.PositionsUpdate
		positions := positionsToGqlList(pu.Positions)
		if len(positions) == 0 {
			return nil
		}
		return &model.StreamEvent{
			Type:    stype,
			EventTs: ets,
			PositionsUpdate: &model.AccountPositionsUpdate{
				EventID:   pu.EventID,
				Type:      accountStreamUpdateType(pu.Type),
				Reason:    pu.Reason,
				Positions: positions,
			},
		}
	case msg.Order != nil:
		if o := OrderTypes2Gql(msg.Order); o != nil {
			return &model.StreamEvent{Type: stype, EventTs: ets, Order: o}
		}
	case msg.SymbolLeverage != nil:
		if sl := SymbolLeverageTypes2Gql(msg.SymbolLeverage); sl != nil {
			return &model.StreamEvent{Type: stype, EventTs: ets, SymbolLeverage: sl}
		}
	case msg.Fill != nil:
		if f := FillTypes2Gql(msg.Fill); f != nil {
			return &model.StreamEvent{Type: stype, EventTs: ets, Fill: f}
		}
	default:
		return nil
	}
	return nil
}
