package simulate

import (
	"sync"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// BalanceKey identifies a wallet bucket + asset code.
type BalanceKey struct {
	Wallet ctypes.WalletType
	Asset  Asset
}

// Position is a one-way net position (positive long, negative short).
type Position struct {
	Qty        decimal.Decimal
	EntryPrice decimal.Decimal
	UsedMargin decimal.Decimal
	Leverage   int32
}

// PerpLeg is one side of a hedge position (qty >= 0).
type PerpLeg struct {
	Qty        decimal.Decimal
	EntryPrice decimal.Decimal
	UsedMargin decimal.Decimal
	Leverage   int32
}

// PerpSlot holds perp state for one (account, symbol).
type PerpSlot struct {
	Mode PositionMode
	Net  Position     // one-way
	Long PerpLeg      // hedge
	Short PerpLeg
}

type accountState struct {
	balances  map[ctypes.WalletType]map[Asset]decimal.Decimal
	perpSlots map[Symbol]*PerpSlot
}

// Ledger holds spot balances and perp slots (one-way or hedge per account config).
type Ledger struct {
	mu       sync.Mutex
	accounts map[string]*accountState
}

// NewLedger creates an empty ledger.
func NewLedger() *Ledger {
	return &Ledger{accounts: make(map[string]*accountState)}
}

func (l *Ledger) ensureAccount(accountID string) *accountState {
	if accountID == "" {
		accountID = "default"
	}
	if acc, ok := l.accounts[accountID]; ok {
		return acc
	}
	acc := &accountState{
		balances:  make(map[ctypes.WalletType]map[Asset]decimal.Decimal),
		perpSlots: make(map[Symbol]*PerpSlot),
	}
	l.accounts[accountID] = acc
	return acc
}

// ApplyQuoteDelta adds delta to the quote asset in a wallet (negative debits). Used for perp funding; no balance floor check.
func (l *Ledger) ApplyQuoteDelta(accountID string, wt ctypes.WalletType, quote Asset, delta decimal.Decimal) {
	if delta.IsZero() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	l.addBal(acc, wt, quote, delta)
}

// ListAccountIDs returns account ids that have ledger state (balances or perp slots).
func (l *Ledger) ListAccountIDs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.accounts))
	for id := range l.accounts {
		out = append(out, id)
	}
	return out
}

// SetBalance sets absolute balance for an asset (initialization / deposit).
func (l *Ledger) SetBalance(accountID string, wt ctypes.WalletType, a Asset, v decimal.Decimal) {
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	m := acc.balances[wt]
	if m == nil {
		m = make(map[Asset]decimal.Decimal)
		acc.balances[wt] = m
	}
	m[a] = v
}

// Balances returns a snapshot of balances.
func (l *Ledger) Balances(accountID string) map[BalanceKey]decimal.Decimal {
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	out := make(map[BalanceKey]decimal.Decimal)
	for wt, m := range acc.balances {
		for a, v := range m {
			out[BalanceKey{Wallet: wt, Asset: a}] = v
		}
	}
	return out
}

func (l *Ledger) addBal(acc *accountState, wt ctypes.WalletType, a Asset, v decimal.Decimal) {
	if acc.balances[wt] == nil {
		acc.balances[wt] = make(map[Asset]decimal.Decimal)
	}
	m := acc.balances[wt]
	m[a] = m[a].Add(v)
}

func (l *Ledger) subBal(acc *accountState, wt ctypes.WalletType, a Asset, v decimal.Decimal) error {
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

// EnsurePerpSlot returns the slot for symbol, initializing mode if missing.
func (l *Ledger) EnsurePerpSlot(accountID string, sym Symbol, mode PositionMode) *PerpSlot {
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	s, ok := acc.perpSlots[sym]
	if !ok {
		s = &PerpSlot{Mode: mode}
		acc.perpSlots[sym] = s
		return s
	}
	return s
}

// SetPerpMode sets position mode for (account, symbol); both legs/net must be flat when switching.
func (l *Ledger) SetPerpMode(accountID string, sym Symbol, mode PositionMode) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	s, ok := acc.perpSlots[sym]
	if !ok {
		acc.perpSlots[sym] = &PerpSlot{Mode: mode}
		return nil
	}
	if !s.Net.Qty.IsZero() || s.Long.Qty.Sign() > 0 || s.Short.Qty.Sign() > 0 {
		return ErrPositionMode
	}
	s.Mode = mode
	return nil
}

// GetPerpSlot returns a copy-safe read: caller must not mutate (lock held internally for copy).
func (l *Ledger) GetPerpSlot(accountID string, sym Symbol) (PerpSlot, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	s, ok := acc.perpSlots[sym]
	if !ok {
		return PerpSlot{}, false
	}
	cp := *s
	cp.Net = s.Net
	cp.Long = s.Long
	cp.Short = s.Short
	return cp, true
}

func (l *Ledger) perpSlotPtr(acc *accountState, sym Symbol, mode PositionMode) *PerpSlot {
	s, ok := acc.perpSlots[sym]
	if !ok {
		s = &PerpSlot{Mode: mode}
		acc.perpSlots[sym] = s
	}
	return s
}

// Position returns one-way net position (legacy / one-way mode).
func (l *Ledger) Position(accountID string, sym Symbol) (Position, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	s, ok := acc.perpSlots[sym]
	if !ok || s.Mode != PositionModeOneWay {
		if !ok {
			return Position{}, false
		}
		if s.Net.Qty.IsZero() {
			return Position{}, false
		}
		return s.Net, true
	}
	if s.Net.Qty.IsZero() {
		return Position{}, false
	}
	return s.Net, true
}

// RemoveAccount clears account state.
func (l *Ledger) RemoveAccount(accountID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.accounts, accountID)
}

// SeedOneWayNet sets net perp position (one-way mode); clears hedge legs.
func (l *Ledger) SeedOneWayNet(accountID string, sym Symbol, pos Position) {
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	s := acc.perpSlots[sym]
	if s == nil {
		s = &PerpSlot{Mode: PositionModeOneWay}
		acc.perpSlots[sym] = s
	}
	s.Mode = PositionModeOneWay
	s.Net = pos
	s.Long = PerpLeg{}
	s.Short = PerpLeg{}
}

// MergeSeedHedgeLeg sets one hedge leg without clearing the other.
func (l *Ledger) MergeSeedHedgeLeg(accountID string, sym Symbol, side ctypes.PositionSide, leg PerpLeg) {
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	s := acc.perpSlots[sym]
	if s == nil {
		s = &PerpSlot{Mode: PositionModeHedge}
		acc.perpSlots[sym] = s
	}
	s.Mode = PositionModeHedge
	s.Net = Position{}
	switch side {
	case ctypes.PositionSideLong:
		s.Long = leg
	case ctypes.PositionSideShort:
		s.Short = leg
	}
}

func slotUsedMarginSum(s *PerpSlot) decimal.Decimal {
	if s == nil {
		return decimal.Zero
	}
	if s.Mode == PositionModeHedge {
		return s.Long.UsedMargin.Add(s.Short.UsedMargin)
	}
	return s.Net.UsedMargin
}

func initialMargin(qty, price decimal.Decimal, lev int32) decimal.Decimal {
	if lev <= 0 {
		lev = int32(DefaultSimulateLeverage)
	}
	return qty.Abs().Mul(price).Div(decimal.NewFromInt32(lev))
}

// SyncPerpSlotLeverage updates leg leverage fields and rescales booked initial margin.
// If the new requirement exceeds available collateral, returns ErrInsufficientBalance.
func (l *Ledger) SyncPerpSlotLeverage(accountID string, sym Symbol, ins *Instrument, newLev int32) error {
	if ins == nil || newLev < 1 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	s, ok := acc.perpSlots[sym]
	if !ok || s == nil {
		return nil
	}
	oldSum := slotUsedMarginSum(s)
	ns := *s

	switch ns.Mode {
	case PositionModeHedge:
		if ns.Long.Qty.Sign() > 0 {
			ns.Long.UsedMargin = initialMargin(ns.Long.Qty, ns.Long.EntryPrice, newLev)
		} else {
			ns.Long.UsedMargin = decimal.Zero
		}
		ns.Long.Leverage = newLev

		if ns.Short.Qty.Sign() > 0 {
			ns.Short.UsedMargin = initialMargin(ns.Short.Qty, ns.Short.EntryPrice, newLev)
		} else {
			ns.Short.UsedMargin = decimal.Zero
		}
		ns.Short.Leverage = newLev

	default:
		if ns.Net.Qty.IsZero() {
			ns.Net.UsedMargin = decimal.Zero
		} else {
			ns.Net.UsedMargin = initialMargin(ns.Net.Qty.Abs(), ns.Net.EntryPrice, newLev)
		}
		ns.Net.Leverage = newLev
	}

	newSum := slotUsedMarginSum(&ns)
	delta := newSum.Sub(oldSum)
	if delta.Sign() > 0 {
		wt := ins.WalletType()
		avail := availableQuoteForPerpOpen(acc, wt, ins.Quote)
		if avail.LessThan(delta) {
			return ErrInsufficientBalance
		}
	}
	*s = ns
	return nil
}

func releaseUsedMargin(used, legQty, closedQty decimal.Decimal) decimal.Decimal {
	if legQty.IsZero() {
		return decimal.Zero
	}
	return used.Mul(closedQty).Div(legQty)
}

// sumUsedMarginSlots returns total isolated-style margin booked on all perp slots (USDT-M style: quote collateral).
func sumUsedMarginSlots(acc *accountState) decimal.Decimal {
	var sum decimal.Decimal
	for _, slot := range acc.perpSlots {
		if slot == nil {
			continue
		}
		if slot.Mode == PositionModeHedge {
			sum = sum.Add(slot.Long.UsedMargin).Add(slot.Short.UsedMargin)
			continue
		}
		sum = sum.Add(slot.Net.UsedMargin)
	}
	return sum
}

// availableQuoteForPerpOpen is ledger balance minus margin already tracked on positions (opening does not move IM into wallet subBal).
func availableQuoteForPerpOpen(acc *accountState, wt ctypes.WalletType, quote Asset) decimal.Decimal {
	var bal decimal.Decimal
	if m := acc.balances[wt]; m != nil {
		bal = m[quote]
	}
	return bal.Sub(sumUsedMarginSlots(acc))
}

// QuoteRealizedPnLOneWayCloseLong returns pre-fee realized PnL in quote for fills closing a long (pos snapshot before close).
func QuoteRealizedPnLOneWayCloseLong(pos Position, fills []Fill) decimal.Decimal {
	if len(fills) == 0 || pos.Qty.Sign() <= 0 {
		return decimal.Zero
	}
	var fq, notional decimal.Decimal
	for _, f := range fills {
		fq = fq.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	if fq.IsZero() {
		return decimal.Zero
	}
	avgExit := AveragePrice(notional, fq)
	return avgExit.Sub(pos.EntryPrice).Mul(fq)
}

// QuoteRealizedPnLOneWayCloseShort returns pre-fee realized PnL in quote for fills closing a short (one-way net before close).
func QuoteRealizedPnLOneWayCloseShort(pos Position, fills []Fill) decimal.Decimal {
	if len(fills) == 0 || pos.Qty.Sign() >= 0 {
		return decimal.Zero
	}
	var fq, notional decimal.Decimal
	for _, f := range fills {
		fq = fq.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	if fq.IsZero() {
		return decimal.Zero
	}
	avgExit := AveragePrice(notional, fq)
	return pos.EntryPrice.Sub(avgExit).Mul(fq)
}

// QuoteRealizedPnLHedgeCloseLong returns pre-fee realized PnL for hedge-mode sell closing long leg.
func QuoteRealizedPnLHedgeCloseLong(leg PerpLeg, fills []Fill) decimal.Decimal {
	if len(fills) == 0 || leg.Qty.IsZero() {
		return decimal.Zero
	}
	var fq, notional decimal.Decimal
	for _, f := range fills {
		fq = fq.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	if fq.IsZero() {
		return decimal.Zero
	}
	avgExit := AveragePrice(notional, fq)
	return avgExit.Sub(leg.EntryPrice).Mul(fq)
}

// QuoteRealizedPnLHedgeCloseShort returns pre-fee realized PnL for hedge-mode buy closing short leg.
func QuoteRealizedPnLHedgeCloseShort(leg PerpLeg, fills []Fill) decimal.Decimal {
	if len(fills) == 0 || leg.Qty.IsZero() {
		return decimal.Zero
	}
	var fq, notional decimal.Decimal
	for _, f := range fills {
		fq = fq.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	if fq.IsZero() {
		return decimal.Zero
	}
	avgExit := AveragePrice(notional, fq)
	return leg.EntryPrice.Sub(avgExit).Mul(fq)
}

// --- spot ---

// ApplySpotBuy spends quote for notional only; fee is taken from received base (fee-in-base).
func (l *Ledger) ApplySpotBuy(accountID string, ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
	if len(fills) == 0 {
		return nil
	}
	var base, quote decimal.Decimal
	for _, f := range fills {
		base = base.Add(f.Size)
		quote = quote.Add(f.Price.Mul(f.Size))
	}
	feeBase := SpotFeeBaseFromQuote(quote, base, feeQuote)
	if feeBase.GreaterThan(base) {
		return ErrInsufficientBalance
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	if err := l.subBal(acc, wt, ins.Quote, quote); err != nil {
		return err
	}
	l.addBal(acc, wt, ins.Base, base.Sub(feeBase))
	return nil
}

// ApplySpotSell spends base, receives quote minus fee.
func (l *Ledger) ApplySpotSell(accountID string, ins *Instrument, fills []Fill, feeQuote decimal.Decimal) error {
	if len(fills) == 0 {
		return nil
	}
	var base, quote decimal.Decimal
	for _, f := range fills {
		base = base.Add(f.Size)
		quote = quote.Add(f.Price.Mul(f.Size))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	if err := l.subBal(acc, wt, ins.Base, base); err != nil {
		return err
	}
	l.addBal(acc, wt, ins.Quote, quote.Sub(feeQuote))
	return nil
}

// --- one-way perp (net qty) ---

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

// ApplyPerpOpenLong adds buy fills to a long or rejects if short exists.
func (l *Ledger) ApplyPerpOpenLong(accountID string, sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, lev int32, slot *PerpSlot) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avg := AveragePrice(notional, filledQty)
	im := initialMargin(filledQty, avg, lev)

	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	if availableQuoteForPerpOpen(acc, wt, ins.Quote).LessThan(im.Add(feeQuote)) {
		return ErrInsufficientBalance
	}
	if err := l.subBal(acc, wt, ins.Quote, feeQuote); err != nil {
		return err
	}
	pos := &slot.Net
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
func (l *Ledger) ApplyPerpOpenShort(accountID string, sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, lev int32, slot *PerpSlot) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avg := AveragePrice(notional, filledQty)
	im := initialMargin(filledQty, avg, lev)

	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	if availableQuoteForPerpOpen(acc, wt, ins.Quote).LessThan(im.Add(feeQuote)) {
		return ErrInsufficientBalance
	}
	if err := l.subBal(acc, wt, ins.Quote, feeQuote); err != nil {
		return err
	}
	pos := &slot.Net
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

// ApplyPerpCloseLong applies sell fills closing a long.
func (l *Ledger) ApplyPerpCloseLong(accountID string, sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, slot *PerpSlot) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avgExit := AveragePrice(notional, filledQty)

	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	pos := &slot.Net
	if pos.Qty.Sign() <= 0 {
		return ErrInvalidIntent
	}
	if filledQty.GreaterThan(pos.Qty) {
		return ErrReduceOnly
	}
	oldQty := pos.Qty
	rel := releaseUsedMargin(pos.UsedMargin, oldQty, filledQty)
	realized := avgExit.Sub(pos.EntryPrice).Mul(filledQty)
	pos.Qty = pos.Qty.Sub(filledQty)
	pos.UsedMargin = pos.UsedMargin.Sub(rel)
	if pos.Qty.IsZero() {
		pos.EntryPrice = decimal.Zero
	}
	l.addBal(acc, wt, ins.Quote, realized.Sub(feeQuote))
	return nil
}

// ApplyPerpCloseShort applies buy fills closing a short.
func (l *Ledger) ApplyPerpCloseShort(accountID string, sym Symbol, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, slot *PerpSlot) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avgExit := AveragePrice(notional, filledQty)

	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	pos := &slot.Net
	if pos.Qty.Sign() >= 0 {
		return ErrInvalidIntent
	}
	shortAbs := pos.Qty.Abs()
	if filledQty.GreaterThan(shortAbs) {
		return ErrReduceOnly
	}
	rel := releaseUsedMargin(pos.UsedMargin, shortAbs, filledQty)
	realized := pos.EntryPrice.Sub(avgExit).Mul(filledQty)
	pos.Qty = pos.Qty.Add(filledQty)
	pos.UsedMargin = pos.UsedMargin.Sub(rel)
	if pos.Qty.IsZero() {
		pos.EntryPrice = decimal.Zero
	}
	l.addBal(acc, wt, ins.Quote, realized.Sub(feeQuote))
	return nil
}

// --- hedge legs ---

// ApplyHedgeOpenLong adds to long leg.
func (l *Ledger) ApplyHedgeOpenLong(accountID string, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, lev int32, slot *PerpSlot) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avg := AveragePrice(notional, filledQty)
	im := initialMargin(filledQty, avg, lev)

	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	if availableQuoteForPerpOpen(acc, wt, ins.Quote).LessThan(im.Add(feeQuote)) {
		return ErrInsufficientBalance
	}
	if err := l.subBal(acc, wt, ins.Quote, feeQuote); err != nil {
		return err
	}
	leg := &slot.Long
	leg.EntryPrice = weightedAvgEntryLong(leg.Qty, leg.EntryPrice, filledQty, avg)
	leg.Qty = leg.Qty.Add(filledQty)
	leg.UsedMargin = leg.UsedMargin.Add(im)
	leg.Leverage = lev
	return nil
}

// ApplyHedgeOpenShort adds to short leg.
func (l *Ledger) ApplyHedgeOpenShort(accountID string, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, lev int32, slot *PerpSlot) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avg := AveragePrice(notional, filledQty)
	im := initialMargin(filledQty, avg, lev)

	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	if availableQuoteForPerpOpen(acc, wt, ins.Quote).LessThan(im.Add(feeQuote)) {
		return ErrInsufficientBalance
	}
	if err := l.subBal(acc, wt, ins.Quote, feeQuote); err != nil {
		return err
	}
	leg := &slot.Short
	leg.EntryPrice = weightedAvgEntryLong(leg.Qty, leg.EntryPrice, filledQty, avg)
	leg.Qty = leg.Qty.Add(filledQty)
	leg.UsedMargin = leg.UsedMargin.Add(im)
	leg.Leverage = lev
	return nil
}

// ApplyHedgeCloseLong closes long leg with sell fills.
func (l *Ledger) ApplyHedgeCloseLong(accountID string, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, slot *PerpSlot) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avgExit := AveragePrice(notional, filledQty)

	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	leg := &slot.Long
	if filledQty.GreaterThan(leg.Qty) {
		return ErrReduceOnly
	}
	rel := releaseUsedMargin(leg.UsedMargin, leg.Qty, filledQty)
	realized := avgExit.Sub(leg.EntryPrice).Mul(filledQty)
	leg.Qty = leg.Qty.Sub(filledQty)
	leg.UsedMargin = leg.UsedMargin.Sub(rel)
	if leg.Qty.IsZero() {
		leg.EntryPrice = decimal.Zero
	}
	l.addBal(acc, wt, ins.Quote, realized.Sub(feeQuote))
	return nil
}

// ApplyHedgeCloseShort closes short leg with buy fills.
func (l *Ledger) ApplyHedgeCloseShort(accountID string, ins *Instrument, fills []Fill, feeQuote decimal.Decimal, slot *PerpSlot) error {
	if len(fills) == 0 {
		return nil
	}
	var filledQty, notional decimal.Decimal
	for _, f := range fills {
		filledQty = filledQty.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	avgExit := AveragePrice(notional, filledQty)

	l.mu.Lock()
	defer l.mu.Unlock()
	acc := l.ensureAccount(accountID)
	wt := ins.WalletType()
	leg := &slot.Short
	if filledQty.GreaterThan(leg.Qty) {
		return ErrReduceOnly
	}
	rel := releaseUsedMargin(leg.UsedMargin, leg.Qty, filledQty)
	realized := leg.EntryPrice.Sub(avgExit).Mul(filledQty)
	leg.Qty = leg.Qty.Sub(filledQty)
	leg.UsedMargin = leg.UsedMargin.Sub(rel)
	if leg.Qty.IsZero() {
		leg.EntryPrice = decimal.Zero
	}
	l.addBal(acc, wt, ins.Quote, realized.Sub(feeQuote))
	return nil
}
