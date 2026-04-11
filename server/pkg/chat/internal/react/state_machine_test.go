package react

import (
	"testing"

	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
)

func TestExecutionStateConstants(t *testing.T) {
	states := []domain.ExecutionState{
		domain.ExecIdle,
		domain.ExecModelCalling,
		domain.ExecAwaitingTools,
		domain.ExecToolRunning,
		domain.ExecToolObserved,
		domain.ExecStreamingAnswer,
		domain.ExecDegradedNoTools,
		domain.ExecCompleted,
		domain.ExecFailed,
	}
	for _, s := range states {
		if string(s) == "" {
			t.Fatalf("empty state constant")
		}
	}
}

func TestEmitWithState_setsMeta(t *testing.T) {
	var got map[string]any
	emit := func(eventType, phase string, delta map[string]any, meta map[string]any) error {
		got = meta
		return nil
	}
	_ = emitWithState(emit, domain.ExecModelCalling, "x", "y", nil, nil)
	if got["executionState"] != string(domain.ExecModelCalling) {
		t.Fatalf("meta: %#v", got)
	}
}
