package types

import (
	"time"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// BotMode Bot运行模式
type BotMode string

const (
	BotModeLive  BotMode = "live"  // 实盘
	BotModePaper BotMode = "paper" // 模拟盘
)

func (m BotMode) Valid() bool {
	switch m {
	case BotModeLive, BotModePaper:
		return true
	}
	return false
}

// BotStatus Bot状态
type BotStatus string

const (
	BotStatusStopped BotStatus = "stopped"
	BotStatusRunning BotStatus = "running"
	BotStatusError   BotStatus = "error"
)

func (s BotStatus) Valid() bool {
	switch s {
	case BotStatusStopped, BotStatusRunning, BotStatusError:
		return true
	}
	return false
}

// Bot Bot实例
type Bot struct {
	ID           int32
	StrategyID   string
	StrategyVer  string
	Exchange     ctypes.Exchange
	AccountID    string
	Mode         BotMode
	Name         string
	Description  string
	Config       BotConfig
	Symbols      []ctypes.Symbol
	Status       BotStatus
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type BotConfig struct {
	Params        map[string]any  `json:"params"`        // 用户提供的策略参数值
	Signals       []SignalBinding `json:"signals"`       // 信号绑定配置（将合并到 config 中）
	InitialAssets []ctypes.Asset  `json:"initialAssets"` // 初始资产，用于模拟盘
}

// CreateBotInput 创建Bot请求
type CreateBotInput struct {
	StrategyID  string
	StrategyVer string
	Exchange    ctypes.Exchange
	AccountID   string
	Mode        BotMode
	Name        string          // Bot 名称
	Description string          // Bot 描述（可选）
	Symbols     []ctypes.Symbol // 交易对列表
	Config      BotConfig       // Bot配置参数
}

// UpdateBotInput 更新Bot请求
type UpdateBotInput struct {
	ID          int32
	Name        string
	Description string
	Symbols     []string // 交易对列表
	Config      string   // JSON 字符串
}

// BotFilter Bot过滤条件
type BotFilter struct {
	ID             *int64
	StrategyID     *string
	Mode           *BotMode
	Status         *BotStatus
	Exchange       *string
	AccountID      *string
	Name           *string
	CreatedAtStart *int64
	CreatedAtEnd   *int64
}

type BotSnapshot struct {
	BotID     int64          `json:"bot_id"`
	Ts        int64          `json:"ts"`
	Variables map[string]any `json:"variables"`
}

type OrderState string
