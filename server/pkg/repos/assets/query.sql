-- name: UpsertAsset :one
-- -- timeout: 1s
WITH prev AS (
    SELECT 
        a.*,
        a.total  AS prev_total,
        a.frozen AS prev_frozen,
        a.order_occupied AS prev_order_occupied
    FROM assets a
    WHERE a.account_id = sqlc.arg('account_id')::varchar
    AND a.asset = sqlc.arg('asset')::varchar
    AND a.wallet_type = sqlc.arg('wallet_type')::wallet_type
    FOR UPDATE
),
upsert AS (
    INSERT INTO assets (account_id, exchange, asset, wallet_type, total, frozen, order_occupied, avg_price, last_updated_ts)
    VALUES (
        sqlc.arg('account_id')::varchar, 
        sqlc.arg('exchange')::varchar, 
        sqlc.arg('asset')::varchar, 
        sqlc.arg('wallet_type')::wallet_type, 
        GREATEST(COALESCE(sqlc.narg('total')::numeric, 0), 0),   -- insert: NULL → 0，且不允许负值
        GREATEST(COALESCE(sqlc.narg('frozen')::numeric, 0), 0),
        GREATEST(COALESCE(sqlc.narg('order_occupied')::numeric, 0), 0),
        COALESCE(sqlc.narg('avg_price')::numeric, 0),   -- insert: NULL → 0
        sqlc.arg('last_updated_ts')::timestamptz)
    ON CONFLICT (account_id, asset, wallet_type)
    DO UPDATE SET 
        total = GREATEST(COALESCE(sqlc.narg('total')::numeric, assets.total), 0),     -- update: NULL → 原值，且不允许负值
        frozen = GREATEST(COALESCE(sqlc.narg('frozen')::numeric, assets.frozen), 0),
        order_occupied = GREATEST(COALESCE(sqlc.narg('order_occupied')::numeric, assets.order_occupied), 0),
        avg_price = COALESCE(sqlc.narg('avg_price')::numeric, assets.avg_price),  -- update: NULL → 原值
        last_updated_ts = EXCLUDED.last_updated_ts,
        updated_at = CURRENT_TIMESTAMP
    WHERE assets.last_updated_ts <= EXCLUDED.last_updated_ts
    RETURNING assets.*
)
SELECT
    COALESCE(u.account_id, p.account_id)       AS account_id,
    COALESCE(u.exchange,   p.exchange)         AS exchange,
    COALESCE(u.asset,      p.asset)            AS asset,
    COALESCE(u.wallet_type,p.wallet_type)      AS wallet_type,
    COALESCE(u.total,      p.total)            AS total,
    COALESCE(u.frozen,     p.frozen)           AS frozen,
    COALESCE(u.order_occupied, p.order_occupied) AS order_occupied,
    COALESCE(u.avg_price, p.avg_price) AS avg_price,
    COALESCE(u.last_updated_ts, p.last_updated_ts) AS last_updated_ts,
    COALESCE(u.created_at, p.created_at)       AS created_at,
    COALESCE(u.updated_at, p.updated_at)       AS updated_at,
    COALESCE(p.prev_total, 0) AS prev_total,
    COALESCE(p.prev_frozen, 0) AS prev_frozen,
    COALESCE(p.prev_order_occupied, 0) AS prev_order_occupied
FROM prev p
FULL JOIN upsert u ON true;

-- name: IncrementAsset :one
-- -- timeout: 1s
UPDATE assets SET 
    total = GREATEST(total + COALESCE(sqlc.narg('total')::numeric, 0), 0), 
    frozen = GREATEST(frozen + COALESCE(sqlc.narg('frozen')::numeric, 0), 0), 
    order_occupied = GREATEST(order_occupied + COALESCE(sqlc.narg('order_occupied')::numeric, 0), 0), 
    updated_at = CURRENT_TIMESTAMP 
WHERE account_id = sqlc.arg('account_id')::varchar
AND asset = sqlc.arg('asset')::varchar
AND wallet_type = sqlc.arg('wallet_type')::wallet_type
AND last_updated_ts < sqlc.arg('last_updated_ts')::timestamptz
RETURNING *;

-- name: IncrementOrderOccupied :one
-- -- timeout: 1s
UPDATE assets SET 
    order_occupied = GREATEST(order_occupied + COALESCE(sqlc.narg('order_occupied')::numeric, 0), 0), 
    updated_at = CURRENT_TIMESTAMP 
WHERE account_id = sqlc.arg('account_id')::varchar
AND asset = sqlc.arg('asset')::varchar
AND wallet_type = sqlc.arg('wallet_type')::wallet_type
RETURNING *;

-- name: UpdateAssetAvgPriceOnIncrease :one
-- -- timeout: 1s
-- 资产变多时按 WAC 更新 avg_price
-- total_delta: 本次增加的数量（正数）
-- price_usdt: 当前 asset/USDT 价格
UPDATE assets SET
  avg_price = (
    (total - sqlc.arg('total_delta')::numeric) * COALESCE(NULLIF(avg_price, 0), 0)
    + sqlc.arg('total_delta')::numeric * sqlc.arg('price_usdt')::numeric
  ) / NULLIF(total, 0),
  updated_at = CURRENT_TIMESTAMP
WHERE account_id = sqlc.arg('account_id')::varchar
  AND asset = sqlc.arg('asset')::varchar
  AND wallet_type = sqlc.arg('wallet_type')::wallet_type
  AND total >= sqlc.arg('total_delta')::numeric
  AND total > 0
RETURNING *;

-- name: SetAssetAvgPrice :one
-- -- timeout: 1s
-- 直接设置 avg_price（用于定时补全）
UPDATE assets SET
  avg_price = sqlc.arg('avg_price')::numeric,
  updated_at = CURRENT_TIMESTAMP
WHERE account_id = sqlc.arg('account_id')::varchar
  AND asset = sqlc.arg('asset')::varchar
  AND wallet_type = sqlc.arg('wallet_type')::wallet_type
RETURNING *;

-- name: GetAsset :one
-- -- timeout: 1s
SELECT * FROM assets
WHERE account_id = $1 AND asset = $2 AND wallet_type = $3;

-- name: GetAssetWithLock :one
-- -- timeout: 1s
SELECT * FROM assets
WHERE account_id = $1 AND asset = $2 AND wallet_type = $3
FOR UPDATE;

-- name: ListAssetsByAccount :many
-- -- timeout: 1s
SELECT * FROM assets
WHERE account_id = $1
ORDER BY asset, wallet_type;

-- name: ResetOrderOccupiedByPendingOrders :many
-- -- timeout: 5s
-- 从当前“在途订单”汇总 locked，重置账户维度的 order_occupied。
-- 说明：
-- - 订单侧 locked_asset 作为资产币种；wallet_type 由 (exchange, symbol.marketType) 推导
-- - 未出现在在途订单汇总中的资产，将被重置为 0
WITH locked_sums AS (
    SELECT
        o.account_id,
        UPPER(TRIM(o.locked_asset)) AS asset,
        CASE
            WHEN o.exchange IN ('okx', 'okx_test') THEN 'trade'::wallet_type
            WHEN split_part(o.symbol, ':', 2) = 'FUTURE' THEN 'future'::wallet_type
            WHEN split_part(o.symbol, ':', 2) = 'SPOT' THEN 'spot'::wallet_type
            ELSE 'fund'::wallet_type
        END AS wallet_type,
        SUM(COALESCE(o.locked, 0)) AS order_occupied
    FROM orders o
    WHERE o.account_id = $1
      AND o.status IN ('NEW', 'PENDING', 'WORKING', 'PARTIAL_DONE')
      AND o.locked_asset IS NOT NULL
      AND TRIM(o.locked_asset) <> ''
    GROUP BY o.account_id, UPPER(TRIM(o.locked_asset)), wallet_type
),
all_assets AS (
    SELECT
        a.account_id,
        a.asset,
        a.wallet_type,
        GREATEST(COALESCE(s.order_occupied, 0), 0) AS order_occupied
    FROM assets a
    LEFT JOIN locked_sums s
      ON s.account_id = a.account_id
     AND s.asset = a.asset
     AND s.wallet_type = a.wallet_type
    WHERE a.account_id = $1
)
UPDATE assets a
SET order_occupied = aa.order_occupied,
    updated_at = CURRENT_TIMESTAMP
FROM all_assets aa
WHERE a.account_id = aa.account_id
  AND a.asset = aa.asset
  AND a.wallet_type = aa.wallet_type
  AND (a.order_occupied IS DISTINCT FROM aa.order_occupied)
RETURNING a.*;
