package exchange

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	bridge "github.com/wangliang139/NovaForge/server/pkg/strategy/exchange/bridge"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/exchange/matching"
	mb "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/marketdata"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// BacktestGateway 是策略内部与撮合引擎之间的桥梁：
// - 对外：实现 exchange.Gateway，同步下单/撤单
// - 对内：订阅 bus 的 MarketSignal，转为 MarketEvent 推入撮合引擎 link
// - 同时消费撮合引擎产出的交易所语义事件，转为策略内部 signal 发布到 bus
type BacktestGateway struct {
	meConfig matching.MatchingConfig

	bus   mb.Bus
	evBus bridge.Bus
	me    *matching.MatchingEngine
	subID bridge.SubscriptionID

	clock          clock.Clock
	accountEngine  strategy.AccountEngine
	marketProvider marketdata.MarketProvider

	baseCurrency string          // 统一计价货币（如 USDT）
	baseExchange ctypes.Exchange // 默认交易所（用于汇率查询）

	mu               sync.RWMutex
	clientToExchange map[ctypes.OrderId]ctypes.OrderId
	exchangeToClient map[ctypes.OrderId]ctypes.OrderId
	positions        map[string]gatewayPositionState // key: "<accountId>|<exSymbolKey>"，净仓位快照缓存

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
}

var _ strategy.Gateway = (*BacktestGateway)(nil)

type gatewayPositionState struct {
	qty   decimal.Decimal
	entry decimal.Decimal
}

func NewExchangeGateway(
	bus mb.Bus,
	clock clock.Clock,
	accountEngine strategy.AccountEngine,
	marketProvider marketdata.MarketProvider,
	baseCurrency string,
	baseExchange ctypes.Exchange,
) (*BacktestGateway, error) {
	if bus == nil {
		return nil, fmt.Errorf("event bus is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("clock is required")
	}
	if accountEngine == nil {
		return nil, fmt.Errorf("account engine is required")
	}
	if marketProvider == nil {
		return nil, fmt.Errorf("market data provider is required")
	}
	if baseCurrency == "" {
		baseCurrency = "USDT"
	}
	if !baseExchange.IsValid() {
		baseExchange = ctypes.ExchangeBinance
	}

	evBus := bridge.NewSyncBus()

	meConfig := matching.DefaultMatchingConfig()

	me, err := matching.NewMatchingEngine(meConfig, clock, evBus, accountEngine, marketProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create matching engine: %w", err)
	}

	return &BacktestGateway{
		meConfig:         meConfig,
		bus:              bus,
		evBus:            evBus,
		clock:            clock,
		me:               me,
		accountEngine:    accountEngine,
		marketProvider:   marketProvider,
		baseCurrency:     baseCurrency,
		baseExchange:     baseExchange,
		clientToExchange: make(map[ctypes.OrderId]ctypes.OrderId),
		exchangeToClient: make(map[ctypes.OrderId]ctypes.OrderId),
		positions:        make(map[string]gatewayPositionState, 128),
	}, nil
}

// Start 启动 gateway：订阅市场事件、启动撮合引擎 loop、消费撮合事件并写入 bus。
func (g *BacktestGateway) Start(ctx context.Context) error {
	if g == nil {
		return nil
	}
	g.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		g.cancel = cancel

		// 订阅撮合引擎输出的交易所语义事件（同步 bus）
		if g.evBus != nil {
			id, _ := g.evBus.Subscribe(func(c context.Context, ev bridge.ExchangeEvent) error {
				return g.handleExchangeEvent(c, ev)
			})
			g.subID = id
		}

		// 当前版本 market signal 由 BacktestExecutor 同步调用 OnMarketSignal，因此这里无需额外 goroutine
		_ = runCtx
	})
	return nil
}

func (g *BacktestGateway) Stop() {
	if g == nil {
		return
	}
	g.stopOnce.Do(func() {
		if g.cancel != nil {
			g.cancel()
		}
		if g.evBus != nil && g.subID != "" {
			_ = g.evBus.Unsubscribe(g.subID)
		}
	})
}

func (g *BacktestGateway) PlaceOrder(ctx context.Context, intent stypes.OrderPlaceIntent) (ctypes.OrderId, error) {
	if g == nil || g.me == nil {
		return "", fmt.Errorf("exchange gateway is nil")
	}
	if intent.GetAccountID() == nil {
		return "", fmt.Errorf("account id is required")
	}
	if intent.GetExchange() == nil || intent.GetSymbol() == nil {
		return "", fmt.Errorf("exchange/symbol is required")
	}
	clientOrderID := ctypes.OrderId(intent.ClientOrderID)
	if clientOrderID == "" {
		return "", fmt.Errorf("client order id is required")
	}

	exSymbol := ctypes.NewExSymbol(*intent.GetExchange(), *intent.GetSymbol())
	// accountID := lo.FromPtr(intent.GetAccountID())

	// 价格：限价单必须提供；市价单用于冻结/校验时使用 lastPx 做估算
	var px decimal.Decimal
	if intent.OrderType == ctypes.OrderTypeLimit {
		if intent.Price == nil {
			return "", fmt.Errorf("price is required for limit order")
		}
		px = *intent.Price
		if px.LessThanOrEqual(decimal.Zero) {
			return "", fmt.Errorf("invalid price")
		}
	} else {
		// market：需要一个可用价格估算 qty/冻结资金
		last, err := g.marketProvider.GetLastPrice(ctx, exSymbol.Exchange, exSymbol.Symbol)
		if err != nil || last.LessThanOrEqual(decimal.Zero) {
			return "", fmt.Errorf("market price not available: %w", err)
		}
		px = last
	}

	if px.LessThanOrEqual(decimal.Zero) {
		return "", fmt.Errorf("price adjusted to zero")
	}

	// 数量
	if intent.Quantity != nil && intent.Quantity.LessThanOrEqual(decimal.Zero) {
		return "", fmt.Errorf("invalid quantity")
	}

	// qty := *intent.Quantity

	// 验证资金/仓位（只读查询，不修改状态）
	// 资金冻结由 OrderManager 在收到 OrderEvent 后通过事件处理
	// 由于复用了策略的account模块，所以这里不需要验证资金/仓位
	// if exSymbol.Symbol.Type == ctypes.MarketTypeFuture {
	// 	if intent.IsBuy {
	// 		// FUTURE：验证 quote 作为保证金/费用缓冲
	// 		lev := g.perpLeverage(ctx, accountID, exSymbol, intent.Side)
	// 		notional := px.Mul(qty)
	// 		margin := notional.Div(lev)
	// 		feeBuf := notional.Mul(decimal.NewFromFloat(g.meConfig.TakerRate()))
	// 		reserve := margin.Add(feeBuf)

	// 		available, err := g.accountManager.GetAvailableBalance(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Quote)
	// 		if err != nil || available.LessThan(reserve) {
	// 			return "", fmt.Errorf("insufficient collateral balance: %w", err)
	// 		}
	// 	} else {
	// 		// 卖出：验证仓位数量
	// 		pos, err := g.accountManager.GetPosition(ctx, accountID, exSymbol.Symbol, intent.Side)
	// 		if err != nil {
	// 			return "", fmt.Errorf("failed to get position: %w", err)
	// 		}
	// 		if pos.Qty.LessThan(qty) {
	// 			return "", fmt.Errorf("insufficient position quantity: %w", err)
	// 		}
	// 	}
	// } else if intent.IsBuy {
	// 	var reserve decimal.Decimal
	// 	if intent.OrderType == ctypes.OrderTypeMarket {
	// 		// 市价单：需要在现价基础上加上 buffer (价格是浮动的，需要考虑滑点)
	// 		reserve = px.Mul(qty).Mul(decimal.NewFromFloat(g.meConfig.MarketOrderFreezeFactor))
	// 	} else {
	// 		// 限价单：只需按订单价值冻结资金
	// 		reserve = px.Mul(qty)
	// 	}
	// 	available, err := g.accountManager.GetAvailableBalance(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Quote)
	// 	if err != nil || available.LessThan(reserve) {
	// 		return "", fmt.Errorf("insufficient quote balance: %w", err)
	// 	}
	// } else {
	// 	// 卖出时因为成交数量是确定的，所以只需要验证 base 余额
	// 	available, err := g.accountManager.GetAvailableBalance(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Base)
	// 	_available := available.String()
	// 	_qty := qty.String()
	// 	_ = _available
	// 	_ = _qty
	// 	if err != nil || available.LessThan(qty) {
	// 		return "", fmt.Errorf("insufficient base balance: %w", err)
	// 	}
	// }

	exOrderID, err := g.me.PlaceOrder(ctx, &intent)
	if err != nil {
		return "", err
	}
	// 记录映射（即使 exOrderID 为空也不会影响 cancel；但正常情况下应为非空）
	if exOrderID != "" {
		g.mu.Lock()
		g.clientToExchange[clientOrderID] = exOrderID
		g.exchangeToClient[exOrderID] = clientOrderID
		g.mu.Unlock()
	}
	return exOrderID, nil
}

func (g *BacktestGateway) CancelOrder(ctx context.Context, intent stypes.OrderCancelIntent) error {
	if g == nil || g.me == nil {
		return fmt.Errorf("exchange gateway is nil")
	}
	if intent.GetExchange() == nil || intent.GetSymbol() == nil {
		return fmt.Errorf("exchange/symbol is required")
	}
	clientOrderID := intent.ClientOrderID
	if clientOrderID == "" {
		return nil
	}

	g.mu.RLock()
	exOrderID := g.clientToExchange[clientOrderID]
	g.mu.RUnlock()
	if exOrderID == "" {
		// 映射不存在：视为幂等撤单
		return nil
	}

	// MatchingEngine Cancel 需要 exchangeOrderID
	cancel := intent
	cancel.ClientOrderID = exOrderID
	return g.me.CancelOrder(ctx, &cancel)
}

func (g *BacktestGateway) GetLeverage(ctx context.Context, accountID string, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error) {
	if g == nil || g.accountEngine == nil {
		return 1, fmt.Errorf("exchange gateway is nil")
	}
	return g.accountEngine.GetLeverage(ctx, accountID, symbol)
}

func (g *BacktestGateway) SetLeverage(ctx context.Context, accountID string, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error {
	if g == nil || g.accountEngine == nil {
		return fmt.Errorf("exchange gateway is nil")
	}
	if leverage <= 0 {
		return fmt.Errorf("invalid leverage: %d", leverage)
	}
	err := g.accountEngine.SetLeverage(ctx, accountID, symbol, leverage)
	if err != nil {
		return err
	}

	// 发布杠杆变更信号（下游模块订阅处理）
	ex := exchange
	sym := symbol
	aid := accountID
	return g.bus.Publish(ctx, &stypes.LeverageChangedSignal{
		BaseSignal: stypes.BaseSignal{Exchange: &ex, Symbol: &sym, AccountID: &aid, Ts: g.now()},
		Leverage:   leverage,
	})
}

func (g *BacktestGateway) now() time.Time {
	if g == nil || g.clock == nil {
		return time.Now()
	}
	return g.clock.Now()
}

func (g *BacktestGateway) OnMarketSignal(ctx context.Context, sig stypes.Signal) error {
	switch ks := sig.(type) {
	case *stypes.KlineSignal:
		if ks == nil || ks.GetExchange() == nil || ks.GetSymbol() == nil {
			return nil
		}
		phase := bridge.MarketPhaseOpen
		if ks.IsClosed {
			phase = bridge.MarketPhaseClose
		}
		ev := bridge.MarketEvent{
			Ts:       ks.GetTimestamp(),
			Exchange: *ks.GetExchange(),
			Symbol:   *ks.GetSymbol(),
			Phase:    phase,
			Open:     ks.Open,
			High:     ks.Open,
			Low:      ks.Open,
			Close:    ks.Open,
			Volume:   decimal.Zero, // open 点不限制 volume
		}
		return g.me.OnMarketEvent(ctx, ev)
	default:
		// 当前撮合引擎仅使用 bar（bar_open/bar_close）；其他 market signal 先忽略
		return nil
	}
}

func (g *BacktestGateway) handleExchangeEvent(ctx context.Context, ev bridge.ExchangeEvent) error {
	switch e := ev.(type) {
	case bridge.OrderEvent:
		return g.publishOrderEvent(ctx, e)
	case bridge.FillEvent:
		return g.publishFillEvent(ctx, e)
	default:
		return nil
	}
}

func (g *BacktestGateway) publishOrderEvent(ctx context.Context, e bridge.OrderEvent) error {
	accountID := e.AccountID
	ex := e.Exchange
	sym := e.Symbol
	ts := e.Ts

	// 策略内部约定：OrderID 统一使用 clientOrderID
	clientOrderID := e.ClientOrderID
	if clientOrderID == "" && e.ExchangeOrderID != "" {
		g.mu.RLock()
		clientOrderID = g.exchangeToClient[e.ExchangeOrderID]
		g.mu.RUnlock()
	}

	// 生命周期结束时清理映射
	cleanup := func() {
		if clientOrderID == "" {
			return
		}
		g.mu.Lock()
		exOrderID := g.clientToExchange[clientOrderID]
		delete(g.clientToExchange, clientOrderID)
		if exOrderID != "" {
			delete(g.exchangeToClient, exOrderID)
		}
		g.mu.Unlock()
	}

	switch e.Kind {
	case bridge.ExchangeEventKindOrderAccepted:
		return g.bus.Publish(ctx, &stypes.OrderLifecycleSignal{
			BaseSignal: stypes.BaseSignal{Exchange: &ex, Symbol: &sym, AccountID: &accountID, Ts: ts},
			OrderID:    clientOrderID,
			Status:     ctypes.OrderStatusNew,
		})
	case bridge.ExchangeEventKindOrderRejected:
		defer cleanup()
		return g.bus.Publish(ctx, &stypes.OrderLifecycleSignal{
			BaseSignal: stypes.BaseSignal{Exchange: &ex, Symbol: &sym, AccountID: &accountID, Ts: ts},
			OrderID:    clientOrderID,
			Status:     ctypes.OrderStatusRejected,
			Reason:     e.Reason,
			Code:       e.Code,
		})
	case bridge.ExchangeEventKindOrderCanceled:
		defer cleanup()
		return g.bus.Publish(ctx, &stypes.OrderLifecycleSignal{
			BaseSignal: stypes.BaseSignal{Exchange: &ex, Symbol: &sym, AccountID: &accountID, Ts: ts},
			OrderID:    clientOrderID,
			Status:     ctypes.OrderStatusCanceled,
			Reason:     e.Reason,
		})
	case bridge.ExchangeEventKindOrderExpired:
		defer cleanup()
		return g.bus.Publish(ctx, &stypes.OrderLifecycleSignal{
			BaseSignal: stypes.BaseSignal{Exchange: &ex, Symbol: &sym, AccountID: &accountID, Ts: ts},
			OrderID:    clientOrderID,
			Status:     ctypes.OrderStatusExpired,
			Reason:     e.Reason,
		})
	case bridge.ExchangeEventKindOrderDone:
		defer cleanup()
		return g.bus.Publish(ctx, &stypes.OrderLifecycleSignal{
			BaseSignal: stypes.BaseSignal{Exchange: &ex, Symbol: &sym, AccountID: &accountID, Ts: ts},
			OrderID:    clientOrderID,
			Status:     ctypes.OrderStatusDone,
		})
	default:
		return nil
	}
}

func (g *BacktestGateway) publishFillEvent(ctx context.Context, e bridge.FillEvent) error {
	accountID := e.AccountID
	ex := e.Exchange
	sym := e.Symbol
	ts := e.Ts

	clientOrderID := e.ClientOrderID
	if clientOrderID == "" && e.ExchangeOrderID != "" {
		g.mu.RLock()
		clientOrderID = g.exchangeToClient[e.ExchangeOrderID]
		g.mu.RUnlock()
	}

	exSymbol := ctypes.NewExSymbol(ex, sym)

	// 计算 BaseCurrency 计价的已实现盈亏和手续费
	realizedPnlBase, _, err := g.calculatePnLInBaseCurrency(ctx, exSymbol, accountID, e)
	if err != nil {
		log.Warn().Err(err).Msg("failed to calculate PnL in base currency")
		// 降级：继续处理，但 PnL 字段为 0
	}

	// 发布资金变更事件（delta 语义）
	err = g.publishBalanceDeltas(ctx, e)
	if err != nil {
		return err
	}

	err = g.bus.Publish(ctx, &stypes.FillSignal{
		BaseSignal:  stypes.BaseSignal{Exchange: &ex, Symbol: &sym, AccountID: &accountID, Ts: ts},
		OrderID:     clientOrderID,
		Side:        e.Side,
		IsBuy:       e.IsBuy,
		Qty:         e.Qty,
		Price:       e.Price,
		Fee:         e.Fee,
		Asset:       e.Asset,
		RealizedPnl: realizedPnlBase,
	})
	if err != nil {
		return err
	}

	// 由交易所网关输出仓位快照（PositionSignal），供账户/组合/风险等消费
	if sym.Type == ctypes.MarketTypeFuture {
		deltaQty := e.Qty
		if !e.IsBuy {
			deltaQty = deltaQty.Neg()
		}

		abs := func(d decimal.Decimal) decimal.Decimal {
			if d.IsNegative() {
				return d.Neg()
			}
			return d
		}

		// Backtest 模式下，PositionSignal 采用“快照语义”，因此网关需要在本地根据 FillEvent 维护净仓位快照。
		exSymbol := ctypes.NewExSymbol(ex, sym)
		posKey := fmt.Sprintf("%s|%s", accountID, exSymbol.Key().String())

		g.mu.Lock()
		old := g.positions[posKey]
		oldQty := old.qty
		oldEntry := old.entry

		newQty := oldQty.Add(deltaQty)
		newEntry := decimal.Zero
		switch {
		case newQty.IsZero():
			newEntry = decimal.Zero
		case oldQty.IsZero() || oldQty.Sign()*newQty.Sign() < 0:
			// 从 0 开仓 / 反手：均价以最新成交价为准
			newEntry = e.Price
		default:
			// 同方向：只有“加仓”会重算均价；减仓保持均价不变
			sameDirIncrease := (oldQty.IsPositive() && deltaQty.IsPositive()) || (oldQty.IsNegative() && deltaQty.IsNegative())
			if sameDirIncrease {
				den := abs(oldQty).Add(abs(deltaQty))
				if den.IsZero() {
					newEntry = decimal.Zero
				} else {
					num := oldEntry.Mul(abs(oldQty)).Add(e.Price.Mul(abs(deltaQty)))
					newEntry = num.Div(den)
				}
			} else {
				newEntry = oldEntry
			}
		}

		if newQty.IsZero() {
			delete(g.positions, posKey)
		} else {
			g.positions[posKey] = gatewayPositionState{qty: newQty, entry: newEntry}
		}
		g.mu.Unlock()

		if err := g.bus.Publish(ctx, &stypes.PositionSignal{
			BaseSignal: stypes.BaseSignal{
				Exchange:  &ex,
				Symbol:    &sym,
				AccountID: &accountID,
				Ts:        ts,
			},
			Side:       e.Side,
			Qty:        abs(newQty),
			EntryPrice: newEntry,
		}); err != nil {
			return err
		}
	}

	return nil
}

// publishBalanceDeltas 根据成交事件计算并发布资金变更增量
// 按照"毛收入/毛到手 + 独立 fee delta"的原则
func (g *BacktestGateway) publishBalanceDeltas(ctx context.Context, e bridge.FillEvent) error {
	accountID := e.AccountID
	ex := e.Exchange
	sym := e.Symbol
	ts := e.Ts

	baseAsset := sym.Base
	quoteAsset := sym.Quote
	feeAsset := e.Asset

	switch sym.Type {
	case ctypes.MarketTypeSpot:
		if e.IsBuy {
			// 现货买入：
			// 1. quote 从冻结余额中扣除支付的 notional（下单时已冻结）
			// 2. base 增加买到的数量（毛数量，不扣手续费）
			// 3. feeAsset 单独扣除手续费
			notional := e.Qty.Mul(e.Price)

			// 发布 quote 资产减少（从 Frozen 扣除，因为下单时已冻结）
			if err := g.publishBalanceDelta(ctx, accountID, ex, sym, ts, quoteAsset, decimal.Zero, notional.Neg()); err != nil {
				return err
			}
			// 发布 base 资产增加（毛数量）
			if err := g.publishBalanceDelta(ctx, accountID, ex, sym, ts, baseAsset, e.Qty, decimal.Zero); err != nil {
				return err
			}
			// 发布手续费扣除（独立）
			if !e.Fee.IsZero() {
				if err := g.publishBalanceDelta(ctx, accountID, ex, sym, ts, feeAsset, e.Fee.Neg(), decimal.Zero); err != nil {
					return err
				}
			}
		} else {
			// 现货卖出：
			// 1. base 从冻结余额中扣除（下单时已冻结）
			// 2. quote 增加收入的 notional（毛收入）
			// 3. feeAsset 单独扣除手续费
			notional := e.Qty.Mul(e.Price)

			// 发布 base 资产减少（从 Frozen 扣除，因为下单时已冻结）
			if err := g.publishBalanceDelta(ctx, accountID, ex, sym, ts, baseAsset, decimal.Zero, e.Qty.Neg()); err != nil {
				return err
			}
			// 发布 quote 资产增加（毛收入）
			if err := g.publishBalanceDelta(ctx, accountID, ex, sym, ts, quoteAsset, notional, decimal.Zero); err != nil {
				return err
			}
			// 发布手续费扣除（独立）
			if !e.Fee.IsZero() {
				if err := g.publishBalanceDelta(ctx, accountID, ex, sym, ts, feeAsset, e.Fee.Neg(), decimal.Zero); err != nil {
					return err
				}
			}
		}
	case ctypes.MarketTypeFuture:
		// 合约场景（净仓模型）：收益只在“减仓/平仓/反手的平仓部分”产生。
		// 注意：不依赖 FillEvent.Side 来判断开/平仓（在一方向模式下 side 可能不可靠），
		// 而是用“当前净仓方向 + 本次成交方向”来判断是否发生平仓。
		profit, err := g.futureRealizedPnlQuote(ctx, accountID, sym, e.IsBuy, e.Qty, e.Price)
		if err != nil {
			return err
		}

		// 发布平仓收益（profit 可能为负）
		if !profit.IsZero() {
			if err := g.publishBalanceDelta(ctx, accountID, ex, sym, ts, quoteAsset, profit, decimal.Zero); err != nil {
				return err
			}
		}

		// 发布手续费扣除（独立，quote 资产）
		if !e.Fee.IsZero() {
			if err := g.publishBalanceDelta(ctx, accountID, ex, sym, ts, quoteAsset, e.Fee.Neg(), decimal.Zero); err != nil {
				return err
			}
		}
	}

	return nil
}

// publishBalanceDelta 发布单个资产的资金变更增量
func (g *BacktestGateway) publishBalanceDelta(ctx context.Context, accountID string, exchange ctypes.Exchange, symbol ctypes.Symbol, ts time.Time, asset string, freeDelta, frozenDelta decimal.Decimal) error {
	// 如果增量为零，跳过发布
	if freeDelta.IsZero() && frozenDelta.IsZero() {
		return nil
	}

	return g.bus.Publish(ctx, &stypes.BalanceDeltaSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange:  &exchange,
			Symbol:    &symbol,
			AccountID: &accountID,
			Ts:        ts,
		},
		WalletType: ctypes.WalletTypeTrade,
		Asset:      asset,
		Free:       freeDelta,
		Frozen:     frozenDelta,
	})
}

// calculatePnLInBaseCurrency 计算以 BaseCurrency 计价的已实现盈亏和手续费
func (g *BacktestGateway) calculatePnLInBaseCurrency(ctx context.Context, exSymbol ctypes.ExSymbol, accountID string, e bridge.FillEvent) (realizedPnlBase, feeInBase decimal.Decimal, err error) {
	realizedPnlBase = decimal.Zero
	feeInBase = decimal.Zero

	// 1. 计算已实现盈亏（以原生 quote 计价）
	realizedPnlQuote := decimal.Zero

	switch exSymbol.GetType() {
	case ctypes.MarketTypeSpot:
		// 现货：从 accountManager 获取 WAC 成本计算的已实现盈亏
		// realizedPnlQuote = g.accountEngine.GetSpotRealizedPnL(ctx, accountID, exSymbol, e.IsBuy, e.Qty, e.Price)
	case ctypes.MarketTypeFuture:
		// 合约（净仓模型）：已实现盈亏只在“减仓/平仓/反手的平仓部分”产生。
		pnl, err := g.futureRealizedPnlQuote(ctx, accountID, exSymbol.Symbol, e.IsBuy, e.Qty, e.Price)
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		realizedPnlQuote = pnl
	}

	// 2. 将 realizedPnlQuote 换算到 BaseCurrency
	quoteAsset := exSymbol.GetQuote()
	quotePrice, err := g.marketProvider.GetPriceInBaseCurrency(ctx, quoteAsset, g.baseCurrency)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	realizedPnlBase = realizedPnlQuote.Mul(quotePrice)

	// 3. 将手续费换算到 BaseCurrency
	feeAsset := e.Asset
	feePrice, err := g.marketProvider.GetPriceInBaseCurrency(ctx, feeAsset, g.baseCurrency)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	feeInBase = e.Fee.Mul(feePrice)

	return realizedPnlBase, feeInBase, nil
}

// futureRealizedPnlQuote 计算合约成交带来的已实现盈亏（quote 计价）。
// 约定：
// - qty 为绝对数量（FillEvent.Qty），isBuy 表示买入方向（买为 +，卖为 -）
// - 以“当前净仓 + 本次成交方向”判断是否发生平仓；不依赖 side（兼容一方向模式 side 不可靠的情况）
// - 若发生反手，仅平掉的那部分计入 realized
func (g *BacktestGateway) futureRealizedPnlQuote(ctx context.Context, accountID string, sym ctypes.Symbol, isBuy bool, qty, price decimal.Decimal) (decimal.Decimal, error) {
	if g == nil || g.accountEngine == nil {
		return decimal.Zero, nil
	}
	if qty.IsZero() || price.IsZero() {
		return decimal.Zero, nil
	}
	if sym.Type != ctypes.MarketTypeFuture {
		return decimal.Zero, nil
	}

	// 获取当前净仓（带符号）与入场均价（quote）
	curQty, entry, err := g.futureNetPosition(ctx, accountID, sym)
	if err != nil {
		return decimal.Zero, err
	}
	if curQty.IsZero() || entry.IsZero() {
		return decimal.Zero, nil
	}

	// 本次成交对净仓的 signed 变化：买入为 +qty，卖出为 -qty
	delta := qty
	if !isBuy {
		delta = delta.Neg()
	}

	// 同方向增仓：不产生已实现盈亏
	if curQty.Sign() == delta.Sign() {
		return decimal.Zero, nil
	}

	// 反方向：减仓/平仓/反手，只有平掉的那部分计入 realized
	closeAbs := decimal.Min(curQty.Abs(), delta.Abs())
	if closeAbs.IsZero() {
		return decimal.Zero, nil
	}

	// 多仓： (exit-entry)*closeQty
	if curQty.IsPositive() {
		return price.Sub(entry).Mul(closeAbs), nil
	}
	// 空仓： (entry-exit)*closeQty
	return entry.Sub(price).Mul(closeAbs), nil
}

// futureNetPosition 返回合约净仓（带符号）及其均价（quote 计价）。
// 说明：Account 内部用 PositionSideLong/Short 存储净仓方向；这里同时查询两边，避免依赖外部 Side 推断。
func (g *BacktestGateway) futureNetPosition(ctx context.Context, accountID string, sym ctypes.Symbol) (qty, entry decimal.Decimal, err error) {
	posL, err := g.accountEngine.GetPosition(ctx, accountID, sym, ctypes.PositionSideLong)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	if posL != nil && posL.Amount.GreaterThan(decimal.Zero) {
		return posL.Amount, posL.EntryPrice, nil
	}

	posS, err := g.accountEngine.GetPosition(ctx, accountID, sym, ctypes.PositionSideShort)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	if posS != nil && posS.Amount.LessThan(decimal.Zero) {
		return posS.Amount, posS.EntryPrice, nil
	}

	return decimal.Zero, decimal.Zero, nil
}
