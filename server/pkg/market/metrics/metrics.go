package metrics

import (
	"sync"
	"time"
)

const defaultWindowHours = 1

// StreamStats 单流统计
type StreamStats struct {
	Exchange       string
	Stream         string
	EventCount     int64
	AvgLatencyMs   float64
	MaxLatencyMs   float64
	ReconnectCount int64
}

// ConnectorMetrics 内存滑动窗口指标采集
type ConnectorMetrics struct {
	mu sync.RWMutex

	// key: "exchange:stream"
	latencySum     map[string]int64
	latencyCount   map[string]int64
	latencyMax     map[string]int64
	eventCount     map[string]int64
	reconnectCount map[string]int64

	windowStart time.Time
}

// NewConnectorMetrics 创建 ConnectorMetrics
func NewConnectorMetrics() *ConnectorMetrics {
	return &ConnectorMetrics{
		latencySum:     make(map[string]int64),
		latencyCount:   make(map[string]int64),
		latencyMax:     make(map[string]int64),
		eventCount:     make(map[string]int64),
		reconnectCount: make(map[string]int64),
		windowStart:    time.Now(),
	}
}

func key(exchange, stream string) string {
	return exchange + ":" + stream
}

// RecordEvent 记录事件（ts 为消息时间戳，receiveAt 为接收时间）
func (m *ConnectorMetrics) RecordEvent(exchange, stream string, ts, receiveAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maybeRotateLocked()

	k := key(exchange, stream)
	m.eventCount[k]++

	if !ts.IsZero() {
		latencyMs := receiveAt.Sub(ts).Milliseconds()
		if latencyMs < 0 {
			latencyMs = 0
		}
		m.latencySum[k] += latencyMs
		m.latencyCount[k]++
		if latencyMs > m.latencyMax[k] {
			m.latencyMax[k] = latencyMs
		}
	}
}

// RecordReconnect 记录重连
func (m *ConnectorMetrics) RecordReconnect(exchange, stream string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maybeRotateLocked()

	k := key(exchange, stream)
	m.reconnectCount[k]++
}

// maybeRotateLocked 超过 1 小时则重置窗口（调用方需持有锁）
func (m *ConnectorMetrics) maybeRotateLocked() {
	if time.Since(m.windowStart) < time.Hour {
		return
	}
	m.latencySum = make(map[string]int64)
	m.latencyCount = make(map[string]int64)
	m.latencyMax = make(map[string]int64)
	m.eventCount = make(map[string]int64)
	m.reconnectCount = make(map[string]int64)
	m.windowStart = time.Now()
}

// Snapshot 返回当前窗口内的统计快照
func (m *ConnectorMetrics) Snapshot(windowHours int) []StreamStats {
	if windowHours <= 0 {
		windowHours = defaultWindowHours
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 收集所有 key（eventCount + reconnectCount 的并集）
	keys := make(map[string]struct{})
	for k := range m.eventCount {
		keys[k] = struct{}{}
	}
	for k := range m.reconnectCount {
		keys[k] = struct{}{}
	}

	result := make([]StreamStats, 0, len(keys))
	for k := range keys {
		ec := m.eventCount[k]
		rc := m.reconnectCount[k]
		avgMs := 0.0
		maxMs := 0.0
		if m.latencyCount[k] > 0 {
			avgMs = float64(m.latencySum[k]) / float64(m.latencyCount[k])
			maxMs = float64(m.latencyMax[k])
		}
		// 解析 exchange:stream
		exchange, stream := "", ""
		for i := 0; i < len(k); i++ {
			if k[i] == ':' {
				exchange = k[:i]
				stream = k[i+1:]
				break
			}
		}
		result = append(result, StreamStats{
			Exchange:       exchange,
			Stream:         stream,
			EventCount:     ec,
			AvgLatencyMs:   avgMs,
			MaxLatencyMs:   maxMs,
			ReconnectCount: rc,
		})
	}
	return result
}
