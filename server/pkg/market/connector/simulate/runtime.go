package simulate

import (
	"fmt"
	"sync"

	"github.com/wangliang139/NovaForge/server/pkg/market/connector/binance"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/okx"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

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
	conns   map[*Connector]struct{}

	// symbolSimReady: symbol.String() -> ensureSymbolInitialized completed for this venue
	// (instrument + depth sync + streams). Until set, public depth updates do not match
	// resting orders and mark-price does not run liquidation.
	simReadyMu     sync.RWMutex
	symbolSimReady map[string]struct{}
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
		Exchange:   ex,
		Engine:     eng,
		Quotes:     q,
		Mark:       NewMarkIndexService(q),
		Liq:        NewLiquidationEngine(eng),
		Public:     pub,
		streamHubs:     make(map[string]*publicStreamHub),
		conns:          make(map[*Connector]struct{}),
		symbolSimReady: make(map[string]struct{}),
	}
	venues[ex] = rt
	return rt, nil
}

func (rt *VenueRuntime) registerConn(c *Connector) {
	rt.connsMu.Lock()
	defer rt.connsMu.Unlock()
	rt.conns[c] = struct{}{}
}

func (rt *VenueRuntime) unregisterConn(c *Connector) {
	rt.connsMu.Lock()
	defer rt.connsMu.Unlock()
	delete(rt.conns, c)
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
