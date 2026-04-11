package converter

import (
	"github.com/bytedance/sonic"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/repos/datasource"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/NovaForge/server/pkg/types"
)

// DataSourceDb2Types 转换 DataSource 数据库模型到类型
func DataSourceDb2Types(ds *datasource.Datasource) (*types.DataSource, error) {
	var exchange *ctypes.Exchange
	var symbol *ctypes.Symbol
	var props map[string]any
	if len(ds.Exchange) > 0 {
		ex, err := ctypes.ParseExchange(ds.Exchange)
		if err != nil {
			return nil, err
		}
		exchange = &ex
	}
	if len(ds.Symbol) > 0 {
		sym, err := ctypes.ParseSymbol(ds.Symbol)
		if err != nil {
			return nil, err
		}
		symbol = &sym
	}
	if len(ds.Props) > 0 {
		var properties map[string]any
		if err := sonic.Unmarshal(ds.Props, &properties); err != nil {
			return nil, err
		}
		props = properties
	}
	return &types.DataSource{
		ID:          ds.ID,
		Type:        types.SignalType(ds.Type),
		Name:        ds.Name,
		Description: ds.Description,
		Exchange:    exchange,
		Symbol:      symbol,
		Props:       props,
		StartTs:     ds.StartTs,
		EndTs:       ds.EndTs,
		CreatedAt:   ds.CreatedAt,
		UpdatedAt:   ds.UpdatedAt,
	}, nil
}

func SignalType2StreamType(sigType stypes.SignalType) (ctypes.StreamType, bool) {
	switch sigType {
	case stypes.SignalTypeKline:
		return ctypes.StreamTypeKline, true
	case stypes.SignalTypeTicker:
		return ctypes.StreamTypeTicker, true
	case stypes.SignalTypeTrade:
		return ctypes.StreamTypeTrade, true
	case stypes.SignalTypeMarkPrice:
		return ctypes.StreamTypeMarkPrice, true
	case stypes.SignalTypeDepth:
		return ctypes.StreamTypeDepth, true
	case stypes.SignalTypeOrder,
		stypes.SignalTypePosition,
		stypes.SignalTypeBalance,
		stypes.SignalTypeFill,
		stypes.SignalTypeLeverage,
		stypes.SignalTypeRisk:
		return ctypes.StreamTypeAccount, true
	default:
		return "", false
	}
}
