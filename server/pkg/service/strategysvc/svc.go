package strategysvc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/common/metrics"
	"github.com/wangliang139/NovaForge/server/pkg/entity"
	"github.com/wangliang139/NovaForge/server/pkg/internal/chsdk"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/service/accountsvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/llmsvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/ordersvc"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging/store"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/signalflow"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/validator"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/snowflake"
)

const (
	AiStrategySceneKey = "ai_strategy_generation"
)

type Service struct {
	db       *repos.Entity
	chClient *chsdk.Client

	accountSvc *accountsvc.Service
	orderSvc   *ordersvc.Service
	llmSvc     *llmsvc.Service
}

func New(db *repos.Entity, accountSvc *accountsvc.Service, orderSvc *ordersvc.Service, llmSvc *llmsvc.Service) (*Service, error) {
	chClient, err := chsdk.Connect(context.Background())
	if err != nil {
		return nil, err
	}
	return &Service{
		db:         db,
		chClient:   chClient,
		accountSvc: accountSvc,
		orderSvc:   orderSvc,
		llmSvc:     llmSvc,
	}, nil
}

func (s *Service) getBotAccountID(ctx context.Context, botID int32) (string, error) {
	if botID <= 0 {
		return "", errors.New(errors.InvalidArgument, "bot_id is required")
	}
	bot, err := entity.Strategy.GetBot(ctx, botID)
	if err != nil {
		return "", err
	}
	if bot == nil || strings.TrimSpace(bot.AccountID) == "" {
		return "", errors.New(errors.NotFound, "bot not found")
	}
	return bot.AccountID, nil
}

// ---- Bot scoped data proxy ----

func (s *Service) GetBotBalance(ctx context.Context, req *stypes.GetBotBalanceRequest) (*stypes.GetBotBalanceResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	accountID, err := s.getBotAccountID(ctx, req.BotID)
	if err != nil {
		return nil, err
	}
	withNotional := lo.ToPtr(true)
	if req.WithNotional != nil {
		withNotional = req.WithNotional
	}
	var wt *ctypes.WalletType
	if req.WalletType != nil {
		w := *req.WalletType
		if w.Valid() {
			wt = &w
		}
	}
	resp, err := s.accountSvc.GetBalance(ctx, &ctypes.GetBalanceRequest{
		AccountID:    accountID,
		WalletType:   wt,
		Asset:        req.Asset,
		WithNotional: withNotional,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Balance == nil {
		return &stypes.GetBotBalanceResponse{}, nil
	}
	return &stypes.GetBotBalanceResponse{Balance: resp.Balance}, nil
}

func (s *Service) GetBotPositions(ctx context.Context, req *stypes.GetBotPositionsRequest) (*stypes.GetBotPositionsResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	accountID, err := s.getBotAccountID(ctx, req.BotID)
	if err != nil {
		return nil, err
	}
	var mt *ctypes.MarketType
	if req.MarketType != nil {
		mt = req.MarketType
	}
	resp, err := s.accountSvc.GetPositions(ctx, &ctypes.GetPositionsRequest{
		AccountID:  accountID,
		MarketType: mt,
		Symbol:     req.Symbol,
	})
	if err != nil {
		return nil, err
	}
	return &stypes.GetBotPositionsResponse{Positions: resp.Positions}, nil
}

func (s *Service) GetBotState(ctx context.Context, req *stypes.GetBotStateRequest) (*stypes.GetBotStateResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if req.BotID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "bot_id is required")
	}

	bot, err := entity.Strategy.GetBot(ctx, req.BotID)
	if err != nil {
		return nil, err
	}
	if bot == nil {
		return nil, errors.New(errors.NotFound, "bot not found")
	}

	resp := &stypes.GetBotStateResponse{
		BotStatus: bot.Status,
	}

	executor, exists := entity.Strategy.GetRunningBotExecutor(req.BotID)
	if exists {
		state := executor.GetState()
		resp.ExecutorStatus = string(state.Status)
		runErr := state.RunErr
		if runErr != nil {
			resp.RunErr = runErr.Error()
		}
		resp.JsRunnerStatus = state.JsRunnerStatus
		resp.LastSignalTs = state.LastSignalTs
		resp.SignalAvgDurationMs = state.SignalAvgDurationMs
		resp.SignalAvgLatencyMs = state.SignalAvgLatencyMs

		snap := state.Portfolio
		botPortfolio := &stypes.BotPortfolio{
			Assets:    make([]stypes.BotPortfolioAsset, 0, len(snap.Balances)),
			Positions: make([]stypes.BotPortfolioPosition, 0, len(snap.Positions)),
			Ts:        time.Now().UnixMilli(),
		}

		for k, v := range snap.Balances {
			updatedTs := int64(0)
			if v.UpdateAt > 0 {
				updatedTs = time.Unix(0, v.UpdateAt).UnixMilli()
			}
			botPortfolio.Assets = append(botPortfolio.Assets, stypes.BotPortfolioAsset{
				Exchange:   k.Exchange,
				WalletType: k.WalletType,
				Asset:      k.Asset,
				Free:       v.Free.String(),
				Frozen:     v.Frozen.String(),
				UpdatedTs:  updatedTs,
			})
		}

		for _, pv := range snap.Positions {
			if pv.Qty.IsZero() {
				continue
			}
			side := pv.Side
			qty := pv.Qty

			updatedTs := int64(0)
			if pv.UpdateAt > 0 {
				updatedTs = time.Unix(0, pv.UpdateAt).UnixMilli()
			}
			botPortfolio.Positions = append(botPortfolio.Positions, stypes.BotPortfolioPosition{
				Exchange:   pv.Symbol.Exchange,
				Symbol:     pv.Symbol.Symbol.String(),
				MarketType: pv.Symbol.Symbol.Type,
				Side:       side,
				Qty:        qty.String(),
				AvgPrice:   pv.AvgPrice.String(),
				UpdatedTs:  updatedTs,
				Leverage:   int32(pv.Leverage),
			})
		}

		resp.Portfolio = botPortfolio
	} else {
		resp.ExecutorStatus = "not_running"
	}

	return resp, nil
}

func (s *Service) GetStrategy(ctx context.Context, req *stypes.GetStrategyRequest) (*stypes.GetStrategyResponse, error) {
	st, err := entity.Strategy.GetStrategy(ctx, req.ID, req.Version)
	if err != nil {
		return nil, err
	}
	return &stypes.GetStrategyResponse{Strategy: st}, nil
}

func (s *Service) CreateStrategy(ctx context.Context, req *stypes.CreateStrategyRequest) (*stypes.CreateStrategyResponse, error) {
	// 校验策略代码
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, errors.New(errors.InvalidArgument, "strategy code is required")
	}
	if err := validator.ValidateStrategyCode(code); err != nil {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid strategy code: %v", err))
	}

	params := req.Params
	signals := req.Signals
	input := &stypes.CreateStrategyRequest{
		Name:        req.Name,
		Description: req.Description,
		Code:        code,
		Params:      params,
		Signals:     signals,
	}

	st, err := entity.Strategy.CreateStrategy(ctx, input)
	if err != nil {
		return nil, err
	}

	return &stypes.CreateStrategyResponse{
		Strategy: st,
	}, nil
}

func (s *Service) UpdateStrategy(ctx context.Context, req *stypes.UpdateStrategyRequest) (*stypes.UpdateStrategyResponse, error) {
	// 校验策略代码：不允许清空或不传
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, errors.New(errors.InvalidArgument, "strategy code cannot be empty")
	}
	if err := validator.ValidateStrategyCode(code); err != nil {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid strategy code: %v", err))
	}

	params := req.Params
	input := &stypes.UpdateStrategyRequest{
		Id:          req.Id,
		Version:     req.Version,
		Name:        req.Name,
		Description: req.Description,
		Code:        code,
		Params:      params,
	}

	if len(req.Signals) > 0 {
		input.Signals = req.Signals
	}

	strategyObj, err := entity.Strategy.UpdateStrategy(ctx, input)
	if err != nil {
		return nil, err
	}
	return &stypes.UpdateStrategyResponse{
		Strategy: strategyObj,
	}, nil
}

func (s *Service) ListStrategies(ctx context.Context, req *stypes.ListStrategiesRequest) (*stypes.ListStrategiesResponse, error) {
	if req.Offset < 0 {
		return nil, errors.New(errors.InvalidArgument, "offset is invalid")
	}
	if req.Limit <= 0 {
		return nil, errors.New(errors.InvalidArgument, "limit is invalid")
	}

	filter := &stypes.StrategyFilter{}
	hasFilter := false

	if req.Version != nil && *req.Version != "" {
		filter.Version = req.Version
		hasFilter = true
	}
	if req.Status != nil && req.Status.Valid() {
		status := stypes.StrategyStatus(*req.Status)
		filter.Status = &status
		hasFilter = true
	}
	if req.Name != nil && *req.Name != "" {
		filter.Name = req.Name
		hasFilter = true
	}
	if req.CreatedAtStart != nil {
		filter.CreatedAtStart = req.CreatedAtStart
		hasFilter = true
	}
	if req.CreatedAtEnd != nil {
		filter.CreatedAtEnd = req.CreatedAtEnd
		hasFilter = true
	}
	if req.ID != nil && *req.ID != "" {
		filter.Id = req.ID
		hasFilter = true
	}

	if !hasFilter {
		filter = nil
	}

	list, err := entity.Strategy.ListStrategies(ctx, filter)
	if err != nil {
		return nil, err
	}

	count := int64(len(list))
	start := int(req.Offset)
	if start > len(list) {
		start = len(list)
	}
	end := start + int(req.Limit)
	if end > len(list) {
		end = len(list)
	}
	paged := list[start:end]

	out := make([]*stypes.Strategy, 0, len(paged))
	for _, st := range paged {
		out = append(out, st)
	}

	return &stypes.ListStrategiesResponse{
		Strategies: out,
		Count:      count,
	}, nil
}

func (s *Service) CountStrategies(ctx context.Context, _ *stypes.CountStrategiesRequest) (*stypes.CountStrategiesResponse, error) {
	count, err := entity.Strategy.CountStrategies(ctx)
	if err != nil {
		return nil, err
	}
	return &stypes.CountStrategiesResponse{Count: count}, nil
}

func (s *Service) DeleteStrategy(ctx context.Context, req *stypes.DeleteStrategyRequest) (*stypes.DeleteStrategyResponse, error) {
	if len(req.ID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}
	if err := entity.Strategy.DeleteStrategy(ctx, req.ID); err != nil {
		return nil, err
	}
	return &stypes.DeleteStrategyResponse{}, nil
}

func (s *Service) ActiveStrategy(ctx context.Context, req *stypes.ActiveStrategyRequest) (*stypes.ActiveStrategyResponse, error) {
	if len(req.ID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}
	if err := entity.Strategy.ActiveStrategy(ctx, req.ID); err != nil {
		return nil, err
	}
	return &stypes.ActiveStrategyResponse{}, nil
}

func (s *Service) InactiveStrategy(ctx context.Context, req *stypes.InactiveStrategyRequest) (*stypes.InactiveStrategyResponse, error) {
	if len(req.ID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}
	if err := entity.Strategy.InactiveStrategy(ctx, req.ID); err != nil {
		return nil, err
	}
	return &stypes.InactiveStrategyResponse{}, nil
}

func (s *Service) CreateDatasource(ctx context.Context, req *stypes.CreateDatasourceRequest) (*stypes.CreateDatasourceResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if !req.Type.Valid() {
		return nil, errors.New(errors.InvalidArgument, "type is required")
	}
	if req.Exchange == nil {
		return nil, errors.New(errors.InvalidArgument, "exchange is required")
	}
	if req.Symbol == nil || strings.TrimSpace(*req.Symbol) == "" {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}
	if req.StartTs <= 0 || req.EndTs <= 0 {
		return nil, errors.New(errors.InvalidArgument, "start_time/end_time is required")
	}
	if req.StartTs >= req.EndTs {
		return nil, errors.New(errors.InvalidArgument, "start_time must be less than end_time")
	}

	exSrv := *req.Exchange
	if !exSrv.IsValid() {
		return nil, errors.New(errors.InvalidArgument, "invalid exchange")
	}
	symbol, err := ctypes.ParseSymbol(*req.Symbol)
	if err != nil {
		return nil, err
	}

	props := make(map[string]any)
	if req.Props != nil && len(*req.Props) > 0 {
		if err := sonic.Unmarshal([]byte(*req.Props), &props); err != nil {
			return nil, err
		}
	}

	input := &ctypes.CreateDatasourceInput{
		Type:        req.Type,
		Name:        req.Name,
		Description: req.Description,
		Exchange:    &exSrv,
		Symbol:      &symbol,
		Props:       props,
		StartTs:     time.Unix(req.StartTs, 0),
		EndTs:       time.Unix(req.EndTs, 0),
	}
	ds, inserted, err := entity.Strategy.CreateDatasource(ctx, input)
	if err != nil {
		return nil, err
	}

	return &stypes.CreateDatasourceResponse{
		Datasource: ds,
		Inserted:   inserted > 0,
	}, nil
}

func (s *Service) ListDatasources(ctx context.Context, req *stypes.ListDatasourcesRequest) (*stypes.ListDatasourcesResponse, error) {
	if req.Offset < 0 {
		return nil, errors.New(errors.InvalidArgument, "offset is invalid")
	}
	if req.Limit <= 0 {
		return nil, errors.New(errors.InvalidArgument, "limit is invalid")
	}

	filter := &ctypes.DatasourceFilter{
		Offset: req.Offset,
		Limit:  req.Limit,
	}
	if req.Type != nil && req.Type.Valid() {
		t := *req.Type
		filter.Type = &t
	}
	if req.Exchange != nil {
		ex := *req.Exchange
		if !ex.IsValid() {
			return nil, errors.New(errors.InvalidArgument, "invalid exchange")
		}
		filter.Exchange = &ex
	}
	if req.Symbol != nil {
		symbol, err := ctypes.ParseSymbol(*req.Symbol)
		if err != nil {
			return nil, err
		}
		filter.Symbol = &symbol
	}

	list, count, err := entity.Strategy.ListDatasources(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &stypes.ListDatasourcesResponse{
		Datasources: list,
		Count:       count,
	}, nil
}

func (s *Service) DeleteDatasource(ctx context.Context, req *stypes.DeleteDatasourceRequest) (*stypes.DeleteDatasourceResponse, error) {
	if req.ID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}
	if err := entity.Strategy.DeleteDatasource(ctx, req.ID); err != nil {
		return nil, err
	}
	return &stypes.DeleteDatasourceResponse{}, nil
}

func (s *Service) RunBacktest(ctx context.Context, req *stypes.RunBacktestRequest) (*stypes.RunBacktestResponse, error) {
	var strategy *stypes.Strategy
	var strategyID, version string

	if req.Strategy == nil {
		return nil, errors.New(errors.InvalidArgument, "strategy is required")
	}

	if req.RunType == 1 {
		if req.Strategy.ID == "" {
			return nil, errors.New(errors.InvalidArgument, "strategy id is required")
		}
		if req.Strategy.Version == "" {
			return nil, errors.New(errors.InvalidArgument, "strategy version is required")
		}
		strategyID = req.Strategy.ID
		version = req.Strategy.Version
	} else {
		strategy = req.Strategy
		strategyID = snowflake.Generate().String()
		version = time.Now().Format("20060102150405")
		strategy.ID = strategyID
		strategy.Version = version
	}

	params := make(map[string]any)
	if len(req.Params) > 0 {
		if err := sonic.Unmarshal([]byte(req.Params), &params); err != nil {
			return nil, errors.New(errors.InvalidArgument, "params is invalid")
		}
	}

	id := snowflake.Generate().String()
	input := &stypes.RunBacktestInput{
		Context: stypes.BacktestContext{
			ID:          id,
			StrategyID:  strategyID,
			StrategyVer: version,
		},
		StartTime: time.Unix(req.StartTime, 0),
		EndTime:   time.Unix(req.EndTime, 0),
		Symbols:   req.Symbols,
		Signals:   req.Signals,
		Params:    params,
		Strategy:  strategy,
	}

	result, err := entity.Strategy.RunBacktest(ctx, input)
	if err != nil {
		return nil, err
	}

	out := &stypes.RunBacktestResponse{
		ID:             result.ID,
		JobID:          result.JobID,
		StrategyID:     result.StrategyID,
		StrategyVer:    result.StrategyVer,
		StartTime:      result.StartTime,
		EndTime:        result.EndTime,
		TimeCost:       result.TimeCost,
		InitialBalance: result.InitialBalance,
		FinalBalance:   result.FinalBalance,
		TotalPnl:       result.TotalPnl,
		TotalTrades:    result.TotalTrades,
		WinTrades:      result.WinTrades,
		LossTrades:     result.LossTrades,
		WinRate:        result.WinRate,
		SharpeRatio:    result.SharpeRatio,
		MaxDrawdown:    result.MaxDrawdown,
		Data:           result.Data,
		CreatedAt:      result.CreatedAt,
	}
	out.Strategy = &stypes.Strategy{
		ID:      result.StrategyID,
		Version: result.StrategyVer,
	}
	return out, nil
}

func (s *Service) CreateBot(ctx context.Context, req *stypes.CreateBotRequest) (*stypes.CreateBotResponse, error) {
	if req.StrategyID == "" {
		return nil, errors.New(errors.InvalidArgument, "strategy_id is required")
	}
	if !req.Mode.Valid() {
		return nil, errors.New(errors.InvalidArgument, "mode is required")
	}
	if req.Name == "" {
		return nil, errors.New(errors.InvalidArgument, "name is required")
	}

	exSrv := req.Exchange
	if !exSrv.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid exchange: %v", req.Exchange))
	}

	symbols := make([]ctypes.Symbol, 0, len(req.Symbols))
	for _, smb := range req.Symbols {
		symbol, err := ctypes.ParseSymbol(smb)
		if err != nil {
			return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %v", err))
		}
		symbols = append(symbols, symbol)
	}

	var mode stypes.BotMode
	switch req.Mode {
	case stypes.BotModeLive:
		mode = stypes.BotModeLive
	case stypes.BotModePaper:
		mode = stypes.BotModePaper
	default:
		return nil, errors.New(errors.InvalidArgument, "invalid mode")
	}

	var config stypes.BotConfig
	if err := sonic.Unmarshal([]byte(req.Config), &config); err != nil {
		return nil, errors.New(errors.InvalidArgument, "invalid config format")
	}

	accountID := req.AccountID
	if mode == stypes.BotModeLive {
		if accountID == "" {
			return nil, errors.New(errors.InvalidArgument, "account_id is required")
		}
		acct, err := entity.Account.GetAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		if acct == nil {
			return nil, errors.New(errors.NotFound, "account not found")
		}
		if acct.AccountType == ctypes.AccountTypeVirtual || acct.AccountType == ctypes.AccountTypeVirtualSub {
			return nil, errors.New(errors.InvalidArgument, "live bot must bind to a real parent account")
		}
		if acct.Exchange != exSrv {
			return nil, errors.New(errors.InvalidArgument, "account exchange mismatch")
		}
		if acct.MultiBotMode {
			assets, err := parseInitialAssets(config)
			if err != nil {
				return nil, err
			}
			if len(assets) == 0 {
				return nil, errors.New(errors.InvalidArgument, "initial assets is required when parent account has multi_bot_mode enabled")
			}
			sub, err := s.accountSvc.CreateVirtualSubAccount(ctx, ctypes.CreateVirtualSubAccountInput{
				ParentAccountID: acct.ID,
				BotName:         req.Name,
				InitialAssets:   assets,
			})
			if err != nil {
				return nil, err
			}
			accountID = sub.ID
		}
	} else {
		assets, err := parseInitialAssets(config)
		if err != nil {
			return nil, err
		}
		if len(assets) == 0 {
			return nil, errors.New(errors.InvalidArgument, "initial assets is required for paper mode")
		}
		paper, err := s.accountSvc.CreateAccount(ctx, ctypes.CreateAccountInput{
			Name:          fmt.Sprintf("paper-%s-%s", req.Name, time.Now().Format("20060102150405")),
			Exchange:      exSrv,
			AccountType:   ctypes.AccountTypeVirtual,
			Algorithm:     ctypes.AuthAlgorithmNone,
			InitialAssets: assets,
		})
		if err != nil {
			return nil, err
		}
		accountID = paper.ID
	}

	input := &stypes.CreateBotInput{
		StrategyID:  req.StrategyID,
		StrategyVer: req.StrategyVer,
		Exchange:    exSrv,
		AccountID:   accountID,
		Mode:        mode,
		Name:        req.Name,
		Description: req.Description,
		Config:      config,
		Symbols:     symbols,
	}

	bot, err := entity.Strategy.CreateBot(ctx, input)
	if err != nil {
		return nil, err
	}

	return &stypes.CreateBotResponse{
		Bot: bot,
	}, nil
}

// UpdateBot 更新Bot
func (s *Service) UpdateBot(ctx context.Context, req *stypes.UpdateBotRequest) (*stypes.UpdateBotResponse, error) {
	input := &stypes.UpdateBotInput{
		ID:          req.ID,
		Name:        lo.FromPtr(req.Name),
		Description: lo.FromPtr(req.Description),
		Symbols:     req.Symbols,
		Config:      req.Config,
	}

	bot, err := entity.Strategy.UpdateBot(ctx, input)
	if err != nil {
		return nil, err
	}

	return &stypes.UpdateBotResponse{
		Bot: bot,
	}, nil
}

func parseInitialAssets(config stypes.BotConfig) ([]ctypes.AssetInput, error) {
	result := make([]ctypes.AssetInput, 0, len(config.InitialAssets))
	for _, asset := range config.InitialAssets {
		result = append(result, ctypes.AssetInput{
			Asset:      asset.Code,
			Total:      asset.Balance.String(),
			Frozen:     asset.Locked().String(),
			WalletType: asset.WalletType,
		})
	}
	return result, nil
}

func (s *Service) ListBots(ctx context.Context, req *stypes.ListBotsRequest) (*stypes.ListBotsResponse, error) {
	if req.Limit <= 0 {
		return nil, errors.New(errors.InvalidArgument, "limit is required")
	}

	filter := &stypes.BotFilter{}
	if req.ID != nil {
		idVal := int64(*req.ID)
		filter.ID = &idVal
	}
	if req.StrategyID != nil {
		filter.StrategyID = req.StrategyID
	}
	if req.Mode != nil && req.Mode.Valid() {
		var mode stypes.BotMode
		switch *req.Mode {
		case stypes.BotModeLive:
			mode = stypes.BotModeLive
		case stypes.BotModePaper:
			mode = stypes.BotModePaper
		}
		filter.Mode = &mode
	}
	if req.Status != nil && req.Status.Valid() {
		var status stypes.BotStatus
		switch *req.Status {
		case stypes.BotStatusStopped:
			status = stypes.BotStatusStopped
		case stypes.BotStatusRunning:
			status = stypes.BotStatusRunning
		case stypes.BotStatusError:
			status = stypes.BotStatusError
		}
		filter.Status = &status
	}
	if req.AccountID != nil && *req.AccountID != "" {
		filter.AccountID = req.AccountID
	}
	if req.Exchange != nil {
		exs := req.Exchange.String()
		filter.Exchange = &exs
	}
	if req.Name != nil && *req.Name != "" {
		filter.Name = req.Name
	}
	if req.CreatedAtStart != nil {
		filter.CreatedAtStart = req.CreatedAtStart
	}
	if req.CreatedAtEnd != nil {
		filter.CreatedAtEnd = req.CreatedAtEnd
	}

	bots, err := entity.Strategy.ListBots(ctx, filter)
	if err != nil {
		return nil, err
	}

	// 分页（简化处理）
	start := int(req.Offset)
	if start > len(bots) {
		start = len(bots)
	}
	end := start + int(req.Limit)
	if end > len(bots) {
		end = len(bots)
	}
	paged := bots[start:end]

	out := make([]*stypes.Bot, 0, len(paged))
	for _, bot := range paged {
		out = append(out, bot)
	}

	return &stypes.ListBotsResponse{
		Bots:       out,
		TotalCount: int32(len(bots)),
	}, nil
}

func (s *Service) CountBots(ctx context.Context, req *stypes.CountBotsRequest) (*stypes.CountBotsResponse, error) {
	var filter *stypes.BotFilter
	if req.Status != nil && req.Status.Valid() {
		var status stypes.BotStatus
		switch *req.Status {
		case stypes.BotStatusStopped:
			status = stypes.BotStatusStopped
		case stypes.BotStatusRunning:
			status = stypes.BotStatusRunning
		case stypes.BotStatusError:
			status = stypes.BotStatusError
		default:
			status = stypes.BotStatusStopped
		}
		filter = &stypes.BotFilter{Status: &status}
	}
	count, err := entity.Strategy.CountBots(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &stypes.CountBotsResponse{Count: int32(count)}, nil
}

func (s *Service) GetBot(ctx context.Context, req *stypes.GetBotRequest) (*stypes.GetBotResponse, error) {
	if req.ID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	bot, err := entity.Strategy.GetBot(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	return &stypes.GetBotResponse{
		Bot: bot,
	}, nil
}

func (s *Service) StartBot(ctx context.Context, req *stypes.StartBotRequest) (*stypes.StartBotResponse, error) {
	if req.ID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	// 启动前校验关联账户必须在线
	bot, err := entity.Strategy.GetBot(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if bot == nil {
		return nil, errors.New(errors.NotFound, "bot not found")
	}
	accountID := strings.TrimSpace(bot.AccountID)
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	acctResp, err := s.accountSvc.QueryAccounts(ctx, &ctypes.QueryAccountsRequest{
		Id:     &accountID,
		Offset: 0,
		Limit:  1,
	})
	if err != nil {
		return nil, err
	}
	if acctResp == nil || len(acctResp.Accounts) == 0 || acctResp.Accounts[0] == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	if acctResp.Accounts[0].Status != ctypes.AccountStatusOnline {
		return nil, errors.New(errors.InvalidArgument, "关联账户未上线，无法启动 Bot")
	}

	if err := entity.Strategy.StartBot(ctx, req.ID); err != nil {
		return nil, err
	}

	return &stypes.StartBotResponse{
		Success: true,
	}, nil
}

func (s *Service) StopBot(ctx context.Context, req *stypes.StopBotRequest) (*stypes.StopBotResponse, error) {
	if req.ID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	if err := entity.Strategy.StopBot(ctx, req.ID); err != nil {
		return nil, err
	}

	return &stypes.StopBotResponse{
		Success: true,
	}, nil
}

func (s *Service) DeleteBot(ctx context.Context, req *stypes.DeleteBotRequest) (*stypes.DeleteBotResponse, error) {
	if req.ID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	if err := entity.Strategy.DeleteBot(ctx, req.ID); err != nil {
		return nil, err
	}

	return &stypes.DeleteBotResponse{
		Success: true,
	}, nil
}

func (s *Service) UpgradeBot(ctx context.Context, req *stypes.UpgradeBotRequest) (*stypes.UpgradeBotResponse, error) {
	if req.ID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	botObj, started, msg, err := entity.Strategy.UpgradeBot(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	return &stypes.UpgradeBotResponse{
		Success: started,
		Message: msg,
		Bot:     botObj,
	}, nil
}

func (s *Service) ListBotOrders(ctx context.Context, req *stypes.ListBotOrdersRequest) (*stypes.ListBotOrdersResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	accountID, err := s.getBotAccountID(ctx, req.BotID)
	if err != nil {
		return nil, err
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	size := req.Size
	if size <= 0 {
		size = 20
	}
	botID := int64(req.BotID)
	request := &ctypes.QueryOrdersByPageRequest{
		AccountID:   accountID,
		Page:        page,
		Size:        size,
		Symbol:      lo.FromPtr(req.Symbol),
		OrderType:   req.OrderType,
		OrderSource: req.OrderSource,
		Statuses:    req.Statuses,
		BotID:       &botID,
	}
	resp, err := s.orderSvc.QueryOrdersByPage(ctx, request)
	if err != nil {
		return nil, err
	}
	return &stypes.ListBotOrdersResponse{
		Orders:     resp.GetOrders(),
		TotalCount: int32(resp.GetTotalCount()),
	}, nil
}

func (s *Service) ListBotLedger(ctx context.Context, req *stypes.ListBotLedgerRequest) (*stypes.ListBotLedgerResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	accountID, err := s.getBotAccountID(ctx, req.BotID)
	if err != nil {
		return nil, err
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	size := req.Size
	if size <= 0 {
		size = 20
	}
	var lWallet *ctypes.WalletType
	if req.WalletType != nil {
		w := *req.WalletType
		if w.Valid() {
			lWallet = &w
		}
	}
	var asset *string
	if req.Asset != nil && *req.Asset != "" {
		asset = req.Asset
	}
	resp, err := s.accountSvc.QueryLedgers(ctx, &ctypes.QueryLedgersRequest{
		AccountID:  accountID,
		WalletType: lWallet,
		Asset:      asset,
		StartTs:    lo.FromPtrOr(req.StartTs, 0),
		EndTs:      lo.FromPtrOr(req.EndTs, 0),
		Size:       size,
		Page:       page,
	})
	if err != nil {
		return nil, err
	}
	return &stypes.ListBotLedgerResponse{
		Ledgers:    resp.Ledgers,
		TotalCount: int32(resp.TotalCount),
	}, nil
}

func (s *Service) ListBotEquity(ctx context.Context, req *stypes.ListBotEquityRequest) (*stypes.ListBotEquityResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	accountID, err := s.getBotAccountID(ctx, req.BotID)
	if err != nil {
		return nil, err
	}
	resp, err := s.accountSvc.QueryEquitys(ctx, &ctypes.QueryEquitysRequest{
		AccountID: accountID,
		StartTs:   req.StartTs,
		EndTs:     req.EndTs,
	})
	if err != nil {
		return nil, err
	}
	return &stypes.ListBotEquityResponse{
		Points: resp.Equitys,
	}, nil
}

func (s *Service) QueryBotMetrics(ctx context.Context, req *stypes.QueryBotMetricsRequest) (*stypes.QueryBotMetricsResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if req.BotID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "bot_id is required")
	}

	bot, err := entity.Strategy.GetBot(ctx, req.BotID)
	if err != nil {
		return nil, err
	}
	if bot == nil || strings.TrimSpace(bot.AccountID) == "" {
		return nil, errors.New(errors.NotFound, "bot not found")
	}
	accountID := bot.AccountID

	// 时间范围，默认最近 30 天
	now := time.Now()
	startTs := now.AddDate(0, 0, -30).Unix()
	endTs := now.Unix()
	if req.StartTs != nil && *req.StartTs > 0 {
		startTs = *req.StartTs
	}
	if req.EndTs != nil && *req.EndTs > 0 {
		endTs = *req.EndTs
	}
	if startTs > endTs {
		startTs, endTs = endTs, startTs
	}

	// 1. 获取权益曲线（account API 使用毫秒）
	startTsMs := startTs * 1000
	endTsMs := endTs * 1000
	equityResp, err := s.accountSvc.QueryEquitys(ctx, &ctypes.QueryEquitysRequest{
		AccountID: accountID,
		StartTs:   startTsMs,
		EndTs:     endTsMs,
	})
	if err != nil {
		return nil, err
	}

	// 2. 分页获取订单（按 bot_id 过滤）
	botID := int64(req.BotID)
	allOrders := make([]*ctypes.Order, 0, 1000)
	for page := int32(1); ; page++ {
		orderResp, err := s.orderSvc.QueryOrdersByPage(ctx, &ctypes.QueryOrdersByPageRequest{
			AccountID: accountID,
			BotID:     &botID,
			StartTs:   &startTs,
			EndTs:     &endTs,
			Page:      page,
			Size:      200,
		})
		if err != nil {
			return nil, err
		}
		ords := orderResp.GetOrders()
		allOrders = append(allOrders, ords...)
		if int64(len(allOrders)) >= orderResp.GetTotalCount() || len(ords) == 0 {
			break
		}
		if len(allOrders) >= 10000 {
			break
		}
	}

	// 3. 构建权益点（Equity.Ts 为毫秒，转为秒）
	equityPoints := make([]metrics.EquityPoint, 0, len(equityResp.Equitys))
	for _, e := range equityResp.Equitys {
		if e == nil {
			continue
		}
		f, _ := e.Notional.Float64()
		equityPoints = append(equityPoints, metrics.EquityPoint{Ts: e.Ts.UnixMilli() / 1000, Notional: f})
	}

	// 4. 构建订单（仅 DONE/PARTIAL_DONE）
	ordersForMetrics := make([]metrics.OrderForMetrics, 0, len(allOrders))
	for _, o := range allOrders {
		if o == nil {
			continue
		}
		st := o.Status
		if st != ctypes.OrderStatusDone && st != ctypes.OrderStatusPartialDone {
			continue
		}
		realizedPnl := decimal.Zero
		if o.RealizedPnl != nil {
			realizedPnl = *o.RealizedPnl
		}
		fee := decimal.Zero
		if o.Fee != nil {
			fee = *o.Fee
		}
		avgPrice := o.AvgPrice
		price := o.Price
		execQty := o.ExecutedQty
		orderstypestr := "LIMIT"
		if o.OrderType == ctypes.OrderTypeMarket {
			orderstypestr = "MARKET"
		}
		exStr := o.Exchange.String()
		ordersForMetrics = append(ordersForMetrics, metrics.OrderForMetrics{
			RealizedPnl: realizedPnl,
			Fee:         fee,
			AvgPrice:    avgPrice,
			Price:       price,
			ExecutedQty: execQty,
			OrderType:   orderstypestr,
			Symbol:      o.Symbol.String(),
			Exchange:    exStr,
		})
	}

	// 5. 计算账户级指标
	resp := &stypes.QueryBotMetricsResponse{
		AccountID: accountID,
		BotID:     req.BotID,
		Dimension: req.Dimension,
		StartTs:   startTs,
		EndTs:     endTs,
	}
	if req.Symbol != nil {
		resp.SymbolsFilter = req.Symbol.String()
	} else {
		parts := make([]string, 0, len(bot.Symbols))
		for _, s := range bot.Symbols {
			parts = append(parts, s.String())
		}
		resp.SymbolsFilter = strings.Join(parts, ",")
	}
	if len(equityPoints) >= 2 {
		resp.Cagr = metrics.CalculateCAGR(equityPoints)
		resp.Sharpe = metrics.CalculateSharpeRatio(equityPoints)
		resp.Sortino = metrics.CalculateSortino(equityPoints)
		resp.MaxDrawdown = metrics.CalculateMaxDrawdown(equityPoints)
		resp.TimeUnderWaterSeconds = metrics.CalculateTimeUnderWater(equityPoints)
		resp.Calmar = metrics.CalculateCalmar(equityPoints)
		resp.RollingSharpe = metrics.CalculateRollingSharpe(equityPoints, 20)
	}
	if len(ordersForMetrics) > 0 {
		resp.WinRate = metrics.CalculateWinRate(ordersForMetrics)
		resp.ProfitFactor = metrics.CalculateProfitFactor(ordersForMetrics)
		resp.FeeRatio = metrics.CalculateFeeRatio(ordersForMetrics)
		resp.MaxConsecutiveLoss = metrics.CalculateMaxConsecutiveLoss(ordersForMetrics)
		resp.AvgSlippageBps = metrics.CalculateAvgSlippage(ordersForMetrics)
	}

	// 6. SYMBOL 维度：按 symbol 分组计算
	if req.Dimension == stypes.MetricsDimensionSymbol {
		symEquityResp, err := s.accountSvc.QuerySymbolEquity(ctx, &ctypes.QuerySymbolEquityRequest{
			AccountID: accountID,
			StartTs:   startTsMs,
			EndTs:     endTsMs,
		})
		if err != nil {
			return nil, err
		}

		// 仅保留 bot 订单中出现的 (exchange, symbol)
		botSymbolKeys := make(map[string]struct{})
		for _, o := range ordersForMetrics {
			botSymbolKeys[o.Exchange+"|"+o.Symbol] = struct{}{}
		}

		equityBySymbol := make(map[string][]metrics.EquityPoint)
		for _, row := range symEquityResp.Items {
			if row == nil {
				continue
			}
			key := row.Exchange + "|" + row.Symbol
			if _, ok := botSymbolKeys[key]; !ok {
				continue
			}
			n, _ := decimal.NewFromString(row.NetValue)
			f, _ := n.Float64()
			equityBySymbol[key] = append(equityBySymbol[key], metrics.EquityPoint{Ts: row.Ts / 1000, Notional: f})
		}

		ordersBySymbol := make(map[string][]metrics.OrderForMetrics)
		for _, o := range ordersForMetrics {
			key := o.Exchange + "|" + o.Symbol
			ordersBySymbol[key] = append(ordersBySymbol[key], o)
		}

		for key, pts := range equityBySymbol {
			ex, sym := splitSymbolKey(key)
			ordList := ordersBySymbol[key]
			sm := stypes.SymbolMetrics{Symbol: sym, Exchange: ex}
			if len(pts) >= 2 {
				sm.Cagr = metrics.CalculateCAGR(pts)
				sm.Sharpe = metrics.CalculateSharpeRatio(pts)
				sm.Sortino = metrics.CalculateSortino(pts)
				sm.MaxDrawdown = metrics.CalculateMaxDrawdown(pts)
				sm.TimeUnderWaterSeconds = metrics.CalculateTimeUnderWater(pts)
				sm.Calmar = metrics.CalculateCalmar(pts)
				sm.RollingSharpe = metrics.CalculateRollingSharpe(pts, 20)
			}
			if len(ordList) > 0 {
				sm.WinRate = metrics.CalculateWinRate(ordList)
				sm.ProfitFactor = metrics.CalculateProfitFactor(ordList)
				sm.FeeRatio = metrics.CalculateFeeRatio(ordList)
				sm.MaxConsecutiveLoss = metrics.CalculateMaxConsecutiveLoss(ordList)
				sm.AvgSlippageBps = metrics.CalculateAvgSlippage(ordList)
			}
			resp.Symbols = append(resp.Symbols, sm)
		}
	}

	return resp, nil
}

func splitSymbolKey(key string) (exchange, symbol string) {
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}

func (s *Service) ListBotLogs(ctx context.Context, req *stypes.ListBotLogsRequest) (*stypes.ListBotLogsResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if req.BotID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "bot_id is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}

	var startTime *time.Time
	if req.StartTime != nil {
		t := time.UnixMilli(lo.FromPtr(req.StartTime))
		startTime = &t
	}
	var endTime *time.Time
	if req.EndTime != nil {
		t := time.UnixMilli(lo.FromPtr(req.EndTime))
		endTime = &t
	}
	if startTime != nil && endTime != nil && startTime.After(*endTime) {
		return nil, errors.New(errors.InvalidArgument, "start_time must be less than or equal to end_time")
	}

	storage, err := store.NewClickhouseStorage(req.BotID, 1*time.Second, s.chClient)
	if err != nil {
		return nil, err
	}

	logs, nextCursor, err := storage.List(ctx, store.ConsoleLogFilter{
		BotID:     req.BotID,
		Limit:     limit,
		Cursor:    lo.FromPtrOr(req.Cursor, ""),
		StartTime: startTime,
		EndTime:   endTime,
		Level:     req.Level,
	})
	if err != nil {
		return nil, err
	}

	var nextCur *string
	if nextCursor != "" {
		nextCur = &nextCursor
	}
	resp := &stypes.ListBotLogsResponse{
		Logs:       make([]stypes.BotLogEntry, 0, len(logs)),
		NextCursor: nextCur,
	}
	for _, logEntry := range logs {
		resp.Logs = append(resp.Logs, stypes.BotLogEntry{
			ID:        logEntry.ID,
			BotID:     logEntry.BotID,
			Level:     logEntry.Level,
			Message:   logEntry.Message,
			Ts:        logEntry.Ts.UnixMilli(),
			CreatedAt: logEntry.CreatedAt.UnixMilli(),
		})
	}
	return resp, nil
}

func (s *Service) QueryBotSignalFlow(ctx context.Context, req *stypes.QueryBotSignalFlowRequest) (*stypes.QueryBotSignalFlowResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if req.BotID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "bot_id is required")
	}
	if s.chClient == nil {
		return nil, errors.New(errors.Unimplemented, "clickhouse is not configured")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	var startTs *time.Time
	if req.StartTsMs != nil {
		t := time.UnixMilli(lo.FromPtr(req.StartTsMs))
		startTs = &t
	}

	streamFilter := ""
	if req.SignalType != nil && req.SignalType.Valid() {
		switch *req.SignalType {
		case stypes.SignalTypeKline:
			streamFilter = "kline"
		case stypes.SignalTypeTrade:
			streamFilter = "trade"
		case stypes.SignalTypeDepth:
			streamFilter = "depth"
		case stypes.SignalTypeTicker:
			streamFilter = "ticker"
		case stypes.SignalTypeSocial:
			streamFilter = "social"
		case stypes.SignalTypeTimer:
			streamFilter = "timer"
		case stypes.SignalTypeOrder:
			streamFilter = "order"
		case stypes.SignalTypePosition:
			streamFilter = "position"
		case stypes.SignalTypeBalance:
			streamFilter = "balance"
		case stypes.SignalTypeRisk:
			streamFilter = "risk"
		case stypes.SignalTypeSystem:
			streamFilter = "system"
		default:
			streamFilter = ""
		}
	}

	querier, err := signalflow.NewQuerier(s.chClient)
	if err != nil {
		return nil, err
	}

	records, err := querier.Query(ctx, signalflow.QueryFilter{
		BotID:   req.BotID,
		Stream:  streamFilter,
		StartTs: startTs,
		StartID: req.StartID,
		Limit:   limit,
	})
	if err != nil {
		return nil, err
	}

	resp := &stypes.QueryBotSignalFlowResponse{
		Events: make([]stypes.BotSignalRecord, 0, len(records)),
	}
	for _, record := range records {
		resp.Events = append(resp.Events, stypes.BotSignalRecord{
			ID:           record.ID,
			BotID:        record.BotID,
			AccountID:    record.AccountID,
			Exchange:     record.Exchange,
			Stream:       record.Stream,
			Topic:        record.Topic,
			EventKind:    record.EventKind,
			TsMs:         record.Ts.UnixMilli(),
			InboundAtMs:  record.InboundAt.UnixMilli(),
			OutboundAtMs: record.OutboundAt.UnixMilli(),
			ReceiveAtMs:  record.ReceiveAt.UnixMilli(),
			IngestAtMs:   record.IngestAt.UnixMilli(),
			PayloadJSON:  record.Payload,
		})
	}
	if len(records) > 0 {
		last := records[len(records)-1]
		resp.NextID = last.ID + 1
	} else {
		resp.NextID = 0
	}
	return resp, nil
}

func (s *Service) GetBotStats(ctx context.Context, req *stypes.GetBotStatsRequest) (*stypes.GetBotStatsResponse, error) {
	if s.chClient == nil {
		return &stypes.GetBotStatsResponse{Stats: nil}, nil
	}
	querier, err := signalflow.NewQuerier(s.chClient)
	if err != nil {
		return nil, err
	}
	startTs := time.Unix(req.StartTs, 0)
	endTs := time.Unix(req.EndTs, 0)
	botID := int32(0)
	if req.BotID != nil && *req.BotID > 0 {
		botID = *req.BotID
	}
	stats, err := querier.QueryStats(ctx, signalflow.StatsFilter{
		StartTs: startTs,
		EndTs:   endTs,
		BotID:   botID,
	})
	if err != nil {
		return nil, err
	}
	resp := &stypes.GetBotStatsResponse{
		Stats: make([]stypes.BotSignalStats, 0, len(stats)),
	}
	for _, st := range stats {
		resp.Stats = append(resp.Stats, stypes.BotSignalStats{
			BotID:        st.BotID,
			Stream:       st.Stream,
			EventCount:   int64(st.EventCount),
			AvgLatencyMs: st.AvgLatencyMs,
			MaxLatencyMs: float64(st.MaxLatencyMs),
		})
	}
	return resp, nil
}

func (s *Service) GenerateStrategy(ctx context.Context, req *stypes.GenerateStrategyRequest) (*stypes.GenerateStrategyResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, errors.New(errors.InvalidArgument, "query is required")
	}

	// load strategy guide
	guide, err := s.db.KvRepo.GetByKey(ctx, "doc.strategy.guide")
	if err != nil {
		return nil, errors.New(errors.Internal, "failed to load strategy guide")
	}
	if guide == nil {
		return nil, errors.New(errors.Internal, "strategy guide not found")
	}

	payload := map[string]any{
		"guide": guide.Value,
		"query": query,
	}
	variables, err := sonic.Marshal(payload)
	if err != nil {
		return nil, errors.New(errors.Internal, "failed to encode llm variables")
	}

	resp, err := s.llmSvc.Completion(ctx, &ctypes.CompletionRequest{
		SceneKey: AiStrategySceneKey,
		TestBy: &ctypes.CompletionTestBy{
			Kind:     ctypes.CompletionTestByPromptID,
			PromptID: 0,
		},
		Variables: variables,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.Success {
		if resp != nil && strings.TrimSpace(resp.Error) != "" {
			return nil, errors.New(errors.Internal, resp.Error)
		}
		return nil, errors.New(errors.Internal, "llm completion failed")
	}
	code := strings.TrimSpace(resp.Result)
	if code == "" {
		return nil, errors.New(errors.Internal, "llm returned empty strategy code")
	}
	if strings.HasSuffix(code, "```") {
		switch {
		case strings.HasPrefix(code, "```javascript"):
			code = strings.TrimPrefix(code, "```javascript")
			code = strings.TrimSuffix(code, "```")
		case strings.HasPrefix(code, "```js"):
			code = strings.TrimPrefix(code, "```js")
			code = strings.TrimSuffix(code, "```")
		case strings.HasPrefix(code, "```"):
			code = strings.TrimPrefix(code, "```")
			code = strings.TrimSuffix(code, "```")
		}
	}

	var completionID int64
	if resp.CompletionID != nil {
		completionID = *resp.CompletionID
	}
	if completionID <= 0 {
		return nil, errors.New(errors.Internal, "llm returned empty completion id")
	}
	return &stypes.GenerateStrategyResponse{
		SessionID: strconv.FormatInt(completionID, 10),
		Content:   code,
	}, nil
}
