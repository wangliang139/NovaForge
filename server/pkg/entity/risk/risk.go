package risk

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/market"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

const cooldownKeyPrefix = "risk:cooldown:"

var StableCoins = []string{
	"USDT",
	"USDC",
	"BUSD",
	"DAI",
	"TUSD",
	"FDUSD",
}

var StableCoinSet = map[string]struct{}{
	StableCoins[0]: {},
	StableCoins[1]: {},
	StableCoins[2]: {},
	StableCoins[3]: {},
	StableCoins[4]: {},
	StableCoins[5]: {},
}

// CooldownKey 生成账户冷静期在 Redis 中使用的 key
func CooldownKey(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ""
	}
	return cooldownKeyPrefix + accountID
}

type AccountProvider interface {
	GetSnapshot(ctx context.Context, accountID string) (*types.AccountState, error)
}

type OrderGateway interface {
	PlaceOrder(ctx context.Context, acct *types.Account, order *ctypes.Order) (*types.PlaceOrderOutput, error)
}

type RiskController struct {
	cache redis.UniversalClient

	engine       *market.Engine
	acctProvider AccountProvider
	orderGateway OrderGateway
}

func NewRiskController(cache redis.UniversalClient, engine *market.Engine) *RiskController {
	return &RiskController{
		cache:  cache,
		engine: engine,
	}
}

func (s *RiskController) SetAccountProvider(accountProvider AccountProvider) {
	s.acctProvider = accountProvider
}

func (s *RiskController) SetOrderGateway(orderGateway OrderGateway) {
	s.orderGateway = orderGateway
}

// SetCooldown 为账户设置冷静期（秒），使用 Redis 过期键实现
func (s *RiskController) SetCooldown(ctx context.Context, accountID string, seconds int32) {
	if seconds <= 0 {
		return
	}
	key := CooldownKey(accountID)
	if key == "" {
		return
	}
	ttl := time.Duration(seconds) * time.Second
	// 不阻塞主流程，忽略错误
	_ = s.cache.Set(ctx, key, time.Now().Format(time.RFC3339), ttl).Err()
}

// InCooldown 判断账户当前是否处于冷静期内
func (s *RiskController) InCooldown(ctx context.Context, accountID string) (bool, error) {
	key := CooldownKey(accountID)
	if key == "" {
		return false, nil
	}
	val, err := s.cache.Get(ctx, key).Result()
	if err != nil {
		// key 不存在或其他错误时视为不在冷静期，让上层决定是否忽略错误
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(val) == "" {
		return false, nil
	}
	return true, nil
}

// IsRiskReducingOrder 判断是否为降险订单（用于豁免部分风控规则）
// 合约减仓/现货买入稳定币视为降险
func IsRiskReducingOrder(order *ctypes.Order) bool {
	if order.Symbol.Type == ctypes.MarketTypeFuture {
		if order.IsReducePosition() {
			return true
		}
	} else {
		base := strings.ToUpper(order.Symbol.Base)
		quote := strings.ToUpper(order.Symbol.Quote)
		if _, ok := StableCoinSet[base]; ok && order.IsBuy {
			return false
		}
		if _, ok := StableCoinSet[quote]; ok && !order.IsBuy {
			return false
		}
	}
	return false
}

func (s *RiskController) CheckOrderRisk(ctx context.Context, acct *types.Account, order *ctypes.Order) error {
	if acct.Config == nil {
		return nil
	}
	if order == nil {
		return fmt.Errorf("order is required")
	}
	if acct == nil {
		return fmt.Errorf("account is required")
	}
	if s.acctProvider == nil {
		return fmt.Errorf("account provider is not set")
	}

	exchange := acct.Exchange
	symbol := order.Symbol
	side := order.Side

	acctState, err := s.acctProvider.GetSnapshot(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("get account state failed: %w", err)
	}
	if acctState == nil {
		return fmt.Errorf("account state is nil")
	}

	// 降险订单豁免大部分规则（单笔订单、单标持仓、日亏损、最大杠杆、维持保证金率、净敞口、总敞口）
	isReducing := IsRiskReducingOrder(order)

	// 冷静期：若账户处于冷静期内，禁止加仓类订单（降险订单仍可执行）
	if acct.Config.CooldownSeconds > 0 && !isReducing {
		ctxRedis, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		inCooldown, err := s.InCooldown(ctxRedis, acct.ID)
		if err == nil && inCooldown {
			return fmt.Errorf("account is in cooldown period, trading temporarily disabled")
		}
	}

	// 计算订单价格
	price := order.Price
	if order.OrderType != ctypes.OrderTypeLimit && price.IsZero() {
		lastPrice, err := s.engine.GetMarketProvider().GetLastPrice(ctx, order.Exchange, order.Symbol)
		if err != nil {
			return err
		}
		if lastPrice.IsZero() {
			return fmt.Errorf("last price is zero")
		}
		price = lastPrice
	}

	// 计算订单名义价值
	notional := price.Mul(order.OriginalQty)

	// 1. 单笔订单限额
	if !isReducing {
		maxOrderSize := acct.Config.MaxOrderSize
		if maxOrderSize.GreaterThan(decimal.Zero) && notional.GreaterThan(maxOrderSize) {
			return fmt.Errorf("order notional %s exceeds max order size %s", notional.String(), maxOrderSize.String())
		}
	}

	// 5. 下单频率限制（每分钟最大订单数）
	if acct.Config.MaxOrdersPerMinute > 0 {
		if err := s.checkOrderRateLimit(ctx, acct.ID, acct.Config.MaxOrdersPerMinute); err != nil {
			return err
		}
	}

	var currentNotional decimal.Decimal
	for _, p := range acctState.Positions {
		if p.Symbol == symbol {
			if p.Notional.IsZero() {
				currentNotional = currentNotional.Add(p.Amount.Mul(p.EntryPrice))
			} else {
				currentNotional = currentNotional.Add(p.Notional)
			}
			break
		}
	}

	// 2. 单标持仓限额（AmountLimit.EffectiveLimit）
	if !isReducing {
		// 仅对 futures 标的生效；现货持仓语义在规则 7/8 中体现
		var limitPerSymbol decimal.Decimal
		// 需要账户权益，后续统一计算
		limitPerSymbol = acct.Config.MaxPositionPerSymbol.EffectiveLimit(acctState.Equity)
		if limitPerSymbol.GreaterThan(decimal.Zero) {
			var newNotional decimal.Decimal
			if side == ctypes.PositionSideLong {
				newNotional = currentNotional.Add(notional)
			} else {
				if currentNotional.GreaterThanOrEqual(notional) {
					newNotional = currentNotional.Sub(notional)
				} else {
					newNotional = notional.Sub(currentNotional)
				}
			}
			if newNotional.GreaterThan(limitPerSymbol) {
				return fmt.Errorf("position size %s would exceed max position per symbol limit %s", newNotional.String(), limitPerSymbol.String())
			}
		}
	}

	// 4. 最大杠杆（预估下单后的账户杠杆）
	equityForLeverage := acctState.Equity
	if !isReducing && acct.Config.MaxLeverage.GreaterThan(decimal.Zero) && order.Symbol.Type == ctypes.MarketTypeFuture && equityForLeverage.GreaterThan(decimal.Zero) {
		// 以总 futures notional / equity 近似账户杠杆
		totalFuturesNotional := decimal.Zero
		for _, p := range acctState.Positions {
			if p == nil || p.Symbol.Type != ctypes.MarketTypeFuture || p.Amount.IsZero() {
				continue
			}
			n := p.Notional
			if n.IsZero() {
				n = p.Amount.Mul(p.EntryPrice)
			}
			totalFuturesNotional = totalFuturesNotional.Add(n)
		}
		newTotalNotional := totalFuturesNotional.Add(notional)
		if newTotalNotional.GreaterThan(decimal.Zero) {
			newLev := newTotalNotional.Div(equityForLeverage)
			if newLev.GreaterThan(acct.Config.MaxLeverage) {
				return fmt.Errorf("account leverage %s would exceed max leverage %s", newLev.String(), acct.Config.MaxLeverage.String())
			}
		}
	}

	// 3. 日亏损限额（amount/ratio）
	if !isReducing {
		maxDailyLossLimit := acct.Config.MaxDailyLoss.EffectiveLimit(acctState.Equity)
		if maxDailyLossLimit.GreaterThan(decimal.Zero) {
			dailyRealized := acctState.DailyPnL
			if dailyRealized.LessThan(decimal.Zero) {
				loss := dailyRealized.Neg()
				if loss.GreaterThan(maxDailyLossLimit) {
					return fmt.Errorf("daily realized loss %s exceeds max daily loss %s", loss.String(), maxDailyLossLimit.String())
				}
			}
		}
	}

	// 6/7/8 相关计算需要账户权益，用于 AmountLimit.EffectiveLimit
	equity := acctState.Equity

	// 6. 维持保证金率（仅合约账户，简单按全账户计算）
	if !isReducing && acct.Config.MinMaintenanceMarginRatio.GreaterThan(decimal.Zero) {
		markPrices, err := s.engine.GetMarketProvider().GetMarkPrices(ctx, exchange)
		if err != nil {
			return fmt.Errorf("get mark prices failed: %w", err)
		}
		mpMap := buildMarkPriceMap(markPrices)

		totalMaintMargin := decimal.Zero
		constMMR := decimal.NewFromFloat(0.005) // 示例 MMR，后续可从 GetLeverageBracket 获取
		for _, p := range acctState.Positions {
			if p == nil || p.Symbol.Type != ctypes.MarketTypeFuture || p.Amount.IsZero() {
				continue
			}
			mp, ok := mpMap[p.Symbol.String()]
			if !ok || mp.LessThanOrEqual(decimal.Zero) {
				continue
			}
			notionalP := p.Amount.Mul(mp)
			totalMaintMargin = totalMaintMargin.Add(notionalP.Mul(constMMR))
		}

		if totalMaintMargin.GreaterThan(decimal.Zero) && equity.GreaterThan(decimal.Zero) {
			ratio := totalMaintMargin.Div(equity)
			if ratio.GreaterThan(acct.Config.MinMaintenanceMarginRatio) {
				return fmt.Errorf("maintenance margin ratio %s exceeds limit %s", ratio.String(), acct.Config.MinMaintenanceMarginRatio.String())
			}
		}
	}

	// 7/8. 净敞口 & 总敞口限额（现货非稳定币 + 合约）
	if !isReducing && (acct.Config.MaxTotalNetExposure.Amount.GreaterThan(decimal.Zero) ||
		acct.Config.MaxTotalNetExposure.Ratio.GreaterThan(decimal.Zero) ||
		acct.Config.MaxTotalGrossExposure.Amount.GreaterThan(decimal.Zero) ||
		acct.Config.MaxTotalGrossExposure.Ratio.GreaterThan(decimal.Zero)) {

		// 现货非稳定币 notional（视为多头）
		spotNotionalByBase := s.spotNotionalByBase(ctx, exchange, acctState.Assets)
		spotNotional := decimal.Zero
		for _, v := range spotNotionalByBase {
			spotNotional = spotNotional.Add(v)
		}

		// 合约多空敞口
		longNotional := decimal.Zero
		shortNotional := decimal.Zero
		grossFutures := decimal.Zero

		markPrices, err := s.engine.GetMarketProvider().GetMarkPrices(ctx, exchange)
		if err != nil {
			return fmt.Errorf("get mark prices failed: %w", err)
		}
		mpMap := buildMarkPriceMap(markPrices)

		for _, p := range acctState.Positions {
			if p == nil || p.Symbol.Type != ctypes.MarketTypeFuture || p.Amount.IsZero() {
				continue
			}
			mp, ok := mpMap[p.Symbol.String()]
			if !ok || mp.LessThanOrEqual(decimal.Zero) {
				continue
			}
			notionalP := p.Amount.Mul(mp)
			grossFutures = grossFutures.Add(notionalP)
			if p.Side == ctypes.PositionSideLong {
				longNotional = longNotional.Add(notionalP)
			} else {
				shortNotional = shortNotional.Add(notionalP)
			}
		}

		netExposure := spotNotional.Add(longNotional).Sub(shortNotional)
		grossExposure := grossFutures.Add(spotNotional)

		// 7. 净敞口限额
		if acct.Config.MaxTotalNetExposure.Amount.GreaterThan(decimal.Zero) || acct.Config.MaxTotalNetExposure.Ratio.GreaterThan(decimal.Zero) {
			limitNet := acct.Config.MaxTotalNetExposure.EffectiveLimit(equity)
			if limitNet.GreaterThan(decimal.Zero) && netExposure.Abs().GreaterThan(limitNet) {
				return fmt.Errorf("net exposure %s exceeds limit %s", netExposure.Abs().String(), limitNet.String())
			}
		}

		// 8. 总敞口限额
		if acct.Config.MaxTotalGrossExposure.Amount.GreaterThan(decimal.Zero) || acct.Config.MaxTotalGrossExposure.Ratio.GreaterThan(decimal.Zero) {
			limitGross := acct.Config.MaxTotalGrossExposure.EffectiveLimit(equity)
			if limitGross.GreaterThan(decimal.Zero) && grossExposure.GreaterThan(limitGross) {
				return fmt.Errorf("gross exposure %s exceeds limit %s", grossExposure.String(), limitGross.String())
			}
		}
	}

	return nil
}

// checkOrderRateLimit 进行账户级下单频率限制
func (s *RiskController) checkOrderRateLimit(ctx context.Context, accountID string, maxPerMinute int) error {
	if maxPerMinute <= 0 {
		return nil
	}

	// 使用 Redis ZSET 按时间窗口计算1分钟内下单数
	key := fmt.Sprintf("order:success:%s", strings.TrimSpace(accountID))
	nowTs := time.Now().Unix()
	oneMinuteAgo := nowTs - 60

	// 清理一小时前的过期数据，防止ZSET无限增长
	_ = s.cache.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", nowTs-3600)).Err()

	// 统计最近1分钟的下单数
	count, err := s.cache.ZCount(ctx, key, fmt.Sprintf("%d", oneMinuteAgo), fmt.Sprintf("%d", nowTs)).Result()
	if err != nil {
		// Redis 异常时不阻塞下单，仅记录日志
		logger.Ctx(ctx).Err(err).Str("account_id", accountID).Msg("order rate limit redis error")
		return nil
	}
	if count >= int64(maxPerMinute) {
		return fmt.Errorf("order rate exceeded: %d orders in last minute, limit %d", count, maxPerMinute)
	}

	return nil
}

// CheckAccountRisk 检查单个账户的风控规则，并按计划执行必要动作（部分规则仅告警）
func (s *RiskController) CheckAccountRisk(ctx context.Context, acct *types.Account) (*types.RiskCheckResult, error) {
	if acct == nil {
		return nil, fmt.Errorf("account is nil")
	}
	if acct.Config == nil {
		return &types.RiskCheckResult{
			AccountID: acct.ID,
			Success:   true,
		}, nil
	}
	if s.acctProvider == nil {
		return nil, fmt.Errorf("account provider is not set")
	}

	accountID := acct.ID

	triggered := make([]string, 0, 8)

	// 如果目前还在冷静期，则不校验
	if inCooldown, err := s.InCooldown(ctx, accountID); err != nil {
		return nil, fmt.Errorf("check in cooldown failed: %w", err)
	} else if inCooldown {
		return &types.RiskCheckResult{
			AccountID: acct.ID,
			Success:   false,
			Error:     "in cooldown",
		}, nil
	}

	acctState, err := s.acctProvider.GetSnapshot(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account state failed: %w", err)
	}
	if acctState == nil {
		return nil, fmt.Errorf("account state is nil")
	}

	// 规则 3：日亏损限额（使用 realized_pnl）
	dailyLoss := acctState.DailyPnL
	if !acct.Config.MaxDailyLoss.Amount.IsZero() || !acct.Config.MaxDailyLoss.Ratio.IsZero() {
		limitDaily := acct.Config.MaxDailyLoss.EffectiveLimit(acctState.Equity)
		if dailyLoss.LessThan(decimal.Zero) && limitDaily.GreaterThan(decimal.Zero) {
			lossAbs := dailyLoss.Neg()
			if lossAbs.GreaterThan(limitDaily) {
				triggered = append(triggered, "max_daily_loss")
				// 行为：close_position（平掉所有合约持仓）
				if err := s.closeAllFuturesPositions(ctx, acct, acctState.Positions); err != nil {
					logger.Ctx(ctx).Err(err).Str("account_id", accountID).Msg("close positions on daily loss failed")
				}
				if err := s.sellAllNonStableSpot(ctx, acct, acctState.Assets); err != nil {
					logger.Ctx(ctx).Err(err).Str("account_id", accountID).Msg("sell non-stable spot on daily loss failed")
				}

				// 设置冷静期
				if acct.Config.CooldownSeconds > 0 {
					s.SetCooldown(ctx, accountID, acct.Config.CooldownSeconds)
				}
			}
		}
	}

	// 规则 4：最大杠杆（alert_only）
	if acct.Config.MaxLeverage.GreaterThan(decimal.Zero) {
		for _, p := range acctState.Positions {
			if p == nil || p.Symbol.Type != ctypes.MarketTypeFuture || p.Leverage <= 0 {
				continue
			}
			if decimal.NewFromInt(int64(p.Leverage)).GreaterThan(acct.Config.MaxLeverage) {
				triggered = append(triggered, "max_leverage")
				break
			}
		}
	}

	// 规则 6：维持保证金率（仅告警，减仓行为后续细化）
	if acct.Config.MinMaintenanceMarginRatio.GreaterThan(decimal.Zero) && acctState.Equity.GreaterThan(decimal.Zero) {
		mpList, err := s.engine.GetMarketProvider().GetMarkPrices(ctx, acct.Exchange)
		if err == nil {
			mpMap := buildMarkPriceMap(mpList)
			totalMaintMargin := decimal.Zero
			constMMR := decimal.NewFromFloat(0.005)
			for _, p := range acctState.Positions {
				if p == nil || p.Symbol.Type != ctypes.MarketTypeFuture || p.Amount.IsZero() {
					continue
				}
				mp, ok := mpMap[p.Symbol.String()]
				if !ok || mp.LessThanOrEqual(decimal.Zero) {
					continue
				}
				notionalP := p.Amount.Mul(mp)
				totalMaintMargin = totalMaintMargin.Add(notionalP.Mul(constMMR))
			}
			if totalMaintMargin.GreaterThan(decimal.Zero) {
				ratio := totalMaintMargin.Div(acctState.Equity)
				if ratio.GreaterThan(acct.Config.MinMaintenanceMarginRatio) {
					triggered = append(triggered, "min_maintenance_margin_ratio")
				}
			}
		}
	}

	// 规则 7/8：净敞口 & 总敞口
	var netExposure, grossExposure decimal.Decimal
	{
		// 现货非稳定币 notional
		spotNotionalByBase := s.spotNotionalByBase(ctx, acct.Exchange, acctState.Assets)

		spotTotal := decimal.Zero
		for _, v := range spotNotionalByBase {
			spotTotal = spotTotal.Add(v)
		}

		mpList, err := s.engine.GetMarketProvider().GetMarkPrices(ctx, acct.Exchange)
		mpMap := make(map[string]decimal.Decimal)
		if err == nil {
			mpMap = buildMarkPriceMap(mpList)
		}

		longNotional := decimal.Zero
		shortNotional := decimal.Zero
		grossFutures := decimal.Zero
		for _, p := range acctState.Positions {
			if p == nil || p.Symbol.Type != ctypes.MarketTypeFuture || p.Amount.IsZero() {
				continue
			}
			mp, ok := mpMap[p.Symbol.String()]
			if !ok || mp.LessThanOrEqual(decimal.Zero) {
				continue
			}
			notionalP := p.Amount.Mul(mp)
			grossFutures = grossFutures.Add(notionalP)
			if p.Side == ctypes.PositionSideLong {
				longNotional = longNotional.Add(notionalP)
			} else {
				shortNotional = shortNotional.Add(notionalP)
			}
		}

		netExposure = spotTotal.Add(longNotional).Sub(shortNotional)
		grossExposure = grossFutures.Add(spotTotal)

		if (!acct.Config.MaxTotalNetExposure.Amount.IsZero() || !acct.Config.MaxTotalNetExposure.Ratio.IsZero()) && acctState.Equity.GreaterThan(decimal.Zero) {
			limitNet := acct.Config.MaxTotalNetExposure.EffectiveLimit(acctState.Equity)
			if limitNet.GreaterThan(decimal.Zero) && netExposure.Abs().GreaterThan(limitNet) {
				triggered = append(triggered, "max_total_net_exposure")
			}
		}
		if (!acct.Config.MaxTotalGrossExposure.Amount.IsZero() || !acct.Config.MaxTotalGrossExposure.Ratio.IsZero()) && acctState.Equity.GreaterThan(decimal.Zero) {
			limitGross := acct.Config.MaxTotalGrossExposure.EffectiveLimit(acctState.Equity)
			if limitGross.GreaterThan(decimal.Zero) && grossExposure.GreaterThan(limitGross) {
				triggered = append(triggered, "max_total_gross_exposure")
			}
		}
	}

	// 风险指数（第 9 项）：0–100
	riskIndex := decimal.Zero
	if acct.Config.RiskIndexThreshold.GreaterThan(decimal.Zero) {
		riskIndex = s.calculateRiskIndex(ctx, acct, acctState, dailyLoss)
		if riskIndex.GreaterThan(acct.Config.RiskIndexThreshold) {
			triggered = append(triggered, "risk_index")
			if strings.EqualFold(acct.Config.RiskIndexAction, "close_and_sell") {
				// 先平掉所有合约持仓，再卖出非稳定币
				if err := s.closeAllFuturesPositions(ctx, acct, acctState.Positions); err != nil {
					logger.Ctx(ctx).Err(err).Str("account_id", accountID).Msg("close positions on risk index failed")
				}
				if err := s.sellAllNonStableSpot(ctx, acct, acctState.Assets); err != nil {
					logger.Ctx(ctx).Err(err).Str("account_id", accountID).Msg("sell non-stable spot on risk index failed")
				}

				// 设置冷静期
				if acct.Config.CooldownSeconds > 0 {
					s.SetCooldown(ctx, accountID, acct.Config.CooldownSeconds)
				}
			}
		}
	}

	return &types.RiskCheckResult{
		AccountID:      accountID,
		TriggeredRules: triggered,
		Success:        true,
		RiskIndex:      riskIndex.String(),
	}, nil
}

func (s *RiskController) closeAllFuturesPositions(ctx context.Context, acct *types.Account, positions []*ctypes.Position) error {
	if s.orderGateway == nil {
		return fmt.Errorf("order gateway is not set")
	}
	for _, p := range positions {
		if p == nil || p.Symbol.Type != ctypes.MarketTypeFuture || p.Amount.IsZero() {
			continue
		}
		qty := p.Amount
		if qty.LessThanOrEqual(decimal.Zero) {
			continue
		}
		isBuy := p.Side == ctypes.PositionSideShort
		order := &ctypes.Order{
			AccountID:     acct.ID,
			Exchange:      acct.Exchange,
			Source:        ctypes.OrderSourceLiquidation,
			Symbol:        p.Symbol,
			Side:          p.Side,
			IsBuy:         isBuy,
			OrderType:     ctypes.OrderTypeMarket,
			OriginalQty:   qty,
			ReduceOnly:    true,
			ClosePosition: true,
		}
		if _, err := s.orderGateway.PlaceOrder(ctx, acct, order); err != nil {
			logger.Ctx(ctx).Err(err).
				Str("account_id", p.AccountID).
				Str("symbol", p.Symbol.String()).
				Msg("failed to place close position order in risk check")
		}
	}
	return nil
}

func (s *RiskController) sellAllNonStableSpot(ctx context.Context, acct *types.Account, assets []*ctypes.AssetBo) error {
	if s.orderGateway == nil {
		return fmt.Errorf("order gateway is not set")
	}
	for _, a := range assets {
		if a == nil {
			continue
		}
		code := strings.ToUpper(a.Code)
		if _, ok := StableCoinSet[code]; ok {
			continue
		}
		if a.Balance.LessThanOrEqual(decimal.Zero) {
			continue
		}
		sym := ctypes.NewSymbol(code, "USDT", ctypes.MarketTypeSpot)
		qty := a.Balance
		if qty.LessThanOrEqual(decimal.Zero) {
			continue
		}
		order := &ctypes.Order{
			AccountID:   acct.ID,
			Exchange:    acct.Exchange,
			Source:      ctypes.OrderSourceLiquidation,
			Symbol:      sym,
			IsBuy:       false,
			OrderType:   ctypes.OrderTypeMarket,
			OriginalQty: qty,
		}
		if _, err := s.orderGateway.PlaceOrder(ctx, acct, order); err != nil {
			logger.Ctx(ctx).Err(err).
				Str("asset", code).
				Msg("failed to sell non-stable spot in risk check")
		}
	}
	return nil
}

func (s *RiskController) CalculateRiskIndex(ctx context.Context, acct *types.Account, acctState *types.AccountState) decimal.Decimal {
	if acct == nil || acctState == nil || acct.Config == nil {
		return decimal.Zero
	}
	dailyLoss := acctState.DailyPnL
	return s.calculateRiskIndex(ctx, acct, acctState, dailyLoss)
}

func (s *RiskController) calculateRiskIndex(ctx context.Context, acct *types.Account, acctState *types.AccountState, dailyLoss decimal.Decimal) decimal.Decimal {
	// 1) 持仓集中度：max(单标 notional / 总 notional)
	spotNotionalByBase := s.spotNotionalByBase(ctx, acct.Exchange, acctState.Assets)
	baseNotional := make(map[string]decimal.Decimal, len(spotNotionalByBase))
	totalNotional := decimal.Zero

	for code, notional := range spotNotionalByBase {
		baseNotional[code] = notional
		totalNotional = totalNotional.Add(notional)
	}

	// futures by base & futures gross exposure
	mpList2, err := s.engine.GetMarketProvider().GetMarkPrices(ctx, acct.Exchange)
	mpMap := make(map[string]decimal.Decimal)
	if err == nil {
		mpMap = buildMarkPriceMap(mpList2)
	}
	grossFutures := decimal.Zero
	for _, p := range acctState.Positions {
		if p == nil || p.Symbol.Type != ctypes.MarketTypeFuture || p.Amount.IsZero() {
			continue
		}
		mp, ok := mpMap[p.Symbol.String()]
		if !ok || mp.LessThanOrEqual(decimal.Zero) {
			continue
		}
		base := strings.ToUpper(p.Symbol.Base)
		notional := p.Amount.Mul(mp)
		grossFutures = grossFutures.Add(notional)
		baseNotional[base] = baseNotional[base].Add(notional)
		totalNotional = totalNotional.Add(notional)
	}

	posConcentrationScore := decimal.Zero
	if totalNotional.GreaterThan(decimal.Zero) {
		maxBase := decimal.Zero
		for _, v := range baseNotional {
			if v.GreaterThan(maxBase) {
				maxBase = v
			}
		}
		if maxBase.GreaterThan(decimal.Zero) {
			posConcentrationScore = maxBase.Div(totalNotional)
		}
	}

	// 2) 单标占用：max(单标 notional) / MaxPositionPerSymbol
	singleUsageScore := decimal.Zero
	if (!acct.Config.MaxPositionPerSymbol.Amount.IsZero() || !acct.Config.MaxPositionPerSymbol.Ratio.IsZero()) && acctState.Equity.GreaterThan(decimal.Zero) {
		limit := acct.Config.MaxPositionPerSymbol.EffectiveLimit(acctState.Equity)
		if limit.GreaterThan(decimal.Zero) {
			maxBase := decimal.Zero
			for _, v := range baseNotional {
				if v.GreaterThan(maxBase) {
					maxBase = v
				}
			}
			if maxBase.GreaterThan(decimal.Zero) {
				singleUsageScore = maxBase.Div(limit)
				if singleUsageScore.GreaterThan(decimal.NewFromInt(1)) {
					singleUsageScore = decimal.NewFromInt(1)
				}
			}
		}
	}

	// 3) 日亏损占用：min(1, |dailyLoss| / MaxDailyLoss)
	dailyLossScore := decimal.Zero
	if (!acct.Config.MaxDailyLoss.Amount.IsZero() || !acct.Config.MaxDailyLoss.Ratio.IsZero()) && acctState.Equity.GreaterThan(decimal.Zero) {
		limitDaily := acct.Config.MaxDailyLoss.EffectiveLimit(acctState.Equity)
		if limitDaily.GreaterThan(decimal.Zero) && dailyLoss.LessThan(decimal.Zero) {
			lossAbs := dailyLoss.Neg()
			dailyLossScore = lossAbs.Div(limitDaily)
			if dailyLossScore.GreaterThan(decimal.NewFromInt(1)) {
				dailyLossScore = decimal.NewFromInt(1)
			}
		}
	}

	// 4) 杠杆占用：actualLeverage / MaxLeverage
	leverageScore := decimal.Zero
	if acct.Config.MaxLeverage.GreaterThan(decimal.Zero) && acctState.Equity.GreaterThan(decimal.Zero) {
		// 以总合约名义价值 / equity 近似账户杠杆
		// 以总合约名义价值 + 现货非稳定币 / equity 近似账户杠杆
		spotTotal := decimal.Zero
		for _, v := range spotNotionalByBase {
			spotTotal = spotTotal.Add(v)
		}
		grossExposure := grossFutures.Add(spotTotal)
		actualLev := decimal.Zero
		if grossExposure.GreaterThan(decimal.Zero) {
			actualLev = grossExposure.Div(acctState.Equity)
		}
		if actualLev.GreaterThan(decimal.Zero) {
			leverageScore = actualLev.Div(acct.Config.MaxLeverage)
			if leverageScore.GreaterThan(decimal.NewFromInt(1)) {
				leverageScore = decimal.NewFromInt(1)
			}
		}
	}

	// 权重：30% / 25% / 25% / 20%
	wPos := decimal.NewFromFloat(0.3)
	wSingle := decimal.NewFromFloat(0.25)
	wDaily := decimal.NewFromFloat(0.25)
	wLev := decimal.NewFromFloat(0.2)

	sum := wPos.Mul(posConcentrationScore).
		Add(wSingle.Mul(singleUsageScore)).
		Add(wDaily.Mul(dailyLossScore)).
		Add(wLev.Mul(leverageScore))

	riskIndex := sum.Mul(decimal.NewFromInt(100))
	if riskIndex.LessThan(decimal.Zero) {
		return decimal.Zero
	}
	if riskIndex.GreaterThan(decimal.NewFromInt(100)) {
		return decimal.NewFromInt(100)
	}
	return riskIndex
}

func buildMarkPriceMap(markPrices []*ctypes.MarkPrice) map[string]decimal.Decimal {
	mpMap := make(map[string]decimal.Decimal, len(markPrices))
	for _, mp := range markPrices {
		if mp == nil {
			continue
		}
		mpMap[mp.Symbol.String()] = mp.MarkPrice
	}
	return mpMap
}

func (s *RiskController) spotNotionalByBase(ctx context.Context, exchange ctypes.Exchange, assets []*ctypes.AssetBo) map[string]decimal.Decimal {
	spotNotionalByBase := make(map[string]decimal.Decimal)
	for _, a := range assets {
		if a == nil {
			continue
		}
		code := strings.ToUpper(a.Code)
		if _, ok := StableCoinSet[code]; ok {
			continue
		}
		if a.Balance.LessThanOrEqual(decimal.Zero) {
			continue
		}
		sym := ctypes.NewSymbol(code, "USDT", ctypes.MarketTypeSpot)
		price, err := s.engine.GetMarketProvider().GetLastPrice(ctx, exchange, sym)
		if err != nil || price.LessThanOrEqual(decimal.Zero) {
			continue
		}
		notional := a.Balance.Mul(price)
		spotNotionalByBase[code] = spotNotionalByBase[code].Add(notional)
	}
	return spotNotionalByBase
}
