package logging

import (
	"context"
	"time"
)

type Storage interface {
	Write(ctx context.Context, entry Entry) error
}

// Entry represents one cached console log entry.
type Entry struct {
	Ts      time.Time `json:"ts"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}
