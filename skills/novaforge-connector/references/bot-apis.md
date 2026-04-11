# Bot 接口

## 常用枚举

- `BotMode`：`live` | `paper`
- `BotStatus`：`stopped` | `running` | `error`
- `WalletType`：`fund` | `trade` | `spot` | `future` | `margin` | `unspecified`
- `MarketType`：`spot` | `future` | `unspecified`
- `OrderType`：`market` | `limit` | `none`
- `OrderSource`：`user` | `strategy` | `liquidation` | `adl`
- `MetricsDimension`：`account` | `symbol` | `unspecified`

## 共享结构

### `Bot`

```json
{
  "id": "1",
  "strategyId": "s1",
  "strategyVer": "v1",
  "strategyName": "demo-strategy",
  "upgradable": false,
  "mode": "paper",
  "name": "demo-bot",
  "description": "example",
  "accountId": "1",
  "exchange": "binance",
  "symbols": ["BTCUSDT"],
  "config": "{}",
  "status": "stopped",
  "errorMessage": "",
  "createdAt": 0,
  "startedAt": 0,
  "stoppedAt": 0
}
```

- `strategyId` / `strategyVer`：绑定的策略与版本
- `strategyName`：策略显示名
- `upgradable`：是否存在可升级版本
- `mode`：实盘或模拟
- `accountId`：绑定账户
- `symbols`：Bot 负责的标的列表
- `config`：Bot 运行配置，通常为 JSON 字符串
- `status`：当前运行状态
- `errorMessage`：最近错误信息

### `BotPortfolioAsset`

```json
{
  "exchange": "binance",
  "walletType": "future",
  "asset": "USDT",
  "free": "1000",
  "frozen": "50",
  "updatedTs": 0
}
```

- `free`：可用余额
- `frozen`：冻结余额

### `BotPortfolioPosition`

```json
{
  "exchange": "binance",
  "symbol": "BTCUSDT",
  "marketType": "future",
  "side": "long",
  "qty": "0.01",
  "avgPrice": "60000",
  "updatedTs": 0,
  "leverage": 10
}
```

- `qty`：持仓数量
- `avgPrice`：持仓均价

### `BotPortfolio`

```json
{
  "assets": [],
  "positions": [],
  "ts": 0
}
```

- `assets`：资产快照
- `positions`：持仓快照
- `ts`：状态时间戳

### `BotState`

```json
{
  "botStatus": "running",
  "executorStatus": "running",
  "jsRunnerStatus": "healthy",
  "portfolio": {},
  "runErr": "",
  "signalAvgDurationMs": 5,
  "signalAvgLatencyMs": 20,
  "lastSignalTs": 0
}
```

- `botStatus`：Bot 状态
- `executorStatus`：执行器状态文本
- `jsRunnerStatus`：JS runner 状态文本
- `portfolio`：组合资产与持仓
- `runErr`：运行错误
- `signalAvgDurationMs`：信号平均处理耗时
- `signalAvgLatencyMs`：信号平均延迟
- `lastSignalTs`：最近信号时间

### `BotMetrics`

```json
{
  "accountId": "1",
  "botId": 1,
  "dimension": "account",
  "symbolsFilter": "",
  "cagr": 0,
  "sharpe": 0,
  "sortino": 0,
  "maxDrawdown": 0,
  "timeUnderWaterSeconds": 0,
  "calmar": 0,
  "winRate": 0,
  "profitFactor": 0,
  "rollingSharpe": 0,
  "avgSlippageBps": 0,
  "feeRatio": 0,
  "maxConsecutiveLoss": 0,
  "startTs": 0,
  "endTs": 0,
  "symbols": []
}
```

- 各指标语义与账户指标一致
- `symbolsFilter`：参与统计的标的过滤条件

### `UpgradeBotResult`

```json
{
  "success": true,
  "message": "upgraded",
  "bot": {}
}
```

- `success`：是否成功
- `message`：补充说明
- `bot`：升级后的 Bot

### 复用返回结构

- `BotBalance` 返回 `Balance`，字段结构与账户文档中的 `Balance` 一致
- `BotPositions` 返回 `Position[]`，字段结构与账户文档中的 `Position` 一致
- `BotOrders` 返回 `OrdersConnection`，其中 `list` 元素字段结构与账户文档中的 `Order` 一致
- `BotLedger` 返回 `LedgersConnection`，其中 `list` 元素字段包含 `id`、`accountId`、`exchange`、`asset`、`walletType`、`total`、`frozen`、`totalDelta`、`frozenDelta`、`type`、`detail`、`isEffective`、`ts`、`createdAt`
- `BotEquity` 返回：

```json
{
  "totalCount": 0,
  "list": []
}
```

- `list` 元素为 `Equity`，字段包含 `id`、`accountId`、`ts`、`notional`、`unRealizedProfit`、`createdAt`

## 接口

### `Bots`

- 输入：

```json
{
  "input": {
    "id": "1",
    "limit": 20,
    "offset": 0,
    "strategyId": "s1",
    "mode": "paper",
    "exchange": "binance",
    "status": "running",
    "accountId": "1",
    "name": "demo",
    "createdAtStart": 0,
    "createdAtEnd": 0
  }
}
```

- 返回：

```json
{
  "data": {
    "Bots": {
      "totalCount": 0,
      "list": []
    }
  }
}
```

- `list` 元素结构：`Bot`

### `Bot`

- 输入：

```json
{
  "id": 1
}
```

- 返回：单个 `Bot`，可能为 `null`

### `BotBalance`

- 输入：

```json
{
  "input": {
    "botId": 1,
    "walletType": "future",
    "asset": "USDT",
    "withNotional": true
  }
}
```

- 返回结构：`Balance`

### `BotPositions`

- 输入：

```json
{
  "input": {
    "botId": 1,
    "marketType": "future",
    "symbol": "BTCUSDT"
  }
}
```

- 返回：`Position[]`

### `BotState`

- 输入：

```json
{
  "input": {
    "botId": 1
  }
}
```

- 返回结构：`BotState`

### `BotOrders`

- 输入：

```json
{
  "input": {
    "botId": 1,
    "symbol": "BTCUSDT",
    "orderType": "limit",
    "orderSource": "strategy",
    "includeFinished": true,
    "page": 1,
    "size": 20
  }
}
```

- 返回：

```json
{
  "data": {
    "BotOrders": {
      "totalCount": 0,
      "list": []
    }
  }
}
```

- `list` 元素结构：`Order`

### `BotLedger`

- 输入：

```json
{
  "input": {
    "botId": 1,
    "walletType": "future",
    "asset": "USDT",
    "startTs": 0,
    "endTs": 0,
    "page": 1,
    "size": 20
  }
}
```

- 返回：

```json
{
  "data": {
    "BotLedger": {
      "totalCount": 0,
      "list": []
    }
  }
}
```

- `list` 元素结构：`Ledger`

### `BotEquity`

- 输入：

```json
{
  "botId": 1,
  "startTs": 0,
  "endTs": 0
}
```

- 返回：`{ totalCount, list }`，其中 `list` 元素为 `Equity`

### `BotMetrics`

- 输入：

```json
{
  "input": {
    "botId": 1,
    "dimension": "account",
    "symbol": "BTCUSDT",
    "startTs": 0,
    "endTs": 0
  }
}
```

- 返回结构：`BotMetrics`

### `CreateBot`

- 输入：

```json
{
  "input": {
    "name": "demo-bot",
    "description": "example",
    "strategyId": "s1",
    "strategyVer": "v1",
    "mode": "paper",
    "exchange": "binance",
    "symbols": ["BTCUSDT"],
    "accountId": "1",
    "config": "{\"risk\":{}}"
  }
}
```

- 字段说明：
  - `strategyId` / `strategyVer`：绑定策略版本
  - `mode`：`live` 或 `paper`
  - `symbols`：运行标的列表
  - `config`：Bot 配置 JSON 字符串
- 返回结构：新建后的 `Bot`

### `UpdateBot`

- 输入：

```json
{
  "input": {
    "id": 1,
    "name": "demo-bot-v2",
    "description": "updated",
    "symbols": ["BTCUSDT", "ETHUSDT"],
    "config": "{\"risk\":{\"maxPosition\":2}}"
  }
}
```

- 返回结构：更新后的 `Bot`

### `StartBot`

- 输入：

```json
{
  "id": 1
}
```

- 返回：布尔值

### `StopBot`

- 输入与返回同 `StartBot`

### `UpgradeBot`

- 输入：

```json
{
  "id": 1
}
```

- 返回结构：`UpgradeBotResult`

### `DeleteBot`

- 输入：

```json
{
  "id": 1
}
```

- 返回：布尔值
