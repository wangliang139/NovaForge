package converter

import (
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/wangliang139/NovaForge/server/pkg/action/model"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/errors"
)

func StrategyStatusTypes2Gql(status stypes.StrategyStatus) model.StrategyStatus {
	switch status {
	case stypes.StrategyStatusDraft:
		return model.StrategyStatusDraft
	case stypes.StrategyStatusActive:
		return model.StrategyStatusActive
	case stypes.StrategyStatusInactive:
		return model.StrategyStatusInactive
	default:
		return model.StrategyStatusUnspecified
	}
}

func StrategyStatusGql2Types(status model.StrategyStatus) stypes.StrategyStatus {
	switch status {
	case model.StrategyStatusDraft:
		return stypes.StrategyStatusDraft
	case model.StrategyStatusActive:
		return stypes.StrategyStatusActive
	case model.StrategyStatusInactive:
		return stypes.StrategyStatusInactive
	default:
		return ""
	}
}

func SignalTypeTypes2Gql(t stypes.SignalType) model.SignalType {
	switch t {
	case stypes.SignalTypeKline:
		return model.SignalTypeKline
	case stypes.SignalTypeTrade:
		return model.SignalTypeTrade
	case stypes.SignalTypeDepth:
		return model.SignalTypeDepth
	case stypes.SignalTypeTicker:
		return model.SignalTypeTicker
	case stypes.SignalTypeMarkPrice:
		return model.SignalTypeMarkPrice
	case stypes.SignalTypeSocial:
		return model.SignalTypeSocial
	case stypes.SignalTypeTimer:
		return model.SignalTypeTimer
	case stypes.SignalTypeOrder:
		return model.SignalTypeOrder
	case stypes.SignalTypePosition:
		return model.SignalTypePosition
	case stypes.SignalTypeBalance:
		return model.SignalTypeBalance
	case stypes.SignalTypeRisk:
		return model.SignalTypeRisk
	case stypes.SignalTypeSystem:
		return model.SignalTypeSystem
	default:
		return model.SignalTypeUnspecified
	}
}

func SignalTypeGql2Types(signalType model.SignalType) stypes.SignalType {
	switch signalType {
	case model.SignalTypeKline:
		return stypes.SignalTypeKline
	case model.SignalTypeTrade:
		return stypes.SignalTypeTrade
	case model.SignalTypeDepth:
		return stypes.SignalTypeDepth
	case model.SignalTypeTicker:
		return stypes.SignalTypeTicker
	case model.SignalTypeMarkPrice:
		return stypes.SignalTypeMarkPrice
	case model.SignalTypeSocial:
		return stypes.SignalTypeSocial
	case model.SignalTypeTimer:
		return stypes.SignalTypeTimer
	case model.SignalTypeOrder:
		return stypes.SignalTypeOrder
	case model.SignalTypePosition:
		return stypes.SignalTypePosition
	case model.SignalTypeBalance:
		return stypes.SignalTypeBalance
	case model.SignalTypeRisk:
		return stypes.SignalTypeRisk
	case model.SignalTypeSystem:
		return stypes.SignalTypeSystem
	default:
		return ""
	}
}

func SignalScopeTypes2Gql(scope ctypes.SignalScope) model.SignalScope {
	switch scope {
	case ctypes.SignalScopeSymbol:
		return model.SignalScopeSymbol
	case ctypes.SignalScopeTarget:
		return model.SignalScopeTarget
	case ctypes.SignalScopeExchange:
		return model.SignalScopeExchange
	case ctypes.SignalScopeStrategy:
		return model.SignalScopeStrategy
	default:
		return model.SignalScopeUnspecified
	}
}

func SignalScopeGql2Types(scope model.SignalScope) ctypes.SignalScope {
	switch scope {
	case model.SignalScopeSymbol:
		return ctypes.SignalScopeSymbol
	case model.SignalScopeTarget:
		return ctypes.SignalScopeTarget
	case model.SignalScopeExchange:
		return ctypes.SignalScopeExchange
	case model.SignalScopeStrategy:
		return ctypes.SignalScopeStrategy
	default:
		return ""
	}
}

func StrategyParamFromInput(p *model.StrategyParamInput) stypes.StrategyParam {
	if p == nil {
		return stypes.StrategyParam{}
	}
	return stypes.StrategyParam{
		Name:        p.Name,
		Description: p.Description,
		Type:        stypes.StrategyParamType(p.Type),
		Required:    p.Required,
		Default:     p.Default,
	}
}

func StrategyParamTypes2Gql(p stypes.StrategyParam) *model.StrategyParam {
	return &model.StrategyParam{
		Name:        p.Name,
		Description: lo.ToPtr(p.Description),
		Type:        p.Type.String(),
		Required:    p.Required,
		Default:     strategyParamDefaultTypes2Gql(p),
	}
}

func strategyParamDefaultTypes2Gql(p stypes.StrategyParam) *string {
	if p.Default == nil {
		return nil
	}
	switch p.Type {
	case stypes.StrategyParamTypeString:
		v, ok := p.Default.(string)
		if !ok {
			return nil
		}
		return &v
	case stypes.StrategyParamTypeNumber:
		b, err := sonic.Marshal(p.Default)
		if err != nil {
			return nil
		}
		return lo.ToPtr(string(b))
	case stypes.StrategyParamTypeBool:
		v, ok := p.Default.(bool)
		if !ok {
			return nil
		}
		return lo.ToPtr(strconv.FormatBool(v))
	case stypes.StrategyParamTypeObject, stypes.StrategyParamTypeArrayString, stypes.StrategyParamTypeArrayNumber,
		stypes.StrategyParamTypeArrayBool, stypes.StrategyParamTypeArrayObject:
		b, err := sonic.Marshal(p.Default)
		if err != nil {
			return nil
		}
		return lo.ToPtr(string(b))
	default:
		b, err := sonic.Marshal(p.Default)
		if err != nil {
			return nil
		}
		return lo.ToPtr(string(b))
	}
}

func StrategyParamTypes2Pb(source *stypes.StrategyParam) (*stypes.StrategyParam, error) {
	if !source.Type.Valid() {
		return nil, errors.New(errors.InvalidArgument, "type is invalid")
	}

	var defaultValue *string
	if source.Default != nil {
		switch source.Type {
		case stypes.StrategyParamTypeString:
			v, ok := source.Default.(string)
			if !ok {
				return nil, errors.New(errors.InvalidArgument, "default is invalid")
			}
			defaultValue = &v
		case stypes.StrategyParamTypeNumber:
			v, err := sonic.Marshal(source.Default)
			if err != nil {
				return nil, errors.New(errors.InvalidArgument, "default is invalid")
			}
			defaultValue = lo.ToPtr(string(v))
		case stypes.StrategyParamTypeBool:
			v, ok := source.Default.(bool)
			if !ok {
				return nil, errors.New(errors.InvalidArgument, "default is invalid")
			}
			defaultValue = lo.ToPtr(strconv.FormatBool(v))
		case stypes.StrategyParamTypeObject:
			v, err := sonic.Marshal(source.Default)
			if err != nil {
				return nil, errors.New(errors.InvalidArgument, "default is invalid")
			}
			defaultValue = lo.ToPtr(string(v))
		case stypes.StrategyParamTypeArrayString:
			v, err := sonic.Marshal(source.Default)
			if err != nil {
				return nil, errors.New(errors.InvalidArgument, "default is invalid")
			}
			defaultValue = lo.ToPtr(string(v))
		case stypes.StrategyParamTypeArrayNumber:
			v, err := sonic.Marshal(source.Default)
			if err != nil {
				return nil, errors.New(errors.InvalidArgument, "default is invalid")
			}
			defaultValue = lo.ToPtr(string(v))
		case stypes.StrategyParamTypeArrayBool:
			v, err := sonic.Marshal(source.Default)
			if err != nil {
				return nil, errors.New(errors.InvalidArgument, "default is invalid")
			}
			defaultValue = lo.ToPtr(string(v))
		case stypes.StrategyParamTypeArrayObject:
			v, err := sonic.Marshal(source.Default)
			if err != nil {
				return nil, errors.New(errors.InvalidArgument, "default is invalid")
			}
			defaultValue = lo.ToPtr(string(v))
		}
	}
	return &stypes.StrategyParam{
		Name:        source.Name,
		Description: source.Description,
		Type:        source.Type,
		Required:    source.Required,
		Default:     defaultValue,
	}, nil
}

func SignalDefinitionTypes2Gql(sig *stypes.SignalDefinition) *model.SignalDefinition {
	if sig == nil {
		return nil
	}
	var scope model.SignalScope = model.SignalScopeUnspecified
	if sig.Scope.Valid() && sig.Scope != "" {
		scope = SignalScopeTypes2Gql(sig.Scope)
	}
	var sym *string
	if sig.Symbol != nil {
		sym = lo.ToPtr(sig.Symbol.String())
	}
	var props *string
	if len(sig.Props) > 0 {
		if b, err := sonic.Marshal(sig.Props); err == nil {
			s := string(b)
			props = &s
		}
	}
	return &model.SignalDefinition{
		ID:       sig.ID,
		Type:     SignalTypeTypes2Gql(sig.Type),
		Exchange: sig.Exchange,
		Symbol:   sym,
		Props:    props,
		Scope:    lo.ToPtr(scope),
	}
}

func SignalDefinitionFromInput(sig *model.SignalDefinitionInput) (stypes.SignalDefinition, error) {
	var props map[string]any
	if sig.Props != nil && *sig.Props != "" {
		_ = sonic.Unmarshal([]byte(*sig.Props), &props)
	}
	var symbol *ctypes.Symbol
	if sig.Symbol != nil && *sig.Symbol != "" {
		sym, err := ctypes.ParseSymbol(*sig.Symbol)
		if err != nil {
			return stypes.SignalDefinition{}, err
		}
		symbol = &sym
	}
	scope := SignalScopeGql2Types(lo.FromPtr(sig.Scope))
	return stypes.SignalDefinition{
		ID:       sig.ID,
		Type:     SignalTypeGql2Types(sig.Type),
		Scope:    scope,
		Exchange: sig.Exchange,
		Symbol:   symbol,
		Props:    props,
	}, nil
}

func StrategyTypes2Gql(strategy *stypes.Strategy) *model.Strategy {
	if strategy == nil {
		return nil
	}
	params := make([]*model.StrategyParam, 0, len(strategy.Params))
	for i := range strategy.Params {
		params = append(params, StrategyParamTypes2Gql(strategy.Params[i]))
	}
	signals := make([]*model.SignalDefinition, 0, len(strategy.Signals))
	for i := range strategy.Signals {
		signals = append(signals, SignalDefinitionTypes2Gql(&strategy.Signals[i]))
	}
	return &model.Strategy{
		ID:          strategy.ID,
		Name:        strategy.Name,
		Description: strategy.Description,
		Code:        strategy.Code,
		Version:     strategy.Version,
		Params:      params,
		Status:      StrategyStatusTypes2Gql(strategy.Status),
		Signals:     signals,
		CreatedAt:   int(strategy.CreatedAt.Unix()),
		UpdatedAt:   int(strategy.UpdatedAt.Unix()),
	}
}

func StrategyInput2Types(input *model.StrategyInput) (*stypes.Strategy, error) {
	if input == nil {
		return nil, nil
	}
	params := make([]stypes.StrategyParam, 0, len(input.Params))
	for _, p := range input.Params {
		params = append(params, StrategyParamFromInput(p))
	}
	signals := make([]stypes.SignalDefinition, 0, len(input.Signals))
	for _, s := range input.Signals {
		signal, err := SignalDefinitionFromInput(s)
		if err != nil {
			return nil, err
		}
		signals = append(signals, signal)
	}
	return &stypes.Strategy{
		Name:    input.Name,
		Code:    input.Code,
		Params:  params,
		Signals: signals,
	}, nil
}

func RunBacktestInputGql2Types(input *model.RunBacktestInput) (*stypes.RunBacktestRequest, error) {
	if input == nil {
		return nil, nil
	}
	req := &stypes.RunBacktestRequest{
		RunType:   int32(input.RunType),
		StartTime: int64(input.StartTime),
		EndTime:   int64(input.EndTime),
		Params:    lo.FromPtr(input.Params),
	}
	if input.Strategy != nil {
		strategy, err := StrategyInput2Types(input.Strategy)
		if err != nil {
			return nil, err
		}
		req.Strategy = strategy
	} else if input.StrategyID != nil && input.Version != nil {
		req.Strategy = &stypes.Strategy{
			ID:      *input.StrategyID,
			Version: *input.Version,
		}
	}
	for _, sym := range input.Symbols {
		if sym == nil {
			return nil, errors.New(errors.InvalidArgument, "backtest symbols contains nil item")
		}
		symbol, err := ctypes.ParseSymbol(sym.Symbol)
		if err != nil {
			return nil, err
		}
		req.Symbols = append(req.Symbols, &stypes.BacktestSymbol{
			Exchange:      sym.Exchange,
			Symbol:        symbol,
			BaseAssetQty:  lo.FromPtr(sym.BaseAssetQty),
			QuoteAssetQty: lo.FromPtr(sym.QuoteAssetQty),
		})
	}
	for _, sig := range input.Signals {
		if sig == nil {
			continue
		}
		b := &stypes.SignalBinding{
			SignalID:     sig.SignalID,
			DatasourceID: int32(sig.DatasourceID),
		}
		if sig.Exchange != nil {
			b.Exchange = sig.Exchange
		}
		if sig.Symbol != nil && *sig.Symbol != "" {
			symbol, err := ctypes.ParseSymbol(*sig.Symbol)
			if err != nil {
				return nil, err
			}
			b.Symbol = &symbol
		}
		req.Signals = append(req.Signals, b)
	}
	return req, nil
}

func EquityPointTypes2Gql(ep *stypes.EquityPoint) *model.Equity {
	if ep == nil {
		return nil
	}
	return &model.Equity{
		Ts:               int(ep.Ts.UnixMilli()),
		Notional:         ep.TotalNetValue.String(),
		UnRealizedProfit: "",
	}
}

func SymbolSummaryTypes2Gql(sym *stypes.SymbolSummary) *model.SymbolSummary {
	if sym == nil {
		return nil
	}
	return &model.SymbolSummary{
		Exchange:           sym.ExSymbol.Exchange,
		Symbol:             sym.ExSymbol.Symbol.String(),
		Base:               sym.ExSymbol.Symbol.Base,
		Quote:              sym.ExSymbol.Symbol.Quote,
		InitialBase:        sym.InitialBase.String(),
		InitialQuote:       sym.InitialQuote.String(),
		FinalBase:          sym.FinalBase.String(),
		FinalQuote:         sym.FinalQuote.String(),
		PositionQty:        sym.PositionQty.String(),
		AvgPrice:           sym.AvgPrice.String(),
		LastPrice:          sym.LastPrice.String(),
		InitialNet:         sym.InitialNet.String(),
		FinalNet:           sym.FinalNet.String(),
		RealizedPnl:        sym.RealizedPnl.String(),
		UnrealizedPnl:      sym.UnrealizedPnl.String(),
		NetPnl:             sym.NetPnl.String(),
		LongRealizedPnl:    sym.LongRealizedPnl.String(),
		ShortRealizedPnl:   sym.ShortRealizedPnl.String(),
		LongUnrealizedPnl:  sym.LongUnrealizedPnl.String(),
		ShortUnrealizedPnl: sym.ShortUnrealizedPnl.String(),
		LongNetPnl:         sym.LongNetPnl.String(),
		ShortNetPnl:        sym.ShortNetPnl.String(),
		LongTrades:         sym.LongTrades,
		ShortTrades:        sym.ShortTrades,
	}
}

func RunBacktestResponseTypes2Gql(resp *stypes.RunBacktestResponse) *model.RunBacktestResponse {
	if resp == nil {
		return nil
	}
	data := &model.BacktestResultData{}
	if resp.Data != nil {
		for _, ep := range resp.Data.Equity {
			data.Equity = append(data.Equity, EquityPointTypes2Gql(&ep))
		}
		for _, sym := range resp.Data.Symbols {
			data.Symbols = append(data.Symbols, SymbolSummaryTypes2Gql(sym))
		}
		for _, o := range resp.Data.Orders {
			data.Orders = append(data.Orders, OrderTypes2Gql(o))
		}
		for _, t := range resp.Data.Trades {
			if t == nil {
				continue
			}
			side := model.PositionSideLong
			if t.Side == ctypes.PositionSideShort {
				side = model.PositionSideShort
			}
			data.Fills = append(data.Fills, &model.Fill{
				Exchange:      t.ExSymbol.Exchange,
				Symbol:        t.ExSymbol.Symbol.String(),
				OrderID:       string(t.OrderID),
				ClientOrderID: string(t.ClientOrderID),
				TradeID:       string(t.OrderID) + "-" + strconv.FormatInt(t.Ts.UnixMilli(), 10),
				Side:          side,
				IsBuy:         t.IsBuy,
				Qty:           t.Qty.String(),
				Price:         t.Price.String(),
				Fee:           t.Fee.String(),
				FeeAsset:      t.Asset,
				RealizedPnl:   t.RealizedPnl.String(),
				IsMaker:       false,
				Ts:            int(t.Ts.UnixMilli()),
			})
		}
		if len(resp.Data.Meta) > 0 {
			if b, err := sonic.Marshal(resp.Data.Meta); err == nil {
				s := string(b)
				data.MetaJSON = &s
			}
		}
	}
	consoleLogs := make([]*model.ConsoleLog, 0)
	if resp.Data != nil {
		for _, log := range resp.Data.ConsoleLogs {
			consoleLogs = append(consoleLogs, &model.ConsoleLog{
				Ts:      int(log.Ts.UnixMilli()),
				Level:   log.Level,
				Message: log.Message,
			})
		}
	}
	return &model.RunBacktestResponse{
		ID:             resp.ID,
		Strategy:       StrategyTypes2Gql(resp.Strategy),
		StartTime:      int(resp.StartTime.Unix()),
		EndTime:        int(resp.EndTime.Unix()),
		InitialBalance: resp.InitialBalance,
		FinalBalance:   resp.FinalBalance,
		TotalPnl:       resp.TotalPnl,
		TotalTrades:    resp.TotalTrades,
		WinTrades:      resp.WinTrades,
		LossTrades:     resp.LossTrades,
		WinRate:        resp.WinRate,
		SharpeRatio:    resp.SharpeRatio,
		MaxDrawdown:    resp.MaxDrawdown,
		Data:           data,
		CreatedAt:      int(resp.CreatedAt.Unix()),
		TimeCost:       int(resp.TimeCost),
		ConsoleLogs:    consoleLogs,
	}
}

func DataSourceTypes2Gql(ds *ctypes.DataSource) *model.DataSource {
	if ds == nil {
		return nil
	}
	return &model.DataSource{
		ID:          int(ds.ID),
		Type:        SignalTypeTypes2Gql(ds.Type),
		Name:        ds.Name,
		Description: ds.Description,
		Exchange:    ds.Exchange,
		Symbol:      SymbolPtrToStrPtr(ds.Symbol),
		Props:       propsMapToStrPtr(ds.Props),
		StartTs:     int(ds.StartTs.Unix()),
		EndTs:       int(ds.EndTs.Unix()),
		CreatedAt:   int(ds.CreatedAt.Unix()),
		UpdatedAt:   int(ds.UpdatedAt.Unix()),
	}
}

func SymbolPtrToStrPtr(s *ctypes.Symbol) *string {
	if s == nil {
		return nil
	}
	str := s.String()
	return &str
}

func propsMapToStrPtr(m map[string]any) *string {
	if len(m) == 0 {
		return nil
	}
	b, err := sonic.Marshal(m)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

func BotModeTypes2Gql(m stypes.BotMode) model.BotMode {
	switch m {
	case stypes.BotModeLive:
		return model.BotModeLive
	case stypes.BotModePaper:
		return model.BotModePaper
	default:
		return model.BotModePaper
	}
}

func BotStatusTypes2Gql(s stypes.BotStatus) model.BotStatus {
	switch s {
	case stypes.BotStatusStopped:
		return model.BotStatusStopped
	case stypes.BotStatusRunning:
		return model.BotStatusRunning
	case stypes.BotStatusError:
		return model.BotStatusError
	default:
		return model.BotStatusStopped
	}
}

func BotTypes2Gql(b *stypes.Bot) *model.Bot {
	if b == nil {
		return nil
	}
	symbols := make([]string, 0, len(b.Symbols))
	for _, s := range b.Symbols {
		symbols = append(symbols, s.String())
	}
	configStr, err := sonic.Marshal(b.Config)
	if err != nil {
		return nil
	}
	return &model.Bot{
		ID:           strconv.Itoa(int(b.ID)),
		StrategyID:   b.StrategyID,
		StrategyVer:  b.StrategyVer,
		Name:         b.Name,
		Description:  b.Description,
		Mode:         BotModeTypes2Gql(b.Mode),
		Exchange:     b.Exchange,
		AccountID:    b.AccountID,
		Symbols:      symbols,
		Config:       string(configStr),
		Status:       BotStatusTypes2Gql(b.Status),
		ErrorMessage: b.ErrorMessage,
		CreatedAt:    int(b.CreatedAt.Unix()),
	}
}

func BotLogTypes2Gql(log stypes.BotLogEntry) *model.BotLog {
	return &model.BotLog{
		ID:        strconv.FormatInt(log.ID, 10),
		BotID:     int(log.BotID),
		Level:     log.Level,
		Message:   log.Message,
		Ts:        int(log.Ts),
		CreatedAt: int(log.CreatedAt),
	}
}

func MetricsDimensionGql2Types(d model.MetricsDimension) stypes.MetricsDimension {
	switch d {
	case model.MetricsDimensionAccount:
		return stypes.MetricsDimensionAccount
	case model.MetricsDimensionSymbol:
		return stypes.MetricsDimensionSymbol
	default:
		return stypes.MetricsDimensionUnspecified
	}
}

func MetricsDimensionTypes2Gql(d stypes.MetricsDimension) model.MetricsDimension {
	switch d {
	case stypes.MetricsDimensionAccount:
		return model.MetricsDimensionAccount
	case stypes.MetricsDimensionSymbol:
		return model.MetricsDimensionSymbol
	default:
		return model.MetricsDimensionUnspecified
	}
}

func SymbolMetricsTypes2Gql(s stypes.SymbolMetrics) *model.SymbolMetrics {
	return &model.SymbolMetrics{
		Symbol:                s.Symbol,
		Exchange:              s.Exchange,
		Cagr:                  s.Cagr,
		Sharpe:                s.Sharpe,
		Sortino:               s.Sortino,
		MaxDrawdown:           s.MaxDrawdown,
		TimeUnderWaterSeconds: int(s.TimeUnderWaterSeconds),
		Calmar:                s.Calmar,
		WinRate:               s.WinRate,
		ProfitFactor:          s.ProfitFactor,
		RollingSharpe:         s.RollingSharpe,
		AvgSlippageBps:        s.AvgSlippageBps,
		FeeRatio:              s.FeeRatio,
		MaxConsecutiveLoss:    int(s.MaxConsecutiveLoss),
	}
}
