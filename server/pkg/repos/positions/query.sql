-- name: UpsertPosition :one
-- -- timeout: 1s
WITH prev AS (
    SELECT
        t.account_id,
        t.exchange,
        t.symbol,
        t.side,
        t.qty,
        t.entry_price,
        t.leverage,
        t.updated_ts
    FROM positions t
    WHERE t.account_id = sqlc.arg('account_id')
      AND t.exchange   = sqlc.arg('exchange')
      AND t.symbol     = sqlc.arg('symbol')
      AND t.side       = sqlc.arg('side')
),

updated AS (
    UPDATE positions
    SET
        qty = sqlc.arg('qty'),
        entry_price = sqlc.arg('entry_price'),
        leverage = COALESCE(sqlc.arg('leverage'), positions.leverage),
        updated_ts = sqlc.arg('updated_ts'),
        updated_at = CURRENT_TIMESTAMP
    WHERE positions.account_id = sqlc.arg('account_id')
      AND positions.exchange   = sqlc.arg('exchange')
      AND positions.symbol     = sqlc.arg('symbol')
      AND positions.side       = sqlc.arg('side')
      AND positions.updated_ts <= sqlc.arg('updated_ts')
    RETURNING *
),

inserted AS (
    INSERT INTO positions (
        account_id,
        exchange,
        symbol,
        side,
        qty,
        entry_price,
        leverage,
        updated_ts
    )
    SELECT
        sqlc.arg('account_id'),
        sqlc.arg('exchange'),
        sqlc.arg('symbol'),
        sqlc.arg('side'),
        sqlc.arg('qty'),
        sqlc.arg('entry_price'),
        sqlc.arg('leverage'),
        sqlc.arg('updated_ts')
    WHERE NOT EXISTS (SELECT 1 FROM prev)
    RETURNING *
),

final AS (
    SELECT * FROM updated
    UNION ALL
    SELECT * FROM inserted
)

SELECT
    f.account_id,
    f.exchange,
    f.symbol,
    f.side,
    f.qty,
    f.entry_price,
    f.leverage,
    f.updated_ts,
    p.qty         AS prev_qty,
    p.entry_price AS prev_entry_price,
    p.leverage    AS prev_leverage,
    p.updated_ts  AS prev_updated_ts
FROM final f
LEFT JOIN prev p
ON true;

-- name: GetPosition :one
-- -- timeout: 1s
SELECT * FROM positions
WHERE account_id = $1 AND exchange = $2 AND symbol = $3 AND side = $4;

-- name: GetPositionWithLock :one
-- -- timeout: 1s
SELECT * FROM positions
WHERE account_id = $1 AND exchange = $2 AND symbol = $3 AND side = $4
FOR UPDATE;

-- name: ListPositionsByAccount :many
-- -- timeout: 1s
SELECT * FROM positions
WHERE account_id = $1
ORDER BY exchange, symbol, side;

-- name: ListPositionsByAccountAndExchange :many
-- -- timeout: 1s
SELECT * FROM positions
WHERE account_id = $1 AND exchange = $2
ORDER BY symbol, side;

-- name: DeletePosition :exec
-- -- timeout: 1s
DELETE FROM positions
WHERE account_id = $1 AND exchange = $2 AND symbol = $3 AND side = $4;

-- name: SetSymbolLeverage :one
-- -- timeout: 1s
UPDATE positions
SET leverage = $5, updated_at = CURRENT_TIMESTAMP
WHERE account_id = $1 AND exchange = $2 AND symbol = $3 AND side = $4 AND leverage != $5
RETURNING *;