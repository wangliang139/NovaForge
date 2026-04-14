package account

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kelseyhightower/envconfig"
	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/NovaForge/server/pkg/converter"
	"github.com/wangliang139/NovaForge/server/pkg/market"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	"github.com/wangliang139/NovaForge/server/pkg/repos/ledgers"
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	"github.com/wangliang139/NovaForge/server/pkg/repos/positions"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/snowflake"
)

var (
	MinTimeUnix = int64(0)
	MaxTimeUnix = int64(4102444800) // 2100-01-01 00:00:00 +0000 UTC
)

type RiskChecker interface {
	CheckAccountRisk(ctx context.Context, acct *types.Account) (*types.RiskCheckResult, error)
	CalculateRiskIndex(ctx context.Context, acct *types.Account, acctState *types.AccountState) decimal.Decimal
}

type Config struct {
	AccountConsumerGroup string `split_words:"true" default:"account.consumer.group"`
	AccountRawMsgTopic   string `split_words:"true" default:"account.raw.msg"`
}

type Entity struct {
	cfg Config
	db  *repos.Entity

	engine *market.Engine

	cache redis.UniversalClient

	writeLocks accountWriteLocker

	ctx        context.Context
	cancelFunc context.CancelFunc

	riskChecker RiskChecker
}

// 编译期断言：*Entity 满足 virtual_sub 包装层所需的子账户读接口（实现位于 connector 包）。
var _ connector.VirtualSubAccountReader = (*Entity)(nil)

func New(db *repos.Entity, engine *market.Engine, cache redis.UniversalClient, riskChecker RiskChecker) *Entity {
	cfg := Config{}
	envconfig.MustProcess("ACCOUNT_ENTITY", &cfg)

	ctx, cancel := context.WithCancel(context.Background())

	return &Entity{
		db:          db,
		cfg:         cfg,
		engine:      engine,
		cache:       cache,
		ctx:         ctx,
		cancelFunc:  cancel,
		riskChecker: riskChecker,
	}
}

func (e *Entity) GetConnector(ctx context.Context, exchange ctypes.Exchange, accountID string) (mdtypes.Connector, error) {
	if !exchange.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid exchange: %s", exchange))
	}
	if len(accountID) == 0 {
		conn, err := connector.GetConnector(exchange, nil)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	acct, err := e.GetAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if acct == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	if acct.Exchange != exchange {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("account exchange mismatch: account=%s request=%s", acct.Exchange, exchange))
	}
	if acct.AccountType == types.AccountTypeVirtual {
		conn, err := connector.GetConnector(exchange, nil)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	if acct.AccountType == types.AccountTypeVirtualSub {
		if acct.ParentAccountID == nil || *acct.ParentAccountID == "" {
			return nil, errors.New(errors.InvalidArgument, "virtual_sub account missing parent_account_id")
		}
		parent, err := e.GetAccount(ctx, *acct.ParentAccountID)
		if err != nil {
			return nil, fmt.Errorf("load parent account: %w", err)
		}
		if parent == nil {
			return nil, errors.New(errors.NotFound, "parent account not found")
		}
		if parent.AccountType != types.AccountTypeReal {
			return nil, errors.New(errors.InvalidArgument, "virtual_sub parent must be a real account")
		}
		if parent.Exchange != exchange {
			return nil, errors.New(errors.InvalidArgument, "virtual_sub exchange must match parent")
		}
		apiAccount := mdtypes.NewSecretApiAccount(acct.ID, parent.Exchange, parent.ApiKey, parent.ApiSecret, parent.Passphrase, string(parent.Algorithm))
		conn, err := connector.GetConnector(exchange, apiAccount)
		if err != nil {
			return nil, err
		}
		return connector.NewVirtualSubConnectorView(conn, e, acct.ID, *acct.ParentAccountID), nil
	}
	apiAccount := mdtypes.NewSecretApiAccount(acct.ID, acct.Exchange, acct.ApiKey, acct.ApiSecret, acct.Passphrase, string(acct.Algorithm))
	conn, err := connector.GetConnector(exchange, apiAccount)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// PublishEvent 发布账户事件到下游
func (e *Entity) PublishEvent(ctx context.Context, msg *ctypes.Message) error {
	if e.engine == nil {
		return nil
	}
	return e.engine.Publish(ctx, msg)
}

// validateAccountParentMultiInvariant 保证 parent_account_id / multi_bot_mode 与 account_type 一致（原 DB CHECK 由业务层承担）。
func validateAccountParentMultiInvariant(t accountrepo.AccountType, parentID *string, multiBot bool) error {
	isSub := t == accountrepo.AccountTypeVirtualSub
	hasParent := parentID != nil && strings.TrimSpace(*parentID) != ""
	if isSub {
		if !hasParent {
			return errors.New(errors.InvalidArgument, "virtual_sub requires parent_account_id")
		}
		if multiBot {
			return errors.New(errors.InvalidArgument, "virtual_sub account must have multi_bot_mode false")
		}
		return nil
	}
	if hasParent {
		return errors.New(errors.InvalidArgument, "parent_account_id is only valid for account_type virtual_sub")
	}
	return nil
}

// validateVirtualSubAccountUpdate 虚拟子账户仅允许改名称（及与库内一致的其它只读字段）。
func validateVirtualSubAccountUpdate(account *accountrepo.Account, input types.UpdateAccountRequest) error {
	if input.MultiBotMode != nil && *input.MultiBotMode {
		return errors.New(errors.InvalidArgument, "virtual_sub account cannot enable multi_bot_mode")
	}
	if ctypes.Exchange(account.Exchange) != input.Exchange {
		return errors.New(errors.InvalidArgument, "virtual_sub account: only name can be modified")
	}
	if ctypes.AuthAlgorithm(account.Algorithm) != input.Algorithm {
		return errors.New(errors.InvalidArgument, "virtual_sub account: only name can be modified")
	}
	if ctypes.AccountStatus(account.Status) != input.Status {
		return errors.New(errors.InvalidArgument, "virtual_sub account: only name can be modified")
	}
	if account.ApiKey != input.ApiKey || account.ApiSecret != input.ApiSecret || account.Passphrase != input.Passphrase {
		return errors.New(errors.InvalidArgument, "virtual_sub account: only name can be modified")
	}
	if !lo.ElementsMatch(account.Tags, input.Tags) {
		return errors.New(errors.InvalidArgument, "virtual_sub account: only name can be modified")
	}
	return nil
}

func (e *Entity) CreateAccount(ctx context.Context, input types.CreateAccountInput) (*types.Account, error) {
	account, err := e.db.AccountRepo.GetByName(ctx, input.Name)
	if err != nil {
		return nil, err
	}
	if account != nil {
		return nil, errors.New(errors.InvalidArgument, "account name already exists")
	}
	algo := accountrepo.Algorithm(input.Algorithm)
	if !algo.Valid() {
		return nil, errors.New(errors.InvalidArgument, "algorithm is invalid")
	}
	ex := accountrepo.Exchange(input.Exchange)
	if !ex.Valid() {
		return nil, errors.New(errors.InvalidArgument, "exchange is invalid")
	}
	id := snowflake.Generate().String()
	accountType := accountrepo.AccountType(input.AccountType)
	if !accountType.Valid() {
		accountType = accountrepo.AccountTypeReal // 默认为真实账户
	}
	multiBot := false
	if input.MultiBotMode != nil {
		multiBot = *input.MultiBotMode
	}
	var parentPtr *string
	useAlgo := algo
	switch accountType {
	case accountrepo.AccountTypeVirtual:
		multiBot = false
		parentPtr = nil
	case accountrepo.AccountTypeVirtualSub:
		multiBot = false
		if input.ParentAccountID == nil || *input.ParentAccountID == "" {
			return nil, errors.New(errors.InvalidArgument, "parent_account_id is required for virtual_sub")
		}
		parentPo, err := e.db.AccountRepo.GetById(ctx, *input.ParentAccountID)
		if err != nil {
			return nil, err
		}
		if parentPo == nil {
			return nil, errors.New(errors.NotFound, "parent account not found")
		}
		if parentPo.AccountType != accountrepo.AccountTypeReal || !parentPo.MultiBotMode {
			return nil, errors.New(errors.InvalidArgument, "virtual_sub parent must be a real account with multi_bot_mode enabled")
		}
		parentPtr = input.ParentAccountID
		useAlgo = parentPo.Algorithm
		if ex != parentPo.Exchange {
			return nil, errors.New(errors.InvalidArgument, "virtual_sub exchange must match parent")
		}
	default:
		parentPtr = nil
	}
	if err := validateAccountParentMultiInvariant(accountType, parentPtr, multiBot); err != nil {
		return nil, err
	}
	po, err := e.db.AccountRepo.Create(ctx, accountrepo.CreateParams{
		ID:              id,
		Exchange:        ex,
		Name:            input.Name,
		Config:          []byte(`{}`),
		ApiKey:          input.ApiKey,
		ApiSecret:       input.ApiSecret,
		Passphrase:      input.Passphrase,
		Algorithm:       useAlgo,
		Tags:            input.Tags,
		Status:          accountrepo.AccountStatusOnline,
		AccountType:     accountType,
		ParentAccountID: parentPtr,
		MultiBotMode:    multiBot,
	}, &id, &input.Name, &ex)
	if err != nil {
		return nil, err
	}
	return converter.AccountRepo2Types(po), nil
}

func (e *Entity) UpdateAccount(ctx context.Context, input types.UpdateAccountRequest) (*types.Account, error) {
	algo := accountrepo.Algorithm(input.Algorithm)
	if !algo.Valid() {
		return nil, errors.New(errors.InvalidArgument, "algorithm is invalid")
	}
	ex := accountrepo.Exchange(input.Exchange)
	if !ex.Valid() {
		return nil, errors.New(errors.InvalidArgument, "exchange is invalid")
	}
	sts := accountrepo.AccountStatus(input.Status)
	if !sts.Valid() {
		return nil, errors.New(errors.InvalidArgument, "status is invalid")
	}

	account, err := e.db.AccountRepo.GetById(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	acct, err := e.db.AccountRepo.GetByName(ctx, input.Name)
	if err != nil {
		return nil, err
	}
	if acct != nil && acct.ID != input.ID {
		return nil, errors.New(errors.InvalidArgument, "account name already exists")
	}

	if account.AccountType == accountrepo.AccountTypeVirtualSub {
		if err := validateVirtualSubAccountUpdate(account, input); err != nil {
			return nil, err
		}
	}

	key := fmt.Sprintf("accountrepo:QueryDefaultAccounts:%s", input.Exchange)
	err = e.db.DCache.Invalidate(ctx, key)
	if err != nil {
		return nil, err
	}

	accountType := accountrepo.AccountType(input.AccountType)
	if !accountType.Valid() {
		accountType = accountrepo.AccountTypeReal // 默认为真实账户
	}
	if account.AccountType != accountType {
		return nil, errors.New(errors.InvalidArgument, "account_type cannot be changed")
	}
	multiBotMode := account.MultiBotMode
	if input.MultiBotMode != nil {
		newVal := *input.MultiBotMode
		if newVal != account.MultiBotMode {
			if account.AccountType == accountrepo.AccountTypeVirtualSub {
				return nil, errors.New(errors.InvalidArgument, "multi_bot_mode cannot be changed for virtual_sub")
			}
			if account.AccountType == accountrepo.AccountTypeVirtual {
				return nil, errors.New(errors.InvalidArgument, "multi_bot_mode cannot be enabled for virtual accounts")
			}
			if account.AccountType == accountrepo.AccountTypeReal {
				if account.MultiBotMode && !newVal {
					subs, err := e.db.AccountRepo.ListVirtualSubByParent(ctx, &input.ID)
					if err != nil {
						return nil, err
					}
					if len(subs) > 0 {
						return nil, errors.New(errors.InvalidArgument,
							"cannot disable multi_bot_mode while virtual sub-accounts exist; delete related bots and sub-accounts first")
					}
				} else if !account.MultiBotMode && newVal {
					cnt, err := e.db.BotRepo.CountActiveBotsForParentAccount(ctx, input.ID)
					if err != nil {
						return nil, err
					}
					if cnt != nil && *cnt > 0 {
						return nil, errors.New(errors.InvalidArgument,
							"cannot enable multi_bot_mode while bots are bound to this account; remove or migrate bots first")
					}
				}
			}
		}
		multiBotMode = newVal
	}
	if account.AccountType == accountrepo.AccountTypeVirtualSub {
		multiBotMode = false
	}
	if account.AccountType == accountrepo.AccountTypeVirtual {
		multiBotMode = false
	}
	if err := validateAccountParentMultiInvariant(account.AccountType, account.ParentAccountID, multiBotMode); err != nil {
		return nil, err
	}
	po, err := e.db.AccountRepo.Update(ctx, accountrepo.UpdateParams{
		ID:           input.ID,
		Exchange:     ex,
		Name:         input.Name,
		ApiKey:       input.ApiKey,
		ApiSecret:    input.ApiSecret,
		Passphrase:   input.Passphrase,
		Algorithm:    algo,
		Tags:         input.Tags,
		Status:       sts,
		AccountType:  accountType,
		MultiBotMode: multiBotMode,
	}, &input.ID, &input.Name, &ex)
	if err != nil {
		return nil, err
	}
	return converter.AccountRepo2Types(po), nil
}

func (e *Entity) UpdateAccountStatus(ctx context.Context, id string, status types.AccountStatus) (*types.Account, error) {
	if len(id) == 0 {
		return nil, errors.New(errors.InvalidArgument, "id is invalid")
	}
	if !status.Valid() {
		return nil, errors.New(errors.InvalidArgument, "status is invalid")
	}

	account, err := e.db.AccountRepo.GetById(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}

	key := fmt.Sprintf("accountrepo:QueryDefaultAccounts:%s", account.Exchange)
	if err = e.db.DCache.Invalidate(ctx, key); err != nil {
		return nil, err
	}

	algo := accountrepo.Algorithm(account.Algorithm)
	if !algo.Valid() {
		return nil, errors.New(errors.InvalidArgument, "algorithm is invalid")
	}
	ex := accountrepo.Exchange(account.Exchange)
	if !ex.Valid() {
		return nil, errors.New(errors.InvalidArgument, "exchange is invalid")
	}
	sts := accountrepo.AccountStatus(status)
	if !sts.Valid() {
		return nil, errors.New(errors.InvalidArgument, "status is invalid")
	}

	accountType := accountrepo.AccountType(account.AccountType)
	if !accountType.Valid() {
		accountType = accountrepo.AccountTypeReal
	}

	po, err := e.db.AccountRepo.Update(ctx, accountrepo.UpdateParams{
		ID:          account.ID,
		Exchange:    ex,
		Name:        account.Name,
		ApiKey:      account.ApiKey,
		ApiSecret:   account.ApiSecret,
		Passphrase:  account.Passphrase,
		Algorithm:   algo,
		Tags:        account.Tags,
		Status:      sts,
		AccountType: accountType,
	}, &account.ID, &account.Name, &ex)
	if err != nil {
		return nil, err
	}
	return converter.AccountRepo2Types(po), nil
}

// DeleteAccount performs logical delete for an account by id.
func (e *Entity) DeleteAccount(ctx context.Context, id string) error {
	if len(id) == 0 {
		return errors.New(errors.InvalidArgument, "id is invalid")
	}

	// invalidate default accounts cache for all exchanges (simple strategy)
	for _, ex := range []accountrepo.Exchange{
		accountrepo.ExchangeBinance,
		accountrepo.ExchangeOkx,
		accountrepo.ExchangeBinanceTest,
		accountrepo.ExchangeOkxTest,
	} {
		key := fmt.Sprintf("accountrepo:QueryDefaultAccounts:%s", ex)
		if err := e.db.DCache.Invalidate(ctx, key); err != nil {
			return err
		}
	}

	count, err := e.db.AccountRepo.DeleteAccount(ctx, id)
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New(errors.NotFound, "account not found")
	}
	return nil
}

func (e *Entity) QueryAccountsCount(ctx context.Context, id *string, exchange *ctypes.Exchange, name *string, tags []string, status *types.AccountStatus, accountType *types.AccountType, createdAtStart, createdAtEnd *int64) (int64, error) {
	var (
		createdAtStartTime time.Time
		createdAtEndTime   time.Time
	)
	if createdAtStart == nil {
		createdAtStart = &MinTimeUnix
	}
	if createdAtEnd == nil {
		createdAtEnd = &MaxTimeUnix
	}
	createdAtStartTime = time.Unix(*createdAtStart, 0)
	createdAtEndTime = time.Unix(*createdAtEnd, 0)
	if createdAtEndTime.Before(createdAtStartTime) {
		return 0, errors.New(errors.InvalidArgument, "illegal createdAt time range")
	}

	sts := accountrepo.NullAccountStatus{}
	if status != nil {
		sts.AccountStatus = accountrepo.AccountStatus(*status)
		sts.Valid = true
	}

	ex := accountrepo.NullExchange{}
	if exchange != nil {
		ex.Exchange = accountrepo.Exchange(*exchange)
		ex.Valid = true
	}

	at := accountrepo.NullAccountType{}
	if accountType != nil {
		at.AccountType = accountrepo.AccountType(*accountType)
		at.Valid = true
	}

	count, err := e.db.AccountRepo.QueryAccountsCount(ctx, accountrepo.QueryAccountsCountParams{
		ID:             id,
		Exchange:       ex,
		Name:           name,
		Tags:           tags,
		Status:         sts,
		AccountType:    at,
		CreatedAtStart: createdAtStartTime,
		CreatedAtEnd:   createdAtEndTime,
	})
	if err != nil {
		return 0, err
	}
	return *count, nil
}

func (e *Entity) QueryAccounts(ctx context.Context, id *string, exchange *ctypes.Exchange, name *string, tags []string, status *types.AccountStatus, accountType *types.AccountType, createdAtStart, createdAtEnd *int64, offset, limit int64) ([]*types.Account, error) {
	var (
		createdAtStartTime time.Time
		createdAtEndTime   time.Time
	)
	if createdAtStart == nil {
		createdAtStart = &MinTimeUnix
	}
	if createdAtEnd == nil {
		createdAtEnd = &MaxTimeUnix
	}
	createdAtStartTime = time.Unix(*createdAtStart, 0)
	createdAtEndTime = time.Unix(*createdAtEnd, 0)
	if createdAtEndTime.Before(createdAtStartTime) {
		return nil, errors.New(errors.InvalidArgument, "illegal createdAt time range")
	}

	sts := accountrepo.NullAccountStatus{}
	if status != nil {
		sts.AccountStatus = accountrepo.AccountStatus(*status)
		sts.Valid = true
	}

	ex := accountrepo.NullExchange{}
	if exchange != nil {
		ex.Exchange = accountrepo.Exchange(*exchange)
		ex.Valid = true
	}

	at := accountrepo.NullAccountType{}
	if accountType != nil {
		at.AccountType = accountrepo.AccountType(*accountType)
		at.Valid = true
	}

	list, err := e.db.AccountRepo.QueryAccounts(ctx, accountrepo.QueryAccountsParams{
		ID:             id,
		Exchange:       ex,
		Name:           name,
		Tags:           tags,
		Status:         sts,
		AccountType:    at,
		CreatedAtStart: createdAtStartTime,
		CreatedAtEnd:   createdAtEndTime,
		Offset:         offset,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*types.Account, len(list))
	for i := range list {
		result[i] = converter.AccountRepo2Types(&list[i])
	}
	return result, nil
}

func (e *Entity) QueryDefaultAccounts(ctx context.Context, exchange ctypes.Exchange, limit int64) ([]*types.Account, error) {
	accounts, err := e.db.AccountRepo.GetDefaultAccounts(ctx, accountrepo.Exchange(exchange))
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	if len(accounts) > int(limit) {
		accounts = accounts[:limit]
	}
	result := make([]*types.Account, len(accounts))
	for i := range accounts {
		result[i] = converter.AccountRepo2Types(&accounts[i])
	}
	return result, nil
}

func (e *Entity) QueryAnyDefaultAccount(ctx context.Context, exchange ctypes.Exchange) (*types.Account, error) {
	accounts, err := e.QueryDefaultAccounts(ctx, exchange, 1)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	return accounts[0], nil
}

// UpdateAccountRiskConfig 更新账户的 config.risk 并返回最新账户
func (e *Entity) UpdateAccountRiskConfig(ctx context.Context, accountID string, riskCfg *types.RiskConfig) (*types.Account, error) {
	if len(accountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is invalid")
	}

	riskJSON, err := types.RiskConfigToJSON(riskCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal risk config: %w", err)
	}

	po, err := e.db.AccountRepo.UpdateRiskConfig(ctx, accountrepo.UpdateRiskConfigParams{
		Risk: riskJSON,
		ID:   accountID,
	}, &accountID, nil, nil)
	if err != nil {
		return nil, err
	}
	if po == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	return converter.AccountRepo2Types(po), nil
}

func (e *Entity) GetAccount(ctx context.Context, id string) (*types.Account, error) {
	po, err := e.db.AccountRepo.GetById(ctx, id)
	if err != nil {
		return nil, err
	}
	if po == nil {
		return nil, nil
	}
	return converter.AccountRepo2Types(po), nil
}

func (e *Entity) ListAccounts(ctx context.Context, status types.AccountStatus) ([]*types.Account, error) {
	accounts, err := e.db.AccountRepo.ListAccounts(ctx, accountrepo.AccountStatus(status))
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	result := make([]*types.Account, len(accounts))
	for i := range accounts {
		result[i] = converter.AccountRepo2Types(&accounts[i])
	}
	return result, nil
}

// GetPositions 查询账户持仓快照
func (e *Entity) GetPositions(ctx context.Context, accountID string) ([]*ctypes.Position, error) {
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "accountID is required")
	}
	list, err := e.db.PositionsRepo.ListPositionsByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	result := make([]*ctypes.Position, 0, len(list))
	for _, item := range list {
		qty := utils.Decimal.PgNumericToDecimal(item.Qty)
		if qty.IsZero() {
			continue
		}
		symbol, err := ctypes.ParseSymbol(item.Symbol)
		if err != nil {
			return nil, err
		}
		side := ctypes.PositionSideLong
		if item.Side == positions.PositionSideSHORT {
			side = ctypes.PositionSideShort
		}
		result = append(result, &ctypes.Position{
			AccountID:  item.AccountID,
			Exchange:   ctypes.Exchange(item.Exchange),
			Symbol:     symbol,
			Side:       side,
			Amount:     utils.Decimal.PgNumericToDecimal(item.Qty),
			EntryPrice: utils.Decimal.PgNumericToDecimal(item.EntryPrice),
			Leverage:   int(item.Leverage),
			UpdatedTs:  item.UpdatedAt,
		})
	}
	return result, nil
}

func (e *Entity) GetOpenOrders(ctx context.Context, accountID string, symbol *ctypes.Symbol) ([]*ctypes.Order, error) {
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "accountID is required")
	}

	var symbolStr *string
	if symbol != nil {
		symbolStr = lo.ToPtr(symbol.String())
	}

	list, err := e.db.OrdersRepo.GetPendingOrders(ctx, orders.GetPendingOrdersParams{
		AccountID: accountID,
		Symbol:    symbolStr,
	})
	if err != nil {
		return nil, err
	}

	result := make([]*ctypes.Order, 0, len(list))
	for _, item := range list {
		order, err := converter.OrderDb2Types(item)
		if err != nil {
			return nil, err
		}
		result = append(result, order)
	}
	return result, nil
}

// QueryOrders 查询账户订单快照
func (e *Entity) QueryOrders(ctx context.Context, accountID string, limit, offset int32) ([]*ctypes.Order, error) {
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "accountID is required")
	}
	if limit <= 0 {
		limit = 100
	}
	list, err := e.db.OrdersRepo.ListOrders(ctx, orders.ListOrdersParams{
		AccountID: accountID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*ctypes.Order, 0, len(list))
	for _, item := range list {
		order, err := converter.OrderDb2Types(item)
		if err != nil {
			return nil, err
		}
		result = append(result, order)
	}
	return result, nil
}

func (e *Entity) GetOrder(ctx context.Context, accountID string, symbol string, clientOrderID, exchangeOrderID string) (*ctypes.Order, error) {
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "accountID is required")
	}
	if len(clientOrderID) == 0 && len(exchangeOrderID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "client_order_id or exchange_order_id is required")
	}

	var (
		po  *orders.Order
		err error
	)
	if len(clientOrderID) > 0 {
		po, err = e.db.OrdersRepo.GetOrderByClientOrderId(ctx, orders.GetOrderByClientOrderIdParams{AccountID: accountID, ClientOrderID: clientOrderID})
	}
	if len(exchangeOrderID) > 0 {
		po, err = e.db.OrdersRepo.GetOrderByOrderId(ctx, orders.GetOrderByOrderIdParams{AccountID: accountID, OrderID: exchangeOrderID})
	}
	if err != nil {
		return nil, err
	}
	if po == nil {
		return nil, errors.New(errors.NotFound, "order not found")
	}
	return converter.OrderDb2Types(*po)
}

type OrderCursorFilter struct {
	Symbol          *ctypes.Symbol
	OrderType       *ctypes.OrderType
	OrderSource     *ctypes.OrderSource
	Statuses        []ctypes.OrderStatus
	CursorCreatedTs *time.Time
	CursorID        *string
	Limit           int32
	BotID           *int64
}

type OrderPageFilter struct {
	Symbol      *ctypes.Symbol
	OrderType   *ctypes.OrderType
	OrderSource *ctypes.OrderSource
	Statuses    []ctypes.OrderStatus
	Page        int32
	Size        int32
	BotID       *int64
	StartTs     *time.Time
	EndTs       *time.Time
	// FinishedStartTs/FinishedEndTs 同时非空时，按 finished_ts 分页查询（与 StartTs/EndTs 互斥，优先本字段）
	FinishedStartTs *time.Time
	FinishedEndTs   *time.Time
}

// QueryOrdersByCursor 查询账户订单（游标翻页）
func (e *Entity) QueryOrdersByCursor(ctx context.Context, accountID string, filter OrderCursorFilter) ([]*ctypes.Order, error) {
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "accountID is required")
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	params := orders.ListOrdersByCursorParams{
		AccountID: accountID,
		Limit:     filter.Limit,
	}
	if filter.Symbol != nil {
		params.Symbol = lo.ToPtr(filter.Symbol.String())
	}
	if filter.OrderType != nil && filter.OrderType.Valid() {
		ot := strings.ToUpper(string(*filter.OrderType))
		params.OrderType = orders.NullOrderType{OrderType: orders.OrderType(ot), Valid: true}
	}
	if filter.OrderSource != nil && filter.OrderSource.Valid() {
		params.OrderSource = orders.NullOrderSource{OrderSource: orders.OrderSource(*filter.OrderSource), Valid: true}
	}
	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			statuses = append(statuses, string(status))
		}
		params.Statuses = statuses
	}
	if filter.CursorCreatedTs != nil {
		params.CursorCreatedTs = filter.CursorCreatedTs
	}
	if filter.CursorID != nil {
		id, err := strconv.ParseInt(*filter.CursorID, 10, 64)
		if err != nil {
			return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid cursor id: %s", *filter.CursorID))
		}
		params.CursorID = lo.ToPtr(id)
	}
	if filter.BotID != nil {
		bid := int32(*filter.BotID)
		params.BotID = &bid
	}
	list, err := e.db.OrdersRepo.ListOrdersByCursor(ctx, params)
	if err != nil {
		return nil, err
	}
	result := make([]*ctypes.Order, 0, len(list))
	for _, item := range list {
		order, err := converter.OrderDb2Types(item)
		if err != nil {
			return nil, err
		}
		result = append(result, order)
	}
	return result, nil
}

// QueryOrdersByPage 查询账户订单（分页）
func (e *Entity) QueryOrdersByPage(ctx context.Context, accountID string, filter OrderPageFilter) ([]*ctypes.Order, int64, error) {
	if accountID == "" {
		return nil, 0, errors.New(errors.InvalidArgument, "accountID is required")
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Size <= 0 {
		filter.Size = 100
	}
	if filter.Size > 200 {
		filter.Size = 200
	}
	offset := (filter.Page - 1) * filter.Size

	params := orders.ListOrdersByPageParams{
		AccountID: accountID,
		Limit:     filter.Size,
		Offset:    offset,
	}
	countParams := orders.CountOrdersByFilterParams{
		AccountID: accountID,
	}
	if filter.Symbol != nil {
		params.Symbol = lo.ToPtr(filter.Symbol.String())
		countParams.Symbol = params.Symbol
	}
	if filter.OrderType != nil && filter.OrderType.Valid() {
		ot := strings.ToUpper(string(*filter.OrderType))
		params.OrderType = orders.NullOrderType{OrderType: orders.OrderType(ot), Valid: true}
		countParams.OrderType = params.OrderType
	}
	if filter.OrderSource != nil && filter.OrderSource.Valid() {
		params.OrderSource = orders.NullOrderSource{OrderSource: orders.OrderSource(*filter.OrderSource), Valid: true}
		countParams.OrderSource = params.OrderSource
	}
	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			statuses = append(statuses, string(status))
		}
		params.Statuses = statuses
		countParams.Statuses = statuses
	}
	if filter.BotID != nil {
		bid := int32(*filter.BotID)
		params.BotID = &bid
		countParams.BotID = &bid
	}
	useFinishedRange := filter.FinishedStartTs != nil && filter.FinishedEndTs != nil
	if useFinishedRange {
		finList := orders.ListOrdersByPageByFinishedTsParams{
			AccountID:       accountID,
			Limit:           filter.Size,
			Offset:          offset,
			FinishedStartTs: filter.FinishedStartTs,
			FinishedEndTs:   filter.FinishedEndTs,
		}
		finCount := orders.CountOrdersByFinishedTsRangeParams{
			AccountID:       accountID,
			FinishedStartTs: filter.FinishedStartTs,
			FinishedEndTs:   filter.FinishedEndTs,
		}
		if filter.Symbol != nil {
			finList.Symbol = lo.ToPtr(filter.Symbol.String())
			finCount.Symbol = finList.Symbol
		}
		if filter.OrderType != nil && filter.OrderType.Valid() {
			ot := strings.ToUpper(string(*filter.OrderType))
			finList.OrderType = orders.NullOrderType{OrderType: orders.OrderType(ot), Valid: true}
			finCount.OrderType = finList.OrderType
		}
		if filter.OrderSource != nil && filter.OrderSource.Valid() {
			finList.OrderSource = orders.NullOrderSource{OrderSource: orders.OrderSource(*filter.OrderSource), Valid: true}
			finCount.OrderSource = finList.OrderSource
		}
		if len(filter.Statuses) > 0 {
			statuses := make([]string, 0, len(filter.Statuses))
			for _, status := range filter.Statuses {
				statuses = append(statuses, string(status))
			}
			finList.Statuses = statuses
			finCount.Statuses = statuses
		}
		if filter.BotID != nil {
			bid := int32(*filter.BotID)
			finList.BotID = &bid
			finCount.BotID = &bid
		}
		totalCount, err := e.db.OrdersRepo.CountOrdersByFinishedTsRange(ctx, finCount)
		if err != nil {
			return nil, 0, err
		}
		list, err := e.db.OrdersRepo.ListOrdersByPageByFinishedTs(ctx, finList)
		if err != nil {
			return nil, 0, err
		}
		result := make([]*ctypes.Order, 0, len(list))
		for _, item := range list {
			order, err := converter.OrderDb2Types(item)
			if err != nil {
				return nil, 0, err
			}
			result = append(result, order)
		}
		if totalCount == nil {
			return result, 0, nil
		}
		return result, *totalCount, nil
	}

	if filter.StartTs != nil {
		params.StartTs = filter.StartTs
		countParams.StartTs = filter.StartTs
	}
	if filter.EndTs != nil {
		params.EndTs = filter.EndTs
		countParams.EndTs = filter.EndTs
	}

	totalCount, err := e.db.OrdersRepo.CountOrdersByFilter(ctx, countParams)
	if err != nil {
		return nil, 0, err
	}
	list, err := e.db.OrdersRepo.ListOrdersByPage(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	result := make([]*ctypes.Order, 0, len(list))
	for _, item := range list {
		order, err := converter.OrderDb2Types(item)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, order)
	}
	return result, *totalCount, nil
}

func (e *Entity) CountLedgers(ctx context.Context, accountID string, startTs, endTs time.Time) (int64, error) {
	if accountID == "" {
		return 0, errors.New(errors.InvalidArgument, "accountID is required")
	}
	count, err := e.db.LedgersRepo.CountLedgers(ctx, ledgers.CountLedgersParams{
		AccountID: accountID,
		Ts:        startTs,
		Ts_2:      endTs,
	})
	if err != nil {
		return 0, err
	}
	return *count, nil
}

// QueryLedgers 查询账户资金流水（简单按账号过滤，时间条件可由调用方在上层实现）
func (e *Entity) QueryLedgers(ctx context.Context, accountID string, startTs, endTs time.Time, offset, limit int32) ([]*ctypes.Ledger, int64, error) {
	if accountID == "" {
		return nil, 0, errors.New(errors.InvalidArgument, "accountID is required")
	}

	totalCount, err := e.CountLedgers(ctx, accountID, startTs, endTs)
	if err != nil {
		return nil, 0, err
	}

	if totalCount == 0 {
		return nil, 0, nil
	}

	if totalCount < int64(offset) {
		return nil, 0, nil
	}

	ledgers, err := e.db.LedgersRepo.ListLedgers(ctx, ledgers.ListLedgersParams{
		AccountID: accountID,
		Ts:        startTs,
		Ts_2:      endTs,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, 0, err
	}
	result := make([]*ctypes.Ledger, 0, len(ledgers))
	for _, item := range ledgers {
		result = append(result, &ctypes.Ledger{
			ID:          item.ID,
			AccountID:   item.AccountID,
			Exchange:    ctypes.Exchange(item.Exchange),
			Asset:       item.Asset,
			WalletType:  ctypes.WalletType(item.WalletType),
			TotalDelta:  utils.Decimal.PgNumericToDecimal(item.TotalDelta),
			FrozenDelta: utils.Decimal.PgNumericToDecimal(item.FrozenDelta),
			Total:       utils.Decimal.PgNumericToDecimal(item.Total),
			Frozen:      utils.Decimal.PgNumericToDecimal(item.Frozen),
			Type:        ctypes.LedgerReason(item.Type),
			Detail:      item.Detail,
			IsEffective: item.IsEffective,
			Ts:          item.Ts,
			CreatedAt:   item.CreatedAt,
		})
	}
	return result, totalCount, nil
}

func (e *Entity) ApplyAccountPositions(ctx context.Context, accountID string, exchange ctypes.Exchange, posList []*ctypes.Position, closeOthers bool) error {
	keyFn := func(symbolStr string, side positions.PositionSide) string {
		return symbolStr + ":" + string(side)
	}

	exchangeStr := exchange.String()

	// closeOthers=true 时，仅关闭同一 exchange 下未出现在本次快照中的仓位
	closedPositions := make(map[string]positions.Position, 0)
	if closeOthers {
		existing, err := e.db.PositionsRepo.ListPositionsByAccountAndExchange(ctx, positions.ListPositionsByAccountAndExchangeParams{
			AccountID: accountID,
			Exchange:  exchangeStr,
		})
		if err != nil {
			return fmt.Errorf("list positions: %w", err)
		}
		for _, p := range existing {
			qty := utils.Decimal.PgNumericToDecimal(p.Qty)
			if qty.IsZero() {
				continue
			}
			closedPositions[keyFn(p.Symbol, p.Side)] = p
		}
	}

	maxTs := time.Time{}
	positionsChanged := false

	applyOne := func(pos *ctypes.Position) error {
		if pos == nil {
			return nil
		}
		pos.AccountID = accountID
		pos.Exchange = exchange

		row, err := e.applyPositionSnapshotRow(ctx, accountID, exchange, pos)
		if err != nil {
			return err
		}
		// log.Info().Interface("pos_leverage", pos.Leverage).Interface("prev_leverage", row.PrevLeverage).Interface("leverage", row.Leverage).Interface("row", row).Interface("pos", pos).Msg("applyPositionSnapshotRow")
		if row == nil {
			return nil
		}
		changed := positionUpsertMeaningfulChange(row)
		if changed {
			positionsChanged = true
			e.recordPositionSnapshotFromUpsertRow(ctx, row)
		}
		if !row.UpdatedTs.IsZero() && (maxTs.IsZero() || row.UpdatedTs.After(maxTs)) {
			maxTs = row.UpdatedTs
		}

		// OKX 没有单独的杠杆更新事件，需要通过仓位更新事件来处理
		if pos.Leverage != 0 && row.PrevLeverage != nil && *row.PrevLeverage != int32(pos.Leverage) && exchange.Base() == ctypes.ExchangeOkx {
			go func() {
				ctx := context.WithoutCancel(ctx)
				selector := ctypes.StreamSelector{
					Stream:  ctypes.StreamTypeAccount,
					Account: lo.ToPtr(accountID),
				}
				msg := ctypes.NewMessage(exchange, selector, &ctypes.SymbolLeverage{
					Exchange:  exchange,
					Symbol:    pos.Symbol,
					Side:      pos.Side,
					Leverage:  pos.Leverage,
					UpdatedTs: pos.UpdatedTs,
				}, pos.UpdatedTs)
				if err := e.engine.Publish(ctx, msg); err != nil {
					logger.Ctx(ctx).Err(err).Str("account_id", accountID).
						Str("exchange", exchangeStr).
						Str("symbol", pos.Symbol.String()).
						Int("prev_leverage", int(*row.PrevLeverage)).
						Int("new_leverage", pos.Leverage).
						Msg("failed to publish symbol leverage update")
				}
			}()
		}
		return nil
	}

	// 先应用传入的 positions
	for _, pos := range posList {
		if pos == nil {
			continue
		}
		side := positions.PositionSideLONG
		if pos.Side == ctypes.PositionSideShort {
			side = positions.PositionSideSHORT
		}
		delete(closedPositions, keyFn(pos.Symbol.String(), side))
		if err := applyOne(pos); err != nil {
			return fmt.Errorf("apply position snapshot: %w", err)
		}
	}

	// 再关闭其他未包含的仓位
	if closeOthers {
		now := time.Now()
		for _, p := range closedPositions {
			symbol, err := ctypes.ParseSymbol(p.Symbol)
			if err != nil {
				return fmt.Errorf("parse symbol: %w", err)
			}
			side := ctypes.PositionSideLong
			if p.Side == positions.PositionSideSHORT {
				side = ctypes.PositionSideShort
			}
			closePos := &ctypes.Position{
				AccountID:  accountID,
				Exchange:   exchange,
				Symbol:     symbol,
				Side:       side,
				Amount:     decimal.Zero,
				EntryPrice: decimal.Zero,
				Leverage:   int(p.Leverage),
				UpdatedTs:  now,
			}
			if err := applyOne(closePos); err != nil {
				return fmt.Errorf("close position snapshot: %w", err)
			}
		}
	}

	// 发布仓位事件（供下游订阅）：snapshot -> PositionSnapshot；update -> PositionsUpdate
	if positionsChanged {
		ts := maxTs
		if ts.IsZero() {
			ts = time.Now()
		}
		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccount,
			Account: lo.ToPtr(accountID),
		}
		nonNil := make([]*ctypes.Position, 0, len(posList))
		for _, p := range posList {
			if p != nil {
				nonNil = append(nonNil, p)
			}
		}
		var payload any
		if closeOthers {
			payload = &ctypes.PositionSnapshot{Positions: nonNil}
		} else {
			payload = &ctypes.PositionsUpdate{Type: ctypes.UpdateTypeSnapshot, Positions: nonNil}
		}
		msg := ctypes.NewMessage(exchange, selector, payload, ts)
		if err := e.engine.Publish(ctx, msg); err != nil {
			return err
		}
	}

	return nil
}

func (e *Entity) applyPositionSnapshotRow(ctx context.Context, accountID string, exchange ctypes.Exchange, pos *ctypes.Position) (*positions.UpsertPositionRow, error) {
	if pos == nil {
		return nil, nil
	}
	side := positions.PositionSideLONG
	if pos.Side == ctypes.PositionSideShort {
		side = positions.PositionSideSHORT
	}

	_qty := pos.Amount.String()
	_ = _qty

	qtyNum := utils.Decimal.DecimalToPgNumeric(pos.Amount)
	entryPriceNum := utils.Decimal.DecimalToPgNumeric(pos.EntryPrice)
	leverage := int32(pos.Leverage)

	if leverage == 0 && pos.Amount.GreaterThan(decimal.Zero) {
		conn, err := e.GetConnector(ctx, exchange, accountID)
		if err != nil {
			return nil, fmt.Errorf("get connector: %w", err)
		}
		symbolConfig, err := conn.SymbolConfig(ctx, pos.Symbol)
		if err != nil {
			return nil, fmt.Errorf("get leverage: %w", err)
		}
		if symbolConfig == nil {
			return nil, fmt.Errorf("symbol config not found")
		}
		leverage = int32(symbolConfig.CrossLeverage[0])
		pos.Leverage = int(leverage)
	}

	params := positions.UpsertPositionParams{
		AccountID:  accountID,
		Exchange:   exchange.String(),
		Symbol:     pos.Symbol.String(),
		Side:       side,
		Qty:        qtyNum,
		Leverage:   leverage,
		EntryPrice: entryPriceNum,
		UpdatedTs:  pos.UpdatedTs,
	}
	row, err := e.db.PositionsRepo.UpsertPosition(ctx, params)
	return row, err
}

// AppendLedger 写入资金变更流水
func (e *Entity) AppendLedger(ctx context.Context, params ledgers.CreateLedgerEntryParams) error {
	if params.AccountID == "" || params.Exchange == "" || params.Asset == "" || params.WalletType == "" {
		return errors.New(errors.InvalidArgument, "accountID, exchange, asset and walletType are required")
	}
	_, err := e.db.LedgersRepo.CreateLedgerEntry(ctx, params)
	return err
}

func (e *Entity) GetSnapshot(ctx context.Context, accountID string) (*types.AccountState, error) {
	acct, err := e.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	assets, err := e.GetAssets(ctx, accountID)
	if err != nil {
		return nil, err
	}
	positions, err := e.GetPositions(ctx, accountID)
	if err != nil {
		return nil, err
	}
	orders, err := e.GetOpenOrders(ctx, accountID, nil)
	if err != nil {
		return nil, err
	}

	state := &types.AccountState{
		Assets:    make([]*ctypes.AssetBo, 0, len(assets)),
		Positions: positions,
		Orders:    orders,
	}
	state.Equity, _, err = e.calculateAccountEquity(ctx, acct.Exchange, assets, positions)
	if err != nil {
		return nil, err
	}
	state.DailyPnL, err = e.CalcDailyPnl(ctx, accountID)
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		state.Assets = append(state.Assets, &ctypes.AssetBo{
			AccountID:  asset.AccountID,
			WalletType: asset.WalletType,
			Code:       asset.Code,
			Balance:    asset.Balance,
			Locked:     asset.Locked(),
			UpdatedTs:  asset.UpdatedTs,
		})
	}

	return state, nil
}

// CalcDailyPnl 根据订单记录计算每日盈亏，仅计算 DONE/PARTIAL_DONE 的订单，后续可改为通过redis滚动窗口计算
func (e *Entity) CalcDailyPnl(ctx context.Context, accountID string) (decimal.Decimal, error) {
	orders, err := e.db.OrdersRepo.ListOrdersByAccountAndTimeRange(ctx, orders.ListOrdersByAccountAndTimeRangeParams{
		AccountID:   accountID,
		CreatedTs:   time.Now().AddDate(0, 0, -1),
		CreatedTs_2: time.Now(),
		Limit:       1000,
	})
	if err != nil {
		return decimal.Zero, err
	}
	totalPnl := decimal.Zero
	for _, order := range orders {
		status := ctypes.OrderStatus(order.Status)
		if !status.IsFinished() {
			continue
		}
		if !order.RealizedPnl.Valid || order.PnlAsset == nil || *order.PnlAsset == "" {
			continue
		}
		realizedPnl := utils.Decimal.PgNumericToDecimal(order.RealizedPnl)
		if realizedPnl.IsZero() {
			continue
		}
		if strings.ToUpper(*order.PnlAsset) == "USDT" {
			totalPnl = totalPnl.Add(realizedPnl)
			continue
		}
		exchange, err := ctypes.ParseExchange(order.Exchange)
		if err != nil {
			return decimal.Zero, err
		}
		pnlSymbol := ctypes.NewSymbol(*order.PnlAsset, "USDT", ctypes.MarketTypeSpot)
		price, err := e.engine.GetMarketProvider().GetLastPrice(ctx, exchange, pnlSymbol)
		if err != nil {
			return decimal.Zero, err
		}
		totalPnl = totalPnl.Add(realizedPnl.Mul(price))
	}
	return totalPnl, nil
}

func (e *Entity) GetRiskIndex(ctx context.Context, accountID string) (decimal.Decimal, error) {
	// 先尝试从缓存中读取
	if e.cache != nil {
		cacheKey := fmt.Sprintf("account:risk_index:%s", accountID)
		if valStr, err := e.cache.Get(ctx, cacheKey).Result(); err == nil && valStr != "" {
			if val, err := decimal.NewFromString(valStr); err == nil {
				return val, nil
			}
		}
	}

	acct, err := e.GetAccount(ctx, accountID)
	if err != nil {
		return decimal.Zero, err
	}
	if acct == nil {
		return decimal.Zero, errors.New(errors.NotFound, "account not found")
	}

	acctState, err := e.GetSnapshot(ctx, accountID)
	if err != nil {
		return decimal.Zero, err
	}

	riskIndex := e.riskChecker.CalculateRiskIndex(ctx, acct, acctState)

	// 缓存 3 分钟，忽略缓存错误
	if e.cache != nil {
		cacheKey := fmt.Sprintf("account:risk_index:%s", accountID)
		_ = e.cache.Set(ctx, cacheKey, riskIndex.String(), 3*time.Minute).Err()
	}

	return riskIndex, nil
}

func (e *Entity) UpdatePositionLeverage(ctx context.Context, accountID string, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return errors.New(errors.InvalidArgument, "account_id is required")
	}
	return e.WithSortedAccountWrites(ctx, []string{accountID}, func(ctx context.Context) error {
		return e.updatePositionLeverageUnlocked(ctx, accountID, exchange, symbol, leverage)
	})
}

func (e *Entity) updatePositionLeverageUnlocked(ctx context.Context, accountID string, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error {
	now := time.Now()
	_, err := e.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		_, err := e.db.PositionsRepo.WithTx(tx).UpsertSymbolLeverage(ctx, positions.UpsertSymbolLeverageParams{
			AccountID: accountID,
			Exchange:  exchange.String(),
			Symbol:    symbol.String(),
			Side:      positions.PositionSideLONG,
			Leverage:  int32(leverage),
			UpdatedTs: now,
		})
		if err != nil {
			return nil, err
		}

		_, err = e.db.PositionsRepo.WithTx(tx).UpsertSymbolLeverage(ctx, positions.UpsertSymbolLeverageParams{
			AccountID: accountID,
			Exchange:  exchange.String(),
			Symbol:    symbol.String(),
			Side:      positions.PositionSideSHORT,
			Leverage:  int32(leverage),
			UpdatedTs: now,
		})
		if err != nil {
			return nil, err
		}

		// OKX 没有单独的杠杆更新事件，需要通过仓位更新事件来处理
		if exchange.Base() == ctypes.ExchangeOkx {
			// 发送杠杆变更事件
			selector := ctypes.StreamSelector{
				Stream:  ctypes.StreamTypeAccount,
				Account: lo.ToPtr(accountID),
			}
			msg := ctypes.NewMessage(exchange, selector, &ctypes.SymbolLeverage{
				Exchange:  exchange,
				Symbol:    symbol,
				Side:      ctypes.PositionSideLong,
				Leverage:  leverage,
				UpdatedTs: now,
			}, now)
			if err := e.engine.Publish(ctx, msg); err != nil {
				return nil, err
			}

			// 发送杠杆变更事件
			msg = ctypes.NewMessage(exchange, selector, &ctypes.SymbolLeverage{
				Exchange:  exchange,
				Symbol:    symbol,
				Side:      ctypes.PositionSideShort,
				Leverage:  leverage,
				UpdatedTs: now,
			}, now)
			if err := e.engine.Publish(ctx, msg); err != nil {
				return nil, err
			}
		}

		return nil, nil
	})
	if err != nil {
		return err
	}
	e.recordPositionSnapshotsForSymbolBothSides(ctx, accountID, exchange.String(), symbol.String(), now)
	return nil
}

