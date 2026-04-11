package chat

import "github.com/wangliang139/NovaForge/server/pkg/chat/domain"

type (
	CreateDialogRequest     = domain.CreateDialogRequest
	RegenerateRequest       = domain.RegenerateRequest
	LlmModelItem            = domain.LlmModelItem
	SessionDTO              = domain.SessionDTO
	DialogPart              = domain.DialogPart
	ContextMeta             = domain.ContextMeta
	DialogDTO               = domain.DialogDTO
	SessionDetailDTO        = domain.SessionDetailDTO
	CreateDialogResponse    = domain.CreateDialogResponse
	DeltaEvent              = domain.DeltaEvent
)

const (
	DefaultProvider  = domain.DefaultProvider
	DefaultModel     = domain.DefaultModel
	MaxOutputTokens  = domain.MaxOutputTokens
	RoleQuestion     = domain.RoleQuestion
	RoleAnswer       = domain.RoleAnswer
	StatusIdle       = domain.StatusIdle
	StatusPending    = domain.StatusPending
	StatusStreaming  = domain.StatusStreaming
	StatusCompleted  = domain.StatusCompleted
	StatusError      = domain.StatusError
	EventReady       = domain.EventReady
	EventStarted     = domain.EventStarted
	EventThinking    = domain.EventThinking
	EventToolCall    = domain.EventToolCall
	EventToolResult  = domain.EventToolResult
	EventText        = domain.EventText
	EventCode        = domain.EventCode
	EventInteractive = domain.EventInteractive
	EventError       = domain.EventError
	EventDone        = domain.EventDone
	EventHeartbeat   = domain.EventHeartbeat
)
