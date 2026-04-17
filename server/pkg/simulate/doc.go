// Package market implements CEX-style aggregated L2 depth maintenance (snapshot + delta patch),
// optional coordination with resync on sequence gaps, read-only shadow execution against the
// public book, and a minimal SimExchange for spot / one-way perp simulation (portfolio, SimBook, PlaceOrder).
package simulate
