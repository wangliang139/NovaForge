-- name: CreateLedgerEntry :one
-- -- timeout: 1s
INSERT INTO ledgers (id, account_id, exchange, asset, wallet_type, total, frozen, total_delta, frozen_delta, type, detail, ts, is_effective)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: ListLedgers :many
-- -- timeout: 5s
SELECT * FROM ledgers
WHERE account_id = $1
AND ts between $2 and $3
ORDER BY id DESC
LIMIT $4
OFFSET $5;

-- name: CountLedgers :one
-- -- timeout: 1s
SELECT COUNT(1) FROM ledgers
WHERE account_id = $1
AND ts between $2 and $3;