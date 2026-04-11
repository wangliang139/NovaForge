package signal

import (
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

func ConvertKlineSignal(base stypes.BaseSignal, k *ctypes.Kline) *stypes.KlineSignal {
	base.Symbol = lo.If(base.Symbol == nil && k.Symbol.IsValid(), &k.Symbol).Else(base.Symbol)
	base.Exchange = lo.If(base.Exchange == nil && k.Exchange.IsValid(), &k.Exchange).Else(base.Exchange)
	// base.Ts = k.CloseTs
	return &stypes.KlineSignal{
		BaseSignal: base,
		Interval:   k.Interval,
		Open:       k.Open,
		High:       k.High,
		Low:        k.Low,
		Close:      k.Close,
		Volume:     k.Volume,
		OpenTs:     k.OpenTs.UnixMilli(),
		IsClosed:   k.IsClosed,
	}
}

func ConvertTickerSignal(base stypes.BaseSignal, t *ctypes.Ticker) *stypes.TickerSignal {
	base.Symbol = lo.If(base.Symbol == nil && t.Symbol.IsValid(), &t.Symbol).Else(base.Symbol)
	base.Exchange = lo.If(base.Exchange == nil && t.Exchange.IsValid(), &t.Exchange).Else(base.Exchange)
	base.Ts = t.Ts
	return &stypes.TickerSignal{
		BaseSignal:    base,
		LastPrice:     t.LastPrice,
		Open24:        t.Open24,
		High24:        t.High24,
		Low24:         t.Low24,
		Avg24:         t.Avg24,
		Volume24:      t.Volume24,
		QuoteVolume24: t.QuoteVolume24,
	}
}

func ConvertDepthSignal(base stypes.BaseSignal, d *ctypes.OrderBook) *stypes.DepthSignal {
	base.Symbol = lo.If(base.Symbol == nil && d.Symbol.IsValid(), &d.Symbol).Else(base.Symbol)
	base.Exchange = lo.If(base.Exchange == nil && d.Exchange.IsValid(), &d.Exchange).Else(base.Exchange)
	base.Ts = d.Ts
	return &stypes.DepthSignal{
		BaseSignal: base,
		OrderBook:  d,
	}
}

func ConvertTradeSignal(base stypes.BaseSignal, t *ctypes.Trade) *stypes.TradeSignal {
	base.Symbol = lo.If(base.Symbol == nil && t.Symbol.IsValid(), &t.Symbol).Else(base.Symbol)
	base.Exchange = lo.If(base.Exchange == nil && t.Exchange.IsValid(), &t.Exchange).Else(base.Exchange)
	base.Ts = t.Ts
	return &stypes.TradeSignal{
		BaseSignal: base,
		TradeID:    t.TradeID,
		Price:      t.Price,
		Size:       t.Size,
		IsBuy:      t.IsBuy,
	}
}

func ConvertMarkPriceSignal(base stypes.BaseSignal, m *ctypes.MarkPrice) *stypes.MarkPriceSignal {
	base.Symbol = lo.If(base.Symbol == nil && m.Symbol.IsValid(), &m.Symbol).Else(base.Symbol)
	base.Exchange = lo.If(base.Exchange == nil && m.Exchange.IsValid(), &m.Exchange).Else(base.Exchange)
	base.Ts = m.Ts
	return &stypes.MarkPriceSignal{
		BaseSignal: base,
		Price:      m.MarkPrice,
	}
}

func ConvertFillSignal(base stypes.BaseSignal, f *ctypes.Fill) *stypes.FillSignal {
	base.Symbol = lo.If(base.Symbol == nil && f.Symbol.IsValid(), &f.Symbol).Else(base.Symbol)
	base.Exchange = lo.If(base.Exchange == nil && f.Exchange.IsValid(), &f.Exchange).Else(base.Exchange)
	base.Ts = f.Ts
	orderID := f.OrderID
	if f.ClientOrderID.String() != "" {
		orderID = f.ClientOrderID
	}
	return &stypes.FillSignal{
		BaseSignal:  base,
		OrderID:     orderID,
		Side:        f.Side,
		IsBuy:       f.IsBuy,
		Qty:         f.Qty,
		Price:       f.Price,
		Fee:         f.Fee,
		Asset:       f.FeeAsset,
		RealizedPnl: f.RealizedPnl,
	}
}

func ConvertLeverageChangedSignal(base stypes.BaseSignal, l *ctypes.SymbolLeverage) *stypes.LeverageChangedSignal {
	base.Symbol = lo.If(base.Symbol == nil && l.Symbol.IsValid(), &l.Symbol).Else(base.Symbol)
	base.Exchange = lo.If(base.Exchange == nil && l.Exchange.IsValid(), &l.Exchange).Else(base.Exchange)
	base.Ts = l.UpdatedTs
	return &stypes.LeverageChangedSignal{
		BaseSignal: base,
		Side:       l.Side,
		Leverage:   l.Leverage,
	}
}

func ConvertOrderSnapshotSignal(base stypes.BaseSignal, o *ctypes.Order) *stypes.OrderSnapshotSignal {
	base.Symbol = lo.If(base.Symbol == nil && o.Symbol.IsValid(), &o.Symbol).Else(base.Symbol)
	base.Exchange = lo.If(base.Exchange == nil && o.Exchange.IsValid(), &o.Exchange).Else(base.Exchange)
	base.Ts = o.UpdatedTs
	orderId := o.OrderID
	if o.ClientOrderID.String() != "" {
		orderId = o.ClientOrderID
	}
	return &stypes.OrderSnapshotSignal{
		BaseSignal: base,
		OrderID:    orderId,
		Order:      o,
	}
}

func ConvertPositionSignal(base stypes.BaseSignal, p *ctypes.Position) *stypes.PositionSignal {
	if p == nil {
		return nil
	}
	base.Symbol = lo.If(base.Symbol == nil && p.Symbol.IsValid(), &p.Symbol).Else(base.Symbol)
	base.Exchange = lo.If(base.Exchange == nil && p.Exchange.IsValid(), &p.Exchange).Else(base.Exchange)
	base.Ts = p.UpdatedTs
	return &stypes.PositionSignal{
		BaseSignal: base,
		Side:       p.Side,
		Qty:        p.Amount,
		EntryPrice: p.EntryPrice,
	}
}

func ConvertBalanceUpdateSignal(base stypes.BaseSignal, u *ctypes.BalanceUpdate) []stypes.Signal {
	out := make([]stypes.Signal, 0, len(u.Assets))
	for _, a := range u.Assets {
		if a == nil {
			continue
		}
		b := base
		if !a.UpdatedTs.IsZero() {
			b.Ts = a.UpdatedTs
		}

		total := decimal.Zero
		if a.Balance != nil {
			total = *a.Balance
		}
		frozen := decimal.Zero
		if a.Locked != nil {
			frozen = *a.Locked
		}
		walletType := a.WalletType
		if !walletType.Valid() {
			walletType = ctypes.WalletTypeFund
		}

		if u.Type == ctypes.UpdateTypeSnapshot {
			out = append(out, &stypes.BalanceSignal{
				BaseSignal: b,
				WalletType: walletType,
				Asset:      a.Code,
				Free:       total.Sub(frozen),
				Frozen:     frozen,
			})
		} else {
			out = append(out, &stypes.BalanceDeltaSignal{
				BaseSignal: b,
				WalletType: walletType,
				Asset:      a.Code,
				Free:       frozen.Neg().Add(total),
				Frozen:     frozen,
			})
		}
	}
	return out
}

func ConvertBalanceSnapshotSignal(base stypes.BaseSignal, u *ctypes.BalanceSnapshot) []stypes.Signal {
	out := make([]stypes.Signal, 0, len(u.Assets))
	for _, a := range u.Assets {
		if a == nil {
			continue
		}
		b := base
		if !a.UpdatedTs.IsZero() {
			b.Ts = a.UpdatedTs
		}

		free := decimal.Zero
		if a.Balance != nil {
			free = *a.Balance
		}
		frozen := decimal.Zero
		if a.Locked != nil {
			frozen = *a.Locked
		}
		walletType := a.WalletType
		if !walletType.Valid() {
			walletType = ctypes.WalletTypeFund
		}
		out = append(out, &stypes.BalanceSignal{
			BaseSignal: b,
			WalletType: walletType,
			Asset:      a.Code,
			Free:       free,
			Frozen:     frozen,
		})
	}
	return out
}

// Parse 将 data-service 的 Envelope/Message 转换为 strategy Signal（可能为 0..N 个）
func Parse(data []byte) ([]stypes.Signal, error) {
	var envelope ctypes.Envelope
	if err := sonic.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}
	return Envelope2Signals(envelope)
}

func Envelope2Signals(envelope ctypes.Envelope) ([]stypes.Signal, error) {
	if envelope.Payload == nil {
		return nil, fmt.Errorf("empty payload")
	}

	exchange, err := ctypes.ParseExchange(envelope.Exchange)
	if err != nil {
		return nil, fmt.Errorf("invalid exchange: %w", err)
	}

	var symbol *ctypes.Symbol
	if envelope.Symbol != nil {
		parsed, err := ctypes.ParseSymbol(*envelope.Symbol)
		if err != nil {
			return nil, fmt.Errorf("invalid symbol: %w", err)
		}
		symbol = &parsed
	}

	base := stypes.BaseSignal{
		Exchange:   &exchange,
		Symbol:     symbol,
		Topic:      &envelope.Topic,
		AccountID:  envelope.Account,
		Ts:         time.UnixMilli(envelope.Ts),
		InboundAt:  time.UnixMilli(envelope.ReceiveAt),
		OutboundAt: time.UnixMilli(envelope.PublishAt),
		ReceiveAt:  time.Now(),
	}

	switch {
	case envelope.Payload.Kline != nil:
		return []stypes.Signal{ConvertKlineSignal(base, envelope.Payload.Kline)}, nil
	case envelope.Payload.Ticker != nil:
		return []stypes.Signal{ConvertTickerSignal(base, envelope.Payload.Ticker)}, nil
	case envelope.Payload.Trade != nil:
		return []stypes.Signal{ConvertTradeSignal(base, envelope.Payload.Trade)}, nil
	case envelope.Payload.Depth != nil:
		return []stypes.Signal{ConvertDepthSignal(base, envelope.Payload.Depth)}, nil
	case envelope.Payload.MarkPrice != nil:
		return []stypes.Signal{ConvertMarkPriceSignal(base, envelope.Payload.MarkPrice)}, nil
	case envelope.Payload.Fill != nil:
		return []stypes.Signal{ConvertFillSignal(base, envelope.Payload.Fill)}, nil
	case envelope.Payload.SymbolLeverage != nil:
		return []stypes.Signal{ConvertLeverageChangedSignal(base, envelope.Payload.SymbolLeverage)}, nil
	case envelope.Payload.Order != nil:
		return []stypes.Signal{ConvertOrderSnapshotSignal(base, envelope.Payload.Order)}, nil
	case envelope.Payload.BalanceUpdate != nil:
		return ConvertBalanceUpdateSignal(base, envelope.Payload.BalanceUpdate), nil
	case envelope.Payload.BalanceSnapshot != nil:
		return ConvertBalanceSnapshotSignal(base, envelope.Payload.BalanceSnapshot), nil
	case envelope.Payload.PositionsUpdate != nil || envelope.Payload.PositionSnapshot != nil:
		var (
			positions  []*ctypes.Position
			isSnapshot bool
		)
		if envelope.Payload.PositionsUpdate != nil {
			u := envelope.Payload.PositionsUpdate
			positions = u.Positions
			isSnapshot = u.Type == ctypes.UpdateTypeSnapshot
		} else {
			snap := envelope.Payload.PositionSnapshot
			positions = snap.Positions
			isSnapshot = true
		}
		if !isSnapshot {
			return nil, nil
		}
		out := make([]stypes.Signal, 0, len(positions))
		for _, pos := range positions {
			if pos == nil {
				continue
			}
			out = append(out, ConvertPositionSignal(base, pos))
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported payload stream: %s", envelope.Stream)
}
