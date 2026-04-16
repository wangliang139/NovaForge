-- name: InsertAccountAssetSnapshot :exec
-- -- timeout: 2s
INSERT INTO asset_snapshot (account_id, exchange, wallet_type, asset, total, effective_ts)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetAccountAssetSnapshotAtOrBefore :one
-- -- timeout: 2s
SELECT *
FROM asset_snapshot
WHERE account_id = $1
  AND exchange = $2
  AND asset = $3
  AND wallet_type = $4
  AND effective_ts <= $5
ORDER BY effective_ts DESC, id DESC
LIMIT 1;

-- name: GetAccountAssetSnapshotAtOrAfter :one
-- -- timeout: 2s
SELECT *
FROM asset_snapshot
WHERE account_id = $1
  AND exchange = $2
  AND asset = $3
  AND wallet_type = $4
  AND effective_ts >= $5
ORDER BY effective_ts DESC, id DESC
LIMIT 1;

-- name: ListLatestAccountAssetSnapshotsAtOrBefore :many
-- -- timeout: 2s
SELECT DISTINCT ON (asset, wallet_type) *
FROM asset_snapshot
WHERE account_id = $1
  AND exchange = $2
  AND effective_ts <= $3
ORDER BY asset, wallet_type, effective_ts DESC, id DESC;

-- name: InsertAccountPositionSnapshot :exec
-- -- timeout: 2s
INSERT INTO position_snapshot (account_id, exchange, symbol, side, qty, entry_price, leverage, effective_ts)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetAccountPositionSnapshotAtOrBefore :one
-- -- timeout: 2s
SELECT *
FROM position_snapshot
WHERE account_id = $1
  AND exchange = $2
  AND symbol = $3
  AND side = $4
  AND effective_ts <= $5
ORDER BY effective_ts DESC, id DESC
LIMIT 1;

-- name: ListLatestAccountPositionSnapshotsAtOrBefore :many
-- -- timeout: 2s
SELECT DISTINCT ON (symbol, side) *
FROM position_snapshot
WHERE account_id = $1
  AND exchange = $2
  AND effective_ts <= $3
ORDER BY symbol, side, effective_ts DESC, id DESC;

-- name: ListAccountAssetSnapshotsInRange :many
-- -- timeout: 5s
SELECT effective_ts, total
FROM asset_snapshot
WHERE account_id = $1
  AND exchange = $2
  AND asset = $3
  AND wallet_type = $4
  AND effective_ts >= $5
  AND effective_ts <= $6
ORDER BY effective_ts ASC, id ASC
LIMIT 10000;

-- name: ListAccountPositionSnapshotsInRange :many
-- -- timeout: 5s
SELECT effective_ts, qty, entry_price
FROM position_snapshot
WHERE account_id = $1
  AND exchange = $2
  AND symbol = $3
  AND side = $4
  AND effective_ts >= $5
  AND effective_ts <= $6
ORDER BY effective_ts ASC, id ASC
LIMIT 10000;
