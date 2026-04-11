package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
)

func TestWhitelistRuntime_CallSkill_missingKV(t *testing.T) {
	rt := NewWhitelistRuntime() // Deps{} → KvGetByKey == nil
	_, err := rt.Call(context.Background(), "skill.get_strategy_development_manual", map[string]any{})
	if err == nil {
		t.Fatalf("expected error when KvGetByKey is nil")
	}
	var re *domain.RuntimeError
	if !errors.As(err, &re) || re.Code != "dependency_missing" {
		t.Fatalf("expected dependency_missing RuntimeError, got %v", err)
	}
}
