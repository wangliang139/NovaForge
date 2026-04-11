package types

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type PositionKey struct {
	ExSymbol ExSymbol
	Side     PositionSide
}

type Position struct {
	AccountID        string          `json:"accountId,omitempty"` // 账户ID
	Exchange         Exchange        `json:"exchange,omitempty"`
	Symbol           Symbol          `json:"symbol,omitempty"`
	Side             PositionSide    `json:"side,omitempty"`
	Isolated         bool            `json:"isolated,omitempty"`
	Amount           decimal.Decimal `json:"amount,omitempty"`
	EntryPrice       decimal.Decimal `json:"entryPrice,omitempty"`
	MarkPrice        decimal.Decimal `json:"markPrice,omitempty"`
	LiquidationPrice decimal.Decimal `json:"liquidationPrice,omitempty"`
	Notional         decimal.Decimal `json:"notional,omitempty"`
	Leverage         int             `json:"leverage,omitempty"`
	InitialMargin    decimal.Decimal `json:"initialMargin,omitempty"`
	MaintMargin      decimal.Decimal `json:"maintMargin,omitempty"`
	UnRealizedProfit decimal.Decimal `json:"unRealizedProfit,omitempty"`
	UpdatedTs        time.Time       `json:"updatedTs,omitempty"`
}

func (p *Position) Update(isBuy bool, qty, price decimal.Decimal) {
	if isBuy {
		totalCost := p.EntryPrice.Mul(p.Amount).Add(price.Mul(qty))
		p.Amount = p.Amount.Add(qty)
		p.EntryPrice = totalCost.Div(p.Amount)
	} else {
		p.Amount = p.Amount.Sub(qty)
		if p.Amount.IsZero() {
			p.EntryPrice = decimal.Zero
		}
	}
}

type PositionSide string

const (
	PositionSideLong  PositionSide = "LONG"
	PositionSideShort PositionSide = "SHORT"
)

func (p PositionSide) String() string {
	return string(p)
}

func (p PositionSide) Valid() bool {
	switch p {
	case PositionSideLong, PositionSideShort:
		return true
	}
	return false
}

func ParsePositionSide(s string) PositionSide {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "LONG":
		return PositionSideLong
	case "SHORT":
		return PositionSideShort
	}
	return PositionSide("")
}
