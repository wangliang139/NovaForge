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

| 接口 | 归属（单体） | 查询参数 | 数据范围 |
|------|--------------|----------|----------|
| QueryAccountMetrics | `server` 内账户/权益/订单域 | account_id | 账户下全部数据 |
| QueryBotMetrics | `server` 内策略/Bot 域 | bot_id | 仅该 Bot 归属数据 |

---

## 二、数据模型

### 2.1 新增表：symbol_equity

**位置**：与 `bots` 同一 PostgreSQL 库（`server/pkg/repos`），便于 Bot 维度查询

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

**写入时机**：每小时整点，与账户侧 `RefreshAccountEquity` 类任务对齐（方案 A）。

**写入逻辑**：`server` 进程内定时任务，每小时执行：
1. 查询 `status=running` 的 Bot 列表
2. 对每个 Bot，在进程内查询其 account 的 assets + positions（账户/持仓仓库或服务）
3. 按 Bot.symbols 拆分，计算每个 symbol 的 net_value（BaseCurrency 计价）
4. 批量写入 symbol_equity

---

## 三、服务职责与实现（单体 NovaForge `server`）

以下为能力拆分说明：在微服务时代可能以 gRPC 暴露；在**当前单体**中，等价能力在 **同一进程** 内以包与 GraphQL resolver 实现，无需跨网络。下文保留消息结构示例，便于对齐字段含义。

### 3.1 账户与订单域（数据与指标）

#### 3.1.1 QueryAccountMetrics（对内 API 或 GraphQL 解析链）

**请求/响应形状示例**（历史上可为 Proto；实现时可落在 Go 结构体 + GraphQL）：

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
- `server/pkg/entity/...`：封装指标计算逻辑
- `server/pkg/service/...`：聚合查询与缓存（按现有分层放置）

#### 3.1.2 ListOrdersByBotId

**用途**：供 `QueryBotMetrics` 使用，按 `bot_id` 过滤订单。

**请求/响应形状示例**：

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
- `server/pkg/repos/orders/query.sql`：新增 `ListOrdersByBotId`、`CountOrdersByBotId`
- `server/pkg/entity/...`：订单列表实体
- `server/pkg/service/...`：对外暴露查询

**注意**：orders 表已有 `bot_id` 字段，仅需新增按 bot_id 的查询。

---

### 3.2 策略与 Bot 域

#### 3.2.1 QueryBotMetrics

**请求/响应形状示例**：

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
3. 在进程内调用账户/订单/权益数据访问层：
   - `ListOrdersByBotId`（bot_id）
   - `ListEquityByAccountAndRange`（account_id，从 Bot 获取）
   - 查询 symbol_equity（bot_id，symbol 仅限 Bot.symbols）
4. 在策略/Bot 相关包内计算 12 项指标，Symbol 级结果按 symbols 白名单过滤

#### 3.2.2 定时任务：RefreshBotSymbolEquity

**用途**：每小时写入 symbol_equity。

**实现**：
- `server/pkg/entity/strategy/...`：`CalculateSymbolEquity`、`UpsertSymbolEquity`（路径以仓库为准）
- `server` 内定时任务注册：`RefreshBotSymbolEquity`，cron `0 * * * *`（每小时整点）
- 流程：
  1. `ListBots(status=running)`
  2. 对每个 Bot，在进程内获取 `GetAssets`、`GetPositions`（account_id）
  3. 按 Bot.symbols + Bot.exchange 拆分，计算每个 symbol 的 net_value
  4. 批量 `INSERT INTO symbol_equity`

**net_value 计算**：参考 `CalculateAccountEquity`，按 symbol 过滤 assets（base/quote 属于该 symbol）和 positions（symbol 匹配），换算为 USDT。

---

### 3.3 GraphQL 网关层（gqlgen）

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
- `AccountMetrics` → 调用进程内账户指标实现（等价于上文 `QueryAccountMetrics`）
- `BotMetrics` → 调用进程内 Bot 指标实现（等价于上文 `QueryBotMetrics`）

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

建议在 `server/pkg/strategy/executor/backtest/collectors/`（或同职责包）中抽取公共计算函数：
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
2. **Phase 2**：在 `server` 内补齐 ListOrdersByBotId、QueryAccountMetrics 能力
3. **Phase 3**：在 `server` 内补齐 QueryBotMetrics 能力
4. **Phase 4**：GraphQL（gqlgen）与 `frontend` 展示

### Phase 1 实施说明

执行建表（在 NovaForge 使用的 PostgreSQL 库中）：

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

symbol_equity 与 Bot 同库的原因：
- 与 bots 表同库，便于按 bot_id 查询
- 写入由策略侧的定时任务触发，需获取 Bot 列表及 symbols
- 避免账户/行情域反向依赖 Bot 元数据

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
