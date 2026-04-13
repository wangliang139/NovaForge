package types

import (
	"fmt"
	"strings"
	"time"
)

type StreamType string

const (
	StreamTypeTicker    StreamType = "ticker"
	StreamTypeTrade     StreamType = "trade"
	StreamTypeDepth     StreamType = "depth"
	StreamTypeKline     StreamType = "kline"
	StreamTypeMarkPrice StreamType = "mark_price"
	// StreamTypeAccountRaw 为交易所推送的原始账户事件流（需要先由 account entity 处理后再对外发布）
	StreamTypeAccountRaw StreamType = "account_raw"
	StreamTypeAccount    StreamType = "account"
	StreamTypeSocial     StreamType = "social"
)

func (s StreamType) String() string {
	return string(s)
}

func (s StreamType) Valid() bool {
	switch s {
	case StreamTypeTicker,
		StreamTypeTrade,
		StreamTypeDepth,
		StreamTypeKline,
		StreamTypeMarkPrice,
		StreamTypeSocial,
		StreamTypeAccountRaw,
		StreamTypeAccount:
		return true
	}
	return false
}

func (s StreamType) IsAccountRequired() bool {
	switch s {
	case StreamTypeTicker,
		StreamTypeTrade,
		StreamTypeDepth,
		StreamTypeKline,
		StreamTypeSocial,
		StreamTypeMarkPrice:
		return false
	default:
		return true
	}
}

func (t StreamType) IsMarketSignal() bool {
	switch t {
	case StreamTypeKline, StreamTypeTrade, StreamTypeDepth, StreamTypeTicker, StreamTypeMarkPrice:
		return true
	}
	return false
}

type Subscription struct {
	ID       string         `json:"id,omitempty"`
	Exchange Exchange       `json:"exchange,omitempty"`
	Selector StreamSelector `json:"selector,omitempty"`
	Refs     int64          `json:"refs,omitempty"`
	Count    int64          `json:"count,omitempty"`
}

type StreamSelector struct {
	Stream   StreamType `json:"stream,omitempty"`
	Symbol   *Symbol    `json:"symbol,omitempty"`
	Interval *Interval  `json:"interval,omitempty"`
	Account  *string    `json:"account,omitempty"` // account id
}

func (s StreamSelector) Validate() error {
	if !s.Stream.Valid() {
		return fmt.Errorf("unsupported stream type: %s", s.Stream)
	}
	switch s.Stream {
	case StreamTypeTicker,
		StreamTypeTrade,
		StreamTypeDepth,
		StreamTypeSocial:
		return nil
	case StreamTypeKline:
		if s.Interval == nil || !s.Interval.Valid() {
			return fmt.Errorf("kline selector requires interval")
		}
		return nil
	case StreamTypeMarkPrice:
		// mark price 仅用于合约行情，通常要求指定 symbol
		if s.Symbol == nil || !s.Symbol.IsValid() {
			return fmt.Errorf("mark price selector requires symbol")
		}
		return nil
	case StreamTypeAccount, StreamTypeAccountRaw:
		return nil
	default:
		return fmt.Errorf("unsupported stream type: %s", s.Stream)
	}
}

func (s StreamSelector) Key() string {
	var idKey, streamKey string

	switch s.Stream {
	case StreamTypeKline:
		streamKey = fmt.Sprintf("kline_%s", s.Interval.String())
	default:
		streamKey = s.Stream.String()
	}

	if s.Account != nil {
		idKey = *s.Account
	} else if s.Symbol != nil {
		idKey = s.Symbol.String()
	}

	if len(idKey) == 0 {
		return streamKey
	}

	return fmt.Sprintf("%s.%s", idKey, streamKey)
}

type MessagePayload interface {
	*Ticker | *Trade | *OrderBook | *Kline | *MarkPrice | *BalanceSnapshot | *BalanceUpdate | *PositionSnapshot | *PositionsUpdate | *Order | *SymbolLeverage | *Fill | *Document
}

type Message struct {
	Exchange  Exchange       `json:"-"`
	Selector  StreamSelector `json:"-"`
	Ts        time.Time      `json:"-"` // event generated timestamp
	PublishAt time.Time      `json:"-"` // event published timestamp
	ReceiveAt time.Time      `json:"-"` // event received timestamp

	Ticker           *Ticker           `json:"ticker,omitempty"`
	Trade            *Trade            `json:"trade,omitempty"`
	Depth            *OrderBook        `json:"depth,omitempty"`
	Kline            *Kline            `json:"kline,omitempty"`
	MarkPrice        *MarkPrice        `json:"mark_price,omitempty"`
	SymbolLeverage   *SymbolLeverage   `json:"symbol_leverage,omitempty"`
	BalanceSnapshot  *BalanceSnapshot  `json:"balance_snapshot,omitempty"`
	BalanceUpdate    *BalanceUpdate    `json:"balance_update,omitempty"`
	PositionSnapshot *PositionSnapshot `json:"position_snapshot,omitempty"`
	PositionsUpdate  *PositionsUpdate  `json:"positions_update,omitempty"`
	Order            *Order            `json:"order,omitempty"`
	Fill             *Fill             `json:"fill,omitempty"`
	Social           *Document         `json:"social,omitempty"`
}

func (m *Message) GetSymbol() *Symbol {
	return m.Selector.Symbol
}

func (m *Message) GetExchange() *Exchange {
	return &m.Exchange
}

func (m *Message) GetStreamType() StreamType {
	return m.Selector.Stream
}

func NewSocialMessage(doc *Document) *Message {
	return &Message{
		Ts:        doc.CreatedAt,
		ReceiveAt: doc.CreatedAt,
		PublishAt: doc.UpdatedAt,
		Social:    doc,
	}
}

func NewMessage(exchange Exchange, selector StreamSelector, payload any, ts time.Time) *Message {
	msg := &Message{
		Exchange:  exchange,
		Selector:  selector,
		Ts:        ts,
		ReceiveAt: time.Now(),
	}
	switch t := any(payload).(type) {
	case *Ticker:
		msg.Ticker = t
	case Ticker:
		msg.Ticker = &t
	case *Trade:
		msg.Trade = t
	case Trade:
		msg.Trade = &t
	case *OrderBook:
		msg.Depth = t
	case OrderBook:
		msg.Depth = &t
	case *Kline:
		msg.Kline = t
	case Kline:
		msg.Kline = &t
	case *MarkPrice:
		msg.MarkPrice = t
	case MarkPrice:
		msg.MarkPrice = &t
	case *BalanceSnapshot:
		msg.BalanceSnapshot = t
	case BalanceSnapshot:
		msg.BalanceSnapshot = &t
	case *BalanceUpdate:
		msg.BalanceUpdate = t
	case BalanceUpdate:
		msg.BalanceUpdate = &t
	case *PositionSnapshot:
		msg.PositionSnapshot = t
	case PositionSnapshot:
		msg.PositionSnapshot = &t
	case *PositionsUpdate:
		msg.PositionsUpdate = t
	case PositionsUpdate:
		msg.PositionsUpdate = &t
	case *Order:
		msg.Order = t
	case Order:
		msg.Order = &t
	case *SymbolLeverage:
		msg.SymbolLeverage = t
	case SymbolLeverage:
		msg.SymbolLeverage = &t
	case *Fill:
		msg.Fill = t
	case Fill:
		msg.Fill = &t
	default:
		panic(fmt.Sprintf("unsupported payload type: %T", t))
	}
	return msg
}

type Envelope struct {
	Topic     string     `json:"topic,omitempty"`
	Exchange  string     `json:"exchange,omitempty"`
	Account   *string    `json:"account,omitempty"`
	Symbol    *string    `json:"symbol,omitempty"`
	Stream    StreamType `json:"stream,omitempty"`
	Payload   *Message   `json:"payload,omitempty"`
	Ts        int64      `json:"ts,omitempty"`         // event generated timestamp in unix millisecond
	ReceiveAt int64      `json:"receive_at,omitempty"` // event received timestamp in unix millisecond
	PublishAt int64      `json:"publish_at,omitempty"` // event published timestamp in unix millisecond
	// P2 T4：父 multi_bot Fanout 合成 account_raw 溯源（见 docs/P2_T0_VIRTUAL_SUB_ATTRIBUTION.md §3.3）
	Synthetic      bool   `json:"synthetic,omitempty"`
	SourceParentID string `json:"source_parent_id,omitempty"`
}

func TopicName(exchange Exchange, selector StreamSelector) string {
	topic := fmt.Sprintf("md.%s.%s", exchange.String(), selector.Key())
	topic = strings.ReplaceAll(topic, "..", ".")
	topic = strings.ReplaceAll(topic, ":", "")
	topic = strings.ReplaceAll(topic, "/", "")
	return topic
}

func StreamKey(exchange Exchange, selector StreamSelector) string {
	return fmt.Sprintf("%s-%s", exchange.String(), selector.Key())
}
