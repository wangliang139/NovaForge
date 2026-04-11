# Bot 策略监控指标技术方案

## 一、概述

本文档描述 Bot 最小监控指标集的实现方案，包括数据模型、接口设计、计算逻辑及服务职责划分。

### 1.1 指标列表（12 项）

| 指标 | 英文 | 类别 |
|------|------|------|
| 年化复合收益率 | CAGR | 收益类 |
| 夏普比率 | Sharpe | 风险类 |
| 索提诺比率 | Sortino | 风险类 |
| 最大回撤 | Max Drawdown | 风险类 |
| 回撤持续时间 | Time Under Water | 风险类 |
| 卡玛比率 | Calmar | 风险类 |
| 胜率 | Win Rate | 盈亏结构 |
| 盈亏比因子 | Profit Factor | 盈亏结构 |
| 滚动夏普 | Rolling Sharpe | 稳定性 |
| 平均滑点 | Avg Slippage | 执行质量 |
| 手续费占比 | Fee Ratio | 执行质量 |
| 最大连续亏损 | Max Consecutive Loss | 稳定性 |

### 1.2 接口与数据流

| 接口 | 归属服务 | 查询参数 | 数据范围 |
|------|----------|----------|----------|
| QueryAccountMetrics | llt-data-api | account_id | 账户下全部数据 |
| QueryBotMetrics | llt-strategy-api | bot_id | 仅该 Bot 归属数据 |

---

## 二、数据模型

### 2.1 新增表：symbol_equity

**位置**：llt-strategy-api（与 bots 同库，便于 Bot 维度查询）

**用途**：存储每个 Bot 下各标的的权益时间序列，支撑 Symbol 级 CAGR/Sharpe/MDD 等指标。

```sql
CREATE TABLE IF NOT EXISTS symbol_equity (
    id BIGSERIAL PRIMARY KEY,
    bot_id INT NOT NULL,
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    symbol VARCHAR(64) NOT NULL,
    net_value DECIMAL(32, 8) NOT NULL,
    base_currency VARCHAR(16) NOT NULL DEFAULT 'USDT',
    ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_symbol_equity_uk ON symbol_equity(bot_id, exchange, symbol, ts);
CREATE INDEX idx_symbol_equity_bot_ts ON symbol_equity(bot_id, ts);
CREATE INDEX idx_symbol_equity_symbol ON symbol_equity(bot_id, exchange, symbol);
```

**写入时机**：每小时整点，与 llt-data-api 的 `RefreshAccountEquity` 同步（方案 A）。

**写入逻辑**：llt-strategy-api 定时任务，每小时执行：
1. 查询 `status=running` 的 Bot 列表
2. 对每个 Bot，通过 gRPC 获取其 account 的 assets + positions（llt-data-api）
3. 按 Bot.symbols 拆分，计算每个 symbol 的 net_value（BaseCurrency 计价）
4. 批量写入 symbol_equity

---

## 三、服务职责与实现

### 3.1 llt-data-api

#### 3.1.1 新增 gRPC：QueryAccountMetrics

**Proto 定义**（建议放在 `account/v1/account.proto` 或新建 `metrics/v1/metrics.proto`）：

```protobuf
message QueryAccountMetricsRequest {
  string account_id = 1;
  MetricsDimension dimension = 2;  // ACCOUNT | SYMBOL
  optional string symbol = 3;      // dimension=SYMBOL 时可选
  optional int64 start_ts = 4;
  optional int64 end_ts = 5;
}

message AccountMetricsResponse {
  string account_id = 1;
  MetricsDimension dimension = 2;
  double cagr = 3;
  double sharpe = 4;
  double sortino = 5;
  double max_drawdown = 6;
  int64 time_under_water_seconds = 7;
  double calmar = 8;
  double win_rate = 9;
  double profit_factor = 10;
  double rolling_sharpe = 11;
  double avg_slippage_bps = 12;
  double fee_ratio = 13;
  int32 max_consecutive_loss = 14;
  int64 start_ts = 15;
  int64 end_ts = 16;
  repeated SymbolMetrics symbols = 17;  // dimension=SYMBOL 时
}
```

**数据来源**：
- 权益曲线：`equity` 表（account_id）
- Symbol 权益：`symbol_equity` 表（account_id 反查，若 account 无 Bot 则无 symbol 级）
- 订单：`orders` 表（account_id）

**实现路径**：
- `pkg/entity/account/metrics.go`：封装指标计算逻辑
- `pkg/service/accountsvc/svc.go`：或新建 `metricssvc`，实现 gRPC 接口

#### 3.1.2 新增 gRPC：ListOrdersByBotId

**用途**：供 llt-strategy-api 的 QueryBotMetrics 使用，按 bot_id 过滤订单。

**Proto 定义**（建议放在 `order/v1/order.proto`）：

```protobuf
message QueryOrdersByBotIdRequest {
  int32 bot_id = 1;
  optional string symbol = 2;
  optional int64 start_ts = 3;
  optional int64 end_ts = 4;
  optional int32 page = 5;
  optional int32 size = 6;
}

message QueryOrdersByBotIdResponse {
  repeated Order orders = 1;
  int32 total_count = 2;
}
```

**实现**：
- `pkg/repos/orders/query.sql`：新增 `ListOrdersByBotId`、`CountOrdersByBotId`
- `pkg/entity/account/order.go` 或 `pkg/entity/order/`：新增 `ListOrdersByBotId`
- `pkg/service/ordersvc/svc.go`：或新建 `QueryOrdersByBotId`（需 account 服务暴露）

**注意**：orders 表已有 `bot_id` 字段，仅需新增按 bot_id 的查询。

---

### 3.2 llt-strategy-api

#### 3.2.1 新增 gRPC：QueryBotMetrics

**Proto 定义**（`strategy/v1/strategy.proto`）：

```protobuf
message QueryBotMetricsRequest {
  int32 bot_id = 1;
  MetricsDimension dimension = 2;  // ACCOUNT | SYMBOL
  optional string symbol = 3;        // 必须在 Bot.symbols 内
  optional int64 start_ts = 4;
  optional int64 end_ts = 5;
}

message BotMetricsResponse {
  string account_id = 1;
  int32 bot_id = 2;
  MetricsDimension dimension = 3;
  string symbols_filter = 4;       // Bot.symbols 白名单
  double cagr = 5;
  double sharpe = 6;
  ...
  repeated SymbolMetrics symbols = 17;
}
```

**实现流程**：
1. 校验 bot_id，获取 Bot 信息（含 symbols）
2. 若 dimension=SYMBOL 且传入 symbol，校验 symbol 在 Bot.symbols 内
3. 调用 llt-data-api：
   - `ListOrdersByBotId`（bot_id）
   - `ListEquityByAccountAndRange`（account_id，从 Bot 获取）
   - 查询 symbol_equity（bot_id，symbol 仅限 Bot.symbols）
4. 在 strategy-api 内计算 12 项指标，Symbol 级结果按 symbols 白名单过滤

#### 3.2.2 新增定时任务：RefreshBotSymbolEquity

**用途**：每小时写入 symbol_equity。

**实现**：
- `pkg/entity/strategy/symbol_equity.go`：`CalculateSymbolEquity`、`UpsertSymbolEquity`
- `pkg/cronjob/` 或 Hatchet：注册 `RefreshBotSymbolEquity`，cron `0 * * * *`（每小时整点）
- 流程：
  1. `ListBots(status=running)`
  2. 对每个 Bot，调用 data-api gRPC 获取 `GetAssets`、`GetPositions`（account_id）
  3. 按 Bot.symbols + Bot.exchange 拆分，计算每个 symbol 的 net_value
  4. 批量 `INSERT INTO symbol_equity`

**net_value 计算**：参考 `CalculateAccountEquity`，按 symbol 过滤 assets（base/quote 属于该 symbol）和 positions（symbol 匹配），换算为 USDT。

---

### 3.3 llt-backoffice-gateway

**GraphQL Schema**：

```graphql
enum MetricsDimension {
  ACCOUNT
  SYMBOL
}

type AccountMetrics {
  accountId: String!
  dimension: MetricsDimension!
  cagr: Float
  sharpe: Float
  sortino: Float
  maxDrawdown: Float
  timeUnderWaterSeconds: Int
  calmar: Float
  winRate: Float
  profitFactor: Float
  rollingSharpe: Float
  avgSlippageBps: Float
  feeRatio: Float
  maxConsecutiveLoss: Int
  startTs: Int!
  endTs: Int!
  symbols: [SymbolMetrics!]
}

type SymbolMetrics {
  symbol: String!
  exchange: String!
  cagr: Float
  sharpe: Float
  # ... 同上 12 项
}

type BotMetrics {
  accountId: String!
  botId: Int!
  dimension: MetricsDimension!
  cagr: Float
  # ... 同上
  symbols: [SymbolMetrics!]
}

type Query {
  AccountMetrics(accountId: String!, dimension: MetricsDimension!, symbol: String, startTs: Int, endTs: Int): AccountMetrics!
  BotMetrics(botId: Int!, dimension: MetricsDimension!, symbol: String, startTs: Int, endTs: Int): BotMetrics!
}
```

**Resolver**：
- `AccountMetrics` → 调用 llt-data-api `QueryAccountMetrics`
- `BotMetrics` → 调用 llt-strategy-api `QueryBotMetrics`

---

## 四、指标计算逻辑

### 4.1 通用公式

| 指标 | 公式 | 数据依赖 |
|------|------|----------|
| CAGR | (V_end/V_start)^(1/T)-1 | 权益序列首尾 |
| Sharpe | mean(r)/std(r)*sqrt(N) | 权益序列收益率 |
| Sortino | mean(r)/std_down(r) | 权益序列，仅负收益 |
| Max Drawdown | max((Peak-NV)/Peak) | 权益序列 |
| Time Under Water | max(t_recovery - t_peak) | 权益序列 |
| Calmar | CAGR / MDD | CAGR + MDD |
| Win Rate | WinTrades/(WinTrades+LossTrades) | orders.realized_pnl |
| Profit Factor | GrossProfit / GrossLoss | orders.realized_pnl |
| Rolling Sharpe | 同 Sharpe，窗口内子序列 | 权益序列 |
| Avg Slippage | sum((avgPrice-price)/price*Notional)/sum(Notional)*10000 | orders |
| Fee Ratio | TotalFee / GrossPnL | orders.fee |
| Max Consecutive Loss | 连续 realized_pnl<0 最大长度 | orders |

### 4.2 边界处理

- 无数据：返回 null 或 0，视业务约定
- T < 7 天：CAGR、Calmar 可不展示
- MDD=0：Calmar 返回 null
- GrossLoss=0：Profit Factor 返回约定值（如 999）
- 限价单：Slippage = (avg_price - price) / price；市价单不参与计算

### 4.3 计算包复用

建议在 `common/go` 或 `llt-strategy-api/pkg/strategy/executor/backtest/collectors/` 中抽取公共计算函数：
- `CalculateSharpeRatio`、`CalculateMaxDrawdown` 已存在于 `collectors/calculator.go`
- 新增：`CalculateSortino`、`CalculateCAGR`、`CalculateTimeUnderWater`、`CalculateCalmar`、`CalculateRollingSharpe`
- 成交类：`CalculateWinRate`、`CalculateProfitFactor`、`CalculateFeeRatio`、`CalculateMaxConsecutiveLoss`、`CalculateAvgSlippage`

---

## 五、数据依赖矩阵

| 指标 | 账户级 | Symbol 级 | 数据表 |
|------|--------|-----------|--------|
| CAGR、Sharpe、Sortino、MDD、TUW、Calmar、Rolling Sharpe | equity | symbol_equity | equity / symbol_equity |
| Win Rate、Profit Factor、Fee Ratio、Max Consecutive Loss | orders(account_id) | orders(bot_id,symbol) | orders |
| Avg Slippage | orders(account_id) | orders(bot_id,symbol) | orders |

---

## 六、实施顺序

1. **Phase 1**：symbol_equity 表 + RefreshBotSymbolEquity 定时任务（已完成）
2. **Phase 2**：llt-data-api 新增 ListOrdersByBotId、QueryAccountMetrics
3. **Phase 3**：llt-strategy-api 新增 QueryBotMetrics
4. **Phase 4**：Gateway GraphQL + 前端展示

### Phase 1 实施说明

执行建表（在 llt-strategy-api 使用的 PostgreSQL 库中）：

```sql
-- 见 pkg/repos/symbol_equity/schema.sql
CREATE TABLE IF NOT EXISTS symbol_equity (...);
CREATE UNIQUE INDEX idx_symbol_equity_uk ON symbol_equity(bot_id, exchange, symbol, ts);
...
```

定时任务 `RefreshBotSymbolEquity` 已注册，每小时整点执行。

---

## 七、附录

### A. symbol_equity 表归属说明

symbol_equity 放在 llt-strategy-api 的原因：
- 与 bots 表同库，便于按 bot_id 查询
- 写入由 strategy-api 的定时任务触发，需获取 Bot 列表及 symbols
- 避免 data-api 依赖 strategy-api 的 Bot 元数据

### B. 权益序列格式

- equity：`{ts, notional}`，account_id 维度
- symbol_equity：`{ts, net_value}`，按 (bot_id, exchange, symbol) 维度

### C. 回测与实盘一致性

回测指标计算已存在于 `collectors/calculator.go`，实盘指标可复用相同公式，仅数据源从内存换为 DB 查询。

### D. Symbol 级 net_value 计算说明

单标的净值需按 symbol 拆分，可采用简化口径：

- **持仓贡献**：`positions` 中该 symbol 的 `qty * mark_price`（多空合并后换算 USDT）
- **已实现盈亏贡献**：`orders` 中该 symbol、`bot_id` 匹配的 `realized_pnl` 累计
- **net_value** = 持仓市值 + 累计已实现盈亏（均以 BaseCurrency 计价）

资产（assets）为账户级共享，不按 symbol 拆分；Symbol 级权益侧重该标的的盈亏表现。
