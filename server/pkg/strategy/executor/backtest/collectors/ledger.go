package collectors

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type ledgerBalanceKey struct {
	accountID  string
	exchange   ctypes.Exchange
	asset      string
	walletType ctypes.WalletType
}

type ledgerBalanceState struct {
	seen   bool
	total  decimal.Decimal
	frozen decimal.Decimal
}

// LedgerCollector 记录回测资金流水（对齐实盘 Ledger 字段，便于前端复用 LedgersTable）。
type LedgerCollector struct {
	mu sync.Mutex

	seq      int64
	entries  []*ctypes.Ledger
	balances map[ledgerBalanceKey]ledgerBalanceState

	lastFill struct {
		accountID string
		orderID   string
		symbol    string
		ts        time.Time
	}
}

func NewLedgerCollector() *LedgerCollector {
	return &LedgerCollector{
		entries:  make([]*ctypes.Ledger, 0, 256),
		balances: make(map[ledgerBalanceKey]ledgerBalanceState),
	}
}

func (c *LedgerCollector) OnBalanceDelta(e *stypes.BalanceDeltaSignal) {
	if e == nil || e.GetExchange() == nil || e.GetSymbol() == nil {
		return
	}
	accountID := ""
	if e.GetAccountID() != nil {
		accountID = *e.GetAccountID()
	}
	freeDelta := e.Free
	frozenDelta := e.Frozen
	totalDelta := freeDelta.Add(frozenDelta)
	if totalDelta.IsZero() && frozenDelta.IsZero() {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := ledgerBalanceKey{
		accountID:  accountID,
		exchange:   *e.GetExchange(),
		asset:      e.Asset,
		walletType: e.WalletType,
	}
	prev := c.balances[key]
	nextTotal := prev.total.Add(totalDelta)
	nextFrozen := prev.frozen.Add(frozenDelta)
	if nextFrozen.LessThan(decimal.Zero) {
		nextFrozen = decimal.Zero
	}
	c.balances[key] = ledgerBalanceState{seen: true, total: nextTotal, frozen: nextFrozen}

	detail := c.buildDetail(accountID, e.GetSymbol().String(), ctypes.LedgerReasonFill, "")
	c.appendEntry(accountID, *e.GetExchange(), e.Asset, e.WalletType, e.GetTimestamp(), nextTotal, nextFrozen, totalDelta, frozenDelta, ctypes.LedgerReasonFill, detail)
}

func (c *LedgerCollector) OnBalanceSnapshot(e *stypes.BalanceSignal) {
	if e == nil || e.GetExchange() == nil || e.GetSymbol() == nil {
		return
	}
	accountID := ""
	if e.GetAccountID() != nil {
		accountID = *e.GetAccountID()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := ledgerBalanceKey{
		accountID:  accountID,
		exchange:   *e.GetExchange(),
		asset:      e.Asset,
		walletType: e.WalletType,
	}
	prev := c.balances[key]
	nextTotal := e.Free.Add(e.Frozen)
	nextFrozen := e.Frozen

	totalDelta := nextTotal
	frozenDelta := nextFrozen
	if prev.seen {
		totalDelta = nextTotal.Sub(prev.total)
		frozenDelta = nextFrozen.Sub(prev.frozen)
	}

	reason := classifySnapshotReason(prev, nextTotal, nextFrozen, *e.GetSymbol())
	c.balances[key] = ledgerBalanceState{seen: true, total: nextTotal, frozen: nextFrozen}

	if totalDelta.IsZero() && frozenDelta.IsZero() && reason == ctypes.LedgerReasonSnapshot && prev.seen {
		return
	}

	detail := c.buildDetail(accountID, e.GetSymbol().String(), reason, "")
	c.appendEntry(accountID, *e.GetExchange(), e.Asset, e.WalletType, e.GetTimestamp(), nextTotal, nextFrozen, totalDelta, frozenDelta, reason, detail)
}

func (c *LedgerCollector) OnFill(e *stypes.FillSignal) {
	if e == nil || e.GetAccountID() == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastFill.accountID = *e.GetAccountID()
	c.lastFill.orderID = string(e.OrderID)
	if e.GetSymbol() != nil {
		c.lastFill.symbol = e.GetSymbol().String()
	}
	c.lastFill.ts = e.GetTimestamp()
}

func (c *LedgerCollector) GetLedgers() []*ctypes.Ledger {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*ctypes.Ledger, len(c.entries))
	copy(out, c.entries)
	return out
}

func (c *LedgerCollector) appendEntry(
	accountID string,
	exchange ctypes.Exchange,
	asset string,
	walletType ctypes.WalletType,
	ts time.Time,
	total, frozen, totalDelta, frozenDelta decimal.Decimal,
	reason ctypes.LedgerReason,
	detail json.RawMessage,
) {
	c.seq++
	entry := &ctypes.Ledger{
		ID:          c.seq,
		AccountID:   accountID,
		Exchange:    exchange,
		Asset:       asset,
		WalletType:  walletType,
		Total:       total,
		Frozen:      frozen,
		TotalDelta:  totalDelta,
		FrozenDelta: frozenDelta,
		Type:        reason,
		Detail:      detail,
		IsEffective: true,
		Ts:          ts,
		CreatedAt:   ts,
	}
	c.entries = append(c.entries, entry)
}

func (c *LedgerCollector) buildDetail(accountID, symbol string, reason ctypes.LedgerReason, orderID string) json.RawMessage {
	payload := map[string]string{
		"source": "backtest",
	}
	if symbol != "" {
		payload["symbol"] = symbol
	}
	if orderID == "" && reason == ctypes.LedgerReasonFill && c.lastFill.accountID == accountID {
		if c.lastFill.symbol != "" {
			payload["symbol"] = c.lastFill.symbol
		}
		orderID = c.lastFill.orderID
	}
	if orderID != "" {
		payload["orderId"] = orderID
	}
	b, _ := json.Marshal(payload)
	return b
}

func classifySnapshotReason(prev ledgerBalanceState, nextTotal, nextFrozen decimal.Decimal, sym ctypes.Symbol) ctypes.LedgerReason {
	if !prev.seen {
		return ctypes.LedgerReasonSnapshot
	}
	totalDelta := nextTotal.Sub(prev.total)
	frozenDelta := nextFrozen.Sub(prev.frozen)
	if frozenDelta.GreaterThan(decimal.Zero) && totalDelta.IsZero() {
		if sym.Type == ctypes.MarketTypeFuture {
			return ctypes.LedgerReasonOrderMarginFreeze
		}
		return ctypes.LedgerReasonFundsFreeze
	}
	if frozenDelta.LessThan(decimal.Zero) && totalDelta.IsZero() {
		if sym.Type == ctypes.MarketTypeFuture {
			return ctypes.LedgerReasonOrderMarginUnfreeze
		}
		return ctypes.LedgerReasonFundsUnfreeze
	}
	return ctypes.LedgerReasonSnapshot
}
