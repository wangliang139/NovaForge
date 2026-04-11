package signal

import (
	"fmt"
	"time"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	types "github.com/wangliang139/NovaForge/server/pkg/types"
)

type CommonSignalSpec struct {
	stypes.SignalDefinition
	StartTs time.Time
	EndTs   time.Time
}

var _ stypes.SignalSpec = (*CommonSignalSpec)(nil)

func NewCommonSignalSpec(signal stypes.SignalDefinition, startTs time.Time, endTs time.Time) *CommonSignalSpec {
	return &CommonSignalSpec{
		SignalDefinition: signal,
		StartTs:          startTs,
		EndTs:            endTs,
	}
}

func (s *CommonSignalSpec) GetID() string {
	ex := ""
	if s.Exchange != nil {
		ex = s.Exchange.String()
	}
	sym := ""
	if s.Symbol != nil {
		sym = s.Symbol.String()
	}
	return fmt.Sprintf("%s-%s-%s", s.Type.String(), ex, sym)
}

func (s *CommonSignalSpec) GetSignalID() string {
	return s.ID
}

func (s *CommonSignalSpec) GetType() types.SignalType {
	return s.Type
}

func (s *CommonSignalSpec) GetScope() types.SignalScope {
	return s.Scope
}

func (s *CommonSignalSpec) GetExchange() *ctypes.Exchange {
	return s.Exchange
}

func (s *CommonSignalSpec) GetSymbol() *ctypes.Symbol {
	return s.Symbol
}

func (s *CommonSignalSpec) GetProps() map[string]any {
	return s.Props
}

func (s *CommonSignalSpec) GetStartTs() time.Time {
	return s.StartTs
}

func (s *CommonSignalSpec) GetEndTs() time.Time {
	return s.EndTs
}

func (s *CommonSignalSpec) MatchProps(props map[string]any) bool {
	return true
}
