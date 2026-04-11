package symbolaccount

import (
	"context"
	"fmt"
	"sync"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	mb "github.com/wangliang139/llt-trade/server/pkg/strategy/infra/bus"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

// BalanceView 资产余额视图（策略级，按 symbol 隔离）
type BalanceView struct {
	Asset    string
	Free     decimal.Decimal
	Frozen   decimal.Decimal
	UpdateAt int64
}

// Total 返回总余额
func (b *BalanceView) Total() decimal.Decimal {
	return b.Free.Add(b.Frozen)
}

// SymbolAccount 单个交易对的资产账本（策略级隔离）
type SymbolAccount struct {
	ExSymbol ctypes.ExSymbol
	Balances map[string]*BalanceView // asset -> balance
}

// Manager 策略级交易对账户管理器
// 按 (exSymbolKey, asset) 维度隔离资金，用于单次回测/策略执行内的资金额度控制
type Manager struct {
	mu       sync.RWMutex
	accounts map[ctypes.ExSymbolKey]*SymbolAccount
}

// NewManager 创建策略级账户管理器
func NewManager() *Manager {
	return &Manager{
		accounts: make(map[ctypes.ExSymbolKey]*SymbolAccount),
	}
}

// Subscribe 订阅资金类事件
func (m *Manager) Subscribe(bus mb.Bus) {
	// SymbolAccount 在状态更新阶段处理（与账户管理器同阶段）
	bus.Subscribe(m.onSignal, int(mb.StageStateUpdate), mb.NewFuncFilter(func(sig stypes.Signal) bool {
		// 只处理带 exchange+symbol 的资金类事件
		if sig.GetExchange() == nil || sig.GetSymbol() == nil {
			return false
		}
		switch sig.GetType() {
		case stypes.SignalTypeBalance, stypes.SignalTypeFill:
			return true
		}
		return false
	}))
}

// onSignal 处理资金变更信号
func (m *Manager) onSignal(ctx context.Context, sig stypes.Signal) error {
	switch e := sig.(type) {
	case *stypes.BalanceDeltaSignal:
		return m.onBalanceDeltaSignal(ctx, e)
	case *stypes.BalanceSignal:
		return m.onBalanceSnapshotSignal(ctx, e)
	}
	return nil
}

// onBalanceDeltaSignal 处理余额变更事件（delta 语义）
func (m *Manager) onBalanceDeltaSignal(ctx context.Context, sig *stypes.BalanceDeltaSignal) error {
	if sig.GetExchange() == nil || sig.GetSymbol() == nil {
		return nil
	}

	exSymbol := ctypes.NewExSymbol(*sig.GetExchange(), *sig.GetSymbol())
	asset := sig.Asset

	m.mu.Lock()
	defer m.mu.Unlock()

	bal := m.getOrCreateBalanceViewLocked(exSymbol, asset)

	// 时间戳保护：旧事件不覆盖新状态
	if sig.GetTimestamp().UnixNano() < bal.UpdateAt {
		return nil
	}

	// delta 语义：累加
	bal.Free = bal.Free.Add(sig.Free)
	bal.Frozen = bal.Frozen.Add(sig.Frozen)

	// 防止负值
	if bal.Free.IsNegative() {
		bal.Free = decimal.Zero
	}
	if bal.Frozen.IsNegative() {
		bal.Frozen = decimal.Zero
	}

	bal.UpdateAt = sig.GetTimestamp().UnixNano()
	return nil
}

// onBalanceSnapshotSignal 处理余额快照事件（snapshot 语义）。
func (m *Manager) onBalanceSnapshotSignal(ctx context.Context, sig *stypes.BalanceSignal) error {
	if sig.GetExchange() == nil || sig.GetSymbol() == nil {
		return nil
	}

	exSymbol := ctypes.NewExSymbol(*sig.GetExchange(), *sig.GetSymbol())
	asset := sig.Asset

	m.mu.Lock()
	defer m.mu.Unlock()

	bal := m.getOrCreateBalanceViewLocked(exSymbol, asset)

	// 时间戳保护：旧事件不覆盖新状态
	if sig.GetTimestamp().UnixNano() < bal.UpdateAt {
		return nil
	}

	// snapshot 语义：覆盖
	bal.Free = sig.Free
	bal.Frozen = sig.Frozen

	// 防止负值
	if bal.Free.IsNegative() {
		bal.Free = decimal.Zero
	}
	if bal.Frozen.IsNegative() {
		bal.Frozen = decimal.Zero
	}

	bal.UpdateAt = sig.GetTimestamp().UnixNano()
	_ = ctx
	return nil
}

// getOrCreateAccount 获取或创建交易对账户（内部方法，需持锁）
func (m *Manager) getOrCreateAccount(exSymbol ctypes.ExSymbol) *SymbolAccount {
	key := exSymbol.Key()
	acc := m.accounts[key]
	if acc == nil {
		acc = &SymbolAccount{
			ExSymbol: exSymbol,
			Balances: make(map[string]*BalanceView),
		}
		m.accounts[key] = acc
	}
	return acc
}

// getOrCreateBalanceViewLocked 获取或创建指定交易对资产余额（需持锁）。
func (m *Manager) getOrCreateBalanceViewLocked(exSymbol ctypes.ExSymbol, asset string) *BalanceView {
	acc := m.getOrCreateAccount(exSymbol)
	bal := acc.Balances[asset]
	if bal == nil {
		bal = &BalanceView{
			Asset:    asset,
			Free:     decimal.Zero,
			Frozen:   decimal.Zero,
			UpdateAt: 0,
		}
		acc.Balances[asset] = bal
	}
	return bal
}

// GetAvailable 获取指定交易对的指定资产可用余额
func (m *Manager) GetAvailable(exSymbol ctypes.ExSymbol, asset string) decimal.Decimal {
	m.mu.RLock()
	defer m.mu.RUnlock()

	acc := m.accounts[exSymbol.Key()]
	if acc == nil {
		return decimal.Zero
	}

	bal := acc.Balances[asset]
	if bal == nil {
		return decimal.Zero
	}

	return bal.Free
}

// GetBalance 获取指定交易对的指定资产余额（Free + Frozen）
func (m *Manager) GetBalance(exSymbol ctypes.ExSymbol, asset string) (free, frozen decimal.Decimal) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	acc := m.accounts[exSymbol.Key()]
	if acc == nil {
		return decimal.Zero, decimal.Zero
	}

	bal := acc.Balances[asset]
	if bal == nil {
		return decimal.Zero, decimal.Zero
	}

	return bal.Free, bal.Frozen
}

// AssertAvailable 校验指定交易对的指定资产可用余额是否足够
// 如果不足，返回错误
func (m *Manager) AssertAvailable(exSymbol ctypes.ExSymbol, asset string, required decimal.Decimal) error {
	available := m.GetAvailable(exSymbol, asset)
	if available.LessThan(required) {
		return fmt.Errorf(
			"insufficient symbol account balance: symbol=%s asset=%s available=%s required=%s",
			exSymbol.String(), asset, available.String(), required.String(),
		)
	}
	return nil
}

// GetSymbolBalances 获取指定交易对的所有资产余额（用于权益计算）
func (m *Manager) GetSymbolBalances(exSymbol ctypes.ExSymbol) map[string]*BalanceView {
	m.mu.RLock()
	defer m.mu.RUnlock()

	acc := m.accounts[exSymbol.Key()]
	if acc == nil {
		return make(map[string]*BalanceView)
	}

	// 返回副本，避免外部修改
	result := make(map[string]*BalanceView, len(acc.Balances))
	for asset, bal := range acc.Balances {
		result[asset] = &BalanceView{
			Asset:    bal.Asset,
			Free:     bal.Free,
			Frozen:   bal.Frozen,
			UpdateAt: bal.UpdateAt,
		}
	}
	return result
}
