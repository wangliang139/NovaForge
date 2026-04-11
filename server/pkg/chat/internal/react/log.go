package react

import (
	zlog "github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
)

// StreamLogFields 结构化日志关联字段（零值表示省略该维度）。
type StreamLogFields struct {
	SessionID int64
	DialogID  int64
	UserID    int64
}

func logReact(fields StreamLogFields, state domain.ExecutionState, msg string, kv func(e *zlog.Event)) {
	e := log.Info().
		Str("component", "chat.react").
		Str("execution_state", string(state))
	if fields.SessionID != 0 {
		e = e.Int64("session_id", fields.SessionID)
	}
	if fields.DialogID != 0 {
		e = e.Int64("dialog_id", fields.DialogID)
	}
	if fields.UserID != 0 {
		e = e.Int64("user_id", fields.UserID)
	}
	if kv != nil {
		kv(e)
	}
	e.Msg(msg)
}
