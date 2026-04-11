-- name: CreateEquity :one
-- -- timeout: 1s
INSERT INTO equity (account_id, ts, notional, unrealized_profit)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetEquityBeforeTs :one
-- -- timeout: 1s
SELECT * FROM equity
WHERE account_id = $1 AND ts <= $2
ORDER BY ts DESC
LIMIT 1;

-- name: ListEquityByAccountAndRange :many
-- -- timeout: 1s
SELECT * FROM equity
WHERE account_id = $1 AND ts >= $2 AND ts <= $3
ORDER BY ts ASC;
