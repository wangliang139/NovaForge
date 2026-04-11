package converter

import (
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/action/model"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

func MarketTypes2Gql(m *ctypes.Market) *model.Market {
	if m == nil {
		return nil
	}
	orderTypes := make([]*model.MarketOrderType, 0, len(m.OrderTypeRules))
	for i := range m.OrderTypeRules {
		ot := &m.OrderTypeRules[i]
		orderTypes = append(orderTypes, &model.MarketOrderType{
			OrderType: ot.OrderType.String(),
			Rules:     MarketRulesTypes2Gql(&ot.Rules),
		})
	}
	return &model.Market{
		Exchange:            m.Exchange,
		Symbol:              m.Symbol.String(),
		Status:              string(m.Status),
		BaseAssetPrecision:  lo.ToPtr(m.BaseAssetPrecision),
		QuoteAssetPrecision: lo.ToPtr(m.QuoteAssetPrecision),
		PricePrecision:      lo.ToPtr(m.PricePrecision),
		Rules:               MarketRulesTypes2Gql(&m.Rules),
		SupportOrderTypes:   orderTypes,
	}
}

func MarketRulesTypes2Gql(r *ctypes.MarketRules) *model.MarketRules {
	if r == nil {
		return nil
	}
	out := &model.MarketRules{}
	if r.MaxOrderNum != 0 {
		out.MaxOrderNum = lo.ToPtr(r.MaxOrderNum)
	}
	if !r.MinPrice.IsZero() {
		s := r.MinPrice.String()
		out.MinPrice = &s
	}
	if !r.MaxPrice.IsZero() {
		s := r.MaxPrice.String()
		out.MaxPrice = &s
	}
	if !r.TickSize.IsZero() {
		s := r.TickSize.String()
		out.TickSize = &s
	}
	if !r.MinQuantity.IsZero() {
		s := r.MinQuantity.String()
		out.MinQuantity = &s
	}
	if !r.MaxQuantity.IsZero() {
		s := r.MaxQuantity.String()
		out.MaxQuantity = &s
	}
	if !r.LotSize.IsZero() {
		s := r.LotSize.String()
		out.LotSize = &s
	}
	if !r.MinNotional.IsZero() {
		s := r.MinNotional.String()
		out.MinNotional = &s
	}
	if !r.MaxNotional.IsZero() {
		s := r.MaxNotional.String()
		out.MaxNotional = &s
	}
	return out
}

func KlineTypes2Gql(k *ctypes.Kline) *model.Kline {
	if k == nil {
		return nil
	}
	return &model.Kline{
		Interval:    k.Interval.String(),
		Open:        k.Open.String(),
		High:        k.High.String(),
		Low:         k.Low.String(),
		Close:       k.Close.String(),
		Volume:      k.Volume.String(),
		QuoteVolume: k.QuoteVolume.String(),
		Trades:      int(k.Trades),
		OpenTs:      int(k.OpenTs.UnixMilli()),
		CloseTs:     int(k.CloseTs.UnixMilli()),
	}
}

func TickerTypes2Gql(t *ctypes.Ticker) *model.Ticker {
	if t == nil {
		return nil
	}
	return &model.Ticker{
		Exchange:       t.Exchange,
		Symbol:         t.Symbol.String(),
		LastPrice:      t.LastPrice.String(),
		Open24h:        t.Open24.String(),
		High24h:        t.High24.String(),
		Low24h:         t.Low24.String(),
		Avg24h:         t.Avg24.String(),
		Volume24h:      t.Volume24.String(),
		QuoteVolume24h: t.QuoteVolume24.String(),
		Ts:             int(t.Ts.UnixMilli()),
	}
}

func TradeTypes2Gql(t *ctypes.Trade) *model.Trade {
	if t == nil {
		return nil
	}
	return &model.Trade{
		TradeID:  t.TradeID,
		Exchange: t.Exchange,
		Symbol:   t.Symbol.String(),
		Price:    t.Price.String(),
		Size:     t.Size.String(),
		IsBuy:    t.IsBuy,
		Ts:       int(t.Ts.UnixMilli()),
	}
}

func OrderBookTypes2Gql(b *ctypes.OrderBook) *model.OrderBook {
	if b == nil {
		return nil
	}
	out := &model.OrderBook{
		Bids:      make([]*model.OrderPriceLevel, 0, len(b.Bids)),
		Asks:      make([]*model.OrderPriceLevel, 0, len(b.Asks)),
		Ts:        int(b.Ts.UnixMilli()),
		SeqID:     int(b.SeqId),
		PrevSeqID: int(b.PrevSeqId),
	}
	for _, bid := range b.Bids {
		out.Bids = append(out.Bids, &model.OrderPriceLevel{
			Price: bid.Price.String(),
			Size:  bid.Size.String(),
		})
	}
	for _, ask := range b.Asks {
		out.Asks = append(out.Asks, &model.OrderPriceLevel{
			Price: ask.Price.String(),
			Size:  ask.Size.String(),
		})
	}
	return out
}

func MarkPriceTypes2Gql(m *ctypes.MarkPrice) *model.MarkPrice {
	if m == nil {
		return nil
	}
	return &model.MarkPrice{
		Exchange:  m.Exchange,
		Symbol:    m.Symbol.String(),
		MarkPrice: m.MarkPrice.String(),
		Ts:        int(m.Ts.UnixMilli()),
	}
}

func IndexPriceTypes2Gql(p *ctypes.IndexPrice) *model.IndexPrice {
	if p == nil {
		return nil
	}
	return &model.IndexPrice{
		Exchange:   p.Exchange,
		Symbol:     p.Symbol.String(),
		IndexPrice: p.IndexPrice.String(),
		Ts:         int(p.Ts.UnixMilli()),
	}
}

func FundingRateTypes2Gql(f *ctypes.FundingRate) *model.FundingRate {
	if f == nil {
		return nil
	}
	return &model.FundingRate{
		Exchange:        f.Exchange,
		Symbol:          f.Symbol.String(),
		FundingRate:     f.FundingRate.String(),
		InterestRate:    f.InterestRate.String(),
		NextFundingTime: int(f.NextFundingTime.UnixMilli()),
		Ts:              int(f.Ts.UnixMilli()),
	}
}

func IndexComponentTypes2Gql(c *ctypes.IndexComponent) *model.IndexComponent {
	if c == nil {
		return nil
	}
	out := &model.IndexComponent{
		Exchange:   c.Exchange,
		Symbol:     c.Symbol.String(),
		Components: make([]*model.IndexComponentItem, 0, len(c.Components)),
		Ts:         int(c.Ts.UnixMilli()),
	}
	if c.Price.IsPositive() {
		s := c.Price.String()
		out.Price = &s
	}
	for i := range c.Components {
		co := &c.Components[i]
		out.Components = append(out.Components, &model.IndexComponentItem{
			Exchange: co.Exchange,
			Symbol:   co.Symbol,
			Price:    co.Price.String(),
			Weight:   co.Weight.String(),
		})
	}
	return out
}

func LeverageBracketTypes2Gql(lb *ctypes.LeverageBracket) *model.LeverageBracket {
	if lb == nil {
		return nil
	}
	out := &model.LeverageBracket{
		Symbol:   lb.Symbol.String(),
		Brackets: make([]*model.Bracket, 0, len(lb.Brackets)),
	}
	for _, b := range lb.Brackets {
		out.Brackets = append(out.Brackets, &model.Bracket{
			Bracket:     b.Bracket,
			MaxLeverage: float64(b.MaxLeverage),
			MinNotional: b.MinNotional.String(),
			MaxNotional: b.MaxNotional.String(),
			Mmr:         b.Mmr.String(),
			Cum:         b.Cum.String(),
		})
	}
	return out
}

func MarketTypeGql2Types(mt model.MarketType) types.MarketType {
	switch mt {
	case model.MarketTypeSpot:
		return types.MarketTypeSpot
	case model.MarketTypeFuture:
		return types.MarketTypeFuture
	default:
		return types.MarketType("")
	}
}

func MarketTypeTypes2Gql(marketType types.MarketType) model.MarketType {
	switch marketType {
	case types.MarketTypeSpot:
		return model.MarketTypeSpot
	case types.MarketTypeFuture:
		return model.MarketTypeFuture
	default:
		return model.MarketTypeUnspecified
	}
}
