-- name: ListAlertsByExchangeSymbol :many
-- -- timeout: 1s
SELECT *
FROM public.alert
WHERE exchange = $1
  AND symbol = $2
  AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: ListAllActiveAlerts :many
-- -- timeout: 1s
SELECT *
FROM public.alert
WHERE status = 'active'
  AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: CreateAlert :one
-- -- timeout: 1s
INSERT INTO public.alert (
    id, exchange, symbol, type, frequency, price, alert_window, percent, remark, cooldown_seconds
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: DeleteAlertByID :execrows
-- -- timeout: 1s
UPDATE public.alert
SET deleted_at = now(),
    updated_at = now()
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetAlertByID :one
-- -- timeout: 1s
SELECT *
FROM public.alert
WHERE id = $1
  AND deleted_at IS NULL;

-- name: CountAlerts :one
-- -- timeout: 1s
SELECT count(*)
FROM public.alert
WHERE deleted_at IS NULL;

-- name: CountAlertsByExchangeSymbol :one
-- -- timeout: 1s
SELECT count(*)
FROM public.alert
WHERE exchange = $1
  AND symbol = $2
  AND deleted_at IS NULL;

-- name: TouchAlertTriggered :exec
-- -- timeout: 1s
UPDATE public.alert
SET last_triggered_at = $2,
    trigger_count     = trigger_count + 1,
    updated_at        = now()
WHERE id = $1
  AND deleted_at IS NULL;

