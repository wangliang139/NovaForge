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

func (d _decimal) PgNumericToFloat64(value pgtype.Numeric) float64 {
	return d.PgNumericToDecimal(value).InexactFloat64()
}
