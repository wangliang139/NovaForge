package order

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/NovaForge/server/pkg/entity/account"
	mtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/snowflake"
)

type OrderRiskChecker interface {
	CheckOrderRisk(ctx context.Context, acct *types.Account, order *ctypes.Order) error
}

type Entity struct {
	db    *repos.Entity
	cache redis.UniversalClient

	acctEntity  *account.Entity
	riskChecker OrderRiskChecker
}

func New(db *repos.Entity, cache redis.UniversalClient, acctEntity *account.Entity, riskChecker OrderRiskChecker) *Entity {
	return &Entity{db: db, cache: cache, acctEntity: acctEntity, riskChecker: riskChecker}
}

// resolveTradingAccountForConnector 将 virtual_sub 沿 parent_account_id 递归解析到用于调用交易所的账户（通常为父 real）；其它类型原样返回。
func (s *Entity) resolveParentAccount(ctx context.Context, acct *types.Account) (*types.Account, error) {
	cur := acct
	for cur != nil && cur.AccountType == types.AccountTypeVirtualSub {
		if cur.ParentAccountID == nil || strings.TrimSpace(*cur.ParentAccountID) == "" {
			return nil, errors.New(errors.InvalidArgument, "virtual_sub account missing parent_account_id")
		}
		parent, err := s.acctEntity.GetAccount(ctx, strings.TrimSpace(*cur.ParentAccountID))
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, errors.New(errors.NotFound, "parent account not found")
		}
		if parent.Exchange != cur.Exchange {
			return nil, errors.New(errors.InvalidArgument, "virtual_sub parent exchange mismatch")
		}
		cur = parent
	}
	if cur == nil {
		return nil, errors.New(errors.InvalidArgument, "account is required")
	}
	return cur, nil
}

func (s *Entity) PlaceOrder(ctx context.Context, acct *types.Account, order *ctypes.Order) (*types.PlaceOrderOutput, error) {
	if order == nil {
		return nil, errors.New(errors.InvalidArgument, "order is required")
	}
	if acct == nil {
		return nil, errors.New(errors.InvalidArgument, "account is required")
	}

	var (
		botId = order.BotID

		symbol    = order.Symbol
		orderType = order.OrderType
		side      = order.Side
		isBuy     = order.IsBuy
		source    = order.Source

		rawPrice    = order.Price
		rawQty      = order.OriginalQty
		rawQuoteQty = order.OriginalQuoteQty

		tif           = order.TimeInForce
		reduceOnly    = order.ReduceOnly
		closePosition = order.ClosePosition
		postOnly      = order.PostOnly
	)

	if len(tif) == 0 {
		tif = ctypes.TimeInForceGTC
	}
	if !tif.Valid() {
		return nil, errors.New(errors.InvalidArgument, "time in force is invalid")
	}

	conn, err := s.acctEntity.GetConnector(ctx, acct.Exchange, acct.ID)
	if err != nil {
		return nil, err
	}

	symbolCfg, err := conn.SymbolConfig(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if symbolCfg == nil {
		return nil, errors.New(errors.NotFound, "symbol config not found")
	}

	// 1) 市场状态 & 订单类型支持校验
	if symbolCfg.Market.Status != ctypes.MarketStatusTrading {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("market status is %s, expected TRADING", symbolCfg.Market.Status))
	}

	// 2) 价格：限价使用传入；市价使用 ticker 估算（用于数量换算/过滤器/冻结估算）
	price := decimal.Zero
	switch orderType {
	case ctypes.OrderTypeLimit:
		if rawPrice.LessThanOrEqual(decimal.Zero) {
			return nil, errors.New(errors.InvalidArgument, "price is required for limit order")
		}
		price = rawPrice
	default:
		tk, err := conn.Ticker(ctx, symbol)
		if err != nil {
			return nil, err
		}
		if tk == nil {
			return nil, errors.New(errors.NotFound, "ticker not available")
		}
		if !tk.LastPrice.GreaterThan(decimal.Zero) {
			return nil, errors.New(errors.NotFound, "market price not available")
		}
		price = tk.LastPrice
	}

	// 3) 数量：Quantity 与 QuoteQty 二选一
	if rawQty.GreaterThan(decimal.Zero) && rawQuoteQty.GreaterThan(decimal.Zero) {
		return nil, errors.New(errors.InvalidArgument, "quantity and quoteQty cannot be provided together")
	}
	if rawQty.LessThanOrEqual(decimal.Zero) && rawQuoteQty.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New(errors.InvalidArgument, "quantity or quoteQty required")
	}
	qty := decimal.Zero
	if rawQty.GreaterThan(decimal.Zero) {
		qty = rawQty
	} else {
		if price.LessThanOrEqual(decimal.Zero) {
			return nil, errors.New(errors.InvalidArgument, "price is zero")
		}
		if rawQuoteQty.LessThanOrEqual(decimal.Zero) {
			return nil, errors.New(errors.InvalidArgument, "invalid quote quantity")
		}
		qty = rawQuoteQty.Div(price)
	}

	// 4) 价格/数量归一化 + 过滤器校验
	price = NormalizeSymbolPrice(price, orderType, symbolCfg.Market)
	if price.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New(errors.InvalidArgument, "price adjusted to zero")
	}
	qty = NormalizeBaseAssetQty(qty, orderType, symbolCfg.Market)
	if qty.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New(errors.InvalidArgument, "quantity adjusted to zero")
	}
	if err := ValidateMarketFilters(symbolCfg.Market, orderType, price, qty, 0); err != nil {
		return nil, err
	}

	// 5) 账户级风控校验
	if err := s.riskChecker.CheckOrderRisk(ctx, acct, order); err != nil {
		return nil, err
	}

	// 6) 生成/透传 ClientOrderID
	if order.ClientOrderID == "" {
		order.ClientOrderID = ctypes.OrderId(snowflake.Generate().String())
	}
	clientOrderID := order.ClientOrderID

	// 6) 资金/保证金校验 + 冻结估算（通过订单快照 locked 写入，驱动 frozen delta）
	totalLocked, lockedAsset, err := calcOrderFrozen(order, symbolCfg)
	if err != nil {
		return nil, err
	}

	// 7) 下单 + 冻结资金 + 落库
	now := time.Now()

	// 7.1) 余额校验/锁定（含手续费 buffer）+ 冻结资金（如果 requiredLocked > 0）
	if lockedAsset != "" && totalLocked.GreaterThan(decimal.Zero) {
		reason := ctypes.LedgerReasonFundsFreeze
		if symbol.Type == ctypes.MarketTypeFuture {
			reason = ctypes.LedgerReasonOrderMarginFreeze
		}
		err = s.acctEntity.CheckAndApplyAssetOrderOccupiedUpdate(
			ctx,
			acct.ID, acct.Exchange,
			&ctypes.AssetEvent{
				WalletType: ctypes.GetWalletType(acct.Exchange, symbol.Type),
				Code:       lockedAsset,
				Locked:     lo.ToPtr(totalLocked),
				UpdatedTs:  now,
			},
			reason,
			order,
		)
		if err != nil {
			return nil, err
		}

		// 下单失败解锁资金
		defer func() {
			if err != nil {
				reason := ctypes.LedgerReasonFundsUnfreeze
				if symbol.Type == ctypes.MarketTypeFuture {
					reason = ctypes.LedgerReasonOrderMarginUnfreeze
				}
				err := s.acctEntity.CheckAndApplyAssetOrderOccupiedUpdate(ctx, acct.ID, acct.Exchange, &ctypes.AssetEvent{
					WalletType: ctypes.GetWalletType(acct.Exchange, symbol.Type),
					Code:       lockedAsset,
					Locked:     lo.ToPtr(totalLocked.Neg()),
					UpdatedTs:  time.Now(),
				}, reason, order)
				if err != nil {
					logger.Ctx(ctx).Err(err).Str("account_id", acct.ID).
						Str("exchange", acct.Exchange.String()).
						Str("asset", lockedAsset).
						Str("reason", string(reason)).
						Msg("failed to apply asset order occupied update")
				}
			}
		}()
	}

	var exchangeOrderID string
	_, err = s.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		// 7.2) 落库订单快照
		dbOrderType := orders.OrderType(string(orderType))
		if !dbOrderType.Valid() {
			return nil, fmt.Errorf("unsupported db order type: %s", orderType)
		}
		dbSide := orders.OrderSide(string(side))
		if !dbSide.Valid() {
			return nil, fmt.Errorf("unsupported db side: %s", side)
		}
		dbTif := orders.TimeInForce(string(tif))
		if !dbTif.Valid() {
			return nil, fmt.Errorf("unsupported db tif: %s", dbTif)
		}

		var lockedNum pgtype.Numeric
		var lockedAssetPtr *string
		if lockedAsset != "" && totalLocked.GreaterThan(decimal.Zero) {
			lockedNum = utils.Decimal.DecimalToPgNumeric(totalLocked)
			la := strings.ToUpper(strings.TrimSpace(lockedAsset))
			lockedAssetPtr = &la
		}

		orderPo, err := s.db.OrdersRepo.WithTx(tx).UpsertOrder(ctx, orders.UpsertOrderParams{
			BotID:         int32(botId),
			AccountID:     acct.ID,
			OrderID:       clientOrderID.String(), // 先使用 clientOrderID 占位
			ClientOrderID: clientOrderID.String(),
			DrivedOrderID: "",
			OrderType:     dbOrderType,
			AlgoType:      orders.AlgoTypeNONE,
			Source:        orders.OrderSource(string(source)),
			Exchange:      acct.Exchange.String(),
			Symbol:        symbol.String(),
			Side:          dbSide,
			IsBuy:         isBuy,
			Price:         utils.Decimal.DecimalToPgNumeric(price),
			Quantity:      utils.Decimal.DecimalToPgNumeric(qty),
			ExecutedQty:   utils.Decimal.DecimalToPgNumeric(decimal.Zero),
			ExecutedPrice: utils.Decimal.DecimalToPgNumeric(decimal.Zero),
			AvgPrice:      utils.Decimal.DecimalToPgNumeric(decimal.Zero),
			Conditions:    nil,
			Detail:        nil,
			Status:        string(ctypes.OrderStatusNew),
			RejectReason:  nil,
			ReduceOnly:    reduceOnly,
			PostOnly:      postOnly,
			Tif:           dbTif,
			CreatedTs:     now,
			WorkingTs:     lo.ToPtr(now),
			FinishedTs:    nil,
			// UpdatedTs:     now, // 这个时间是用来管理变更顺序的，由交易所覆盖；由于时间对齐问题，这里塞的时间可能晚于交易所时间，导致后续事件无法更新
			Locked:      lockedNum,
			LockedAsset: lockedAssetPtr,
			Fee:         pgtype.Numeric{Valid: false},
			FeeAsset:    nil,
			RealizedPnl: pgtype.Numeric{Valid: false},
		})
		if err != nil {
			return nil, err
		}

		// 7.3) 调用交易所下单（统一使用 quantity 下单）
		var targetPrice *decimal.Decimal
		if orderType == ctypes.OrderTypeLimit {
			targetPrice = lo.ToPtr(price)
		}
		input := mtypes.PlaceOrderInput{
			Symbol:        symbol,
			Side:          side,
			IsBuy:         isBuy,
			OrderType:     orderType,
			Price:         targetPrice,
			Quantity:      lo.ToPtr(qty),
			ClientOrderID: lo.ToPtr(clientOrderID),
			TimeInForce:   &tif,
			ReduceOnly:    &reduceOnly,
			ClosePosition: &closePosition,
		}

		if acct.AccountType == types.AccountTypeVirtualSub {
			parentAcct, err := s.resolveParentAccount(ctx, acct)
			if err != nil {
				return nil, err
			}
			order.AccountID = parentAcct.ID
			res, err := s.PlaceOrder(ctx, parentAcct, order)
			if err != nil {
				return nil, err
			}
			exchangeOrderID = res.OrderId.String()
		} else {
			res, err := conn.PlaceOrder(ctx, input)
			if err != nil {
				return nil, err
			}
			exchangeOrderID = res.OrderID.String()
		}

		// 7.4) 更新订单 ID
		return s.db.OrdersRepo.WithTx(tx).UpdateOrderId(ctx, orders.UpdateOrderIdParams{
			ID:      orderPo.ID,
			OrderID: exchangeOrderID,
		})
	})
	if err != nil {
		// 如果事务失败但交易所已经下单成功（理论上仅 commit 阶段失败），尝试补偿撤单
		if exchangeOrderID != "" {
			_ = conn.CancelOrder(context.WithoutCancel(ctx), symbol, exchangeOrderID)
		}
		return nil, err
	}

	// 增加下单成功次数
	s.cache.ZAdd(ctx, fmt.Sprintf("order:success:%s", acct.ID), redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: fmt.Sprintf("%s:%s", acct.ID, clientOrderID.String()),
	})

	return &types.PlaceOrderOutput{
		OrderId:       ctypes.OrderId(exchangeOrderID),
		ClientOrderId: clientOrderID,
		Status:        ctypes.OrderStatusNew,
	}, nil
}

func getOrderTypeRules(market *ctypes.Market, orderType ctypes.OrderType) *ctypes.MarketRules {
	if market == nil {
		return nil
	}
	for _, ot := range market.OrderTypeRules {
		if ot.OrderType == orderType {
			return &ot.Rules
		}
	}
	return &market.Rules
}

func ValidateMarketFilters(market ctypes.Market, orderType ctypes.OrderType, price, qty decimal.Decimal, openOrderCount int) error {
	rules := getOrderTypeRules(&market, orderType)
	if rules == nil {
		return errors.New(errors.InvalidArgument, fmt.Sprintf("no rules found for order type %s", orderType))
	}
	if !rules.MinPrice.IsZero() && price.LessThan(rules.MinPrice) {
		return fmt.Errorf("price %s is less than min price %s", price.String(), rules.MinPrice.String())
	}
	if !rules.MaxPrice.IsZero() && price.GreaterThan(rules.MaxPrice) {
		return fmt.Errorf("price %s is greater than max price %s", price.String(), rules.MaxPrice.String())
	}
	if orderType == ctypes.OrderTypeLimit && !rules.TickSize.IsZero() {
		adjusted := NormalizeSymbolPrice(price, orderType, market)
		if !adjusted.Equal(price) {
			return fmt.Errorf("price %s is not a multiple of tick size %s", price.String(), rules.TickSize.String())
		}
	}
	if !rules.MinQuantity.IsZero() && qty.LessThan(rules.MinQuantity) {
		return fmt.Errorf("quantity %s is less than min quantity %s", qty.String(), rules.MinQuantity.String())
	}
	if !rules.MaxQuantity.IsZero() && qty.GreaterThan(rules.MaxQuantity) {
		return fmt.Errorf("quantity %s is greater than max quantity %s", qty.String(), rules.MaxQuantity.String())
	}
	if !rules.LotSize.IsZero() {
		adjusted := NormalizeBaseAssetQty(qty, orderType, market)
		if !adjusted.Equal(qty) {
			return fmt.Errorf("quantity %s is not a multiple of lot size %s", qty.String(), rules.LotSize.String())
		}
	}
	notional := price.Mul(qty)
	if !rules.MinNotional.IsZero() && notional.LessThan(rules.MinNotional) {
		return fmt.Errorf("notional %s is less than min notional %s", notional.String(), rules.MinNotional.String())
	}
	if !rules.MaxNotional.IsZero() && notional.GreaterThan(rules.MaxNotional) {
		return fmt.Errorf("notional %s is greater than max notional %s", notional.String(), rules.MaxNotional.String())
	}
	if market.Rules.MaxOrderNum > 0 && openOrderCount >= market.Rules.MaxOrderNum {
		return fmt.Errorf("open order count %d exceeds max order num %d", openOrderCount, market.Rules.MaxOrderNum)
	}
	return nil
}

// calcOrderFrozen 计算订单冻结资金（不包括手续费）
func calcOrderFrozen(order *ctypes.Order, symbolCfg *ctypes.SymbolConfig) (decimal.Decimal, string, error) {
	marketOrderFreezeFactor := decimal.NewFromFloat(1.05)
	totalLocked := decimal.Zero
	lockedAsset := ""

	symbol := order.Symbol
	price := order.Price
	qty := order.OriginalQty
	side := order.Side
	orderType := order.OrderType

	isOpenPosition := func(posSide ctypes.PositionSide, isBuy bool) bool {
		return (posSide == ctypes.PositionSideLong && isBuy) || (posSide == ctypes.PositionSideShort && !isBuy)
	}

	switch symbol.Type {
	case ctypes.MarketTypeFuture:
		lev := int64(symbolCfg.CrossLeverage[0])
		if !order.IsBuy && symbolCfg.CrossLeverage[1] > 0 {
			lev = int64(symbolCfg.CrossLeverage[1])
		}
		if lev <= 0 {
			lev = 1
		}
		notional := price.Mul(qty)
		margin := notional.Div(decimal.NewFromInt(lev))
		lockedAsset = symbol.Quote

		openPos := isOpenPosition(side, order.IsBuy)
		if openPos {
			// 可用余额要求覆盖 margin
			totalLocked = margin
		} else {
			// 平仓：不冻结
			totalLocked = decimal.Zero
		}
	default:
		if order.IsBuy {
			lockedAsset = symbol.Quote
			totalLocked = price.Mul(qty)
			if orderType == ctypes.OrderTypeMarket {
				totalLocked = totalLocked.Mul(marketOrderFreezeFactor)
			}
		} else {
			lockedAsset = symbol.Base
			totalLocked = qty
		}
	}
	return totalLocked, lockedAsset, nil
}
