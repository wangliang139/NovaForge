-- name: UpsertOrder :one
-- -- timeout: 1s
INSERT INTO orders (bot_id, account_id, order_id, client_order_id, drived_order_id, order_type, algo_type, source, exchange, symbol, side, is_buy, price, quantity, executed_qty, executed_price, avg_price, conditions, detail, status, reject_reason, reduce_only, post_only, tif, created_ts, working_ts, finished_ts, updated_ts, locked, locked_asset, fee, fee_asset, realized_pnl, pnl_asset)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34)
ON CONFLICT (account_id, order_id) DO UPDATE
SET drived_order_id = coalesce(EXCLUDED.drived_order_id, orders.drived_order_id),
    executed_qty = coalesce(EXCLUDED.executed_qty, orders.executed_qty),
    executed_price = coalesce(EXCLUDED.executed_price, orders.executed_price),
    avg_price = coalesce(EXCLUDED.avg_price, orders.avg_price),
    conditions = coalesce(EXCLUDED.conditions, orders.conditions),
    detail = coalesce(EXCLUDED.detail, orders.detail),
    status = coalesce(EXCLUDED.status, orders.status),
    reject_reason = coalesce(EXCLUDED.reject_reason, orders.reject_reason),
    working_ts = coalesce(orders.working_ts, EXCLUDED.working_ts),
    finished_ts = coalesce(EXCLUDED.finished_ts, orders.finished_ts),
    updated_ts = coalesce(EXCLUDED.updated_ts, orders.updated_ts),
    locked = CASE
        WHEN coalesce(EXCLUDED.status, orders.status) IN ('DONE', 'CANCELED', 'REJECTED', 'EXPIRED') THEN 0
        ELSE coalesce(EXCLUDED.locked, orders.locked)
    END,
    locked_asset = coalesce(EXCLUDED.locked_asset, orders.locked_asset),
    fee = coalesce(EXCLUDED.fee, orders.fee),
    fee_asset = coalesce(EXCLUDED.fee_asset, orders.fee_asset),
    realized_pnl = coalesce(EXCLUDED.realized_pnl, orders.realized_pnl),
    pnl_asset = coalesce(EXCLUDED.pnl_asset, orders.pnl_asset),
    updated_at = CURRENT_TIMESTAMP
WHERE orders.updated_ts <= EXCLUDED.updated_ts
RETURNING *;

-- name: GetOrder :one
-- -- timeout: 1s
SELECT * FROM orders
WHERE id = $1;

-- name: GetOrderByOrderIdWithLock :one
-- -- timeout: 1s
SELECT * FROM orders
WHERE account_id = $1
  AND order_id = $2
FOR UPDATE;

-- name: GetOrderByClientOrderIdWithLock :one
-- -- timeout: 1s
SELECT * FROM orders
WHERE account_id = $1
  AND client_order_id = $2
FOR UPDATE;

-- name: GetOrderByClientOrderId :one
-- -- timeout: 1s
SELECT * FROM orders
WHERE account_id = $1
  AND client_order_id = $2;

-- name: GetOrderByOrderId :one
-- -- timeout: 1s
SELECT * FROM orders
WHERE account_id = $1
  AND order_id = $2;

-- name: GetOrderByClientOrderIdUnderParent :one
-- -- timeout: 1s
SELECT o.*
FROM orders o
LEFT JOIN public.account a ON a.id = o.account_id AND a.deleted_at IS NULL
WHERE o.client_order_id = $1
  AND o.exchange = $2
  AND (
    o.account_id = $3
    OR a.parent_account_id = $3
  )
ORDER BY o.updated_at DESC NULLS LAST, o.id DESC
LIMIT 1;

-- name: GetOrderByOrderIdUnderVirtualSubs :one
-- -- timeout: 1s
SELECT o.*
FROM orders o
INNER JOIN public.account a ON a.id = o.account_id AND a.deleted_at IS NULL
WHERE o.order_id = $1
  AND o.exchange = $2
  AND a.parent_account_id = $3
ORDER BY o.updated_at DESC NULLS LAST, o.id DESC
LIMIT 1;

-- name: ListOrders :many
-- -- timeout: 1s
SELECT * FROM orders
WHERE account_id = $1
  AND (sqlc.narg('bot_id')::int IS NULL OR bot_id = sqlc.narg('bot_id')::int)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListOrdersByPage :many
-- -- timeout: 1s
SELECT * FROM orders
WHERE account_id = $1
  AND (sqlc.narg('bot_id')::int IS NULL OR bot_id = sqlc.narg('bot_id')::int)
  AND (sqlc.narg('start_ts')::timestamptz IS NULL OR created_ts >= sqlc.narg('start_ts')::timestamptz)
  AND (sqlc.narg('end_ts')::timestamptz IS NULL OR created_ts <= sqlc.narg('end_ts')::timestamptz)
  AND symbol = coalesce(sqlc.narg('symbol')::varchar, symbol)
  AND source = coalesce(sqlc.narg('order_source')::order_source, source)
  AND order_type = coalesce(sqlc.narg('order_type')::order_type, order_type)
  AND (sqlc.narg('statuses')::varchar[] IS NULL OR status = ANY(sqlc.narg('statuses')::varchar[]))
ORDER BY created_ts DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CountOrdersByFilter :one
-- -- timeout: 1s
SELECT COUNT(1) FROM orders
WHERE account_id = $1
  AND (sqlc.narg('bot_id')::int IS NULL OR bot_id = sqlc.narg('bot_id')::int)
  AND (sqlc.narg('start_ts')::timestamptz IS NULL OR created_ts >= sqlc.narg('start_ts')::timestamptz)
  AND (sqlc.narg('end_ts')::timestamptz IS NULL OR created_ts <= sqlc.narg('end_ts')::timestamptz)
  AND symbol = coalesce(sqlc.narg('symbol')::varchar, symbol)
  AND source = coalesce(sqlc.narg('order_source')::order_source, source)
  AND order_type = coalesce(sqlc.narg('order_type')::order_type, order_type)
  AND (sqlc.narg('statuses')::varchar[] IS NULL OR status = ANY(sqlc.narg('statuses')::varchar[]));

-- name: ListOrdersByPageByFinishedTs :many
-- -- timeout: 1s
SELECT * FROM orders
WHERE account_id = $1
  AND (sqlc.narg('bot_id')::int IS NULL OR bot_id = sqlc.narg('bot_id')::int)
  AND finished_ts IS NOT NULL
  AND finished_ts >= sqlc.narg('finished_start_ts')::timestamptz
  AND finished_ts <= sqlc.narg('finished_end_ts')::timestamptz
  AND symbol = coalesce(sqlc.narg('symbol')::varchar, symbol)
  AND source = coalesce(sqlc.narg('order_source')::order_source, source)
  AND order_type = coalesce(sqlc.narg('order_type')::order_type, order_type)
  AND (sqlc.narg('statuses')::varchar[] IS NULL OR status = ANY(sqlc.narg('statuses')::varchar[]))
ORDER BY finished_ts DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CountOrdersByFinishedTsRange :one
-- -- timeout: 1s
SELECT COUNT(1) FROM orders
WHERE account_id = $1
  AND (sqlc.narg('bot_id')::int IS NULL OR bot_id = sqlc.narg('bot_id')::int)
  AND finished_ts IS NOT NULL
  AND finished_ts >= sqlc.narg('finished_start_ts')::timestamptz
  AND finished_ts <= sqlc.narg('finished_end_ts')::timestamptz
  AND symbol = coalesce(sqlc.narg('symbol')::varchar, symbol)
  AND source = coalesce(sqlc.narg('order_source')::order_source, source)
  AND order_type = coalesce(sqlc.narg('order_type')::order_type, order_type)
  AND (sqlc.narg('statuses')::varchar[] IS NULL OR status = ANY(sqlc.narg('statuses')::varchar[]));

-- name: ListOrdersByAccountAndTimeRange :many
-- -- timeout: 5s
SELECT * FROM orders
WHERE account_id = $1
  AND (sqlc.narg('bot_id')::int IS NULL OR bot_id = sqlc.narg('bot_id')::int)
  AND (sqlc.narg('symbol')::varchar IS NULL OR symbol = sqlc.narg('symbol')::varchar)
  AND created_ts >= $2
  AND created_ts <= $3
ORDER BY created_ts ASC, id ASC
LIMIT $4;

-- name: ListOrdersByCursor :many
-- -- timeout: 1s
SELECT * FROM orders
WHERE account_id = $1
  AND (sqlc.narg('bot_id')::int IS NULL OR bot_id = sqlc.narg('bot_id')::int)
  AND symbol = coalesce(sqlc.narg('symbol')::varchar, symbol)
  AND source = coalesce(sqlc.narg('order_source')::order_source, source)
  AND order_type = coalesce(sqlc.narg('order_type')::order_type, order_type)
  AND (sqlc.narg('statuses')::varchar[] IS NULL OR status = ANY(sqlc.narg('statuses')::varchar[]))
  AND (
    sqlc.narg('cursor_created_ts')::timestamptz IS NULL
    OR created_ts < sqlc.narg('cursor_created_ts')::timestamptz
    OR (
      created_ts = sqlc.narg('cursor_created_ts')::timestamptz
      AND id < sqlc.narg('cursor_id')::bigint
    )
  )
ORDER BY created_ts DESC, id DESC
LIMIT $2;

-- name: GetPendingOrders :many
-- -- timeout: 1s
SELECT * FROM orders
WHERE account_id = $1
AND status IN ('NEW', 'PENDING', 'WORKING', 'PARTIAL_DONE')
AND symbol = coalesce(sqlc.narg('symbol')::varchar, symbol)
ORDER BY created_at DESC;

-- name: CancelOrderStatusWithReason :one
-- -- timeout: 1s
UPDATE orders
SET status = 'CANCELED',
    reject_reason = $3,
    locked = 0,
    finished_ts = $4,
    updated_ts = $5,
    updated_at = CURRENT_TIMESTAMP
WHERE account_id = $1
  AND order_id = $2
RETURNING *;

-- name: SetOrderLockedAsset :one
-- -- timeout: 1s
UPDATE orders
SET locked = $3,
    locked_asset = $4
WHERE account_id = $1
  AND order_id = $2
RETURNING *;

-- name: UpdateOrderId :one
-- -- timeout: 1s
UPDATE orders
SET order_id = $2
WHERE id = $1
RETURNING *;
