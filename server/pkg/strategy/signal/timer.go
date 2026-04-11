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

type TimerSignalSpec struct {
	CommonSignalSpec
	Interval time.Duration `json:"interval,omitempty"`
	Topic    string        `json:"topic,omitempty"`
}

var _ stypes.SignalSpec = (*TimerSignalSpec)(nil)

func (s *TimerSignalSpec) GetID() string {
	return fmt.Sprintf("%s-%s-%s", s.CommonSignalSpec.GetID(), s.Interval.String(), s.Topic)
}

func (s *TimerSignalSpec) GetSignalID() string {
	return s.SignalDefinition.ID
}

func (s *TimerSignalSpec) GetType() stypes.SignalType {
	return s.SignalDefinition.Type
}

func (s *TimerSignalSpec) GetScope() types.SignalScope {
	return s.SignalDefinition.Scope
}

func (s *TimerSignalSpec) GetExchange() *ctypes.Exchange {
	return s.SignalDefinition.Exchange
}

func (s *TimerSignalSpec) GetSymbol() *ctypes.Symbol {
	return s.SignalDefinition.Symbol
}

func (s *TimerSignalSpec) GetProps() map[string]any {
	return s.SignalDefinition.Props
}

func (s *TimerSignalSpec) GetStartTs() time.Time {
	return s.StartTs
}

func (s *TimerSignalSpec) GetEndTs() time.Time {
	return s.EndTs
}

func (s *TimerSignalSpec) MatchProps(props map[string]any) bool {
	interval, ok := props["interval"]
	if !ok {
		return true
	}
	intervalStr, ok := interval.(string)
	if !ok {
		return false
	}
	topic, ok := props["topic"]
	if !ok {
		return true
	}
	topicStr, ok := topic.(string)
	if !ok {
		return false
	}
	return topicStr == s.Topic && intervalStr == s.Interval.String()
}

func CreateTimerSignalSpec(signal stypes.SignalDefinition, startTs time.Time, endTs time.Time) (*TimerSignalSpec, error) {
	if _, ok := signal.Props["interval"]; !ok {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("signal %s props.interval is required", signal.ID))
	}
	interval, err := converter.AnyToInt(signal.Props["interval"])
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("signal %s props.interval is invalid", signal.ID))
	}
	if _, ok := signal.Props["topic"]; !ok {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("signal %s props.topic is required", signal.ID))
	}
	topic, err := converter.AnyToString(signal.Props["topic"])
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("signal %s props.topic is invalid", signal.ID))
	}
	return &TimerSignalSpec{
		CommonSignalSpec: *NewCommonSignalSpec(signal, startTs, endTs),
		Interval:         time.Duration(interval) * time.Millisecond,
		Topic:            topic,
	}, nil
}
