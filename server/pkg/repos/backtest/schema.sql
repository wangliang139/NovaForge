-- 回测结果表
CREATE TABLE IF NOT EXISTS backtest_results (
    id VARCHAR(64) PRIMARY KEY,
    job_id VARCHAR(64) NOT NULL,              -- 回测任务ID（一次性，不依赖bot）
    strategy_id VARCHAR(64) NOT NULL,
    strategy_version VARCHAR(32) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    symbol VARCHAR(64) NOT NULL,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    initial_balance DECIMAL(32, 8) NOT NULL,
    final_balance DECIMAL(32, 8) NOT NULL,
    total_pnl DECIMAL(32, 8) NOT NULL,
    total_trades INT NOT NULL DEFAULT 0,
    win_trades INT NOT NULL DEFAULT 0,
    loss_trades INT NOT NULL DEFAULT 0,
    win_rate DECIMAL(5, 2),
    sharpe_ratio DECIMAL(10, 4),
    max_drawdown DECIMAL(10, 4),
    result_data JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_backtest_results_job_id ON backtest_results(job_id);
CREATE INDEX idx_backtest_results_strategy ON backtest_results(strategy_id, strategy_version);
CREATE INDEX idx_backtest_results_created_at ON backtest_results(created_at);
