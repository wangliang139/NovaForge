package simulate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/binance"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/okx"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	simcore "github.com/wangliang139/NovaForge/server/pkg/simulate"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type Connector struct {
	exchange  ctypes.Exchange
	accountID string

	state *exchangeState

	subMu       sync.Mutex
	accountSubs map[chan *ctypes.Message]struct{}
}

type exchangeState struct {
	mu sync.RWMutex

	ex         *simcore.SimExchange
	adapter    *simcore.ConnectorAdapter
	ticker     *simcore.TickerStore
	publicConn mdtypes.Connector
	depths     map[simcore.Symbol]*simcore.MarketDepth
	leverages  map[accountSymbolKey]int
	bootstraps map[string]bool

	// markPriceOnce ensures at most one public mark-price subscription per symbol (exchange-wide).
	markPriceMu   sync.Mutex
	markPriceOnce map[string]*sync.Once
}

type accountSymbolKey struct {
	accountID string
	symbol    simcore.Symbol
}

var (
	statesMu sync.Mutex
	states   = make(map[ctypes.Exchange]*exchangeState)
)

var _ mdtypes.Connector = (*Connector)(nil)

func New(exchange ctypes.Exchange, account *mdtypes.ApiAccount) (*Connector, error) {
	if !exchange.IsValid() {
		return nil, fmt.Errorf("invalid exchange: %s", exchange)
	}
	if account == nil || account.ID == "" {
		return nil, fmt.Errorf("simulate connector requires account id")
	}

	state, err := getOrCreateState(exchange)
	if err != nil {
		return nil, err
	}

	return &Connector{
		exchange:    exchange,
		accountID:   account.ID,
		state:       state,
		accountSubs: make(map[chan *ctypes.Message]struct{}),
	}, nil
}

func getOrCreateState(exchange ctypes.Exchange) (*exchangeState, error) {
	statesMu.Lock()
	defer statesMu.Unlock()

	if st, ok := states[exchange]; ok {
		return st, nil
	}

	publicConn, err := newPublicConnector(exchange)
	if err != nil {
		return nil, err
	}

	ex := simcore.NewSimExchange()
	st := &exchangeState{
		ex:            ex,
		adapter:       simcore.NewConnectorAdapter(ex),
		ticker:        simcore.NewTickerStore(),
		publicConn:    publicConn,
		depths:        make(map[simcore.Symbol]*simcore.MarketDepth),
		leverages:     make(map[accountSymbolKey]int),
		bootstraps:    make(map[string]bool),
		markPriceOnce: make(map[string]*sync.Once),
	}
	states[exchange] = st
	return st, nil
}

func newPublicConnector(exchange ctypes.Exchange) (mdtypes.Connector, error) {
	switch exchange {
	case ctypes.ExchangeBinance:
		return binance.New(binance.Config{}, nil)
	case ctypes.ExchangeBinanceTest:
		return binance.New(binance.Config{UseDemo: true}, nil)
	case ctypes.ExchangeOkx:
		return okx.New(okx.Config{}, nil)
	case ctypes.ExchangeOkxTest:
		return okx.New(okx.Config{UseTestnet: true}, nil)
	default:
		return nil, fmt.Errorf("unsupported exchange: %s", exchange)
	}
}

func (c *Connector) Exchange() ctypes.Exchange {
	return c.exchange
}

func (c *Connector) IsPrivate() bool {
	return true
}

func (c *Connector) Supports(selector ctypes.StreamSelector) bool {
	switch selector.Stream {
	case ctypes.StreamTypeTicker,
		ctypes.StreamTypeTrade,
		ctypes.StreamTypeDepth,
		ctypes.StreamTypeKline,
		ctypes.StreamTypeMarkPrice:
		return c.state.publicConn.Supports(selector)
	case ctypes.StreamTypeAccountRaw, ctypes.StreamTypeAccount:
		return true
	default:
		return false
	}
}

func (c *Connector) Subscribe(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	switch selector.Stream {
	case ctypes.StreamTypeTicker, ctypes.StreamTypeDepth, ctypes.StreamTypeMarkPrice:
		handle, err := c.state.publicConn.Subscribe(ctx, selector)
		if err != nil {
			return nil, err
		}
		out := make(chan *ctypes.Message, 256)
		errCh := make(chan error, 1)
		stopC := make(chan struct{})
		doneC := make(chan struct{})

		go func() {
			defer close(doneC)
			defer close(out)
			for {
				select {
				case <-ctx.Done():
					return
				case <-stopC:
					return
				case err, ok := <-handle.ErrCh:
					if !ok {
						return
					}
					select {
					case errCh <- err:
					default:
					}
				case msg, ok := <-handle.C:
					if !ok {
						return
					}
					if msg != nil {
						c.ingestMessage(msg)
						select {
						case out <- msg:
						case <-ctx.Done():
							return
						case <-stopC:
							return
						}
					}
				}
			}
		}()

		return mdtypes.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	case ctypes.StreamTypeAccountRaw, ctypes.StreamTypeAccount:
		if selector.Account != nil && *selector.Account != "" && *selector.Account != c.accountID {
			return nil, fmt.Errorf("account mismatch: selector=%s connector=%s", *selector.Account, c.accountID)
		}
		out := make(chan *ctypes.Message, 256)
		errCh := make(chan error, 1)
		stopC := make(chan struct{})
		doneC := make(chan struct{})
		c.subMu.Lock()
		c.accountSubs[out] = struct{}{}
		c.subMu.Unlock()
		go func() {
			defer close(doneC)
			defer close(out)
			select {
			case <-ctx.Done():
			case <-stopC:
			}
			c.subMu.Lock()
			delete(c.accountSubs, out)
			c.subMu.Unlock()
		}()
		return mdtypes.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	default:
		return nil, fmt.Errorf("unsupported selector: %s", selector.Stream)
	}
}

func (c *Connector) ingestMessage(msg *ctypes.Message) {
	if msg.Depth != nil && msg.Depth.Symbol.IsValid() {
		beforeBal, beforePos := c.snapshotAccountState(msg.Depth.Symbol)
		c.state.mu.Lock()
		events := c.applyDepthBookLocked(msg.Depth, true)
		c.state.mu.Unlock()
		afterBal, afterPos := c.snapshotAccountState(msg.Depth.Symbol)
		for _, em := range c.buildMakerMatchMessages(msg.Depth.Symbol, events, beforeBal, afterBal, beforePos, afterPos) {
			c.publishAccountMessage(em)
		}
	}
	if msg.Ticker != nil && msg.Ticker.Symbol.IsValid() {
		c.state.mu.Lock()
		c.state.ticker.Update(simcore.Ticker{
			Symbol: toSimSymbol(msg.Ticker.Symbol),
			Last:   msg.Ticker.LastPrice,
			Mark:   msg.Ticker.LastPrice,
			Ts:     msg.Ticker.Ts,
		})
		c.state.mu.Unlock()
	}
	if msg.MarkPrice != nil && msg.MarkPrice.Symbol.IsValid() {
		c.ingestMarkPrice(msg.MarkPrice)
	}
}

func (c *Connector) GetMarkets(ctx context.Context, tps []ctypes.MarketType) ([]*ctypes.Market, error) {
	return c.state.publicConn.GetMarkets(ctx, tps)
}

func (c *Connector) GetMarket(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Market, error) {
	market, err := c.state.publicConn.GetMarket(ctx, symbol)
	if err != nil {
		return nil, err
	}
	return market, nil
}

func (c *Connector) Prices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.Price, error) {
	return c.state.publicConn.Prices(ctx, marketType)
}

func (c *Connector) Price(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Price, error) {
	price, err := c.state.publicConn.Price(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if price != nil {
		c.state.mu.Lock()
		c.state.ticker.Update(simcore.Ticker{
			Symbol: toSimSymbol(symbol),
			Last:   price.Price,
			Mark:   price.Price,
			Ts:     price.Ts,
		})
		c.state.mu.Unlock()
	}
	return price, nil
}

func (c *Connector) BookPrices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.BookPrice, error) {
	return c.state.publicConn.BookPrices(ctx, marketType)
}

func (c *Connector) BookPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.BookPrice, error) {
	return c.state.publicConn.BookPrice(ctx, symbol)
}

func (c *Connector) MarkPrices(ctx context.Context) ([]*ctypes.MarkPrice, error) {
	return c.state.publicConn.MarkPrices(ctx)
}

func (c *Connector) MarkPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.MarkPrice, error) {
	return c.state.publicConn.MarkPrice(ctx, symbol)
}

func (c *Connector) IndexPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexPrice, error) {
	return c.state.publicConn.IndexPrice(ctx, symbol)
}

func (c *Connector) IndexComponent(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexComponent, error) {
	return c.state.publicConn.IndexComponent(ctx, symbol)
}

func (c *Connector) Ticker(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Ticker, error) {
	sym := toSimSymbol(symbol)
	c.state.mu.RLock()
	local, ok := c.state.ticker.Get(sym)
	c.state.mu.RUnlock()
	if ok && local.Last.GreaterThan(decimal.Zero) {
		return &ctypes.Ticker{
			Exchange:  c.exchange,
			Symbol:    symbol,
			LastPrice: local.Last,
			Ts:        local.Ts,
		}, nil
	}
	return c.state.publicConn.Ticker(ctx, symbol)
}

func (c *Connector) Trades(ctx context.Context, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	return c.state.publicConn.Trades(ctx, symbol, limit)
}

func (c *Connector) Depth(ctx context.Context, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error) {
	return c.state.publicConn.Depth(ctx, symbol, limit)
}

func (c *Connector) Klines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	return c.state.publicConn.Klines(ctx, symbol, interval, limit)
}

func (c *Connector) HisKlines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	return c.state.publicConn.HisKlines(ctx, symbol, interval, startTs, endTs, limit)
}

func (c *Connector) FundingRate(ctx context.Context, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	return c.state.publicConn.FundingRate(ctx, symbol)
}

func (c *Connector) HisFundingRates(ctx context.Context, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	return c.state.publicConn.HisFundingRates(ctx, symbol, startTs, endTs, limit)
}

func (c *Connector) OpenInterest(ctx context.Context, symbol ctypes.Symbol) (decimal.Decimal, error) {
	return c.state.publicConn.OpenInterest(ctx, symbol)
}

func (c *Connector) Account(ctx context.Context) (*ctypes.AccountBo, error) {
	return &ctypes.AccountBo{
		Exchange:        c.exchange,
		Uid:             c.accountID,
		IsSpotEnabled:   true,
		IsFutureEnabled: true,
	}, nil
}

func (c *Connector) Balance(ctx context.Context) (*ctypes.Balance, error) {
	c.state.mu.RLock()
	balMap := c.state.adapter.Balance(ctx, c.accountID)
	c.state.mu.RUnlock()

	assets := make([]*ctypes.AssetBo, 0, len(balMap))
	for key, amount := range balMap {
		assets = append(assets, &ctypes.AssetBo{
			AccountID:  c.accountID,
			WalletType: key.Wallet,
			Code:       string(key.Asset),
			Balance:    amount,
			Locked:     decimal.Zero,
		})
	}
	return &ctypes.Balance{Assets: assets}, nil
}

func (c *Connector) Positions(ctx context.Context, mt *ctypes.MarketType) ([]*ctypes.Position, error) {
	if mt != nil && *mt == ctypes.MarketTypeSpot {
		return []*ctypes.Position{}, nil
	}
	c.state.mu.RLock()
	defer c.state.mu.RUnlock()
	out := make([]*ctypes.Position, 0)
	for sym := range c.state.depths {
		typedSym := toTypesSymbol(sym)
		if !typedSym.IsValid() || typedSym.Type != ctypes.MarketTypeFuture {
			continue
		}
		pos, ok := c.state.adapter.Position(ctx, c.accountID, sym)
		if !ok || pos.Qty.IsZero() {
			continue
		}
		side := ctypes.PositionSideLong
		amount := pos.Qty
		if pos.Qty.Sign() < 0 {
			side = ctypes.PositionSideShort
			amount = pos.Qty.Abs()
		}
		mark := c.resolveMarkPriceLocked(typedSym)
		notional := amount.Mul(mark)
		lev := int(pos.Leverage)
		if lev <= 0 {
			lev = c.leverageLocked(sym)
		}
		if lev <= 0 {
			lev = 1
		}
		out = append(out, &ctypes.Position{
			AccountID:     c.accountID,
			Exchange:      c.exchange,
			Symbol:        typedSym,
			Side:          side,
			Isolated:      true,
			Amount:        amount,
			EntryPrice:    pos.EntryPrice,
			MarkPrice:     mark,
			Notional:      notional,
			Leverage:      lev,
			InitialMargin: pos.UsedMargin,
			MaintMargin:   decimal.Zero,
			UpdatedTs:     time.Now().UTC(),
		})
	}
	return out, nil
}

func (c *Connector) SymbolConfig(ctx context.Context, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	market, err := c.GetMarket(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if market == nil {
		return nil, nil
	}
	maker, taker := defaultFeesByMarketType(symbol.Type)
	cfg := &ctypes.SymbolConfig{
		Exchange:        c.exchange,
		Symbol:          symbol,
		Market:          *market,
		MakerCommission: maker,
		TakerCommission: taker,
	}
	c.normalizeMarketPrecision(&cfg.Market)
	return cfg, nil
}

func (c *Connector) GetOrders(ctx context.Context, symbol *ctypes.Symbol) ([]*ctypes.Order, error) {
	if symbol == nil {
		return []*ctypes.Order{}, nil
	}
	c.state.mu.RLock()
	orders := c.state.adapter.GetOrders(ctx, c.accountID, toSimSymbol(*symbol))
	c.state.mu.RUnlock()
	out := make([]*ctypes.Order, 0, len(orders))
	for _, od := range orders {
		out = append(out, toTypesOrder(c.exchange, od))
	}
	return out, nil
}

func (c *Connector) GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	c.state.mu.RLock()
	order, ok := c.state.adapter.GetOrder(ctx, c.accountID, toSimSymbol(symbol), orderId)
	c.state.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	return toTypesOrder(c.exchange, order), nil
}

func (c *Connector) CalcOrderFee(ctx context.Context, order ctypes.Order) (*decimal.Decimal, *string, error) {
	return nil, nil, nil
}

func (c *Connector) PlaceOrder(ctx context.Context, input mdtypes.PlaceOrderInput) (*mdtypes.PlaceOrderResult, error) {
	if err := c.validatePlaceOrderInputBasic(input); err != nil {
		return nil, err
	}

	market, err := c.GetMarket(ctx, input.Symbol)
	if err != nil {
		return nil, err
	}
	if market == nil {
		return nil, fmt.Errorf("market not found: %s", input.Symbol)
	}
	if err := c.validatePlaceOrderByMarketRules(ctx, input, market); err != nil {
		return nil, err
	}
	c.ensureInstrument(market)
	if err := c.syncMarketDepth(ctx, input.Symbol, true); err != nil {
		return nil, err
	}

	req := simcore.PlaceOrderRequest{
		AccountID:     c.accountID,
		Symbol:        toSimSymbol(input.Symbol),
		OrderType:     toSimOrderType(input.OrderType),
		Side:          toSimSide(input.IsBuy),
		Intent:        toSimIntent(input),
		ReduceOnly:    lo.FromPtr(input.ReduceOnly),
		Leverage:      int32(c.currentLeverage(toSimSymbol(input.Symbol))),
		Price:         lo.FromPtr(input.Price),
		Qty:           lo.FromPtr(input.Quantity),
		ClientOrderID: lo.FromPtr(input.ClientOrderID).String(),
	}
	beforeBal, beforePos := c.snapshotAccountState(input.Symbol)
	c.state.mu.Lock()
	res, err := c.state.adapter.PlaceOrder(ctx, req)
	c.state.mu.Unlock()
	if err != nil {
		return nil, err
	}
	afterBal, afterPos := c.snapshotAccountState(input.Symbol)
	for _, em := range c.buildOrderAndSnapshotMessages(input.Symbol, beforeBal, afterBal, beforePos, afterPos, res) {
		c.publishAccountMessage(em)
	}
	return &mdtypes.PlaceOrderResult{
		OrderID:       ctypes.OrderId(res.Order.ID),
		ClientOrderID: ctypes.OrderId(res.Order.ClientOrderID),
		Status:        toTypesOrderStatus(res.Order.Status),
	}, nil
}

// SeedAccountBalances initializes simulate balances once from external account assets.
func (c *Connector) SeedAccountBalances(bals map[ctypes.WalletType]map[simcore.Asset]decimal.Decimal) error {
	if len(bals) == 0 {
		return nil
	}
	c.state.mu.Lock()
	if c.state.bootstraps[c.accountID] {
		c.state.mu.Unlock()
		return nil
	}
	c.state.bootstraps[c.accountID] = true
	c.state.mu.Unlock()
	return c.state.ex.InitBalances(c.accountID, bals)
}

func (c *Connector) SeedAccountPositions(positions map[ctypes.Symbol]ctypes.Position) error {
	for sym, p := range positions {
		if !sym.IsValid() {
			continue
		}
		sqty := p.Amount
		if p.Side == ctypes.PositionSideShort {
			sqty = sqty.Neg()
		}
		err := c.state.ex.InitPosition(c.accountID, toSimSymbol(sym), simcore.Position{
			Qty:        sqty,
			EntryPrice: p.EntryPrice,
			UsedMargin: p.InitialMargin,
			Leverage:   int32(maxInt(p.Leverage, 1)),
		})
		if err != nil {
			return err
		}
		if p.Leverage > 0 {
			c.state.mu.Lock()
			c.state.leverages[accountSymbolKey{accountID: c.accountID, symbol: toSimSymbol(sym)}] = p.Leverage
			c.state.mu.Unlock()
		}
	}
	return nil
}

func (c *Connector) SeedOpenOrders(orders []*ctypes.Order) error {
	for _, od := range orders {
		if od == nil || !od.Symbol.IsValid() {
			continue
		}
		rem := od.OriginalQty.Sub(od.ExecutedQty)
		if rem.Sign() <= 0 {
			continue
		}
		st := simcore.OrderStatusNew
		if od.Status == ctypes.OrderStatusPartialDone {
			st = simcore.OrderStatusPartiallyFilled
		}
		err := c.state.ex.SeedOpenOrder(simcore.SimOrder{
			ID:            od.OrderID.String(),
			AccountID:     c.accountID,
			ClientOrderID: od.ClientOrderID.String(),
			Symbol:        toSimSymbol(od.Symbol),
			OrderType:     toSimOrderType(od.OrderType),
			Side:          toSimSide(od.IsBuy),
			Price:         od.Price,
			QtyOriginal:   od.OriginalQty,
			QtyFilled:     od.ExecutedQty,
			QtyRemaining:  rem,
			AvgFillPrice:  od.AvgPrice,
			Status:        st,
			CreatedAt:     od.CreatedTs,
			LastUpdatedAt: od.UpdatedTs,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Connector) RemoveAccountState() {
	c.state.mu.Lock()
	delete(c.state.bootstraps, c.accountID)
	for key := range c.state.leverages {
		if key.accountID == c.accountID {
			delete(c.state.leverages, key)
		}
	}
	c.state.mu.Unlock()
	c.state.ex.RemoveAccount(c.accountID)
}

func (c *Connector) WarmSymbols(ctx context.Context, symbols []ctypes.Symbol) {
	uniq := make(map[string]ctypes.Symbol)
	for _, sym := range symbols {
		if sym.IsValid() {
			uniq[sym.String()] = sym
		}
	}
	for _, sym := range uniq {
		_ = c.ensureSymbolInitialized(ctx, sym)
	}
}

func (c *Connector) ensureSymbolInitialized(ctx context.Context, symbol ctypes.Symbol) error {
	market, err := c.GetMarket(ctx, symbol)
	if err != nil {
		return err
	}
	if market != nil {
		c.ensureInstrument(market)
	}
	if err := c.syncMarketDepth(ctx, symbol, false); err != nil {
		return err
	}
	if symbol.Type == ctypes.MarketTypeFuture {
		if mp, err := c.state.publicConn.MarkPrice(ctx, symbol); err == nil && mp != nil && mp.MarkPrice.GreaterThan(decimal.Zero) {
			c.ingestMarkPrice(mp)
		}
		c.ensureMarkPriceStreamStarted(symbol)
	}
	return nil
}

// syncMarketDepth loads public L2 into the shared MarketDepth without triggering maker matching.
// emptyFallback: when true (PlaceOrder), install an empty snapshot if the feed never produced a seq id;
// when false (WarmSymbols), no empty book — mirroring the previous ensureDepthInitialized / ensureSymbolInitialized split.
func (c *Connector) syncMarketDepth(ctx context.Context, symbol ctypes.Symbol, emptyFallback bool) error {
	sym := toSimSymbol(symbol)
	c.state.mu.RLock()
	d, ok := c.state.depths[sym]
	depthReady := ok && d != nil && d.LastSeqID() > 0
	c.state.mu.RUnlock()
	if depthReady {
		return nil
	}
	snapshot, err := c.Depth(ctx, symbol, 500)
	if err != nil {
		return err
	}
	c.state.mu.Lock()
	if snapshot != nil && snapshot.Symbol.IsValid() {
		_ = c.applyDepthBookLocked(snapshot, false)
	}
	d, ok = c.state.depths[sym]
	depthReady = ok && d != nil && d.LastSeqID() > 0
	if !depthReady && emptyFallback {
		// Fallback for private-order flows where public depth is unavailable:
		// bootstrap an empty snapshot so order lifecycle can proceed.
		_ = c.applyDepthBookLocked(&ctypes.OrderBook{
			Symbol: symbol,
			SeqId:  1,
			Ts:     time.Now().UTC(),
		}, false)
	}
	c.state.mu.Unlock()
	return nil
}

// applyDepthBookLocked installs a depth snapshot/delta into the shared MarketDepth. When matchResting
// is true (incremental public updates), resting orders are matched via SimExchange.OnDepthUpdated.
// Bootstrap paths (initialization) pass matchResting=false so no fills are driven during warm-up.
func (c *Connector) applyDepthBookLocked(book *ctypes.OrderBook, matchResting bool) []simcore.MatchEvent {
	if book == nil || !book.Symbol.IsValid() {
		return nil
	}
	sym := toSimSymbol(book.Symbol)
	d, ok := c.state.depths[sym]
	if !ok {
		d = simcore.NewMarketDepth()
		c.state.depths[sym] = d
		_ = c.state.ex.BindDepth(sym, d)
	}
	depth := toSimOrderBook(book)
	if depth.PrevSeqId > 0 && d.LastSeqID() > 0 {
		if err := d.ApplyDelta(lo.ToPtr(depth)); err != nil {
			_ = d.ApplySnapshot(lo.ToPtr(depth))
		}
	} else {
		_ = d.ApplySnapshot(lo.ToPtr(depth))
	}
	if !matchResting {
		return nil
	}
	events, _ := c.state.ex.OnDepthUpdated(sym)
	return events
}

func (c *Connector) ingestMarkPrice(mp *ctypes.MarkPrice) {
	if mp == nil || !mp.Symbol.IsValid() || !mp.MarkPrice.GreaterThan(decimal.Zero) {
		return
	}
	c.state.mu.Lock()
	sym := toSimSymbol(mp.Symbol)
	prev, _ := c.state.ticker.Get(sym)
	last := prev.Last
	if !last.GreaterThan(decimal.Zero) {
		last = mp.MarkPrice
	}
	c.state.ticker.Update(simcore.Ticker{
		Symbol: sym,
		Last:   last,
		Mark:   mp.MarkPrice,
		Ts:     mp.Ts,
	})
	c.state.mu.Unlock()
	c.tryLiquidateAfterMark(mp.Symbol, mp.MarkPrice)
}

func (c *Connector) ensureMarkPriceStreamStarted(symbol ctypes.Symbol) {
	if symbol.Type != ctypes.MarketTypeFuture || !symbol.IsValid() {
		return
	}
	key := symbol.String()
	c.state.markPriceMu.Lock()
	if c.state.markPriceOnce == nil {
		c.state.markPriceOnce = make(map[string]*sync.Once)
	}
	o, ok := c.state.markPriceOnce[key]
	if !ok {
		o = &sync.Once{}
		c.state.markPriceOnce[key] = o
	}
	c.state.markPriceMu.Unlock()

	sel := ctypes.StreamSelector{Stream: ctypes.StreamTypeMarkPrice, Symbol: &symbol}
	if !c.state.publicConn.Supports(sel) {
		return
	}

	o.Do(func() {
		// Stream lifetime follows the process; cancellation is not tied to WarmSymbols ctx.
		go c.runMarkPriceIngest(context.Background(), symbol)
	})
}

func (c *Connector) runMarkPriceIngest(ctx context.Context, symbol ctypes.Symbol) {
	sel := ctypes.StreamSelector{Stream: ctypes.StreamTypeMarkPrice, Symbol: &symbol}
	h, err := c.state.publicConn.Subscribe(ctx, sel)
	if err != nil {
		return
	}
	defer h.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-h.C:
			if !ok {
				return
			}
			if msg != nil && msg.MarkPrice != nil {
				c.ingestMarkPrice(msg.MarkPrice)
			}
		}
	}
}

// liquidationLossRatio triggers forced close when unrealized PnL <= -ratio * usedInitialMargin (minimal model).
var liquidationLossRatio = decimal.RequireFromString("0.9")

func (c *Connector) tryLiquidateAfterMark(symbol ctypes.Symbol, mark decimal.Decimal) {
	if symbol.Type != ctypes.MarketTypeFuture || !mark.GreaterThan(decimal.Zero) {
		return
	}
	sym := toSimSymbol(symbol)
	pos, ok := c.state.adapter.Position(context.Background(), c.accountID, sym)
	if !ok || pos.Qty.IsZero() {
		return
	}
	ins, ok := c.state.ex.InstrumentBySymbol(sym)
	if !ok || ins.Kind != simcore.KindPerp {
		return
	}

	var upnl decimal.Decimal
	if pos.Qty.Sign() > 0 {
		upnl = mark.Sub(pos.EntryPrice).Mul(pos.Qty)
	} else {
		upnl = pos.EntryPrice.Sub(mark).Mul(pos.Qty.Abs())
	}
	trigger := pos.UsedMargin.Mul(liquidationLossRatio).Neg()
	if upnl.GreaterThan(trigger) {
		return
	}

	beforeBal, beforePos := c.snapshotAccountState(symbol)
	if err := c.state.ex.ForceClosePerpAtMark(c.accountID, sym, mark); err != nil {
		return
	}
	afterBal, afterPos := c.snapshotAccountState(symbol)
	for _, em := range c.buildSnapshotDiffMessages(symbol, beforeBal, afterBal, beforePos, afterPos) {
		c.publishAccountMessage(em)
	}
}

func (c *Connector) validatePlaceOrderInputBasic(input mdtypes.PlaceOrderInput) error {
	if !input.Symbol.IsValid() {
		return fmt.Errorf("invalid symbol: %s", input.Symbol)
	}
	if !input.OrderType.Valid() {
		return fmt.Errorf("invalid order type: %s", input.OrderType)
	}
	switch input.OrderType {
	case ctypes.OrderTypeLimit:
		if input.Price == nil || !input.Price.GreaterThan(decimal.Zero) {
			return fmt.Errorf("price must be > 0 for limit order")
		}
		if input.Quantity == nil || !input.Quantity.GreaterThan(decimal.Zero) {
			return fmt.Errorf("quantity must be > 0 for limit order")
		}
	case ctypes.OrderTypeMarket:
		if input.Quantity != nil && !input.Quantity.GreaterThan(decimal.Zero) {
			return fmt.Errorf("quantity must be > 0")
		}
		if input.QuoteQty != nil && !input.QuoteQty.GreaterThan(decimal.Zero) {
			return fmt.Errorf("quote_qty must be > 0")
		}
		// Spot MVP keeps behavior deterministic with quantity path only.
		if input.Quantity == nil {
			return fmt.Errorf("quantity is required for market order in simulate spot mvp")
		}
	default:
		return fmt.Errorf("unsupported order type: %s", input.OrderType)
	}
	return nil
}

func (c *Connector) validatePlaceOrderByMarketRules(ctx context.Context, input mdtypes.PlaceOrderInput, market *ctypes.Market) error {
	if market == nil {
		return nil
	}
	if market.Status != "" && market.Status != ctypes.MarketStatusTrading {
		return fmt.Errorf("market is not tradable: %s", market.Status)
	}
	rules := market.Rules
	qty := lo.FromPtr(input.Quantity)
	price := lo.FromPtr(input.Price)

	if input.OrderType == ctypes.OrderTypeLimit {
		if err := validatePriceRules(price, rules); err != nil {
			return err
		}
	} else {
		refPrice, err := c.resolveMarketOrderReferencePrice(ctx, input.Symbol)
		if err != nil {
			return err
		}
		price = refPrice
	}
	if err := validateQuantityRules(qty, rules); err != nil {
		return err
	}
	return validateNotionalRules(price, qty, rules)
}

func (c *Connector) resolveMarketOrderReferencePrice(ctx context.Context, symbol ctypes.Symbol) (decimal.Decimal, error) {
	c.state.mu.RLock()
	tk, ok := c.state.ticker.Get(toSimSymbol(symbol))
	c.state.mu.RUnlock()
	if ok && tk.Last.GreaterThan(decimal.Zero) {
		return tk.Last, nil
	}
	price, err := c.state.publicConn.Price(ctx, symbol)
	if err != nil {
		return decimal.Zero, err
	}
	if price == nil || !price.Price.GreaterThan(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("price unavailable for market order: %s", symbol)
	}
	return price.Price, nil
}

func validatePriceRules(price decimal.Decimal, rules ctypes.MarketRules) error {
	if rules.MinPrice.GreaterThan(decimal.Zero) && price.LessThan(rules.MinPrice) {
		return fmt.Errorf("price must be >= %s", rules.MinPrice)
	}
	if rules.MaxPrice.GreaterThan(decimal.Zero) && price.GreaterThan(rules.MaxPrice) {
		return fmt.Errorf("price must be <= %s", rules.MaxPrice)
	}
	if rules.TickSize.GreaterThan(decimal.Zero) && !isStepAligned(price, rules.TickSize) {
		return fmt.Errorf("price must align with tick size %s", rules.TickSize)
	}
	return nil
}

func validateQuantityRules(qty decimal.Decimal, rules ctypes.MarketRules) error {
	if rules.MinQuantity.GreaterThan(decimal.Zero) && qty.LessThan(rules.MinQuantity) {
		return fmt.Errorf("quantity must be >= %s", rules.MinQuantity)
	}
	if rules.MaxQuantity.GreaterThan(decimal.Zero) && qty.GreaterThan(rules.MaxQuantity) {
		return fmt.Errorf("quantity must be <= %s", rules.MaxQuantity)
	}
	if rules.LotSize.GreaterThan(decimal.Zero) && !isStepAligned(qty, rules.LotSize) {
		return fmt.Errorf("quantity must align with lot size %s", rules.LotSize)
	}
	return nil
}

func validateNotionalRules(price decimal.Decimal, qty decimal.Decimal, rules ctypes.MarketRules) error {
	notional := price.Mul(qty)
	if rules.MinNotional.GreaterThan(decimal.Zero) && notional.LessThan(rules.MinNotional) {
		return fmt.Errorf("notional must be >= %s", rules.MinNotional)
	}
	if rules.MaxNotional.GreaterThan(decimal.Zero) && notional.GreaterThan(rules.MaxNotional) {
		return fmt.Errorf("notional must be <= %s", rules.MaxNotional)
	}
	return nil
}

func isStepAligned(v decimal.Decimal, step decimal.Decimal) bool {
	if !step.GreaterThan(decimal.Zero) {
		return true
	}
	q := v.Div(step)
	return q.Equal(q.Truncate(0))
}

func defaultFeesByMarketType(mt ctypes.MarketType) (decimal.Decimal, decimal.Decimal) {
	switch mt {
	case ctypes.MarketTypeFuture:
		return decimal.RequireFromString("0.0002"), decimal.RequireFromString("0.0005")
	default:
		return decimal.RequireFromString("0.001"), decimal.RequireFromString("0.001")
	}
}

func (c *Connector) normalizeMarketPrecision(market *ctypes.Market) {
	if market == nil {
		return
	}
	if market.PricePrecision <= 0 {
		market.PricePrecision = precisionFromStep(market.Rules.TickSize)
	}
	if market.BaseAssetPrecision <= 0 {
		market.BaseAssetPrecision = precisionFromStep(market.Rules.LotSize)
	}
	if market.QuoteAssetPrecision <= 0 {
		// Quote precision falls back to tick-size precision for spot pairs.
		market.QuoteAssetPrecision = precisionFromStep(market.Rules.TickSize)
	}
}

func precisionFromStep(step decimal.Decimal) int {
	if !step.GreaterThan(decimal.Zero) {
		return 8
	}
	s := strings.TrimRight(strings.TrimRight(step.String(), "0"), ".")
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return 0
	}
	return len(s) - i - 1
}

func (c *Connector) CancelOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) error {
	beforeBal, beforePos := c.snapshotAccountState(symbol)
	c.state.mu.Lock()
	err := c.state.adapter.CancelOrder(ctx, c.accountID, toSimSymbol(symbol), orderId)
	c.state.mu.Unlock()
	if err != nil {
		return err
	}
	afterBal, afterPos := c.snapshotAccountState(symbol)
	msg := c.newOrderLifecycleMessage(&ctypes.Order{
		AccountID: c.accountID,
		Exchange:  c.exchange,
		Symbol:    symbol,
		OrderID:   ctypes.OrderId(orderId),
		Status:    ctypes.OrderStatusCanceled,
		UpdatedTs: time.Now().UTC(),
	})
	c.publishAccountMessage(msg)
	for _, em := range c.buildSnapshotDiffMessages(symbol, beforeBal, afterBal, beforePos, afterPos) {
		c.publishAccountMessage(em)
	}
	return nil
}

func (c *Connector) SetLeverage(ctx context.Context, symbol ctypes.Symbol, leverage int) (int, error) {
	if !symbol.IsValid() {
		return 0, fmt.Errorf("invalid symbol: %s", symbol)
	}
	if leverage <= 0 {
		return 0, fmt.Errorf("leverage must be > 0")
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.leverages[accountSymbolKey{accountID: c.accountID, symbol: toSimSymbol(symbol)}] = leverage
	return leverage, nil
}

func (c *Connector) GetLeverageBracket(ctx context.Context, symbol ctypes.Symbol, markPrice decimal.Decimal) (*ctypes.LeverageBracket, error) {
	if !symbol.IsValid() {
		return nil, fmt.Errorf("invalid symbol: %s", symbol)
	}
	maxLev := float32(c.currentLeverage(toSimSymbol(symbol)))
	if maxLev <= 0 {
		maxLev = 1
	}
	return &ctypes.LeverageBracket{
		Symbol: symbol,
		Brackets: []ctypes.Bracket{
			{
				Bracket:     1,
				MaxLeverage: maxLev,
				MinNotional: decimal.Zero,
				MaxNotional: decimal.RequireFromString("1000000000"),
				Mmr:         decimal.Zero,
				Cum:         decimal.Zero,
			},
		},
	}, nil
}

func (c *Connector) ensureInstrument(market *ctypes.Market) {
	if market == nil {
		return
	}
	ins := simcore.Instrument{
		Symbol:             toSimSymbol(market.Symbol),
		Kind:               marketTypeToInstrumentKind(market.Symbol.Type),
		Exchange:           c.exchange,
		Market:             market.Symbol.Type,
		Base:               simcore.Asset(market.Symbol.Base),
		Quote:              simcore.Asset(market.Symbol.Quote),
		PriceTick:          market.Rules.TickSize,
		QtyStep:            market.Rules.LotSize,
		MinQty:             market.Rules.MinQuantity,
		MinNotional:        market.Rules.MinNotional,
		ContractMultiplier: decimal.NewFromInt(1),
		MakerFeeBps:        10,
		TakerFeeBps:        10,
	}
	_ = c.state.ex.RegisterInstrument(&ins)
}

func toSimOrderBook(book *ctypes.OrderBook) simcore.OrderBook {
	out := simcore.OrderBook{
		Symbol: toSimSymbol(book.Symbol),
		Ts:     book.Ts,
		SeqId:  book.SeqId,
	}
	for _, bid := range book.Bids {
		out.Bids = append(out.Bids, simcore.OrderBookLevel{Price: bid.Price, Size: bid.Size})
	}
	for _, ask := range book.Asks {
		out.Asks = append(out.Asks, simcore.OrderBookLevel{Price: ask.Price, Size: ask.Size})
	}
	return out
}

func toTypesOrder(exchange ctypes.Exchange, od simcore.SimOrder) *ctypes.Order {
	return &ctypes.Order{
		AccountID:        od.AccountID,
		Exchange:         exchange,
		Symbol:           toTypesSymbol(od.Symbol),
		OrderID:          ctypes.OrderId(od.ID),
		ClientOrderID:    ctypes.OrderId(od.ClientOrderID),
		OrderType:        toTypesOrderType(od.OrderType),
		IsBuy:            od.Side == simcore.SideBuy,
		Price:            od.Price,
		OriginalQty:      od.QtyOriginal,
		ExecutedQty:      od.QtyFilled,
		AvgPrice:         od.AvgFillPrice,
		Status:           toTypesOrderStatus(od.Status),
		CreatedTs:        od.CreatedAt,
		UpdatedTs:        od.LastUpdatedAt,
		RejectReason:     od.RejectReason,
		ExecutedQuoteQty: od.QtyFilled.Mul(od.AvgFillPrice),
	}
}

func toSimSymbol(symbol ctypes.Symbol) simcore.Symbol { return simcore.Symbol(symbol.String()) }
func toTypesSymbol(symbol simcore.Symbol) ctypes.Symbol {
	s, _ := ctypes.ParseSymbol(string(symbol))
	return s
}

func toSimOrderType(tp ctypes.OrderType) simcore.OrderType {
	if tp == ctypes.OrderTypeLimit {
		return simcore.OrderTypeLimit
	}
	return simcore.OrderTypeMarket
}

func toTypesOrderType(tp simcore.OrderType) ctypes.OrderType {
	if tp == simcore.OrderTypeLimit {
		return ctypes.OrderTypeLimit
	}
	return ctypes.OrderTypeMarket
}

func toSimSide(isBuy bool) simcore.Side {
	if isBuy {
		return simcore.SideBuy
	}
	return simcore.SideSell
}

func toSimIntent(input mdtypes.PlaceOrderInput) simcore.ContractIntent {
	if lo.FromPtr(input.ReduceOnly) {
		return simcore.IntentClose
	}
	return simcore.IntentOpen
}

func marketTypeToInstrumentKind(mt ctypes.MarketType) simcore.InstrumentKind {
	if mt == ctypes.MarketTypeFuture {
		return simcore.KindPerp
	}
	return simcore.KindSpot
}

func (c *Connector) leverageLocked(sym simcore.Symbol) int {
	if v, ok := c.state.leverages[accountSymbolKey{accountID: c.accountID, symbol: sym}]; ok && v > 0 {
		return v
	}
	return 1
}

func (c *Connector) currentLeverage(sym simcore.Symbol) int {
	c.state.mu.RLock()
	defer c.state.mu.RUnlock()
	return c.leverageLocked(sym)
}

func (c *Connector) markFromTicker(symbol ctypes.Symbol) (*ctypes.MarkPrice, bool) {
	c.state.mu.RLock()
	tk, ok := c.state.ticker.Get(toSimSymbol(symbol))
	c.state.mu.RUnlock()
	if !ok {
		return nil, false
	}
	m := tk.Mark
	if !m.GreaterThan(decimal.Zero) {
		m = tk.Last
	}
	if !m.GreaterThan(decimal.Zero) {
		return nil, false
	}
	return &ctypes.MarkPrice{
		Exchange:  c.exchange,
		Symbol:    symbol,
		MarkPrice: m,
		Ts:        tk.Ts,
	}, true
}

func (c *Connector) publicMarkPriceFallback(ctx context.Context, symbol ctypes.Symbol) (*ctypes.MarkPrice, error) {
	price, err := c.state.publicConn.Price(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if price == nil {
		return nil, nil
	}
	return &ctypes.MarkPrice{
		Exchange:  c.exchange,
		Symbol:    symbol,
		MarkPrice: price.Price,
		Ts:        price.Ts,
	}, nil
}

func (c *Connector) fetchMarkPrices(ctx context.Context) ([]*ctypes.MarkPrice, error) {
	c.state.mu.RLock()
	symbols := make([]simcore.Symbol, 0, len(c.state.depths))
	for sym := range c.state.depths {
		symbols = append(symbols, sym)
	}
	c.state.mu.RUnlock()
	out := make([]*ctypes.MarkPrice, 0, len(symbols))
	for _, sym := range symbols {
		typed := toTypesSymbol(sym)
		if !typed.IsValid() {
			continue
		}
		if mp, ok := c.markFromTicker(typed); ok {
			out = append(out, mp)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	return []*ctypes.MarkPrice{}, nil
}

func (c *Connector) computeOpenInterest(symbol ctypes.Symbol) decimal.Decimal {
	c.state.mu.RLock()
	defer c.state.mu.RUnlock()
	pos, ok := c.state.adapter.Position(context.Background(), c.accountID, toSimSymbol(symbol))
	if !ok {
		return decimal.Zero
	}
	return pos.Qty.Abs()
}

func (c *Connector) resolveMarkPriceLocked(symbol ctypes.Symbol) decimal.Decimal {
	tk, ok := c.state.ticker.Get(toSimSymbol(symbol))
	if !ok {
		return decimal.Zero
	}
	if tk.Mark.GreaterThan(decimal.Zero) {
		return tk.Mark
	}
	return tk.Last
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (c *Connector) nextEventID() string {
	return simcore.GenerateCompactID(c.accountID)
}

func (c *Connector) publishAccountMessage(msg *ctypes.Message) {
	if msg == nil {
		return
	}
	c.subMu.Lock()
	defer c.subMu.Unlock()
	for ch := range c.accountSubs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (c *Connector) snapshotAccountState(symbol ctypes.Symbol) (map[simcore.BalanceKey]decimal.Decimal, simcore.Position) {
	c.state.mu.RLock()
	defer c.state.mu.RUnlock()
	bal := c.state.adapter.Balance(context.Background(), c.accountID)
	pos, _ := c.state.adapter.Position(context.Background(), c.accountID, toSimSymbol(symbol))
	return bal, pos
}

func (c *Connector) buildOrderAndSnapshotMessages(symbol ctypes.Symbol, beforeBal, afterBal map[simcore.BalanceKey]decimal.Decimal, beforePos, afterPos simcore.Position, res *simcore.PlaceOrderResult) []*ctypes.Message {
	out := make([]*ctypes.Message, 0)
	if res != nil {
		if len(res.Fills) == 0 {
			out = append(out, c.newOrderLifecycleMessage(toTypesOrder(c.exchange, res.Order)))
		} else {
			for _, f := range res.Fills {
				out = append(out, c.newOrderFillMessage(symbol, res.Order, f))
			}
		}
	}
	out = append(out, c.buildSnapshotDiffMessages(symbol, beforeBal, afterBal, beforePos, afterPos)...)
	return out
}

func (c *Connector) buildSnapshotDiffMessages(symbol ctypes.Symbol, beforeBal, afterBal map[simcore.BalanceKey]decimal.Decimal, beforePos, afterPos simcore.Position) []*ctypes.Message {
	now := time.Now().UTC()
	out := make([]*ctypes.Message, 0)
	changedAssets := make([]*ctypes.AssetEvent, 0)
	assetSeen := map[simcore.BalanceKey]struct{}{}
	for k, v := range beforeBal {
		assetSeen[k] = struct{}{}
		if !afterBal[k].Equal(v) {
			b := afterBal[k]
			changedAssets = append(changedAssets, &ctypes.AssetEvent{
				WalletType: k.Wallet,
				Code:       string(k.Asset),
				Balance:    &b,
				Locked:     lo.ToPtr(decimal.Zero),
				UpdatedTs:  now,
			})
		}
	}
	for k, v := range afterBal {
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
	posChanged := !beforePos.Qty.Equal(afterPos.Qty) || !beforePos.EntryPrice.Equal(afterPos.EntryPrice) || beforePos.Leverage != afterPos.Leverage
	if posChanged {
		side := ctypes.PositionSideLong
		amount := afterPos.Qty
		if afterPos.Qty.Sign() < 0 {
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
					EntryPrice:    afterPos.EntryPrice,
					InitialMargin: afterPos.UsedMargin,
					Leverage:      int(afterPos.Leverage),
					UpdatedTs:     now,
				},
			},
		}, now))
	}
	return out
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

func (c *Connector) newOrderFillMessage(symbol ctypes.Symbol, od simcore.SimOrder, fill simcore.Fill) *ctypes.Message {
	ts := time.Now().UTC()
	order := toTypesOrder(c.exchange, od)
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

func (c *Connector) buildMakerMatchMessages(symbol ctypes.Symbol, events []simcore.MatchEvent, beforeBal, afterBal map[simcore.BalanceKey]decimal.Decimal, beforePos, afterPos simcore.Position) []*ctypes.Message {
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
		out = append(out, c.buildSnapshotDiffMessages(symbol, beforeBal, afterBal, beforePos, afterPos)...)
	}
	return out
}

func (c *Connector) mustEventMetaJSON(ts time.Time) string {
	payload, _ := json.Marshal(map[string]any{
		"eventId": c.nextEventID(),
		"ts":      ts.UnixMilli(),
		"source":  "simulate",
	})
	return string(payload)
}

func toTypesOrderStatus(st simcore.OrderStatus) ctypes.OrderStatus {
	switch st {
	case simcore.OrderStatusNew:
		return ctypes.OrderStatusNew
	case simcore.OrderStatusPartiallyFilled:
		return ctypes.OrderStatusPartialDone
	case simcore.OrderStatusFilled:
		return ctypes.OrderStatusDone
	case simcore.OrderStatusCanceled:
		return ctypes.OrderStatusCanceled
	case simcore.OrderStatusRejected:
		return ctypes.OrderStatusRejected
	default:
		return ctypes.OrderStatusPending
	}
}
