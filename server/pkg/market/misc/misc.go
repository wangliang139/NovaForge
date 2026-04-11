package misc

import (
	"sync"

	"github.com/shopspring/decimal"
)

type Map interface {
	~map[string]any | *sync.Map
}

func MapString2Decimal[M Map](m M, path []string) (decimal.Decimal, error) {
	val, ok := MapValue[M, string](m, path...)
	if !ok {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(val)
}

func MustMapString2Decimal[M Map](m M, path []string) decimal.Decimal {
	val, err := MapString2Decimal(m, path)
	if err != nil {
		panic(err)
	}
	return val
}

func MapValueWithDefault[M Map, T any](m M, path string, defaultValue T) T {
	val, ok := MapValue[M, T](m, path)
	if !ok {
		return defaultValue
	}
	return val
}

func MapPathWithDefault[M Map, T any](m M, path []string, defaultValue T) T {
	val, ok := MapValue[M, T](m, path...)
	if !ok {
		return defaultValue
	}
	return val
}

func MapValue[M Map, T any](m M, path ...string) (T, bool) {
	var zero T
	if m == nil {
		return zero, false
	}

	for i, key := range path {
		switch mm := any(m).(type) {
		case map[string]any:
			val, ok := mm[key]
			if !ok {
				return zero, false
			}
			if i == len(path)-1 {
				v, ok := val.(T)
				if ok {
					return v, true
				}
				return zero, false
			}
			m, ok = val.(M)
			if !ok {
				return zero, false
			}
		case *sync.Map:
			val, ok := mm.Load(key)
			if !ok {
				return zero, false
			}
			if i == len(path)-1 {
				v, ok := val.(T)
				if ok {
					return v, true
				}
				return zero, false
			}
			m, ok = val.(M)
			if !ok {
				return zero, false
			}
		default:
			return zero, false
		}
	}
	return zero, false
}
