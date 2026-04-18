package simulate

import (
	"time"

	"github.com/shopspring/decimal"
)

// Exchange identifies a venue (opaque string for flexibility).
type Exchange string

// Symbol identifies a market (opaque string for flexibility).
type Symbol string

// OrderBookLevel is one aggregated price level on the L2 book.
type OrderBookLevel struct {
	Price decimal.Decimal `json:"price,omitempty"`
	Size  decimal.Decimal `json:"size,omitempty"`
}

// OrderBook is the wire DTO for both full snapshots and incremental updates.
type OrderBook struct {
	Exchange  Exchange         `json:"exchange,omitempty"`
	Symbol    Symbol           `json:"symbol,omitempty"`
	Bids      []OrderBookLevel `json:"bids,omitempty"`
	Asks      []OrderBookLevel `json:"asks,omitempty"`
	Ts        time.Time        `json:"ts,omitempty"`
	SeqId     int64            `json:"seqId,omitempty"`
	PrevSeqId int64            `json:"prevSeqId,omitempty"`
}
