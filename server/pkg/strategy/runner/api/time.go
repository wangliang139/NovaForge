package api

import (
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	"rogchap.com/v8go"
)

// TimeAPI 时间API
type TimeAPI struct {
	clock clock.Clock
}

// NewTimeAPI 创建时间API
// nowFn: 时间提供器函数，回测场景传入 matchingEngine.CurrentTime，实盘场景传入 time.Now
func NewTimeAPI(clk clock.Clock) *TimeAPI {
	return &TimeAPI{
		clock: clk,
	}
}

// Now JS函数：获取当前时间（Unix时间戳，毫秒）
func (t *TimeAPI) Now(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	now := t.clock.Now()
	// 返回 Unix 时间戳（毫秒），与 JavaScript Date.now() 一致
	timestamp := now.UnixMilli()
	val, _ := v8go.NewValue(ctx.Isolate(), int64(timestamp))
	return val
}

// NowISO JS函数：获取当前时间（ISO 8601 字符串）
func (t *TimeAPI) NowISO(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	now := t.clock.Now()
	isoStr := now.Format(time.RFC3339Nano)
	val, _ := v8go.NewValue(ctx.Isolate(), isoStr)
	return val
}
