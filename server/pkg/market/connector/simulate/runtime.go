package simulate

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/binance"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/okx"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// placeOrderQueueCap bounds async user orders per venue; when full, PlaceOrder returns an error.
const placeOrderQueueCap = 1024

type placeOrderJob struct {
	c   *Connector
	req PlaceOrderRequest
}

// VenueRuntime holds exchange-wide paper state: engine, public feed, stream hubs, liquidation.
type VenueRuntime struct {
	Exchange ctypes.Exchange
	Engine   *Engine
	Quotes   *QuoteCache
	Mark     *MarkIndexService
	Liq      *LiquidationEngine
	Public   mdtypes.Connector

	streamHubMu sync.Mutex
	streamHubs  map[string]*publicStreamHub

	connsMu sync.RWMutex
	conns   map[string][]*Connector // accountID -> connectors

	// symbolSimReady: symbol.String() -> ensureSymbolInitialized completed for this venue
	// (instrument + depth sync + streams). Until set, public depth updates do not match
	// resting orders and mark-price does not run liquidation.
	simReadyMu     sync.RWMutex
	symbolSimReady map[string]struct{}

	placeOrderCh chan placeOrderJob

	accountPublishCh       chan AccountEvent
	accountPublishQueueCap int

	// Perp funding: min-heap of scheduled settlements (Binance/OKX-aligned), one wall timer to next deadline.
	fundingWakeMu sync.Mutex
	fundingMu     sync.Mutex
	fundingHeap   *fundingHeap
	fundingTimer  *time.Timer
	fundingNext   map[Symbol]time.Time // canonical next fire time per paper symbol (lazy heap eviction)
}

var (
	venueMu sync.Mutex
	venues  = make(map[ctypes.Exchange]*VenueRuntime)
)

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
		return nil, fmt.Errorf("simulate: unsupported exchange: %s", exchange)
	}
}

func getOrCreateVenue(ex ctypes.Exchange) (*VenueRuntime, error) {
	venueMu.Lock()
	defer venueMu.Unlock()
	if v, ok := venues[ex]; ok {
		return v, nil
	}
	pub, err := newPublicConnector(ex)
	if err != nil {
		return nil, err
	}
	eng := NewEngine()
	q := NewQuoteCache()
	rt := &VenueRuntime{
		Exchange:         ex,
		Engine:           eng,
		Quotes:           q,
		Mark:             NewMarkIndexService(q),
		Liq:              NewLiquidationEngine(eng),
		Public:           pub,
		streamHubs:       make(map[string]*publicStreamHub),
		conns:            make(map[string][]*Connector),
		symbolSimReady:   make(map[string]struct{}),
		placeOrderCh:     make(chan placeOrderJob, placeOrderQueueCap),
		accountPublishCh: make(chan AccountEvent, 1024),
	}
	eng.WithRuntime(rt)
	venues[ex] = rt
	go rt.runPlaceOrderLoop()
	go rt.runAccountPublishLoop()
	return rt, nil
}

func (rt *VenueRuntime) enqueueAccountPublish(event AccountEvent) {
	select {
	case rt.accountPublishCh <- event:
	default:
		log.Error().Msg("account publish channel is full")
	}
}

func (rt *VenueRuntime) runAccountPublishLoop() {
	for event := range rt.accountPublishCh {
		aid := accountIDFromAccountEvent(event)
		msg := rt.accountEventToMessage(event)
		if msg == nil || aid == "" {
			continue
		}
		rt.connsMu.RLock()
		conns := rt.conns[aid]
		rt.connsMu.RUnlock()
		for _, conn := range conns {
			conn.PublishAccountMessage(msg)
		}
	}
}

func (rt *VenueRuntime) runPlaceOrderLoop() {
	for job := range rt.placeOrderCh {
		ctx := context.Background()
		rt.Engine.PlaceOrder(ctx, job.req)
	}
}

func (rt *VenueRuntime) registerConn(c *Connector) {
	rt.connsMu.Lock()
	defer rt.connsMu.Unlock()
	rt.conns[c.accountID] = append(rt.conns[c.accountID], c)
}

func (rt *VenueRuntime) unregisterConn(c *Connector) {
	rt.connsMu.Lock()
	defer rt.connsMu.Unlock()
	conns := rt.conns[c.accountID]
	for i, conn := range conns {
		if conn == c {
			next := append(conns[:i], conns[i+1:]...)
			if len(next) == 0 {
				delete(rt.conns, c.accountID)
			} else {
				rt.conns[c.accountID] = next
			}
			return
		}
	}
}

// tryMarkSymbolSimReady records that ensureSymbolInitialized finished for sym.
// It returns true the first time sym becomes ready (caller may run post-ready hooks).
func (rt *VenueRuntime) tryMarkSymbolSimReady(sym ctypes.Symbol) bool {
	if !sym.IsValid() {
		return false
	}
	key := sym.String()
	rt.simReadyMu.Lock()
	defer rt.simReadyMu.Unlock()
	if rt.symbolSimReady == nil {
		rt.symbolSimReady = make(map[string]struct{})
	}
	if _, ok := rt.symbolSimReady[key]; ok {
		return false
	}
	rt.symbolSimReady[key] = struct{}{}
	return true
}

// SymbolSimReady reports whether sym has completed ensureSymbolInitialized on this venue.
func (rt *VenueRuntime) SymbolSimReady(sym ctypes.Symbol) bool {
	if !sym.IsValid() {
		return false
	}
	rt.simReadyMu.RLock()
	defer rt.simReadyMu.RUnlock()
	_, ok := rt.symbolSimReady[sym.String()]
	return ok
}

func (rt *VenueRuntime) removeStreamHub(key string) {
	rt.streamHubMu.Lock()
	defer rt.streamHubMu.Unlock()
	delete(rt.streamHubs, key)
}

func (rt *VenueRuntime) getOrCreateStreamHub(sel ctypes.StreamSelector) *publicStreamHub {
	key := sel.Key()
	rt.streamHubMu.Lock()
	defer rt.streamHubMu.Unlock()
	if rt.streamHubs == nil {
		rt.streamHubs = make(map[string]*publicStreamHub)
	}
	if h, ok := rt.streamHubs[key]; ok {
		return h
	}
	h := newPublicStreamHub(rt, sel, key)
	rt.streamHubs[key] = h
	return h
}

func accountIDFromAccountEvent(ev AccountEvent) string {
	if ev.accountID != "" {
		return ev.accountID
	}
	if ev.order != nil {
		return ev.order.AccountID
	}
	return ""
}

func simulateAccountEventMeta(accountID string, ts time.Time) string {
	payload, _ := json.Marshal(map[string]any{
		"eventId": GenerateCompactID(accountID),
		"ts":      ts.UnixMilli(),
		"source":  "simulate",
	})
	return string(payload)
}

func accountSnapshotToBalanceUpdate(eventID string, snap *AccountSnapshot, now time.Time) ctypes.BalanceUpdate {
	var assets []*ctypes.AssetEvent
	for k, v := range snap.Bal {
		b := v
		assets = append(assets, &ctypes.AssetEvent{
			WalletType: k.Wallet,
			Code:       string(k.Asset),
			Balance:    &b,
			Locked:     lo.ToPtr(decimal.Zero),
			UpdatedTs:  now,
		})
	}
	return ctypes.BalanceUpdate{
		EventID: eventID,
		Type:    ctypes.UpdateTypeSnapshot,
		Reason:  ctypes.LedgerReasonSnapshot,
		Assets:  assets,
	}
}

func perpSlotToPositionsUpdate(
	ex ctypes.Exchange,
	accountID string,
	symbol ctypes.Symbol,
	slot *PerpSlot,
	mode PositionMode,
	eventID string,
	now time.Time,
) *ctypes.PositionsUpdate {
	if slot == nil || !symbol.IsValid() {
		return nil
	}
	var positions []*ctypes.Position
	switch mode {
	case PositionModeHedge:
		if !slot.Long.Qty.IsZero() {
			positions = append(positions, &ctypes.Position{
				AccountID:     accountID,
				Exchange:      ex,
				Symbol:        symbol,
				Side:          ctypes.PositionSideLong,
				Amount:        slot.Long.Qty,
				EntryPrice:    slot.Long.EntryPrice,
				InitialMargin: slot.Long.UsedMargin,
				Leverage:      int(slot.Long.Leverage),
				UpdatedTs:     now,
			})
		}
		if !slot.Short.Qty.IsZero() {
			positions = append(positions, &ctypes.Position{
				AccountID:     accountID,
				Exchange:      ex,
				Symbol:        symbol,
				Side:          ctypes.PositionSideShort,
				Amount:        slot.Short.Qty,
				EntryPrice:    slot.Short.EntryPrice,
				InitialMargin: slot.Short.UsedMargin,
				Leverage:      int(slot.Short.Leverage),
				UpdatedTs:     now,
			})
		}
	default:
		side := ctypes.PositionSideLong
		amount := slot.Net.Qty
		if amount.Sign() < 0 {
			side = ctypes.PositionSideShort
			amount = amount.Abs()
		}
		if amount.IsZero() {
			// 净仓归零：同时下发多空两腿 qty=0，避免仅 LONG 快照无法清除此前 SHORT 侧 DB 行。
			positions = append(positions,
				&ctypes.Position{
					AccountID: accountID,
					Exchange:  ex,
					Symbol:    symbol,
					Side:      ctypes.PositionSideLong,
					Amount:    decimal.Zero,
					UpdatedTs: now,
				},
				&ctypes.Position{
					AccountID: accountID,
					Exchange:  ex,
					Symbol:    symbol,
					Side:      ctypes.PositionSideShort,
					Amount:    decimal.Zero,
					UpdatedTs: now,
				},
			)
			break
		}
		positions = append(positions, &ctypes.Position{
			AccountID:     accountID,
			Exchange:      ex,
			Symbol:        symbol,
			Side:          side,
			Amount:        amount,
			EntryPrice:    slot.Net.EntryPrice,
			InitialMargin: slot.Net.UsedMargin,
			Leverage:      int(slot.Net.Leverage),
			UpdatedTs:     now,
		})
	}
	if len(positions) == 0 {
		// 该标的已无持仓（常见于双向模式平掉最后一腿）：仍发快照，否则 accountEventToMessage 返回 nil，下游收不到平仓后的仓位更新。
		positions = []*ctypes.Position{
			{
				AccountID: accountID,
				Exchange:  ex,
				Symbol:    symbol,
				Side:      ctypes.PositionSideLong,
				Amount:    decimal.Zero,
				UpdatedTs: now,
			},
			{
				AccountID: accountID,
				Exchange:  ex,
				Symbol:    symbol,
				Side:      ctypes.PositionSideShort,
				Amount:    decimal.Zero,
				UpdatedTs: now,
			},
		}
	}
	return &ctypes.PositionsUpdate{
		EventID:   eventID,
		Type:      ctypes.UpdateTypeSnapshot,
		Positions: positions,
	}
}

func (rt *VenueRuntime) accountEventToMessage(ev AccountEvent) *ctypes.Message {
	now := time.Now().UTC()
	aid := accountIDFromAccountEvent(ev)
	if aid == "" {
		return nil
	}
	eventID := GenerateCompactID(aid)
	acctSel := lo.ToPtr(aid)
	sel := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Account: acctSel,
	}

	switch ev.kind {
	case AccountEventTypeLeverage:
		sym := toTypesSymbol(ev.symbol)
		if ev.leverage == nil || !sym.IsValid() || ev.leverage.leverage < 1 {
			return nil
		}
		sl := &ctypes.SymbolLeverage{
			Exchange:  rt.Exchange,
			Symbol:    sym,
			Side:      ev.leverage.leverageSide,
			Leverage:  ev.leverage.leverage,
			UpdatedTs: now,
		}
		return ctypes.NewMessage(rt.Exchange, sel, sl, now)

	case AccountEventTypeOrder:
		if ev.order == nil {
			return nil
		}
		co := toTypesOrder(rt.Exchange, ev.order)
		co.Raw = simulateAccountEventMeta(aid, now)
		return ctypes.NewMessage(rt.Exchange, sel, co, now)

	case AccountEventTypeBalance:
		if ev.balance == nil {
			return nil
		}
		up := accountSnapshotToBalanceUpdate(eventID, ev.balance, now)
		return ctypes.NewMessage(rt.Exchange, sel, up, now)

	case AccountEventTypePosition:
		if ev.position == nil {
			return nil
		}
		sym := toTypesSymbol(ev.symbol)
		pu := perpSlotToPositionsUpdate(rt.Exchange, aid, sym, ev.position, ev.position.Mode, eventID, now)
		if pu == nil {
			return nil
		}
		return ctypes.NewMessage(rt.Exchange, sel, pu, now)

	default:
		return nil
	}
}
