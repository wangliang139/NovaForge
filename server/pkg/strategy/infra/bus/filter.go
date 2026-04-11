package bus

import (
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// Filter 事件过滤器
type Filter interface {
	// Match 检查事件是否匹配过滤器
	Match(sig stypes.Signal) bool
}

// FuncFilter 函数过滤器
type funcFilter struct {
	fn func(sig stypes.Signal) bool
}

func (f *funcFilter) Match(sig stypes.Signal) bool {
	return f.fn(sig)
}

func NewFuncFilter(fn func(sig stypes.Signal) bool) Filter {
	return &funcFilter{fn: fn}
}

func NewMarketSignalFilter() Filter {
	return NewFuncFilter(func(sig stypes.Signal) bool {
		return sig.GetType().IsMarketSignal()
	})
}

// TypeFilter 按事件类型过滤
type TypeFilter struct {
	EventType stypes.SignalType
}

func (f *TypeFilter) Match(sig stypes.Signal) bool {
	return sig.GetType() == f.EventType
}

// NewTypeFilter 创建类型过滤器
func NewTypeFilter(eventType stypes.SignalType) *TypeFilter {
	return &TypeFilter{
		EventType: eventType,
	}
}

// KindFilter 按事件类型过滤
type KindFilter struct {
	Kind stypes.SignalKind
}

func (f *KindFilter) Match(sig stypes.Signal) bool {
	return sig.GetKind() == f.Kind
}

func NewKindFilter(kind stypes.SignalKind) *KindFilter {
	return &KindFilter{
		Kind: kind,
	}
}

// ExSymbolFilter 按交易所和交易对过滤
type ExSymbolFilter struct {
	ExSymbol ctypes.ExSymbol
}

func (f *ExSymbolFilter) Match(sig stypes.Signal) bool {
	if sig.GetExchange() == nil || sig.GetSymbol() == nil {
		return false
	}
	exSymbol := ctypes.NewExSymbol(*sig.GetExchange(), *sig.GetSymbol())
	return exSymbol.Key() == f.ExSymbol.Key()
}

// NewExSymbolFilter 创建交易所和交易对过滤器
func NewExSymbolFilter(exSymbol ctypes.ExSymbol) *ExSymbolFilter {
	return &ExSymbolFilter{ExSymbol: exSymbol}
}

// CompositeFilter 组合过滤器（所有过滤器都必须匹配）
type CompositeFilter struct {
	Filters []Filter
}

func (f *CompositeFilter) Match(sig stypes.Signal) bool {
	for _, filter := range f.Filters {
		if !filter.Match(sig) {
			return false
		}
	}
	return true
}

// NewCompositeFilter 创建组合过滤器
func NewCompositeFilter(filters ...Filter) *CompositeFilter {
	return &CompositeFilter{Filters: filters}
}
