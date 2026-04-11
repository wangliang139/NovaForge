package types

import (
	"errors"
	"fmt"
)

// SourceError 标识某个 ExternalSource 在某个操作上的失败。
type SourceError struct {
	SourceID string
	Op       string
	Err      error
}

func (e *SourceError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("external source %s %s: %v", e.SourceID, e.Op, e.Err)
}

func (e *SourceError) Unwrap() error { return e.Err }

func IsSourceError(err error) (*SourceError, bool) {
	var se *SourceError
	if errors.As(err, &se) {
		return se, true
	}
	return nil, false
}

var (
	ErrInvalidInternalEvent = errors.New("invalid internal event")
	ErrInvalidInternalSeq   = errors.New("invalid internal event source_seq")
	ErrBotAlreadyRunning    = errors.New("bot is already running")
	ErrBotAlreadyStarting   = errors.New("bot is already starting")
)
