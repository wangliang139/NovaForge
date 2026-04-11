package types

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
)

type SymbolKey string

func (k SymbolKey) String() string {
	return string(k)
}

type Symbol struct {
	Base  string     `json:"base,omitempty"`
	Quote string     `json:"quote,omitempty"`
	Type  MarketType `json:"type,omitempty"`
}

func NewSymbol(base string, quote string, tp MarketType) Symbol {
	return Symbol{
		Base:  base,
		Quote: quote,
		Type:  tp,
	}
}

func (s Symbol) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(s.String())
}

func (s *Symbol) UnmarshalJSON(data []byte) error {
	var v string
	if err := sonic.Unmarshal(data, &v); err != nil {
		return err
	}

	sym, err := ParseSymbol(v)
	if err != nil {
		return err
	}

	*s = sym
	return nil
}

func (s Symbol) String() string {
	return fmt.Sprintf("%s/%s:%s", s.Base, s.Quote, s.Type)
}

func (s Symbol) IsValid() bool {
	return s.Base != "" && s.Quote != "" && s.Type.Valid()
}

func (s Symbol) Key() SymbolKey {
	return SymbolKey(s.String())
}

func (s Symbol) Equal(other Symbol) bool {
	return s.Base == other.Base && s.Quote == other.Quote && s.Type == other.Type
}

type SymbolConfig struct {
	Exchange Exchange `json:"exchange,omitempty"`
	Symbol   Symbol   `json:"symbol,omitempty"`

	Market Market `json:"market,omitempty"`

	// commission
	MakerCommission decimal.Decimal `json:"makerCommission,omitempty"`
	TakerCommission decimal.Decimal `json:"takerCommission,omitempty"`

	// future config
	IsolatedMarginEnabled bool            `json:"isolatedMarginEnabled,omitempty"` // 是否开启逐仓模式
	CrossMarginEnabled    bool            `json:"crossMarginEnabled,omitempty"`    // 是否开启全仓模式
	IsAutoAddMargin       bool            `json:"isAutoAddMargin,omitempty"`       // 是否自动增加保证金
	CrossLeverage         [2]int          `json:"crossLeverage,omitempty"`         // 0: 买单杠杆倍数, 1: 卖单杠杆倍数
	MaxNotionalValue      decimal.Decimal `json:"maxNotionalValue,omitempty"`      // 持仓最大名义价值

	// ...
}

var SymbolRegex = regexp.MustCompile(`^.+/.+(:[A-Z0-9]+)?$`)

// BTC/USDT:SWAP
// ParseSymbol parses a symbol string like "BTC/USDT:SWAP" or "eth_usdt" or "BTCUSDT:spot" into a Symbol struct.
// Supports forms: BASE/QUOTE[:TYPE], BASE_QUOTE[:TYPE], BASEQUOTE[:TYPE].
// If TYPE 不指定，则Type 为空字符串（""）。
func ParseSymbol(str string) (Symbol, error) {
	str = strings.ToUpper(strings.TrimSpace(str))
	if str == "" {
		return Symbol{}, fmt.Errorf("empty symbol string")
	}

	if !SymbolRegex.MatchString(str) {
		return Symbol{}, fmt.Errorf("invalid symbol string")
	}

	marketType := MarketTypeSpot

	if idx := strings.LastIndex(str, ":"); idx > 0 {
		marketType = MarketType(str[idx+1:])
		if !marketType.Valid() {
			return Symbol{}, fmt.Errorf("invalid market type: %s", marketType)
		}
		str = str[:idx]
		if len(str) == 0 {
			return Symbol{}, fmt.Errorf("invalid symbol string")
		}
	}

	base := strings.Split(str, "/")[0]
	quote := strings.Split(str, "/")[1]

	return Symbol{
		Base:  base,
		Quote: quote,
		Type:  marketType,
	}, nil
}

type ExSymbolKey string

func (k ExSymbolKey) String() string {
	return string(k)
}

type ExSymbol struct {
	Exchange Exchange `json:"exchange,omitempty"`
	Symbol   Symbol   `json:"symbol,omitempty"`
}

func NewExSymbol(exchange Exchange, symbol Symbol) ExSymbol {
	return ExSymbol{
		Exchange: exchange,
		Symbol:   symbol,
	}
}

func (e ExSymbol) GetExchange() Exchange {
	return e.Exchange
}

func (e ExSymbol) GetBase() string {
	return e.Symbol.Base
}

func (e ExSymbol) GetQuote() string {
	return e.Symbol.Quote
}

func (e ExSymbol) GetType() MarketType {
	return e.Symbol.Type
}

func (e ExSymbol) Key() ExSymbolKey {
	return ExSymbolKey(e.String())
}

func (e ExSymbol) String() string {
	return fmt.Sprintf("%s:%s/%s:%s", e.Exchange.String(), e.Symbol.Base, e.Symbol.Quote, e.Symbol.Type)
}

func (e ExSymbol) IsValid() bool {
	return e.Symbol.IsValid() && e.Exchange.IsValid()
}

var ExSymbolRegex = regexp.MustCompile(`^[A-Z0-9]+:[A-Z0-9]+/[A-Z0-9]+(:[A-Z0-9]+)?$`)

func ParseExSymbol(str string) (ExSymbol, error) {
	str = strings.ToUpper(strings.TrimSpace(str))
	if str == "" {
		return ExSymbol{}, fmt.Errorf("empty ex symbol string")
	}

	if !ExSymbolRegex.MatchString(str) {
		return ExSymbol{}, fmt.Errorf("invalid ex symbol string")
	}

	exchange := strings.Split(str, ":")[0]
	symbol := strings.Split(str, ":")[1]

	ex, err := ParseExchange(exchange)
	if err != nil {
		return ExSymbol{}, err
	}

	sym, err := ParseSymbol(symbol)
	if err != nil {
		return ExSymbol{}, err
	}

	return ExSymbol{
		Exchange: ex,
		Symbol:   sym,
	}, nil
}

type SortType string

const (
	SortTypeAsc  SortType = "ASC"
	SortTypeDesc SortType = "DESC"
)

func (s SortType) Valid() bool {
	switch s {
	case SortTypeAsc, SortTypeDesc:
		return true
	}
	return false
}

type SymbolStatus string

const (
	SymbolStatusTesting      SymbolStatus = "TESTING"
	SymbolStatusPreTrading   SymbolStatus = "PRE_TRADING"
	SymbolStatusTrading      SymbolStatus = "TRADING"
	SymbolStatusPostTrading  SymbolStatus = "POST_TRADING"
	SymbolStatusEndOfDay     SymbolStatus = "END_OF_DAY"
	SymbolStatusHalt         SymbolStatus = "HALT"
	SymbolStatusAuctionMatch SymbolStatus = "AUCTION_MATCH"
	SymbolStatusBreak        SymbolStatus = "BREAK"
)

func (s SymbolStatus) Valid() bool {
	switch s {
	case SymbolStatusPreTrading, SymbolStatusTrading, SymbolStatusPostTrading, SymbolStatusEndOfDay, SymbolStatusHalt, SymbolStatusAuctionMatch, SymbolStatusBreak:
		return true
	}
	return false
}
