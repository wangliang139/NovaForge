package simulate

import (
	"sync"

	"github.com/shopspring/decimal"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// Position is a one-way net position (positive long, negative short).
type Position struct {
	Qty        decimal.Decimal
	EntryPrice decimal.Decimal
	UsedMargin decimal.Decimal
	Leverage   int32
}

// Portfolio holds spot balances and isolated per-symbol perp state.
type Portfolio struct {
	mu sync.Mutex

	accounts map[string]*AccountState
}

// BalanceKey identifies a wallet bucket + asset code. The wallet dimension is exchange-specific:
// routing uses types.GetWalletType(exchange, marketType) — e.g. OKX maps both spot and futures to
// WalletTypeTrade (one shared bucket); Binance keeps spot vs futures in separate buckets.
type BalanceKey struct {
	Wallet ctypes.WalletType
	Asset  Asset
}

type AccountState struct {
	balances  map[ctypes.WalletType]map[Asset]decimal.Decimal
	positions map[Symbol]*Position
}

// NewPortfolio creates an empty portfolio.
func NewPortfolio() *Portfolio {
	return &Portfolio{
		accounts: make(map[string]*AccountState),
	}
}

// SetBalance sets absolute balance for an asset (initialization / deposit).
func (p *Portfolio) SetBalance(accountID string, wt ctypes.WalletType, a Asset, v decimal.Decimal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	m := acc.balances[wt]
	if m == nil {
		m = make(map[Asset]decimal.Decimal)
		acc.balances[wt] = m
	}
	m[a] = v
}

// Balances returns a snapshot of balances (including zeros for known keys only if set).
func (p *Portfolio) Balances(accountID string) map[BalanceKey]decimal.Decimal {
	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	out := make(map[BalanceKey]decimal.Decimal)
	for wt, m := range acc.balances {
		for a, v := range m {
			out[BalanceKey{Wallet: wt, Asset: a}] = v
		}
	}
	return out
}

// Balance returns balance for asset in a wallet bucket.
func (p *Portfolio) Balance(accountID string, wt ctypes.WalletType, a Asset) decimal.Decimal {
	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	if acc.balances[wt] == nil {
		return decimal.Zero
	}
	return acc.balances[wt][a]
}

func (p *Portfolio) addBal(acc *AccountState, wt ctypes.WalletType, a Asset, v decimal.Decimal) {
	if acc.balances[wt] == nil {
		acc.balances[wt] = make(map[Asset]decimal.Decimal)
	}
	m := acc.balances[wt]
	m[a] = m[a].Add(v)
}

func (p *Portfolio) subBal(acc *AccountState, wt ctypes.WalletType, a Asset, v decimal.Decimal) error {
	if acc.balances[wt] == nil {
		acc.balances[wt] = make(map[Asset]decimal.Decimal)
	}
	m := acc.balances[wt]
	cur := m[a]
	if cur.Sub(v).Sign() < 0 {
		return ErrInsufficientBalance
	}
	m[a] = cur.Sub(v)
	return nil
}

// Position returns a copy of the perp position for symbol.
func (p *Portfolio) Position(accountID string, sym Symbol) (Position, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	pos, ok := acc.positions[sym]
	if !ok {
		return Position{}, false
	}
	return *pos, true
}

func (p *Portfolio) posPtr(acc *AccountState, sym Symbol) *Position {
	if pos, ok := acc.positions[sym]; ok {
		return pos
	}
	pos := &Position{
		Qty:        decimal.Zero,
		EntryPrice: decimal.Zero,
		UsedMargin: decimal.Zero,
		Leverage:   1,
	}
	acc.positions[sym] = pos
	return pos
}

func (p *Portfolio) ensureAccount(accountID string) *AccountState {
	if accountID == "" {
		accountID = "default"
	}
	if acc, ok := p.accounts[accountID]; ok {
		return acc
	}
	acc := &AccountState{
		balances:  make(map[ctypes.WalletType]map[Asset]decimal.Decimal),
		positions: make(map[Symbol]*Position),
	}
	p.accounts[accountID] = acc
	return acc
}

func (p *Portfolio) SetPosition(accountID string, sym Symbol, pos Position) {
	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	cp := pos
	acc.positions[sym] = &cp
}

func (p *Portfolio) RemoveAccount(accountID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.accounts, accountID)
}

// ApplySpotBuy spends quote (incl. fee), receives base.
func (p *Portfolio) ApplySpotBuy(accountID string, ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
	if len(fills) == 0 {
		return nil
	}
	var base, quote decimal.Decimal
	for _, f := range fills {
		base = base.Add(f.Size)
		quote = quote.Add(f.Price.Mul(f.Size))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	wt := ins.WalletType()
	if err := p.subBal(acc, wt, ins.Quote, quote.Add(feeQuote)); err != nil {
		return err
	}
	p.addBal(acc, wt, ins.Base, base)
	return nil
}

// ApplySpotSell spends base, receives quote minus fee.
func (p *Portfolio) ApplySpotSell(accountID string, ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
	if len(fills) == 0 {
		return nil
	}
	var base, quote decimal.Decimal
	for _, f := range fills {
		base = base.Add(f.Size)
		quote = quote.Add(f.Price.Mul(f.Size))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	wt := ins.WalletType()
	if err := p.subBal(acc, wt, ins.Base, base); err != nil {
		return err
	}
	p.addBal(acc, wt, ins.Quote, quote.Sub(feeQuote))
	return nil
}

// InitialMargin returns abs(qty)*price / leverage (linear quote margin).
func InitialMargin(qty, price decimal.Decimal, lev int32) decimal.Decimal {
	if lev <= 0 {
		lev = 1
	}
	return qty.Abs().Mul(price).Div(decimal.NewFromInt32(lev))
}

func releaseUsedMargin(pos *Position, closedQtyAbs, oldQtyAbs decimal.Decimal) decimal.Decimal {
	if oldQtyAbs.IsZero() {
		return decimal.Zero
	}
	return pos.UsedMargin.Mul(closedQtyAbs).Div(oldQtyAbs)
}

// weightedAvgEntryLong returns new average entry for adding size to a long.
func weightedAvgEntryLong(oldQty, oldEntry, addQty, addPrice decimal.Decimal) decimal.Decimal {
	if oldQty.Sign() <= 0 {
		return addPrice
	}
	num := oldEntry.Mul(oldQty).Add(addPrice.Mul(addQty))
	den := oldQty.Add(addQty)
	if den.IsZero() {
		return decimal.Zero
	}
	return num.Div(den)
}

// weightedAvgEntryShort returns new average entry magnitude for adding size to a short (qty negative).
func weightedAvgEntryShort(oldQtyAbs, oldEntry, addQty, addPrice decimal.Decimal) decimal.Decimal {
	if oldQtyAbs.IsZero() {
		return addPrice
	}
	num := oldEntry.Mul(oldQtyAbs).Add(addPrice.Mul(addQty))
	den := oldQtyAbs.Add(addQty)
	if den.IsZero() {
		return decimal.Zero
	}
	return num.Div(den)
}

// ApplyPerpOpenLong adds buy fills to a long or reduces short then errors if would flip (minimal: expect flat/long).
func (p *Portfolio) ApplyPerpOpenLong(accountID string, sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, lev int32) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avg := AveragePrice(notional, filledQty)
	im := InitialMargin(filledQty, avg, lev)

	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	wt := ins.WalletType()
	if err := p.subBal(acc, wt, ins.Quote, im.Add(feeQuote)); err != nil {
		return err
	}
	pos := p.posPtr(acc, sym)
	if pos.Qty.Sign() < 0 {
		return ErrInvalidIntent
	}
	pos.EntryPrice = weightedAvgEntryLong(pos.Qty, pos.EntryPrice, filledQty, avg)
	pos.Qty = pos.Qty.Add(filledQty)
	pos.UsedMargin = pos.UsedMargin.Add(im)
	pos.Leverage = lev
	return nil
}

// ApplyPerpOpenShort adds sell fills to a short (negative qty). Rejects if existing long.
func (p *Portfolio) ApplyPerpOpenShort(accountID string, sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, lev int32) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avg := AveragePrice(notional, filledQty)
	im := InitialMargin(filledQty, avg, lev)

	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	wt := ins.WalletType()
	if err := p.subBal(acc, wt, ins.Quote, im.Add(feeQuote)); err != nil {
		return err
	}
	pos := p.posPtr(acc, sym)
	if pos.Qty.Sign() > 0 {
		return ErrInvalidIntent
	}
	oldAbs := pos.Qty.Abs()
	pos.EntryPrice = weightedAvgEntryShort(oldAbs, pos.EntryPrice, filledQty, avg)
	pos.Qty = pos.Qty.Sub(filledQty)
	pos.UsedMargin = pos.UsedMargin.Add(im)
	pos.Leverage = lev
	return nil
}

// ApplyPerpCloseLong applies sell fills closing a long: realizes PnL in quote, releases margin.
func (p *Portfolio) ApplyPerpCloseLong(accountID string, sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avgExit := AveragePrice(notional, filledQty)

	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	wt := ins.WalletType()
	pos := p.posPtr(acc, sym)
	if pos.Qty.Sign() <= 0 {
		return ErrInvalidIntent
	}
	if filledQty.GreaterThan(pos.Qty) {
		return ErrReduceOnly
	}
	oldQty := pos.Qty
	rel := releaseUsedMargin(pos, filledQty, oldQty)
	realized := avgExit.Sub(pos.EntryPrice).Mul(filledQty)
	pos.Qty = pos.Qty.Sub(filledQty)
	pos.UsedMargin = pos.UsedMargin.Sub(rel)
	if pos.Qty.IsZero() {
		pos.EntryPrice = decimal.Zero
	}
	p.addBal(acc, wt, ins.Quote, realized.Sub(feeQuote).Add(rel))
	return nil
}

// ApplyPerpCloseShort applies buy fills closing a short.
func (p *Portfolio) ApplyPerpCloseShort(accountID string, sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avgExit := AveragePrice(notional, filledQty)

	p.mu.Lock()
	defer p.mu.Unlock()
	acc := p.ensureAccount(accountID)
	wt := ins.WalletType()
	pos := p.posPtr(acc, sym)
	if pos.Qty.Sign() >= 0 {
		return ErrInvalidIntent
	}
	shortAbs := pos.Qty.Abs()
	if filledQty.GreaterThan(shortAbs) {
		return ErrReduceOnly
	}
	rel := releaseUsedMargin(pos, filledQty, shortAbs)
	// short PnL: (entry - exit) * qty
	realized := pos.EntryPrice.Sub(avgExit).Mul(filledQty)
	pos.Qty = pos.Qty.Add(filledQty)
	pos.UsedMargin = pos.UsedMargin.Sub(rel)
	if pos.Qty.IsZero() {
		pos.EntryPrice = decimal.Zero
	}
	p.addBal(acc, wt, ins.Quote, realized.Sub(feeQuote).Add(rel))
	return nil
}
