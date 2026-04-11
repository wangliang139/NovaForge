package runner

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"rogchap.com/v8go"
)

// 内置三方库：放在 pkg/strategy/runner/libs 下，编译期打包进二进制。
// 当前包含：decimal.js、trading-signals.cjs、trading-signals-decimal.js、lodash.js、moment.js、indicator.js 等。
// 策略中通过 require("indicator") 使用技术指标库。
//
//go:embed libs/*.js
var embeddedLibFS embed.FS

var (
	embeddedLibOnce sync.Once
	embeddedLibs    map[string]string // name -> source
	embeddedLibErr  error
)

func loadEmbeddedLibs() (map[string]string, error) {
	embeddedLibOnce.Do(func() {
		embeddedLibs = make(map[string]string)
		entries, err := embeddedLibFS.ReadDir("libs")
		if err != nil {
			embeddedLibErr = err
			return
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			name := ent.Name()
			// 支持 .js 和 .cjs 后缀
			if !strings.HasSuffix(name, ".js") && !strings.HasSuffix(name, ".cjs") {
				continue
			}
			b, err := embeddedLibFS.ReadFile(filepath.Join("libs", name))
			if err != nil {
				embeddedLibErr = err
				return
			}
			src := string(b)
			// 注册完整文件名
			embeddedLibs[name] = src
			// 兼容无后缀引用：require("decimal") -> decimal.js
			embeddedLibs[strings.TrimSuffix(name, ".js")] = src
			embeddedLibs[strings.TrimSuffix(name, ".cjs")] = src
		}
	})
	if embeddedLibErr != nil {
		return nil, embeddedLibErr
	}
	return embeddedLibs, nil
}

func normalizeModuleName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, `"'`)
	// 允许 require("./decimal.js") 这种写法：只取最后一个 path 段
	name = filepath.Base(name)
	return name
}

// injectRequire 注入一个安全的 CommonJS require()：
// - 只允许加载系统内置 libs/*.js（随服务发布的白名单）
// - 带模块缓存（同一模块只执行一次）
func (r *Runtime) injectRequire(ctx *v8go.Context) error {
	iso := ctx.Isolate()
	global := ctx.Global()

	libs, err := loadEmbeddedLibs()
	if err != nil {
		return fmt.Errorf("load embedded libs: %w", err)
	}

	var (
		mu    sync.Mutex
		cache = make(map[string]*v8go.Value)
	)

	tpl := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		defer info.Release()

		args := info.Args()
		if len(args) < 1 {
			msg, _ := v8go.NewValue(iso, "require(name) missing module name")
			return iso.ThrowException(msg)
		}

		name := normalizeModuleName(args[0].String())
		mu.Lock()
		if v, ok := cache[name]; ok {
			mu.Unlock()
			return v
		}
		// 先放一个占位 exports，避免递归 require 时死循环（最小实现）
		exportsObj, _ := v8go.NewObjectTemplate(iso).NewInstance(ctx)
		cache[name] = exportsObj.Value
		mu.Unlock()

		src, ok := libs[name]
		if !ok {
			msg, _ := v8go.NewValue(iso, fmt.Sprintf("Cannot find module '%s'", name))
			return iso.ThrowException(msg)
		}

		moduleObj, _ := v8go.NewObjectTemplate(iso).NewInstance(ctx)
		_ = moduleObj.Set("exports", exportsObj.Value)

		// 通过 IIFE 方式执行模块：CommonJS wrapper
		wrapperCode := "(function(exports, module, require){\n" + src + "\n;return module.exports;})"
		fnVal, runErr := ctx.RunScript(wrapperCode, name)
		if runErr != nil {
			msg, _ := v8go.NewValue(iso, runErr.Error())
			return iso.ThrowException(msg)
		}
		fn, asErr := fnVal.AsFunction()
		if asErr != nil {
			msg, _ := v8go.NewValue(iso, fmt.Sprintf("invalid module wrapper for '%s'", name))
			return iso.ThrowException(msg)
		}

		reqVal, _ := global.Get("require")
		ret, callErr := fn.Call(global, exportsObj.Value, moduleObj.Value, reqVal)
		if callErr != nil {
			msg, _ := v8go.NewValue(iso, callErr.Error())
			return iso.ThrowException(msg)
		}
		if ret == nil {
			ret, _ = moduleObj.Get("exports")
		}

		mu.Lock()
		cache[name] = ret
		mu.Unlock()
		return ret
	})

	fn := tpl.GetFunction(ctx)
	global.Set("require", fn)
	// 保护 require 函数，防止被覆盖
	protectGlobalProperty(ctx, "require")

	return nil
}
