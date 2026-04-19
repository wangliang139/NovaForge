package simulate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

const DefaultPrecision int = 8

// DefaultSimulateLeverage is used when no per-symbol leverage was persisted or configured in-engine.
const DefaultSimulateLeverage int = 10

// Connector implements mdtypes.Connector for paper trading (see VenueRuntime).
type Connector struct {
	exchange  ctypes.Exchange
	accountID string
	rt        *VenueRuntime

	subMu       sync.Mutex
	accountSubs map[chan *ctypes.Message]struct{}
}

var _ mdtypes.Connector = (*Connector)(nil)

// New constructs a connector and registers it on the venue runtime.
func New(exchange ctypes.Exchange, account *mdtypes.ApiAccount) (*Connector, error) {
	if !exchange.IsValid() {
		return nil, fmt.Errorf("simulate: invalid exchange: %s", exchange)
	}
	if account == nil || account.ID == "" {
		return nil, fmt.Errorf("simulate: requires account id")
	}
	rt, err := getOrCreateVenue(exchange)
	if err != nil {
		return nil, err
	}
	c := &Connector{
		exchange:    exchange,
		accountID:   account.ID,
		rt:          rt,
		accountSubs: make(map[chan *ctypes.Message]struct{}),
	}
	rt.registerConn(c)
	return c, nil
}

func (c *Connector) Exchange() ctypes.Exchange { return c.exchange }

func (c *Connector) IsPrivate() bool { return true }

func (c *Connector) Supports(selector ctypes.StreamSelector) bool {
	switch selector.Stream {
	case ctypes.StreamTypeTicker, ctypes.StreamTypeTrade, ctypes.StreamTypeDepth,
		ctypes.StreamTypeKline, ctypes.StreamTypeMarkPrice:
		return c.rt.Public.Supports(selector)
	case ctypes.StreamTypeAccountRaw, ctypes.StreamTypeAccount:
		return true
	default:
		return false
	}
}

func (c *Connector) Subscribe(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	switch selector.Stream {
	case ctypes.StreamTypeDepth, ctypes.StreamTypeTicker, ctypes.StreamTypeMarkPrice,
		ctypes.StreamTypeTrade, ctypes.StreamTypeKline:
		if err := validateSimulatePublicSelector(selector); err != nil {
			return nil, err
		}
		if !c.rt.Public.Supports(selector) {
			return nil, fmt.Errorf("simulate: public connector does not support stream %s", selector.Stream)
		}
		hub := c.rt.getOrCreateStreamHub(selector)
		return hub.attachListener(ctx)
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
		return nil, fmt.Errorf("simulate: unsupported selector: %s", selector.Stream)
	}
}

// Close unregisters this connector from the venue.
func (c *Connector) Close() {
	c.rt.unregisterConn(c)
}

// SetPositionMode switches default perp mode for this account (requires flat book per symbol; use Engine.Ledger().SetPerpMode).
func (c *Connector) SetPositionMode(mode PositionMode) {
	c.rt.Engine.SetAccountPositionMode(c.accountID, mode)
}

func (c *Connector) GetMarkets(ctx context.Context, tps []ctypes.MarketType) ([]*ctypes.Market, error) {
	return c.rt.Public.GetMarkets(ctx, tps)
}

func (c *Connector) GetMarket(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Market, error) {
	return c.rt.Public.GetMarket(ctx, symbol)
}

func (c *Connector) Prices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.Price, error) {
	return c.rt.Public.Prices(ctx, marketType)
}

func (c *Connector) Price(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Price, error) {
	return c.rt.Public.Price(ctx, symbol)
}

func (c *Connector) BookPrices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.BookPrice, error) {
	return c.rt.Public.BookPrices(ctx, marketType)
}

func (c *Connector) BookPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.BookPrice, error) {
	return c.rt.Public.BookPrice(ctx, symbol)
}

func (c *Connector) MarkPrices(ctx context.Context) ([]*ctypes.MarkPrice, error) {
	return c.rt.Public.MarkPrices(ctx)
}

func (c *Connector) MarkPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.MarkPrice, error) {
	return c.rt.Public.MarkPrice(ctx, symbol)
}

func (c *Connector) IndexPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexPrice, error) {
	return c.rt.Public.IndexPrice(ctx, symbol)
}

func (c *Connector) IndexComponent(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexComponent, error) {
	return c.rt.Public.IndexComponent(ctx, symbol)
}

func (c *Connector) Ticker(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Ticker, error) {
	return c.rt.Public.Ticker(ctx, symbol)
}

func (c *Connector) Trades(ctx context.Context, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	return c.rt.Public.Trades(ctx, symbol, limit)
}

func (c *Connector) Depth(ctx context.Context, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error) {
	return c.rt.Public.Depth(ctx, symbol, limit)
}

func (c *Connector) Klines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	return c.rt.Public.Klines(ctx, symbol, interval, limit)
}

func (c *Connector) HisKlines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, startTs, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	return c.rt.Public.HisKlines(ctx, symbol, interval, startTs, endTs, limit)
}

func (c *Connector) FundingRate(ctx context.Context, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	return c.rt.Public.FundingRate(ctx, symbol)
}

func (c *Connector) HisFundingRates(ctx context.Context, symbol ctypes.Symbol, startTs, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	return c.rt.Public.HisFundingRates(ctx, symbol, startTs, endTs, limit)
}

func (c *Connector) OpenInterest(ctx context.Context, symbol ctypes.Symbol) (decimal.Decimal, error) {
	return c.rt.Public.OpenInterest(ctx, symbol)
}

func (c *Connector) Account(ctx context.Context) (*ctypes.AccountBo, error) {
	return &ctypes.AccountBo{
		Uid:             c.accountID,
		Exchange:        c.exchange,
		IsSpotEnabled:   true,
		IsFutureEnabled: true,
	}, nil
}

func (c *Connector) Balance(ctx context.Context) (*ctypes.Balance, error) {
	_ = ctx
	balMap := c.rt.Engine.Balances(c.accountID)
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

func (c *Connector) resolveMarkPrice(symbol ctypes.Symbol) decimal.Decimal {
	tk, ok := c.rt.Quotes.Get(Symbol(symbol.String()))
	if ok {
		if tk.Mark.GreaterThan(decimal.Zero) {
			return tk.Mark
		}
		if tk.Last.GreaterThan(decimal.Zero) {
			return tk.Last
		}
	}
	return decimal.Zero
}

func (c *Connector) Positions(ctx context.Context, mt *ctypes.MarketType) ([]*ctypes.Position, error) {
	_ = ctx
	if mt != nil && *mt == ctypes.MarketTypeSpot {
		return []*ctypes.Position{}, nil
	}
	out := make([]*ctypes.Position, 0)
	for _, sym := range c.rt.Engine.AllSymbols() {
		typedSym := toTypesSymbol(sym)
		if !typedSym.IsValid() || typedSym.Type != ctypes.MarketTypeFuture {
			continue
		}
		mode := c.rt.Engine.AccountPositionMode(c.accountID)
		snap, _ := c.rt.Engine.Ledger().GetPerpSlot(c.accountID, sym)
		if mode == PositionModeHedge {
			mark := c.resolveMarkPrice(typedSym)
			if snap.Long.Qty.Sign() > 0 {
				out = append(out, c.ctypePositionLeg(typedSym, ctypes.PositionSideLong, snap.Long, mark))
			}
			if snap.Short.Qty.Sign() > 0 {
				out = append(out, c.ctypePositionLeg(typedSym, ctypes.PositionSideShort, snap.Short, mark))
			}
			continue
		}
		pos, ok := c.rt.Engine.NetPosition(c.accountID, sym)
		if !ok || pos.Qty.IsZero() {
			continue
		}
		side := ctypes.PositionSideLong
		amount := pos.Qty
		if pos.Qty.Sign() < 0 {
			side = ctypes.PositionSideShort
			amount = pos.Qty.Abs()
		}
		mark := c.resolveMarkPrice(typedSym)
		notional := amount.Mul(mark)
		lev := int(pos.Leverage)
		if lev <= 0 {
			lev = c.rt.Engine.Leverage(c.accountID, sym)
		}
		if lev <= 0 {
			lev = DefaultSimulateLeverage
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

func (c *Connector) ctypePositionLeg(typedSym ctypes.Symbol, side ctypes.PositionSide, leg PerpLeg, mark decimal.Decimal) *ctypes.Position {
	lev := int(leg.Leverage)
	if lev <= 0 {
		lev = c.rt.Engine.Leverage(c.accountID, Symbol(typedSym.String()))
	}
	if lev <= 0 {
		lev = DefaultSimulateLeverage
	}
	return &ctypes.Position{
		AccountID:     c.accountID,
		Exchange:      c.exchange,
		Symbol:        typedSym,
		Side:          side,
		Isolated:      true,
		Amount:        leg.Qty,
		EntryPrice:    leg.EntryPrice,
		MarkPrice:     mark,
		Notional:      leg.Qty.Mul(mark),
		Leverage:      lev,
		InitialMargin: leg.UsedMargin,
		MaintMargin:   decimal.Zero,
		UpdatedTs:     time.Now().UTC(),
	}
}

func (c *Connector) SymbolConfig(ctx context.Context, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	market, err := c.GetMarket(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if market == nil {
		return nil, nil
	}
	bracket, err := c.GetLeverageBracket(ctx, symbol, decimal.Zero)
	if err != nil {
		return nil, err
	}
	if bracket == nil {
		return nil, nil
	}
	leverage := c.rt.Engine.Leverage(c.accountID, Symbol(symbol.String()))
	if leverage <= 0 {
		leverage = DefaultSimulateLeverage
	}
	maker, taker := defaultFeesByMarketType(symbol.Type)
	cfg := &ctypes.SymbolConfig{
		Exchange:              c.exchange,
		Symbol:                symbol,
		Market:                *market,
		MakerCommission:       maker,
		TakerCommission:       taker,
		IsolatedMarginEnabled: false,
		CrossMarginEnabled:    true,
		IsAutoAddMargin:       false,
		CrossLeverage:         [2]int{leverage, leverage},
		MaxNotionalValue:      decimal.Zero,
	}

	if cfg.Market.PricePrecision <= 0 {
		cfg.Market.PricePrecision = precisionFromStep(cfg.Market.Rules.TickSize)
	}
	if cfg.Market.BaseAssetPrecision <= 0 {
		cfg.Market.BaseAssetPrecision = precisionFromStep(cfg.Market.Rules.LotSize)
	}
	if cfg.Market.QuoteAssetPrecision <= 0 {
		cfg.Market.QuoteAssetPrecision = precisionFromStep(cfg.Market.Rules.TickSize)
	}

	return cfg, nil
}

func (c *Connector) GetOrders(ctx context.Context, symbol *ctypes.Symbol) ([]*ctypes.Order, error) {
	_ = ctx
	var orders []Order
	if symbol == nil {
		orders = c.rt.Engine.ListAllOpenOrders(c.accountID)
	} else {
		if !symbol.IsValid() {
			return []*ctypes.Order{}, nil
		}
		orders = c.rt.Engine.ListOpenOrders(c.accountID, Symbol(symbol.String()))
	}
	out := make([]*ctypes.Order, 0, len(orders))
	for i := range orders {
		out = append(out, toTypesOrder(c.exchange, &orders[i]))
	}
	return out, nil
}

func (c *Connector) GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	_ = ctx
	order, ok := c.rt.Engine.GetOrder(c.accountID, Symbol(symbol.String()), orderId)
	if !ok {
		return nil, nil
	}
	return toTypesOrder(c.exchange, &order), nil
}

func (c *Connector) CalcOrderFee(ctx context.Context, order ctypes.Order) (*decimal.Decimal, *string, error) {
	return nil, nil, nil
}

func (c *Connector) PlaceOrder(ctx context.Context, input mdtypes.PlaceOrderInput) (*mdtypes.PlaceOrderResult, error) {
	if err := ValidatePlaceOrderInputBasic(input); err != nil {
		return nil, err
	}
	market, err := c.GetMarket(ctx, input.Symbol)
	if err != nil {
		return nil, err
	}
	if market == nil {
		return nil, fmt.Errorf("simulate: unknown market")
	}
	if err := ValidatePlaceOrderByMarketRules(ctx, input, market); err != nil {
		return nil, err
	}
	if err := c.ensureSymbolInitialized(ctx, input.Symbol); err != nil {
		return nil, err
	}

	req := placeOrderRequestFromInput(c, input, market)
	if req.OrderID == "" {
		req.OrderID = GenerateCompactID(c.accountID)
	}
	job := placeOrderJob{c: c, symbol: input.Symbol, req: req}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case c.rt.placeOrderCh <- job:
		return &mdtypes.PlaceOrderResult{
			OrderID:       ctypes.OrderId(req.OrderID),
			ClientOrderID: ctypes.OrderId(req.ClientOrderID),
			Status:        ctypes.OrderStatusNew,
		}, nil
	default:
		return nil, fmt.Errorf("simulate: place order queue is full")
	}
}

func (c *Connector) CancelOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) error {
	return c.rt.Engine.CancelOrder(ctx, c.accountID, Symbol(symbol.String()), orderId)
}

func (c *Connector) SetLeverage(ctx context.Context, symbol ctypes.Symbol, leverage int) (int, error) {
	_ = ctx
	if leverage < 1 {
		return 0, fmt.Errorf("simulate: invalid leverage")
	}
	c.rt.Engine.SetLeverage(c.accountID, Symbol(symbol.String()), leverage)
	return leverage, nil
}

// SyncSymbolLeveragesFromPersistence merges per-symbol leverage from DB-backed flat legs (qty=0 rows).
// Keys are persisted symbol strings; only valid futures symbols are applied. Does not remove existing entries.
func (c *Connector) SyncSymbolLeveragesFromPersistence(bySymbolString map[string]int) {
	m := make(map[Symbol]int, len(bySymbolString))
	for sStr, lev := range bySymbolString {
		if lev <= 0 {
			continue
		}
		sym, err := ctypes.ParseSymbol(sStr)
		if err != nil || !sym.IsValid() || sym.Type != ctypes.MarketTypeFuture {
			continue
		}
		m[Symbol(sym.String())] = lev
	}
	c.rt.Engine.MergeSymbolLeverages(c.accountID, m)
}

func (c *Connector) GetLeverageBracket(ctx context.Context, symbol ctypes.Symbol, markPrice decimal.Decimal) (*ctypes.LeverageBracket, error) {
	_ = ctx
	_ = markPrice
	if !symbol.IsValid() {
		return nil, fmt.Errorf("simulate: invalid symbol")
	}
	maxLev := float32(c.rt.Engine.Leverage(c.accountID, Symbol(symbol.String())))
	if maxLev <= 0 {
		maxLev = float32(DefaultSimulateLeverage)
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

func (c *Connector) nextEventID() string {
	return GenerateCompactID(c.accountID)
}

func (c *Connector) mustEventMetaJSON(ts time.Time) string {
	payload, _ := json.Marshal(map[string]any{
		"eventId": c.nextEventID(),
		"ts":      ts.UnixMilli(),
		"source":  "simulate",
	})
	return string(payload)
}

func defaultFeesByMarketType(mt ctypes.MarketType) (decimal.Decimal, decimal.Decimal) {
	switch mt {
	case ctypes.MarketTypeFuture:
		return decimal.NewFromFloat(0.0002), decimal.NewFromFloat(0.0004)
	default:
		return decimal.NewFromFloat(0.001), decimal.NewFromFloat(0.001)
	}
}

func precisionFromStep(step decimal.Decimal) int {
	if !step.GreaterThan(decimal.Zero) {
		return DefaultPrecision
	}
	s := strings.TrimRight(strings.TrimRight(step.String(), "0"), ".")
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return 0
	}
	return len(s) - i - 1
}

func (c *Connector) ensureInstrument(market *ctypes.Market) {
	ins := &Instrument{
		Symbol:             toPaperSymbol(market.Symbol),
		Kind:               marketTypeToInstrumentKind(market.Symbol.Type),
		Exchange:           c.exchange,
		Market:             market.Symbol.Type,
		Base:               Asset(market.Symbol.Base),
		Quote:              Asset(market.Symbol.Quote),
		PriceTick:          market.Rules.TickSize,
		QtyStep:            market.Rules.LotSize,
		MinQty:             market.Rules.MinQuantity,
		MinNotional:        market.Rules.MinNotional,
		ContractMultiplier: decimal.NewFromInt(1),
		MakerFeeBps:        10,
		TakerFeeBps:        10,
	}
	_ = c.rt.Engine.RegisterInstrument(ins)
}

func (c *Connector) ensureSymbolInitialized(ctx context.Context, symbol ctypes.Symbol) error {
	if c.rt.SymbolSimReady(symbol) {
		return nil
	}
	market, err := c.GetMarket(ctx, symbol)
	if err != nil {
		return err
	}
	if market == nil {
		return fmt.Errorf("simulate: unknown market %s", symbol)
	}
	c.ensureInstrument(market)
	if err := c.syncMarketDepth(ctx, symbol, true); err != nil {
		return err
	}
	if c.rt.tryMarkSymbolSimReady(symbol) {
		c.rt.onSymbolSimReady(symbol)
	}
	return nil
}

func (c *Connector) syncMarketDepth(ctx context.Context, symbol ctypes.Symbol, emptyFallback bool) error {
	sym := toPaperSymbol(symbol)
	d, ok := c.rt.Engine.Depth(sym)
	depthReady := ok && d != nil && d.LastSeqID() > 0
	if depthReady {
		return nil
	}
	snapshot, err := c.rt.Public.Depth(ctx, symbol, 500)
	if err != nil {
		return err
	}
	if snapshot != nil && snapshot.Symbol.IsValid() {
		_, _ = c.rt.Engine.ApplyDepthBook(snapshot, false)
	}
	d, ok = c.rt.Engine.Depth(sym)
	depthReady = ok && d != nil && d.LastSeqID() > 0
	if !depthReady && emptyFallback {
		_, _ = c.rt.Engine.ApplyDepthBook(&ctypes.OrderBook{
			Symbol: symbol,
			SeqId:  1,
			Ts:     time.Now().UTC(),
		}, false)
	}
	c.ensureDepthStreamStarted(symbol)
	if symbol.Type == ctypes.MarketTypeFuture {
		if mp, err := c.rt.Public.MarkPrice(ctx, symbol); err == nil && mp != nil && mp.MarkPrice.GreaterThan(decimal.Zero) {
			c.ingestMarkPrice(mp)
		}
		c.ensureMarkPriceStreamStarted(symbol)
	}
	return nil
}

func (c *Connector) ensureDepthStreamStarted(symbol ctypes.Symbol) {
	if !symbol.IsValid() {
		return
	}
	sym := symbol
	sel := ctypes.StreamSelector{Stream: ctypes.StreamTypeDepth, Symbol: &sym}
	if !c.rt.Public.Supports(sel) {
		return
	}
	hub := c.rt.getOrCreateStreamHub(sel)
	hub.setPinned()
	hub.ensureRunning()
}

func (c *Connector) ensureMarkPriceStreamStarted(symbol ctypes.Symbol) {
	if symbol.Type != ctypes.MarketTypeFuture || !symbol.IsValid() {
		return
	}
	sym := symbol
	sel := ctypes.StreamSelector{Stream: ctypes.StreamTypeMarkPrice, Symbol: &sym}
	if !c.rt.Public.Supports(sel) {
		return
	}
	hub := c.rt.getOrCreateStreamHub(sel)
	hub.setPinned()
	hub.ensureRunning()
}

func (c *Connector) ingestMarkPrice(mp *ctypes.MarkPrice) {
	if mp == nil {
		return
	}
	c.rt.dispatchMarkPricePayload(mp)
}

func (c *Connector) tryLiquidateAfterMark(symbol ctypes.Symbol, mark decimal.Decimal) {
	if symbol.Type != ctypes.MarketTypeFuture || !mark.GreaterThan(decimal.Zero) {
		return
	}
	sym := toPaperSymbol(symbol)
	before := c.rt.Engine.AccountSnapshot(c.accountID, sym)
	results := c.rt.Liq.OnMark(c.accountID, sym, mark, nil)
	snap := before
	for _, res := range results {
		if res == nil {
			continue
		}
		after := c.rt.Engine.AccountSnapshot(c.accountID, sym)
		c.publishPlaceOrderOutcome(symbol, snap, after, res)
		snap = after
	}
}

// SeedAccountBalances replaces wallet balances from persisted assets (virtual accounts).
func (c *Connector) SeedAccountBalances(bals map[ctypes.WalletType]map[Asset]decimal.Decimal) error {
	c.rt.Engine.InitBalances(c.accountID, bals)
	return nil
}

// SeedAccountPositions seeds perp slots from DB snapshots (one-way net or hedge legs).
// Callers must pass each leg separately in hedge mode (same Symbol, distinct Side); a map keyed only by Symbol would drop a leg.
func (c *Connector) SeedAccountPositions(positions []*ctypes.Position) error {
	mode := c.rt.Engine.AccountPositionMode(c.accountID)
	for _, p := range positions {
		if p == nil {
			continue
		}
		sym := p.Symbol
		if !sym.IsValid() || sym.Type != ctypes.MarketTypeFuture {
			continue
		}
		psym := Symbol(sym.String())
		lev := int32(p.Leverage)
		if lev <= 0 {
			lev = int32(c.rt.Engine.Leverage(c.accountID, psym))
		}
		if lev <= 0 {
			lev = int32(DefaultSimulateLeverage)
		}
		if mode == PositionModeHedge {
			if !p.Side.Valid() {
				continue
			}
			leg := PerpLeg{
				Qty:        p.Amount,
				EntryPrice: p.EntryPrice,
				UsedMargin: p.InitialMargin,
				Leverage:   lev,
			}
			c.rt.Engine.SeedLedgerHedgeLeg(c.accountID, psym, p.Side, leg)
			continue
		}
		var qty decimal.Decimal
		switch p.Side {
		case ctypes.PositionSideLong:
			qty = p.Amount
		case ctypes.PositionSideShort:
			qty = p.Amount.Neg()
		default:
			continue
		}
		if qty.IsZero() {
			continue
		}
		net := Position{
			Qty:        qty,
			EntryPrice: p.EntryPrice,
			UsedMargin: p.InitialMargin,
			Leverage:   lev,
		}
		c.rt.Engine.SeedLedgerOneWayNet(c.accountID, psym, net)
	}
	return nil
}

// SeedOpenOrders hydrates open orders from DB: limit via resting book, market via immediate match (SeedOpenOrder).
func (c *Connector) SeedOpenOrders(orders []*ctypes.Order) error {
	ctx := context.Background()
	for _, od := range orders {
		if od == nil || !od.Symbol.IsValid() {
			continue
		}
		switch od.OrderType {
		case ctypes.OrderTypeLimit, ctypes.OrderTypeMarket:
		default:
			continue
		}
		switch od.Status {
		case ctypes.OrderStatusNew, ctypes.OrderStatusPartialDone, ctypes.OrderStatusWorking, ctypes.OrderStatusPending:
		default:
			continue
		}
		rem := od.OriginalQty.Sub(od.ExecutedQty)
		if rem.Sign() <= 0 {
			continue
		}
		market, err := c.GetMarket(ctx, od.Symbol)
		if err != nil {
			return fmt.Errorf("simulate: seed order market %s: %w", od.Symbol, err)
		}
		if market == nil {
			return fmt.Errorf("simulate: seed order unknown market %s", od.Symbol)
		}
		if err := c.ensureSymbolInitialized(ctx, od.Symbol); err != nil {
			return fmt.Errorf("simulate: seed order %s: %w", od.Symbol, err)
		}

		po := orderFromTypes(c, od, rem)
		var onNew PlaceOrderNewFunc
		var onComplete PlaceOrderCompleteFunc
		if od.OrderType == ctypes.OrderTypeMarket {
			onNew = func(o Order) { c.publishOrderAcceptedNew(od.Symbol, o) }
			onComplete = func(before, after AccountSnapshot, r *PlaceOrderResult) {
				c.publishPlaceOrderOutcome(od.Symbol, before, after, r)
			}
		}
		if err := c.rt.Engine.SeedOpenOrder(c.accountID, po, onNew, onComplete); err != nil {
			return err
		}
	}
	return nil
}
