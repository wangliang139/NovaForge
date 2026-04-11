package signal

import (
	"fmt"
	"time"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/converter"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	types "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/errors"
)

type KlineSignalSpec struct {
	CommonSignalSpec
	Interval ctypes.Interval `json:"interval,omitempty"`
}

var _ stypes.SignalSpec = (*KlineSignalSpec)(nil)

func (s *KlineSignalSpec) GetStartTs() time.Time {
	return s.StartTs
}

func (s *KlineSignalSpec) GetEndTs() time.Time {
	return s.EndTs
}

func (s *KlineSignalSpec) GetID() string {
	return fmt.Sprintf("%s-%s", s.CommonSignalSpec.GetID(), s.Interval.String())
}

func (s *KlineSignalSpec) GetSignalID() string {
	return s.SignalDefinition.ID
}

func (s *KlineSignalSpec) GetType() stypes.SignalType {
	return s.SignalDefinition.Type
}

func (s *KlineSignalSpec) GetScope() types.SignalScope {
	return s.SignalDefinition.Scope
}

func (s *KlineSignalSpec) GetExchange() *ctypes.Exchange {
	return s.SignalDefinition.Exchange
}

func (s *KlineSignalSpec) GetSymbol() *ctypes.Symbol {
	return s.SignalDefinition.Symbol
}

func (s *KlineSignalSpec) GetProps() map[string]any {
	return s.SignalDefinition.Props
}

func (s *KlineSignalSpec) MatchProps(props map[string]any) bool {
	interval, ok := props["interval"]
	if !ok {
		return true
	}
	intervalStr, ok := interval.(string)
	if !ok {
		return false
	}
	return intervalStr == s.Interval.String()
}

func CreateKlineSignalSpec(signal stypes.SignalDefinition, startTs time.Time, endTs time.Time) (*KlineSignalSpec, error) {
	if _, ok := signal.Props["interval"]; !ok {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("signal %s props.interval is required", signal.ID))
	}
	interval, err := converter.AnyToString(signal.Props["interval"])
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("signal %s props.interval is invalid", signal.ID))
	}
	itv := ctypes.Interval(interval)
	if !itv.Valid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("signal %s props.interval is invalid: %s", signal.ID, interval))
	}
	return &KlineSignalSpec{
		CommonSignalSpec: *NewCommonSignalSpec(signal, startTs, endTs),
		Interval:         itv,
	}, nil
}
