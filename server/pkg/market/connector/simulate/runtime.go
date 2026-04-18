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

	bootstraps map[string]bool
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
		streamHubs: make(map[string]*publicStreamHub),
		conns:      make(map[*Connector]struct{}),
		bootstraps: make(map[string]bool),
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
