package binance

import (
	"context"
	"fmt"
	"strconv"
	"time"

	binance "github.com/adshao/go-binance/v2"
	spot "github.com/adshao/go-binance/v2"
	future "github.com/adshao/go-binance/v2/futures"
	futures "github.com/adshao/go-binance/v2/futures"
	portfolio "github.com/adshao/go-binance/v2/portfolio"
	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/market/misc"
	"github.com/wangliang139/mow/number"
)

func FormatSymbol(symbol ctypes.Symbol) string {
	return fmt.Sprintf("%s%s", symbol.Base, symbol.Quote)
}

func ParseInterval(interval ctypes.Interval) (Interval, error) {
	switch interval {
	case ctypes.Interval1s:
		return Interval1s, nil
	case ctypes.Interval1m:
		return Interval1m, nil
	case ctypes.Interval5m:
		return Interval5m, nil
	case ctypes.Interval15m:
		return Interval15m, nil
	case ctypes.Interval30m:
		return Interval30m, nil
	case ctypes.Interval1h:
		return Interval1h, nil
	case ctypes.Interval4h:
		return Interval4h, nil
	case ctypes.Interval12h:
		return Interval12h, nil
	case ctypes.Interval1d:
		return Interval1d, nil
	case ctypes.Interval3d:
		return Interval3d, nil
	case ctypes.Interval1w:
		return Interval1w, nil
	case ctypes.Interval1M:
		return Interval1M, nil
	}
	return "", fmt.Errorf("invalid interval: %s", interval)
}

func ConvertMarketStatus2Types(status string) ctypes.MarketStatus {
	switch status {
	case "TRADING":
		return ctypes.MarketStatusTrading
	case "END_OF_DAY":
		return ctypes.MarketStatusClosing
	case "BREAK":
		return ctypes.MarketStatusSuspended
	case "PENDING_TRADING":
		return ctypes.MarketStatusPending
	case "PRE_DELIVERING", "DELIVERING", "DELIVERED":
		return ctypes.MarketStatusDelivering
	case "PRE_SETTLE", "SETTLING", "SETTLED":
		return ctypes.MarketStatusSettling
	case "HALT", "CLOSE":
		return ctypes.MarketStatusDelisted
	}
	panic(fmt.Sprintf("invalid market status: %s", status))
}

func ConvertPositionSide2Types(side string) ctypes.PositionSide {
	switch side {
	case "LONG":
		return ctypes.PositionSideLong
	case "SHORT":
		return ctypes.PositionSideShort
	case "BOTH":
		return ctypes.PositionSideLong
	}
	panic(fmt.Errorf("invalid position side: %s", side))
}

func BuildSymbolMarketRules(filters []map[string]any) (result ctypes.MarketRules, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("convert symbol market rules: %v", r)
			result = ctypes.MarketRules{}
		}
	}()
	for _, filter := range filters {
		filterType, ok := misc.MapValue[map[string]any, string](filter, "filterType")
		if !ok {
			continue
		}
		switch filterType {
		case "PRICE_FILTER":
			result.MinPrice = misc.MustMapString2Decimal(filter, []string{"minPrice"})
			result.MaxPrice = misc.MustMapString2Decimal(filter, []string{"maxPrice"})
			result.TickSize = misc.MustMapString2Decimal(filter, []string{"tickSize"})
		case "LOT_SIZE":
			result.MinQuantity = misc.MustMapString2Decimal(filter, []string{"minQty"})
			result.MaxQuantity = misc.MustMapString2Decimal(filter, []string{"maxQty"})
			result.LotSize = misc.MustMapString2Decimal(filter, []string{"stepSize"})
		case "MAX_NUM_ORDERS":
			result.MaxOrderNum = misc.MapValueWithDefault(filter, "maxNumOrders", 0)
		case "MIN_NOTIONAL":
			if result.MinNotional.IsZero() {
				result.MinNotional = misc.MustMapString2Decimal(filter, []string{"minNotional"})
			}
			result.MaxNotional = misc.MustMapString2Decimal(filter, []string{"maxNotional"})
		case "NOTIONAL":
			if result.MinNotional.IsZero() {
				result.MinNotional = misc.MustMapString2Decimal(filter, []string{"notional"})
			}
		}
	}
	return result, nil
}

func BuildOrderTypeMarketRules(orderType ctypes.OrderType, filters []map[string]any) (result ctypes.MarketRules, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("convert order type market rules: %v", r)
			result = ctypes.MarketRules{}
		}
	}()
	if orderType != ctypes.OrderTypeMarket {
		return result, nil
	}
	for _, filter := range filters {
		filterType, ok := misc.MapValue[map[string]any, string](filter, "filterType")
		if !ok {
			continue
		}
		switch filterType {
		case "MARKET_LOT_SIZE":
			result.MinQuantity = misc.MustMapString2Decimal(filter, []string{"minQty"})
			result.MaxQuantity = misc.MustMapString2Decimal(filter, []string{"maxQty"})
			result.LotSize = misc.MustMapString2Decimal(filter, []string{"stepSize"})
		}
	}
	return result, nil
}

func ConvertSpotMarket2Types(exchange ctypes.Exchange, market *spot.Symbol) (result *ctypes.Market, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("convert spot market to types: %v", r)
			result = nil
		}
	}()

	marketRules, err := BuildSymbolMarketRules(market.Filters)
	if err != nil {
		return nil, fmt.Errorf("build spot market rules: %w", err)
	}

	supportOrderTypes := make([]ctypes.OrderTypeRule, 0, len(market.OrderTypes))
	for _, item := range market.OrderTypes {
		orderType, ok := MapSpotOrderType2Types[item]
		if !ok {
			continue
		}
		orderTypeRules, err := BuildOrderTypeMarketRules(orderType, market.Filters)
		if err != nil {
			return nil, fmt.Errorf("build order type market rules: %w", err)
		}
		supportOrderTypes = append(supportOrderTypes, ctypes.OrderTypeRule{
			OrderType: orderType,
			Rules:     orderTypeRules,
		})
	}
	return &ctypes.Market{
		Exchange: exchange,
		Symbol: ctypes.Symbol{
			Base:  market.BaseAsset,
			Quote: market.QuoteAsset,
			Type:  ctypes.MarketTypeSpot,
		},
		Status:              ConvertMarketStatus2Types(market.Status),
		BaseAssetPrecision:  market.BaseAssetPrecision,
		QuoteAssetPrecision: number.GetPrecision(marketRules.TickSize.String()),
		PricePrecision:      number.GetPrecision(marketRules.TickSize.String()),
		Rules:               marketRules,
		OrderTypeRules:      supportOrderTypes,
	}, nil
}

func ConvertFutureMarket2Types(exchange ctypes.Exchange, market *future.Symbol) (result *ctypes.Market, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("convert future market to types: %v", r)
			result = nil
		}
	}()
	marketRules, err := BuildSymbolMarketRules(market.Filters)
	if err != nil {
		return nil, fmt.Errorf("build future market rules: %w", err)
	}

	supportOrderTypes := make([]ctypes.OrderTypeRule, 0, len(market.OrderType))
	for _, item := range market.OrderType {
		orderType, ok := MapFutureOrderType2Types[string(item)]
		if !ok {
			continue
		}
		orderTypeRules, err := BuildOrderTypeMarketRules(orderType, market.Filters)
		if err != nil {
			return nil, fmt.Errorf("build order type market rules: %w", err)
		}
		supportOrderTypes = append(supportOrderTypes, ctypes.OrderTypeRule{
			OrderType: orderType,
			Rules:     orderTypeRules,
		})
	}
	return &ctypes.Market{
		Exchange: exchange,
		Symbol: ctypes.Symbol{
			Base:  market.BaseAsset,
			Quote: market.QuoteAsset,
			Type:  ctypes.MarketTypeFuture,
		},
		Status:              ConvertMarketStatus2Types(market.Status),
		BaseAssetPrecision:  market.QuantityPrecision,
		QuoteAssetPrecision: market.PricePrecision,
		PricePrecision:      number.GetPrecision(marketRules.TickSize.String()),
		Rules:               marketRules,
		OrderTypeRules:      supportOrderTypes,
	}, nil
}

func ConvertSpotTicker2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, event *spot.WsMarketStatEvent) (*ctypes.Ticker, error) {
	price, err := decimal.NewFromString(event.LastPrice)
	if err != nil {
		return nil, fmt.Errorf("parse price: %w", err)
	}
	open24 := number.DecimalFromString(event.OpenPrice)
	high24, err := decimal.NewFromString(event.HighPrice)
	if err != nil {
		return nil, fmt.Errorf("parse high24: %w", err)
	}
	low24, err := decimal.NewFromString(event.LowPrice)
	if err != nil {
		return nil, fmt.Errorf("parse low24: %w", err)
	}
	volume24, err := decimal.NewFromString(event.BaseVolume)
	if err != nil {
		return nil, fmt.Errorf("parse volume24: %w", err)
	}
	quoteVolume24, err := decimal.NewFromString(event.QuoteVolume)
	if err != nil {
		return nil, fmt.Errorf("parse quote volume24: %w", err)
	}
	avg24, err := decimal.NewFromString(event.WeightedAvgPrice)
	if err != nil {
		return nil, fmt.Errorf("parse avg24: %w", err)
	}
	return &ctypes.Ticker{
		Exchange:      exchange,
		Symbol:        symbol,
		LastPrice:     price,
		Open24:        open24,
		High24:        high24,
		Low24:         low24,
		Avg24:         avg24,
		Volume24:      volume24,
		QuoteVolume24: quoteVolume24,
		Ts:            time.UnixMilli(event.Time),
	}, nil
}

func ConvertFutureTicker2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, event *future.WsMarketTickerEvent) (*ctypes.Ticker, error) {
	price, err := decimal.NewFromString(event.ClosePrice)
	if err != nil {
		return nil, fmt.Errorf("parse price: %w", err)
	}
	open24 := number.DecimalFromString(event.OpenPrice)
	high24, err := decimal.NewFromString(event.HighPrice)
	if err != nil {
		return nil, fmt.Errorf("parse high24: %w", err)
	}
	low24, err := decimal.NewFromString(event.LowPrice)
	if err != nil {
		return nil, fmt.Errorf("parse low24: %w", err)
	}
	volume24, err := decimal.NewFromString(event.BaseVolume)
	if err != nil {
		return nil, fmt.Errorf("parse volume24: %w", err)
	}
	quoteVolume24, err := decimal.NewFromString(event.QuoteVolume)
	if err != nil {
		return nil, fmt.Errorf("parse quote volume24: %w", err)
	}
	avg24, err := decimal.NewFromString(event.WeightedAvgPrice)
	if err != nil {
		return nil, fmt.Errorf("parse avg24: %w", err)
	}
	return &ctypes.Ticker{
		Exchange:      exchange,
		Symbol:        symbol,
		LastPrice:     price,
		Open24:        open24,
		High24:        high24,
		Low24:         low24,
		Avg24:         avg24,
		Volume24:      volume24,
		QuoteVolume24: quoteVolume24,
		Ts:            time.UnixMilli(event.Time),
	}, nil
}

func ConvertSpotTrade2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, event *spot.WsAggTradeEvent) (*ctypes.Trade, error) {
	price, err := decimal.NewFromString(event.Price)
	if err != nil {
		return nil, fmt.Errorf("parse trade price: %w", err)
	}
	size, err := decimal.NewFromString(event.Quantity)
	if err != nil {
		return nil, fmt.Errorf("parse trade qty: %w", err)
	}
	return &ctypes.Trade{
		Exchange: exchange,
		Symbol:   symbol,
		TradeID:  fmt.Sprintf("%d", event.LastBreakdownTradeID),
		Price:    price,
		Size:     size,
		IsBuy:    !event.IsBuyerMaker,
		Ts:       time.UnixMilli(event.TradeTime),
	}, nil
}

func ConvertFutureTrade2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, event *future.WsAggTradeEvent) (*ctypes.Trade, error) {
	price, err := decimal.NewFromString(event.Price)
	if err != nil {
		return nil, fmt.Errorf("parse trade price: %w", err)
	}
	size, err := decimal.NewFromString(event.Quantity)
	if err != nil {
		return nil, fmt.Errorf("parse trade qty: %w", err)
	}
	return &ctypes.Trade{
		Exchange: exchange,
		Symbol:   symbol,
		TradeID:  fmt.Sprintf("%d", event.AggregateTradeID),
		Price:    price,
		Size:     size,
		IsBuy:    !event.Maker,
		Ts:       time.UnixMilli(event.TradeTime),
	}, nil
}

func ConvertSpotDepth2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, event *spot.WsDepthEvent) (*ctypes.OrderBook, error) {
	convert := func(levels []spot.Bid) ([]ctypes.OrderBookLevel, error) {
		res := make([]ctypes.OrderBookLevel, 0, len(levels))
		for _, lvl := range levels {
			price, err := decimal.NewFromString(lvl.Price)
			if err != nil {
				return nil, err
			}
			size, err := decimal.NewFromString(lvl.Quantity)
			if err != nil {
				return nil, err
			}
			res = append(res, ctypes.OrderBookLevel{
				Price: price,
				Size:  size,
			})
		}
		return res, nil
	}
	bids, err := convert(event.Bids)
	if err != nil {
		return nil, fmt.Errorf("parse bids: %w", err)
	}
	asksRaw := make([]spot.Bid, len(event.Asks))
	for i := range event.Asks {
		asksRaw[i] = spot.Bid{
			Price:    event.Asks[i].Price,
			Quantity: event.Asks[i].Quantity,
		}
	}
	asks, err := convert(asksRaw)
	if err != nil {
		return nil, fmt.Errorf("parse asks: %w", err)
	}
	return &ctypes.OrderBook{
		Exchange:  exchange,
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Ts:        time.UnixMilli(event.Time),
		SeqId:     event.LastUpdateID,
		PrevSeqId: event.FirstUpdateID - 1,
	}, nil
}

func ConvertFutureDepth2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, event *future.WsDepthEvent) (*ctypes.OrderBook, error) {
	convert := func(levels []spot.Bid) ([]ctypes.OrderBookLevel, error) {
		res := make([]ctypes.OrderBookLevel, 0, len(levels))
		for _, lvl := range levels {
			price, err := decimal.NewFromString(lvl.Price)
			if err != nil {
				return nil, err
			}
			size, err := decimal.NewFromString(lvl.Quantity)
			if err != nil {
				return nil, err
			}
			res = append(res, ctypes.OrderBookLevel{
				Price: price,
				Size:  size,
			})
		}
		return res, nil
	}
	bids, err := convert(event.Bids)
	if err != nil {
		return nil, fmt.Errorf("parse bids: %w", err)
	}
	asksRaw := make([]spot.Bid, len(event.Asks))
	for i := range event.Asks {
		asksRaw[i] = spot.Bid{
			Price:    event.Asks[i].Price,
			Quantity: event.Asks[i].Quantity,
		}
	}
	asks, err := convert(asksRaw)
	if err != nil {
		return nil, fmt.Errorf("parse asks: %w", err)
	}
	return &ctypes.OrderBook{
		Exchange:  exchange,
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Ts:        time.UnixMilli(event.Time),
		SeqId:     event.LastUpdateID,
		PrevSeqId: event.PrevLastUpdateID,
	}, nil
}

func ConvertSpotKline2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, event *spot.WsKlineEvent) (*ctypes.Kline, error) {
	open, err := decimal.NewFromString(event.Kline.Open)
	if err != nil {
		return nil, fmt.Errorf("parse open: %w", err)
	}
	close, err := decimal.NewFromString(event.Kline.Close)
	if err != nil {
		return nil, fmt.Errorf("parse close: %w", err)
	}
	high, err := decimal.NewFromString(event.Kline.High)
	if err != nil {
		return nil, fmt.Errorf("parse high: %w", err)
	}
	low, err := decimal.NewFromString(event.Kline.Low)
	if err != nil {
		return nil, fmt.Errorf("parse low: %w", err)
	}
	volume, err := decimal.NewFromString(event.Kline.Volume)
	if err != nil {
		return nil, fmt.Errorf("parse volume: %w", err)
	}
	quoteVolume, err := decimal.NewFromString(event.Kline.QuoteVolume)
	if err != nil {
		return nil, fmt.Errorf("parse quote volume: %w", err)
	}
	return &ctypes.Kline{
		Exchange:    exchange,
		Symbol:      symbol,
		Interval:    interval,
		Open:        open,
		High:        high,
		Low:         low,
		Close:       close,
		Volume:      volume,
		QuoteVolume: quoteVolume,
		Trades:      event.Kline.TradeNum,
		OpenTs:      time.UnixMilli(event.Kline.StartTime),
		CloseTs:     time.UnixMilli(event.Kline.EndTime),
		IsClosed:    event.Time >= event.Kline.EndTime,
	}, nil
}

func ConvertFutureKline2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, event *future.WsKlineEvent) (*ctypes.Kline, error) {
	open, err := decimal.NewFromString(event.Kline.Open)
	if err != nil {
		return nil, fmt.Errorf("parse open: %w", err)
	}
	close, err := decimal.NewFromString(event.Kline.Close)
	if err != nil {
		return nil, fmt.Errorf("parse close: %w", err)
	}
	high, err := decimal.NewFromString(event.Kline.High)
	if err != nil {
		return nil, fmt.Errorf("parse high: %w", err)
	}
	low, err := decimal.NewFromString(event.Kline.Low)
	if err != nil {
		return nil, fmt.Errorf("parse low: %w", err)
	}
	volume, err := decimal.NewFromString(event.Kline.Volume)
	if err != nil {
		return nil, fmt.Errorf("parse volume: %w", err)
	}
	quoteVolume, err := decimal.NewFromString(event.Kline.QuoteVolume)
	if err != nil {
		return nil, fmt.Errorf("parse quote volume: %w", err)
	}
	return &ctypes.Kline{
		Exchange:    exchange,
		Symbol:      symbol,
		Interval:    interval,
		Open:        open,
		High:        high,
		Low:         low,
		Close:       close,
		Volume:      volume,
		QuoteVolume: quoteVolume,
		Trades:      event.Kline.TradeNum,
		OpenTs:      time.UnixMilli(event.Kline.StartTime),
		CloseTs:     time.UnixMilli(event.Kline.EndTime),
		IsClosed:    event.Time >= event.Kline.EndTime,
	}, nil
}

func (c *Connector) ConvertWsSpotOrderUpdate2Types(ctx context.Context, symbol ctypes.Symbol, order *binance.WsOrderUpdate) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	var workingTs *time.Time
	if order.WorkingTime != 0 {
		workingTs = lo.ToPtr(time.UnixMilli(order.WorkingTime))
	}

	postOnly := false
	conditions := make([]ctypes.OrderCondition, 0)
	switch order.Type {
	case "STOP", "STOP_MARKET", "TAKE_PROFIT", "TAKE_PROFIT_MARKET":
		triggerType := ctypes.TriggerTakeProfit
		if order.Type == "STOP" || order.Type == "STOP_MARKET" {
			triggerType = ctypes.TriggerStopLoss
		}
		conditions = append(conditions, ctypes.OrderCondition{
			TriggerType:      triggerType,
			ActivationPrice:  number.DecimalFromString(order.StopPrice),
			OrderPrice:       number.DecimalFromString(order.Price),
			PriceWorkingType: ctypes.PriceWorkingTypeLatest,
			Activated:        order.IsInOrderBook,
			ActivatedTs:      workingTs,
		})
	case "LIMIT", "MARKET":
	case "LIMIT_MAKER":
		postOnly = true
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.Type)
	}

	if order.IsMaker {
		postOnly = true
	}

	ordType, ok := MapSpotOrderType2Types[string(order.Type)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapSpotOrderType2AlgoType[string(order.Type)]

	executedQty := number.DecimalFromString(order.FilledVolume)
	executedQuoteQty := number.DecimalFromString(order.FilledQuoteVolume)
	avgPrice := decimal.Zero
	if !executedQty.IsZero() {
		avgPrice = executedQuoteQty.Div(executedQty)
	}

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[string(order.Status)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(time.UnixMilli(order.TransactionTime))
	}

	result := &ctypes.Order{
		Exchange:         c.exchange,
		Symbol:           symbol,
		ClientOrderID:    ctypes.OrderId(order.ClientOrderId),
		OrderID:          ctypes.OrderId(strconv.FormatInt(order.Id, 10)),
		OrderType:        ordType,
		AlgoType:         algoType,
		Source:           ctypes.OrderSourceUser,
		Side:             ctypes.PositionSideLong, // 现货默认做多
		IsBuy:            order.Side == string(binance.SideTypeBuy),
		Price:            number.DecimalFromString(order.Price),
		OriginalQty:      number.DecimalFromString(order.Volume),
		ExecutedQty:      executedQty,
		OriginalQuoteQty: number.DecimalFromString(order.QuoteVolume),
		ExecutedQuoteQty: executedQuoteQty,
		AvgPrice:         avgPrice,
		PriceWorkingType: ctypes.PriceWorkingTypeLatest,
		TimeInForce:      ctypes.TimeInForce(order.TimeInForce),
		PostOnly:         postOnly,
		Status:           status,
		Conditions:       conditions,
		IsWorking:        order.IsInOrderBook,
		WorkingTs:        workingTs,
		RejectReason:     order.RejectReason,
		Raw:              details,
		CreatedTs:        time.UnixMilli(order.CreateTime),
		FinishedTs:       finishedTs,
		UpdatedTs:        time.UnixMilli(order.TransactionTime),
	}
	c.calculateOrder(ctx, result)
	return result, nil
}

func (c *Connector) ConvertSpotOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *binance.Order) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	var workingTs *time.Time
	if order.WorkingTime != 0 {
		workingTs = lo.ToPtr(time.UnixMilli(order.WorkingTime))
	}

	postOnly := false
	conditions := make([]ctypes.OrderCondition, 0)
	switch order.Type {
	case "STOP", "STOP_MARKET", "TAKE_PROFIT", "TAKE_PROFIT_MARKET":
		triggerType := ctypes.TriggerTakeProfit
		if order.Type == "STOP" || order.Type == "STOP_MARKET" {
			triggerType = ctypes.TriggerStopLoss
		}
		conditions = append(conditions, ctypes.OrderCondition{
			TriggerType:      triggerType,
			ActivationPrice:  number.DecimalFromString(order.StopPrice),
			OrderPrice:       number.DecimalFromString(order.Price),
			PriceWorkingType: ctypes.PriceWorkingTypeLatest,
			Activated:        order.IsWorking,
			ActivatedTs:      workingTs,
		})
	case "LIMIT", "MARKET":
	case "LIMIT_MAKER":
		postOnly = true
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.Type)
	}

	ordType, ok := MapSpotOrderType2Types[string(order.Type)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapSpotOrderType2AlgoType[string(order.Type)]

	executedQty := number.DecimalFromString(order.ExecutedQuantity)
	executedQuoteQty := number.DecimalFromString(order.CummulativeQuoteQuantity)
	avgPrice := decimal.Zero
	if !executedQty.IsZero() {
		avgPrice = executedQuoteQty.Div(executedQty)
	}

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[string(order.Status)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(time.UnixMilli(order.UpdateTime))
	}

	result := &ctypes.Order{
		Exchange:         c.exchange,
		Symbol:           symbol,
		ClientOrderID:    ctypes.OrderId(order.ClientOrderID),
		OrderID:          ctypes.OrderId(strconv.FormatInt(order.OrderID, 10)),
		OrderType:        ordType,
		AlgoType:         algoType,
		Side:             ctypes.PositionSideLong, // 现货默认做多
		IsBuy:            order.Side == binance.SideTypeBuy,
		Price:            number.DecimalFromString(order.Price),
		OriginalQty:      number.DecimalFromString(order.OrigQuantity),
		ExecutedQty:      executedQty,
		OriginalQuoteQty: number.DecimalFromString(order.OrigQuoteOrderQuantity),
		ExecutedQuoteQty: executedQuoteQty,
		AvgPrice:         avgPrice,
		PriceWorkingType: ctypes.PriceWorkingTypeLatest,
		TimeInForce:      ctypes.TimeInForce(order.TimeInForce),
		PostOnly:         postOnly,
		Status:           status,
		WorkingTs:        workingTs,
		IsWorking:        order.IsWorking,
		Conditions:       conditions,
		Raw:              details,
		CreatedTs:        time.UnixMilli(order.Time),
		UpdatedTs:        time.UnixMilli(order.UpdateTime),
		FinishedTs:       finishedTs,
	}
	c.calculateOrder(ctx, result)
	return result, nil
}

func (c *Connector) ConvertWsFutureOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *future.WsOrderTradeUpdate, ts time.Time) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	switch order.Type {
	case futures.OrderTypeLimit, futures.OrderTypeMarket, futures.OrderTypeLiquidation:
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.Type)
	}

	ordType, ok := MapFutureOrderType2Types[string(order.Type)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[string(order.Type)]

	price := number.DecimalFromString(order.OriginalPrice)
	avgPrice := number.DecimalFromString(order.AveragePrice)
	origQty := number.DecimalFromString(order.OriginalQty)
	execQty := number.DecimalFromString(order.AccumulatedFilledQty)

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[string(order.Status)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(ts)
	}

	result := &ctypes.Order{
		Exchange:         c.exchange,
		Symbol:           symbol,
		ClientOrderID:    ctypes.OrderId(order.ClientOrderID),
		OrderID:          ctypes.OrderId(fmt.Sprintf("%d", order.ID)),
		Side:             ConvertPositionSide2Types(string(order.PositionSide)),
		IsBuy:            order.Side == future.SideTypeBuy,
		OrderType:        ordType,
		AlgoType:         algoType,
		Source:           ctypes.OrderSourceUser,
		Price:            price,
		OriginalQty:      origQty,
		ExecutedQty:      execQty,
		AvgPrice:         avgPrice,
		Status:           status,
		TimeInForce:      ctypes.TimeInForce(order.TimeInForce),
		ReduceOnly:       order.IsReduceOnly,
		PostOnly:         order.IsMaker,
		ClosePosition:    order.IsClosingPosition,
		PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		PriceMode:        order.PriceMode,
		IsWorking:        true,
		WorkingTs:        lo.ToPtr(ts),
		Fee:              lo.ToPtr(number.DecimalFromString(order.Commission).Neg()),
		FeeAsset:         lo.ToPtr(order.CommissionAsset),
		RealizedPnl:      lo.ToPtr(number.DecimalFromString(order.RealizedPnL)),
		Raw:              details,
		CreatedTs:        ts,
		UpdatedTs:        ts,
		FinishedTs:       finishedTs,
	}
	c.calculateOrder(ctx, result)
	return result, nil
}

func (c *Connector) ConvertFutureOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *future.Order) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}
	postOnly := false
	switch order.Type {
	case futures.OrderTypeLimit, futures.OrderTypeMarket, futures.OrderTypeLiquidation:
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.Type)
	}

	ordType, ok := MapFutureOrderType2Types[string(order.Type)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[string(order.Type)]

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[string(order.Status)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(time.UnixMilli(order.UpdateTime))
	}

	result := &ctypes.Order{
		Exchange:         c.exchange,
		Symbol:           symbol,
		OrderID:          ctypes.OrderId(strconv.FormatInt(order.OrderID, 10)),
		OrderType:        ordType,
		AlgoType:         algoType,
		Source:           ctypes.OrderSourceUser,
		Side:             ConvertPositionSide2Types(string(order.PositionSide)),
		IsBuy:            order.Side == futures.SideTypeBuy,
		Price:            number.DecimalFromString(order.Price),
		OriginalQty:      number.DecimalFromString(order.OrigQuantity),
		ExecutedQty:      number.DecimalFromString(order.ExecutedQuantity),
		ExecutedQuoteQty: number.DecimalFromString(order.CumQuote),
		AvgPrice:         number.DecimalFromString(order.AvgPrice),
		Status:           status,
		IsWorking:        true,
		WorkingTs:        lo.ToPtr(time.UnixMilli(order.Time)),
		ReduceOnly:       order.ReduceOnly,
		PostOnly:         postOnly,
		ClosePosition:    order.ClosePosition,
		PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		PriceMode:        string(order.PriceMatch),
		TimeInForce:      ctypes.TimeInForce(order.TimeInForce),
		Raw:              details,
		CreatedTs:        time.UnixMilli(order.Time),
		UpdatedTs:        time.UnixMilli(order.UpdateTime),
		FinishedTs:       finishedTs,
	}
	c.calculateOrder(ctx, result)
	return result, nil
}

func (c *Connector) ConvertWsFutureAlgoOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *future.WsAlgoUpdate, ts time.Time) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	triggered := len(order.OrderID) > 0
	var triggerTime *time.Time
	if order.TriggerTime != 0 {
		triggerTime = lo.ToPtr(time.UnixMilli(order.TriggerTime))
	}
	// 条件单配置（止盈/止损/追踪）
	postOnly := false
	conditions := make([]ctypes.OrderCondition, 0)
	switch order.OrderType {
	case futures.AlgoOrderTypeStop, futures.AlgoOrderTypeStopMarket, futures.AlgoOrderTypeTakeProfit, futures.AlgoOrderTypeTakeProfitMarket, futures.AlgoOrderTypeTrailingStopMarket:
		triggerType := ctypes.TriggerTakeProfit
		isTrailing := false
		switch order.OrderType {
		case futures.AlgoOrderTypeStop, futures.AlgoOrderTypeStopMarket:
			triggerType = ctypes.TriggerStopLoss
		case futures.AlgoOrderTypeTrailingStopMarket:
			triggerType = ctypes.TriggerStopLoss
			isTrailing = true
		}
		activation := number.DecimalFromString(order.TriggerPrice)
		price := number.DecimalFromString(order.OrderPrice)
		// 跟踪止盈/止损单信息？
		conditions = append(conditions, ctypes.OrderCondition{
			TriggerType:      triggerType,
			OrderPrice:       price, // 限价单这里怎么取?
			ActivationPrice:  activation,
			IsTrailing:       isTrailing,
			CallbackDistance: decimal.Zero,
			CallbackRate:     decimal.Zero,
			PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
			PriceMode:        order.PriceMatchMode,
			Activated:        triggered,
			ActivatedTs:      triggerTime,
		})
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.OrderType)
	}

	side := ctypes.PositionSideLong
	if order.PositionSide == future.PositionSideTypeShort {
		side = ctypes.PositionSideShort
	}

	ordType, ok := MapFutureOrderType2Types[string(order.OrderType)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[string(order.OrderType)]

	price := number.DecimalFromString(order.OrderPrice)
	avgPrice := number.DecimalFromString(order.AvgPrice)
	origQty := number.DecimalFromString(order.Quantity)
	execQty := number.DecimalFromString(order.ExecutedQuantity)

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapAlgoOrderStatus2Types[string(order.AlgoStatus)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(ts)
	}

	result := &ctypes.Order{
		Exchange:         c.exchange,
		Symbol:           symbol,
		ClientOrderID:    ctypes.OrderId(order.ClientAlgoID),
		DrivedOrderID:    ctypes.OrderId(order.OrderID),
		OrderID:          ctypes.OrderId(fmt.Sprintf("%d", order.AlgoID)),
		Side:             side,
		IsBuy:            order.Side == future.SideTypeBuy,
		OrderType:        ordType,
		AlgoType:         algoType,
		Price:            price,
		OriginalQty:      origQty,
		ExecutedQty:      execQty,
		AvgPrice:         avgPrice,
		Status:           status,
		TimeInForce:      ctypes.TimeInForce(order.TimeInForce),
		ReduceOnly:       order.ReduceOnly,
		PostOnly:         postOnly,
		ClosePosition:    order.CloseAll,
		PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		PriceMode:        order.PriceMatchMode,
		Conditions:       conditions,
		IsWorking:        true,
		WorkingTs:        triggerTime,
		RejectReason:     order.FailedReason,
		Raw:              details,
		CreatedTs:        ts,
		UpdatedTs:        ts,
		FinishedTs:       finishedTs,
	}
	c.calculateOrder(ctx, result)
	return result, nil
}

func (c *Connector) ConvertFutureAlgoOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *futures.GetAlgoOrderResp) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	triggered := len(order.ActualOrderId) > 0
	var triggerTime *time.Time
	if order.TriggerTime != 0 {
		triggerTime = lo.ToPtr(time.UnixMilli(order.TriggerTime))
	}

	// 条件单配置（止盈/止损/追踪）
	postOnly := false
	conditions := make([]ctypes.OrderCondition, 0)
	switch order.OrderType {
	case futures.AlgoOrderTypeStop, futures.AlgoOrderTypeStopMarket, futures.AlgoOrderTypeTakeProfit, futures.AlgoOrderTypeTakeProfitMarket, futures.AlgoOrderTypeTrailingStopMarket:
		triggerType := ctypes.TriggerTakeProfit
		isTrailing := false
		switch order.OrderType {
		case futures.AlgoOrderTypeStop, futures.AlgoOrderTypeStopMarket:
			triggerType = ctypes.TriggerStopLoss
		case futures.AlgoOrderTypeTrailingStopMarket:
			triggerType = ctypes.TriggerStopLoss
			isTrailing = true
		}
		activationPrice := decimal.Zero
		price := decimal.Zero
		if triggerType == ctypes.TriggerStopLoss {
			activationPrice = number.DecimalFromString(order.SlTriggerPrice)
			price = number.DecimalFromString(order.SlPrice)
		} else {
			activationPrice = number.DecimalFromString(order.TpTriggerPrice)
			price = number.DecimalFromString(order.TpPrice)
		}
		// 跟踪止盈/止损单信息？
		conditions = append(conditions, ctypes.OrderCondition{
			TriggerType:      triggerType,
			OrderPrice:       price, // 限价单这里怎么取?
			ActivationPrice:  activationPrice,
			IsTrailing:       isTrailing,
			CallbackDistance: decimal.Zero,
			CallbackRate:     decimal.Zero,
			PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
			PriceMode:        string(order.PriceMatch),
			Activated:        triggered,
			ActivatedTs:      triggerTime,
		})
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.OrderType)
	}

	ordType, ok := MapFutureOrderType2Types[string(order.OrderType)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[string(order.OrderType)]

	price := number.DecimalFromString(order.Price)
	avgPrice := number.DecimalFromString(order.ActualPrice)
	origQty := number.DecimalFromString(order.Quantity)

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapAlgoOrderStatus2Types[string(order.AlgoStatus)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(time.UnixMilli(order.UpdateTime))
	}

	result := &ctypes.Order{
		Exchange:         c.exchange,
		Symbol:           symbol,
		ClientOrderID:    ctypes.OrderId(order.ClientAlgoId),
		DrivedOrderID:    ctypes.OrderId(order.ActualOrderId),
		OrderID:          ctypes.OrderId(fmt.Sprintf("%d", order.AlgoId)),
		Side:             ConvertPositionSide2Types(string(order.Side)),
		IsBuy:            order.Side == future.SideTypeBuy,
		OrderType:        ordType,
		AlgoType:         algoType,
		Price:            price,
		OriginalQty:      origQty,
		ExecutedQty:      decimal.Zero,
		AvgPrice:         avgPrice,
		Status:           status,
		TimeInForce:      ctypes.TimeInForce(order.TimeInForce),
		ReduceOnly:       order.ReduceOnly,
		PostOnly:         postOnly,
		ClosePosition:    order.ClosePosition,
		PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		PriceMode:        string(order.PriceMatch),
		Conditions:       conditions,
		IsWorking:        true,
		WorkingTs:        triggerTime,
		Raw:              details,
		CreatedTs:        time.UnixMilli(order.CreateTime),
		UpdatedTs:        time.UnixMilli(order.UpdateTime),
		FinishedTs:       finishedTs,
	}
	return result, nil
}

func (c *Connector) ConvertWsPortfolioUMOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *portfolio.WsFuturesOrderData, ts time.Time) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	// 条件单配置（止盈/止损/追踪）
	orderSource := ctypes.OrderSourceUser
	conditions := make([]ctypes.OrderCondition, 0)
	switch order.OrderType {
	case portfolio.OrderTypeLimit, portfolio.OrderTypeMarket:
	case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTakeProfit, portfolio.OrderTypeTakeProfitLimit, portfolio.OrderTypeTakeProfitMarket, portfolio.OrderTypeTrailingStopMarket:
		triggerType := ctypes.TriggerTakeProfit
		switch order.OrderType {
		case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTrailingStopMarket:
			triggerType = ctypes.TriggerStopLoss
		}
		activation := number.DecimalFromString(order.StopPrice)
		conditions = append(conditions, ctypes.OrderCondition{
			TriggerType:      triggerType,
			OrderPrice:       number.DecimalFromString(order.OriginalPrice),
			ActivationPrice:  activation,
			IsTrailing:       order.OrderType == portfolio.OrderTypeTrailingStopMarket,
			CallbackDistance: decimal.Zero,
			CallbackRate:     decimal.Zero,
			// PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
			// PriceMode:        string(order.PriceMatch),
			Activated:   true,
			ActivatedTs: lo.ToPtr(ts),
		})
	case portfolio.OrderTypeLiquidation:
		orderSource = ctypes.OrderSourceLiquidation
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.OrderType)
	}

	ordType, ok := MapFutureOrderType2Types[string(order.OrderType)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[string(order.OrderType)]

	price := number.DecimalFromString(order.OriginalPrice)
	avgPrice := number.DecimalFromString(order.AveragePrice)
	origQty := number.DecimalFromString(order.OriginalQty)
	execQty := number.DecimalFromString(order.FilledAccumQty)

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[string(order.OrderStatus)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(ts)
	}

	result := &ctypes.Order{
		Exchange:      c.exchange,
		Symbol:        symbol,
		ClientOrderID: ctypes.OrderId(order.ClientOrderID),
		OrderID:       ctypes.OrderId(fmt.Sprintf("%d", order.OrderID)),
		Side:          ConvertPositionSide2Types(string(order.PositionSide)),
		IsBuy:         order.Side == portfolio.SideTypeBuy,
		OrderType:     ordType,
		AlgoType:      algoType,
		Source:        orderSource,
		Price:         price,
		OriginalQty:   origQty,
		ExecutedQty:   execQty,
		AvgPrice:      avgPrice,
		// PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		// PriceMode:        string(order.PriceMatch),
		Status:      status,
		TimeInForce: ctypes.TimeInForce(order.TimeInForce),
		ReduceOnly:  order.IsReduceOnly,
		PostOnly:    order.IsMaker,
		Conditions:  conditions,
		Fee:         lo.ToPtr(number.DecimalFromString(order.Commission).Neg()),
		FeeAsset:    lo.ToPtr(order.CommissionAsset),
		RealizedPnl: lo.ToPtr(number.DecimalFromString(order.RealizedProfit)),
		Raw:         details,
		IsWorking:   true,
		WorkingTs:   lo.ToPtr(ts),
		CreatedTs:   ts,
		UpdatedTs:   ts,
		FinishedTs:  finishedTs,
	}
	c.calculateOrder(ctx, result)
	return result, nil
}

func (c *Connector) ConvertPortfolioUMOpenOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *portfolio.UMOpenOrdersResponse) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	// 条件单配置（止盈/止损/追踪）
	orderSource := ctypes.OrderSourceUser
	conditions := make([]ctypes.OrderCondition, 0)
	switch portfolio.OrderType(order.Type) {
	case portfolio.OrderTypeLimit, portfolio.OrderTypeMarket:
	case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTakeProfit, portfolio.OrderTypeTakeProfitLimit, portfolio.OrderTypeTakeProfitMarket, portfolio.OrderTypeTrailingStopMarket:
	case portfolio.OrderTypeLiquidation:
		orderSource = ctypes.OrderSourceLiquidation
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.Type)
	}

	ordType, ok := MapFutureOrderType2Types[order.Type]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[order.Type]

	price := number.DecimalFromString(order.Price)
	avgPrice := number.DecimalFromString(order.AvgPrice)
	origQty := number.DecimalFromString(order.OrigQty)
	execQty := number.DecimalFromString(order.ExecutedQty)
	execQuoteQty := number.DecimalFromString(order.CumQuote)

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[order.Status]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(time.UnixMilli(order.UpdateTime))
	}

	result := &ctypes.Order{
		Exchange:         c.exchange,
		Symbol:           symbol,
		ClientOrderID:    ctypes.OrderId(order.ClientOrderID),
		OrderID:          ctypes.OrderId(fmt.Sprintf("%d", order.OrderID)),
		Side:             ConvertPositionSide2Types(string(order.PositionSide)),
		IsBuy:            order.Side == "BUY",
		OrderType:        ordType,
		AlgoType:         algoType,
		Source:           orderSource,
		Price:            price,
		OriginalQty:      origQty,
		ExecutedQty:      execQty,
		ExecutedQuoteQty: execQuoteQty,
		AvgPrice:         avgPrice,
		// PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		PriceMode:   string(order.PriceMatch),
		Status:      status,
		TimeInForce: ctypes.TimeInForce(order.TimeInForce),
		ReduceOnly:  order.ReduceOnly,
		PostOnly:    false,
		Conditions:  conditions,
		Raw:         details,
		IsWorking:   true,
		WorkingTs:   lo.ToPtr(time.UnixMilli(order.Time)),
		CreatedTs:   time.UnixMilli(order.Time),
		UpdatedTs:   time.UnixMilli(order.UpdateTime),
		FinishedTs:  finishedTs,
	}
	c.calculateOrder(ctx, result)
	return result, nil
}

func (c *Connector) ConvertPortfolioUMAllOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *portfolio.UMAllOrdersResponse) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	// 条件单配置（止盈/止损/追踪）
	orderSource := ctypes.OrderSourceUser
	conditions := make([]ctypes.OrderCondition, 0)
	switch portfolio.OrderType(order.Type) {
	case portfolio.OrderTypeLimit, portfolio.OrderTypeMarket:
	case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTakeProfit, portfolio.OrderTypeTakeProfitLimit, portfolio.OrderTypeTakeProfitMarket, portfolio.OrderTypeTrailingStopMarket:
	case portfolio.OrderTypeLiquidation:
		orderSource = ctypes.OrderSourceLiquidation
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.Type)
	}

	ordType, ok := MapFutureOrderType2Types[order.Type]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[order.Type]

	price := number.DecimalFromString(order.Price)
	avgPrice := number.DecimalFromString(order.AvgPrice)
	origQty := number.DecimalFromString(order.OrigQty)
	execQty := number.DecimalFromString(order.ExecutedQty)
	execQuoteQty := number.DecimalFromString(order.CumQuote)

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[order.Status]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(time.UnixMilli(order.UpdateTime))
	}

	result := &ctypes.Order{
		Exchange:         c.exchange,
		Symbol:           symbol,
		ClientOrderID:    ctypes.OrderId(order.ClientOrderID),
		OrderID:          ctypes.OrderId(fmt.Sprintf("%d", order.OrderID)),
		Side:             ConvertPositionSide2Types(string(order.PositionSide)),
		IsBuy:            order.Side == "BUY",
		OrderType:        ordType,
		AlgoType:         algoType,
		Source:           orderSource,
		Price:            price,
		OriginalQty:      origQty,
		ExecutedQty:      execQty,
		ExecutedQuoteQty: execQuoteQty,
		AvgPrice:         avgPrice,
		// PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		PriceMode:   string(order.PriceMatch),
		Status:      status,
		TimeInForce: ctypes.TimeInForce(order.TimeInForce),
		ReduceOnly:  order.ReduceOnly,
		PostOnly:    false,
		Conditions:  conditions,
		Raw:         details,
		IsWorking:   true,
		WorkingTs:   lo.ToPtr(time.UnixMilli(order.Time)),
		CreatedTs:   time.UnixMilli(order.Time),
		UpdatedTs:   time.UnixMilli(order.UpdateTime),
		FinishedTs:  finishedTs,
	}
	c.calculateOrder(ctx, result)
	return result, nil
}

func (c *Connector) ConvertWsPortfolioUMAlgoOrder2Types(ctx context.Context, symbol ctypes.Symbol, evt *portfolio.WsConditionalOrderTradeUpdate, ts time.Time) (*ctypes.Order, error) {
	if evt == nil {
		return nil, fmt.Errorf("nil future order")
	}

	order := evt.Order

	triggered := order.OrderID > 0
	var triggerTime *time.Time
	if order.OrderTime != 0 {
		triggerTime = lo.ToPtr(time.UnixMilli(order.OrderTime))
	}

	// 条件单配置（止盈/止损/追踪）
	orderSource := ctypes.OrderSourceUser
	conditions := make([]ctypes.OrderCondition, 0)
	switch portfolio.OrderType(order.StrategyType) {
	case portfolio.OrderTypeLimit, portfolio.OrderTypeMarket:
	case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTakeProfit, portfolio.OrderTypeTakeProfitLimit, portfolio.OrderTypeTakeProfitMarket, portfolio.OrderTypeTrailingStopMarket:
		triggerType := ctypes.TriggerTakeProfit
		switch portfolio.OrderType(order.StrategyType) {
		case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTrailingStopMarket:
			triggerType = ctypes.TriggerStopLoss
		}
		isTrailing := portfolio.OrderType(order.StrategyType) == portfolio.OrderTypeTrailingStopMarket
		activation := number.DecimalFromString(order.StopPrice)
		callbackRate := decimal.Zero
		if isTrailing {
			callbackRate = number.DecimalFromString(order.CallbackRate)
			activation = number.DecimalFromString(order.ActivationPrice)
		}
		conditions = append(conditions, ctypes.OrderCondition{
			TriggerType:      triggerType,
			OrderPrice:       number.DecimalFromString(order.Price),
			ActivationPrice:  activation,
			IsTrailing:       isTrailing,
			CallbackDistance: decimal.Zero,
			CallbackRate:     callbackRate,
			PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
			// PriceMode:        string(order.PriceMatch),
			Activated:   triggered,
			ActivatedTs: triggerTime,
		})
	case portfolio.OrderTypeLiquidation:
		orderSource = ctypes.OrderSourceLiquidation
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.StrategyType)
	}

	ordType, ok := MapFutureOrderType2Types[string(order.StrategyType)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[string(order.StrategyType)]

	price := number.DecimalFromString(order.Price)
	origQty := number.DecimalFromString(order.Quantity)

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[string(order.OrderStatus)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(time.UnixMilli(order.UpdateTime))
	}

	result := &ctypes.Order{
		Exchange:         c.exchange,
		Symbol:           symbol,
		ClientOrderID:    ctypes.OrderId(order.ClientOrderID),
		OrderID:          ctypes.OrderId(fmt.Sprintf("%d", order.StrategyID)),
		DrivedOrderID:    ctypes.OrderId(fmt.Sprintf("%d", order.OrderID)),
		Side:             ConvertPositionSide2Types(string(order.PositionSide)),
		IsBuy:            order.Side == "BUY",
		OrderType:        ordType,
		AlgoType:         algoType,
		Source:           orderSource,
		Price:            price,
		OriginalQty:      origQty,
		ExecutedQty:      decimal.Zero,
		AvgPrice:         decimal.Zero,
		PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		// PriceMode:        string(order.PriceMatch),
		Status:        status,
		TimeInForce:   ctypes.TimeInForce(order.TimeInForce),
		ReduceOnly:    order.ReduceOnly,
		ClosePosition: order.ClosePosition,
		Conditions:    conditions,
		Raw:           details,
		IsWorking:     triggered,
		WorkingTs:     triggerTime,
		CreatedTs:     time.UnixMilli(order.UpdateTime),
		UpdatedTs:     time.UnixMilli(order.UpdateTime),
		FinishedTs:    finishedTs,
	}
	return result, nil
}

func (c *Connector) ConvertPortfolioUMAlgoOpenOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *portfolio.UMOpenConditionalOrderResponse) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	triggered := false
	var triggerTime *time.Time
	if order.BookTime != 0 {
		triggerTime = lo.ToPtr(time.UnixMilli(order.BookTime))
	}

	// 条件单配置（止盈/止损/追踪）
	orderSource := ctypes.OrderSourceUser
	conditions := make([]ctypes.OrderCondition, 0)
	switch portfolio.OrderType(order.StrategyType) {
	case portfolio.OrderTypeLimit, portfolio.OrderTypeMarket:
	case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTakeProfit, portfolio.OrderTypeTakeProfitLimit, portfolio.OrderTypeTakeProfitMarket, portfolio.OrderTypeTrailingStopMarket:
		triggerType := ctypes.TriggerTakeProfit
		switch portfolio.OrderType(order.StrategyType) {
		case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTrailingStopMarket:
			triggerType = ctypes.TriggerStopLoss
		}
		isTrailing := portfolio.OrderType(order.StrategyType) == portfolio.OrderTypeTrailingStopMarket
		activation := number.DecimalFromString(order.StopPrice)
		callbackRate := decimal.Zero
		if isTrailing {
			callbackRate = number.DecimalFromString(order.PriceRate)
			activation = number.DecimalFromString(order.ActivatePrice)
		}
		conditions = append(conditions, ctypes.OrderCondition{
			TriggerType:      triggerType,
			OrderPrice:       number.DecimalFromString(order.Price),
			ActivationPrice:  activation,
			IsTrailing:       isTrailing,
			CallbackDistance: decimal.Zero,
			CallbackRate:     callbackRate,
			// PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
			PriceMode:   string(order.PriceMatch),
			Activated:   triggered,
			ActivatedTs: triggerTime,
		})
	case portfolio.OrderTypeLiquidation:
		orderSource = ctypes.OrderSourceLiquidation
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.StrategyType)
	}

	ordType, ok := MapFutureOrderType2Types[string(order.StrategyType)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[string(order.StrategyType)]

	price := number.DecimalFromString(order.Price)
	origQty := number.DecimalFromString(order.OrigQty)

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[string(order.StrategyStatus)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(time.UnixMilli(order.UpdateTime))
	}

	result := &ctypes.Order{
		Exchange:      c.exchange,
		Symbol:        symbol,
		ClientOrderID: ctypes.OrderId(order.NewClientStrategyID),
		OrderID:       ctypes.OrderId(fmt.Sprintf("%d", order.StrategyID)),
		// DrivedOrderID:    ctypes.OrderId(fmt.Sprintf("%d", order.OrderID)),
		Side:        ConvertPositionSide2Types(string(order.PositionSide)),
		IsBuy:       order.Side == "BUY",
		OrderType:   ordType,
		AlgoType:    algoType,
		Source:      orderSource,
		Price:       price,
		OriginalQty: origQty,
		ExecutedQty: decimal.Zero,
		AvgPrice:    decimal.Zero,
		// PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		PriceMode:   string(order.PriceMatch),
		Status:      status,
		TimeInForce: ctypes.TimeInForce(order.TimeInForce),
		ReduceOnly:  order.ReduceOnly,
		// ClosePosition: order.ClosePosition,
		Conditions: conditions,
		Raw:        details,
		IsWorking:  triggered,
		WorkingTs:  triggerTime,
		CreatedTs:  time.UnixMilli(order.UpdateTime),
		UpdatedTs:  time.UnixMilli(order.UpdateTime),
		FinishedTs: finishedTs,
	}
	return result, nil
}

func (c *Connector) ConvertPortfolioUMAlgoOrder2Types(ctx context.Context, symbol ctypes.Symbol, order *portfolio.UMConditionalOrderResponse) (*ctypes.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}

	triggered := order.OrderID > 0
	var triggerTime *time.Time
	if order.TriggerTime != 0 {
		triggerTime = lo.ToPtr(time.UnixMilli(order.TriggerTime))
	}

	// 条件单配置（止盈/止损/追踪）
	orderSource := ctypes.OrderSourceUser
	conditions := make([]ctypes.OrderCondition, 0)
	switch portfolio.OrderType(order.StrategyType) {
	case portfolio.OrderTypeLimit, portfolio.OrderTypeMarket:
	case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTakeProfit, portfolio.OrderTypeTakeProfitLimit, portfolio.OrderTypeTakeProfitMarket, portfolio.OrderTypeTrailingStopMarket:
		triggerType := ctypes.TriggerTakeProfit
		switch portfolio.OrderType(order.StrategyType) {
		case portfolio.OrderTypeStop, portfolio.OrderTypeStopMarket, portfolio.OrderTypeTrailingStopMarket:
			triggerType = ctypes.TriggerStopLoss
		}
		isTrailing := portfolio.OrderType(order.StrategyType) == portfolio.OrderTypeTrailingStopMarket
		activation := number.DecimalFromString(order.StopPrice)
		callbackRate := decimal.Zero
		if isTrailing {
			callbackRate = number.DecimalFromString(order.PriceRate)
			activation = number.DecimalFromString(order.ActivatePrice)
		}
		conditions = append(conditions, ctypes.OrderCondition{
			TriggerType:      triggerType,
			OrderPrice:       number.DecimalFromString(order.Price),
			ActivationPrice:  activation,
			IsTrailing:       isTrailing,
			CallbackDistance: decimal.Zero,
			CallbackRate:     callbackRate,
			// PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
			PriceMode:   string(order.PriceMatch),
			Activated:   triggered,
			ActivatedTs: triggerTime,
		})
	case portfolio.OrderTypeLiquidation:
		orderSource = ctypes.OrderSourceLiquidation
	default:
		return nil, fmt.Errorf("unspported order type: %s", order.StrategyType)
	}

	ordType, ok := MapFutureOrderType2Types[string(order.StrategyType)]
	if !ok {
		ordType = ctypes.OrderTypeUnknown
	}

	algoType := MapFutureOrderType2AlgoType[string(order.StrategyType)]

	price := number.DecimalFromString(order.Price)
	origQty := number.DecimalFromString(order.OrigQty)

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	status, ok := MapOrderStatus2Types[string(order.StrategyStatus)]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		finishedTs = lo.ToPtr(time.UnixMilli(order.UpdateTime))
	}

	result := &ctypes.Order{
		Exchange:      c.exchange,
		Symbol:        symbol,
		ClientOrderID: ctypes.OrderId(order.NewClientStrategyID),
		OrderID:       ctypes.OrderId(fmt.Sprintf("%d", order.StrategyID)),
		DrivedOrderID: ctypes.OrderId(fmt.Sprintf("%d", order.OrderID)),
		Side:          ConvertPositionSide2Types(string(order.PositionSide)),
		IsBuy:         order.Side == "BUY",
		OrderType:     ordType,
		AlgoType:      algoType,
		Source:        orderSource,
		Price:         price,
		OriginalQty:   origQty,
		ExecutedQty:   decimal.Zero,
		AvgPrice:      decimal.Zero,
		// PriceWorkingType: ctypes.PriceWorkingType(order.WorkingType),
		PriceMode:   string(order.PriceMatch),
		Status:      status,
		TimeInForce: ctypes.TimeInForce(order.TimeInForce),
		ReduceOnly:  order.ReduceOnly,
		// ClosePosition: order.ClosePosition,
		Raw:        details,
		Conditions: conditions,
		IsWorking:  triggered,
		WorkingTs:  triggerTime,
		CreatedTs:  time.UnixMilli(order.UpdateTime),
		UpdatedTs:  time.UnixMilli(order.UpdateTime),
		FinishedTs: finishedTs,
	}
	return result, nil
}
