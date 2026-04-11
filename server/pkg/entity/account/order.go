package account

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stumble/wpgx"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	"github.com/wangliang139/NovaForge/server/pkg/repos/assets"
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/snowflake"
)

// ApplyOrderSnapshot 写入或更新订单记录，返回更新前的订单记录（可能为 nil）
func (e *Entity) ApplyOrderSnapshot(ctx context.Context, order *ctypes.Order) (*orders.Order, error) {
	if order == nil {
		return nil, nil
	}

	var err error

	accountID := order.AccountID
	exchangeStr := order.Exchange.String()

	orderType := orders.OrderType(strings.ToUpper(string(order.OrderType)))
	algoType := orders.AlgoType(strings.ToUpper(string(order.AlgoType)))
	source := orders.OrderSource(strings.ToUpper(string(order.Source)))
	if source == "" {
		source = orders.OrderSourceUSER
	}
	status := strings.ToUpper(strings.TrimSpace(string(order.Status)))
	if status == "" {
		status = string(ctypes.OrderStatusNew)
	}
	side := orders.OrderSide(strings.ToUpper(string(order.Side)))
	tif := orders.TimeInForce(strings.ToUpper(string(order.TimeInForce)))

	// 序列化 conditions
	var conditionsJSON []byte
	if len(order.Conditions) > 0 {
		conditionsJSON, err = sonic.Marshal(order.Conditions)
		if err != nil {
			return nil, fmt.Errorf("marshal conditions: %w", err)
		}
	}

	// 处理拒绝原因
	var rejectReason *string
	if order.RejectReason != "" {
		rejectReason = &order.RejectReason
	}

	// 处理完成时间
	var finishedTs *time.Time
	if !order.UpdatedTs.IsZero() {
		switch ctypes.OrderStatus(status) {
		case ctypes.OrderStatusDone, ctypes.OrderStatusCanceled, ctypes.OrderStatusRejected, ctypes.OrderStatusExpired:
			finishedTs = &order.UpdatedTs
		}
	}

	var details []byte
	if order.Raw != "" {
		details = []byte(order.Raw)
	}

	executedQty := order.ExecutedQty
	executedQuoteQty := order.ExecutedQuoteQty
	avgPrice := order.AvgPrice
	emptyCount := 0
	if executedQty.LessThanOrEqual(decimal.Zero) {
		emptyCount++
	}
	if executedQuoteQty.LessThanOrEqual(decimal.Zero) {
		emptyCount++
	}
	if avgPrice.LessThanOrEqual(decimal.Zero) {
		emptyCount++
	}
	if emptyCount == 1 {
		market, err := e.GetConnector(ctx, order.Exchange, accountID)
		if err != nil {
			return nil, fmt.Errorf("get connector: %w", err)
		}
		symbolConfig, err := market.SymbolConfig(ctx, order.Symbol)
		if err != nil {
			return nil, fmt.Errorf("get symbol config: %w", err)
		}
		if symbolConfig == nil {
			return nil, fmt.Errorf("symbol config not found")
		}
		switch {
		case executedQty.LessThanOrEqual(decimal.Zero):
			executedQty = executedQuoteQty.Div(avgPrice).Round(int32(symbolConfig.Market.BaseAssetPrecision))
		case executedQuoteQty.LessThanOrEqual(decimal.Zero):
			executedQuoteQty = executedQty.Mul(avgPrice).Round(int32(symbolConfig.Market.QuoteAssetPrecision))
		case avgPrice.LessThanOrEqual(decimal.Zero):
			avgPrice = executedQuoteQty.Div(executedQty).Round(int32(symbolConfig.Market.PricePrecision))
		}
	}

	var fee pgtype.Numeric
	if order.Fee != nil {
		fee = utils.Decimal.DecimalToPgNumeric(*order.Fee)
	}
	var realizedPnl pgtype.Numeric
	var pnlAsset *string
	if order.RealizedPnl != nil {
		realizedPnl = utils.Decimal.DecimalToPgNumeric(*order.RealizedPnl)
	}
	// 现货订单：计算 realized_pnl 和 pnl_asset（交易所不提供）
	switch order.Symbol.Type {
	case ctypes.MarketTypeSpot:
		if (status == "DONE" || status == "PARTIAL_DONE") && executedQty.GreaterThan(decimal.Zero) {
			spotPnl, spotAsset := e.calcSpotOrderPnl(ctx, order, executedQty, executedQuoteQty)
			if spotAsset != "" {
				realizedPnl = utils.Decimal.DecimalToPgNumeric(spotPnl)
				pnlAsset = &spotAsset
			}
		}
	case ctypes.MarketTypeFuture:
		if order.IsReducePosition() {
			pnlAsset = lo.ToPtr(order.Symbol.Quote)
		}
	}

	// 这里只落订单快照，细节字段可后续补充
	params := orders.UpsertOrderParams{
		BotID:         int32(order.BotID),
		AccountID:     accountID,
		OrderID:       order.OrderID.String(),
		ClientOrderID: order.ClientOrderID.String(),
		DrivedOrderID: order.DrivedOrderID.String(),
		OrderType:     orderType,
		AlgoType:      algoType,
		Source:        source,
		Exchange:      exchangeStr,
		Symbol:        order.Symbol.String(),
		Side:          side,
		IsBuy:         order.IsBuy,
		ReduceOnly:    order.ReduceOnly,
		PostOnly:      order.PostOnly,
		Tif:           tif,
		Price:         utils.Decimal.DecimalToPgNumeric(order.Price),
		Quantity:      utils.Decimal.DecimalToPgNumeric(order.OriginalQty),
		ExecutedQty:   utils.Decimal.DecimalToPgNumeric(executedQty),
		ExecutedPrice: utils.Decimal.DecimalToPgNumeric(executedQuoteQty),
		AvgPrice:      utils.Decimal.DecimalToPgNumeric(avgPrice),
		Conditions:    conditionsJSON,
		Detail:        details,
		Status:        status,
		RejectReason:  rejectReason,
		CreatedTs:     order.CreatedTs,
		WorkingTs:     order.WorkingTs,
		FinishedTs:    finishedTs,
		UpdatedTs:     order.UpdatedTs,
		Fee:           fee,
		FeeAsset:      order.FeeAsset,
		RealizedPnl:   realizedPnl,
		PnlAsset:      pnlAsset,
	}

	result, err := e.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		prevOrder, err := e.db.OrdersRepo.WithTx(tx).GetOrderByOrderIdWithLock(ctx, orders.GetOrderByOrderIdWithLockParams{
			AccountID: accountID,
			OrderID:   order.OrderID.String(),
		})
		if err != nil {
			return nil, fmt.Errorf("get order by order id: %w", err)
		}

		_, err = e.db.OrdersRepo.WithTx(tx).UpsertOrder(ctx, params)
		if err != nil {
			return nil, err
		}

		// 合并处理：成交资金变更 + 订单冻结/解冻
		if err := e.applyOrderFillBalanceUpdate(ctx, tx, order, prevOrder); err != nil {
			logger.Ctx(ctx).Err(err).Msg("apply order fill and locked balance update")
		}

		return prevOrder, nil
	})
	if err != nil {
		return nil, err
	}

	prevOrder, _ := result.(*orders.Order)
	return prevOrder, nil
}

// calcSpotOrderPnl 计算现货订单的 realized_pnl
// 买入：减少 quote，pnl = qty_quote × (quote_usdt_price - avg_price_quote)
// 卖出：减少 base，pnl = qty_base × (base_usdt_price - avg_price_base)
// 返回 (realized_pnl, pnl_asset)，若无法计算则返回 (0, "")
func (e *Entity) calcSpotOrderPnl(ctx context.Context, order *ctypes.Order, executedQty, executedQuoteQty decimal.Decimal) (decimal.Decimal, string) {
	var reducedAsset string
	var reducedQty decimal.Decimal
	if order.IsBuy {
		reducedAsset = strings.ToUpper(order.Symbol.Quote)
		reducedQty = executedQuoteQty
	} else {
		reducedAsset = strings.ToUpper(order.Symbol.Base)
		reducedQty = executedQty
	}
	if reducedQty.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ""
	}

	if reducedAsset == "USDT" {
		return decimal.Zero, "USDT"
	}

	walletType := ctypes.GetWalletType(order.Exchange, ctypes.MarketTypeSpot)
	assetPo, err := e.db.AssetsRepo.GetAsset(ctx, assets.GetAssetParams{
		AccountID:  order.AccountID,
		Asset:      reducedAsset,
		WalletType: assets.WalletType(walletType),
	})

	avgPrice := decimal.Zero
	if err == nil && assetPo != nil && assetPo.AvgPrice.Valid {
		avgPrice = utils.Decimal.PgNumericToDecimal(assetPo.AvgPrice)
	}
	if !avgPrice.GreaterThan(decimal.Zero) {
		return decimal.Zero, reducedAsset
	}

	provider := e.engine.GetMarketProvider()
	priceUsdt, err := provider.GetLastPrice(ctx, order.Exchange, ctypes.NewSymbol(reducedAsset, "USDT", ctypes.MarketTypeSpot))
	if err != nil || !priceUsdt.GreaterThan(decimal.Zero) {
		logger.Ctx(ctx).Err(err).Str("asset", reducedAsset).Msg("get asset/USDT price for spot pnl")
		return decimal.Zero, ""
	}

	pnl := reducedQty.Mul(priceUsdt.Sub(avgPrice))
	return pnl, reducedAsset
}

// shouldPublishOrderSnapshot 判断是否需要发布订单快照事件
// 仅当订单状态/成交量/locked 等关键字段变化时才发布，避免重复推送
func shouldPublishOrderSnapshot(prev *orders.Order, cur *ctypes.Order) bool {
	if cur == nil {
		return false
	}

	// 新订单，必须发布
	if prev == nil {
		return true
	}

	// 状态变化
	curStatus := strings.ToUpper(strings.TrimSpace(string(cur.Status)))
	if curStatus == "" {
		curStatus = string(ctypes.OrderStatusNew)
	}
	if prev.Status != curStatus {
		return true
	}

	// 成交量变化
	prevExecutedQty := utils.Decimal.PgNumericToDecimal(prev.ExecutedQty)
	if !cur.ExecutedQty.Equal(prevExecutedQty) {
		return true
	}

	// locked 变化
	prevLocked := utils.Decimal.PgNumericToDecimal(prev.Locked)
	if cur.Locked != nil && !cur.Locked.Equal(prevLocked) {
		return true
	}

	// 更新时间推进（防止时间倒退的旧事件）
	if !cur.UpdatedTs.IsZero() && cur.UpdatedTs.After(prev.UpdatedTs) {
		return true
	}

	// 无有效变更，不发布
	return false
}

// deriveOrderLocked 推导订单应冻结的资金
// 返回值：(冻结金额, 冻结资产, 是否成功推导)
// 参考 ordersvc.PlaceOrder 中的资金冻结计算逻辑
func (e *Entity) deriveOrderLocked(ctx context.Context, order ctypes.Order) (decimal.Decimal, string, error) {
	// 1) 计算剩余未成交量
	remain := order.OriginalQty.Sub(order.ExecutedQty)
	if remain.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, "", nil
	}

	connector, err := e.GetConnector(ctx, order.Exchange, order.AccountID)
	if err != nil {
		return decimal.Zero, "", err
	}
	symbolConfig, err := connector.SymbolConfig(ctx, order.Symbol)
	if err != nil {
		return decimal.Zero, "", err
	}
	if symbolConfig == nil {
		return decimal.Zero, "", fmt.Errorf("symbol config not found")
	}

	// 2) 根据市场类型推导 locked
	switch order.Symbol.Type {
	case ctypes.MarketTypeFuture:
		// 合约订单：判断是否为开仓单
		isOpenPosition := (order.Side == ctypes.PositionSideLong && order.IsBuy) ||
			(order.Side == ctypes.PositionSideShort && !order.IsBuy)

		// 平仓单不冻结保证金
		if !isOpenPosition {
			return decimal.Zero, "", nil
		}

		// 开仓单：计算保证金
		return e.deriveFutureOrderLocked(ctx, connector, symbolConfig, order, remain)
	case ctypes.MarketTypeSpot:
		// 现货订单
		return e.deriveSpotOrderLocked(ctx, connector, order, remain)
	default:
		return decimal.Zero, "", fmt.Errorf("unsupported market type: %s", order.Symbol.Type)
	}
}

// deriveFutureOrderLocked 推导合约开仓订单的保证金
func (e *Entity) deriveFutureOrderLocked(ctx context.Context, conn mdtypes.Connector, symbolCfg *ctypes.SymbolConfig, order ctypes.Order, remain decimal.Decimal) (decimal.Decimal, string, error) {
	// 1) 获取杠杆倍数
	lev := int64(symbolCfg.CrossLeverage[0])
	if !order.IsBuy && len(symbolCfg.CrossLeverage) > 1 && symbolCfg.CrossLeverage[1] > 0 {
		lev = int64(symbolCfg.CrossLeverage[1])
	}
	if lev <= 0 {
		lev = 1
	}

	// 2) 计算价格
	var price decimal.Decimal
	switch order.OrderType {
	case ctypes.OrderTypeLimit:
		// 限价单使用订单价格
		if order.Price.LessThanOrEqual(decimal.Zero) {
			return decimal.Zero, "", fmt.Errorf("price is zero")
		}
		price = order.Price
	case ctypes.OrderTypeMarket:
		// 市价单使用平均成交价或最新价
		if order.AvgPrice.GreaterThan(decimal.Zero) {
			price = order.AvgPrice
		} else {
			// 尝试获取市场最新价
			ticker, err := conn.Ticker(ctx, order.Symbol)
			if err != nil || ticker == nil || ticker.LastPrice.LessThanOrEqual(decimal.Zero) {
				return decimal.Zero, "", fmt.Errorf("ticker not found")
			}
			price = ticker.LastPrice
		}
	default:
		return decimal.Zero, "", fmt.Errorf("unsupported order type: %s", order.OrderType)
	}

	// 3) 计算名义价值 (notional)
	notional := price.Mul(remain)

	// 4) 计算保证金 (margin = notional / leverage)
	margin := notional.Div(decimal.NewFromInt(lev))

	// 5) 保证金资产为 quote 币种
	asset := strings.ToUpper(order.Symbol.Quote)

	return margin, asset, nil
}

// deriveSpotOrderLocked 推导现货订单的冻结资金
func (e *Entity) deriveSpotOrderLocked(ctx context.Context, conn mdtypes.Connector, order ctypes.Order, remain decimal.Decimal) (decimal.Decimal, string, error) {
	switch order.OrderType {
	case ctypes.OrderTypeLimit:
		// 限价单
		if order.Price.LessThanOrEqual(decimal.Zero) {
			return decimal.Zero, "", fmt.Errorf("price is zero")
		}
		if order.IsBuy {
			// 买单：冻结 remain × price (quote 资产)
			locked := remain.Mul(order.Price)
			asset := strings.ToUpper(order.Symbol.Quote)
			return locked, asset, nil
		} else {
			// 卖单：冻结 remain (base 资产)
			locked := remain
			asset := strings.ToUpper(order.Symbol.Base)
			return locked, asset, nil
		}
	case ctypes.OrderTypeMarket:
		// 市价单
		if order.IsBuy {
			// 市价买单：优先使用 OriginalQuoteQty (按金额下单)
			if order.OriginalQuoteQty.GreaterThan(decimal.Zero) {
				executedQuote := order.ExecutedQuoteQty
				remainQuote := order.OriginalQuoteQty.Sub(executedQuote)
				if remainQuote.LessThanOrEqual(decimal.Zero) {
					return decimal.Zero, "", fmt.Errorf("remain quote is zero")
				}
				// 冻结剩余金额 (quote 资产)
				locked := remainQuote
				asset := strings.ToUpper(order.Symbol.Quote)
				return locked, asset, nil
			}

			// 按数量下单的市价买单：使用估算价格
			// 1. 优先使用订单的平均成交价
			var estimatePrice decimal.Decimal
			if order.AvgPrice.GreaterThan(decimal.Zero) {
				estimatePrice = order.AvgPrice
			} else {
				// 2. 从市场获取最新价
				ticker, err := conn.Ticker(ctx, order.Symbol)
				if err != nil || ticker == nil || ticker.LastPrice.LessThanOrEqual(decimal.Zero) {
					return decimal.Zero, "", fmt.Errorf("ticker not found")
				}
				estimatePrice = ticker.LastPrice
			}

			// 应用市价单缓冲系数 (1.05x)
			marketOrderFreezeFactor := decimal.NewFromFloat(1.05)
			locked := remain.Mul(estimatePrice).Mul(marketOrderFreezeFactor)
			asset := strings.ToUpper(order.Symbol.Quote)
			return locked, asset, nil
		} else {
			// 市价卖单：冻结剩余数量 (base 资产)
			locked := remain
			asset := strings.ToUpper(order.Symbol.Base)
			return locked, asset, nil
		}
	default:
		return decimal.Zero, "", fmt.Errorf("unsupported order type: %s", order.OrderType)
	}
}

func (e *Entity) applyOrderLockedDelta(ctx context.Context, tx *wpgx.WTx, order *ctypes.Order, asset string, delta decimal.Decimal) error {
	if order == nil || asset == "" || delta.IsZero() {
		return nil
	}
	reason := ctypes.LedgerReasonFundsFreeze
	if order.Symbol.Type == ctypes.MarketTypeFuture {
		reason = ctypes.LedgerReasonOrderMarginFreeze
	}
	if delta.IsNegative() {
		reason = ctypes.LedgerReasonFundsUnfreeze
		if order.Symbol.Type == ctypes.MarketTypeFuture {
			reason = ctypes.LedgerReasonOrderMarginUnfreeze
		}
	}

	walletType := ctypes.GetWalletType(order.Exchange, order.Symbol.Type)
	ts := time.Now()
	if !order.UpdatedTs.IsZero() && order.UpdatedTs.Before(ts) {
		ts = order.UpdatedTs.Add(10 * time.Millisecond)
	}

	return e.CheckAndApplyAssetOrderOccupiedUpdateWithTx(ctx, tx, order.AccountID, order.Exchange, &ctypes.AssetEvent{
		WalletType: walletType,
		Code:       asset,
		Locked:     lo.ToPtr(delta),
		UpdatedTs:  ts,
	}, reason, order)
}

// applyOrderFillBalanceUpdate 处理订单冻结/解冻逻辑以及发送成交事件
// 注意：成交资金变更由单独的 asset update 事件触发，此函数不发布资金变更的 BalanceUpdate
func (e *Entity) applyOrderFillBalanceUpdate(ctx context.Context, tx *wpgx.WTx, order *ctypes.Order, prev *orders.Order) error {
	if order == nil {
		return nil
	}
	if order.AccountID == "" || !order.Exchange.IsValid() || !order.Symbol.IsValid() {
		return nil
	}

	prevExecutedQty := decimal.Zero
	if prev != nil {
		prevExecutedQty = utils.Decimal.PgNumericToDecimal(prev.ExecutedQty)
	}
	fillQtyDelta := order.ExecutedQty.Sub(prevExecutedQty)

	// ========== 第一部分：发送成交事件（Fill） ==========
	if fillQtyDelta.GreaterThan(decimal.Zero) {
		err := e.sendOrderDerivedFillEvent(ctx, order, prev)
		if err != nil {
			return err
		}
	}

	// ========== 第二部分：处理订单冻结/解冻 ==========
	// 订单已完结，解冻 DB 中所有已冻结资金
	status := order.Status
	if status.IsFinished() {
		if prev != nil {
			prevLocked := utils.Decimal.PgNumericToDecimal(prev.Locked)
			if prevLocked.GreaterThan(decimal.Zero) {
				prevAsset := ""
				if prev.LockedAsset != nil && strings.TrimSpace(*prev.LockedAsset) != "" {
					prevAsset = strings.ToUpper(strings.TrimSpace(*prev.LockedAsset))
				}
				if prevAsset != "" {
					return e.applyOrderLockedDelta(ctx, tx, order, prevAsset, prevLocked.Neg())
				}
			}
		}
		return nil
	}

	// 未完结的新订单，需要冻结资金（仅限外部订单，内部订单已在 PlaceOrder 时冻结）
	if prev == nil && shouldFreezeExternalOrder(*order) {
		curLocked, curAsset, err := e.deriveOrderLocked(ctx, *order)
		if err != nil {
			return err
		}
		if curLocked.GreaterThan(decimal.Zero) {
			_, err = e.db.OrdersRepo.WithTx(tx).SetOrderLockedAsset(ctx, orders.SetOrderLockedAssetParams{
				AccountID:   order.AccountID,
				OrderID:     order.OrderID.String(),
				Locked:      utils.Decimal.DecimalToPgNumeric(curLocked),
				LockedAsset: lo.ToPtr(curAsset),
			})
			if err != nil {
				return err
			}
			return e.applyOrderLockedDelta(ctx, tx, order, curAsset, curLocked)
		}
		return nil
	}

	if prev == nil || fillQtyDelta.Equal(decimal.Zero) {
		return nil
	}

	// 未完结的旧订单，根据 prev 和 cur 的 locked 变化计算解冻 delta
	prevTotalQty := utils.Decimal.PgNumericToDecimal(prev.Quantity)
	prevLocked := utils.Decimal.PgNumericToDecimal(prev.Locked)
	_prevLocked := prevLocked.String()
	_ = _prevLocked
	if prevLocked.LessThanOrEqual(decimal.Zero) || prevTotalQty.Sub(prevExecutedQty).LessThanOrEqual(decimal.Zero) {
		return nil
	}

	// 计算 delta
	curAsset := *prev.LockedAsset
	delta := prevLocked.Div(prevTotalQty.Sub(prevExecutedQty)).Mul(fillQtyDelta).Neg()

	logger.Ctx(ctx).Info().
		Str("order_id", order.OrderID.String()).
		Str("prev_locked", prevLocked.String()).
		Str("total_qty", prevTotalQty.String()).
		Str("prev_executed_qty", prevExecutedQty.String()).
		Str("executed_qty", order.ExecutedQty.String()).
		Str("fill_qty_delta", fillQtyDelta.String()).
		Str("delta", delta.String()).
		Msg("apply order locked delta")

	_, err := e.db.OrdersRepo.WithTx(tx).SetOrderLockedAsset(ctx, orders.SetOrderLockedAssetParams{
		AccountID:   order.AccountID,
		OrderID:     order.OrderID.String(),
		Locked:      utils.Decimal.DecimalToPgNumeric(prevLocked.Add(delta)),
		LockedAsset: lo.ToPtr(curAsset),
	})
	if err != nil {
		return err
	}
	return e.applyOrderLockedDelta(ctx, tx, order, curAsset, delta)
}

func (e *Entity) sendOrderDerivedFillEvent(ctx context.Context, order *ctypes.Order, prev *orders.Order) error {
	prevExecutedQty := decimal.Zero
	prevFee := decimal.Zero
	prevPnl := decimal.Zero
	if prev != nil {
		prevExecutedQty = utils.Decimal.PgNumericToDecimal(prev.ExecutedQty)
		prevFee = utils.Decimal.PgNumericToDecimal(prev.Fee)
		prevPnl = utils.Decimal.PgNumericToDecimal(prev.RealizedPnl)
	}

	// 仅当有成交量增加时才发送 Fill 事件
	fillQtyDelta := order.ExecutedQty.Sub(prevExecutedQty)
	if !fillQtyDelta.GreaterThan(decimal.Zero) {
		return nil
	}

	currentFee := decimal.Zero
	if order.Fee != nil {
		currentFee = *order.Fee
	}
	feeDelta := currentFee.Sub(prevFee)
	currentPnl := decimal.Zero
	if order.RealizedPnl != nil {
		currentPnl = *order.RealizedPnl
	}
	pnlDelta := currentPnl.Sub(prevPnl)

	fee := feeDelta
	if fee.IsNegative() {
		fee = fee.Neg()
	}

	feeAsset := ""
	if order.FeeAsset != nil {
		feeAsset = *order.FeeAsset
	}

	fill := &ctypes.Fill{
		Exchange:      order.Exchange,
		Symbol:        order.Symbol,
		AccountID:     order.AccountID,
		OrderID:       order.OrderID,
		ClientOrderID: order.ClientOrderID,
		TradeID:       snowflake.Generate().String(),
		Side:          order.Side,
		IsBuy:         order.IsBuy,
		Qty:           fillQtyDelta,
		Price:         order.AvgPrice,
		Fee:           fee,
		FeeAsset:      feeAsset,
		RealizedPnl:   pnlDelta,
		Ts:            order.UpdatedTs,
	}

	fillSelector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(order.AccountID),
		Symbol:  lo.ToPtr(order.Symbol),
	}
	fillMsg := ctypes.NewMessage(order.Exchange, fillSelector, fill, order.UpdatedTs)
	if err := e.engine.Publish(ctx, fillMsg); err != nil {
		return err
	}
	return nil
}

// shouldFreezeExternalOrder 判断外部订单是否需要冻结资金
// 规则：
// 1. 内部订单（bot_id > 0）由 ordersvc 预占，此处返回 false
// 2. Binance 现货无法区分策略单，全部需要冻结，返回 true
// 3. 交易所侧策略单/算法单不冻结，返回 false
// 4. 普通外部订单需要冻结，返回 true
func shouldFreezeExternalOrder(order ctypes.Order) bool {
	// 内部订单由 ordersvc 预占，此处不处理
	if order.BotID > 0 {
		return false
	}

	// 已完结的订单不处理
	if order.Status.IsFinished() {
		return false
	}

	// Binance 现货无法区分策略单，全部冻结
	if (order.Exchange == ctypes.ExchangeBinance || order.Exchange == ctypes.ExchangeBinanceTest) &&
		order.Symbol.Type == ctypes.MarketTypeSpot {
		return true
	}

	// 交易所侧策略单/算法单不冻结
	// 判断依据：algo_type != NONE
	if order.AlgoType != ctypes.AlgoTypeNone {
		return false
	}

	// 普通外部订单需要冻结
	return true
}
