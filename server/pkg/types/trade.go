package types

import (
	"time"

	"github.com/shopspring/decimal"
)

type Ticker struct {
	Exchange      Exchange        `json:"exchange,omitempty"`
	Symbol        Symbol          `json:"symbol,omitempty"`
	LastPrice     decimal.Decimal `json:"lastPrice,omitempty"`
	Open24        decimal.Decimal `json:"open24h,omitempty"`
	High24        decimal.Decimal `json:"high24h,omitempty"`
	Low24         decimal.Decimal `json:"low24h,omitempty"`
	Avg24         decimal.Decimal `json:"avg24h,omitempty"`
	Volume24      decimal.Decimal `json:"volume24h,omitempty"`
	QuoteVolume24 decimal.Decimal `json:"quoteVolume24h,omitempty"`
	Ts            time.Time       `json:"ts,omitempty"`
}

type Trade struct {
	Exchange Exchange        `json:"exchange,omitempty"`
	Symbol   Symbol          `json:"symbol,omitempty"`
	TradeID  string          `json:"tradeId,omitempty"`
	Price    decimal.Decimal `json:"price,omitempty"`
	Size     decimal.Decimal `json:"size,omitempty"`
	IsBuy    bool            `json:"isBuy,omitempty"`
	Ts       time.Time       `json:"ts,omitempty"`
}

type OrderBookLevel struct {
	Price decimal.Decimal `json:"price,omitempty"`
	Size  decimal.Decimal `json:"size,omitempty"`
}

type OrderBook struct {
	Exchange  Exchange         `json:"exchange,omitempty"`
	Symbol    Symbol           `json:"symbol,omitempty"`
	Bids      []OrderBookLevel `json:"bids,omitempty"`
	Asks      []OrderBookLevel `json:"asks,omitempty"`
	Ts        time.Time        `json:"ts,omitempty"`
	SeqId     int64            `json:"seqId,omitempty"`
	PrevSeqId int64            `json:"prevSeqId,omitempty"`
}

type Kline struct {
	Exchange    Exchange        `json:"exchange,omitempty"`
	Symbol      Symbol          `json:"symbol,omitempty"`
	Interval    Interval        `json:"interval,omitempty"`
	Open        decimal.Decimal `json:"open,omitempty"`
	High        decimal.Decimal `json:"high,omitempty"`
	Low         decimal.Decimal `json:"low,omitempty"`
	Close       decimal.Decimal `json:"close,omitempty"`
	Volume      decimal.Decimal `json:"volume,omitempty"`
	QuoteVolume decimal.Decimal `json:"quoteVolume,omitempty"`
	Trades      int64           `json:"trades,omitempty"`
	IsClosed    bool            `json:"isClosed,omitempty"`
	OpenTs      time.Time       `json:"openTs,omitempty"`
	CloseTs     time.Time       `json:"closeTs,omitempty"` // 不是事件时间，而是 bar 结束时间
}

type Price struct {
	Exchange Exchange        `json:"exchange,omitempty"`
	Symbol   Symbol          `json:"symbol,omitempty"`
	Price    decimal.Decimal `json:"price,omitempty"`
	Ts       time.Time       `json:"ts,omitempty"`
}

type BookPrice struct {
	Exchange Exchange        `json:"exchange,omitempty"`
	Symbol   Symbol          `json:"symbol,omitempty"`
	BidPrice decimal.Decimal `json:"bidPrice,omitempty"`
	BidQty   decimal.Decimal `json:"bidQty,omitempty"`
	AskPrice decimal.Decimal `json:"askPrice,omitempty"`
	AskQty   decimal.Decimal `json:"askQty,omitempty"`
	Ts       time.Time       `json:"ts,omitempty"`
}

type MarkPrice struct {
	Exchange  Exchange        `json:"exchange,omitempty"`
	Symbol    Symbol          `json:"symbol,omitempty"`
	MarkPrice decimal.Decimal `json:"markPrice,omitempty"`
	Ts        time.Time       `json:"ts,omitempty"`
}

type IndexPrice struct {
	Exchange   Exchange        `json:"exchange,omitempty"`
	Symbol     Symbol          `json:"symbol,omitempty"`
	IndexPrice decimal.Decimal `json:"indexPrice,omitempty"`
	Ts         time.Time       `json:"ts,omitempty"`
}

type IndexComponent struct {
	Exchange   Exchange `json:"exchange,omitempty"`
	Symbol     Symbol   `json:"symbol,omitempty"`
	Components []struct {
		Exchange string          `json:"exchange,omitempty"`
		Symbol   string          `json:"symbol,omitempty"`
		Price    decimal.Decimal `json:"price,omitempty"`
		Weight   decimal.Decimal `json:"weight,omitempty"`
	} `json:"components,omitempty"`
	Price decimal.Decimal `json:"price,omitempty"`
	Ts    time.Time       `json:"ts,omitempty"`
}

type LeverageBracket struct {
	Symbol   Symbol    `json:"symbol,omitempty"`
	Brackets []Bracket `json:"brackets,omitempty"`
}

type Bracket struct {
	Bracket     int             `json:"bracket,omitempty"`     // Notional bracket
	MaxLeverage float32         `json:"maxLeverage,omitempty"` // Max leverage for this bracket
	MinNotional decimal.Decimal `json:"minNotional,omitempty"` // 最小名义价值
	MaxNotional decimal.Decimal `json:"maxNotional,omitempty"` // 最大名义价值
	Mmr         decimal.Decimal `json:"mmr,omitempty"`         // 维持保证金率
	Cum         decimal.Decimal `json:"cum,omitempty"`         // Auxiliary number for quick calculation
}
