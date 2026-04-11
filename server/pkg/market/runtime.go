package market

import (
	"sync"
	"sync/atomic"

	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

type StreamStatus int32

const (
	StreamStatusInit         StreamStatus = 0
	StreamStatusReady        StreamStatus = 1
	StreamStatusStopped      StreamStatus = 2
	StreamStatusReconnecting StreamStatus = 3
)

type streamRuntime struct {
	exchange ctypes.Exchange
	selector StreamSelector

	connector Connector
	handle    *StreamHandle

	mu       sync.RWMutex
	status   atomic.Int32
	stopCh   chan struct{}
	stopOnce sync.Once
}

func (s *streamRuntime) stop() {
	if s == nil {
		return
	}
	if s.status.Load() == int32(StreamStatusStopped) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
		if s.handle != nil && s.handle.Stop != nil {
			s.handle.Stop()
		}
		s.status.Store(int32(StreamStatusStopped))
	})
}
