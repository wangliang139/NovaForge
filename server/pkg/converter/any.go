package converter

import (
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
)

func AnyToInt(v any) (int, error) {
	switch x := v.(type) {
	case int:
		return x, nil
	case int32:
		return int(x), nil
	case int64:
		return int(x), nil
	case string:
		i, err := strconv.Atoi(x)
		if err != nil {
			return 0, err
		}
		return i, nil
	case float64:
		return int(x), nil
	case float32:
		return int(x), nil
	case decimal.Decimal:
		return int(x.IntPart()), nil
	case *decimal.Decimal:
		if x == nil {
			return 0, fmt.Errorf("decimal is nil")
		}
		return int(x.IntPart()), nil
	}
	return 0, fmt.Errorf("invalid int value: %v", v)
}

func AnyToDecimal(v any) (decimal.Decimal, error) {
	switch x := v.(type) {
	case float64:
		return decimal.NewFromFloat(x), nil
	case float32:
		return decimal.NewFromFloat32(x), nil
	case int64:
		return decimal.NewFromInt(x), nil
	case int:
		return decimal.NewFromInt(int64(x)), nil
	case decimal.Decimal:
		return x, nil
	case *decimal.Decimal:
		if x == nil {
			return decimal.Zero, fmt.Errorf("decimal is nil")
		}
		return *x, nil
	}
	return decimal.Zero, fmt.Errorf("invalid decimal value: %v", v)
}

func AnyToString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case int:
		return strconv.FormatInt(int64(x), 10), nil
	}
	return "", fmt.Errorf("invalid string value: %v", v)
}

func AnyToBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		return strconv.ParseBool(x)
	}
	return false, fmt.Errorf("invalid bool value: %v", v)
}
