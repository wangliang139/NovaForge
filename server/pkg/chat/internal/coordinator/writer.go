package coordinator

import (
	"fmt"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
)

type eventWriter struct {
	ch        chan<- domain.DeltaEvent
	sessionID string
	dialogID  string
	seq       int
}

func NewEventWriter(ch chan<- domain.DeltaEvent, sessionID, dialogID string) *eventWriter {
	return &eventWriter{
		ch:        ch,
		sessionID: sessionID,
		dialogID:  dialogID,
	}
}

func (w *eventWriter) Write(eventType, phase string, delta map[string]any, meta map[string]any) error {
	w.seq++
	event := domain.DeltaEvent{
		V:         1,
		ID:        fmt.Sprintf("%s:%d", w.dialogID, w.seq),
		SessionID: w.sessionID,
		DialogID:  w.dialogID,
		Seq:       w.seq,
		Type:      eventType,
		Phase:     phase,
		Ts:        time.Now().Unix(),
		Delta:     delta,
		Meta:      meta,
	}
	w.ch <- event
	return nil
}

func (w *eventWriter) LastSeq() int32 {
	return int32(w.seq)
}
