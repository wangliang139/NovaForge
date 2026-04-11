package domain

// ExecutionState 表示单轮 ReAct 的显式状态（SSE / parts / 日志共用同一套取值）。
//
// 合法迁移（文档约定，运行时不强制 panic）：
//
//	idle → model_calling
//	model_calling → awaiting_tools（模型返回 tool_calls）
//	model_calling → streaming_answer（模型返回文本）
//	model_calling → degraded_no_tools（连续工具失败后强制一轮禁工具）
//	awaiting_tools → tool_running → tool_observed（每个工具；多个工具则 tool_observed → tool_running 链式）
//	tool_observed → model_calling（本步内工具跑完，进入下一轮模型调用）
//	streaming_answer → completed
//	* → failed（provider/empty_choices/loop_limit 等致命路径）
type ExecutionState string

const (
	ExecIdle            ExecutionState = "idle"
	ExecModelCalling    ExecutionState = "model_calling"
	ExecAwaitingTools   ExecutionState = "awaiting_tools"
	ExecToolRunning     ExecutionState = "tool_running"
	ExecToolObserved    ExecutionState = "tool_observed"
	ExecStreamingAnswer ExecutionState = "streaming_answer"
	ExecDegradedNoTools ExecutionState = "degraded_no_tools"
	ExecCompleted       ExecutionState = "completed"
	ExecFailed          ExecutionState = "failed"
)
