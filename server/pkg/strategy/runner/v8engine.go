package runner

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/misc"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/mow/logger"
	"rogchap.com/v8go"
)

// jsTask 表示一个 JS 执行任务
type jsTask struct {
	taskType string // "call", "init", "signal"
	funcName string
	args     []any
	signal   stypes.Signal
	resultCh chan jsTaskResult
	ctx      context.Context
}

// jsTaskResult 表示任务执行结果
type jsTaskResult struct {
	value *v8go.Value
	err   error
}

// V8Engine V8 JS引擎封装（单线程执行模型）
type V8Engine struct {
	vm      *v8go.Context
	isolate *v8go.Isolate
	sandbox *Sandbox
	runtime *Runtime
	mu      sync.RWMutex
	closed  bool

	logger logging.Logger

	// 单线程执行队列
	taskQueue    chan *jsTask
	workerCtx    context.Context
	workerCancel context.CancelFunc
	workerDone   chan struct{}
}

// NewV8Engine 创建新的V8引擎实例（单线程执行模型）
func NewV8Engine(code string, sandbox *Sandbox, runtime *Runtime, logger logging.Logger) (*V8Engine, error) {
	if sandbox == nil {
		sandbox = DefaultSandbox()
	}

	iso := v8go.NewIsolate()
	ctx := v8go.NewContext(iso)

	// 注入运行时API
	if runtime != nil {
		if err := runtime.Inject(ctx); err != nil {
			ctx.Close()
			iso.Dispose()
			return nil, fmt.Errorf("failed to inject runtime: %w", err)
		}
	}

	// 执行策略代码
	_, err := ctx.RunScript(code, "strategy.js")
	if err != nil {
		ctx.Close()
		iso.Dispose()
		return nil, fmt.Errorf("failed to execute strategy code: %w", err)
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	engine := &V8Engine{
		vm:           ctx,
		isolate:      iso,
		sandbox:      sandbox,
		runtime:      runtime,
		logger:       logger,
		taskQueue:    make(chan *jsTask, 1024),
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
		workerDone:   make(chan struct{}),
	}

	// 启动单线程 worker
	go engine.worker()

	return engine, nil
}

// worker 单线程处理所有 JS 调用（保证串行执行）
// V8 Isolate 必须由同一 OS 线程访问，LockOSThread 将本 goroutine 绑定到当前线程，避免 v8go 跨线程访问崩溃。
func (e *V8Engine) worker() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(e.workerDone)

	for {
		select {
		case <-e.workerCtx.Done():
			// 处理剩余任务
			e.drainTasks()
			return
		case task, ok := <-e.taskQueue:
			if !ok {
				return
			}
			if task == nil {
				continue
			}
			e.executeTask(task)
		}
	}
}

// drainTasks 排空剩余任务
func (e *V8Engine) drainTasks() {
	for {
		select {
		case task, ok := <-e.taskQueue:
			if !ok {
				return
			}
			if task != nil {
				task.resultCh <- jsTaskResult{nil, fmt.Errorf("engine is closing")}
				close(task.resultCh)
			}
		default:
			return
		}
	}
}

// executeTask 执行单个任务（带超时中断）
func (e *V8Engine) executeTask(task *jsTask) {
	defer func() {
		if r := recover(); r != nil {
			task.resultCh <- jsTaskResult{nil, fmt.Errorf("panic in JS execution: %v", r)}
		}
		close(task.resultCh)
	}()

	e.mu.RLock()
	if e.closed || e.vm == nil {
		e.mu.RUnlock()
		task.resultCh <- jsTaskResult{nil, fmt.Errorf("engine is closed")}
		return
	}
	vm := e.vm
	isolate := e.isolate
	e.mu.RUnlock()

	// 创建带超时的 context
	execCtx, cancel := context.WithTimeout(task.ctx, e.sandbox.MaxCPU)
	defer cancel()

	// 在单独的 goroutine 中执行，主 goroutine 监控超时
	done := make(chan jsTaskResult, 1)
	go func() {
		var result jsTaskResult
		switch task.taskType {
		case "call":
			val, err := e.doCallFunction(task.ctx, vm, task.funcName, task.args...)
			result = jsTaskResult{val, err}
		case "init":
			err := e.doOnInit(task.ctx, vm)
			result = jsTaskResult{nil, err}
		case "signal":
			err := e.doOnSignal(task.ctx, vm, task.signal)
			result = jsTaskResult{nil, err}
		default:
			result = jsTaskResult{nil, fmt.Errorf("unknown task type: %s", task.taskType)}
		}
		select {
		case done <- result:
		case <-execCtx.Done():
		}
	}()

	select {
	case result := <-done:
		// 正常完成
		task.resultCh <- result
	case <-execCtx.Done():
		// 超时，终止执行
		log.Warn().
			Str("task_type", task.taskType).
			Str("func_name", task.funcName).
			Dur("timeout", e.sandbox.MaxCPU).
			Msg("JS execution timeout, terminating")

		// 终止 V8 执行
		isolate.TerminateExecution()

		// 必须等待执行 goroutine 退出后再继续，否则 Close 时 dispose isolate 会导致 use-after-free / 崩溃
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			log.Warn().Msg("timeout waiting for JS goroutine to exit after TerminateExecution")
		}

		task.resultCh <- jsTaskResult{nil, fmt.Errorf("execution timeout after %v (engine disabled)", e.sandbox.MaxCPU)}
	}
}

// doCallFunction 实际执行函数调用（在 worker 中）
func (e *V8Engine) doCallFunction(ctx context.Context, vm *v8go.Context, funcName string, args ...any) (*v8go.Value, error) {
	// 转换参数
	v8Args := make([]v8go.Valuer, len(args))
	for i, arg := range args {
		v8Val, err := misc.AnyToV8Value(vm, arg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert argument %d: %w", i, err)
		}
		v8Args[i] = v8Val
	}

	// 调用函数
	fn, err := vm.Global().Get(funcName)
	if err != nil {
		return nil, fmt.Errorf("function %s not found: %w", funcName, err)
	}

	fnObj, err := fn.AsFunction()
	if err != nil {
		return nil, fmt.Errorf("%s is not a function", funcName)
	}

	result, err := fnObj.Call(vm.Global(), v8Args...)
	return result, err
}

// OnSignal 处理信号（通过任务队列）
func (e *V8Engine) OnSignal(ctx context.Context, signal stypes.Signal) error {
	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return fmt.Errorf("engine is closed")
	}
	e.mu.RUnlock()

	task := &jsTask{
		taskType: "signal",
		signal:   signal,
		resultCh: make(chan jsTaskResult, 1),
		ctx:      ctx,
	}

	// 任务入队超时
	enqueueCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	select {
	case e.taskQueue <- task:
		// 等待结果（超时由 executeTask 处理）
		result := <-task.resultCh
		return result.err
	case <-enqueueCtx.Done():
		return fmt.Errorf("failed to enqueue task: timeout")
	}
}

// doOnSignal 实际处理信号（在 worker 中）
func (e *V8Engine) doOnSignal(ctx context.Context, vm *v8go.Context, signal stypes.Signal) error {
	handlerName := "onSignal"

	// check if onSignal is a function
	val, err := vm.Global().Get(handlerName)
	if err != nil {
		return fmt.Errorf("failed to access %s: %w", handlerName, err)
	}
	if val == nil || !val.IsFunction() {
		return fmt.Errorf("%s is not a function", handlerName)
	}

	_, err = e.doCallFunction(ctx, vm, handlerName, signal)
	if err != nil {
		logger.Ctx(ctx).Err(err).
			Str("handler", handlerName).
			Str("signal_type", signal.GetType().String()).
			Msg("failed to call signal handler")
		return fmt.Errorf("failed to call %s: %w", handlerName, err)
	}

	return nil
}

// OnInit 调用初始化函数（通过任务队列）
func (e *V8Engine) OnInit(ctx context.Context) error {
	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return fmt.Errorf("engine is closed")
	}
	e.mu.RUnlock()

	task := &jsTask{
		taskType: "init",
		resultCh: make(chan jsTaskResult, 1),
		ctx:      ctx,
	}

	// 任务入队超时
	enqueueCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	select {
	case e.taskQueue <- task:
		// 等待结果（超时由 executeTask 处理）
		result := <-task.resultCh
		return result.err
	case <-enqueueCtx.Done():
		return fmt.Errorf("failed to enqueue task: timeout")
	}
}

// doOnInit 实际执行初始化（在 worker 中）
func (e *V8Engine) doOnInit(ctx context.Context, vm *v8go.Context) error {
	val, err := vm.Global().Get("onInit")
	if err != nil {
		// 获取全局变量出错，说明环境可能有问题
		return fmt.Errorf("failed to access onInit: %w", err)
	}
	if val == nil || !val.IsFunction() {
		// onInit 未定义或不是函数，直接跳过
		return nil
	}

	_, err = e.doCallFunction(ctx, vm, "onInit")
	if err != nil {
		e.logger.Errorf("call onInit failed: %s", err.Error())
	}
	return nil
}

// Close 关闭引擎
func (e *V8Engine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.mu.Unlock()

	// 停止 worker
	e.workerCancel()
	close(e.taskQueue)

	// 等待 worker 结束
	<-e.workerDone

	// 清理 V8 资源
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.vm != nil {
		e.vm.Close()
		e.vm = nil
	}

	if e.isolate != nil {
		e.isolate.Dispose()
		e.isolate = nil
	}

	return nil
}

// IsClosed 返回引擎是否已关闭
func (e *V8Engine) IsClosed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closed
}
