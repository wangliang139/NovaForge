package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/wangliang139/NovaForge/server/pkg/chat/domain"
)

func TestIsRecoverableToolError_runtimeError(t *testing.T) {
	if !IsRecoverableToolError(domain.NewRuntimeError("invalid_argument", "x")) {
		t.Fatal("invalid_argument should be recoverable")
	}
	if !IsRecoverableToolError(domain.NewRuntimeError("unsupported_tool", "x")) {
		t.Fatal("unsupported_tool should be recoverable")
	}
	if !IsRecoverableToolError(context.DeadlineExceeded) {
		t.Fatal("deadline should be recoverable")
	}
	if !IsRecoverableToolError(SyntheticToolErrorFromParse(errors.New("bad json"))) {
		t.Fatal("parse error should be recoverable")
	}
}

func TestErrorCodeFromError(t *testing.T) {
	if got := ErrorCodeFromError(domain.NewRuntimeError("unsupported_tool", "x")); got != "unsupported_tool" {
		t.Fatalf("got %q", got)
	}
	if got := ErrorCodeFromError(SyntheticToolErrorFromParse(errors.New("x"))); got != "invalid_tool_arguments" {
		t.Fatalf("got %q", got)
	}
}
