package account

import (
	"context"
	"errors"
	"sync"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/strategy"
	mb "github.com/wangliang139/llt-trade/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/infra/clock"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	"github.com/wangliang139/llt-trade/server/pkg/types"
)

// AccountManager 账户管理器，管理多个 Account（按 accountId 分组）。
type AccountManager struct {
	mu       sync.RWMutex
	accounts map[string]*account
	bus      mb.Bus
	clock    clock.Clock
}

var _ strategy.AccountEngine = (*AccountManager)(nil)

// NewAccountManager 创建账户管理器
func NewAccountManager(bus mb.Bus, clk clock.Clock) (*AccountManager, error) {
	if bus == nil {
		return nil, errors.New("bus is required")
	}
	if clk == nil {
		return nil, errors.New("clock is required")
	}
	m := &AccountManager{
		accounts: make(map[string]*account),
		bus:      bus,
		clock:    clk,
	}

	// 账户管理器在状态更新阶段处理事件（在撮合之后、策略执行之前）
	// 订阅杠杆变更事件
	bus.Subscribe(m.onLeverageChanged, int(mb.StageStateUpdate), mb.NewTypeFilter(types.SignalTypeLeverage))

	// 订阅成交事件
	bus.Subscribe(func(ctx context.Context, event stypes.Signal) error {
		return m.onFillSignal(ctx, event)
	}, int(mb.StageStateUpdate), mb.NewCompositeFilter(
		mb.NewTypeFilter(types.SignalTypeFill),
	))

	// 订阅余额变更事件（用于初始化 & 同步余额）
	bus.Subscribe(func(ctx context.Context, event stypes.Signal) error {
		return m.onBalanceSignal(ctx, event)
	}, int(mb.StageStateUpdate), mb.NewCompositeFilter(
		mb.NewTypeFilter(types.SignalTypeBalance),
	))

	// 订阅仓位变更事件（由交易所网关发布的真值）
	bus.Subscribe(func(ctx context.Context, event stypes.Signal) error {
		return m.onPositionSignal(ctx, event)
	}, int(mb.StageStateUpdate), mb.NewCompositeFilter(
		mb.NewTypeFilter(types.SignalTypePosition),
	))

	return m, nil
}

func (m *AccountManager) onFillSignal(ctx context.Context, event stypes.Signal) error {
	if event == nil || event.GetAccountID() == nil {
		return nil
	}
	if event.GetType() != types.SignalTypeFill {
		return nil
	}
	fe, ok := event.(*stypes.FillSignal)
	if !ok || fe == nil {
		return nil
	}
	accountID := *fe.GetAccountID()
	acc := m.GetAccount(accountID)
	if acc == nil {
		return errors.New("account not found")
	}
	return acc.ApplyFill(ctx, fe)
}

func (m *AccountManager) onBalanceSignal(ctx context.Context, event stypes.Signal) error {
	if event == nil || event.GetAccountID() == nil {
		return nil
	}
	if event.GetType() != types.SignalTypeBalance {
		return nil
	}
	accountID := *event.GetAccountID()
	acc := m.GetAccount(accountID)
	if acc == nil {
		return errors.New("account not found")
	}
	switch bs := event.(type) {
	case *stypes.BalanceDeltaSignal:
		return acc.ApplyBalanceDelta(ctx, bs)
	case *stypes.BalanceSignal:
		return acc.ApplyBalanceSnapshot(ctx, bs)
	default:
		return nil
	}
}

func (m *AccountManager) onPositionSignal(ctx context.Context, event stypes.Signal) error {
	if event == nil || event.GetAccountID() == nil {
		return nil
	}
	if event.GetType() != types.SignalTypePosition {
		return nil
	}
	ps, ok := event.(*stypes.PositionSignal)
	if !ok || ps == nil {
		return nil
	}
	accountID := *ps.GetAccountID()
	acc := m.GetAccount(accountID)
	if acc == nil {
		return errors.New("account not found")
	}
	return acc.ApplyPosition(ctx, ps)
}

func (m *AccountManager) onLeverageChanged(ctx context.Context, sig stypes.Signal) error {
	s, ok := sig.(*stypes.LeverageChangedSignal)
	if !ok || s == nil {
		return nil
	}
	if s.GetAccountID() == nil || s.GetExchange() == nil || s.GetSymbol() == nil {
		return nil
	}
	accountID := *s.GetAccountID()
	symbol := *s.GetSymbol()
	// 变更信号内的 leverage 若非法，忽略（防御）
	if s.Leverage <= 0 {
		return nil
	}
	return m.SetLeverage(ctx, accountID, symbol, s.Leverage)
}

// GetOrCreateAccount 获取或创建指定 accountId 的 Account
func (m *AccountManager) CreateAccount(accountID string, config AccountConfig) *account {
	m.mu.Lock()
	defer m.mu.Unlock()

	acc := m.accounts[accountID]
	if acc == nil {
		acc = NewAccount(accountID, config, m.bus, m.clock)
		m.accounts[accountID] = acc
	}
	return acc
}

// GetAccount 获取指定 accountId 的 Account，如果不存在返回 nil
func (m *AccountManager) GetAccount(accountID string) *account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.accounts[accountID]
}

// AddAccount 添加一个 Account
func (m *AccountManager) AddAccount(acc *account) {
	if acc == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accounts[acc.GetAccountID()] = acc
}

func (m *AccountManager) GetSymbolConfig(ctx context.Context, accountID string, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	acc := m.GetAccount(accountID)
	if acc == nil {
		return nil, errors.New("account not found")
	}
	return acc.GetSymbolConfig(ctx, accountID, symbol)
}

// GetLeverage 获取指定账户、指定标的、指定方向的杠杆倍数；若未设置，返回 1。
func (m *AccountManager) GetLeverage(ctx context.Context, accountID string, symbol ctypes.Symbol) (int, error) {
	acc := m.GetAccount(accountID)
	if acc == nil {
		// 语义：未设置（或账户尚未初始化）默认杠杆为 1
		return 1, nil
	}
	lev, err := acc.GetLeverage(ctx, accountID, symbol)
	if err != nil {
		return 1, err
	}
	if lev <= 0 {
		return 1, nil
	}
	return lev, nil
}

// SetLeverage 设置指定账户、指定标的的杠杆倍数。
// 注意：该方法只落地配置，不负责发布变更事件（由 exchange gateway 在变更后发布）。
func (m *AccountManager) SetLeverage(ctx context.Context, accountID string, symbol ctypes.Symbol, leverage int) error {
	acc := m.GetAccount(accountID)
	if acc == nil {
		// 杠杆配置应当可以在账户未初始化时落地（例如：先设置杠杆再注入余额/仓位）。
		acc = m.CreateAccount(accountID, AccountConfig{Exchange: acc.GetExchange()})
	}
	err := acc.SetLeverage(ctx, accountID, symbol, leverage)
	if err != nil {
		return err
	}
	return nil
}

func (m *AccountManager) GetBalance(ctx context.Context, accountID string) ([]*ctypes.AssetBo, error) {
	acc := m.GetAccount(accountID)
	if acc == nil {
		return nil, errors.New("account not found")
	}
	return acc.GetBalance(ctx, accountID)
}

func (m *AccountManager) GetAsset(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string) (*ctypes.AssetBo, error) {
	acc := m.GetAccount(accountID)
	if acc == nil {
		return nil, errors.New("account not found")
	}
	return acc.GetAsset(ctx, accountID, symbol, asset)
}

func (m *AccountManager) FreezeFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error {
	acc := m.GetAccount(accountID)
	if acc == nil {
		return errors.New("account not found")
	}
	return acc.FreezeFunds(ctx, accountID, symbol, asset, amount, order)
}

func (m *AccountManager) UnfreezeFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error {
	acc := m.GetAccount(accountID)
	if acc == nil {
		return errors.New("account not found")
	}
	return acc.UnfreezeFunds(ctx, accountID, symbol, asset, amount, order)
}

func (m *AccountManager) GetPositions(ctx context.Context, accountID string) ([]*ctypes.Position, error) {
	acc := m.GetAccount(accountID)
	if acc == nil {
		return nil, errors.New("account not found")
	}
	return acc.GetPositions(ctx, accountID)
}

func (m *AccountManager) GetPosition(ctx context.Context, accountID string, symbol ctypes.Symbol, side ctypes.PositionSide) (*ctypes.Position, error) {
	acc := m.GetAccount(accountID)
	if acc == nil {
		return nil, errors.New("account not found")
	}
	return acc.GetPosition(ctx, accountID, symbol, side)
}

func (m *AccountManager) GetPositionQty(ctx context.Context, accountID string, symbol ctypes.Symbol, side ctypes.PositionSide) (decimal.Decimal, error) {
	acc := m.GetAccount(accountID)
	if acc == nil {
		return decimal.Zero, errors.New("account not found")
	}
	return acc.GetPositionQty(ctx, accountID, symbol, side)
}

func (m *AccountManager) GetAvgPrice(ctx context.Context, accountID string, symbol ctypes.Symbol, side ctypes.PositionSide) (decimal.Decimal, error) {
	acc := m.GetAccount(accountID)
	if acc == nil {
		return decimal.Zero, errors.New("account not found")
	}
	return acc.GetAvgPrice(ctx, accountID, symbol, side)
}
