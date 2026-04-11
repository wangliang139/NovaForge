package types

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/mow/number"
)

// AmountLimit 表示同时支持绝对值和按权益比例的限额配置
type AmountLimit struct {
	Amount decimal.Decimal `json:"amount"` // 绝对值，0 表示未设置
	Ratio  decimal.Decimal `json:"ratio"`  // 比例 0–1，0 表示未设置
}

// EffectiveLimit 计算在给定 equity 下的实际限额：
// - 仅 amount>0 时返回 amount
// - 仅 ratio>0 时返回 equity*ratio
// - 两者都>0 时返回 min(amount, equity*ratio)
// - 均为 0 时返回 0（表示不限制，由调用方判断）
func (l AmountLimit) EffectiveLimit(equity decimal.Decimal) decimal.Decimal {
	hasAmount := l.Amount.GreaterThan(decimal.Zero)
	hasRatio := l.Ratio.GreaterThan(decimal.Zero)

	if !hasAmount && !hasRatio {
		return decimal.Zero
	}

	if hasAmount && !hasRatio {
		return l.Amount
	}

	if !hasAmount && hasRatio {
		return equity.Mul(l.Ratio)
	}

	limitByRatio := equity.Mul(l.Ratio)
	if l.Amount.LessThan(limitByRatio) {
		return l.Amount
	}
	return limitByRatio
}

// RiskConfig 聚合账户级风控配置
type RiskConfig struct {
	// 1. 单笔订单限额（绝对值）
	MaxOrderSize decimal.Decimal `json:"max_order_size"`

	// 2. 单标持仓限额（数量/比例）
	MaxPositionPerSymbol AmountLimit `json:"max_position_per_symbol"`

	// 3. 日亏损限额（数量/比例）
	MaxDailyLoss AmountLimit `json:"max_daily_loss"`

	// 4. 最大杠杆（倍数）
	MaxLeverage decimal.Decimal `json:"max_leverage"`

	// 5. 下单频率限制（每分钟最大订单数）
	MaxOrdersPerMinute int `json:"max_orders_per_minute"`

	// 6. 维持保证金率下限
	MinMaintenanceMarginRatio decimal.Decimal `json:"min_maintenance_margin_ratio"`

	// 7. 总净敞口限额（数量/比例）
	MaxTotalNetExposure AmountLimit `json:"max_total_net_exposure"`

	// 8. 总敞口限额（数量/比例）
	MaxTotalGrossExposure AmountLimit `json:"max_total_gross_exposure"`

	// 9. 风险指数阈值 & 动作
	RiskIndexThreshold decimal.Decimal `json:"risk_index_threshold"`
	RiskIndexAction    string          `json:"risk_index_action"` // "close_and_sell" 等

	// 10. 冷静期（秒），触发全平后，在冷静期结束前禁止加仓
	CooldownSeconds int32 `json:"cooldown_seconds"`
}

// AccountRiskConfigJSON 用于中间解析 config.risk 的 JSON 结构
type AccountRiskConfigJSON struct {
	MaxOrderSize              string            `json:"max_order_size"`
	MaxPositionPerSymbol      *AmountLimitJSON  `json:"max_position_per_symbol"`
	MaxDailyLoss              *AmountLimitJSON  `json:"max_daily_loss"`
	MaxLeverage               string            `json:"max_leverage"`
	MaxOrdersPerMinute        int               `json:"max_orders_per_minute"`
	MinMaintenanceMarginRatio string            `json:"min_maintenance_margin_ratio"`
	MaxTotalNetExposure       *AmountLimitJSON  `json:"max_total_net_exposure"`
	MaxTotalGrossExposure     *AmountLimitJSON  `json:"max_total_gross_exposure"`
	RiskIndexThreshold        string            `json:"risk_index_threshold"`
	RiskIndexAction           string            `json:"risk_index_action"`
	CooldownSeconds           int               `json:"cooldown_seconds"`
	Stablecoins               []string          `json:"stablecoins"`  // 可选：稳定币列表
	OnViolation               map[string]string `json:"on_violation"` // 可选：规则超限行为
	Extra                     map[string]any    `json:"-"`            // 预留
}

// AmountLimitJSON 对应 JSON 中 {amount, ratio} 结构
type AmountLimitJSON struct {
	Amount string `json:"amount"`
	Ratio  string `json:"ratio"`
}

// ParseRiskConfigFromJSON 解析 account.config 中的 risk JSON 为 AccountRiskConfig
func ParseRiskConfigFromJSON(b []byte) (*RiskConfig, error) {
	if len(b) == 0 {
		return nil, nil
	}

	fromAmountLimitJSON := func(src *AmountLimitJSON) AmountLimit {
		if src == nil {
			return AmountLimit{}
		}
		return AmountLimit{
			Amount: number.DecimalFromString(src.Amount),
			Ratio:  number.DecimalFromString(src.Ratio),
		}
	}

	var root map[string]json.RawMessage
	if err := sonic.Unmarshal(b, &root); err != nil {
		return nil, err
	}

	riskRaw, ok := root["risk"]
	if !ok || len(riskRaw) == 0 {
		return nil, nil
	}

	var cfgJSON AccountRiskConfigJSON
	if err := sonic.Unmarshal(riskRaw, &cfgJSON); err != nil {
		return nil, err
	}

	cfg := &RiskConfig{
		MaxOrderSize:              number.DecimalFromString(cfgJSON.MaxOrderSize),
		MaxPositionPerSymbol:      fromAmountLimitJSON(cfgJSON.MaxPositionPerSymbol),
		MaxDailyLoss:              fromAmountLimitJSON(cfgJSON.MaxDailyLoss),
		MaxLeverage:               number.DecimalFromString(cfgJSON.MaxLeverage),
		MaxOrdersPerMinute:        cfgJSON.MaxOrdersPerMinute,
		MinMaintenanceMarginRatio: number.DecimalFromString(cfgJSON.MinMaintenanceMarginRatio),
		MaxTotalNetExposure:       fromAmountLimitJSON(cfgJSON.MaxTotalNetExposure),
		MaxTotalGrossExposure:     fromAmountLimitJSON(cfgJSON.MaxTotalGrossExposure),
		RiskIndexThreshold:        number.DecimalFromString(cfgJSON.RiskIndexThreshold),
		RiskIndexAction:           cfgJSON.RiskIndexAction,
		CooldownSeconds:           int32(cfgJSON.CooldownSeconds),
	}

	return cfg, nil
}

// RiskConfigToJSON 将 AccountRiskConfig 序列化为适合写入 config.risk 的 JSON 字节
func RiskConfigToJSON(cfg *RiskConfig) ([]byte, error) {
	if cfg == nil {
		return sonic.Marshal(map[string]any{})
	}

	toAmountLimitJSON := func(l AmountLimit) *AmountLimitJSON {
		if l.Amount.IsZero() && l.Ratio.IsZero() {
			return nil
		}
		return &AmountLimitJSON{
			Amount: l.Amount.String(),
			Ratio:  l.Ratio.String(),
		}
	}

	risk := AccountRiskConfigJSON{
		MaxOrderSize:              cfg.MaxOrderSize.String(),
		MaxPositionPerSymbol:      toAmountLimitJSON(cfg.MaxPositionPerSymbol),
		MaxDailyLoss:              toAmountLimitJSON(cfg.MaxDailyLoss),
		MaxLeverage:               cfg.MaxLeverage.String(),
		MaxOrdersPerMinute:        cfg.MaxOrdersPerMinute,
		MinMaintenanceMarginRatio: cfg.MinMaintenanceMarginRatio.String(),
		MaxTotalNetExposure:       toAmountLimitJSON(cfg.MaxTotalNetExposure),
		MaxTotalGrossExposure:     toAmountLimitJSON(cfg.MaxTotalGrossExposure),
		RiskIndexThreshold:        cfg.RiskIndexThreshold.String(),
		RiskIndexAction:           cfg.RiskIndexAction,
		CooldownSeconds:           int(cfg.CooldownSeconds),
	}

	return sonic.Marshal(risk)
}

type RiskCheckResult struct {
	AccountID      string   `json:"account_id"`
	TriggeredRules []string `json:"triggered_rules,omitempty"`
	RiskIndex      string   `json:"risk_index,omitempty"`
	Success        bool     `json:"success"`
	Error          string   `json:"error,omitempty"`
	Action         *string  `json:"action,omitempty"`
}
