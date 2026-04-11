package okx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/number"
	okx "github.com/wangliang139/okx-connector-go"
)

func Symbol2InstId(symbol ctypes.Symbol) string {
	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		return fmt.Sprintf("%s-%s", symbol.Base, symbol.Quote)
	case ctypes.MarketTypeFuture:
		return fmt.Sprintf("%s-%s-SWAP", symbol.Base, symbol.Quote)
	}
	panic(fmt.Sprintf("invalid symbol type: %s", symbol.Type))
}

func Symbol2InstFamily(symbol ctypes.Symbol) string {
	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		return fmt.Sprintf("%s-%s", symbol.Base, symbol.Quote)
	case ctypes.MarketTypeFuture:
		return fmt.Sprintf("%s-%s", symbol.Base, symbol.Quote)
	}
	panic(fmt.Sprintf("invalid symbol type: %s", symbol.Type))
}

func FormatMarketType(marketType ctypes.MarketType) string {
	switch marketType {
	case ctypes.MarketTypeSpot:
		return "SPOT"
	case ctypes.MarketTypeFuture:
		return "SWAP"
	}
	panic(fmt.Sprintf("invalid market type: %s", marketType))
}

func ParseMarketType(instType string, ctType string) ctypes.MarketType {
	switch instType {
	case "SPOT":
		return ctypes.MarketTypeSpot
	case "SWAP":
		return ctypes.MarketTypeFuture
	default:
		panic(fmt.Sprintf("invalid inst type: %s", instType))
	}
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
	case "live":
		return ctypes.MarketStatusTrading
	case "suspend":
		return ctypes.MarketStatusSuspended
	case "preopen", "test":
		return ctypes.MarketStatusPending
	}
	panic(fmt.Sprintf("invalid market status: %s", status))
}

func (c *Connector) ConvertTicker2Types(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, event *okx.Ticker) (*ctypes.Ticker, error) {
	price := number.DecimalFromString(event.Last)
	open24 := number.DecimalFromString(event.Open24h)
	high24 := number.DecimalFromString(event.High24h)
	low24 := number.DecimalFromString(event.Low24h)
	var volume24, quoteVolume24 decimal.Decimal
	if symbol.Type == ctypes.MarketTypeSpot {
		volume24 = number.DecimalFromString(event.Vol24h)
		quoteVolume24 = number.DecimalFromString(event.VolCcy24h)
	} else {
		volume24 = number.DecimalFromString(event.VolCcy24h)
	}
	avg24 := decimal.Zero
	if !volume24.IsZero() && !quoteVolume24.IsZero() {
		avg24 = quoteVolume24.Div(volume24)
	}
	ts, err := strconv.ParseInt(event.Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse ticker time: %w", err)
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
		Ts:            time.UnixMilli(ts),
	}, nil
}

func ConvertTrade2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, event *okx.AggTrade) (*ctypes.Trade, error) {
	price, err := decimal.NewFromString(event.Px)
	if err != nil {
		return nil, fmt.Errorf("parse trade price: %w", err)
	}
	size, err := decimal.NewFromString(event.Sz)
	if err != nil {
		return nil, fmt.Errorf("parse trade qty: %w", err)
	}

	isBuy := event.Side == "buy"

	ts, err := strconv.ParseInt(event.Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse trade time: %w", err)
	}
	return &ctypes.Trade{
		Exchange: exchange,
		Symbol:   symbol,
		TradeID:  event.TradeId,
		Price:    price,
		Size:     size,
		IsBuy:    isBuy,
		Ts:       time.UnixMilli(ts),
	}, nil
}

func ConvertDepth2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, event *okx.Depth) (*ctypes.OrderBook, error) {
	convert := func(levels [][]string) ([]ctypes.OrderBookLevel, error) {
		res := make([]ctypes.OrderBookLevel, 0, len(levels))
		for _, lvl := range levels {
			price, err := decimal.NewFromString(lvl[0])
			if err != nil {
				return nil, err
			}
			size, err := decimal.NewFromString(lvl[1])
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
	asks, err := convert(event.Asks)
	if err != nil {
		return nil, fmt.Errorf("parse asks: %w", err)
	}

	ts, err := strconv.ParseInt(event.Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse depth time: %w", err)
	}

	return &ctypes.OrderBook{
		Exchange:  exchange,
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Ts:        time.UnixMilli(ts),
		SeqId:     int64(event.SeqId),
		PrevSeqId: int64(event.PrevSeqId),
	}, nil
}

func ConvertKline2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, event []string) (*ctypes.Kline, error) {
	openTs, err := strconv.ParseInt(event[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse kline time: %w", err)
	}
	duration, err := interval.Duration()
	if err != nil {
		return nil, err
	}
	closeTs := openTs + int64(duration.Milliseconds())

	open, err := decimal.NewFromString(event[1])
	if err != nil {
		return nil, fmt.Errorf("parse open: %w", err)
	}
	close, err := decimal.NewFromString(event[4])
	if err != nil {
		return nil, fmt.Errorf("parse close: %w", err)
	}
	high, err := decimal.NewFromString(event[2])
	if err != nil {
		return nil, fmt.Errorf("parse high: %w", err)
	}
	low, err := decimal.NewFromString(event[3])
	if err != nil {
		return nil, fmt.Errorf("parse low: %w", err)
	}
	var volume decimal.Decimal
	if symbol.Type == ctypes.MarketTypeSpot {
		volume, err = decimal.NewFromString(event[5])
		if err != nil {
			return nil, fmt.Errorf("parse volume: %w", err)
		}
	} else {
		volume, err = decimal.NewFromString(event[6])
		if err != nil {
			return nil, fmt.Errorf("parse volume: %w", err)
		}
	}
	quoteVolume, err := decimal.NewFromString(event[7])
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
		OpenTs:      time.UnixMilli(openTs),
		CloseTs:     time.UnixMilli(closeTs),
		IsClosed:    event[8] == "1",
	}, nil
}

func ConvertInstId2Symbol(instId string) (ctypes.Symbol, error) {
	parts := strings.Split(strings.ToUpper(instId), "-")
	switch len(parts) {
	case 2:
		return ctypes.Symbol{
			Base:  parts[0],
			Quote: parts[1],
			Type:  ctypes.MarketTypeSpot,
		}, nil
	case 3:
		if parts[2] != "SWAP" {
			return ctypes.Symbol{}, fmt.Errorf("invalid inst id: %s", instId)
		}
		return ctypes.Symbol{
			Base:  parts[0],
			Quote: parts[1],
			Type:  ctypes.MarketTypeFuture,
		}, nil
	}
	return ctypes.Symbol{}, fmt.Errorf("invalid inst id: %s", instId)
}

func ConvertMarket2Types(exchange ctypes.Exchange, market *okx.SymbolInfo) (result *ctypes.Market, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("convert market to types: %v", r)
			result = nil
		}
	}()

	symbol, err := ConvertInstId2Symbol(market.InstId)
	if err != nil {
		return nil, fmt.Errorf("convert inst id to symbol: %w", err)
	}

	ctVal := decimal.NewFromInt(1)
	if symbol.Type == ctypes.MarketTypeFuture {
		ctVal = number.DecimalFromString(market.CtVal)
	}

	normalizeQuantityUnit := func(str string) decimal.Decimal {
		quantity := number.DecimalFromString(str)
		return quantity.Mul(ctVal)
	}

	tickSz := number.DecimalFromString(market.TickSz)
	lotSz := normalizeQuantityUnit(market.LotSz)
	minSz := normalizeQuantityUnit(market.MinSz)
	maxLmtSz := normalizeQuantityUnit(market.MaxLmtSz)
	maxMktSz := normalizeQuantityUnit(market.MaxMktSz)
	maxStopSz := normalizeQuantityUnit(market.MaxStopSz)

	supportOrderTypes := make([]ctypes.OrderTypeRule, 0)
	for _, orderType := range ctypes.AllOrderTypes() {
		orderTypeRules := ctypes.MarketRules{}
		switch orderType {
		case ctypes.OrderTypeMarket:
			orderTypeRules.MaxQuantity = decimal.Max(maxMktSz, maxStopSz)
			orderTypeRules.MaxNotional = number.DecimalFromString(market.MaxMktAmt)
		case ctypes.OrderTypeLimit:
			orderTypeRules.MaxQuantity = decimal.Max(maxLmtSz, maxStopSz)
			orderTypeRules.MaxNotional = number.DecimalFromString(market.MaxLmtAmt)
		}
		supportOrderTypes = append(supportOrderTypes, ctypes.OrderTypeRule{
			OrderType: orderType,
			Rules: ctypes.MarketRules{
				MinPrice: tickSz,
				MaxPrice: decimal.Zero,
			},
		})
	}

	return &ctypes.Market{
		Exchange:            exchange,
		Symbol:              symbol,
		Status:              ConvertMarketStatus2Types(market.State),
		BaseAssetPrecision:  number.GetPrecision(lotSz.String()),
		QuoteAssetPrecision: number.GetPrecision(tickSz.String()),
		PricePrecision:      number.GetPrecision(tickSz.String()),
		Rules: ctypes.MarketRules{
			MinPrice:    tickSz,
			MaxPrice:    decimal.Zero,
			TickSize:    tickSz,
			MinQuantity: minSz,
			MaxQuantity: decimal.Zero,
			LotSize:     lotSz,
			MinNotional: minSz.Mul(tickSz),
		},
		OrderTypeRules: supportOrderTypes,
	}, nil
}

func (c *Connector) ConvertOrder2Types(ctx context.Context, order *okx.Order) (*ctypes.Order, error) {
	cTime, err := strconv.ParseInt(order.CTime, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse order time: %w", err)
	}
	uTime, err := strconv.ParseInt(order.UTime, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse order time: %w", err)
	}

	algoType := GetAlgoType(order)
	orderType := GetOrderType(order)

	postOnly := false
	timeInForce := ctypes.TimeInForceGTC
	switch order.OrdType {
	case "fok":
		timeInForce = ctypes.TimeInForceFOK
	case "ioc":
		timeInForce = ctypes.TimeInForceIOC
	case "optimal_limit_ioc":
	case "market":
	case "limit":
	case "post_only":
		postOnly = true
	case "conditional":
	case "oco":
	case "trigger":
	case "chase":
	case "move_order_stop":
	case "iceberg":
	case "twap":
	default:
		return nil, fmt.Errorf("unsupported order type: %s", order.OrdType)
	}

	var orderId, clOrderId, drivedOrderId string
	if algoType == ctypes.AlgoTypeNone {
		orderId = order.OrdId
		clOrderId = order.ClOrdId
	} else {
		orderId = order.AlgoId
		clOrderId = order.AlgoClOrdId
		drivedOrderId = order.OrdId
	}

	symbol, err := ConvertInstId2Symbol(order.InstId)
	if err != nil {
		return nil, fmt.Errorf("convert inst id to symbol: %w", err)
	}

	ctVal := decimal.NewFromInt(1)
	if symbol.Type == ctypes.MarketTypeFuture {
		metadata, err := c.getMarketSymbol(context.Background(), symbol)
		if err != nil {
			return nil, fmt.Errorf("get market symbol: %w", err)
		}
		ctVal = number.DecimalFromString(metadata.CtVal)
	}

	status, ok := MapOrderStatus2Types[order.State]
	if !ok {
		status = ctypes.OrderStatusNew
	}

	positionSide := ctypes.PositionSideLong
	if order.PosSide == "short" {
		positionSide = ctypes.PositionSideShort
	}

	// 构造条件单配置
	conditions := buildOrderConditions(order)

	// 判断订单是否已触发生效
	isWorking := false
	var workingTs *time.Time
	if order.TriggerTime != "" && order.TriggerTime != "0" {
		triggerTime, err := strconv.ParseInt(order.TriggerTime, 10, 64)
		if err == nil && triggerTime > 0 {
			isWorking = true
			workingTs = lo.ToPtr(time.UnixMilli(triggerTime))
		}
	}
	if workingTs == nil && algoType == ctypes.AlgoTypeNone {
		workingTs = lo.ToPtr(time.UnixMilli(cTime))
	}
	// 普通单和已成交的算法单认为是生效的
	if algoType == ctypes.AlgoTypeNone || status == ctypes.OrderStatusDone || status == ctypes.OrderStatusPartialDone {
		isWorking = true
	}

	// 获取订单来源
	orderSource := GetOrderSource(order)

	originalQty := number.DecimalFromString(order.Sz).Mul(ctVal)
	price := number.DecimalFromString(order.Px)

	// 计算已成交金额
	executedQty := number.DecimalFromString(order.AccFillSz).Mul(ctVal)
	avgPrice := number.DecimalFromString(order.AvgPx)
	executedQuoteQty := executedQty.Mul(avgPrice)

	// 获取价格工作类型（优先使用触发价类型）
	priceWorkingType := ctypes.PriceWorkingTypeLatest
	if order.TriggerPxType != "" {
		priceWorkingType = ConvertPriceWorkingType(order.TriggerPxType)
	} else if order.TpTriggerPxType != "" {
		priceWorkingType = ConvertPriceWorkingType(order.TpTriggerPxType)
	} else if order.SlTriggerPxType != "" {
		priceWorkingType = ConvertPriceWorkingType(order.SlTriggerPxType)
	}

	details, err := sonic.MarshalString(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order details: %w", err)
	}

	var finishedTs *time.Time
	if status.IsFinished() {
		fillTime, err := strconv.ParseInt(order.FillTime, 10, 64)
		if err == nil && fillTime > 0 {
			finishedTs = lo.ToPtr(time.UnixMilli(fillTime))
		}
	}

	leverage := number.DecimalFromString(order.Lever)
	if leverage.LessThan(decimal.Zero) {
		leverage = decimal.Zero
	}

	result := &ctypes.Order{
		Exchange:         c.Exchange(),
		Symbol:           symbol,
		ClientOrderID:    ctypes.OrderId(clOrderId),
		DrivedOrderID:    ctypes.OrderId(drivedOrderId),
		OrderID:          ctypes.OrderId(orderId),
		OrderType:        orderType,
		AlgoType:         algoType,
		Source:           orderSource,
		Side:             positionSide,
		IsBuy:            order.Side == "buy",
		Price:            price,
		OriginalQty:      originalQty,
		ExecutedQty:      executedQty,
		ExecutedQuoteQty: executedQuoteQty,
		AvgPrice:         avgPrice,
		PriceWorkingType: priceWorkingType,
		Status:           status,
		PostOnly:         postOnly,
		TimeInForce:      timeInForce,
		ReduceOnly:       order.ReduceOnly == "true",
		ClosePosition:    order.CloseFraction == "1",
		Conditions:       conditions,
		IsWorking:        isWorking,
		WorkingTs:        workingTs,
		RejectReason:     order.CancelSourceReason,
		Fee:              lo.ToPtr(number.DecimalFromString(order.Fee)),
		FeeAsset:         lo.ToPtr(order.FeeCcy),
		RealizedPnl:      lo.ToPtr(number.DecimalFromString(order.Pnl)),
		Raw:              details,
		CreatedTs:        time.UnixMilli(cTime),
		FinishedTs:       finishedTs,
		UpdatedTs:        time.UnixMilli(uTime),
	}

	// 计算手续费
	if result.Fee == nil {
		fee, asset, err := c.CalcOrderFee(ctx, *result)
		if err != nil {
			logger.Ctx(ctx).Err(err).Msg("calc order fee failed")
		} else {
			result.Fee = fee
			result.FeeAsset = asset
		}
	}

	return result, nil
}

// buildOrderConditions 构造订单条件列表
func buildOrderConditions(order *okx.Order) []ctypes.OrderCondition {
	conditions := make([]ctypes.OrderCondition, 0)

	// 处理计划委托（trigger）
	if order.TriggerPx != "" && order.TriggerPx != "0" {
		condition := ctypes.OrderCondition{
			TriggerType:      ctypes.TriggerNone, // 计划委托无明确触发类型
			ActivationPrice:  number.DecimalFromString(order.TriggerPx),
			OrderPrice:       number.DecimalFromString(order.OrdPx),
			PriceWorkingType: ConvertPriceWorkingType(order.TriggerPxType),
		}
		// 判断是否已激活
		if order.TriggerTime != "" && order.TriggerTime != "0" {
			triggerTime, err := strconv.ParseInt(order.TriggerTime, 10, 64)
			if err == nil && triggerTime > 0 {
				condition.Activated = true
				condition.ActivatedTs = lo.ToPtr(time.UnixMilli(triggerTime))
			}
		}
		conditions = append(conditions, condition)
	}

	// 处理止盈条件
	if order.TpTriggerPx != "" && order.TpTriggerPx != "0" {
		condition := ctypes.OrderCondition{
			TriggerType:      ctypes.TriggerTakeProfit,
			ActivationPrice:  number.DecimalFromString(order.TpTriggerPx),
			OrderPrice:       number.DecimalFromString(order.TpOrdPx),
			PriceWorkingType: ConvertPriceWorkingType(order.TpTriggerPxType),
		}
		if condition.OrderPrice.LessThan(decimal.Zero) {
			condition.OrderPrice = decimal.Zero
		}
		// 判断是否已激活
		if order.TriggerTime != "" && order.TriggerTime != "0" {
			triggerTime, err := strconv.ParseInt(order.TriggerTime, 10, 64)
			if err == nil && triggerTime > 0 {
				condition.Activated = true
				condition.ActivatedTs = lo.ToPtr(time.UnixMilli(triggerTime))
			}
		}
		conditions = append(conditions, condition)
	}

	// 处理止损条件
	if order.SlTriggerPx != "" && order.SlTriggerPx != "0" {
		condition := ctypes.OrderCondition{
			TriggerType:      ctypes.TriggerStopLoss,
			ActivationPrice:  number.DecimalFromString(order.SlTriggerPx),
			OrderPrice:       number.DecimalFromString(order.SlOrdPx),
			PriceWorkingType: ConvertPriceWorkingType(order.SlTriggerPxType),
		}
		if condition.OrderPrice.LessThan(decimal.Zero) {
			condition.OrderPrice = decimal.Zero
		}
		// 判断是否已激活
		if order.TriggerTime != "" && order.TriggerTime != "0" {
			triggerTime, err := strconv.ParseInt(order.TriggerTime, 10, 64)
			if err == nil && triggerTime > 0 {
				condition.Activated = true
				condition.ActivatedTs = lo.ToPtr(time.UnixMilli(triggerTime))
			}
		}
		conditions = append(conditions, condition)
	}

	// 处理移动止盈止损（move_order_stop）
	if order.OrdType == "move_order_stop" {
		// 判断是止盈还是止损
		triggerType := ctypes.TriggerStopLoss
		if order.ActualSide == "tp" {
			triggerType = ctypes.TriggerTakeProfit
		}

		condition := ctypes.OrderCondition{
			TriggerType: triggerType,
			IsTrailing:  true,
			OrderPrice:  number.DecimalFromString(order.Px),
			Activated:   false,
		}

		// 激活价格
		if order.ActivePx != "" && order.ActivePx != "0" {
			condition.ActivationPrice = number.DecimalFromString(order.ActivePx)
		}

		// 回调比例或价距
		if order.CallbackRatio != "" && order.CallbackRatio != "0" {
			condition.CallbackRate = number.DecimalFromString(order.CallbackRatio)
		}
		if order.CallbackSpread != "" && order.CallbackSpread != "0" {
			condition.CallbackDistance = number.DecimalFromString(order.CallbackSpread)
		}

		// 移动触发价
		if order.MoveTriggerPx != "" && order.MoveTriggerPx != "0" {
			// MoveTriggerPx 表示当前的追踪触发价，可以作为实时激活价
			condition.ActivationPrice = number.DecimalFromString(order.MoveTriggerPx)
		}

		// 判断是否已激活
		if order.TriggerTime != "" && order.TriggerTime != "0" {
			triggerTime, err := strconv.ParseInt(order.TriggerTime, 10, 64)
			if err == nil && triggerTime > 0 {
				condition.Activated = true
				condition.ActivatedTs = lo.ToPtr(time.UnixMilli(triggerTime))
			}
		}

		conditions = append(conditions, condition)
	}

	return conditions
}

func ConvertBalance2Types(event *okx.WsAccountEvent) []*ctypes.AssetEvent {
	assets := make([]*ctypes.AssetEvent, 0)
	for _, data := range event.Data {
		for _, balance := range data.Details {
			uTime, err := strconv.ParseInt(balance.UTime, 10, 64)
			if err != nil {
				continue
			}
			_balance := number.DecimalFromString(balance.CashBal)
			_freezed := number.DecimalFromString(balance.FrozenBal)
			_margin := number.DecimalFromString(balance.Imr)
			// okx 口径的冻结资产包含了保证金，需要减去保证金
			_locked := _freezed.Sub(_margin)
			if _locked.LessThan(decimal.Zero) {
				_locked = decimal.Zero
			}
			assets = append(assets, &ctypes.AssetEvent{
				WalletType: ctypes.WalletTypeTrade,
				Code:       balance.Ccy,
				Balance:    &_balance,
				Locked:     &_locked,
				UpdatedTs:  time.UnixMilli(uTime),
			})
		}
	}
	return assets
}

func ConvertMarkPrice2Types(exchange ctypes.Exchange, symbol ctypes.Symbol, mp *okx.MarkPrice) (*ctypes.MarkPrice, error) {
	markPrice, err := decimal.NewFromString(mp.MarkPx)
	if err != nil {
		return nil, fmt.Errorf("parse mark price: %w", err)
	}
	ts, err := strconv.ParseInt(mp.Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse mark price time: %w", err)
	}
	return &ctypes.MarkPrice{
		Exchange:  exchange,
		Symbol:    symbol,
		MarkPrice: markPrice,
		Ts:        time.UnixMilli(ts),
	}, nil
}
