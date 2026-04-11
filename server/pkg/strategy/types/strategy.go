package types

import (
	"time"
)

// StrategyStatus 策略状态
type StrategyStatus string

const (
	StrategyStatusDraft    StrategyStatus = "draft"
	StrategyStatusActive   StrategyStatus = "active"
	StrategyStatusInactive StrategyStatus = "inactive"
)

func (s StrategyStatus) Valid() bool {
	switch s {
	case StrategyStatusDraft, StrategyStatusActive, StrategyStatusInactive:
		return true
	}
	return false
}

// Strategy 策略定义
type Strategy struct {
	ID          string
	Name        string
	Description string
	Code        string // JS代码
	Version     string
	Params      []StrategyParam
	// Signals 策略需求信号定义
	Signals   []SignalDefinition
	Status    StrategyStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// StrategyParam 策略参数定义
type StrategyParam struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Type        StrategyParamType `json:"type,omitempty"`
	Required    bool              `json:"required,omitempty"`
	Default     any               `json:"default,omitempty"`
}

type StrategyParamType string

const (
	StrategyParamTypeString StrategyParamType = "string"
	StrategyParamTypeNumber StrategyParamType = "number"
	StrategyParamTypeBool   StrategyParamType = "bool"
	StrategyParamTypeObject StrategyParamType = "object"

	StrategyParamTypeArrayString StrategyParamType = "[]string"
	StrategyParamTypeArrayNumber StrategyParamType = "[]number"
	StrategyParamTypeArrayBool   StrategyParamType = "[]bool"
	StrategyParamTypeArrayObject StrategyParamType = "[]object"
)

func (t StrategyParamType) Valid() bool {
	switch t {
	case StrategyParamTypeString, StrategyParamTypeNumber, StrategyParamTypeBool, StrategyParamTypeObject, StrategyParamTypeArrayString, StrategyParamTypeArrayNumber, StrategyParamTypeArrayBool, StrategyParamTypeArrayObject:
		return true
	}
	return false
}

func (t StrategyParamType) String() string {
	return string(t)
}
