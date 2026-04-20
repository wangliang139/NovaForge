package account

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/internal/consts"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	mb "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func formatAmountWithPrecision(amount decimal.Decimal, precision int) decimal.Decimal {
	if precision <= 0 {
		precision = consts.DefaultAssetPrecision
	}
	return amount.Round(int32(precision))
}

type AccountConfig struct {
	Exchange       ctypes.Exchange
	AssetPrecision int
}

type account struct {
	config AccountConfig

	accountID string

	mu sync.RWMutex

	clock clock.Clock
	bus   mb.Bus

	// 内部按 market type 隔离
	spotLedgers   map[string]*AssetLedger // asset -> AssetLedger
	futureLedgers map[string]*AssetLedger // asset -> AssetLedger

	// exSymbolKey:side -> leverage
	leverages map[string]int
}

var _ strategy.AccountEngine = (*account)(nil)

func NewAccount(accountID string, config AccountConfig, bus mb.Bus, clk clock.Clock) *account {
	if config.AssetPrecision <= 0 {
		config.AssetPrecision = consts.DefaultAssetPrecision
	}
	account := &account{
		accountID:     accountID,
		config:        config,
		clock:         clk,
		bus:           bus,
		spotLedgers:   make(map[string]*AssetLedger),
		futureLedgers: make(map[string]*AssetLedger),
		leverages:     make(map[string]int),
	}
	return account
}

func (a *account) GetAccountID() string {
	return a.accountID
}

func (a *account) GetExchange() ctypes.Exchange {
	return a.config.Exchange
}

func (a *account) assetPrecision() int {
	if a.config.AssetPrecision <= 0 {
		return consts.DefaultAssetPrecision
	}
	return a.config.AssetPrecision
}

func (a *account) formatAmount(amount decimal.Decimal) decimal.Decimal {
	return formatAmountWithPrecision(amount, a.assetPrecision())
}

func leverageKey(exSymbol ctypes.ExSymbol) string {
	return exSymbol.Key().String()
}

func (a *account) GetSymbolConfig(ctx context.Context, accountID string, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	symbolConfig, err := proxy.GetSymbolConfig(ctx, accountID, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get symbol config: %w", err)
	}
	if symbolConfig == nil {
		return nil, fmt.Errorf("symbol config not found")
	}
	return symbolConfig, nil
}

func (a *account) GetLeverage(ctx context.Context, accountID string, symbol ctypes.Symbol) (int, error) {
	return a.leverages[leverageKey(ctypes.NewExSymbol(a.GetExchange(), symbol))], nil
}

func (a *account) SetLeverage(ctx context.Context, accountID string, symbol ctypes.Symbol, leverage int) error {
	if leverage <= 0 {
		return errors.New("leverage must be > 0")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.leverages[leverageKey(ctypes.NewExSymbol(a.GetExchange(), symbol))] = leverage
	return nil
}

// ApplyBalance 保持兼容：仅处理增量余额事件（delta 语义）。
func (a *account) ApplyBalance(ctx context.Context, msg stypes.Signal) error {
	b, ok := msg.(*stypes.BalanceDeltaSignal)
	if !ok || b == nil {
		return nil
	}
	return a.ApplyBalanceDelta(ctx, b)
}

// ApplyBalanceDelta 应用资金变更增量（delta 语义）。
func (a *account) ApplyBalanceDelta(ctx context.Context, b *stypes.BalanceDeltaSignal) error {
	if b.GetExchange() == nil || b.GetSymbol() == nil {
		return nil
	}
	exchange := *b.GetExchange()
	if exchange != a.GetExchange() {
		return nil
	}
	symbol := *b.GetSymbol()
	asset := b.Asset
	if asset == "" {
		return nil
	}

	ledger := a.ensureAssetLedger(symbol.Type, asset)

	a.mu.Lock()
	defer a.mu.Unlock()

	freeDelta := a.formatAmount(b.Free)
	frozenDelta := a.formatAmount(b.Frozen)

	ledger.Available = a.formatAmount(ledger.Available.Add(freeDelta))
	if ledger.Available.IsNegative() {
		ledger.Available = decimal.Zero
	}

	ledger.Locked = a.formatAmount(ledger.Locked.Add(frozenDelta))
	if ledger.Locked.IsNegative() {
		ledger.Locked = decimal.Zero
	}

	ledger.updateAvailable()

	_ = ctx
	return nil
}

// ApplyBalanceSnapshot 应用资金快照（snapshot 语义）。
func (a *account) ApplyBalanceSnapshot(ctx context.Context, b *stypes.BalanceSignal) error {
	if b.GetExchange() == nil || b.GetSymbol() == nil {
		return nil
	}
	exchange := *b.GetExchange()
	if exchange != a.GetExchange() {
		return nil
	}
	symbol := *b.GetSymbol()
	asset := b.Asset
	if asset == "" {
		return nil
	}

	ledger := a.ensureAssetLedger(symbol.Type, asset)

	a.mu.Lock()
	defer a.mu.Unlock()

	ledger.Available = a.formatAmount(b.Free)
	if ledger.Available.IsNegative() {
		ledger.Available = decimal.Zero
	}

	ledger.Locked = a.formatAmount(b.Frozen)
	if ledger.Locked.IsNegative() {
		ledger.Locked = decimal.Zero
	}

	ledger.updateAvailable()

	_ = ctx
	return nil
}

func (a *account) ledgersFor(mt ctypes.MarketType) map[string]*AssetLedger {
	if mt == ctypes.MarketTypeFuture {
		return a.futureLedgers
	}
	return a.spotLedgers
}

// ensureAssetLedger 确保资产账本存在（线程安全）
func (a *account) ensureAssetLedger(mt ctypes.MarketType, asset string) *AssetLedger {
	a.mu.Lock()
	defer a.mu.Unlock()

	m := a.ledgersFor(mt)
	ledger := m[asset]
	if ledger == nil {
		ledger = NewAssetLedgerWithPrecision(asset, a.assetPrecision())
		m[asset] = ledger
	}
	return ledger
}

func (a *account) GetBalance(ctx context.Context, accountID string) ([]*ctypes.AssetBo, error) {
	_ = ctx
	_ = accountID

	a.mu.RLock()
	defer a.mu.RUnlock()

	assets := make([]*ctypes.AssetBo, 0, len(a.spotLedgers)+len(a.futureLedgers))
	for _, ledger := range a.spotLedgers {
		assets = append(assets, &ctypes.AssetBo{
			AccountID:  a.GetAccountID(),
			WalletType: ctypes.WalletTypeTrade,
			Code:       ledger.Asset,
			Balance:    ledger.Available.Add(ledger.Locked),
			Locked:     ledger.Locked,
		})
	}
	for _, ledger := range a.futureLedgers {
		assets = append(assets, &ctypes.AssetBo{
			AccountID:  a.GetAccountID(),
			WalletType: ctypes.WalletTypeTrade,
			Code:       ledger.Asset,
			Balance:    ledger.Available.Add(ledger.Locked),
			Locked:     ledger.Locked,
		})
	}
	return assets, nil
}

// 实现 AccountProvider 接口
func (a *account) GetAsset(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string) (*ctypes.AssetBo, error) {
	_ = ctx
	ledger := a.ensureAssetLedger(symbol.Type, asset)

	a.mu.RLock()
	defer a.mu.RUnlock()

	ledger.updateAvailable()
	return &ctypes.AssetBo{
		AccountID:  a.GetAccountID(),
		WalletType: ctypes.WalletTypeTrade,
		Code:       asset,
		Balance:    ledger.Available.Add(ledger.Locked),
		Locked:     ledger.Locked,
	}, nil
}

func (a *account) FreezeFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error {
	amount = a.formatAmount(amount)

	if !amount.GreaterThan(decimal.Zero) {
		return nil
	}

	ledger := a.ensureAssetLedger(symbol.Type, asset)

	a.mu.Lock()
	ledger.updateAvailable()
	if amount.GreaterThan(ledger.Available) {
		a.mu.Unlock()
		return errors.New("insufficient balance")
	}
	ledger.Locked = ledger.Locked.Add(amount)
	ledger.Available = ledger.Available.Sub(amount)
	ledger.updateAvailable()
	a.mu.Unlock()

	// 发送冻结事件（在锁外发送，避免死锁）
	if a.bus != nil {
		return a.bus.Publish(ctx, &stypes.BalanceDeltaSignal{
			BaseSignal: stypes.BaseSignal{
				Exchange:  lo.ToPtr(a.GetExchange()),
				Symbol:    &symbol,
				AccountID: lo.ToPtr(a.GetAccountID()),
				Ts:        a.clock.Now(),
			},
			WalletType: ctypes.WalletTypeTrade,
			Asset:      asset,
			Frozen:     amount,
		})
	}
	return nil
}

func (a *account) UnfreezeFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error {
	amount = a.formatAmount(amount)
	if !amount.GreaterThan(decimal.Zero) {
		return nil
	}
	ledger := a.ensureAssetLedger(symbol.Type, asset)

	a.mu.Lock()
	ledger.updateAvailable()
	if ledger.Locked.LessThanOrEqual(decimal.Zero) {
		a.mu.Unlock()
		return nil
	}

	// 防御：最多解冻已冻结的部分，避免负数
	if amount.GreaterThan(ledger.Locked) {
		amount = ledger.Locked
	}
	ledger.Locked = ledger.Locked.Sub(amount)
	ledger.Available = ledger.Available.Add(amount)
	ledger.updateAvailable()
	a.mu.Unlock()

	// 发送解冻事件（在锁外发送，避免死锁）
	if a.bus != nil && amount.GreaterThan(decimal.Zero) {
		err := a.bus.Publish(ctx, &stypes.BalanceDeltaSignal{
			BaseSignal: stypes.BaseSignal{
				Exchange:  lo.ToPtr(a.GetExchange()),
				Symbol:    &symbol,
				AccountID: lo.ToPtr(a.GetAccountID()),
				Ts:        a.clock.Now(),
			},
			WalletType: ctypes.WalletTypeTrade,
			Asset:      asset,
			Frozen:     amount,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to publish funds unfrozen signal")
			return err
		}
	}
	return nil
}

func (a *account) DeductFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal) error {
	_amt := amount.String()
	amount = a.formatAmount(amount)
	_amt = amount.String()
	_ = _amt

	ledger := a.ensureAssetLedger(symbol.Type, asset)

	a.mu.Lock()
	defer a.mu.Unlock()

	_available := ledger.Available.String()
	_locked := ledger.Locked.String()
	_ = _available
	_ = _locked

	ledger.updateAvailable()
	if amount.GreaterThan(ledger.Balance()) {
		return errors.New("insufficient total balance")
	}

	ledger.Locked = ledger.Locked.Sub(amount)
	if ledger.Locked.LessThan(decimal.Zero) {
		ledger.Available = ledger.Available.Add(ledger.Locked)
		ledger.Locked = decimal.Zero
	}
	ledger.updateAvailable()
	return nil
}

func (a *account) AddFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal) {
	amount = a.formatAmount(amount)
	ledger := a.ensureAssetLedger(symbol.Type, asset)

	a.mu.Lock()
	defer a.mu.Unlock()

	ledger.Available = ledger.Available.Add(amount)
	ledger.updateAvailable()
}

// determinePositionSide 根据Side和IsBuy确定持仓方向
func determinePositionSide(side ctypes.PositionSide, isBuy bool) ctypes.PositionSide {
	// Side=Long && IsBuy=true 开多仓 -> LONG
	// Side=Long && IsBuy=false 平多仓 -> LONG (平仓时保持原方向)
	// Side=Short && IsBuy=true 开空仓 -> SHORT
	// Side=Short && IsBuy=false 平空仓 -> SHORT (平仓时保持原方向)
	return side
}

func (a *account) GetPositions(ctx context.Context, accountID string) ([]*ctypes.Position, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	positions := make([]*ctypes.Position, 0)

	// 现货：从 SpotCostBasis 汇总持仓
	for _, ledger := range a.spotLedgers {
		for _, cb := range ledger.SpotCostBasis {
			if cb == nil || !cb.Qty.GreaterThan(decimal.Zero) {
				continue
			}
			positions = append(positions, &ctypes.Position{
				Symbol:     cb.ExSymbol.Symbol,
				Side:       ctypes.PositionSideLong,
				Amount:     cb.Qty,
				EntryPrice: cb.AvgCostQuote,
			})
		}
	}

	// 合约：从 Positions 汇总持仓
	for _, ledger := range a.futureLedgers {
		for _, pos := range ledger.Positions {
			if pos == nil || pos.Amount.IsZero() {
				continue
			}
			if pos.Side == ctypes.PositionSideLong && pos.Amount.GreaterThan(decimal.Zero) {
				positions = append(positions, pos)
				continue
			}
			if pos.Side == ctypes.PositionSideShort && pos.Amount.LessThan(decimal.Zero) {
				positions = append(positions, pos)
			}
		}
	}

	return positions, nil
}

func (a *account) GetPosition(ctx context.Context, accountID string, symbol ctypes.Symbol, side ctypes.PositionSide) (*ctypes.Position, error) {
	_ = ctx
	exSymbol := ctypes.NewExSymbol(a.GetExchange(), symbol)
	marginAsset := exSymbol.GetQuote() // 期货合约的保证金资产是Quote
	ledger := a.ensureAssetLedger(symbol.Type, marginAsset)

	a.mu.RLock()
	defer a.mu.RUnlock()

	// 现货：从 SpotCostBasis 获取持仓和成本价
	if symbol.Type == ctypes.MarketTypeSpot {
		cb := ledger.GetSpotCostBasis(exSymbol)
		if cb != nil && cb.Qty.GreaterThan(decimal.Zero) {
			return &ctypes.Position{
				Symbol:     symbol,
				Side:       ctypes.PositionSideLong, // 现货只有多仓
				Amount:     cb.Qty,
				EntryPrice: cb.AvgCostQuote,
			}, nil
		}
		// 没有持仓，返回空持仓
		return &ctypes.Position{
			Symbol:     symbol,
			Side:       ctypes.PositionSideLong,
			Amount:     decimal.Zero,
			EntryPrice: decimal.Zero,
		}, nil
	}

	// 合约：从 Positions 获取
	// 尝试查找LONG持仓
	if side == ctypes.PositionSideLong {
		longKey := PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideLong}
		if pos, ok := ledger.Positions[longKey]; ok && pos.Amount.GreaterThan(decimal.Zero) {
			return pos, nil
		}
	} else {
		// 尝试查找SHORT持仓
		shortKey := PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideShort}
		if pos, ok := ledger.Positions[shortKey]; ok && pos.Amount.LessThan(decimal.Zero) {
			return pos, nil
		}
	}

	// 没有持仓，返回空持仓
	return &ctypes.Position{
		Symbol:     symbol,
		Side:       side,
		Amount:     decimal.Zero,
		EntryPrice: decimal.Zero,
	}, nil
}

func (a *account) GetPositionQty(ctx context.Context, accountID string, symbol ctypes.Symbol, side ctypes.PositionSide) (decimal.Decimal, error) {
	pos, err := a.GetPosition(ctx, accountID, symbol, side)
	if err != nil {
		return decimal.Zero, err
	}
	return a.formatAmount(pos.Amount), nil
}

func (a *account) GetAvgPrice(ctx context.Context, accountID string, symbol ctypes.Symbol, side ctypes.PositionSide) (decimal.Decimal, error) {
	// GetPosition 已经处理了现货/合约的差异
	pos, err := a.GetPosition(ctx, accountID, symbol, side)
	if err != nil {
		return decimal.Zero, err
	}
	return pos.EntryPrice, nil
}

func (a *account) UpdatePosition(ctx context.Context, accountID string, symbol ctypes.Symbol, side ctypes.PositionSide, isBuy bool, qty, price decimal.Decimal) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return
	}
	exSymbol := ctypes.NewExSymbol(a.GetExchange(), symbol)
	marginAsset := exSymbol.GetQuote() // 期货合约的保证金资产是Quote
	ledger := a.ensureAssetLedger(symbol.Type, marginAsset)
	qty = a.formatAmount(qty)

	a.mu.Lock()
	defer a.mu.Unlock()

	// 确定持仓方向
	positionSide := determinePositionSide(side, isBuy)
	positionKey := PositionKey{ExSymbol: exSymbol, Side: positionSide}

	// 获取或创建持仓
	pos := ledger.Positions[positionKey]
	if pos == nil {
		pos = &ctypes.Position{
			Symbol:     symbol,
			Side:       positionSide,
			Amount:     decimal.Zero,
			EntryPrice: decimal.Zero,
		}
		ledger.Positions[positionKey] = pos
	}
	pos.Amount = a.formatAmount(pos.Amount)

	// FUTURE：支持双向持仓
	if positionSide == ctypes.PositionSideLong {
		// 多仓：isBuy增加，!isBuy减少
		if isBuy {
			// 开多或加多
			if pos.Amount.IsZero() {
				pos.EntryPrice = price
				pos.Amount = qty
			} else {
				totalCost := pos.EntryPrice.Mul(pos.Amount).Add(price.Mul(qty))
				newQty := a.formatAmount(pos.Amount.Add(qty))
				if !newQty.IsZero() {
					pos.EntryPrice = totalCost.Div(newQty)
				} else {
					pos.EntryPrice = decimal.Zero
				}
				pos.Amount = newQty
			}
		} else {
			// 平多
			closeQty := decimal.Min(pos.Amount, qty)
			pos.Amount = a.formatAmount(pos.Amount.Sub(closeQty))
			if pos.Amount.IsZero() {
				pos.EntryPrice = decimal.Zero
			}
		}
	} else {
		// 空仓：!isBuy增加，isBuy减少
		if !isBuy {
			// 开空或加空
			if pos.Amount.IsZero() {
				pos.EntryPrice = price
				pos.Amount = a.formatAmount(qty.Neg())
			} else {
				// 用 abs qty 做加权均价
				totalCost := pos.EntryPrice.Mul(pos.Amount.Abs()).Add(price.Mul(qty))
				newQty := a.formatAmount(pos.Amount.Sub(qty))
				if !newQty.IsZero() {
					pos.EntryPrice = totalCost.Div(newQty.Abs())
				} else {
					pos.EntryPrice = decimal.Zero
				}
				pos.Amount = newQty
			}
		} else {
			// 平空
			closeQty := decimal.Min(pos.Amount.Abs(), qty)
			pos.Amount = a.formatAmount(pos.Amount.Add(closeQty))
			if pos.Amount.IsZero() {
				pos.EntryPrice = decimal.Zero
			}
		}
	}

	// 如果持仓为0，删除持仓记录
	if pos.Amount.IsZero() {
		delete(ledger.Positions, positionKey)
	}
}

func (a *account) GetMarginUsed(ctx context.Context, accountID string, symbol ctypes.Symbol) (decimal.Decimal, error) {
	exSymbol := ctypes.NewExSymbol(a.GetExchange(), symbol)
	marginAsset := exSymbol.GetQuote() // 期货合约的保证金资产是Quote
	ledger := a.ensureAssetLedger(symbol.Type, marginAsset)

	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.formatAmount(ledger.MarginUsed), nil
}

func (a *account) SetLocked(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal) {
	_ = ctx
	amount = a.formatAmount(amount)
	ledger := a.ensureAssetLedger(symbol.Type, asset)

	a.mu.Lock()
	defer a.mu.Unlock()

	ledger.Locked = amount
	if ledger.Locked.LessThan(decimal.Zero) {
		ledger.Locked = decimal.Zero
	}
	if ledger.Locked.GreaterThan(ledger.Balance()) {
		ledger.Locked = ledger.Balance()
	}
	ledger.updateAvailable()
}

// ApplyFill 处理成交事件
//
// 说明：
// - 资金变更由 exchange gateway 发布的 BalanceSignal（delta 语义）处理
// - 持仓变更由 exchange gateway 发布的 PositionSignal 处理
// - 本函数负责：现货 WAC 成本跟踪，供 gateway 计算已实现盈亏使用
func (a *account) ApplyFill(ctx context.Context, msg stypes.Signal) error {
	f, ok := msg.(*stypes.FillSignal)
	if !ok {
		return errors.New("invalid fill signal")
	}

	if f.GetExchange() == nil || f.GetSymbol() == nil {
		return errors.New("fill signal missing exchange/symbol")
	}

	exSymbol := ctypes.NewExSymbol(*f.GetExchange(), *f.GetSymbol())

	// 只对现货更新成本跟踪
	if exSymbol.GetType() != ctypes.MarketTypeSpot {
		return nil
	}

	// 使用 quote 资产账本（现货成本以 quote 计价）
	ledger := a.ensureAssetLedger(exSymbol.GetType(), exSymbol.GetQuote())

	a.mu.Lock()
	defer a.mu.Unlock()

	// 更新现货成本跟踪（会返回已实现盈亏，但这里不使用，由 gateway 计算）
	_ = ledger.UpdateSpotCostBasis(exSymbol, f.IsBuy, f.Qty, f.Price)

	return nil
}

// ApplyPosition 根据 PositionSignal 校准账户持仓（PositionSignal 为快照语义）
func (a *account) ApplyPosition(ctx context.Context, msg stypes.Signal) error {
	ps, ok := msg.(*stypes.PositionSignal)
	if !ok {
		return errors.New("invalid position signal")
	}
	if ps.GetExchange() == nil || ps.GetSymbol() == nil {
		return errors.New("position signal missing exchange/symbol")
	}

	exSymbol := ctypes.NewExSymbol(*ps.GetExchange(), *ps.GetSymbol())
	marginAsset := exSymbol.GetQuote()
	ledger := a.ensureAssetLedger(exSymbol.GetType(), marginAsset)

	a.mu.Lock()
	defer a.mu.Unlock()

	longKey := PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideLong}
	shortKey := PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideShort}

	// 清理旧方向记录
	delete(ledger.Positions, longKey)
	delete(ledger.Positions, shortKey)

	// PositionSignal：Qty 为快照规模（非负），方向以 Side 为准；账本空仓仍用负 Amount 与 UpdatePosition 一致
	mag := a.formatAmount(ps.Qty.Abs())
	newEntry := ps.EntryPrice
	if mag.IsZero() {
		return nil
	}

	side := ps.Side
	if !side.Valid() {
		side = ctypes.PositionSideLong
	}
	switch side {
	case ctypes.PositionSideLong:
		ledger.Positions[longKey] = &ctypes.Position{
			Symbol:     exSymbol.Symbol,
			Side:       ctypes.PositionSideLong,
			Amount:     mag,
			EntryPrice: newEntry,
		}
	case ctypes.PositionSideShort:
		ledger.Positions[shortKey] = &ctypes.Position{
			Symbol:     exSymbol.Symbol,
			Side:       ctypes.PositionSideShort,
			Amount:     mag.Neg(),
			EntryPrice: newEntry,
		}
	}

	return nil
}
