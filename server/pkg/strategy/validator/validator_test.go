package validator

import (
	"strings"
	"testing"
)

func TestValidateStrategyCode_SyntaxError(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "missing closing brace",
			code: `function onInit( {}`,
		},
		{
			name: "invalid token",
			code: `function onInit() { @#$ }`,
		},
		{
			name: "unclosed string",
			code: `function onInit() { console.log("test; }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStrategyCode(tt.code)
			if err == nil {
				t.Errorf("expected syntax error, got nil")
			}
			if !strings.Contains(err.Error(), "syntax error") {
				t.Errorf("expected 'syntax error' in error message, got: %v", err)
			}
		})
	}
}

func TestValidateStrategyCode_MissingEntryFunctions(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		missingFunc string
	}{
		{
			name: "missing onInit",
			code: `
function onSignal(signal) {
  console.log("signal received");
}
`,
			missingFunc: "onInit",
		},
		{
			name: "missing onSignal",
			code: `
function onInit() {
  console.log("initialized");
}
`,
			missingFunc: "onSignal",
		},
		{
			name: "missing both",
			code: `
var a = 1;
console.log("no entry functions");
`,
			missingFunc: "onInit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStrategyCode(tt.code)
			if err == nil {
				t.Errorf("expected missing function error, got nil")
			}
			if !strings.Contains(err.Error(), tt.missingFunc) {
				t.Errorf("expected '%s' in error message, got: %v", tt.missingFunc, err)
			}
		})
	}
}

func TestValidateStrategyCode_ValidCode(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "minimal valid strategy",
			code: `
function onInit() {
  console.log("init");
}

function onSignal(signal) {
  console.log("signal");
}
`,
		},
		{
			name: "strategy with variables and helpers",
			code: `
var state = { count: 0 };

function helper() {
  return state.count++;
}

function onInit() {
  console.log("Strategy initialized");
  helper();
}

function onSignal(signal) {
  console.log("Signal received:", signal.type);
  helper();
}
`,
		},
		{
			name: "arrow function entry points",
			code: `
const onInit = () => {
  console.log("init");
};

const onSignal = (signal) => {
  console.log("signal");
};
`,
		},
		{
			name: "strategy with require at top level",
			code: `
const helper = require("some-lib").utils.helper;

function onInit() {
  console.log("init");
}

function onSignal(signal) {
  console.log("signal");
}
`,
		},
		{
			name: "commonjs exports entry points",
			code: `
const dep = require("another-lib");

module.exports = {
  onInit: function () {
    console.log("init");
  },
  onSignal: function (signal) {
    console.log("signal");
  },
};
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStrategyCode(tt.code)
			if err != nil {
				t.Errorf("expected valid code, got error: %v", err)
			}
		})
	}
}

func TestValidateStrategyCode_EmptyCode(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "empty string",
			code: "",
		},
		{
			name: "whitespace only",
			code: "   \n\t  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStrategyCode(tt.code)
			if err == nil {
				t.Errorf("expected error for empty code, got nil")
			}
			if !strings.Contains(err.Error(), "required") {
				t.Errorf("expected 'required' in error message, got: %v", err)
			}
		})
	}
}

func TestValidateStrategyCode_Timeout(t *testing.T) {
	// 测试带有顶层无限循环的代码是否会被超时终止
	// 注意：函数体内的无限循环不会在 RunScript 阶段触发，只有顶层代码的无限循环才会
	code := `
// 顶层无限循环，会在 RunScript 时执行
while(true) {
  // infinite loop at top level
}

function onInit() {
  console.log("init");
}

function onSignal(signal) {
  console.log("signal");
}
`
	err := ValidateStrategyCode(code)
	if err == nil {
		t.Errorf("expected timeout error, got nil")
		return
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "timeout") && !strings.Contains(errStr, "terminated") {
		t.Errorf("expected 'timeout' or 'terminated' in error message, got: %v", err)
	}
}
