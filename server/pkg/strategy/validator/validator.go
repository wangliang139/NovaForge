package validator

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"rogchap.com/v8go"
)

// ValidateStrategyCode 校验策略 JS 代码的语法和必要入口函数
// 返回错误表示校验失败，nil 表示通过
func ValidateStrategyCode(code string) error {
	if strings.TrimSpace(code) == "" {
		return fmt.Errorf("strategy code is required")
	}

	// 创建隔离的 V8 环境
	iso := v8go.NewIsolate()
	defer iso.Dispose()

	ctx := v8go.NewContext(iso)
	defer ctx.Close()

	// 使用超时机制执行校验，防止死循环
	const validationTimeout = 3 * time.Second
	timeoutCtx, cancel := context.WithTimeout(context.Background(), validationTimeout)
	defer cancel()

	errCh := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		defer func() {
			if r := recover(); r != nil {
				select {
				case errCh <- fmt.Errorf("validation panic: %v", r):
				default:
				}
			}
		}()

		// 注入最小的 CommonJS 垫片，避免 require/module/exports 未定义导致校验误判失败
		// 注意：这里的 require 不做真实模块加载，仅用于让语法/入口函数校验通过。
		prelude := `
(function () {
  const __nfEmpty = new Proxy(function () {}, {
    get: function () { return __nfEmpty; },
    apply: function () { return __nfEmpty; },
    construct: function () { return __nfEmpty; },
  });

  if (typeof globalThis.require !== 'function') {
    globalThis.require = function () { return __nfEmpty; };
  }

  if (typeof globalThis.module === 'undefined' || !globalThis.module) {
    globalThis.module = { exports: {} };
  } else if (typeof globalThis.module.exports === 'undefined') {
    globalThis.module.exports = {};
  }

  if (typeof globalThis.exports === 'undefined') {
    globalThis.exports = globalThis.module.exports;
  }

  if (typeof globalThis.ai === 'undefined') {
    globalThis.ai = {
      complete: function () {
        return { result: "", json: null, model: "", duration: 0 };
      }
    };
  }
})();
`
		_, err := ctx.RunScript(prelude, "prelude.js")
		if err != nil {
			select {
			case errCh <- fmt.Errorf("failed to init validation runtime: %w", err):
			case <-timeoutCtx.Done():
			}
			return
		}

		// 执行脚本以触发语法解析（也会运行顶层代码）
		_, err = ctx.RunScript(code, "strategy.js")
		if err != nil {
			select {
			case errCh <- fmt.Errorf("syntax error: %w", err):
			case <-timeoutCtx.Done():
			}
			return
		}

		// 检查必要的入口函数是否存在
		// 使用 typeof 检查，避免 const/let 顶层绑定的可见性问题，并兼容 module.exports/exports 风格导出
		onInitExpr := "(typeof onInit === 'function') || (typeof module !== 'undefined' && module && module.exports && typeof module.exports.onInit === 'function') || (typeof exports !== 'undefined' && exports && typeof exports.onInit === 'function')"
		onInitCheck, err := ctx.RunScript(onInitExpr, "check_onInit.js")
		if err != nil {
			select {
			case errCh <- fmt.Errorf("failed to check onInit: %w", err):
			case <-timeoutCtx.Done():
			}
			return
		}
		if !onInitCheck.Boolean() {
			select {
			case errCh <- fmt.Errorf("missing required entry function: onInit"):
			case <-timeoutCtx.Done():
			}
			return
		}

		onSignalExpr := "(typeof onSignal === 'function') || (typeof module !== 'undefined' && module && module.exports && typeof module.exports.onSignal === 'function') || (typeof exports !== 'undefined' && exports && typeof exports.onSignal === 'function')"
		onSignalCheck, err := ctx.RunScript(onSignalExpr, "check_onSignal.js")
		if err != nil {
			select {
			case errCh <- fmt.Errorf("failed to check onSignal: %w", err):
			case <-timeoutCtx.Done():
			}
			return
		}
		if !onSignalCheck.Boolean() {
			select {
			case errCh <- fmt.Errorf("missing required entry function: onSignal"):
			case <-timeoutCtx.Done():
			}
			return
		}

		select {
		case errCh <- nil:
		case <-timeoutCtx.Done():
		}
	}()

	// 等待校验完成或超时
	select {
	case err := <-errCh:
		return err
	case <-timeoutCtx.Done():
		// 超时，终止执行
		iso.TerminateExecution()
		// 等待 goroutine 退出
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			// 如果等待超时，记录但继续
		}
		return fmt.Errorf("validation timeout: script execution took too long (possible infinite loop)")
	}
}
