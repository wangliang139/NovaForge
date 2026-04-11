
-- 回测结果不再依赖 bot，关联回测任务及策略信息
INSERT INTO backtest_results (id, job_id, strategy_id, strategy_version, exchange, symbol, start_time, end_time, initial_balance, final_balance, total_pnl, total_trades, win_trades, loss_trades, win_rate, sharpe_ratio, max_drawdown, result_data, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
RETURNING *;

-- name: GetBacktestResult :one
-- -- timeout: 1s
SELECT * FROM backtest_results
WHERE id = $1;

-- name: ListBacktestResults :many
-- -- timeout: 1s
SELECT * FROM backtest_results
WHERE ($1::varchar IS NULL OR job_id = $1)
  AND ($2::varchar IS NULL OR strategy_id = $2)
ORDER BY created_at DESC;
