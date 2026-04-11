package restart

import (
	"os"
	"strings"
	"syscall"
	"time"
)

// SigtermRestartEnabled 是否允许通过 GraphQL 触发当前进程 SIGTERM 退出（默认关闭）。
// 环境变量：ENABLE_SIGTERM_RESTART=true 或 1。
func SigtermRestartEnabled() bool {
	v := strings.TrimSpace(os.Getenv("ENABLE_SIGTERM_RESTART"))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ScheduleSigtermToCurrentProcess 在短暂延迟后向当前进程发送 SIGTERM，便于 HTTP 响应先返回客户端。
func ScheduleSigtermToCurrentProcess() {
	go func() {
		time.Sleep(400 * time.Millisecond)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}()
}
