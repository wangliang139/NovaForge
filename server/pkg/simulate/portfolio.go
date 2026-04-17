package simulate

import (
	"sync"

	"github.com/shopspring/decimal"
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

	balances map[Asset]decimal.Decimal

	positions map[Symbol]*Position
}

// NewPortfolio creates an empty portfolio.
func NewPortfolio() *Portfolio {
	return &Portfolio{
		balances:  make(map[Asset]decimal.Decimal),
		positions: make(map[Symbol]*Position),
	}
}

// SetBalance sets absolute balance for an asset (initialization / deposit).
func (p *Portfolio) SetBalance(a Asset, v decimal.Decimal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.balances[a] = v
}

// Balances returns a snapshot of balances (including zeros for known keys only if set).
func (p *Portfolio) Balances() map[Asset]decimal.Decimal {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[Asset]decimal.Decimal, len(p.balances))
	for k, v := range p.balances {
		out[k] = v
	}
	return out
}

// Balance returns balance for asset.
func (p *Portfolio) Balance(a Asset) decimal.Decimal {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.balances[a]
}

func (p *Portfolio) addBal(a Asset, v decimal.Decimal) {
	p.balances[a] = p.balances[a].Add(v)
}

func (p *Portfolio) subBal(a Asset, v decimal.Decimal) error {
	cur := p.balances[a]
	if cur.Sub(v).Sign() < 0 {
		return ErrInsufficientBalance
	}
	p.balances[a] = cur.Sub(v)
	return nil
}

// Position returns a copy of the perp position for symbol.
func (p *Portfolio) Position(sym Symbol) (Position, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pos, ok := p.positions[sym]
	if !ok {
		return Position{}, false
	}
	return *pos, true
}

func (p *Portfolio) posPtr(sym Symbol) *Position {
	if pos, ok := p.positions[sym]; ok {
		return pos
	}
	pos := &Position{
		Qty:        decimal.Zero,
		EntryPrice: decimal.Zero,
		UsedMargin: decimal.Zero,
		Leverage:   1,
	}
	p.positions[sym] = pos
	return pos
}

// ApplySpotBuy spends quote (incl. fee), receives base.
func (p *Portfolio) ApplySpotBuy(ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
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
	if err := p.subBal(ins.Quote, quote.Add(feeQuote)); err != nil {
		return err
	}
	p.addBal(ins.Base, base)
	return nil
}

// ApplySpotSell spends base, receives quote minus fee.
func (p *Portfolio) ApplySpotSell(ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
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
	if err := p.subBal(ins.Base, base); err != nil {
		return err
	}
	p.addBal(ins.Quote, quote.Sub(feeQuote))
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
func (p *Portfolio) ApplyPerpOpenLong(sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, lev int32) error {
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
	if err := p.subBal(ins.Quote, im.Add(feeQuote)); err != nil {
		return err
	}
	pos := p.posPtr(sym)
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
func (p *Portfolio) ApplyPerpOpenShort(sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, lev int32) error {
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
	if err := p.subBal(ins.Quote, im.Add(feeQuote)); err != nil {
		return err
	}
	pos := p.posPtr(sym)
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
func (p *Portfolio) ApplyPerpCloseLong(sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
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
	pos := p.posPtr(sym)
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
	p.addBal(ins.Quote, realized.Sub(feeQuote).Add(rel))
	return nil
}

// ApplyPerpCloseShort applies buy fills closing a short.
func (p *Portfolio) ApplyPerpCloseShort(sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
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
	pos := p.posPtr(sym)
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
	p.addBal(ins.Quote, realized.Sub(feeQuote).Add(rel))
	return nil
}
