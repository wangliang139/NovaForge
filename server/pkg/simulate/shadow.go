package simulate

import (
	"github.com/shopspring/decimal"
)

// Fill is one slice of a simulated aggressive execution against the public L2.
type Fill struct {
	Price decimal.Decimal
	Size  decimal.Decimal
}

// SimulateMarketBuy walks the ask side without mutating d (uses a clone for isolation).
func SimulateMarketBuy(d *MarketDepth, qty decimal.Decimal) (fills []Fill, leftover decimal.Decimal, notional decimal.Decimal) {
	if qty.Sign() <= 0 {
		return nil, qty, decimal.Zero
	}
	snap := d.Clone()
	left := qty
	var totalNotional decimal.Decimal

	snap.WalkAsks(func(price decimal.Decimal, size decimal.Decimal) bool {
		if left.Sign() <= 0 {
			return false
		}
		take := decimal.Min(left, size)
		if take.Sign() <= 0 {
			return true
		}
		fills = append(fills, Fill{Price: price, Size: take})
		totalNotional = totalNotional.Add(price.Mul(take))
		left = left.Sub(take)
		return true
	})

	if left.Sign() > 0 {
		return fills, left, totalNotional
	}
	return fills, decimal.Zero, totalNotional
}

// SimulateMarketSell walks the bid side without mutating d.
func SimulateMarketSell(d *MarketDepth, qty decimal.Decimal) (fills []Fill, leftover decimal.Decimal, notional decimal.Decimal) {
	if qty.Sign() <= 0 {
		return nil, qty, decimal.Zero
	}
	snap := d.Clone()
	left := qty
	var totalNotional decimal.Decimal

	snap.WalkBids(func(price decimal.Decimal, size decimal.Decimal) bool {
		if left.Sign() <= 0 {
			return false
		}
		take := decimal.Min(left, size)
		if take.Sign() <= 0 {
			return true
		}
		fills = append(fills, Fill{Price: price, Size: take})
		totalNotional = totalNotional.Add(price.Mul(take))
		left = left.Sub(take)
		return true
	})

	if left.Sign() > 0 {
		return fills, left, totalNotional
	}
	return fills, decimal.Zero, totalNotional
}

// AveragePrice returns totalNotional / filledQty, or zero if filledQty is zero.
func AveragePrice(totalNotional, filledQty decimal.Decimal) decimal.Decimal {
	if filledQty.IsZero() {
		return decimal.Zero
	}
	return totalNotional.Div(filledQty)
}

// SimulateLimitBuy walks asks with price <= limitPrice (inclusive) without mutating d.
func SimulateLimitBuy(d *MarketDepth, limitPrice, qty decimal.Decimal) (fills []Fill, leftover decimal.Decimal, notional decimal.Decimal) {
	if qty.Sign() <= 0 {
		return nil, qty, decimal.Zero
	}
	snap := d.Clone()
	left := qty
	var totalNotional decimal.Decimal

	snap.WalkAsks(func(price decimal.Decimal, size decimal.Decimal) bool {
		if left.Sign() <= 0 {
			return false
		}
		if price.GreaterThan(limitPrice) {
			return false
		}
		take := decimal.Min(left, size)
		if take.Sign() <= 0 {
			return true
		}
		fills = append(fills, Fill{Price: price, Size: take})
		totalNotional = totalNotional.Add(price.Mul(take))
		left = left.Sub(take)
		return true
	})

	if left.Sign() > 0 {
		return fills, left, totalNotional
	}
	return fills, decimal.Zero, totalNotional
}

// SimulateLimitSell walks bids with price >= limitPrice (inclusive) without mutating d.
func SimulateLimitSell(d *MarketDepth, limitPrice, qty decimal.Decimal) (fills []Fill, leftover decimal.Decimal, notional decimal.Decimal) {
	if qty.Sign() <= 0 {
		return nil, qty, decimal.Zero
	}
	snap := d.Clone()
	left := qty
	var totalNotional decimal.Decimal

	snap.WalkBids(func(price decimal.Decimal, size decimal.Decimal) bool {
		if left.Sign() <= 0 {
			return false
		}
		if price.LessThan(limitPrice) {
			return false
		}
		take := decimal.Min(left, size)
		if take.Sign() <= 0 {
			return true
		}
		fills = append(fills, Fill{Price: price, Size: take})
		totalNotional = totalNotional.Add(price.Mul(take))
		left = left.Sub(take)
		return true
	})

	if left.Sign() > 0 {
		return fills, left, totalNotional
	}
	return fills, decimal.Zero, totalNotional
}
