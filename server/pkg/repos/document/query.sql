-- name: Create :one
-- -- timeout: 3s
INSERT INTO public.document (source, provider, catalog, title, content, format, lang, url, authors, md5, published_at, status, err_msg, deduped_by)
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
        $14)
returning *;

-- name: BatchCreate :execrows
-- -- timeout: 5s
INSERT INTO public.document (source, provider, catalog, title, content, format, lang, url, authors, md5, published_at, status, err_msg, deduped_by)
values (UNNEST(@source::varchar[]),
        UNNEST(@provider::varchar[]),
        UNNEST(@catalog::document_catalog[]),
        UNNEST(@title::varchar[]),
        UNNEST(@content::varchar[]),
        UNNEST(@format::document_format[]),
        UNNEST(@url::varchar[]),
        UNNEST(@authors::varchar[][]),
        UNNEST(@lang::varchar[]),
        UNNEST(@md5::varchar[]),
        UNNEST(@published_at::timestamptz[]),
        UNNEST(@status::document_status[]),
        UNNEST(@err_msg::varchar[]),
        UNNEST(@deduped_by::bigint[]));

-- name: GetById :one
-- -- timeout: 3s
SELECT id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at
FROM public.document
WHERE id = sqlc.arg('id');

-- name: GetByIdWithEmbedding :one
-- -- timeout: 5s
SELECT id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,embedding::float4[] as embedding,created_at,updated_at  
FROM public.document
WHERE id = sqlc.arg('id');

-- name: GetByMd5 :one
-- -- timeout: 5s
SELECT id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at 
FROM public.document k
WHERE k.md5 = $1;

-- name: GetByTitle :one
-- -- timeout: 5s
SELECT id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at
FROM public.document k
WHERE k.title = sqlc.arg('title')
and published_at >= sqlc.arg('published_at')
and status = coalesce(sqlc.narg('status')::document_status, k.status)
limit 1;

-- name: GetPendings :many
-- -- timeout: 5s
SELECT id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at 
FROM public.document k
WHERE k.status = 'pending'
    and coalesce(sqlc.narg('source'), k.source) = k.source;

-- name: SaveDraftToPending :one
-- -- timeout: 3s
UPDATE public.document
SET status = 'pending',
    content = sqlc.arg('content'),
    format = sqlc.arg('format'),
    updated_at = now()
WHERE id = sqlc.arg('id')
    and status = 'draft'
returning id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at;

-- name: UpdateStatus :one
-- -- timeout: 3s
UPDATE public.document
SET status = sqlc.arg('status'),
    err_msg = coalesce(sqlc.narg('err_msg'), err_msg),
    deduped_by = coalesce(sqlc.narg('deduped_by'), deduped_by),
    updated_at = now()
WHERE id = sqlc.arg('id')
    and status = sqlc.arg('prev_status')
returning id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at;

-- name: SaveAiSummary :one
-- -- timeout: 3s
UPDATE public.document
SET ai_title = sqlc.arg('ai_title'),
    ai_summary = sqlc.arg('ai_summary'),
    ai_tags = sqlc.arg('ai_tags'),
    ai_coins = sqlc.arg('ai_coins'),
    ai_influence = sqlc.arg('ai_influence'),
    ai_influence_score = sqlc.arg('ai_influence_score'),
    ai_sentiment = sqlc.arg('ai_sentiment'),
    status = sqlc.arg('status'),
    err_msg = coalesce(sqlc.narg('err_msg'), err_msg),
    deduped_by = coalesce(sqlc.narg('deduped_by'), deduped_by),
    updated_at = now()
WHERE id = sqlc.arg('id')
    and status = 'pending'
returning id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at;

-- name: ArchiveDocument :one
-- -- timeout: 3s
UPDATE public.document
SET status = 'archived'
WHERE id = sqlc.arg('id')
    and (status = 'pending' or status = 'active')
returning id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at;

-- name: CountDocuments :one
-- -- timeout: 5s
SELECT COUNT(1) 
FROM public.document k
WHERE 1 = 1
    and (
        sqlc.narg('keyword')::varchar is null
        or k.title @@@ sqlc.narg('keyword')::varchar
        or k.content @@@ sqlc.narg('keyword')::varchar
    )
    and k.id = coalesce(sqlc.narg('id')::bigint, k.id)
    and k.status = coalesce(sqlc.narg('status')::document_status, k.status)
    and k.source = coalesce(sqlc.narg('source')::varchar, k.source)
    and k.provider = coalesce(sqlc.narg('provider')::varchar, k.provider)
    and k.catalog = coalesce(sqlc.narg('catalog')::document_catalog, k.catalog)
    and (sqlc.narg('tag')::varchar is null or k.ai_tags @> ARRAY[sqlc.narg('tag')::varchar]::varchar[])
    and (sqlc.narg('coin')::varchar is null or k.ai_coins @> ARRAY[sqlc.narg('coin')::varchar]::varchar[])
    and k.ai_influence_score = coalesce(sqlc.narg('influence_score')::int, k.ai_influence_score)
    and k.ai_sentiment = coalesce(sqlc.narg('sentiment')::int, k.ai_sentiment)
    and k.id @@@ paradedb.range (
            field => 'published_at',
            range => tstzrange (
                    coalesce(
                            sqlc.narg ('published_at_start')::timestamptz,
                            to_timestamp(0)
                    ),
                    sqlc.narg ('published_at_end')::timestamptz,
                    '[)'
            )
        );

-- name: QueryDocuments :many
-- -- timeout: 5s
SELECT id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at,
    case
        when sqlc.narg('keyword')::varchar is null then 0::float4
        else coalesce(paradedb.score (k.id)::float4, 0)
    end as score
FROM public.document k
WHERE 1 = 1
    and (
        sqlc.narg('keyword')::varchar is null
        or k.title @@@ sqlc.narg('keyword')::varchar
        or k.content @@@ sqlc.narg('keyword')::varchar
    )
    and k.id = coalesce(sqlc.narg('id')::bigint, k.id)
    and k.status = coalesce(sqlc.narg('status')::document_status, k.status)
    and k.source = coalesce(sqlc.narg('source')::varchar, k.source)
    and k.provider = coalesce(sqlc.narg('provider')::varchar, k.provider)
    and k.catalog = coalesce(sqlc.narg('catalog')::document_catalog, k.catalog)
    and (sqlc.narg('tag')::varchar is null or k.ai_tags @> ARRAY[sqlc.narg('tag')::varchar]::varchar[])
    and (sqlc.narg('coin')::varchar is null or k.ai_coins @> ARRAY[sqlc.narg('coin')::varchar]::varchar[])
    and k.ai_influence_score = coalesce(sqlc.narg('influence_score')::int, k.ai_influence_score)
    and k.ai_sentiment = coalesce(sqlc.narg('sentiment')::int, k.ai_sentiment)
    and k.id @@@ paradedb.range (
            field => 'published_at',
            range => tstzrange (
                    coalesce(
                            sqlc.narg ('published_at_start')::timestamptz,
                            to_timestamp(0)
                    ),
                    sqlc.narg ('published_at_end')::timestamptz,
                    '[)'
            )
        )
order by score desc, published_at desc
offset sqlc.arg('offset') 
limit sqlc.arg('limit');

-- name: UpdateEmbedding :execrows
-- -- timeout: 10s
UPDATE public.document
SET embedding = sqlc.arg('embedding')::float4[]::vector
WHERE id = sqlc.arg('id');

-- name: GetDocumentStats :one
-- -- timeout: 5s
SELECT
  COUNT(*)::bigint AS total_count,
  COUNT(*) FILTER (WHERE status = 'active')::bigint AS success_count,
  COALESCE(AVG(EXTRACT(EPOCH FROM (created_at - published_at))) FILTER (WHERE created_at > published_at), 0)::float8 AS avg_publish_to_ingest_sec,
  COALESCE(AVG(EXTRACT(EPOCH FROM (updated_at - created_at))) FILTER (WHERE status = 'active'), 0)::float8 AS avg_ingest_to_success_sec
FROM public.document
WHERE created_at >= sqlc.arg('created_at_start')::timestamptz
  AND created_at < sqlc.arg('created_at_end')::timestamptz;

-- name: GetDocumentCountByChannel :many
-- -- timeout: 5s
SELECT source, provider,
  COUNT(*)::bigint AS document_count,
  COUNT(*) FILTER (WHERE status = 'active')::bigint AS success_count
FROM public.document
WHERE created_at >= sqlc.arg('created_at_start')::timestamptz
  AND created_at < sqlc.arg('created_at_end')::timestamptz
GROUP BY source, provider;

-- name: SemanticSearch :many
-- -- timeout: 5s
SELECT id,source,provider,catalog,title,content,ai_title,ai_summary,ai_tags,ai_coins,ai_influence,ai_influence_score,ai_sentiment,format,authors,lang,url,md5,published_at,status,err_msg,deduped_by,created_at,updated_at,(1 - (embedding <=> sqlc.arg('embedding')::float4[]::vector))::float4 as similarity
FROM public.document
WHERE (1 - (embedding <=> sqlc.arg('embedding')::float4[]::vector)) >= sqlc.arg('threshold')::float4
    and published_at >= sqlc.arg('published_at_start')::timestamptz
    and published_at <= sqlc.arg('published_at_end')::timestamptz
    and status = sqlc.arg('status')::document_status
    and (sqlc.narg('excludes')::bigint[] is null or id != any(sqlc.narg('excludes')::bigint[]))
order by similarity desc, published_at desc
limit sqlc.arg('top_k');

-- name: DeleteOldDocuments :one
-- -- timeout: 10s
DELETE FROM public.document WHERE created_at < sqlc.arg('cutoff_time')
RETURNING id;
