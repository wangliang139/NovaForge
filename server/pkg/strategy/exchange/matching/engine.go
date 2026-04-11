package matching

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	bridge "github.com/wangliang139/NovaForge/server/pkg/strategy/exchange/bridge"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/marketdata"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/misc"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// MatchingConfig 撮合/成交相关配置（回测）
type MatchingConfig struct {
	Symbols      []stypes.BacktestSymbol
	BaseCurrency string
	BaseExchange ctypes.Exchange

	// Maker/Taker 手续费率（若为 0，则回退到 CommissionRate）
	MakerCommissionRate float64
	TakerCommissionRate float64

	// CommissionRate 兼容历史：当 Maker/Taker 为 0 时使用
	CommissionRate float64

	SlippageRate float64 // 市价单成交价格滑点率

	MarketOrderFreezeFactor float64 // 市价单成交价格冻结因子

	// ExchangeDelay 模拟交易所延迟：撮合引擎产出的 internal event 时间戳会在“触发该事件的基准时间”上整体加上该延迟。
	// - 对 market signal 驱动的撮合：基准时间为 signal.Ts
	// - 对下单/撤单意图：基准时间为 intent.SubmittedAt（缺省则使用当前 curTime）
	//
	// 用途：
	// - 保证撮合引擎生成的 internal event 在时间线上晚于对应 market signal，避免同一时间点的排序歧义。
	//
	// 兼容：
	// - 若 ExchangeDelay=0 且 FillDelay>0，则使用 FillDelay 作为延迟值（历史字段）。
	ExchangeDelay time.Duration

	// FillDelay 兼容历史：早期仅用于成交延迟。建议改用 ExchangeDelay。
	FillDelay time.Duration
}

func DefaultMatchingConfig() MatchingConfig {
	return MatchingConfig{
		MakerCommissionRate:     0.0005,
		TakerCommissionRate:     0.001,
		CommissionRate:          0.001,
		SlippageRate:            0,
		MarketOrderFreezeFactor: 1.2,
		ExchangeDelay:           10 * time.Millisecond,
		FillDelay:               10 * time.Millisecond,
	}
}

func (c *MatchingConfig) MakerRate() float64 {
	if c == nil {
		return 0
	}
	if c.MakerCommissionRate != 0 {
		return c.MakerCommissionRate
	}
	return c.CommissionRate
}

func (c *MatchingConfig) TakerRate() float64 {
	if c == nil {
		return 0
	}
	if c.TakerCommissionRate != 0 {
		return c.TakerCommissionRate
	}
	return c.CommissionRate
}

// MatchingEngine 撮合引擎：异步消费意图单，产出事件到 EventStore。
type MatchingEngine struct {
	cfg MatchingConfig

	mu sync.RWMutex

	open       map[ctypes.ExSymbolKey]*orderBook // exchange|symbol -> book (只保存挂单最小信息)
	orderCache map[ctypes.OrderId]*ctypes.Order  // 订单状态缓存（只读，从 OrderEvent 中更新，用于撮合时查询订单状态）
	// exchangeOrderID -> accountId（用于 Fill/OrderEvent 透传 AccountID，避免共享 bus 多账户串账）
	orderAccountIDs map[ctypes.OrderId]string

	nextFunding map[ctypes.ExSymbolKey]time.Time // exchange|symbol -> next funding timestamp

	// 外部依赖（通过接口解耦）
	clock          clock.Clock               // 时钟
	accountEngine  strategy.AccountEngine    // 账户管理器（只读查询）
	marketProvider marketdata.MarketProvider // 市场数据提供器
	outBus         bridge.Bus                // engine -> gateway 的内部同步 bus
}

func NewMatchingEngine(cfg MatchingConfig, clk clock.Clock, outBus bridge.Bus, accountEngine strategy.AccountEngine, marketProvider marketdata.MarketProvider) (*MatchingEngine, error) {
	if clk == nil {
		return nil, fmt.Errorf("clock is required")
	}
	if accountEngine == nil {
		return nil, fmt.Errorf("account provider is required")
	}
	if marketProvider == nil {
		return nil, fmt.Errorf("market data provider is required")
	}
	if outBus == nil {
		return nil, fmt.Errorf("exchange out bus is required")
	}

	m := &MatchingEngine{
		cfg:             cfg,
		clock:           clk,
		open:            make(map[ctypes.ExSymbolKey]*orderBook),
		orderCache:      make(map[ctypes.OrderId]*ctypes.Order),
		orderAccountIDs: make(map[ctypes.OrderId]string),
		nextFunding:     make(map[ctypes.ExSymbolKey]time.Time),
		accountEngine:   accountEngine,
		marketProvider:  marketProvider,
		outBus:          outBus,
	}

	// 从回测配置参数中读取模拟交易所延迟（可选）：
	// - exchange_latency: duration string（例如 "50ms" / "1s"）
	// - exchange_latency_ms: number|string（毫秒）
	// - exchange_delay_ms: number|string（毫秒，别名）
	// if d, ok := parseExchangeDelay(btCfg.Params); ok {
	// 	m.cfg.ExchangeDelay = d
	// }

	return m, nil
}

func (m *MatchingEngine) exchangeDelay() time.Duration {
	if m == nil {
		return 0
	}
	if m.cfg.ExchangeDelay != 0 {
		return m.cfg.ExchangeDelay
	}
	// backward compatible
	return m.cfg.FillDelay
}

func (m *MatchingEngine) delayedTs(ts time.Time) time.Time {
	d := m.exchangeDelay()
	if d <= 0 {
		return ts
	}
	return ts.Add(d)
}

// OnMarketEvent 处理 gateway 转发的市场事件（bar）。
func (m *MatchingEngine) OnMarketEvent(ctx context.Context, ev bridge.MarketEvent) error {
	exSymbol := ctypes.NewExSymbol(ev.Exchange, ev.Symbol)
	bar := &marketBar{
		Open:   ev.Open,
		High:   ev.High,
		Low:    ev.Low,
		Close:  ev.Close,
		Volume: ev.Volume,
	}

	// 撮合逻辑在锁内，事件输出在锁外（避免阻塞）
	m.mu.Lock()
	book := m.ensureBookLocked(exSymbol)
	if book == nil {
		m.mu.Unlock()
		return nil
	}
	var err error
	switch ev.Phase {
	case bridge.MarketPhaseOpen:
		// open：只撮市价单（语义：next open 成交）
		err = m.matchMarketOrdersLocked(ctx, exSymbol, book, bar, ev.Ts)
	case bridge.MarketPhaseClose:
		// close：只撮限价单（语义：bar 内触达/成交）
		err = m.matchLimitOrdersLocked(ctx, exSymbol, book, bar, ev.Ts)
	default:
		// 兼容旧行为：无 phase 时两者都撮
		err = m.matchMarketOrdersLocked(ctx, exSymbol, book, bar, ev.Ts)
		if err == nil {
			err = m.matchLimitOrdersLocked(ctx, exSymbol, book, bar, ev.Ts)
		}
	}
	m.mu.Unlock()

	return err
}

// PlaceOrder 同步下单：校验通过则立即入簿，并返回 exchangeOrderId。
// 注意：
// - MatchingEngine 内部仅使用 exchangeOrderId；clientOrderId 仅透传到事件。
// - 该方法会发布 OrderAccepted/OrderRejected 等事件（与异步意图路径一致）。
func (m *MatchingEngine) PlaceOrder(ctx context.Context, intent *stypes.OrderPlaceIntent) (ctypes.OrderId, error) {
	if intent == nil {
		return "", fmt.Errorf("nil intent")
	}
	return m.handlePlace(intent)
}

// CancelOrderSync 同步撤单：按 exchangeOrderId 撤单，并发布 OrderCanceled 事件。
func (m *MatchingEngine) CancelOrder(ctx context.Context, intent *stypes.OrderCancelIntent) error {
	if intent == nil {
		return fmt.Errorf("nil intent")
	}
	return m.handleCancel(intent)
}

func (m *MatchingEngine) handlePlace(signal stypes.OrderIntent) (ctypes.OrderId, error) {
	if signal == nil {
		return "", fmt.Errorf("nil intent")
	}
	intent, ok := signal.(*stypes.OrderPlaceIntent)
	if !ok {
		return "", fmt.Errorf("invalid intent: %T", intent)
	}
	if intent.GetAccountID() == nil {
		return "", fmt.Errorf("account id is required")
	}

	ctx := context.Background()

	m.mu.Lock()

	now := intent.GetTimestamp()
	if now.IsZero() {
		now = m.clock.Now()
	}
	evTs := m.delayedTs(now)

	exSymbol := ctypes.NewExSymbol(*intent.GetExchange(), *intent.GetSymbol())

	accountID := lo.FromPtr(intent.GetAccountID())
	clientOrderID := ctypes.OrderId(intent.ClientOrderID)

	tif := ctypes.TimeInForceGTC
	if intent.TimeInForce != nil {
		tif = *intent.TimeInForce
	}

	order := &ctypes.Order{
		Exchange:      exSymbol.Exchange,
		Symbol:        exSymbol.Symbol,
		ClientOrderID: clientOrderID,
		Side:          intent.Side,
		IsBuy:         intent.IsBuy,
		OrderType:     intent.OrderType,
		Status:        ctypes.OrderStatusNew,
		TimeInForce:   tif,
		CreatedTs:     evTs,
		UpdatedTs:     evTs,
	}

	book := m.ensureBookLocked(exSymbol)

	// 价格：限价单必须提供；市价单用于冻结/校验时使用 lastPx 做估算
	var px decimal.Decimal
	if intent.OrderType == ctypes.OrderTypeLimit {
		px = *intent.Price
		order.Price = px
	} else {
		// market：需要一个可用价格估算 qty/冻结资金
		last, err := m.marketProvider.GetLastPrice(ctx, exSymbol.Exchange, exSymbol.Symbol)
		if err != nil || last.LessThanOrEqual(decimal.Zero) {
			return "", fmt.Errorf("market price not available")
		}
		px = last
	}

	order.OriginalQty = *intent.Quantity
	order.OrderID = ctypes.OrderId(uuid.New().String())
	// MatchingEngine 作为“交易所/撮合所”，内部只关心 exchangeOrderID；clientOrderID 仅作为事件透传字段。
	m.orderAccountIDs[order.OrderID] = accountID

	// 入簿（使用简化的 orderEntry）
	entry := &orderEntry{
		OrderID:     order.OrderID,
		Price:       px,
		RemainQty:   *intent.Quantity,
		IsBuy:       order.IsBuy,
		Ts:          order.CreatedTs,
		TimeInForce: order.TimeInForce,
		Status:      order.Status, // 同步订单状态
	}
	if order.OrderType == ctypes.OrderTypeMarket {
		m.insertMarketOrderLocked(book, entry)
	} else {
		m.insertLimitOrderLocked(book, entry)
	}

	// log.Info().Interface("order", order).Msg("inserted order entry")

	// 产出订单事件（NEW）
	// 写入缓存（撮合时会读取并原地更新）
	m.orderCache[order.OrderID] = order
	m.emitOrderAcceptedEvent(ctx, exSymbol, accountID, order.OrderID, order.ClientOrderID, evTs)

	m.mu.Unlock()
	return order.OrderID, nil
}

func (m *MatchingEngine) handleCancel(signal stypes.OrderIntent) error {
	if signal == nil {
		return fmt.Errorf("nil intent")
	}
	intent, ok := signal.(*stypes.OrderCancelIntent)
	if !ok {
		return fmt.Errorf("invalid intent: %T", signal)
	}

	ctx := context.Background()

	m.mu.Lock()

	exSymbol := ctypes.NewExSymbol(*intent.GetExchange(), *intent.GetSymbol())

	// 从订单缓存中获取订单（已持有写锁，直接访问）
	order := m.orderCache[intent.ClientOrderID]
	if order == nil {
		return nil
	}
	if order.Status != ctypes.OrderStatusNew && order.Status != ctypes.OrderStatusPartialDone {
		return nil
	}

	now := intent.Ts
	if now.IsZero() {
		now = m.clock.Now()
	}
	evTs := m.delayedTs(now)

	book := m.ensureBookLocked(exSymbol)
	if book != nil {
		book.marketBuys = removeOrderEntryFromSlice(book.marketBuys, order.OrderID)
		book.marketSells = removeOrderEntryFromSlice(book.marketSells, order.OrderID)
		book.limitBuys = removeOrderEntryFromSlice(book.limitBuys, order.OrderID)
		book.limitSells = removeOrderEntryFromSlice(book.limitSells, order.OrderID)
	}

	// cancelOpenOrderLocked 会更新计数器并输出 OrderEvent
	_ = m.cancelOpenOrderLocked(ctx, exSymbol, order, ctypes.OrderStatusCanceled, evTs, "")
	// 预留资金释放由 OrderManager 在收到 OrderEvent 后处理

	m.mu.Unlock()
	return nil
}

func (m *MatchingEngine) ensureBookLocked(exSymbol ctypes.ExSymbol) *orderBook {
	key := exSymbol.Key()
	b := m.open[key]
	if b == nil {
		b = &orderBook{}
		m.open[key] = b
	}
	return b
}

// updateOrderEntryStatus 更新订单簿中 orderEntry 的状态
func (m *MatchingEngine) updateOrderEntryStatusLocked(exSymbol ctypes.ExSymbol, orderID ctypes.OrderId, status ctypes.OrderStatus) {
	book := m.ensureBookLocked(exSymbol)
	if book == nil {
		return
	}

	// 更新市价买单
	for _, entry := range book.marketBuys {
		if entry != nil && entry.OrderID == orderID {
			entry.Status = status
			return
		}
	}

	// 更新市价卖单
	for _, entry := range book.marketSells {
		if entry != nil && entry.OrderID == orderID {
			entry.Status = status
			return
		}
	}

	// 更新限价买单
	for _, entry := range book.limitBuys {
		if entry != nil && entry.OrderID == orderID {
			entry.Status = status
			return
		}
	}

	// 更新限价卖单
	for _, entry := range book.limitSells {
		if entry != nil && entry.OrderID == orderID {
			entry.Status = status
			return
		}
	}
}

// updateOrderCount 更新挂单计数器（当订单状态变化时调用）
func (m *MatchingEngine) updateOrderCount(book *orderBook, oldStatus, newStatus ctypes.OrderStatus) {
	if book == nil {
		return
	}
	wasOpen := oldStatus == ctypes.OrderStatusNew || oldStatus == ctypes.OrderStatusPartialDone
	isOpen := newStatus == ctypes.OrderStatusNew || newStatus == ctypes.OrderStatusPartialDone

	if wasOpen && !isOpen {
		// 从挂单状态变为非挂单状态，减少计数
		if book.openOrderCount > 0 {
			book.openOrderCount--
		}
	} else if !wasOpen && isOpen {
		// 从非挂单状态变为挂单状态，增加计数
		book.openOrderCount++
	}
}

func (m *MatchingEngine) insertMarketOrderLocked(book *orderBook, entry *orderEntry) {
	if book == nil || entry == nil {
		return
	}
	if entry.IsBuy {
		book.marketBuys = append(book.marketBuys, entry)
	} else {
		book.marketSells = append(book.marketSells, entry)
	}

	// 更新挂单计数器
	book.openOrderCount++
}

func (m *MatchingEngine) insertLimitOrderLocked(book *orderBook, entry *orderEntry) {
	if book == nil || entry == nil {
		return
	}

	var list *[]*orderEntry
	var less func(*orderEntry, *orderEntry) bool

	if entry.IsBuy {
		// buy: price desc, ts asc
		list = &book.limitBuys
		less = func(a, b *orderEntry) bool {
			if a == nil || b == nil {
				return a != nil
			}
			return a.Price.GreaterThan(b.Price) || (a.Price.Equal(b.Price) && a.Ts.Before(b.Ts))
		}
	} else {
		// sell: price asc, ts asc
		list = &book.limitSells
		less = func(a, b *orderEntry) bool {
			if a == nil || b == nil {
				return a != nil
			}
			return a.Price.LessThan(b.Price) || (a.Price.Equal(b.Price) && a.Ts.Before(b.Ts))
		}
	}

	// 使用二分查找插入位置
	i := sort.Search(len(*list), func(i int) bool {
		return less(entry, (*list)[i])
	})

	// 插入订单
	*list = append(*list, nil)
	copy((*list)[i+1:], (*list)[i:])
	(*list)[i] = entry

	// 更新挂单计数器
	book.openOrderCount++
}

func (m *MatchingEngine) matchMarketOrdersLocked(ctx context.Context, exSymbol ctypes.ExSymbol, book *orderBook, bar *marketBar, ts time.Time) error {
	if book == nil || bar == nil {
		return errors.New("book or bar is nil")
	}
	// volume 作为“本 bar 最大可成交 base 数量”；若为 0，视为无限（不限制）
	volRemaining := bar.Volume
	limitByVol := volRemaining.GreaterThan(decimal.Zero)

	// 先处理 buy market
	outBuys := book.marketBuys[:0]
	for _, entry := range book.marketBuys {
		if entry == nil {
			continue
		}
		// 直接使用 entry.Status，减少对 orderCache 的查询
		if entry.Status != ctypes.OrderStatusNew && entry.Status != ctypes.OrderStatusPartialDone {
			continue
		}
		if limitByVol && volRemaining.LessThanOrEqual(decimal.Zero) {
			outBuys = append(outBuys, entry)
			continue
		}
		// 只在需要完整订单信息时才查询 orderCache
		order := m.orderCache[entry.OrderID]
		if order == nil {
			continue
		}
		filled, done, err := m.fillMarketBuyLocked(ctx, exSymbol, order, entry, bar, &volRemaining, ts)
		if err != nil {
			return err
		}
		if filled && limitByVol {
			// volRemaining 在 fill 内推进
		}
		if !done {
			entry.RemainQty = order.OriginalQty.Sub(order.ExecutedQty)
			outBuys = append(outBuys, entry)
		}
		// 订单完成时，预留资金释放由 OrderManager 在收到 OrderEvent 后处理
	}
	book.marketBuys = outBuys

	// 再处理 sell market
	outSells := book.marketSells[:0]
	for _, entry := range book.marketSells {
		if entry == nil {
			continue
		}
		// 直接使用 entry.Status，减少对 orderCache 的查询
		if entry.Status != ctypes.OrderStatusNew && entry.Status != ctypes.OrderStatusPartialDone {
			continue
		}
		if limitByVol && volRemaining.LessThanOrEqual(decimal.Zero) {
			outSells = append(outSells, entry)
			continue
		}
		// 只在需要完整订单信息时才查询 orderCache
		order := m.orderCache[entry.OrderID]
		if order == nil {
			continue
		}
		filled, done, err := m.fillMarketSellLocked(ctx, exSymbol, order, entry, bar, &volRemaining, ts)
		if err != nil {
			return err
		}
		if filled && limitByVol {
			// volRemaining 在 fill 内推进
		}
		if !done {
			entry.RemainQty = order.OriginalQty.Sub(order.ExecutedQty)
			outSells = append(outSells, entry)
		}
		// 订单完成时，预留资金释放由 OrderManager 在收到 OrderEvent 后处理
	}
	book.marketSells = outSells
	return nil
}

func (m *MatchingEngine) matchLimitOrdersLocked(ctx context.Context, exSymbol ctypes.ExSymbol, book *orderBook, bar *marketBar, ts time.Time) error {
	if book == nil || bar == nil {
		return errors.New("book or bar is nil")
	}
	volRemaining := bar.Volume
	limitByVol := !volRemaining.IsZero()

	// buy limits
	outBuys := book.limitBuys[:0]
	for _, entry := range book.limitBuys {
		if entry == nil {
			continue
		}
		// 直接使用 entry.Status，减少对 orderCache 的查询
		if entry.Status != ctypes.OrderStatusNew && entry.Status != ctypes.OrderStatusPartialDone {
			continue
		}
		if limitByVol && volRemaining.LessThanOrEqual(decimal.Zero) {
			outBuys = append(outBuys, entry)
			continue
		}
		eligible := !entry.Price.IsZero() && entry.Price.GreaterThanOrEqual(bar.Low)
		if !eligible {
			outBuys = append(outBuys, entry)
			continue
		}
		// 只在需要完整订单信息时才查询 orderCache
		order := m.orderCache[entry.OrderID]
		if order == nil {
			continue
		}

		// FOK：本 bar 无法全额成交则不成交（保守：同时受 volume 与冻结资金约束）
		remainQty := order.OriginalQty.Sub(order.ExecutedQty)
		if remainQty.LessThanOrEqual(decimal.Zero) {
			continue
		}
		if entry.TimeInForce == ctypes.TimeInForceFOK {
			canFill := remainQty
			if limitByVol && volRemaining.LessThan(canFill) {
				_ = m.cancelOpenOrderLocked(ctx, exSymbol, order, ctypes.OrderStatusExpired, m.delayedTs(ts), "FOK")
				continue
			}
			// 冻结资金是否足够：由 fill 内部再次校验；这里做一次快速预检
			canAfford, err := m.canAffordBuyLocked(ctx, exSymbol, order, canFill, entry.Price, m.cfg.TakerRate())
			if err != nil {
				return err
			}
			if !canAfford {
				_ = m.cancelOpenOrderLocked(ctx, exSymbol, order, ctypes.OrderStatusExpired, m.delayedTs(ts), "FOK")
				continue
			}
		}

		filled, done, err := m.fillLimitBuyLocked(ctx, exSymbol, order, entry.Price, bar, &volRemaining, ts)
		if err != nil {
			return err
		}
		if filled && limitByVol {
			// volRemaining 在 fill 内推进
		}
		if !done {
			// IOC：本 bar 未成交部分立即取消
			if entry.TimeInForce == ctypes.TimeInForceIOC {
				_ = m.cancelOpenOrderLocked(ctx, exSymbol, order, ctypes.OrderStatusExpired, m.delayedTs(ts), "IOC")
				continue
			}
			entry.RemainQty = order.OriginalQty.Sub(order.ExecutedQty)
			outBuys = append(outBuys, entry)
		}
		// 订单完成时，预留资金释放由 OrderManager 在收到 OrderEvent 后处理
	}
	book.limitBuys = outBuys

	// sell limits
	outSells := book.limitSells[:0]
	for _, entry := range book.limitSells {
		if entry == nil {
			continue
		}
		// 直接使用 entry.Status，减少对 orderCache 的查询
		if entry.Status != ctypes.OrderStatusNew && entry.Status != ctypes.OrderStatusPartialDone {
			continue
		}
		if limitByVol && volRemaining.LessThanOrEqual(decimal.Zero) {
			outSells = append(outSells, entry)
			continue
		}
		eligible := !entry.Price.IsZero() && entry.Price.LessThanOrEqual(bar.High)
		if !eligible {
			outSells = append(outSells, entry)
			continue
		}
		// 只在需要完整订单信息时才查询 orderCache
		order := m.orderCache[entry.OrderID]
		if order == nil {
			continue
		}
		remainQty := order.OriginalQty.Sub(order.ExecutedQty)
		if remainQty.LessThanOrEqual(decimal.Zero) {
			continue
		}
		if entry.TimeInForce == ctypes.TimeInForceFOK {
			canFill := remainQty
			if limitByVol && volRemaining.LessThan(canFill) {
				_ = m.cancelOpenOrderLocked(ctx, exSymbol, order, ctypes.OrderStatusExpired, m.delayedTs(ts), "FOK")
				continue
			}
			// 冻结仓位是否足够（通常是足够，因为已冻结）；这里防御检查
			canDeliver, err := m.canDeliverSellLocked(ctx, exSymbol, order, canFill)
			if err != nil {
				return err
			}
			if !canDeliver {
				_ = m.cancelOpenOrderLocked(ctx, exSymbol, order, ctypes.OrderStatusExpired, m.delayedTs(ts), "FOK")
				continue
			}
		}

		filled, done, err := m.fillLimitSellLocked(ctx, exSymbol, order, entry.Price, bar, &volRemaining, ts)
		if err != nil {
			return err
		}
		if filled && limitByVol {
			// volRemaining 在 fill 内推进
		}
		if !done {
			if entry.TimeInForce == ctypes.TimeInForceIOC {
				_ = m.cancelOpenOrderLocked(ctx, exSymbol, order, ctypes.OrderStatusExpired, m.delayedTs(ts), "IOC")
				continue
			}
			entry.RemainQty = order.OriginalQty.Sub(order.ExecutedQty)
			outSells = append(outSells, entry)
		}
		// 订单完成时，预留资金释放由 OrderManager 在收到 OrderEvent 后处理
	}
	book.limitSells = outSells
	return nil
}

func (m *MatchingEngine) perpLeverage(ctx context.Context, accountID string, exSymbol ctypes.ExSymbol) decimal.Decimal {
	if m == nil || m.accountEngine == nil {
		return decimal.NewFromInt(1)
	}
	lev, err := m.accountEngine.GetLeverage(ctx, accountID, exSymbol.Symbol)
	if err != nil || lev <= 0 {
		lev = 1
	}
	return decimal.NewFromInt(int64(lev))
}

// calculateAffordQty 计算可承受数量（受资金/持仓约束）
func (m *MatchingEngine) calculateAffordQty(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, px decimal.Decimal, feeRate float64, isBuy bool, marketInfo *ctypes.Market) (decimal.Decimal, error) {
	if ord == nil || px.IsZero() {
		return decimal.Zero, errors.New("order or price is zero")
	}

	accountID, ok := m.orderAccountIDs[ord.OrderID]
	if !ok {
		return decimal.Zero, errors.New("account id not found")
	}

	// 从账户余额计算可承受数量（不再依赖预留资金）
	if exSymbol.GetType() == ctypes.MarketTypeFuture {
		// FUTURE：单位数量所需预留：px/leverage + px*feeRate
		lev := m.perpLeverage(ctx, accountID, exSymbol)
		denom := px.Div(lev).Add(px.Mul(decimal.NewFromFloat(feeRate)))
		if denom.IsZero() {
			return decimal.Zero, errors.New("denom is zero")
		}
		asset, err := m.accountEngine.GetAsset(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Quote)
		if err != nil {
			return decimal.Zero, err
		}
		return misc.NormalizeBaseAssetQty(asset.Balance.Div(denom), ord.OrderType, marketInfo), nil
	}

	// SPOT
	if isBuy {
		// 买入：受 quote 约束（不包含手续费，因为手续费不冻结，且买入手续费从 base 扣）
		if px.IsZero() {
			return decimal.Zero, errors.New("denom is zero")
		}
		asset, err := m.accountEngine.GetAsset(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Quote)
		if err != nil {
			return decimal.Zero, err
		}
		return misc.NormalizeBaseAssetQty(asset.Balance.Div(px), ord.OrderType, marketInfo), nil
	}

	// 卖出：受 base 约束
	asset, err := m.accountEngine.GetAsset(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Base)
	if err != nil {
		return decimal.Zero, err
	}
	return misc.NormalizeBaseAssetQty(asset.Balance, ord.OrderType, marketInfo), nil
}

func (m *MatchingEngine) canAffordBuyLocked(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, qty decimal.Decimal, px decimal.Decimal, feeRate float64) (bool, error) {
	if ord == nil {
		return false, errors.New("order is nil")
	}
	accountID, ok := m.orderAccountIDs[ord.OrderID]
	if !ok {
		return false, errors.New("account id not found")
	}
	// 直接查询账户可用余额（不再依赖预留资金）
	asset, err := m.accountEngine.GetAsset(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Quote)
	if err != nil {
		return false, err
	}
	// 买入只校验支付的 quote，不包含手续费
	return asset.Free().GreaterThanOrEqual(px.Mul(qty)), nil
}

func (m *MatchingEngine) canDeliverSellLocked(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, qty decimal.Decimal) (bool, error) {
	if ord == nil {
		return false, errors.New("order is nil")
	}
	if exSymbol.Symbol.Type == ctypes.MarketTypeFuture {
		// FUTURE：卖出开空不依赖 base 余额，交割为现金结算
		return true, nil
	}
	accountID, ok := m.orderAccountIDs[ord.OrderID]
	if !ok {
		return false, errors.New("account id not found")
	}
	// 直接查询账户可用余额（不再依赖预留资金）
	asset, err := m.accountEngine.GetAsset(ctx, accountID, exSymbol.Symbol, exSymbol.Symbol.Base)
	if err != nil {
		return false, err
	}
	return asset.Free().GreaterThanOrEqual(qty), nil
}

func (m *MatchingEngine) cancelOpenOrderLocked(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, status ctypes.OrderStatus, ts time.Time, reason string) error {
	if ord == nil {
		return errors.New("order is nil")
	}
	oldStatus := ord.Status
	if oldStatus != ctypes.OrderStatusNew && oldStatus != ctypes.OrderStatusPartialDone {
		return errors.New("order is not new or partial done")
	}

	accountID, ok := m.orderAccountIDs[ord.OrderID]
	if !ok {
		return errors.New("account id not found")
	}

	ord.Status = status
	ord.UpdatedTs = ts

	// 更新挂单计数器
	book := m.ensureBookLocked(exSymbol)
	m.updateOrderCount(book, oldStatus, status)

	// 同步更新 orderEntry 的状态
	m.updateOrderEntryStatusLocked(exSymbol, ord.OrderID, status)

	var err error
	switch status {
	case ctypes.OrderStatusCanceled:
		err = m.emitOrderCanceledEvent(ctx, exSymbol, accountID, ord.OrderID, ord.ClientOrderID, ts, reason)
	case ctypes.OrderStatusExpired:
		err = m.emitOrderExpiredEvent(ctx, exSymbol, accountID, ord.OrderID, ord.ClientOrderID, ts, reason)
	default:
		// 兜底：按 canceled 处理
		err = m.emitOrderCanceledEvent(ctx, exSymbol, accountID, ord.OrderID, ord.ClientOrderID, ts, reason)
	}
	if err != nil {
		return err
	}

	// 生命周期结束时清理映射/缓存，避免泄漏
	if status != ctypes.OrderStatusNew && status != ctypes.OrderStatusPartialDone {
		delete(m.orderAccountIDs, ord.OrderID)
		delete(m.orderCache, ord.OrderID)
	}
	return nil
}

// calculateMarketPrice 计算市价单价格（含滑点）
func (m *MatchingEngine) calculateMarketPrice(marketInfo *ctypes.Market, basePrice decimal.Decimal, isBuy bool) decimal.Decimal {
	if basePrice.IsZero() {
		return decimal.Zero
	}
	px := basePrice
	slippage := basePrice.Mul(decimal.NewFromFloat(m.cfg.SlippageRate))
	if isBuy {
		px = px.Add(slippage)
	} else {
		px = px.Sub(slippage)
	}
	px = misc.NormalizeSymbolPrice(px, ctypes.OrderTypeMarket, marketInfo)
	if px.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	return px
}

// updateOrderAfterFill 更新订单成交后状态
func (m *MatchingEngine) updateOrderAfterFill(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, fillTs time.Time) (done bool, err error) {
	oldStatus := ord.Status
	remainQty := ord.OriginalQty.Sub(ord.ExecutedQty)
	if remainQty.LessThanOrEqual(decimal.Zero) {
		ord.Status = ctypes.OrderStatusDone
		ord.UpdatedTs = fillTs
		// 更新计数器
		book := m.ensureBookLocked(exSymbol)
		m.updateOrderCount(book, oldStatus, ord.Status)
		// 同步更新 orderEntry 的状态
		m.updateOrderEntryStatusLocked(exSymbol, ord.OrderID, ord.Status)
		// 发布 OrderDoneSignal（在最后一次 FillEvent 之后）
		accountID, _ := m.orderAccountIDs[ord.OrderID]
		err = m.emitOrderDoneEvent(ctx, exSymbol, accountID, ord.OrderID, ord.ClientOrderID, fillTs)
		if err != nil {
			return false, err
		}
		// 生命周期结束时清理映射/缓存，避免泄漏
		delete(m.orderAccountIDs, ord.OrderID)
		delete(m.orderCache, ord.OrderID)
		return true, nil
	}
	ord.Status = ctypes.OrderStatusPartialDone
	ord.UpdatedTs = fillTs
	// 更新计数器（如果状态从 NEW 变为 PARTIAL_DONE，计数器不变）
	book := m.ensureBookLocked(exSymbol)
	m.updateOrderCount(book, oldStatus, ord.Status)
	// 同步更新 orderEntry 的状态
	m.updateOrderEntryStatusLocked(exSymbol, ord.OrderID, ord.Status)
	return false, nil
}

func (m *MatchingEngine) fillMarketBuyLocked(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, entry *orderEntry, bar *marketBar, volRemaining *decimal.Decimal, ts time.Time) (filled bool, done bool, err error) {
	if ord == nil || bar == nil || volRemaining == nil {
		return false, true, errors.New("order, bar or volRemaining is nil")
	}
	remainQty := ord.OriginalQty.Sub(ord.ExecutedQty)
	if remainQty.LessThanOrEqual(decimal.Zero) {
		// 订单已完成，通过统一的更新路径
		fillTs := m.delayedTs(ts)
		m.updateOrderAfterFill(ctx, exSymbol, ord, fillTs)
		return false, true, nil
	}

	marketInfo, err := proxy.GetMarket(ctx, exSymbol.Exchange, exSymbol.Symbol)
	if err != nil {
		return false, false, err
	}
	if marketInfo == nil {
		return false, false, errors.New("market not found")
	}

	// 市价单使用 bar.Open 并叠加滑点
	px := m.calculateMarketPrice(marketInfo, bar.Open, true)
	if px.IsZero() {
		return false, false, errors.New("price is zero")
	}

	feeRate := m.cfg.TakerRate()

	maxQty := remainQty
	if (*volRemaining).GreaterThan(decimal.Zero) {
		maxQty = decimal.Min(maxQty, *volRemaining)
	}

	// 计算可承受数量
	affordQty, err := m.calculateAffordQty(ctx, exSymbol, ord, px, feeRate, true, marketInfo)
	if err != nil {
		return false, false, err
	}
	if affordQty.LessThanOrEqual(decimal.Zero) {
		return false, false, errors.New("afford quantity is zero")
	}
	fillQty := decimal.Min(maxQty, affordQty)
	if fillQty.LessThanOrEqual(decimal.Zero) {
		return false, false, errors.New("fill quantity is zero")
	}

	fillTs := m.delayedTs(ts)
	err = m.applyFillBuyLocked(ctx, exSymbol, ord, fillQty, px, feeRate, fillTs, false, marketInfo)
	if err != nil {
		return false, false, err
	}
	if (*volRemaining).GreaterThan(decimal.Zero) {
		*volRemaining = (*volRemaining).Sub(fillQty)
	}

	done, err = m.updateOrderAfterFill(ctx, exSymbol, ord, fillTs)
	if err != nil {
		return false, false, err
	}
	return true, done, nil
}

func (m *MatchingEngine) fillMarketSellLocked(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, entry *orderEntry, bar *marketBar, volRemaining *decimal.Decimal, ts time.Time) (filled bool, done bool, err error) {
	if ord == nil || bar == nil || volRemaining == nil {
		return false, true, errors.New("order, bar or volRemaining is nil")
	}
	remainQty := ord.OriginalQty.Sub(ord.ExecutedQty)
	if remainQty.LessThanOrEqual(decimal.Zero) {
		// 订单已完成，通过统一的更新路径
		fillTs := m.delayedTs(ts)
		m.updateOrderAfterFill(ctx, exSymbol, ord, fillTs)
		return false, true, nil
	}

	marketInfo, err := proxy.GetMarket(ctx, exSymbol.Exchange, exSymbol.Symbol)
	if err != nil {
		return false, false, err
	}
	if marketInfo == nil {
		return false, false, errors.New("market not found")
	}

	// 市价单使用 bar.Open 并叠加滑点
	px := m.calculateMarketPrice(marketInfo, bar.Open, false)
	if px.IsZero() {
		return false, false, errors.New("price is zero")
	}

	feeRate := m.cfg.TakerRate()
	maxQty := remainQty
	if !(*volRemaining).IsZero() {
		maxQty = decimal.Min(maxQty, *volRemaining)
	}

	// 计算可承受数量
	affordQty, err := m.calculateAffordQty(ctx, exSymbol, ord, px, feeRate, false, marketInfo)
	if err != nil {
		return false, false, err
	}
	if affordQty.LessThanOrEqual(decimal.Zero) {
		return false, false, errors.New("afford quantity is zero")
	}
	fillQty := decimal.Min(maxQty, affordQty)
	if fillQty.LessThanOrEqual(decimal.Zero) {
		return false, false, errors.New("fill quantity is zero")
	}

	fillTs := m.delayedTs(ts)
	err = m.applyFillSellLocked(ctx, exSymbol, ord, fillQty, px, feeRate, fillTs, false, marketInfo)
	if err != nil {
		return false, false, err
	}
	if !(*volRemaining).IsZero() {
		*volRemaining = (*volRemaining).Sub(fillQty)
	}

	done, err = m.updateOrderAfterFill(ctx, exSymbol, ord, fillTs)
	if err != nil {
		return false, false, err
	}
	return true, done, nil
}

func (m *MatchingEngine) fillLimitBuyLocked(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, px decimal.Decimal, bar *marketBar, volRemaining *decimal.Decimal, ts time.Time) (filled bool, done bool, err error) {
	if ord == nil || bar == nil || volRemaining == nil {
		return false, true, errors.New("order, bar or volRemaining is nil")
	}
	remainQty := ord.OriginalQty.Sub(ord.ExecutedQty)
	if remainQty.LessThanOrEqual(decimal.Zero) {
		// 订单已完成，通过统一的更新路径
		fillTs := m.delayedTs(ts)
		m.updateOrderAfterFill(ctx, exSymbol, ord, fillTs)
		return false, true, nil
	}
	if px.IsZero() {
		return false, false, errors.New("price is zero")
	}

	marketInfo, err := proxy.GetMarket(ctx, exSymbol.Exchange, exSymbol.Symbol)
	if err != nil {
		return false, false, err
	}
	if marketInfo == nil {
		return false, false, errors.New("market not found")
	}

	feeRate := m.cfg.MakerRate()
	maxQty := remainQty
	if !(*volRemaining).IsZero() {
		maxQty = decimal.Min(maxQty, *volRemaining)
	}

	// 计算可承受数量
	affordQty, err := m.calculateAffordQty(ctx, exSymbol, ord, px, feeRate, true, marketInfo)
	if err != nil {
		return false, false, err
	}
	if affordQty.LessThanOrEqual(decimal.Zero) {
		return false, false, errors.New("afford quantity is zero")
	}
	fillQty := decimal.Min(maxQty, affordQty)
	if fillQty.LessThanOrEqual(decimal.Zero) {
		return false, false, errors.New("fill quantity is zero")
	}

	fillTs := m.delayedTs(ts)
	err = m.applyFillBuyLocked(ctx, exSymbol, ord, fillQty, px, feeRate, fillTs, true, marketInfo)
	if err != nil {
		return false, false, err
	}
	if !(*volRemaining).IsZero() {
		*volRemaining = (*volRemaining).Sub(fillQty)
	}

	done, err = m.updateOrderAfterFill(ctx, exSymbol, ord, fillTs)
	if err != nil {
		return false, false, err
	}
	return true, done, nil
}

func (m *MatchingEngine) fillLimitSellLocked(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, px decimal.Decimal, bar *marketBar, volRemaining *decimal.Decimal, ts time.Time) (filled bool, done bool, err error) {
	if ord == nil || bar == nil || volRemaining == nil {
		return false, true, errors.New("order, bar or volRemaining is nil")
	}
	remainQty := ord.OriginalQty.Sub(ord.ExecutedQty)
	if remainQty.LessThanOrEqual(decimal.Zero) {
		// 订单已完成，通过统一的更新路径
		fillTs := m.delayedTs(ts)
		m.updateOrderAfterFill(ctx, exSymbol, ord, fillTs)
		return false, true, nil
	}
	if px.IsZero() {
		return false, false, errors.New("price is zero")
	}

	marketInfo, err := proxy.GetMarket(ctx, exSymbol.Exchange, exSymbol.Symbol)
	if err != nil {
		return false, false, err
	}
	if marketInfo == nil {
		return false, false, errors.New("market not found")
	}

	feeRate := m.cfg.MakerRate()
	maxQty := remainQty
	if !(*volRemaining).IsZero() {
		maxQty = decimal.Min(maxQty, *volRemaining)
	}

	// 计算可承受数量
	affordQty, err := m.calculateAffordQty(ctx, exSymbol, ord, px, feeRate, false, marketInfo)
	if err != nil {
		return false, false, err
	}
	if affordQty.LessThanOrEqual(decimal.Zero) {
		return false, false, errors.New("afford quantity is zero")
	}
	fillQty := decimal.Min(maxQty, affordQty)
	if fillQty.LessThanOrEqual(decimal.Zero) {
		return false, false, errors.New("fill quantity is zero")
	}

	fillTs := m.delayedTs(ts)
	err = m.applyFillSellLocked(ctx, exSymbol, ord, fillQty, px, feeRate, fillTs, true, marketInfo)
	if err != nil {
		return false, false, err
	}
	if !(*volRemaining).IsZero() {
		*volRemaining = (*volRemaining).Sub(fillQty)
	}

	done, err = m.updateOrderAfterFill(ctx, exSymbol, ord, fillTs)
	if err != nil {
		return false, false, err
	}
	return true, done, nil
}

func (m *MatchingEngine) applyFillBuyLocked(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, fillQty decimal.Decimal, px decimal.Decimal, feeRate float64, ts time.Time, isMaker bool, marketInfo *ctypes.Market) error {
	if m.accountEngine == nil {
		return errors.New("account manager is nil")
	}
	aid, ok := m.orderAccountIDs[ord.OrderID]
	if !ok {
		return errors.New("account id not found")
	}

	notional := px.Mul(fillQty)
	feeAsset := exSymbol.Symbol.Base
	fee := fillQty.Mul(decimal.NewFromFloat(feeRate))

	if exSymbol.Symbol.Type == ctypes.MarketTypeFuture {
		// FUTURE：手续费从保证金资产扣，需确保余额覆盖手续费
		asset, _ := m.accountEngine.GetAsset(ctx, aid, exSymbol.Symbol, exSymbol.Symbol.Quote)
		if asset == nil || asset.Free().LessThan(fee) {
			// 资金不足以支付手续费：不成交（保守处理）
			return errors.New("insufficient collateral balance")
		}
		// 期货场景手续费扣在保证金资产上
		feeAsset = exSymbol.Symbol.Quote
		fee = notional.Mul(decimal.NewFromFloat(feeRate))
	}

	fee = fee.RoundDown(int32(marketInfo.BaseAssetPrecision))

	// 更新订单成交状态
	ord.ExecutedQty = ord.ExecutedQty.Add(fillQty)
	ord.ExecutedQuoteQty = ord.ExecutedQuoteQty.Add(notional)
	if ord.ExecutedQty.GreaterThan(decimal.Zero) {
		ord.AvgPrice = ord.ExecutedQuoteQty.Div(ord.ExecutedQty)
	}
	ord.UpdatedTs = ts

	// 发布成交事件（Account 和 OrderManager 会订阅并更新状态）
	return m.emitFillEvent(ctx, exSymbol, aid, ord.OrderID, ord.ClientOrderID, ord.Side, ord.IsBuy, fillQty, px, fee, feeAsset, ts)
}

func (m *MatchingEngine) applyFillSellLocked(ctx context.Context, exSymbol ctypes.ExSymbol, ord *ctypes.Order, fillQty decimal.Decimal, px decimal.Decimal, feeRate float64, ts time.Time, isMaker bool, marketInfo *ctypes.Market) error {
	if m.accountEngine == nil {
		return errors.New("account manager or event bus is nil")
	}
	aid, ok := m.orderAccountIDs[ord.OrderID]
	if !ok {
		return errors.New("account id not found")
	}

	notional := px.Mul(fillQty)
	feeAsset := exSymbol.Symbol.Quote
	fee := notional.Mul(decimal.NewFromFloat(feeRate))

	if exSymbol.Symbol.Type == ctypes.MarketTypeFuture {
		// FUTURE：卖出意味着 posQty 减少（可用于减多/开空），手续费从 collateral 扣
		asset, _ := m.accountEngine.GetAsset(ctx, aid, exSymbol.Symbol, exSymbol.Symbol.Quote)
		if asset == nil || asset.Free().LessThan(fee) {
			return errors.New("insufficient collateral balance")
		}
	}

	fee = fee.RoundDown(int32(marketInfo.PricePrecision))

	// 更新订单成交状态
	ord.ExecutedQty = ord.ExecutedQty.Add(fillQty)
	ord.ExecutedQuoteQty = ord.ExecutedQuoteQty.Add(notional)
	if ord.ExecutedQty.GreaterThan(decimal.Zero) {
		ord.AvgPrice = ord.ExecutedQuoteQty.Div(ord.ExecutedQty)
	}
	ord.UpdatedTs = ts

	// 发布成交事件（Account 和 OrderManager 会订阅并更新状态）
	return m.emitFillEvent(ctx, exSymbol, aid, ord.OrderID, ord.ClientOrderID, ord.Side, ord.IsBuy, fillQty, px, fee, feeAsset, ts)
}

func (m *MatchingEngine) emitOrderAcceptedEvent(ctx context.Context, exSymbol ctypes.ExSymbol, accountID string, exOrderID, clientOrderID ctypes.OrderId, ts time.Time) error {
	ev := bridge.OrderEvent{
		Kind:            bridge.ExchangeEventKindOrderAccepted,
		Ts:              ts,
		Exchange:        exSymbol.Exchange,
		Symbol:          exSymbol.Symbol,
		AccountID:       accountID,
		ExchangeOrderID: exOrderID,
		ClientOrderID:   clientOrderID,
	}
	return m.outBus.Publish(ctx, ev)
}

func (m *MatchingEngine) emitOrderCanceledEvent(ctx context.Context, exSymbol ctypes.ExSymbol, accountID string, exOrderID, clientOrderID ctypes.OrderId, ts time.Time, reason string) error {
	ev := bridge.OrderEvent{
		Kind:            bridge.ExchangeEventKindOrderCanceled,
		Ts:              ts,
		Exchange:        exSymbol.Exchange,
		Symbol:          exSymbol.Symbol,
		AccountID:       accountID,
		ExchangeOrderID: exOrderID,
		ClientOrderID:   clientOrderID,
		Reason:          reason,
	}
	return m.outBus.Publish(ctx, ev)
}

func (m *MatchingEngine) emitOrderRejectedEvent(ctx context.Context, exSymbol ctypes.ExSymbol, accountID string, exOrderID, clientOrderID ctypes.OrderId, ts time.Time, code, reason string) error {
	ev := bridge.OrderEvent{
		Kind:            bridge.ExchangeEventKindOrderRejected,
		Ts:              ts,
		Exchange:        exSymbol.Exchange,
		Symbol:          exSymbol.Symbol,
		AccountID:       accountID,
		ExchangeOrderID: exOrderID,
		ClientOrderID:   clientOrderID,
		Reason:          reason,
		Code:            code,
	}
	return m.outBus.Publish(ctx, ev)
}

func (m *MatchingEngine) emitOrderExpiredEvent(ctx context.Context, exSymbol ctypes.ExSymbol, accountID string, exOrderID, clientOrderID ctypes.OrderId, ts time.Time, reason string) error {
	ev := bridge.OrderEvent{
		Kind:            bridge.ExchangeEventKindOrderExpired,
		Ts:              ts,
		Exchange:        exSymbol.Exchange,
		Symbol:          exSymbol.Symbol,
		AccountID:       accountID,
		ExchangeOrderID: exOrderID,
		ClientOrderID:   clientOrderID,
		Reason:          reason,
	}
	return m.outBus.Publish(ctx, ev)
}

func (m *MatchingEngine) emitOrderDoneEvent(ctx context.Context, exSymbol ctypes.ExSymbol, accountID string, exOrderID, clientOrderID ctypes.OrderId, ts time.Time) error {
	ev := bridge.OrderEvent{
		Kind:            bridge.ExchangeEventKindOrderDone,
		Ts:              ts,
		Exchange:        exSymbol.Exchange,
		Symbol:          exSymbol.Symbol,
		AccountID:       accountID,
		ExchangeOrderID: exOrderID,
		ClientOrderID:   clientOrderID,
	}
	return m.outBus.Publish(ctx, ev)
}

// emitFillEvent 输出成交事件（交易所语义）
func (m *MatchingEngine) emitFillEvent(ctx context.Context, exSymbol ctypes.ExSymbol, accountID string, exOrderID, clientOrderID ctypes.OrderId, side ctypes.PositionSide, isBuy bool, fillQty, px, fee decimal.Decimal, feeAsset string, ts time.Time) error {
	ev := bridge.FillEvent{
		Kind:            bridge.ExchangeEventKindFill,
		Ts:              ts,
		Exchange:        exSymbol.Exchange,
		Symbol:          exSymbol.Symbol,
		AccountID:       accountID,
		ExchangeOrderID: exOrderID,
		ClientOrderID:   clientOrderID,
		Side:            side,
		IsBuy:           isBuy,
		Qty:             fillQty,
		Price:           px,
		Fee:             fee,
		Asset:           feeAsset,
	}
	log.Info().Interface("fillEvent", ev).Msg("emit fill event")
	return m.outBus.Publish(ctx, ev)
}

func removeOrderEntryFromSlice(in []*orderEntry, orderID ctypes.OrderId) []*orderEntry {
	if len(in) == 0 || orderID == "" {
		return in
	}
	out := in[:0]
	for _, e := range in {
		if e == nil || e.OrderID == orderID {
			continue
		}
		out = append(out, e)
	}
	return out
}
