-- name: BatchInsert :execrows
-- -- timeout: 10s
INSERT INTO ds_items (ds_id, data, ts)
VALUES (UNNEST(@ds_id::int[]), UNNEST(@data::jsonb[]), UNNEST(@ts::timestamptz[]));

-- name: GetItemsByDsIdAndTs :many
-- -- timeout: 10s
SELECT *
FROM ds_items
WHERE ds_id = sqlc.arg('ds_id')
  AND (
    ts > sqlc.arg('start_ts')
    OR (ts = sqlc.arg('start_ts') AND id > sqlc.arg('start_id'))
  )
  AND ts <= sqlc.arg('end_ts')
ORDER BY ts, id
LIMIT sqlc.arg('limit');