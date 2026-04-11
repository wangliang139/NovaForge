// portfolio.go
package portfolio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/strategy"
	mb "github.com/wangliang139/llt-trade/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/marketdata"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

type Portfolio struct {
	mu sync.RWMutex

	marketProvider marketdata.MarketProvider

	reducer *Reducer

	// 账户初始化相关
	accountEngine    strategy.AccountEngine
	accountID        string
	exchange         ctypes.Exchange
	initialised      bool
	lastSnapshotTime int64
}

func NewPortfolio(bus mb.Bus, marketProvider marketdata.MarketProvider) *Portfolio {
	portfolio := &Portfolio{
		reducer:        NewReducer(),
		marketProvider: marketProvider,
	}
	// Portfolio 在状态更新阶段处理事件（在账户/订单更新之后）
	bus.Subscribe(portfolio.OnEvent, int(mb.StageStateUpdate)+10, mb.NewFuncFilter(func(sig stypes.Signal) bool {
		switch sig.GetType() {
		// 资金、仓位、杠杆等账户相关事件
		case stypes.SignalTypeBalance,
			stypes.SignalTypePosition,
			stypes.SignalTypeLeverage:
			return true
		// 预留：当有 Portfolio 快照信号时，用于整体重置
		default:
			return false
		}
	}))
	return portfolio
}

// Init 初始化 Portfolio：从 AccountEngine 批量拉取账户当前状态
// - 仅在策略启动/重启阶段调用一次
// - 后续状态通过 bus 上的增量事件维护
func (p *Portfolio) Init(ctx context.Context, accountEngine strategy.AccountEngine, accountID string, exchange ctypes.Exchange, symbols []ctypes.Symbol) error {
	if accountEngine == nil {
		return fmt.Errorf("account engine is required")
	}
	if accountID == "" {
		return fmt.Errorf("account id is required")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.accountEngine = accountEngine
	p.accountID = accountID
	p.exchange = exchange

	// 1. 初始化余额：按资产维度聚合
	assets, err := accountEngine.GetBalance(ctx, accountID)
	if err != nil {
		return fmt.Errorf("init portfolio balance from account engine: %w", err)
	}
	now := time.Now()
	for _, a := range assets {
		if a == nil {
			continue
		}
		key := ctypes.AssetKey{
			Exchange:   exchange,
			WalletType: a.WalletType,
			Asset:      a.Code,
		}
		view := p.reducer.balances[key]
		view.Asset = a.Code
		view.Free = a.Balance
		view.Frozen = a.Locked
		// 初始快照时间使用当前时间
		view.UpdateAt = now.UnixNano()
		p.reducer.balances[key] = view
	}

	// 2. 初始化持仓（期货）：从 AccountEngine 获取所有仓位后按 symbol 聚合
	positions, err := accountEngine.GetPositions(ctx, accountID)
	if err != nil {
		return fmt.Errorf("init portfolio positions from account engine: %w", err)
	}
	for _, pos := range positions {
		if pos == nil {
			continue
		}
		exSymbol := ctypes.NewExSymbol(exchange, pos.Symbol)
		key := ctypes.PositionKey{
			ExSymbol: exSymbol,
			Side:     pos.Side,
		}
		view := p.reducer.positions[key]
		view.Qty = pos.Amount
		view.Side = pos.Side
		view.AvgPrice = pos.EntryPrice
		view.Symbol = exSymbol
		view.UpdateAt = pos.UpdatedTs.UnixNano()
		p.reducer.positions[key] = view
		p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideLong}] = pos.Leverage
		p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideShort}] = pos.Leverage
	}

	p.initialised = true
	p.lastSnapshotTime = now.UnixNano()
	return nil
}

func (p *Portfolio) OnEvent(ctx context.Context, sig stypes.Signal) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reducer.Apply(ctx, sig)
}

func (p *Portfolio) Snapshot() stypes.PortfolioSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()

	snapshot := stypes.PortfolioSnapshot{
		Positions: make(map[ctypes.PositionKey]stypes.PositionView, len(p.reducer.positions)),
		Balances:  make(map[ctypes.AssetKey]stypes.BalanceView, len(p.reducer.balances)),
		Ts:        time.Now().UnixNano(),
	}
	for k, v := range p.reducer.balances {
		snapshot.Balances[k] = stypes.BalanceView{
			Asset:    v.Asset,
			Free:     v.Free,
			Frozen:   v.Frozen,
			UpdateAt: v.UpdateAt,
		}
	}
	for k, v := range p.reducer.positions {
		lev := p.GetLocalLeverage(k.ExSymbol.Exchange, k.ExSymbol.Symbol)
		snapshot.Positions[k] = stypes.PositionView{
			Symbol:        v.Symbol,
			Side:          v.Side,
			Qty:           v.Qty,
			Leverage:      lev,
			AvgPrice:      v.AvgPrice,
			UnrealizedPnL: v.UnrealizedPnL,
			UpdateAt:      v.UpdateAt,
		}
	}
	return snapshot
}

func (p *Portfolio) GetAsset(exchange ctypes.Exchange, symbol ctypes.Symbol, asset string) (*ctypes.AssetBo, error) {
	if exchange != p.exchange {
		return &ctypes.AssetBo{}, nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	snap := p.reducer.balances

	walletType := ctypes.GetWalletType(exchange, symbol.Type)

	key := ctypes.AssetKey{
		Exchange:   exchange,
		WalletType: walletType,
		Asset:      asset,
	}
	balance, ok := snap[key]
	if !ok {
		return &ctypes.AssetBo{}, nil
	}

	return &ctypes.AssetBo{
		Code:    asset,
		Balance: balance.Free,
		Locked:  balance.Frozen,
	}, nil
}

// GetPositions 获取仓位
func (p *Portfolio) GetPositions(exchange ctypes.Exchange, symbol *ctypes.Symbol) ([]*ctypes.Position, error) {
	if symbol == nil {
		return nil, fmt.Errorf("symbol is required")
	}
	if exchange != p.exchange {
		return []*ctypes.Position{}, nil
	}

	exSymbol := ctypes.NewExSymbol(exchange, *symbol)
	if !exSymbol.IsValid() {
		return nil, fmt.Errorf("invalid exchange/symbol: %s", exSymbol.String())
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		// 现货：底层不维护 PositionSignal/PositionView；对 JS/策略层提供统一"仓位"视图
		// 这里约定：现货交易对的 position = base 资产总持有量（Free+Frozen），Side 固定为 LONG。
		walletType := ctypes.GetWalletType(exchange, symbol.Type)
		key := ctypes.AssetKey{
			Exchange:   exchange,
			WalletType: walletType,
			Asset:      symbol.Base,
		}
		baseBal, ok := p.reducer.balances[key]
		if !ok {
			return []*ctypes.Position{}, nil
		}
		qty := baseBal.Free.Add(baseBal.Frozen)
		if qty.IsZero() {
			return []*ctypes.Position{}, nil
		}

		// 现货不依赖杠杆，直接暴露基础数量即可
		tradableQty := qty

		// 忽略微小仓位
		market, err := p.marketProvider.GetMarket(context.Background(), exchange, *symbol)
		if err == nil && market != nil {
			minQty := market.Rules.MinQuantity
			if minQty.IsZero() && market.BaseAssetPrecision > 0 {
				minQty = decimal.NewFromInt(1).Div(decimal.NewFromInt(10).Pow(decimal.NewFromInt(int64(market.BaseAssetPrecision))))
			}
			if tradableQty.LessThan(minQty) {
				return []*ctypes.Position{}, nil
			}
		}

		return []*ctypes.Position{
			{
				Symbol:     *symbol,
				Side:       ctypes.PositionSideLong,
				Leverage:   1,
				Amount:     tradableQty,
				EntryPrice: decimal.Zero, // 现货成本价需要单独的成本跟踪；此处不做推导
			},
		}, nil
	case ctypes.MarketTypeFuture:
		result := make([]*ctypes.Position, 0)

		leverage := p.GetLocalLeverage(exchange, exSymbol.Symbol)

		longPos := p.reducer.positions[ctypes.PositionKey{
			ExSymbol: exSymbol,
			Side:     ctypes.PositionSideLong,
		}]

		if !longPos.Qty.IsZero() {
			result = append(result, &ctypes.Position{
				Symbol:     exSymbol.Symbol,
				Side:       longPos.Side,
				Amount:     longPos.Qty,
				Leverage:   leverage,
				EntryPrice: longPos.AvgPrice,
			})
		}

		shortPos := p.reducer.positions[ctypes.PositionKey{
			ExSymbol: exSymbol,
			Side:     ctypes.PositionSideShort,
		}]
		if !shortPos.Qty.IsZero() {
			result = append(result, &ctypes.Position{
				Symbol:     exSymbol.Symbol,
				Side:       shortPos.Side,
				Amount:     shortPos.Qty,
				Leverage:   leverage,
				EntryPrice: shortPos.AvgPrice,
			})
		}

		return result, nil
	default:
		// 其他市场类型：暂按无持仓处理（避免误导）
		return []*ctypes.Position{}, nil
	}
}

// GetLeverage 获取杠杆配置
// - 优先使用本地缓存；如果缓存没有且配置了 AccountEngine，则调用 AccountEngine.GetLeverage 兜底并写入缓存
func (p *Portfolio) GetLeverage(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error) {
	if exchange != p.exchange {
		return 1, nil
	}

	exSymbol := ctypes.NewExSymbol(exchange, symbol)
	if !exSymbol.IsValid() {
		return 0, fmt.Errorf("invalid exchange/symbol: %s", exSymbol.String())
	}

	p.mu.RLock()
	lv, ok := p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideLong}]
	if !ok {
		lv, ok = p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideShort}]
	}
	engine := p.accountEngine
	accountID := p.accountID
	p.mu.RUnlock()

	if ok {
		return lv, nil
	}

	if engine == nil || accountID == "" {
		return 0, fmt.Errorf("leverage not found in portfolio cache and account engine is not configured")
	}

	lev, err := engine.GetLeverage(ctx, accountID, symbol)
	if err != nil {
		return 0, err
	}

	p.mu.Lock()
	p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideLong}] = lev
	p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideShort}] = lev
	p.mu.Unlock()

	return lev, nil
}

// SetLeverage 设置杠杆配置
// - 通过 AccountEngine 设置真实账户杠杆
// - 同时更新本地缓存（也可以依赖上游发布 LeverageChangedSignal 再更新）
func (p *Portfolio) SetLeverage(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error {
	if exchange != p.exchange {
		return nil
	}

	if leverage <= 0 {
		return fmt.Errorf("leverage must be positive")
	}

	p.mu.RLock()
	engine := p.accountEngine
	accountID := p.accountID
	p.mu.RUnlock()

	if engine == nil || accountID == "" {
		return fmt.Errorf("account engine is not configured for portfolio")
	}

	if err := engine.SetLeverage(ctx, accountID, symbol, leverage); err != nil {
		return err
	}

	exSymbol := ctypes.NewExSymbol(exchange, symbol)
	if !exSymbol.IsValid() {
		return nil
	}

	p.mu.Lock()
	p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideLong}] = leverage
	p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideShort}] = leverage
	p.mu.Unlock()

	return nil
}

// buildRiskState 从 portfolio 快照构建风险状态（供 state 为 nil 时使用）
func (p *Portfolio) BuildRiskState() *stypes.RiskState {
	snap := p.Snapshot()
	state := &stypes.RiskState{
		Exchange:   p.exchange,
		Positions:  make([]*ctypes.Position, 0),
		Assets:     make([]*ctypes.AssetBo, 0),
		DailyPnL:   decimal.Zero,
		OpenOrders: make(map[ctypes.OrderId]*ctypes.Order),
	}
	for ak, av := range snap.Balances {
		state.Assets = append(state.Assets, &ctypes.AssetBo{
			AccountID:  p.accountID,
			WalletType: ak.WalletType,
			Code:       ak.Asset,
			Balance:    av.Free,
			Locked:     av.Frozen,
			Notional:   av.Free.Add(av.Frozen),
			UpdatedTs:  time.Unix(av.UpdateAt, 0),
		})
	}
	for pk, pv := range snap.Positions {
		lev := p.GetLocalLeverage(pk.ExSymbol.Exchange, pk.ExSymbol.Symbol)
		notional := pv.Qty.Mul(pv.AvgPrice)
		if pv.Side == ctypes.PositionSideShort {
			notional = notional.Neg()
		}
		state.Positions = append(state.Positions, &ctypes.Position{
			Exchange:   pk.ExSymbol.Exchange,
			Symbol:     pk.ExSymbol.Symbol,
			Side:       pv.Side,
			Amount:     pv.Qty,
			EntryPrice: pv.AvgPrice,
			Notional:   notional,
			Leverage:   lev,
		})
	}
	return state
}

func (p *Portfolio) GetLocalLeverage(exchange ctypes.Exchange, symbol ctypes.Symbol) int {
	if exchange != p.exchange {
		return 1
	}

	exSymbol := ctypes.NewExSymbol(exchange, symbol)
	if !exSymbol.IsValid() {
		return 1
	}

	lev, ok := p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideLong}]
	if !ok {
		lev, ok = p.reducer.leverages[ctypes.PositionKey{ExSymbol: exSymbol, Side: ctypes.PositionSideShort}]
	}
	if !ok {
		return 1
	}
	return lev
}
