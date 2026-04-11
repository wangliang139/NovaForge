package atob

import (
	"math/big"
	"strconv"

	"github.com/shopspring/decimal"
)

func PIntToPInt64(i *int) *int64 {
	if i == nil {
		return nil
	}
	i64 := int64(*i)
	return &i64
}

func PInt32ToPInt(i *int32) *int {
	if i == nil {
		return nil
	}
	p := int(*i)
	return &p
}

func PStringToPInt64(s *string) (*int64, error) {
	if s == nil {
		return nil, nil
	}
	i, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func StringToFloat64(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func BigToDecimal(b *big.Float) decimal.Decimal {
	if b == nil {
		return decimal.Zero
	}
	d, _ := decimal.NewFromString(b.String())
	return d
}
