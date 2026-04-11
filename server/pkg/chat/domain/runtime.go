package domain

// RuntimeError 工具执行可预期的失败（含参数校验）；写入 model 的 tool 消息时使用 FormatToolError。
type RuntimeError struct {
	Code    string
	Message string
}

func (e *RuntimeError) Error() string { return e.Message }

func NewRuntimeError(code, msg string) error {
	return &RuntimeError{Code: code, Message: msg}
}
