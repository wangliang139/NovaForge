-- name: Upsert :one
-- -- timeout: 1s
INSERT INTO public.calendar (date_id, source, sid, type, category, country, project, symbol, title, content, importance, url, ext, published_at, md5)
VALUES ($1,
        $2,
        $3,
        $4,
        $5,
        $6,
        $7,
        $8,
        $9,
        $10,
        $11,
        $12,
        $13,
        $14,
        $15)
ON CONFLICT (md5) DO UPDATE SET
    date_id = excluded.date_id,
    source = excluded.source,
    sid = excluded.sid,
    type = excluded.type,
    category = excluded.category,
    country = excluded.country,
    project = excluded.project,
    symbol = excluded.symbol,
    title = excluded.title,
    content = excluded.content,
    importance = excluded.importance,
    url = excluded.url,
    ext = excluded.ext,
    published_at = excluded.published_at,
    updated_at = now()
returning *;

-- name: GroupByDateId :many
-- -- timeout: 3s
SELECT date_id, COUNT(1)
FROM public.calendar
WHERE source = coalesce(sqlc.narg('source')::calendar_source, source)
  AND type = coalesce(sqlc.narg('type')::calendar_type, type)
  AND category = coalesce(sqlc.narg('category')::varchar, category)
  AND (country = coalesce(sqlc.narg('country')::varchar, country) or country is null)
  AND importance >= coalesce(sqlc.narg('min_importance')::int, importance)
  AND date_id between sqlc.arg('date_id_start')::int and sqlc.arg('date_id_end')::int
GROUP BY date_id
ORDER BY date_id ASC;

-- name: QueryByDateId :many
-- -- timeout: 3s
SELECT *
FROM public.calendar
WHERE date_id = sqlc.arg('date_id')::int
  AND source = coalesce(sqlc.narg('source')::calendar_source, source)
  AND type = coalesce(sqlc.narg('type')::calendar_type, type)
  AND category = coalesce(sqlc.narg('category')::varchar, category)
  AND (country = coalesce(sqlc.narg('country')::varchar, country) or country is null)
  AND importance >= coalesce(sqlc.narg('min_importance')::int, importance)
ORDER BY published_at, importance DESC
OFFSET sqlc.arg('offset')::int8 LIMIT sqlc.arg('limit')::int8;

-- name: GetBySid :one
-- -- timeout: 3s
SELECT *
FROM public.calendar
WHERE source = sqlc.arg('source')::calendar_source
  AND sid = sqlc.arg('sid')::varchar;

-- name: UpdateBySid :one
-- -- timeout: 1s
UPDATE public.calendar
SET 
    date_id = sqlc.arg('date_id')::int,
    type = sqlc.arg('type')::calendar_type,
    category = sqlc.arg('category')::varchar,
    country = sqlc.narg('country')::varchar,
    project = sqlc.narg('project')::varchar,
    symbol = sqlc.narg('symbol')::varchar,
    title = sqlc.arg('title')::varchar,
    content = sqlc.arg('content')::varchar,
    importance = sqlc.arg('importance')::int,
    url = sqlc.arg('url')::varchar,
    ext = sqlc.arg('ext')::jsonb,
    md5 = sqlc.arg('md5')::varchar,
    published_at = sqlc.arg('published_at')::timestamp with time zone,
    updated_at = now()
WHERE source = sqlc.arg('source')::calendar_source
  AND sid = sqlc.arg('sid')::varchar
RETURNING *;