-- name: InsertAccountAssetSnapshot :exec
-- -- timeout: 2s
INSERT INTO account_asset_snapshot (account_id, exchange, wallet_type, asset, total, frozen, effective_ts)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetAccountAssetSnapshotAtOrBefore :one
-- -- timeout: 2s
SELECT *
FROM account_asset_snapshot
WHERE account_id = $1
  AND exchange = $2
  AND asset = $3
  AND wallet_type = $4
  AND effective_ts <= $5
ORDER BY effective_ts DESC, id DESC
LIMIT 1;

-- name: InsertAccountPositionSnapshot :exec
-- -- timeout: 2s
INSERT INTO account_position_snapshot (account_id, exchange, symbol, side, qty, entry_price, leverage, effective_ts)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetAccountPositionSnapshotAtOrBefore :one
-- -- timeout: 2s
SELECT *
FROM account_position_snapshot
WHERE account_id = $1
  AND exchange = $2
  AND symbol = $3
  AND side = $4
  AND effective_ts <= $5
ORDER BY effective_ts DESC, id DESC
LIMIT 1;
