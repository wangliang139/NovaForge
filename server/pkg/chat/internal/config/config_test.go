package config

import "testing"

func TestConfig_normalize(t *testing.T) {
	c := Config{
		MaxSteps:               0,
		MaxToolCalls:           -1,
		ToolTimeoutSeconds:     500,
		ToolsPromptBudgetBytes: -9,
	}
	c.normalize()
	if c.MaxSteps != 8 {
		t.Fatalf("MaxSteps: got %d", c.MaxSteps)
	}
	if c.MaxToolCalls != 16 {
		t.Fatalf("MaxToolCalls: got %d", c.MaxToolCalls)
	}
	if c.ToolTimeoutSeconds != 300 {
		t.Fatalf("ToolTimeoutSeconds should cap at 300, got %d", c.ToolTimeoutSeconds)
	}
	if c.ToolsPromptBudgetBytes != 0 {
		t.Fatalf("ToolsPromptBudgetBytes: got %d", c.ToolsPromptBudgetBytes)
	}
}

func TestConfig_normalize_max_clamp(t *testing.T) {
	c := Config{MaxSteps: 500, MaxToolCalls: 9999}
	c.normalize()
	if c.MaxSteps != 64 {
		t.Fatalf("MaxSteps cap: got %d", c.MaxSteps)
	}
	if c.MaxToolCalls != 256 {
		t.Fatalf("MaxToolCalls cap: got %d", c.MaxToolCalls)
	}
}

func TestConfig_normalize_consecutive_tool_errors(t *testing.T) {
	c := Config{MaxConsecutiveToolErrors: -1}
	c.normalize()
	if c.MaxConsecutiveToolErrors != 0 {
		t.Fatalf("got %d", c.MaxConsecutiveToolErrors)
	}
	c = Config{MaxConsecutiveToolErrors: 999}
	c.normalize()
	if c.MaxConsecutiveToolErrors != 50 {
		t.Fatalf("cap: got %d", c.MaxConsecutiveToolErrors)
	}
}
