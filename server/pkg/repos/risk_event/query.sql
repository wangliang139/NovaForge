-- name: CreateRiskEvent :one
-- -- timeout: 1s
INSERT INTO public.risk_event (
    account_id,
    exchange,
    rule,
    risk_index,
    payload
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: ListRiskEventsByAccount :many
-- -- timeout: 1s
SELECT
    id,
    account_id,
    exchange,
    rule,
    risk_index,
    payload,
    created_at
FROM public.risk_event
WHERE account_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

