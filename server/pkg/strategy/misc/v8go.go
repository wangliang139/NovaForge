package misc

import (
	"fmt"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"rogchap.com/v8go"
)

func GetStringParam(val *v8go.Value, key string) (string, error) {
	v, err := val.Object().Get(key)
	if err != nil {
		return "", err
	}
	return v.String(), nil
}

func RequiredStringParam(obj *v8go.Value, key string) (string, error) {
	v, err := GetStringParam(obj, key)
	if err != nil {
		return "", fmt.Errorf("%s is required", key)
	}
	if v == "" || v == "undefined" || v == "null" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func OptionalStringParam(obj *v8go.Value, key string) (string, bool) {
	v, err := GetStringParam(obj, key)
	if err != nil || v == "" || v == "undefined" || v == "null" {
		return "", false
	}
	return v, true
}

func AnyToDecimal(v any) (decimal.Decimal, bool) {
	switch x := v.(type) {
	case float64:
		return decimal.NewFromFloat(x), true
	case float32:
		return decimal.NewFromFloat32(x), true
	case int64:
		return decimal.NewFromInt(x), true
	case int:
		return decimal.NewFromInt(int64(x)), true
	case decimal.Decimal:
		return x, true
	case *decimal.Decimal:
		if x == nil {
			return decimal.Zero, false
		}
		return *x, true
	case string:
		d, err := decimal.NewFromString(x)
		if err != nil {
			return decimal.Zero, false
		}
		return d, true
	default:
		return decimal.Zero, false
	}
}

func AnyToString(v any) (*string, error) {
	if v == nil {
		return nil, nil
	}
	var str string
	switch x := v.(type) {
	case string:
		str = x
	case int:
		str = strconv.Itoa(x)
	case int64:
		str = strconv.FormatInt(x, 10)
	case int32:
		str = strconv.FormatInt(int64(x), 10)
	case float64:
		str = strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		str = strconv.FormatFloat(float64(x), 'f', -1, 32)
	case bool:
		str = strconv.FormatBool(x)
	case decimal.Decimal:
		str = x.String()
	case *decimal.Decimal:
		if x == nil {
			return nil, fmt.Errorf("decimal is nil")
		}
		str = x.String()
	case []any:
		bytes, err := sonic.Marshal(x)
		if err != nil {
			return nil, err
		}
		str = string(bytes)
	case map[string]any:
		bytes, err := sonic.Marshal(x)
		if err != nil {
			return nil, err
		}
		str = string(bytes)
	default:
		return nil, fmt.Errorf("invalid string value: %v", v)
	}
	return &str, nil
}

// AnyToV8Value 将任意类型转换为v8值
func AnyToV8Value(ctx *v8go.Context, v any) (*v8go.Value, error) {
	switch val := v.(type) {
	case string:
		return v8go.NewValue(ctx.Isolate(), val)
	case int:
		return v8go.NewValue(ctx.Isolate(), int32(val))
	case int32:
		return v8go.NewValue(ctx.Isolate(), val)
	case int64:
		return v8go.NewValue(ctx.Isolate(), float64(val))
	case float64:
		return v8go.NewValue(ctx.Isolate(), val)
	case bool:
		return v8go.NewValue(ctx.Isolate(), val)
	case map[string]any:
		obj := v8go.NewObjectTemplate(ctx.Isolate())
		inst, err := obj.NewInstance(ctx)
		if err != nil {
			return nil, err
		}
		for k, vv := range val {
			converted, err := AnyToV8Value(ctx, vv)
			if err != nil {
				return nil, err
			}
			inst.Set(k, converted)
		}
		return inst.Value, nil
	case nil:
		return v8go.Null(ctx.Isolate()), nil
	case []any:
		// 简化：用 JSON 序列化
		bytes, err := sonic.Marshal(val)
		if err != nil {
			return nil, err
		}
		return v8go.JSONParse(ctx, string(bytes))
	default:
		bytes, err := sonic.Marshal(val)
		if err != nil {
			return nil, err
		}
		return v8go.JSONParse(ctx, string(bytes))
	}
}

// V8ValueToMap 将 V8 对象转换为 Go map
func V8ValueToMap(ctx *v8go.Context, val *v8go.Value) (map[string]any, error) {
	if val == nil || !val.IsObject() {
		return nil, fmt.Errorf("value is not an object")
	}

	// 简化：通过 JSON 序列化/反序列化
	jsonStr, err := v8go.JSONStringify(ctx, val)
	if err != nil {
		return nil, fmt.Errorf("failed to stringify object: %w", err)
	}

	var result map[string]any
	if err := sonic.UnmarshalString(jsonStr, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return result, nil
}
