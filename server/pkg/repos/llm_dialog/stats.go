package llm_dialog

import "encoding/json"

// EmptyDialogStatsJSON 新建对话行时的默认 stats（{} 解析为零值 DialogStats）。
var EmptyDialogStatsJSON = []byte("{}")

// DialogStats 持久化在 stats jsonb 中（snake_case）。
type DialogStats struct {
	StepCount        int32  `json:"step_count"`
	ToolCallCount    int32  `json:"tool_call_count"`
	FinishReason     string `json:"finish_reason"`
	LastEventSeq     int32  `json:"last_event_seq"`
	PromptTokens     int32  `json:"prompt_tokens"`
	CompletionTokens int32  `json:"completion_tokens"`
	TotalTokens      int32  `json:"total_tokens"`
}

// MarshalDialogStats 序列化为 JSON；失败时返回 "{}".
func MarshalDialogStats(s DialogStats) []byte {
	b, err := json.Marshal(s)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// ParseDialogStats 从 JSON 解析；空或非法时返回零值结构。
func ParseDialogStats(b []byte) DialogStats {
	var s DialogStats
	if len(b) == 0 {
		return s
	}
	_ = json.Unmarshal(b, &s)
	return s
}
