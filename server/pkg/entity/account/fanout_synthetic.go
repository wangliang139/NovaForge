package account

import (
	"time"

	"github.com/samber/lo"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func envelopeMillisFromOrder(ord *ctypes.Order) int64 {
	if ord != nil && !ord.UpdatedTs.IsZero() {
		return ord.UpdatedTs.UnixMilli()
	}
	return time.Now().UnixMilli()
}

// newSyntheticAccountRawOrderEnvelope P2 T4：父 multi_bot 将子派发封装为合成 account_raw，供 handleAccountMessage 统一落库与 Publish。
func newSyntheticAccountRawOrderEnvelope(parentID string, exchange ctypes.Exchange, subID string, ord ctypes.Order) *ctypes.Envelope {
	op := ord
	return &ctypes.Envelope{
		Exchange:       exchange.String(),
		Account:        lo.ToPtr(subID),
		Stream:         ctypes.StreamTypeAccountRaw,
		Payload:        &ctypes.Message{Order: &op},
		Ts:             envelopeMillisFromOrder(&op),
		Synthetic:      true,
		SourceParentID: parentID,
	}
}

func envelopeMillisFromBalanceUpdate(bu *ctypes.BalanceUpdate) int64 {
	if bu == nil || len(bu.Assets) == 0 || bu.Assets[0] == nil {
		return time.Now().UnixMilli()
	}
	if !bu.Assets[0].UpdatedTs.IsZero() {
		return bu.Assets[0].UpdatedTs.UnixMilli()
	}
	return time.Now().UnixMilli()
}

// newSyntheticAccountRawBalanceUpdateEnvelope P2 T7：父 multi_bot 可归因 BalanceUpdate 子份额走合成 account_raw，与 T4 Order 路径一致经 handleAccountMessage 落库。
func newSyntheticAccountRawBalanceUpdateEnvelope(parentID string, exchange ctypes.Exchange, subID string, bu *ctypes.BalanceUpdate) *ctypes.Envelope {
	if bu == nil {
		return nil
	}
	cp := *bu
	return &ctypes.Envelope{
		Exchange:       exchange.String(),
		Account:        lo.ToPtr(subID),
		Stream:         ctypes.StreamTypeAccountRaw,
		Payload:        &ctypes.Message{BalanceUpdate: &cp},
		Ts:             envelopeMillisFromBalanceUpdate(&cp),
		Synthetic:      true,
		SourceParentID: parentID,
	}
}

func envelopeMillisFromSymbolLeverage(sl *ctypes.SymbolLeverage) int64 {
	if sl != nil && !sl.UpdatedTs.IsZero() {
		return sl.UpdatedTs.UnixMilli()
	}
	return time.Now().UnixMilli()
}

// newSyntheticAccountRawSymbolLeverageEnvelope P2 T8：父 multi_bot 将 SymbolLeverage 派发到子账户的合成 account_raw。
func newSyntheticAccountRawSymbolLeverageEnvelope(parentID string, exchange ctypes.Exchange, subID string, sl *ctypes.SymbolLeverage) *ctypes.Envelope {
	if sl == nil {
		return nil
	}
	cp := *sl
	return &ctypes.Envelope{
		Exchange:       exchange.String(),
		Account:        lo.ToPtr(subID),
		Stream:         ctypes.StreamTypeAccountRaw,
		Payload:        &ctypes.Message{SymbolLeverage: &cp},
		Ts:             envelopeMillisFromSymbolLeverage(&cp),
		Synthetic:      true,
		SourceParentID: parentID,
	}
}
