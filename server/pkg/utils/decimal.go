package utils

import (
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

type _decimal struct {
}

var Decimal = _decimal{}

func (d _decimal) PgNumericToDecimal(value pgtype.Numeric) decimal.Decimal {
	if !value.Valid {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(value.Int, int32(value.Exp))
}

func (d _decimal) DecimalToPgNumeric(value decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{
		Int:   value.Coefficient(),
		Exp:   int32(value.Exponent()),
		Valid: true,
	}
}

func (d _decimal) PgNumericToString(value pgtype.Numeric) string {
	return d.PgNumericToDecimal(value).String()
}

// PgNumericToFloat64 仅用于展示、图表或第三方需要 float 的接口。
// 交易、风控、撮合、记账等路径禁止使用 float64 参与金额运算。
func (d _decimal) PgNumericToFloat64(value pgtype.Numeric) float64 {
	return d.PgNumericToDecimal(value).InexactFloat64()
}
