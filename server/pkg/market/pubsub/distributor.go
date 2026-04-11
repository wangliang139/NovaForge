package pubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/market/provider"
)

type Publisher interface {
	Name() string
	Publish(ctx context.Context, topic string, envelope *ctypes.Envelope) error
	Close() error
}

type Recorder interface {
	Record(envelope *ctypes.Envelope)
	Close() error
}

type Distributor struct {
	accountRawMsgTopic string
	publishers         []Publisher
	recorder           Recorder
	marketProvider     *provider.MarketProvider

	subsMu      sync.RWMutex
	nextSubID   atomic.Int64
	subscribers map[string]map[int64]chan *ctypes.Envelope
}

func NewDistributor(accountRawMsgTopic string) *Distributor {
	return &Distributor{
		accountRawMsgTopic: accountRawMsgTopic,
		subscribers:        make(map[string]map[int64]chan *ctypes.Envelope),
	}
}

func (d *Distributor) Subscribe(topic string, buffer int) (<-chan *ctypes.Envelope, func()) {
	if buffer <= 0 {
		buffer = 128
	}
	ch := make(chan *ctypes.Envelope, buffer)
	id := d.nextSubID.Add(1)

	d.subsMu.Lock()
	if _, ok := d.subscribers[topic]; !ok {
		d.subscribers[topic] = make(map[int64]chan *ctypes.Envelope)
	}
	d.subscribers[topic][id] = ch
	d.subsMu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			d.subsMu.Lock()
			if subs, ok := d.subscribers[topic]; ok {
				if sub, ok := subs[id]; ok {
					delete(subs, id)
					close(sub)
				}
				if len(subs) == 0 {
					delete(d.subscribers, topic)
				}
			}
			d.subsMu.Unlock()
		})
	}
	return ch, cancel
}

func (d *Distributor) Register(pub Publisher) {
	if pub == nil {
		return
	}
	d.publishers = append(d.publishers, pub)
}

func (d *Distributor) SetRecorder(rec Recorder) {
	d.recorder = rec
}

func (d *Distributor) SetMarketProvider(provider *provider.MarketProvider) {
	d.marketProvider = provider
}

func (d *Distributor) Publish(ctx context.Context, msg *ctypes.Message) error {
	if msg == nil {
		return fmt.Errorf("nil message")
	}

	topic := ctypes.TopicName(msg.Exchange, msg.Selector)
	env := ctypes.Envelope{
		Topic:     topic,
		Exchange:  msg.Exchange.String(),
		Stream:    msg.Selector.Stream,
		Payload:   msg,
		Ts:        msg.Ts.UnixMilli(),
		ReceiveAt: msg.ReceiveAt.UnixMilli(),
		PublishAt: time.Now().UnixMilli(),
	}
	if msg.Selector.Symbol != nil {
		env.Symbol = lo.ToPtr(msg.Selector.Symbol.String())
	}
	if msg.Selector.Account != nil {
		env.Account = msg.Selector.Account
	}

	// 旁路记录所有事件流到 ClickHouse（含 account、market 等）
	if d.recorder != nil {
		d.recorder.Record(&env)
	}

	if d.marketProvider != nil {
		d.marketProvider.OnEvent(ctx, msg)
	}

	// 对外分发消息
	if msg.Selector.Stream != ctypes.StreamTypeAccountRaw {
		d.broadcast(topic, &env)
	}

	if len(d.publishers) > 0 {
		for _, pub := range d.publishers {
			var err error
			// account raw stream 需要先由账户模块处理后再发布
			if msg.Selector.Stream == ctypes.StreamTypeAccountRaw {
				err = pub.Publish(ctx, d.accountRawMsgTopic, &env)
			} else {
				// err = pub.Publish(ctx, topic, &env)
			}
			if err != nil && !errors.Is(err, ErrCircuitOpen) {
				log.Err(err).Str("topic", topic).Msg("failed to publish message")
			}
		}
	}
	return nil
}

func (d *Distributor) broadcast(topic string, env *ctypes.Envelope) {
	d.subsMu.RLock()
	defer d.subsMu.RUnlock()
	subs, ok := d.subscribers[topic]
	if !ok || len(subs) == 0 {
		return
	}
	for _, ch := range subs {
		select {
		case ch <- env:
		default:
			log.Error().Str("topic", topic).Msg("market stream subscriber channel full, dropping message")
		}
	}
}

func (d *Distributor) Close() error {
	var err error
	for _, pub := range d.publishers {
		if perr := pub.Close(); perr != nil {
			err = perr
		}
	}
	if d.recorder != nil {
		if perr := d.recorder.Close(); perr != nil {
			err = perr
		}
	}
	d.subsMu.Lock()
	for topic, subs := range d.subscribers {
		for id, ch := range subs {
			delete(subs, id)
			close(ch)
		}
		delete(d.subscribers, topic)
	}
	d.subsMu.Unlock()
	return err
}
