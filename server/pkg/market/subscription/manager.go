package subscription

import (
	"fmt"
	"sync"

	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/utils"
)

type Manager struct {
	mu sync.RWMutex

	streams map[string]*streamEntry
}

type streamEntry struct {
	id       string
	exchange ctypes.Exchange
	selector ctypes.StreamSelector
	refs     int64
	count    int64
}

func NewManager() *Manager {
	return &Manager{
		streams: make(map[string]*streamEntry),
	}
}

func (m *Manager) id(exchange ctypes.Exchange, selector ctypes.StreamSelector) string {
	return SubscriptionID(exchange, selector)
}

// SubscriptionID 根据 exchange 与 selector 计算订阅 ID，与 Manager 内部 id 一致，供外部按 selector 释放订阅时使用。
func SubscriptionID(exchange ctypes.Exchange, selector ctypes.StreamSelector) string {
	return utils.Hash.ShortMd5(ctypes.StreamKey(exchange, selector))
}

func (m *Manager) Add(exchange ctypes.Exchange, selector ctypes.StreamSelector) (*ctypes.Subscription, error) {
	if err := selector.Validate(); err != nil {
		return nil, err
	}
	if !exchange.IsValid() {
		return nil, fmt.Errorf("invalid exchange/symbol")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.id(exchange, selector)
	stream, ok := m.streams[id]
	if !ok {
		stream = &streamEntry{
			id:       id,
			exchange: exchange,
			selector: selector,
		}
		m.streams[id] = stream
	}
	stream.refs++
	stream.count++
	return &ctypes.Subscription{
		ID:       stream.id,
		Exchange: stream.exchange,
		Selector: stream.selector,
		Refs:     stream.refs,
		Count:    stream.count,
	}, nil
}

func (m *Manager) Get(id string) *ctypes.Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stream, ok := m.streams[id]
	if !ok {
		return nil
	}
	return &ctypes.Subscription{
		ID:       stream.id,
		Exchange: stream.exchange,
		Selector: stream.selector,
		Refs:     stream.refs,
		Count:    stream.count,
	}
}

func (m *Manager) Remove(id string) (*ctypes.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stream, ok := m.streams[id]
	if !ok {
		return nil, nil
	}
	stream.refs--
	if stream.refs == 0 {
		delete(m.streams, id)
	}
	return &ctypes.Subscription{
		ID:       stream.id,
		Exchange: stream.exchange,
		Selector: stream.selector,
		Refs:     stream.refs,
		Count:    stream.count,
	}, nil
}

func (m *Manager) Snapshot(filterExchange *ctypes.Exchange, filterSymbol *ctypes.Symbol, accountID *string) []ctypes.Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var (
		ex string
		sb string
	)
	if filterExchange != nil {
		ex = filterExchange.String()
	}
	if filterSymbol != nil {
		sb = filterSymbol.String()
	}

	result := make([]ctypes.Subscription, 0, len(m.streams))
	for _, stream := range m.streams {
		if ex != "" && stream.exchange.String() != ex {
			continue
		}
		if sb != "" && (stream.selector.Symbol == nil || stream.selector.Symbol.String() != sb) {
			continue
		}
		if accountID != nil && (stream.selector.Account == nil || *stream.selector.Account != *accountID) {
			continue
		}
		result = append(result, ctypes.Subscription{
			Exchange: stream.exchange,
			Selector: stream.selector,
			Refs:     stream.refs,
			Count:    stream.count,
		})
	}
	return result
}
