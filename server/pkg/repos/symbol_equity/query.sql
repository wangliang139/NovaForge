-- name: UpsertSymbolEquity :one
-- -- timeout: 1s
INSERT INTO symbol_equity (account_id, exchange, symbol, net_value, base_currency, ts)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (account_id, exchange, symbol, ts) DO UPDATE
SET net_value = EXCLUDED.net_value,
    created_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: ListSymbolEquityByAccountAndRange :many
-- -- timeout: 1s
SELECT * FROM symbol_equity
WHERE account_id = $1
  AND ts >= $2
  AND ts <= $3
  AND (sqlc.narg('exchange')::varchar IS NULL OR exchange = sqlc.narg('exchange')::varchar)
  AND (sqlc.narg('symbol')::varchar IS NULL OR symbol = sqlc.narg('symbol')::varchar)
ORDER BY ts ASC;
