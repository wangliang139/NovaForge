package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wangliang139/NovaForge/server/pkg/chat/domain"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/tools"
)

func TestParseToolArguments(t *testing.T) {
	args, err := ParseToolArguments(`{"payload":{"a":1}}`)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if args["payload"] == nil {
		t.Fatalf("expected payload in args")
	}
}

func TestWhitelistRuntime_echoMissingPayload(t *testing.T) {
	rt := NewWhitelistRuntime()
	_, err := rt.Call(context.Background(), tools.ToolEchoJSON, map[string]any{})
	if err == nil {
		t.Fatalf("expected error")
	}
	var re *domain.RuntimeError
	if !errors.As(err, &re) || re.Code != "invalid_argument" {
		t.Fatalf("expected RuntimeError invalid_argument, got %v", err)
	}
}

func TestWhitelistRuntime_unsupportedTool(t *testing.T) {
	rt := NewWhitelistRuntime()
	_, err := rt.Call(context.Background(), "nope", nil)
	var re *domain.RuntimeError
	if !errors.As(err, &re) || re.Code != "unsupported_tool" {
		t.Fatalf("expected unsupported_tool, got %v", err)
	}
}

func TestFormatToolError_runtimeError(t *testing.T) {
	s := FormatToolError(domain.NewRuntimeError("x", "y"))
	if !strings.Contains(s, "x") || !strings.Contains(s, "y") {
		t.Fatalf("unexpected json: %s", s)
	}
}

func TestWhitelistRuntime_Call(t *testing.T) {
	rt := NewWhitelistRuntime()
	got, err := rt.Call(context.Background(), tools.ToolNowISO8601, map[string]any{})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map result")
	}
	if obj["now"] == nil {
		t.Fatalf("expected now field")
	}
}

func TestWhitelistRuntime_GetSkillDetail(t *testing.T) {
	rt := NewWhitelistRuntime()
	got, err := rt.Call(context.Background(), tools.ToolGetSkillDetail, map[string]any{
		"skill_name": "获取策略开发手册",
	})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map result")
	}
	if obj["name"] == nil {
		t.Fatalf("expected name field")
	}
}
