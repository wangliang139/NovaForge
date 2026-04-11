package domain

import (
	"strconv"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/internal/zai"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
)

const (
	DefaultProvider = "openrouter"
	DefaultModel    = "minimax/minimax-m2.5"

	MaxOutputTokens = 20000

	// SceneKeyAISessionTitle 对应 llm_scene.key；prompt 模板可使用变量 {{.question}}、{{.answer}}。
	SceneKeyAISessionTitle = "ai_session_title"

	SessionTitleSummaryThreshold = 24
)

// Deps 是工具和技能运行时所需的外部依赖。
type Env struct {
	DB        *repos.Entity
	ZaiEngine *zai.Engine
	Model     string
}

const (
	RoleQuestion = "question"
	RoleAnswer   = "answer"
)

const (
	StatusIdle      = "idle"
	StatusPending   = "pending"
	StatusStreaming = "streaming"
	StatusCompleted = "completed"
	StatusError     = "error"
)

const (
	EventReady       = "ready"
	EventStarted     = "started"
	EventThinking    = "thinking"
	EventToolCall    = "tool_call"
	EventToolResult  = "tool_result"
	EventText        = "text"
	EventCode        = "code"
	EventInteractive = "interactive"
	EventError       = "error"
	EventDone        = "done"
	EventHeartbeat   = "heartbeat"
)

type CreateDialogRequest struct {
	UserID    int64  `json:"userId"`
	SessionID int64  `json:"sessionId"`
	Content   string `json:"content"`
	Model     string `json:"model"`
}

type RegenerateRequest struct {
	UserID    int64  `json:"userId"`
	SessionID int64  `json:"sessionId"`
	DialogID  int64  `json:"dialogId"`
	Model     string `json:"model"`
}

type LlmModelItem struct {
	ID      string `json:"id"`
	OwnedBy string `json:"ownedBy,omitempty"`
}

type SessionDTO struct {
	ID               string `json:"id"`
	UserID           string `json:"userId"`
	Title            string `json:"title"`
	Status           string `json:"status"`
	Summary          string `json:"summary"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	MaxHistoryTurns  int32  `json:"maxHistoryTurns"`
	PreferSummary    bool   `json:"preferSummary"`
	AllowToolContext bool   `json:"allowToolContext"`
	MaxInputTokens   int32  `json:"maxInputTokens"`
	MaxOutputTokens  int32  `json:"maxOutputTokens"`
	LastDialogID     string `json:"lastDialogId"`
	DialogCount      int32  `json:"dialogCount"`
	TurnCount        int32  `json:"turnCount"`
	LastDialogAt     int64  `json:"lastDialogAt"`
	CreatedAt        int64  `json:"createdAt"`
	UpdatedAt        int64  `json:"updatedAt"`
}

type DialogPart struct {
	Type string `json:"type"`
	// ExecutionState 可选：该片段产生时的 ReAct 状态机取值，便于重放与排障。
	ExecutionState ExecutionState `json:"executionState,omitempty"`
	Text           string         `json:"text,omitempty"`
	BlockID        string         `json:"blockId,omitempty"`
	Language       string         `json:"language,omitempty"`
	ToolCallID     string         `json:"toolCallId,omitempty"`
	ToolName       string         `json:"toolName,omitempty"`
	Format         string         `json:"format,omitempty"`
	Status         string         `json:"status,omitempty"`
	Arguments      map[string]any `json:"arguments,omitempty"`
	Result         any            `json:"result,omitempty"`
	ActionID       string         `json:"actionId,omitempty"`
	Component      string         `json:"component,omitempty"`
	Props          map[string]any `json:"props,omitempty"`
	Code           string         `json:"code,omitempty"`
	Message        string         `json:"message,omitempty"`
	Collapsed      bool           `json:"collapsed,omitempty"`
	Append         bool           `json:"append,omitempty"`
}

type ContextMeta struct {
	Strategy            string   `json:"strategy"`
	SummaryUsed         bool     `json:"summaryUsed"`
	IncludedDialogIDs   []string `json:"includedDialogIds"`
	ToolContextIncluded bool     `json:"toolContextIncluded"`
	Truncated           bool     `json:"truncated"`
	// ToolsPromptCompact：TOOLS 系统消息因预算使用了紧凑/降级 JSON。
	ToolsPromptCompact bool `json:"toolsPromptCompact,omitempty"`
	// SkillsPromptCompact：SKILLS 系统消息因预算缩短或仅保留名称等。
	SkillsPromptCompact  bool `json:"skillsPromptCompact,omitempty"`
	InputTokens          int  `json:"inputTokens"`
	ReservedOutputTokens int  `json:"reservedOutputTokens"`
}

type DialogDTO struct {
	ID               string       `json:"id"`
	SessionID        string       `json:"sessionId"`
	DialogID         string       `json:"dialogId"`
	Role             string       `json:"role"`
	Status           string       `json:"status"`
	ContentText      string       `json:"contentText"`
	Parts            []DialogPart `json:"parts"`
	ContextMeta      *ContextMeta `json:"contextMeta,omitempty"`
	Seq              int32        `json:"seq"`
	Provider         string       `json:"provider,omitempty"`
	Model            string       `json:"model,omitempty"`
	PromptTokens     int32        `json:"promptTokens"`
	CompletionTokens int32        `json:"completionTokens"`
	TotalTokens      int32        `json:"totalTokens"`
	CanRegenerate    bool         `json:"canRegenerate"`
	ErrorCode        string       `json:"errorCode,omitempty"`
	ErrorMessage     string       `json:"errorMessage,omitempty"`
	StartedAt        int64        `json:"startedAt"`
	CompletedAt      int64        `json:"completedAt"`
	CreatedAt        int64        `json:"createdAt"`
	UpdatedAt        int64        `json:"updatedAt"`
}

type SessionDetailDTO struct {
	Session SessionDTO  `json:"session"`
	Dialogs []DialogDTO `json:"dialogs"`
}

type CreateDialogResponse struct {
	Question DialogDTO `json:"question"`
	Answer   DialogDTO `json:"answer"`
}

type DeltaEvent struct {
	V         int            `json:"v"`
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId"`
	DialogID  string         `json:"dialogId"`
	Seq       int            `json:"seq"`
	Type      string         `json:"type"`
	Phase     string         `json:"phase"`
	Ts        int64          `json:"ts"`
	Delta     map[string]any `json:"delta"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// ChatMessage 为发往 LLM 的单条消息。
type ChatMessage struct {
	Role    string
	Content string
}

// ContextBuildResult 为组好的对话上下文（系统提示 + 历史 + 元信息）。
type ContextBuildResult struct {
	Messages     []ChatMessage
	ContextMeta  ContextMeta
	QuestionText string
}

func ToUnix(ts *time.Time) int64 {
	if ts == nil {
		return 0
	}
	return ts.Unix()
}

func Int64String(v int64) string {
	return strconv.FormatInt(v, 10)
}

// ChatRequest 统一流式对话入口（创建会话 / 新轮次 / 重放 / 重新生成）。
type ChatRequest struct {
	UserID     int64
	SessionID  int64 // 0 且需写入用户消息时表示新建会话
	DialogID   int64 // 非 0 时表示重放或 regenerate 的答案 id
	Regenerate bool
	Content    string
	Model      string
}
