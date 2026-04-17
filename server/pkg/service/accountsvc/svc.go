package accountsvc

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/common"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/common/calculator"
	converter "github.com/wangliang139/NovaForge/server/pkg/converter"
	"github.com/wangliang139/NovaForge/server/pkg/entity"
	"github.com/wangliang139/NovaForge/server/pkg/entity/account"
	"github.com/wangliang139/NovaForge/server/pkg/internal/chsdk"
	"github.com/wangliang139/NovaForge/server/pkg/internal/encrypt"
	"github.com/wangliang139/NovaForge/server/pkg/internal/okxsdk"
	"github.com/wangliang139/NovaForge/server/pkg/market/eventflow"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/repos/equity"
	"github.com/wangliang139/NovaForge/server/pkg/repos/risk_event"
	"github.com/wangliang139/NovaForge/server/pkg/repos/symbol_equity"
	"github.com/wangliang139/NovaForge/server/pkg/settings"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/snowflake"
	"golang.org/x/sync/errgroup"
)

type Config struct {
	DecryptKeyBase64 string `split_words:"true" required:"true" envconfig:"APP_DECRYPT_KEY_BASE64"`
}

func (c *Config) String() string {
	if c == nil {
		return "nil"
	}
	tmp := *c
	if len(tmp.DecryptKeyBase64) > 0 {
		tmp.DecryptKeyBase64 = "*****"
	}
	return fmt.Sprintf("%+v", tmp)
}

func stringToDecimal(s string) decimal.Decimal {
	if strings.TrimSpace(s) == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

type Service struct {
	cfg           Config
	db            *repos.Entity
	eventFlowRepo EventFlowQuerier
}

type EventFlowQuerier interface {
	Query(ctx context.Context, filter eventflow.QueryFilter) ([]eventflow.EventRecord, error)
}

func New(db *repos.Entity) (*Service, error) {
	var cfg Config
	envconfig.MustProcess("account_svc", &cfg)
	log.Info().Msgf("account service config: %+v", &cfg)

	svc := &Service{
		cfg: cfg,
		db:  db,
	}

	// 尝试初始化 event flow querier（可选功能）
	svc.eventFlowRepo = initEventFlowQuerier()

	return svc, nil
}

func initEventFlowQuerier() EventFlowQuerier {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := chsdk.Connect(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("event flow querier not available (ClickHouse not configured)")
		return nil
	}

	querier, err := eventflow.NewQuerier(client)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create event flow querier")
		return nil
	}

	log.Info().Msg("event flow querier initialized")
	return querier
}

func (s *Service) CreateAccount(ctx context.Context, input types.CreateAccountInput) (*types.Account, error) {
	log.Debug().Interface("request", input).Msg("CreateAccount params")
	if !input.Exchange.IsValid() {
		return nil, errors.New(errors.InvalidArgument, "exchange is invalid")
	}
	if len(input.Name) == 0 {
		return nil, errors.New(errors.InvalidArgument, "name is required")
	}
	if input.AccountType == types.AccountTypeVirtualSub {
		return nil, errors.New(errors.InvalidArgument, "virtual_sub accounts cannot be created via this API")
	}

	// 虚拟账户不需要校验 apikey/apiSecret
	if input.AccountType == types.AccountTypeVirtual {
		// 虚拟账户可以为空，设置默认值
		input.ApiKey = ""
		input.ApiSecret = ""
		input.Passphrase = ""
	} else {
		// 真实账户需要校验
		if len(input.ApiKey) == 0 {
			return nil, errors.New(errors.InvalidArgument, "api key is required")
		}
		if len(input.ApiSecret) == 0 {
			return nil, errors.New(errors.InvalidArgument, "api secret is required")
		}
		if (input.Exchange == types.ExchangeOkx || input.Exchange == types.ExchangeOkxTest) && len(input.Passphrase) == 0 {
			return nil, errors.New(errors.InvalidArgument, "passphrase is required")
		}

		// 校验apiKey和apiSecret是否合法
		err := s.checkAccountApiSecret(ctx, input.Exchange, input.ApiKey, input.ApiSecret, input.Passphrase, input.Algorithm)
		if err != nil {
			log.Err(err).Msg("failed check api key/secret")
			return nil, errors.New(errors.InvalidArgument, "api key/secret is not available")
		}
	}

	po, err := entity.Account.CreateAccount(ctx, input)
	if err != nil {
		return nil, err
	}

	if input.AccountType == types.AccountTypeVirtual && len(input.InitialAssets) > 0 {
		if err := s.applyInitialAssets(ctx, po.ID, input.Exchange, input.InitialAssets); err != nil {
			return nil, err
		}
	}

	return po, nil
}

func (s *Service) UpdateAccount(ctx context.Context, request *types.UpdateAccountRequest) (*types.UpdateAccountResponse, error) {
	log.Debug().Interface("request", request).Msg("UpdateAccount params")

	if len(request.ID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}
	if !request.Exchange.IsValid() {
		return nil, errors.New(errors.InvalidArgument, "exchange is invalid")
	}
	if len(request.Name) == 0 {
		return nil, errors.New(errors.InvalidArgument, "name is required")
	}

	existingMeta, err := s.getAccount(ctx, request.ID)
	if err != nil {
		return nil, err
	}
	if existingMeta == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	// 更新不允许改账户类型；客户端未传 GraphQL account_type 时常误默认为 real，与库内类型对齐
	request.AccountType = existingMeta.AccountType

	// 虚拟账户 / 虚拟子账户：不向交易所校验密钥；子账户仅允许改名等约束在 entity 层校验
	switch existingMeta.AccountType {
	case types.AccountTypeVirtual:
		if len(request.ApiKey) == 0 {
			request.ApiKey = ""
		}
		if len(request.ApiSecret) == 0 {
			request.ApiSecret = ""
		}
		if len(request.Passphrase) == 0 {
			request.Passphrase = ""
		}
	case types.AccountTypeVirtualSub:
		// 请求体需携带与库内一致的占位密钥等，由 entity.validateVirtualSubAccountUpdate 约束
	default:
		if len(request.ApiKey) == 0 {
			return nil, errors.New(errors.InvalidArgument, "api key is required")
		}
		if len(request.ApiSecret) == 0 {
			return nil, errors.New(errors.InvalidArgument, "api secret is required")
		}
		if (request.Exchange == types.ExchangeOkx || request.Exchange == types.ExchangeOkxTest) && len(request.Passphrase) == 0 {
			return nil, errors.New(errors.InvalidArgument, "passphrase is required")
		}
	}

	if !request.Status.Valid() {
		return nil, errors.New(errors.InvalidArgument, "status is invalid")
	}
	if !request.Algorithm.Valid() {
		return nil, errors.New(errors.InvalidArgument, "algorithm is invalid")
	}

	// 校验 apiKey/apiSecret（仅真实账户且 online；虚拟子账户不调用交易所）
	if existingMeta.AccountType == types.AccountTypeReal && request.Status == types.AccountStatusOnline {
		err := s.checkAccountApiSecret(ctx, request.Exchange, request.ApiKey, request.ApiSecret, request.Passphrase, request.Algorithm)
		if err != nil {
			log.Err(err).Msg("failed check api key/secret")
			return nil, errors.New(errors.InvalidArgument, "api key/secret is not available")
		}
	}

	po, err := entity.Account.UpdateAccount(ctx, *request)
	if err != nil {
		return nil, err
	}
	return &types.UpdateAccountResponse{
		Account: po,
	}, nil
}

func (s *Service) QueryAccounts(ctx context.Context, request *types.QueryAccountsRequest) (*types.QueryAccountsResponse, error) {
	log.Debug().Interface("request", request).Msg("QueryAccounts params")
	if request.Offset < 0 {
		return nil, errors.New(errors.InvalidArgument, "offset is invalid")
	}
	if request.Limit <= 0 {
		return nil, errors.New(errors.InvalidArgument, "limit is invalid")
	}

	var sts *types.AccountStatus
	if request.Status != nil && request.Status.Valid() {
		sts = request.Status
	}
	var exchange *types.Exchange
	if request.Exchange != nil && request.Exchange.IsValid() {
		exchange = request.Exchange
	}
	var acctType *types.AccountType
	if request.AccountType != nil && request.AccountType.Valid() {
		acctType = request.AccountType
	} else if request.AccountType != nil {
		return nil, errors.New(errors.InvalidArgument, "account_type is invalid")
	}
	count, err := entity.Account.QueryAccountsCount(ctx, request.Id, exchange, request.Name, request.Tags, sts, acctType, request.CreatedAtStart, request.CreatedAtEnd)
	if err != nil {
		return nil, err
	}

	resp := &types.QueryAccountsResponse{
		Count: count,
	}

	if request.Offset >= count {
		return resp, nil
	}

	accounts, err := entity.Account.QueryAccounts(ctx, request.Id, exchange, request.Name, request.Tags, sts, acctType,
		request.CreatedAtStart, request.CreatedAtEnd, request.Offset, request.Limit)
	if err != nil {
		return nil, err
	}

	resp.Accounts = accounts
	return resp, nil
}

func (s *Service) QueryRiskEvents(ctx context.Context, req *types.QueryRiskEventsRequest) (*types.QueryRiskEventsResponse, error) {
	if strings.TrimSpace(req.AccountID) == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.RiskEventRepo.ListRiskEventsByAccount(ctx, risk_event.ListRiskEventsByAccountParams{
		AccountID: req.AccountID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, err
	}

	events := make([]*types.RiskEvent, 0, len(rows))
	for _, r := range rows {
		riskIndexStr := ""
		if r.RiskIndex.Valid {
			riskIndexStr = utils.Decimal.PgNumericToDecimal(r.RiskIndex).String()
		}
		re := &types.RiskEvent{
			ID:        r.ID,
			AccountID: r.AccountID,
			Exchange:  r.Exchange,
			Rule:      r.Rule,
			RiskIndex: riskIndexStr,
			PayloadJSON: func() string {
				if r.Payload == nil {
					return ""
				}
				return string(r.Payload)
			}(),
			CreatedAt: r.CreatedAt.Unix(),
		}
		events = append(events, re)
	}

	return &types.QueryRiskEventsResponse{
		Events: events,
	}, nil
}

func (s *Service) GetRiskIndex(ctx context.Context, req *types.GetRiskIndexRequest) (*types.GetRiskIndexResponse, error) {
	if req == nil || strings.TrimSpace(req.AccountID) == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}

	riskIndex, err := entity.Account.GetRiskIndex(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	return &types.GetRiskIndexResponse{
		RiskIndex: riskIndex.String(),
	}, nil
}

// UpdateAccountRiskConfig 仅更新账户级风控配置（account.config.risk）
func (s *Service) UpdateAccountRiskConfig(ctx context.Context, req *types.UpdateAccountRiskConfigRequest) (*types.UpdateAccountRiskConfigResponse, error) {
	log.Debug().Interface("request", req).Msg("UpdateAccountRiskConfig params")
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if req.AccountID == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}

	amountLimitFromStr := func(src *types.AmountLimitStrings) types.AmountLimit {
		if src == nil {
			return types.AmountLimit{}
		}
		return types.AmountLimit{
			Amount: stringToDecimal(src.Amount),
			Ratio:  stringToDecimal(src.Ratio),
		}
	}
	// 构建 AccountRiskConfig
	risk := &types.RiskConfig{
		MaxOrderSize:              stringToDecimal(req.MaxOrderSize),
		MaxLeverage:               stringToDecimal(req.MaxLeverage),
		MaxOrdersPerMinute:        int(req.MaxOrdersPerMinute),
		MinMaintenanceMarginRatio: stringToDecimal(req.MinMaintenanceMarginRatio),
		RiskIndexThreshold:        stringToDecimal(req.RiskIndexThreshold),
		RiskIndexAction:           strings.TrimSpace(req.RiskIndexAction),
		CooldownSeconds:           req.CooldownSeconds,
		MaxPositionPerSymbol:      amountLimitFromStr(req.MaxPositionPerSymbol),
		MaxDailyLoss:              amountLimitFromStr(req.MaxDailyLoss),
		MaxTotalNetExposure:       amountLimitFromStr(req.MaxTotalNetExposure),
		MaxTotalGrossExposure:     amountLimitFromStr(req.MaxTotalGrossExposure),
	}

	po, err := entity.Account.UpdateAccountRiskConfig(ctx, req.AccountID, risk)
	if err != nil {
		return nil, err
	}
	return &types.UpdateAccountRiskConfigResponse{
		Account: po,
	}, nil
}

func (s *Service) DeleteAccount(ctx context.Context, request *types.DeleteAccountRequest) (*types.DeleteAccountResponse, error) {
	log.Debug().Interface("request", request).Msg("DeleteAccount params")
	if request == nil || len(request.Id) == 0 {
		return nil, errors.New(errors.InvalidArgument, "id is invalid")
	}

	// 取消订阅 account stream
	if entity.Market != nil {
		entity.Market.ReleaseSubscriptionsForAccount(ctx, request.Id)
	}
	if err := entity.Account.DeleteAccount(ctx, request.Id); err != nil {
		return nil, err
	}

	return &types.DeleteAccountResponse{}, nil
}

func (s *Service) OnlineAccount(ctx context.Context, request *types.OnlineAccountRequest) (*types.OnlineAccountResponse, error) {
	log.Debug().Interface("request", request).Msg("OnlineAccount params")
	if request == nil || len(request.Id) == 0 {
		return nil, errors.New(errors.InvalidArgument, "id is invalid")
	}

	account, err := entity.Account.GetAccount(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}

	if account.AccountType != types.AccountTypeVirtual {
		err = s.checkAccountApiSecret(ctx, account.Exchange, account.ApiKey, account.ApiSecret, account.Passphrase, account.Algorithm)
		if err != nil {
			log.Err(err).Msg("failed check api key/secret")
			return nil, errors.New(errors.InvalidArgument, "api key/secret is not available")
		}
	}

	po, err := entity.Account.UpdateAccountStatus(ctx, request.Id, types.AccountStatusOnline)
	if err != nil {
		return nil, err
	}
	// 新上线的账户自动订阅 account stream（与 autoSubscribeAccountStreams 逻辑一致）
	if po.AccountType != types.AccountTypeVirtual && entity.Market != nil {
		accountID := po.ID
		selector := types.StreamSelector{
			Stream:  types.StreamTypeAccountRaw,
			Account: &accountID,
		}
		_, subErr := entity.Market.EnsureSubscription(ctx, po.Exchange, selector)
		if subErr != nil {
			log.Err(subErr).Str("account_id", po.ID).Str("exchange", po.Exchange.String()).Msg("failed to subscribe account stream after online")
		} else {
			log.Info().Str("account_id", po.ID).Str("exchange", po.Exchange.String()).Msg("subscribed account stream after online")
		}
	}
	return &types.OnlineAccountResponse{
		Account: po,
	}, nil
}

func (s *Service) OfflineAccount(ctx context.Context, request *types.OfflineAccountRequest) (*types.OfflineAccountResponse, error) {
	log.Debug().Interface("request", request).Msg("OfflineAccount params")
	if request == nil || len(request.Id) == 0 {
		return nil, errors.New(errors.InvalidArgument, "id is invalid")
	}

	// 下线的账户自动取消订阅 account stream
	if entity.Market != nil {
		entity.Market.ReleaseSubscriptionsForAccount(ctx, request.Id)
	}
	po, err := entity.Account.UpdateAccountStatus(ctx, request.Id, types.AccountStatusOffline)
	if err != nil {
		return nil, err
	}
	return &types.OfflineAccountResponse{
		Account: po,
	}, nil
}

func (s *Service) RefreshAccountSnapshots(ctx context.Context, request *types.RefreshAccountSnapshotsRequest) (*types.RefreshAccountSnapshotsResponse, error) {
	log.Debug().Interface("request", request).Msg("RefreshAccountSnapshots params")
	if request == nil || len(request.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is invalid")
	}

	if err := entity.Account.RefreshSingleAccountSnapshots(ctx, request.AccountID); err != nil {
		return nil, err
	}

	return &types.RefreshAccountSnapshotsResponse{
		Success: true,
	}, nil
}

func (s *Service) QueryEquitys(ctx context.Context, request *types.QueryEquitysRequest) (*types.QueryEquitysResponse, error) {
	log.Debug().Interface("request", request).Msg("QueryEquitys params")
	if request == nil || len(request.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is invalid")
	}
	startTs := time.UnixMilli(request.StartTs)
	endTs := time.UnixMilli(request.EndTs)
	if endTs.Before(startTs) {
		return nil, errors.New(errors.InvalidArgument, "end_ts must be >= start_ts")
	}

	list, err := entity.Account.QueryEquities(ctx, request.AccountID, startTs, endTs)
	if err != nil {
		return nil, err
	}

	resp := &types.QueryEquitysResponse{}
	if len(list) == 0 {
		return resp, nil
	}

	resp.Equitys = make([]*types.Equity, 0, len(list))
	for _, item := range list {
		if item == nil {
			continue
		}
		resp.Equitys = append(resp.Equitys, item)
	}
	return resp, nil
}

func (s *Service) QuerySymbolEquity(ctx context.Context, request *types.QuerySymbolEquityRequest) (*types.QuerySymbolEquityResponse, error) {
	if request == nil || len(request.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	startTs := time.UnixMilli(request.StartTs)
	endTs := time.UnixMilli(request.EndTs)
	if endTs.Before(startTs) {
		return nil, errors.New(errors.InvalidArgument, "end_ts must be >= start_ts")
	}

	var exPtr, symPtr *string
	if strings.TrimSpace(request.Exchange) != "" {
		ex := strings.TrimSpace(request.Exchange)
		exPtr = &ex
	}
	if strings.TrimSpace(request.Symbol) != "" {
		sym := strings.TrimSpace(request.Symbol)
		symPtr = &sym
	}

	list, err := s.db.SymbolEquityRepo.ListSymbolEquityByAccountAndRange(ctx, symbol_equity.ListSymbolEquityByAccountAndRangeParams{
		AccountID: request.AccountID,
		Ts:        startTs,
		Ts_2:      endTs,
		Exchange:  exPtr,
		Symbol:    symPtr,
	})
	if err != nil {
		return nil, err
	}
	resp := &types.QuerySymbolEquityResponse{
		Items: make([]*types.SymbolEquity, 0, len(list)),
	}
	for i := range list {
		row := &list[i]
		resp.Items = append(resp.Items, &types.SymbolEquity{
			ID:           row.ID,
			AccountID:    row.AccountID,
			Exchange:     row.Exchange,
			Symbol:       row.Symbol,
			NetValue:     utils.Decimal.PgNumericToDecimal(row.NetValue).String(),
			BaseCurrency: row.BaseCurrency,
			Ts:           row.Ts.UnixMilli(),
			CreatedAt:    row.CreatedAt.Unix(),
		})
	}
	return resp, nil
}

func (s *Service) applyInitialAssets(ctx context.Context, accountID string, exchange types.Exchange, assets []types.AssetInput) error {
	if !exchange.IsValid() {
		return errors.New(errors.InvalidArgument, "exchange is invalid")
	}
	if len(assets) == 0 {
		return errors.New(errors.InvalidArgument, "assets is required")
	}

	now := time.Now()
	for i := range assets {
		item := &assets[i]
		if len(item.Asset) == 0 {
			return errors.New(errors.InvalidArgument, "asset is required")
		}
		if !item.WalletType.Valid() {
			return errors.New(errors.InvalidArgument, "wallet_type is invalid")
		}
		if item.Total == "" {
			return errors.New(errors.InvalidArgument, "total is required")
		}
		total, err := decimal.NewFromString(item.Total)
		if err != nil || total.IsNegative() {
			return errors.New(errors.InvalidArgument, "total is invalid")
		}
		frozen := decimal.Zero
		if item.Frozen != "" {
			frozen, err = decimal.NewFromString(item.Frozen)
			if err != nil || frozen.IsNegative() {
				return errors.New(errors.InvalidArgument, "frozen is invalid")
			}
		}
		if frozen.GreaterThan(total) {
			return errors.New(errors.InvalidArgument, "total must be >= frozen")
		}

		_, err = entity.Account.ApplyAssetSnapshot(ctx, accountID, exchange, item.WalletType, item.Asset, &total, &frozen, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func allocKey(asset string, wt types.WalletType) string {
	return types.ParseAssetCode(asset) + "|" + string(wt)
}

func positionAllocKey(symbol string, side types.PositionSide) string {
	return strings.ToUpper(strings.TrimSpace(symbol)) + "|" + string(side)
}

// GetAccountUnallocatedAssets 父账户 multi_bot 模式下，各资产维度「父余额 − 子账已登记初始分配」。
// P2 归因权重 w_unalloc 与此同键同口径，见 docs/P2_T0_VIRTUAL_SUB_ATTRIBUTION.md §3。
func (s *Service) GetAccountUnallocatedAssets(ctx context.Context, req *types.GetAccountUnallocatedAssetsRequest) (*types.GetAccountUnallocatedAssetsResponse, error) {
	if req == nil || strings.TrimSpace(req.ParentAccountID) == "" {
		return nil, errors.New(errors.InvalidArgument, "parent_account_id is required")
	}
	parent, err := entity.Account.GetAccount(ctx, req.ParentAccountID)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	if parent.AccountType != types.AccountTypeReal || !parent.MultiBotMode {
		return nil, errors.New(errors.InvalidArgument, "account must be a real parent with multi_bot_mode enabled")
	}
	parentBal, err := s.GetBalance(ctx, &types.GetBalanceRequest{
		AccountID:    req.ParentAccountID,
		WithNotional: lo.ToPtr(false),
	})
	if err != nil {
		return nil, err
	}
	if parentBal == nil || parentBal.Balance == nil {
		return &types.GetAccountUnallocatedAssetsResponse{Items: nil}, nil
	}

	subs, err := s.db.AccountRepo.ListVirtualSubByParent(ctx, &req.ParentAccountID)
	if err != nil {
		return nil, err
	}
	subTotals := make(map[string]decimal.Decimal)
	for i := range subs {
		rows, err := s.db.AssetsRepo.ListAssetsByAccount(ctx, subs[i].ID)
		if err != nil {
			return nil, err
		}
		for j := range rows {
			r := &rows[j]
			wt := types.WalletType(r.WalletType)
			if !wt.Valid() {
				continue
			}
			k := allocKey(r.Asset, wt)
			subTotals[k] = subTotals[k].Add(utils.Decimal.PgNumericToDecimal(r.Total))
		}
	}

	out := make([]*types.AccountUnallocatedAsset, 0)
	for _, a := range parentBal.Balance.Assets {
		if a == nil {
			continue
		}
		k := allocKey(a.Code, a.WalletType)
		subsAl := subTotals[k]
		free := a.Balance
		unalloc := free.Sub(subsAl)
		if unalloc.IsNegative() {
			unalloc = decimal.Zero
		}
		out = append(out, &types.AccountUnallocatedAsset{
			Asset:         types.ParseAssetCode(a.Code),
			WalletType:    a.WalletType,
			ParentTotal:   free,
			SubsAllocated: subsAl,
			Unallocated:   unalloc,
		})
	}
	return &types.GetAccountUnallocatedAssetsResponse{Items: out}, nil
}

func (s *Service) GetAccountMultiBotDetails(ctx context.Context, req *types.GetAccountMultiBotDetailsRequest) (*types.GetAccountMultiBotDetailsResponse, error) {
	if req == nil || strings.TrimSpace(req.ParentAccountID) == "" {
		return nil, errors.New(errors.InvalidArgument, "parent_account_id is required")
	}
	parent, err := entity.Account.GetAccount(ctx, req.ParentAccountID)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	if parent.AccountType != types.AccountTypeReal || !parent.MultiBotMode {
		return nil, errors.New(errors.InvalidArgument, "account must be a real parent with multi_bot_mode enabled")
	}

	subs, err := s.db.AccountRepo.ListVirtualSubByParent(ctx, &req.ParentAccountID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(subs, func(i, j int) bool {
		return subs[i].CreatedAt.Before(subs[j].CreatedAt)
	})
	subAccounts := make([]*types.MultiBotSubAccount, 0, len(subs))
	subIDs := make([]string, 0, len(subs))
	for i := range subs {
		subAccounts = append(subAccounts, &types.MultiBotSubAccount{
			AccountID: subs[i].ID,
			Name:      subs[i].Name,
			CreatedAt: subs[i].CreatedAt.Unix(),
		})
		subIDs = append(subIDs, subs[i].ID)
	}

	parentBal, err := s.GetBalance(ctx, &types.GetBalanceRequest{
		AccountID:    req.ParentAccountID,
		WithNotional: lo.ToPtr(false),
	})
	if err != nil {
		return nil, err
	}
	subAssetMap := make(map[string]map[string]decimal.Decimal)
	for _, sid := range subIDs {
		subBal, err := s.GetBalance(ctx, &types.GetBalanceRequest{
			AccountID:    sid,
			WithNotional: lo.ToPtr(false),
		})
		if err != nil {
			return nil, err
		}
		if subBal == nil || subBal.Balance == nil {
			continue
		}
		for _, a := range subBal.Balance.Assets {
			if a == nil {
				continue
			}
			key := allocKey(a.Code, a.WalletType)
			if _, ok := subAssetMap[key]; !ok {
				subAssetMap[key] = make(map[string]decimal.Decimal)
			}
			subAssetMap[key][sid] = a.Balance
		}
	}

	assetAllocations := make([]*types.MultiBotAssetAllocation, 0)
	if parentBal != nil && parentBal.Balance != nil {
		for _, a := range parentBal.Balance.Assets {
			if a == nil {
				continue
			}
			key := allocKey(a.Code, a.WalletType)
			subAlloc := make(map[string]decimal.Decimal, len(subIDs))
			sumSub := decimal.Zero
			for _, sid := range subIDs {
				val := decimal.Zero
				if v, ok := subAssetMap[key][sid]; ok {
					val = v
				}
				subAlloc[sid] = val
				sumSub = sumSub.Add(val)
			}
			unallocated := a.Balance.Sub(sumSub)
			if unallocated.IsNegative() {
				unallocated = decimal.Zero
			}
			assetAllocations = append(assetAllocations, &types.MultiBotAssetAllocation{
				Asset:          types.ParseAssetCode(a.Code),
				WalletType:     a.WalletType,
				ParentTotal:    a.Balance,
				SubAllocations: subAlloc,
				Unallocated:    unallocated,
			})
		}
	}
	sort.SliceStable(assetAllocations, func(i, j int) bool {
		li := allocKey(assetAllocations[i].Asset, assetAllocations[i].WalletType)
		lj := allocKey(assetAllocations[j].Asset, assetAllocations[j].WalletType)
		return li < lj
	})

	parentPos, err := s.GetPositions(ctx, &types.GetPositionsRequest{AccountID: req.ParentAccountID})
	if err != nil {
		return nil, err
	}
	subPosMap := make(map[string]map[string]decimal.Decimal)
	for _, sid := range subIDs {
		resp, err := s.GetPositions(ctx, &types.GetPositionsRequest{AccountID: sid})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			continue
		}
		for _, p := range resp.Positions {
			if p == nil {
				continue
			}
			key := positionAllocKey(p.Symbol.String(), p.Side)
			if _, ok := subPosMap[key]; !ok {
				subPosMap[key] = make(map[string]decimal.Decimal)
			}
			subPosMap[key][sid] = subPosMap[key][sid].Add(p.Amount)
		}
	}

	positionAllocations := make([]*types.MultiBotPositionAllocation, 0)
	if parentPos != nil {
		for _, p := range parentPos.Positions {
			if p == nil {
				continue
			}
			key := positionAllocKey(p.Symbol.String(), p.Side)
			subAlloc := make(map[string]decimal.Decimal, len(subIDs))
			sumSub := decimal.Zero
			for _, sid := range subIDs {
				val := decimal.Zero
				if v, ok := subPosMap[key][sid]; ok {
					val = v
				}
				subAlloc[sid] = val
				sumSub = sumSub.Add(val)
			}
			unallocated := p.Amount.Sub(sumSub)
			if unallocated.IsNegative() {
				unallocated = decimal.Zero
			}
			positionAllocations = append(positionAllocations, &types.MultiBotPositionAllocation{
				Symbol:         p.Symbol.String(),
				Side:           p.Side,
				ParentTotal:    p.Amount,
				SubAllocations: subAlloc,
				Unallocated:    unallocated,
			})
		}
	}
	sort.SliceStable(positionAllocations, func(i, j int) bool {
		ki := positionAllocKey(positionAllocations[i].Symbol, positionAllocations[i].Side)
		kj := positionAllocKey(positionAllocations[j].Symbol, positionAllocations[j].Side)
		return ki < kj
	})

	return &types.GetAccountMultiBotDetailsResponse{
		SubAccounts:         subAccounts,
		AssetAllocations:    assetAllocations,
		PositionAllocations: positionAllocations,
	}, nil
}

// CreateVirtualSubAccount 在父账户 multi_bot 模式下创建 virtual_sub 并写入初始资产快照。
func (s *Service) CreateVirtualSubAccount(ctx context.Context, input types.CreateVirtualSubAccountInput) (*types.Account, error) {
	if strings.TrimSpace(input.ParentAccountID) == "" {
		return nil, errors.New(errors.InvalidArgument, "parent_account_id is required")
	}
	if len(strings.TrimSpace(input.BotName)) == 0 {
		return nil, errors.New(errors.InvalidArgument, "bot_name is required")
	}
	if len(input.InitialAssets) == 0 {
		return nil, errors.New(errors.InvalidArgument, "initial assets is required")
	}
	parent, err := entity.Account.GetAccount(ctx, input.ParentAccountID)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, errors.New(errors.NotFound, "parent account not found")
	}
	if parent.AccountType != types.AccountTypeReal || !parent.MultiBotMode {
		return nil, errors.New(errors.InvalidArgument, "parent must be a real account with multi_bot_mode enabled")
	}
	unalloc, err := s.GetAccountUnallocatedAssets(ctx, &types.GetAccountUnallocatedAssetsRequest{ParentAccountID: input.ParentAccountID})
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]decimal.Decimal)
	for _, row := range unalloc.Items {
		if row == nil {
			continue
		}
		allowed[allocKey(row.Asset, row.WalletType)] = row.Unallocated
	}
	// 同一资产+钱包多行时按 key 汇总后再与未分配额度比较
	perKeyTotal := make(map[string]decimal.Decimal)
	for i := range input.InitialAssets {
		a := &input.InitialAssets[i]
		if !a.WalletType.Valid() {
			return nil, errors.New(errors.InvalidArgument, "wallet_type is invalid")
		}
		total, err := decimal.NewFromString(a.Total)
		if err != nil || total.IsNegative() {
			return nil, errors.New(errors.InvalidArgument, "initial asset total is invalid")
		}
		if total.IsZero() {
			continue
		}
		k := allocKey(a.Asset, a.WalletType)
		perKeyTotal[k] = perKeyTotal[k].Add(total)
	}
	for k, sum := range perKeyTotal {
		u, ok := allowed[k]
		if !ok {
			u = decimal.Zero
		}
		if sum.GreaterThan(u) {
			return nil, errors.New(errors.InvalidArgument, fmt.Sprintf(
				"initial allocation for %s exceeds unallocated balance: requested %s, available %s",
				k, sum.String(), u.String()))
		}
	}

	subName := fmt.Sprintf("%s-%s-%s", parent.Name, strings.TrimSpace(input.BotName), snowflake.Generate().String())
	mbFalse := false
	acc, err := entity.Account.CreateAccount(ctx, types.CreateAccountInput{
		Name:            subName,
		Exchange:        parent.Exchange,
		ApiKey:          "",
		ApiSecret:       "",
		Passphrase:      "",
		Tags:            nil,
		Status:          types.AccountStatusOnline,
		Algorithm:       parent.Algorithm,
		AccountType:     types.AccountTypeVirtualSub,
		ParentAccountID: lo.ToPtr(input.ParentAccountID),
		MultiBotMode:    &mbFalse,
	})
	if err != nil {
		return nil, err
	}
	if err := s.applyInitialAssets(ctx, acc.ID, parent.Exchange, input.InitialAssets); err != nil {
		return nil, err
	}
	return entity.Account.GetAccount(ctx, acc.ID)
}

func (s *Service) checkAccountApiSecret(ctx context.Context, exchange types.Exchange, apiKey, apiSecret, passphrase string, algorithm types.AuthAlgorithm) error {
	destApiSecret, err := encrypt.DecryptBase64(apiSecret)
	if err != nil {
		return errors.New(errors.InvalidArgument, "api secret is invalid")
	}
	httpProxyURL, err := settings.GetHttpProxyURL(ctx)
	if err != nil {
		return fmt.Errorf("get http proxy url: %w", err)
	}
	useTestnet := exchange.IsTestnet()
	switch exchange {
	case types.ExchangeBinance, types.ExchangeBinanceTest:
		var client *binance.Client

		if httpProxyURL != "" {
			client = binance.NewProxiedClient(apiKey, string(destApiSecret), httpProxyURL)
		} else {
			client = binance.NewClient(apiKey, string(destApiSecret))
		}
		if exchange == types.ExchangeBinanceTest {
			client.SetUseDemo()
		}
		switch algorithm {
		case types.AuthAlgorithmHmac:
			client.KeyType = common.KeyTypeHmac
		case types.AuthAlgorithmEd25519:
			client.KeyType = common.KeyTypeEd25519
		case types.AuthAlgorithmRsa:
			client.KeyType = common.KeyTypeRsa
		}
		_, err = client.NewGetAccountService().Do(ctx)
		if err != nil {
			return fmt.Errorf("check account secret: %w", err)
		}
	case types.ExchangeOkx, types.ExchangeOkxTest:
		client := okxsdk.NewRestClient(okxsdk.Config{
			ApiKey:     apiKey,
			ApiSecret:  destApiSecret,
			Passphrase: passphrase,
			IsTestNet:  useTestnet,
			Proxy:      lo.ToPtr(httpProxyURL),
		})
		_, err := client.GetAccountBalance(ctx)
		if err != nil {
			return fmt.Errorf("check account secret: %w", err)
		}
	}
	return nil
}

func (s *Service) GetAccount(ctx context.Context, req *types.GetAccountRequest) (*types.GetAccountResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, acct.Exchange, acct.ID)
	if err != nil {
		return nil, err
	}
	account, err := conn.Account(ctx)
	if err != nil {
		return nil, err
	}
	return &types.GetAccountResponse{Account: account}, nil
}

func (s *Service) GetBalance(ctx context.Context, req *types.GetBalanceRequest) (*types.GetBalanceResponse, error) {
	if req == nil || len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	withNotional := true
	if req.WithNotional != nil {
		withNotional = *req.WithNotional
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	assetsPo, err := entity.Account.GetAssets(ctx, acct.ID)
	if err != nil {
		return nil, err
	}

	bos := make([]*types.AssetBo, 0, len(assetsPo))
	for _, p := range assetsPo {
		if p == nil {
			continue
		}
		bos = append(bos, &types.AssetBo{
			AccountID:  p.AccountID,
			WalletType: p.WalletType,
			Code:       p.Code,
			Balance:    p.Balance,
			Locked:     p.Locked(),
			AvgPrice:   p.AvgPrice,
			UpdatedTs:  p.UpdatedTs,
		})
	}

	balance := &types.Balance{}

	if withNotional {
		conn, err := entity.Account.GetConnector(ctx, acct.Exchange, acct.ID)
		if err != nil {
			return nil, fmt.Errorf("get exchange connector: %w", err)
		}
		priceMap, err := s.getSpotPriceMap(ctx, conn)
		if err != nil {
			return nil, err
		}
		lockedByAsset, uplByAsset, err := s.calcAssetLockedAndUpl(ctx, conn, *acct)
		if err != nil {
			return nil, err
		}

		totalUpl := decimal.Zero
		totalNotional := decimal.Zero
		for _, bo := range bos {
			if bo == nil {
				continue
			}
			key := assetKey{code: types.ParseAssetCode(bo.Code), walletType: bo.WalletType}
			if locked, ok := lockedByAsset[key]; ok {
				bo.Locked = bo.Locked.Add(locked)
			}
			if upl, ok := uplByAsset[key]; ok {
				bo.UnRealizedProfit = bo.UnRealizedProfit.Add(upl)
			}
			uplUsdt := s.calcAssetNotional(bo.Code, bo.UnRealizedProfit, priceMap)
			// USDT 为基础计价单位，行内现金价值应等于余额；不要把合约 UPL 并入 notional，
			// 否则前端用 notional/balance 推导的「均价」会错误地 >1。
			qtyForNotional := bo.Balance.Add(bo.UnRealizedProfit)
			strippedUpl := decimal.Zero
			if types.ParseAssetCode(bo.Code) == "USDT" {
				strippedUpl = bo.UnRealizedProfit
				qtyForNotional = bo.Balance
			}
			bo.Notional = s.calcAssetNotional(bo.Code, qtyForNotional, priceMap)
			totalUpl = totalUpl.Add(uplUsdt)
			totalNotional = totalNotional.Add(bo.Notional).Add(strippedUpl)
		}

		notional24HChange := s.getNotional24HChange(ctx, acct.ID, totalNotional)

		balance.Notional = totalNotional
		balance.UnRealizedProfit = totalUpl
		if notional24HChange != nil {
			balance.Notional24HChange = *notional24HChange
		}
	}

	for _, bo := range bos {
		if bo == nil {
			continue
		}
		balance.Assets = append(balance.Assets, bo)
	}

	return &types.GetBalanceResponse{Balance: balance}, nil
}

func (s *Service) GetPositions(ctx context.Context, req *types.GetPositionsRequest) (*types.GetPositionsResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	conn, err := entity.Account.GetConnector(ctx, acct.Exchange, acct.ID)
	if err != nil {
		return nil, err
	}

	positions := make([]*types.Position, 0)
	pos, err := entity.Account.GetPositions(ctx, acct.ID)
	if err != nil {
		return nil, err
	}
	if err := s.completePositions(ctx, *acct, conn, pos, true); err != nil {
		return nil, err
	}
	symbolLiqPriceMap := make(map[string]decimal.Decimal) // symbol -> liquidation price
	for _, p := range pos {
		if p.Amount.IsZero() {
			continue
		}
		if err := s.normalizePosition(ctx, conn, p); err != nil {
			logger.Ctx(ctx).Err(err).Msg("failed normalize position")
			continue
		}
		liqPrice, ok := symbolLiqPriceMap[p.Symbol.String()]
		if !ok {
			price, err := s.calcLiquidationPrice(ctx, *acct, conn, p.Symbol, pos, p.MarkPrice)
			if err != nil {
				logger.Ctx(ctx).Err(err).Msg("failed calc liquidation price")
				continue
			}
			symbolLiqPriceMap[p.Symbol.String()] = price
			liqPrice = price
		}
		p.LiquidationPrice = liqPrice
		positions = append(positions, p)
	}

	return &types.GetPositionsResponse{Positions: positions}, nil
}

// EstimateOrder 下单预估接口：在真实下单前，对本次订单的核心风险/收益信息进行统一预估。
// 当前实现：
//   - 合约开仓：预估强平价格 + 预估手续费
//   - 合约平仓：预估平仓盈亏（不含手续费）+ 预估手续费
//   - 现货：仅预估手续费（不计算强平价格 / 盈亏）
func (s *Service) EstimateOrder(ctx context.Context, req *types.EstimateOrderRequest) (*types.EstimateOrderResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if req.AccountID == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if strings.TrimSpace(req.Symbol) == "" {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}

	priceDec, err := decimal.NewFromString(strings.TrimSpace(req.Price))
	if err != nil || !priceDec.IsPositive() {
		return nil, errors.New(errors.InvalidArgument, "price must be a positive decimal")
	}
	quoteQtyDec, err := decimal.NewFromString(strings.TrimSpace(req.QuoteQty))
	if err != nil || !quoteQtyDec.IsPositive() {
		return nil, errors.New(errors.InvalidArgument, "quote_qty must be a positive decimal")
	}
	baseQty := quoteQtyDec.Div(priceDec)
	if !baseQty.IsPositive() {
		return nil, errors.New(errors.InvalidArgument, "derived base quantity must be positive")
	}

	symbol, err := types.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}
	if !symbol.IsValid() {
		return nil, errors.New(errors.InvalidArgument, "invalid symbol")
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}

	side := types.PositionSideLong
	if req.Side == types.PositionSideShort {
		side = types.PositionSideShort
	}

	conn, err := entity.Account.GetConnector(ctx, acct.Exchange, acct.ID)
	if err != nil {
		return nil, err
	}

	// 是否为“开仓方向”：多+买 / 空+卖
	isOpen := (req.Side == types.PositionSideLong && req.IsBuy) ||
		(req.Side == types.PositionSideShort && !req.IsBuy)

	var liquidationPrice string
	var expectedPnlStr string

	// 仅合约支持强平价 / 平仓盈亏预估
	if symbol.Type == types.MarketTypeFuture {
		existingPositions, err := entity.Account.GetPositions(ctx, acct.ID)
		if err != nil {
			return nil, err
		}

		if isOpen {
			// 优先使用请求中的 leverage，其次使用当前持仓或交易所默认杠杆
			var leverage int = int(req.Leverage)
			for _, pos := range existingPositions {
				if pos != nil && pos.Symbol.Equal(symbol) && pos.Leverage > 0 {
					leverage = pos.Leverage
					break
				}
			}
			if leverage <= 0 {
				config, err := conn.SymbolConfig(ctx, symbol)
				if err == nil && config != nil && len(config.CrossLeverage) > 0 {
					leverage = int(config.CrossLeverage[0])
				}
			}
			if leverage <= 0 {
				leverage = 1
			}

			hypotheticalPosition := &types.Position{
				AccountID:  acct.ID,
				Exchange:   acct.Exchange,
				Symbol:     symbol,
				Side:       side,
				Isolated:   false,
				Amount:     baseQty,
				EntryPrice: priceDec,
				Leverage:   leverage,
			}
			allPositions := append(existingPositions, hypotheticalPosition)

			markPrices, err := conn.MarkPrices(ctx)
			if err != nil {
				return nil, err
			}
			if len(markPrices) == 0 {
				return nil, errors.New(errors.NotFound, "mark prices not found")
			}
			var markPrice decimal.Decimal
			for _, mp := range markPrices {
				if mp != nil && mp.Symbol.Equal(symbol) {
					markPrice = mp.MarkPrice
					break
				}
			}
			if markPrice.IsZero() {
				return nil, errors.New(errors.NotFound, "mark price is zero")
			}

			liqPrice, err := s.calcLiquidationPrice(ctx, *acct, conn, symbol, allPositions, markPrice)
			if err != nil {
				return nil, err
			}
			if !liqPrice.IsZero() {
				liquidationPrice = liqPrice.String()
			}
		} else {
			// 平仓方向：基于当前持仓估算本次平仓盈亏（不含手续费）
			var posForSymbol *types.Position
			for _, pos := range existingPositions {
				if pos == nil {
					continue
				}
				if pos.Symbol.Equal(symbol) && pos.Side == side {
					posForSymbol = pos
					break
				}
			}

			if posForSymbol == nil || posForSymbol.Amount.LessThan(baseQty) {
				return nil, errors.New(errors.InvalidArgument, "not enough positions to close")
			}

			if posForSymbol.EntryPrice.GreaterThan(decimal.Zero) {
				pnl := decimal.Zero
				switch posForSymbol.Side {
				case types.PositionSideLong:
					// 多头平仓：平仓价 - 开仓价
					pnl = priceDec.Sub(posForSymbol.EntryPrice).Mul(baseQty)
				case types.PositionSideShort:
					// 空头平仓：开仓价 - 平仓价
					pnl = posForSymbol.EntryPrice.Sub(priceDec).Mul(baseQty)
				}
				if !pnl.IsZero() {
					expectedPnlStr = pnl.String()
				}
			}
		}
	}

	// 使用统一的 CalcOrderFee 计算手续费：
	// - 对于现货：根据买/卖方向与成交额自动选择 base / quote 计价资产
	// - 对于合约：按名义价值和费率计算
	orderForFee := types.Order{
		AccountID:        acct.ID,
		Exchange:         acct.Exchange,
		Symbol:           symbol,
		Side:             types.PositionSideLong, // 手续费计算与 Side 关系不大，这里固定为 long
		IsBuy:            req.IsBuy,
		OrderType:        types.OrderType(req.OrderType),
		ExecutedQty:      baseQty,
		ExecutedQuoteQty: quoteQtyDec,
		AvgPrice:         priceDec,
		// 预估时无法准确判断是否 maker，这里默认走 taker 费率（更保守）
		PostOnly: false,
	}

	var feeStr, feeAssetStr string
	var feeDec *decimal.Decimal
	var feeAsset *string
	feeDec, feeAsset, err = conn.CalcOrderFee(ctx, orderForFee)
	if err != nil {
		return nil, err
	}
	if feeDec != nil && !feeDec.IsZero() {
		// 对于预估展示使用绝对值，避免正负号带来困惑
		feeStr = feeDec.Abs().String()
	}
	if feeAsset != nil && strings.TrimSpace(*feeAsset) != "" {
		feeAssetStr = *feeAsset
	}

	out := &types.EstimateOrderResponse{
		LiquidationPrice: liquidationPrice,
		Fee:              feeStr,
		FeeAsset:         feeAssetStr,
		ExpectedPnl:      expectedPnlStr,
	}
	return out, nil
}

func (s *Service) GetSymbolConfig(ctx context.Context, req *types.GetSymbolConfigRequest) (*types.GetSymbolConfigResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	symbol, err := types.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}
	if !symbol.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}

	conn, err := entity.Account.GetConnector(ctx, acct.Exchange, acct.ID)
	if err != nil {
		return nil, err
	}
	cfg, err := conn.SymbolConfig(ctx, symbol)
	if err != nil {
		return nil, err
	}
	return &types.GetSymbolConfigResponse{Config: cfg}, nil
}

func (s *Service) QueryLedgers(ctx context.Context, req *types.QueryLedgersRequest) (*types.QueryLedgersResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	startTs := time.UnixMilli(req.StartTs)
	endTs := time.UnixMilli(req.EndTs)
	if endTs.Before(startTs) {
		return nil, errors.New(errors.InvalidArgument, "end_ts must be >= start_ts")
	}

	page := req.Page
	size := req.Size

	offset := (page - 1) * size
	if offset < 0 {
		offset = 0
	}
	if size <= 0 {
		size = 100
	}
	if size > 200 {
		size = 200
	}

	resp := &types.QueryLedgersResponse{}

	entries, totalCount, err := entity.Account.QueryLedgers(ctx, acct.ID, startTs, endTs, offset, size)
	if err != nil {
		return nil, err
	}
	resp.TotalCount = totalCount
	for _, l := range entries {
		if l == nil {
			continue
		}
		resp.Ledgers = append(resp.Ledgers, l)
	}

	return resp, nil
}

func (s *Service) SetLeverage(ctx context.Context, req *types.SetLeverageRequest) (*types.SetLeverageResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if len(req.Symbol) == 0 {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}
	if req.Leverage <= 0 {
		return nil, errors.New(errors.InvalidArgument, "leverage must be > 0")
	}

	symbol, err := types.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}
	if !symbol.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
	}

	leverage := int(req.Leverage)
	if leverage <= 0 {
		return nil, errors.New(errors.InvalidArgument, "leverage must be > 0")
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	lockIDs, err := entity.Account.AccountWriteLockIDsForTradingAccountChain(ctx, acct)
	if err != nil {
		return nil, err
	}

	var resp *types.SetLeverageResponse
	wrapErr := entity.Account.WithSortedAccountWrites(ctx, lockIDs, func(ctx context.Context) error {
		ctx = account.WithAccountWriteSkip(ctx)

		// 风控校验：最大杠杆
		if acct.Config != nil && acct.Config.MaxLeverage.GreaterThan(decimal.Zero) {
			maxLev := acct.Config.MaxLeverage.IntPart()
			if int64(leverage) > maxLev {
				return errors.New(errors.InvalidArgument,
					fmt.Sprintf("leverage %d exceeds max_leverage %d", leverage, maxLev))
			}
		}

		conn, connErr := entity.Account.GetConnector(ctx, acct.Exchange, acct.ID)
		if connErr != nil {
			return connErr
		}

		newLeverage, setErr := conn.SetLeverage(ctx, symbol, leverage)
		if setErr != nil {
			return setErr
		}

		if updateErr := entity.Account.UpdatePositionLeverage(ctx, acct.ID, acct.Exchange, symbol, newLeverage); updateErr != nil {
			return updateErr
		}
		resp = &types.SetLeverageResponse{Success: true, Leverage: int32(newLeverage)}
		return nil
	})
	if wrapErr != nil {
		return nil, wrapErr
	}

	return resp, nil
}

func (s *Service) GetLeverage(ctx context.Context, req *types.GetLeverageRequest) (*types.GetLeverageResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if len(req.Symbol) == 0 {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}
	symbol, err := types.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}
	if !symbol.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
	}
	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	// 优先使用本地 positions 快照中该 symbol 的杠杆值（SetLeverage 会落库），
	// 避免页面刷新后被交易所默认杠杆覆盖。
	posRows, err := s.db.PositionsRepo.ListPositionsByAccount(ctx, acct.ID)
	if err != nil {
		return nil, err
	}
	for _, row := range posRows {
		if !strings.EqualFold(strings.TrimSpace(row.Symbol), symbol.String()) {
			continue
		}
		if row.Leverage > 0 {
			return &types.GetLeverageResponse{Leverage: row.Leverage}, nil
		}
	}

	conn, err := entity.Account.GetConnector(ctx, acct.Exchange, acct.ID)
	if err != nil {
		return nil, err
	}
	config, err := conn.SymbolConfig(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if config == nil || len(config.CrossLeverage) == 0 {
		return &types.GetLeverageResponse{Leverage: 1}, nil
	}
	leverage := config.CrossLeverage[0]
	return &types.GetLeverageResponse{Leverage: int32(leverage)}, nil
}

func (s *Service) FundsFreeze(ctx context.Context, req *types.FundsFreezeRequest) (*types.FundsFreezeResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if req.AccountID == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if req.Symbol == "" {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}
	if req.FreezeType == types.FundsFreezeTypeUnspecified {
		return nil, errors.New(errors.InvalidArgument, "freeze_type is required")
	}
	asset := types.ParseAssetCode(req.Asset)
	if asset == "" {
		return nil, errors.New(errors.InvalidArgument, "asset is required")
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, "amount is invalid")
	}
	if !amount.GreaterThan(decimal.Zero) {
		return nil, errors.New(errors.InvalidArgument, "amount must be > 0")
	}

	symbol, err := types.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}
	if !symbol.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	walletType := types.GetWalletType(acct.Exchange, symbol.Type)

	var reasonCode types.LedgerReason
	var detail any
	if req.FreezeType == types.FundsFreezeTypeOrder {
		reasonCode = types.LedgerReasonFundsFreeze
		if symbol.Type == types.MarketTypeFuture {
			reasonCode = types.LedgerReasonOrderMarginFreeze
		}
		if req.Order == nil {
			return nil, errors.New(errors.InvalidArgument, "order is required")
		}
		detail = req.Order
	} else {
		return nil, errors.New(errors.InvalidArgument, "invalid freeze_type")
	}

	ts := time.Now()
	frozenDelta := amount
	err = entity.Account.CheckAndApplyAssetOrderOccupiedUpdate(
		ctx, req.AccountID,
		acct.Exchange,
		&types.AssetEvent{
			WalletType: walletType,
			Code:       asset,
			Locked:     lo.ToPtr(frozenDelta),
			UpdatedTs:  ts,
		},
		reasonCode,
		detail,
	)
	if err != nil {
		return nil, err
	}

	return &types.FundsFreezeResponse{Success: true}, nil
}

func (s *Service) FundsUnfreeze(ctx context.Context, req *types.FundsUnfreezeRequest) (*types.FundsUnfreezeResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if req.AccountID == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if req.FreezeType == types.FundsFreezeTypeUnspecified {
		return nil, errors.New(errors.InvalidArgument, "freeze_type is required")
	}
	if req.Symbol == "" {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}
	asset := types.ParseAssetCode(req.Asset)
	if asset == "" {
		return nil, errors.New(errors.InvalidArgument, "asset is required")
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, "amount is invalid")
	}
	if !amount.GreaterThan(decimal.Zero) {
		return nil, errors.New(errors.InvalidArgument, "amount must be > 0")
	}

	symbol, err := types.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}
	if !symbol.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	walletType := types.GetWalletType(acct.Exchange, symbol.Type)

	var reasonCode types.LedgerReason
	var detail any
	if req.FreezeType == types.FundsFreezeTypeOrder {
		reasonCode = types.LedgerReasonFundsUnfreeze
		if symbol.Type == types.MarketTypeFuture {
			reasonCode = types.LedgerReasonOrderMarginUnfreeze
		}
		if req.Order == nil {
			return nil, errors.New(errors.InvalidArgument, "order is required")
		}
		detail = req.Order
	} else {
		return nil, errors.New(errors.InvalidArgument, "invalid freeze_type")
	}

	ts := time.Now()
	frozenDelta := amount.Neg()
	err = entity.Account.CheckAndApplyAssetOrderOccupiedUpdate(
		ctx,
		req.AccountID,
		acct.Exchange,
		&types.AssetEvent{
			WalletType: walletType,
			Code:       asset,
			Locked:     lo.ToPtr(frozenDelta),
			UpdatedTs:  ts,
		},
		reasonCode,
		detail,
	)
	if err != nil {
		return nil, err
	}
	return &types.FundsUnfreezeResponse{Success: true}, nil
}

func (s *Service) getAccount(ctx context.Context, accountID string) (*types.Account, error) {
	acct, err := s.db.AccountRepo.GetById(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	return converter.AccountRepo2Types(acct), nil
}

func (s *Service) getSpotPriceMap(ctx context.Context, conn mdtypes.Connector) (map[string]decimal.Decimal, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	prices, err := conn.Prices(ctx, lo.ToPtr(types.MarketTypeSpot))
	if err != nil {
		return nil, fmt.Errorf("get ticker prices: %w", err)
	}

	priceMap := make(map[string]decimal.Decimal)
	for _, tp := range prices {
		if tp.Symbol.Quote != "USDT" {
			continue
		}
		priceMap[tp.Symbol.Base] = tp.Price
	}
	return priceMap, nil
}

func (s *Service) calcAssetNotional(asset string, qty decimal.Decimal, priceMap map[string]decimal.Decimal) decimal.Decimal {
	asset = types.ParseAssetCode(asset)
	var notional decimal.Decimal
	if asset == "USDT" {
		notional = qty
	} else if price, exists := priceMap[asset]; exists {
		notional = qty.Mul(price)
	} else {
		notional = decimal.Zero
	}
	return notional
}

type assetKey struct {
	code       string
	walletType types.WalletType
}

func (s *Service) calcAssetLockedAndUpl(ctx context.Context, conn mdtypes.Connector, acct types.Account) (map[assetKey]decimal.Decimal, map[assetKey]decimal.Decimal, error) {
	lockedByAsset := make(map[assetKey]decimal.Decimal)
	uplByAsset := make(map[assetKey]decimal.Decimal)

	if acct.ID == "" {
		return lockedByAsset, uplByAsset, nil
	}

	positions, err := entity.Account.GetPositions(ctx, acct.ID)
	if err != nil {
		return lockedByAsset, uplByAsset, err
	}
	if err := s.completePositions(ctx, acct, conn, positions, false); err != nil {
		return lockedByAsset, uplByAsset, err
	}

	for _, pos := range positions {
		if pos == nil || pos.Amount.IsZero() || pos.Symbol.Type != types.MarketTypeFuture {
			continue
		}
		assetCode := types.ParseAssetCode(pos.Symbol.Quote)
		if assetCode == "" {
			assetCode = "USDT"
		}
		key := assetKey{code: assetCode, walletType: types.GetWalletType(acct.Exchange, pos.Symbol.Type)}
		if pos.InitialMargin.GreaterThan(decimal.Zero) {
			lockedByAsset[key] = lockedByAsset[key].Add(pos.InitialMargin)
		}
		uplByAsset[key] = uplByAsset[key].Add(pos.UnRealizedProfit)
	}

	return lockedByAsset, uplByAsset, nil
}

func (s *Service) getNotional24HChange(ctx context.Context, accountID string, currentNotional decimal.Decimal) *decimal.Decimal {
	if accountID == "" {
		return nil
	}
	ts := time.Now().Add(-24 * time.Hour)
	row, err := s.db.EquityRepo.GetEquityBeforeTs(ctx, equity.GetEquityBeforeTsParams{AccountID: accountID, Ts: ts})
	if err != nil {
		return nil
	}
	if row == nil {
		return nil
	}
	prev := utils.Decimal.PgNumericToDecimal(row.Notional)
	diff := currentNotional.Sub(prev)
	return &diff
}

func (s *Service) completePositions(ctx context.Context, acct types.Account, conn mdtypes.Connector, positions []*types.Position, uplToUsdt bool) error {
	if len(positions) == 0 {
		return nil
	}

	cctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	markPrices, err := conn.MarkPrices(cctx)
	if err != nil {
		return err
	}
	if len(markPrices) == 0 {
		return errors.New(errors.NotFound, "mark prices not found")
	}
	markPriceMap := make(map[string]decimal.Decimal)
	for _, mp := range markPrices {
		markPriceMap[mp.Symbol.String()] = mp.MarkPrice
	}

	for _, pos := range positions {
		if pos.Amount.IsZero() || pos.Leverage <= 0 {
			continue
		}

		markPrice, ok := markPriceMap[pos.Symbol.String()]
		if !ok {
			continue
		}

		var upl decimal.Decimal
		if pos.Side == types.PositionSideLong {
			upl = pos.Amount.Mul(markPrice.Sub(pos.EntryPrice))
		} else {
			upl = pos.Amount.Mul(pos.EntryPrice.Sub(markPrice))
		}
		notional := pos.Amount.Mul(markPrice)
		quoteToUsdt := decimal.Zero
		if pos.Symbol.Quote == "USDT" {
			quoteToUsdt = decimal.NewFromInt(1)
		} else {
			quoteUsdtSymbol := types.Symbol{Base: pos.Symbol.Quote, Quote: "USDT"}
			if price, exists := markPriceMap[quoteUsdtSymbol.String()]; exists {
				quoteToUsdt = price
			}
		}

		notional = notional.Mul(quoteToUsdt)
		if uplToUsdt {
			upl = upl.Mul(quoteToUsdt)
		}

		leverageBracket, err := conn.GetLeverageBracket(ctx, pos.Symbol, markPrice)
		if err != nil {
			return err
		}

		leverage := decimal.NewFromInt(int64(pos.Leverage))
		initialMargin := calculator.CalcInitialMargin(notional, leverage)
		maintMargin := calculator.CalcMaintMargin(notional, leverageBracket)

		pos.MarkPrice = markPrice
		pos.Notional = notional
		pos.InitialMargin = initialMargin
		pos.MaintMargin = maintMargin
		pos.UnRealizedProfit = upl
	}
	return nil
}

func (s *Service) calcLiquidationPrice(ctx context.Context, acct types.Account, conn mdtypes.Connector, symbol types.Symbol, all []*types.Position, markPrice decimal.Decimal) (decimal.Decimal, error) {
	if markPrice.IsZero() {
		return decimal.Zero, errors.New(errors.NotFound, "mark price is zero")
	}

	var (
		symbolCfg   *types.SymbolConfig
		bracket     *types.LeverageBracket
		walletAsset types.Asset
	)

	errgrp := errgroup.Group{}
	errgrp.Go(func() error {
		var err error
		symbolCfg, err = conn.SymbolConfig(ctx, symbol)
		if err != nil {
			return err
		}
		if symbolCfg == nil {
			return errors.New(errors.NotFound, "symbol config not found")
		}
		return nil
	})
	errgrp.Go(func() error {
		var err error
		walletAsset, err = s.getWalletAsset(ctx, acct, symbol.Type, symbol.Quote)
		if err != nil {
			return err
		}
		return nil
	})
	errgrp.Go(func() error {
		var err error
		bracket, err = conn.GetLeverageBracket(ctx, symbol, markPrice)
		if err != nil {
			return err
		}
		return nil
	})
	if err := errgrp.Wait(); err != nil {
		return decimal.Zero, err
	}

	return calculator.CalcLiquidationPrice(symbolCfg, bracket, walletAsset, all, markPrice)
}

func (s *Service) normalizePosition(ctx context.Context, conn mdtypes.Connector, pos *types.Position) error {
	if pos == nil {
		return nil
	}
	symbolCfg, err := conn.SymbolConfig(ctx, pos.Symbol)
	if err != nil {
		return err
	}
	if symbolCfg == nil {
		return errors.New(errors.NotFound, "symbol config not found")
	}

	pos.EntryPrice = pos.EntryPrice.RoundCeil(int32(symbolCfg.Market.PricePrecision))
	pos.MarkPrice = pos.MarkPrice.RoundCeil(int32(symbolCfg.Market.PricePrecision))
	pos.LiquidationPrice = pos.LiquidationPrice.RoundCeil(int32(symbolCfg.Market.PricePrecision))
	pos.Notional = pos.Notional.RoundCeil(int32(symbolCfg.Market.QuoteAssetPrecision))
	pos.InitialMargin = pos.InitialMargin.RoundCeil(int32(symbolCfg.Market.QuoteAssetPrecision))
	pos.MaintMargin = pos.MaintMargin.RoundCeil(int32(symbolCfg.Market.QuoteAssetPrecision))
	pos.UnRealizedProfit = pos.UnRealizedProfit.RoundCeil(int32(symbolCfg.Market.QuoteAssetPrecision))
	return nil
}

func (s *Service) getWalletAsset(ctx context.Context, acct types.Account, marketType types.MarketType, quote string) (types.Asset, error) {
	assets, err := entity.Account.GetAssets(ctx, acct.ID)
	if err != nil {
		return types.Asset{}, err
	}

	walletType := types.GetWalletType(acct.Exchange, marketType)

	var result types.Asset
	for _, asset := range assets {
		if asset == nil {
			continue
		}
		if !strings.EqualFold(asset.Code, quote) || asset.WalletType != walletType {
			continue
		}
		result = types.Asset{
			AccountID:     asset.AccountID,
			WalletType:    asset.WalletType,
			Code:          asset.Code,
			Balance:       asset.Balance,
			Frozened:      asset.Frozened,
			OrderOccupied: asset.OrderOccupied,
			UpdatedTs:     asset.UpdatedTs,
		}
		break
	}
	return result, nil
}

func (s *Service) QueryAccountEventFlow(ctx context.Context, req *types.QueryAccountEventFlowRequest) (*types.QueryAccountEventFlowResponse, error) {
	if req == nil || req.AccountID == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if s.eventFlowRepo == nil {
		return nil, errors.New(errors.Unimplemented, "event flow recording not enabled")
	}

	var startTs *time.Time
	if req.StartTsMs != nil {
		startTs = lo.ToPtr(time.UnixMilli(*req.StartTsMs))
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	streamFilter := ""
	switch req.Stream {
	case types.EventFlowStreamAccountRaw:
		streamFilter = "account_raw"
	case types.EventFlowStreamAccount:
		streamFilter = "account"
	case types.EventFlowStreamAll:
		streamFilter = ""
	}

	filter := eventflow.QueryFilter{
		AccountID: req.AccountID,
		Stream:    streamFilter,
		StartTs:   startTs,
		StartID:   req.StartID,
		Limit:     limit,
	}

	records, err := s.eventFlowRepo.Query(ctx, filter)
	if err != nil {
		return nil, err
	}

	resp := &types.QueryAccountEventFlowResponse{
		Events: make([]*types.EventRecord, 0, len(records)),
	}

	for _, record := range records {
		resp.Events = append(resp.Events, &types.EventRecord{
			ID:          record.ID,
			AccountID:   record.AccountID,
			Exchange:    record.Exchange,
			Stream:      record.Stream,
			Topic:       record.Topic,
			EventKind:   record.EventKind,
			TsMs:        record.Ts.UnixMilli(),
			ReceiveAtMs: record.ReceiveAt.UnixMilli(),
			PublishAtMs: record.PublishAt.UnixMilli(),
			IngestAtMs:  record.IngestAt.UnixMilli(),
			PayloadJSON: record.Payload,
		})
	}

	// 设置 next_start_ts_ms 为最后一条记录的时间 + 1ms
	if len(records) > 0 {
		lastRecord := records[len(records)-1]
		resp.NextID = lastRecord.ID + 1
	} else {
		resp.NextID = 0
	}

	return resp, nil
}

// QueryAccountMetrics 查询账户指标
func (s *Service) QueryAccountMetrics(ctx context.Context, req *types.QueryAccountMetricsRequest) (*types.QueryAccountMetricsResponse, error) {
	if req == nil || req.AccountID == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}

	// 解析时间范围，默认最近 30 天
	now := time.Now()
	startTs := now.AddDate(0, 0, -30)
	endTs := now
	if req.StartTs != nil {
		startTs = time.Unix(*req.StartTs, 0)
	}
	if req.EndTs != nil {
		endTs = time.Unix(*req.EndTs, 0)
	}
	if startTs.After(endTs) {
		startTs, endTs = endTs, startTs
	}

	dimension := 1
	if req.Dimension == types.AccountAPIMetricsDimensionSymbol {
		dimension = 2
	}

	var symbol *string
	if req.Symbol != nil && *req.Symbol != "" {
		symbol = req.Symbol
	}

	input := account.AccountMetricsInput{
		AccountID: req.AccountID,
		Symbol:    symbol,
		StartTs:   startTs,
		EndTs:     endTs,
		Dimension: dimension,
	}

	result, err := entity.Account.QueryAccountMetrics(ctx, input)
	if err != nil {
		return nil, err
	}

	resp := &types.QueryAccountMetricsResponse{
		AccountID:             req.AccountID,
		Dimension:             req.Dimension,
		Cagr:                  result.Cagr,
		Sharpe:                result.Sharpe,
		Sortino:               result.Sortino,
		MaxDrawdown:           result.MaxDrawdown,
		TimeUnderWaterSeconds: result.TimeUnderWaterSeconds,
		Calmar:                result.Calmar,
		WinRate:               result.WinRate,
		ProfitFactor:          result.ProfitFactor,
		RollingSharpe:         result.RollingSharpe,
		AvgSlippageBps:        result.AvgSlippageBps,
		FeeRatio:              result.FeeRatio,
		MaxConsecutiveLoss:    result.MaxConsecutiveLoss,
		StartTs:               result.StartTs,
		EndTs:                 result.EndTs,
	}

	for _, sm := range result.SymbolMetrics {
		resp.Symbols = append(resp.Symbols, &types.AccountSymbolMetrics{
			Symbol:                sm.Symbol,
			Exchange:              sm.Exchange,
			Cagr:                  sm.Cagr,
			Sharpe:                sm.Sharpe,
			Sortino:               sm.Sortino,
			MaxDrawdown:           sm.MaxDrawdown,
			TimeUnderWaterSeconds: sm.TimeUnderWaterSeconds,
			Calmar:                sm.Calmar,
			WinRate:               sm.WinRate,
			ProfitFactor:          sm.ProfitFactor,
			RollingSharpe:         sm.RollingSharpe,
			AvgSlippageBps:        sm.AvgSlippageBps,
			FeeRatio:              sm.FeeRatio,
			MaxConsecutiveLoss:    sm.MaxConsecutiveLoss,
		})
	}

	return resp, nil
}
