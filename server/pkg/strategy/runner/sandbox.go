package runner

import (
	"time"
)

// Sandbox 沙箱配置
type Sandbox struct {
	MaxMemory   int64         // 最大内存（字节）
	MaxCPU      time.Duration // 最大CPU时间
	AllowedAPIs []string      // 允许的API列表
	BlockedAPIs []string      // 禁止的API列表
}

// DefaultSandbox 返回默认沙箱配置
func DefaultSandbox() *Sandbox {
	return &Sandbox{
		MaxMemory: 128 * 1024 * 1024, // 128MB
		MaxCPU:    5 * time.Second,   // 5秒
		AllowedAPIs: []string{
			"market", "order", "account", "indicator", "utils",
			"console", "JSON", "Math", "Date", "String", "Number",
			"require", // 仅允许加载内置/注册的安全库（由运行时实现限制）
		},
		BlockedAPIs: []string{
			"eval", "Function",
		},
	}
}

// ValidateAPI 验证API是否允许访问
func (s *Sandbox) ValidateAPI(apiName string) bool {
	// 检查是否在禁止列表中
	for _, blocked := range s.BlockedAPIs {
		if blocked == apiName {
			return false
		}
	}
	// 如果允许列表为空，则允许所有（除了禁止的）
	if len(s.AllowedAPIs) == 0 {
		return true
	}
	// 检查是否在允许列表中
	for _, allowed := range s.AllowedAPIs {
		if allowed == apiName {
			return true
		}
	}
	return false
}
