package runtime

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
)

// 可恢复工具错误码：出现时不终止整轮 ReAct，将错误 JSON 回注模型继续推理。
var recoverableToolCodes = map[string]struct{}{
	"invalid_argument":       {},
	"unsupported_tool":       {},
	"tool_timeout":           {},
	"invalid_tool_arguments": {},
	"runtime_error":          {}, // 未知异常也允许模型自纠一轮
}

// IsRecoverableToolError 判定工具路径是否应继续 ReAct（不触发 FinalizeAnswer 错误收尾）。
func IsRecoverableToolError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var tp *ToolArgumentsParseError
	if errors.As(err, &tp) {
		return true
	}
	var re *domain.RuntimeError
	if errors.As(err, &re) {
		_, ok := recoverableToolCodes[re.Code]
		return ok
	}
	return true
}

// ErrorCodeFromError 用于日志/指标标签；非 RuntimeError 时返回 generic。
func ErrorCodeFromError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "tool_timeout"
	}
	var tp *ToolArgumentsParseError
	if errors.As(err, &tp) {
		return "invalid_tool_arguments"
	}
	var re *domain.RuntimeError
	if errors.As(err, &re) {
		return re.Code
	}
	return "runtime_error"
}

// ToolArgumentsParseError 模型返回的 function.arguments 无法解析为 JSON。
type ToolArgumentsParseError struct {
	Err error
}

func (e *ToolArgumentsParseError) Error() string {
	if e.Err != nil {
		return "invalid tool arguments: " + e.Err.Error()
	}
	return "invalid tool arguments"
}

func (e *ToolArgumentsParseError) Unwrap() error { return e.Err }

// SyntheticToolErrorFromParse 将解析失败标记为可恢复错误。
func SyntheticToolErrorFromParse(parseErr error) error {
	return &ToolArgumentsParseError{Err: parseErr}
}

// FormatSyntheticParseError 写入 tool 消息的 JSON（recoverable）。
func FormatSyntheticParseError(parseErr error) string {
	raw, _ := json.Marshal(map[string]string{
		"code":    "invalid_tool_arguments",
		"message": parseErr.Error(),
	})
	return string(raw)
}
