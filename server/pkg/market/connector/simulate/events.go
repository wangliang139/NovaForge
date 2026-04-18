package simulate

import (
	"time"

	"github.com/google/uuid"
)

// CorrelationID ties commands to downstream effects (logs, UI).
type CorrelationID string

// NewCorrelationID returns a new random correlation id.
func NewCorrelationID() CorrelationID {
	return CorrelationID(uuid.NewString())
}

// DepthCommittedEvent is emitted after L2 state advances.
type DepthCommittedEvent struct {
	CorrelationID CorrelationID
	Symbol        Symbol
	SeqID         int64
	Ts            time.Time
}

// MarkUpdatedEvent is emitted when mark price used for risk changes.
type MarkUpdatedEvent struct {
	CorrelationID CorrelationID
	Symbol        Symbol
	Mark          string // decimal as string to keep event struct light; optional
	Ts            time.Time
}

// AccountEffectKind classifies trading-plane outcomes.
type AccountEffectKind int

const (
	EffectOrderUpdate AccountEffectKind = iota
	EffectBalanceOrPosition
)

// AccountEffect is a single observable outcome (optional bus consumer).
type AccountEffect struct {
	Kind          AccountEffectKind
	CorrelationID CorrelationID
	AccountID     string
	Symbol        Symbol
	Ts            time.Time
}
