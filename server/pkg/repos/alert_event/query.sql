-- name: CreateAlertTriggerEvent :one
-- -- timeout: 1s
INSERT INTO public.alert_event (
    id, alert_id, exchange, symbol, type, frequency, target_price, alert_window, percent, baseline_price, trigger_price, triggered_at, notify_result, error_message, meta
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
RETURNING *;

-- name: CleanupAlertTriggerEventsBefore :execrows
-- -- timeout: 1s
DELETE
FROM public.alert_event
WHERE triggered_at < $1;
