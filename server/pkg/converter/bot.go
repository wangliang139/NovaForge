package converter

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/wangliang139/NovaForge/server/pkg/repos/bot"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// BotPo2Types 将数据库模型转换为类型
func BotPo2Types(po *bot.Bot) (*stypes.Bot, error) {
	if po == nil {
		return nil, nil
	}

	// 解析 Config（包含 params 和 signals）
	var config stypes.BotConfig
	if len(po.Config) > 0 {
		if err := sonic.Unmarshal(po.Config, &config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal bot config: %w", err)
		}
	}

	symbols := make([]ctypes.Symbol, 0)
	for _, symbolString := range po.Symbols {
		symbol, err := ctypes.ParseSymbol(symbolString)
		if err != nil {
			return nil, fmt.Errorf("failed to parse symbol: %w", err)
		}
		symbols = append(symbols, symbol)
	}

	exchange, err := ctypes.ParseExchange(po.Exchange)
	if err != nil {
		return nil, fmt.Errorf("failed to parse exchange: %w", err)
	}

	return &stypes.Bot{
		ID:           po.ID,
		StrategyID:   po.StrategyID,
		StrategyVer:  po.StrategyVersion,
		AccountID:    po.AccountID,
		Exchange:     exchange,
		Mode:         stypes.BotMode(po.Mode),
		Name:         po.Name,
		Description:  po.Desc,
		Config:       config,
		Symbols:      symbols,
		Status:       stypes.BotStatus(po.Status),
		CreatedAt:    po.CreatedAt,
		UpdatedAt:    po.UpdatedAt,
		ErrorMessage: po.ErrorMessage,
	}, nil
}
