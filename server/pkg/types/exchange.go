package types

import (
	"fmt"
	"strings"
)

type Exchange string

const (
	ExchangeBinance     Exchange = "binance"
	ExchangeOkx         Exchange = "okx"
	ExchangeBinanceTest Exchange = "binance_test"
	ExchangeOkxTest     Exchange = "okx_test"
)

func (e Exchange) IsValid() bool {
	switch e {
	case ExchangeBinance, ExchangeOkx, ExchangeBinanceTest, ExchangeOkxTest:
		return true
	}
	return false
}

func (e Exchange) String() string {
	return string(e)
}

func (e Exchange) Equal(other Exchange) bool {
	return e == other
}

// IsTestnet 表示当前交易所是否为测试环境。
func (e Exchange) IsTestnet() bool {
	return e == ExchangeBinanceTest || e == ExchangeOkxTest
}

// Base 返回对应的生产环境交易所（例如 binance_test -> binance）。
func (e Exchange) Base() Exchange {
	switch e {
	case ExchangeBinanceTest:
		return ExchangeBinance
	case ExchangeOkxTest:
		return ExchangeOkx
	default:
		return e
	}
}

func ParseExchange(str string) (Exchange, error) {
	str = strings.ToLower(strings.TrimSpace(str))
	if str == "" {
		return Exchange("unknown"), fmt.Errorf("empty exchange string")
	}
	// 兼容前端/配置中可能出现的连字符写法
	str = strings.ReplaceAll(str, "-", "_")
	ex := Exchange(str)
	if ex.IsValid() {
		return ex, nil
	}
	return Exchange("unknown"), fmt.Errorf("invalid exchange: %s", str)
}

var AllExchange = []Exchange{
	ExchangeBinance,
	ExchangeOkx,
	ExchangeBinanceTest,
	ExchangeOkxTest,
}
