package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config 对话 / ReAct 行为配置，环境变量前缀 CHAT_（如 CHAT_REACT_ENABLED）。
type Config struct {
	ReactEnabled bool `split_words:"true" default:"true"`

	MaxSteps     int `split_words:"true" default:"8"`
	MaxToolCalls int `split_words:"true" default:"16"`

	// ToolTimeoutSeconds 单次工具执行上限（秒）。
	ToolTimeoutSeconds int `split_words:"true" default:"300"`

	// ToolsPromptBudgetBytes / SkillsPromptBudgetBytes：写入 system 消息中 JSON 的字节上限（UTF-8）。
	// 0 表示不限制；超出时按 capability 降级为紧凑表示。
	ToolsPromptBudgetBytes  int `split_words:"true" default:"16384"`
	SkillsPromptBudgetBytes int `split_words:"true" default:"8192"`

	// MaxConsecutiveToolErrors 连续可恢复工具失败后，下一轮强制 tool_choice=none（0 表示关闭）。
	MaxConsecutiveToolErrors int `split_words:"true" default:"0"`
}

// Default 返回与 Load 一致的默认字段值（不读取环境变量）。
func Default() Config {
	c := Config{
		ReactEnabled:             true,
		MaxSteps:                 8,
		MaxToolCalls:             16,
		ToolTimeoutSeconds:       300,
		ToolsPromptBudgetBytes:   16384,
		SkillsPromptBudgetBytes:  8192,
		MaxConsecutiveToolErrors: 0,
	}
	c.normalize()
	return c
}

// Load 从环境变量读取（前缀 CHAT_）。
func Load() Config {
	var c Config
	envconfig.MustProcess("CHAT", &c)
	c.normalize()
	return c
}

func (c *Config) normalize() {
	if c.MaxSteps < 1 {
		c.MaxSteps = 8
	}
	if c.MaxSteps > 64 {
		c.MaxSteps = 64
	}
	if c.MaxToolCalls < 1 {
		c.MaxToolCalls = 16
	}
	if c.MaxToolCalls > 256 {
		c.MaxToolCalls = 256
	}
	if c.ToolTimeoutSeconds < 1 {
		c.ToolTimeoutSeconds = 8
	}
	if c.ToolTimeoutSeconds > 300 {
		c.ToolTimeoutSeconds = 300
	}
	if c.ToolsPromptBudgetBytes < 0 {
		c.ToolsPromptBudgetBytes = 0
	}
	if c.SkillsPromptBudgetBytes < 0 {
		c.SkillsPromptBudgetBytes = 0
	}
	if c.MaxConsecutiveToolErrors < 0 {
		c.MaxConsecutiveToolErrors = 0
	}
	if c.MaxConsecutiveToolErrors > 50 {
		c.MaxConsecutiveToolErrors = 50
	}
}

func (c *Config) ToolTimeout() time.Duration {
	return time.Duration(c.ToolTimeoutSeconds) * time.Second
}
