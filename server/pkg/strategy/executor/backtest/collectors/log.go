package collectors

import (
	"sync"
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging/store"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// LogCollector 日志收集器
// 从 Strategy Runtime 收集日志（包装 consoleApiLogger）
type LogCollector struct {
	mu     sync.RWMutex
	logger *store.BufferStorage
}

// NewLogCollector 创建日志收集器
func NewLogCollector(logger *store.BufferStorage) *LogCollector {
	return &LogCollector{
		logger: logger,
	}
}

// OnLog 记录日志（用于直接调用）
func (c *LogCollector) OnLog(ts time.Time, level string, message string) {
	// LogCollector 主要从 logger 中获取，这里提供接口以备扩展
	// 实际日志通过 logger 记录
}

// GetLogs 获取所有日志
func (c *LogCollector) GetLogs() []stypes.ConsoleLog {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.logger == nil {
		return nil
	}

	entries := c.logger.List()
	if len(entries) == 0 {
		return nil
	}

	logs := make([]stypes.ConsoleLog, 0, len(entries))
	for _, en := range entries {
		logs = append(logs, stypes.ConsoleLog{
			Ts:      en.Ts,
			Level:   en.Level,
			Message: en.Message,
		})
	}
	return logs
}

// GetLogStats 获取日志统计信息
func (c *LogCollector) GetLogStats() (total int, maxCache int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.logger == nil {
		return 0, 0
	}

	entries := c.logger.List()
	maxCache = c.logger.Max()
	return len(entries), maxCache
}
