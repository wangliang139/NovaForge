package converter

import (
	"strings"

	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/action/model"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

func AccountStatusGql2TypesPtr(s *model.AccountStatus) *ctypes.AccountStatus {
	if s == nil || *s == model.AccountStatusUnspecified {
		return nil
	}
	st := ctypes.AccountStatus(strings.ToLower(string(*s)))
	if !st.Valid() {
		return nil
	}
	return &st
}

func AccountTypes2Gql(a *ctypes.Account) *model.Account {
	if a == nil {
		return nil
	}
	return &model.Account{
		ID:          a.ID,
		Name:        a.Name,
		Exchange:    a.Exchange,
		APIKey:      a.ApiKey,
		APISecret:   a.ApiSecret,
		Passphrase:  a.Passphrase,
		Tags:        a.Tags,
		Status:      model.AccountStatus(a.Status),
		Algorithm:   model.AuthAlgorithm(a.Algorithm),
		AccountType: model.AccountType(a.AccountType),
		Config:      RiskConfigTypes2Gql(a.Config),
		CreatedAt:   int(a.CreatedAt.Unix()),
		UpdatedAt:   int(a.UpdatedAt.Unix()),
	}
}

func RiskConfigTypes2Gql(cfg *ctypes.RiskConfig) *model.AccountConfig {
	if cfg == nil {
		return &model.AccountConfig{}
	}
	toAmt := func(l ctypes.AmountLimit) *model.AmountLimit {
		if l.Amount.IsZero() && l.Ratio.IsZero() {
			return nil
		}
		return &model.AmountLimit{
			Amount: lo.ToPtr(l.Amount.String()),
			Ratio:  lo.ToPtr(l.Ratio.String()),
		}
	}
	var maxOrderSize *string
	if !cfg.MaxOrderSize.IsZero() {
		maxOrderSize = lo.ToPtr(cfg.MaxOrderSize.String())
	}
	var maxLev *string
	if !cfg.MaxLeverage.IsZero() {
		maxLev = lo.ToPtr(cfg.MaxLeverage.String())
	}
	var maxOPM *int
	if cfg.MaxOrdersPerMinute > 0 {
		maxOPM = lo.ToPtr(cfg.MaxOrdersPerMinute)
	}
	var minMMR *string
	if !cfg.MinMaintenanceMarginRatio.IsZero() {
		minMMR = lo.ToPtr(cfg.MinMaintenanceMarginRatio.String())
	}
	var riskIdxTh *string
	if !cfg.RiskIndexThreshold.IsZero() {
		riskIdxTh = lo.ToPtr(cfg.RiskIndexThreshold.String())
	}
	var riskAct *string
	if strings.TrimSpace(cfg.RiskIndexAction) != "" {
		riskAct = lo.ToPtr(strings.TrimSpace(cfg.RiskIndexAction))
	}
	var cooldown *int
	if cfg.CooldownSeconds > 0 {
		v := int(cfg.CooldownSeconds)
		cooldown = &v
	}
	return &model.AccountConfig{
		MaxOrderSize:              maxOrderSize,
		MaxPositionPerSymbol:      toAmt(cfg.MaxPositionPerSymbol),
		MaxDailyLoss:              toAmt(cfg.MaxDailyLoss),
		MaxLeverage:               maxLev,
		MaxOrdersPerMinute:        maxOPM,
		MinMaintenanceMarginRatio: minMMR,
		MaxTotalNetExposure:       toAmt(cfg.MaxTotalNetExposure),
		MaxTotalGrossExposure:     toAmt(cfg.MaxTotalGrossExposure),
		RiskIndexThreshold:        riskIdxTh,
		RiskIndexAction:           riskAct,
		CooldownSeconds:           cooldown,
	}
}

func BalanceTypes2Gql(b *ctypes.Balance) *model.Balance {
	if b == nil {
		return nil
	}
	assets := make([]*model.Asset, 0, len(b.Assets))
	for _, a := range b.Assets {
		if a == nil {
			continue
		}
		assets = append(assets, &model.Asset{
			Code:             a.Code,
			Balance:          a.Balance.String(),
			Locked:           a.Locked.String(),
			Notional:         a.Notional.String(),
			UnRealizedProfit: a.UnRealizedProfit.String(),
			WalletType:       WalletTypeTypes2Gql(a.WalletType),
			UpdatedTs:        int(a.UpdatedTs.UnixMilli()),
		})
	}
	return &model.Balance{
		Notional:          b.Notional.String(),
		UnRealizedProfit:  b.UnRealizedProfit.String(),
		Notional24HChange: b.Notional24HChange.String(),
		Assets:            assets,
	}
}

func WalletTypeTypes2Gql(w ctypes.WalletType) model.WalletType {
	switch w {
	case ctypes.WalletTypeFund:
		return model.WalletTypeFund
	case ctypes.WalletTypeTrade:
		return model.WalletTypeTrade
	case ctypes.WalletTypeSpot:
		return model.WalletTypeSpot
	case ctypes.WalletTypeFuture:
		return model.WalletTypeFuture
	case ctypes.WalletTypeMargin:
		return model.WalletTypeMargin
	default:
		return model.WalletTypeUnspecified
	}
}

func WalletTypeGql2Types(walletType *model.WalletType) *ctypes.WalletType {
	if walletType == nil {
		return nil
	}
	switch *walletType {
	case model.WalletTypeFund:
		return lo.ToPtr(ctypes.WalletTypeFund)
	case model.WalletTypeTrade:
		return lo.ToPtr(ctypes.WalletTypeTrade)
	case model.WalletTypeSpot:
		return lo.ToPtr(ctypes.WalletTypeSpot)
	case model.WalletTypeFuture:
		return lo.ToPtr(ctypes.WalletTypeFuture)
	case model.WalletTypeMargin:
		return lo.ToPtr(ctypes.WalletTypeMargin)
	default:
		return nil
	}
}

func MarketAccountBo2Gql(b *ctypes.AccountBo) *model.MarketAccount {
	if b == nil {
		return nil
	}
	return &model.MarketAccount{
		Exchange:        b.Exchange,
		UID:             b.Uid,
		IsSpotEnabled:   b.IsSpotEnabled,
		IsFutureEnabled: b.IsFutureEnabled,
	}
}

// SymbolLeverageTypes2Gql 将运行时杠杆事件转为 GraphQL model.SymbolLeverage。
func SymbolLeverageTypes2Gql(sl *ctypes.SymbolLeverage) *model.SymbolLeverage {
	if sl == nil {
		return nil
	}
	side := model.PositionSideLong
	switch sl.Side {
	case ctypes.PositionSideShort:
		side = model.PositionSideShort
	}
	return &model.SymbolLeverage{
		Exchange:  sl.Exchange,
		Symbol:    sl.Symbol.String(),
		Side:      side,
		Leverage:  sl.Leverage,
		UpdatedTs: int(sl.UpdatedTs.UnixMilli()),
	}
}

func PositionTypes2Gql(p *ctypes.Position) *model.Position {
	if p == nil {
		return nil
	}
	side := model.PositionSideLong
	switch p.Side {
	case ctypes.PositionSideLong:
		side = model.PositionSideLong
	case ctypes.PositionSideShort:
		side = model.PositionSideShort
	}
	return &model.Position{
		Symbol:           p.Symbol.String(),
		Side:             side,
		Isolated:         p.Isolated,
		Amount:           p.Amount.String(),
		EntryPrice:       p.EntryPrice.String(),
		MarkPrice:        p.MarkPrice.String(),
		LiquidationPrice: p.LiquidationPrice.String(),
		Notional:         p.Notional.String(),
		Leverage:         p.Leverage,
		InitialMargin:    p.InitialMargin.String(),
		MaintMargin:      p.MaintMargin.String(),
		UnRealizedProfit: p.UnRealizedProfit.String(),
		UpdatedTs:        int(p.UpdatedTs.UnixMilli()),
	}
}

func LedgerTypes2Gql(l *ctypes.Ledger) *model.Ledger {
	if l == nil {
		return nil
	}
	return &model.Ledger{
		ID:          int(l.ID),
		AccountID:   l.AccountID,
		Exchange:    l.Exchange,
		Asset:       l.Asset,
		WalletType:  WalletTypeTypes2Gql(l.WalletType),
		Total:       l.Total.String(),
		Frozen:      l.Frozen.String(),
		TotalDelta:  l.TotalDelta.String(),
		FrozenDelta: l.FrozenDelta.String(),
		Type:        string(l.Type),
		Detail:      string(l.Detail),
		IsEffective: l.IsEffective,
		Ts:          int(l.Ts.UnixMilli()),
		CreatedAt:   int(l.CreatedAt.UnixMilli()),
	}
}

func EquityTypes2Gql(e *ctypes.Equity) *model.Equity {
	if e == nil {
		return nil
	}
	return &model.Equity{
		ID:               int(e.ID),
		AccountID:        e.AccountID,
		Ts:               int(e.Ts.UnixMilli()),
		Notional:         e.Notional.String(),
		UnRealizedProfit: e.UnRealizedProfit.String(),
		CreatedAt:        int(e.CreatedAt.Unix()),
	}
}

func EventFlowStreamGql2Types(s model.EventFlowStream) ctypes.EventFlowStream {
	switch s {
	case model.EventFlowStreamAccountRaw:
		return ctypes.EventFlowStreamAccountRaw
	case model.EventFlowStreamAccount:
		return ctypes.EventFlowStreamAccount
	case model.EventFlowStreamAll:
		return ctypes.EventFlowStreamAll
	default:
		return ctypes.EventFlowStreamUnspecified
	}
}

func AccountMetricsTypes2Model(resp *ctypes.QueryAccountMetricsResponse) *model.AccountMetrics {
	if resp == nil {
		return nil
	}
	symbols := make([]*model.SymbolMetrics, 0, len(resp.Symbols))
	for _, s := range resp.Symbols {
		if s == nil {
			continue
		}
		symbols = append(symbols, &model.SymbolMetrics{
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
		})
	}
	return &model.AccountMetrics{
		AccountID:             resp.AccountID,
		Dimension:             AccountAPIMetricsDim2Gql(resp.Dimension),
		Cagr:                  resp.Cagr,
		Sharpe:                resp.Sharpe,
		Sortino:               resp.Sortino,
		MaxDrawdown:           resp.MaxDrawdown,
		TimeUnderWaterSeconds: int(resp.TimeUnderWaterSeconds),
		Calmar:                resp.Calmar,
		WinRate:               resp.WinRate,
		ProfitFactor:          resp.ProfitFactor,
		RollingSharpe:         resp.RollingSharpe,
		AvgSlippageBps:        resp.AvgSlippageBps,
		FeeRatio:              resp.FeeRatio,
		MaxConsecutiveLoss:    int(resp.MaxConsecutiveLoss),
		StartTs:               int(resp.StartTs),
		EndTs:                 int(resp.EndTs),
		Symbols:               symbols,
	}
}

func AccountAPIMetricsDim2Gql(d ctypes.AccountAPIMetricsDimension) model.MetricsDimension {
	switch d {
	case ctypes.AccountAPIMetricsDimensionAccount:
		return model.MetricsDimensionAccount
	case ctypes.AccountAPIMetricsDimensionSymbol:
		return model.MetricsDimensionSymbol
	default:
		return model.MetricsDimensionUnspecified
	}
}

func MetricsDimensionGql2AccountAPI(d model.MetricsDimension) ctypes.AccountAPIMetricsDimension {
	switch d {
	case model.MetricsDimensionAccount:
		return ctypes.AccountAPIMetricsDimensionAccount
	case model.MetricsDimensionSymbol:
		return ctypes.AccountAPIMetricsDimensionSymbol
	default:
		return ctypes.AccountAPIMetricsDimensionUnspecified
	}
}

func AccountStatusTypes2Gql(status *types.AccountStatus) model.AccountStatus {
	if status == nil {
		return model.AccountStatusUnspecified
	}
	switch *status {
	case types.AccountStatusOnline:
		return model.AccountStatusOnline
	case types.AccountStatusOffline:
		return model.AccountStatusOffline
	default:
		return model.AccountStatusUnspecified
	}
}

func AccountStatusGql2Types(status *model.AccountStatus) *types.AccountStatus {
	if status == nil {
		return nil
	}
	switch *status {
	case model.AccountStatusOnline:
		return lo.ToPtr(types.AccountStatusOnline)
	case model.AccountStatusOffline:
		return lo.ToPtr(types.AccountStatusOffline)
	default:
		return nil
	}
}

func AuthAlgorithmTypes2Gql(algorithm types.AuthAlgorithm) model.AuthAlgorithm {
	switch algorithm {
	case types.AuthAlgorithmNone:
		return model.AuthAlgorithmNone
	case types.AuthAlgorithmHmac:
		return model.AuthAlgorithmHmac
	case types.AuthAlgorithmEd25519:
		return model.AuthAlgorithmEd25519
	case types.AuthAlgorithmRsa:
		return model.AuthAlgorithmRsa
	default:
		return model.AuthAlgorithmNone
	}
}

func AuthAlgorithmGql2Types(algorithm *model.AuthAlgorithm) *types.AuthAlgorithm {
	if algorithm == nil {
		return nil
	}
	switch *algorithm {
	case model.AuthAlgorithmNone:
		return lo.ToPtr(types.AuthAlgorithmNone)
	case model.AuthAlgorithmHmac:
		return lo.ToPtr(types.AuthAlgorithmHmac)
	case model.AuthAlgorithmEd25519:
		return lo.ToPtr(types.AuthAlgorithmEd25519)
	case model.AuthAlgorithmRsa:
		return lo.ToPtr(types.AuthAlgorithmRsa)
	default:
		return nil
	}
}

func AccountTypeTypes2Gql(accountType types.AccountType) model.AccountType {
	switch accountType {
	case types.AccountTypeReal:
		return model.AccountTypeReal
	case types.AccountTypeVirtual:
		return model.AccountTypeVirtual
	default:
		return model.AccountTypeUnspecified
	}
}

func PositionSideTypes2Gql(side types.PositionSide) model.PositionSide {
	switch side {
	case types.PositionSideShort:
		return model.PositionSideShort
	}
	return model.PositionSideLong
}

func PositionSideGql2Types(side model.PositionSide) types.PositionSide {
	switch side {
	case model.PositionSideShort:
		return types.PositionSideShort
	}
	return types.PositionSideLong
}
