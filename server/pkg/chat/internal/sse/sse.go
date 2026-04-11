package sse

import "github.com/wangliang139/llt-trade/server/pkg/chat/domain"

func PhaseForPart(partType string) string {
	switch partType {
	case domain.EventThinking:
		return "thinking"
	case domain.EventToolCall:
		return "action"
	case domain.EventToolResult:
		return "observation"
	case domain.EventStarted, domain.EventDone, domain.EventError, domain.EventHeartbeat:
		return "control"
	default:
		return "final"
	}
}

func DeltaFromPart(part domain.DialogPart) map[string]any {
	delta := map[string]any{}
	if part.Text != "" {
		delta["text"] = part.Text
	}
	if part.BlockID != "" {
		delta["blockId"] = part.BlockID
	}
	if part.Language != "" {
		delta["language"] = part.Language
	}
	if part.ToolCallID != "" {
		delta["toolCallId"] = part.ToolCallID
	}
	if part.ToolName != "" {
		delta["toolName"] = part.ToolName
	}
	if part.Format != "" {
		delta["format"] = part.Format
	}
	if part.Status != "" {
		delta["status"] = part.Status
	}
	if part.Arguments != nil {
		delta["arguments"] = part.Arguments
	}
	if part.Result != nil {
		delta["result"] = part.Result
	}
	if part.ActionID != "" {
		delta["actionId"] = part.ActionID
	}
	if part.Component != "" {
		delta["component"] = part.Component
	}
	if part.Props != nil {
		delta["props"] = part.Props
	}
	if part.Code != "" {
		delta["code"] = part.Code
	}
	if part.Message != "" {
		delta["message"] = part.Message
	}
	if part.Append {
		delta["append"] = true
	}
	if part.ExecutionState != "" {
		delta["executionState"] = string(part.ExecutionState)
	}
	return delta
}
