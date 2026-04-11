# 策略与回测接口

## 常用枚举

- `StrategyStatus`：`draft` | `active` | `inactive` | `unspecified`
- `SignalType`：`kline` | `trade` | `depth` | `ticker` | `mark_price` | `social` | `timer` | `order` | `position` | `balance` | `risk` | `system` | `unspecified`
- `SignalScope`：`symbol` | `target` | `exchange` | `strategy` | `unspecified`

## 共享结构

### `StrategyParamInput` / `StrategyParam`

```json
{
  "name": "window",
  "description": "lookback window",
  "type": "number",
  "required": true,
  "default": "20"
}
```

- `name`：参数名
- `description`：参数说明
- `type`：参数类型描述，例如 `string`、`number`、`boolean`、`json`
- `required`：是否必填
- `default`：默认值，字符串表示

### `SignalDefinitionInput` / `SignalDefinition`

```json
{
  "id": "ticker-btc",
  "type": "ticker",
  "exchange": "binance",
  "symbol": "BTCUSDT",
  "props": "{\"interval\":\"1m\"}",
  "scope": "symbol"
}
```

- `id`：信号定义 ID，需在策略内唯一
- `type`：信号类型
- `exchange`：信号来源交易所
- `symbol`：关联标的
- `props`：扩展配置，通常为 JSON 字符串
- `scope`：信号作用范围

### `Strategy`

```json
{
  "id": "s1",
  "name": "demo-strategy",
  "description": "example",
  "code": "export default {}",
  "version": "v1",
  "params": [],
  "status": "draft",
  "signals": [],
  "createdAt": 0,
  "updatedAt": 0
}
```

- `id`：策略 ID
- `name`：策略名
- `description`：描述
- `code`：策略代码
- `version`：当前版本号
- `params`：参数定义数组
- `status`：草稿 / 激活 / 停用
- `signals`：信号定义数组

### `GenerateStrategyResponse`

```json
{
  "sessionId": "sess-1",
  "content": "generated strategy draft"
}
```

- `sessionId`：生成会话 ID
- `content`：生成的策略草稿内容

### `BacktestSymbolInput`

```json
{
  "exchange": "binance",
  "symbol": "BTCUSDT",
  "baseAssetQty": "0",
  "quoteAssetQty": "10000"
}
```

- `exchange`：交易所
- `symbol`：标的
- `baseAssetQty`：初始基础资产数量
- `quoteAssetQty`：初始计价资产数量

### `BacktestSignalInput`

```json
{
  "signalId": "ticker-btc",
  "datasourceId": 1,
  "exchange": "binance",
  "symbol": "BTCUSDT"
}
```

- `signalId`：对应策略中的信号定义 ID
- `datasourceId`：回测使用的数据源 ID
- `exchange` / `symbol`：可选覆盖

### `SymbolSummary`

```json
{
  "exchange": "binance",
  "symbol": "BTCUSDT",
  "base": "BTC",
  "quote": "USDT",
  "initialBase": "0",
  "initialQuote": "10000",
  "finalBase": "0.01",
  "finalQuote": "9800",
  "positionQty": "0.01",
  "avgPrice": "60000",
  "lastPrice": "60500",
  "initialNet": "10000",
  "finalNet": "10050",
  "realizedPnl": "20",
  "unrealizedPnl": "30",
  "netPnl": "50",
  "longRealizedPnl": "20",
  "shortRealizedPnl": "0",
  "longUnrealizedPnl": "30",
  "shortUnrealizedPnl": "0",
  "longNetPnl": "50",
  "shortNetPnl": "0",
  "longTrades": 2,
  "shortTrades": 0
}
```

- `initial*` / `final*`：回测前后资产状态
- `positionQty`：最终持仓
- `avgPrice` / `lastPrice`：平均成交价与最新价
- `realizedPnl` / `unrealizedPnl` / `netPnl`：盈亏汇总
- `long*` / `short*`：分多空统计

### `RunBacktestResponse`

```json
{
  "id": "bt-1",
  "strategy": {},
  "startTime": 0,
  "endTime": 0,
  "initialBalance": "10000",
  "finalBalance": "10100",
  "totalPnl": "100",
  "totalTrades": 10,
  "winTrades": 6,
  "lossTrades": 4,
  "winRate": 0.6,
  "sharpeRatio": 1.2,
  "maxDrawdown": 0.08,
  "data": {
    "symbols": [],
    "equity": [],
    "orders": [],
    "fills": [],
    "metaJson": "{}"
  },
  "createdAt": 0,
  "timeCost": 200,
  "consoleLogs": []
}
```

- `strategy`：回测时采用的策略快照
- `initialBalance` / `finalBalance`：起始与结束资金
- `totalPnl`：总盈亏
- `totalTrades` / `winTrades` / `lossTrades` / `winRate`：交易统计
- `sharpeRatio` / `maxDrawdown`：回测风险指标
- `data.symbols`：标的级汇总，元素结构见 `SymbolSummary`
- `data.equity`：权益曲线，字段结构与账户文档中的 `Equity` 一致
- `data.orders`：订单列表，字段结构与账户文档中的 `Order` 一致
- `data.fills`：成交列表，包含 `exchange`、`symbol`、`orderId`、`clientOrderId`、`side`、`isBuy`、`qty`、`price`、`fee`、`feeAsset`、`realizedPnl`、`isMaker`、`ts`
- `data.metaJson`：补充元数据
- `consoleLogs`：回测期间日志

## 接口

### `Strategies`

- 输入：

```json
{
  "input": {
    "id": "s1",
    "limit": 20,
    "offset": 0,
    "version": "v1",
    "status": "active",
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
    "Strategies": {
      "totalCount": 0,
      "list": []
    }
  }
}
```

- `list` 元素结构：`Strategy`

### `Strategy`

- 输入：

```json
{
  "id": "s1"
}
```

- 返回：单个 `Strategy`，可能为 `null`

### `CreateStrategy`

- 输入：

```json
{
  "input": {
    "name": "demo-strategy",
    "description": "example",
    "code": "export default {}",
    "params": [
      {
        "name": "window",
        "description": "lookback window",
        "type": "number",
        "required": true,
        "default": "20"
      }
    ],
    "signals": [
      {
        "id": "ticker-btc",
        "type": "ticker",
        "exchange": "binance",
        "symbol": "BTCUSDT",
        "props": "{}",
        "scope": "symbol"
      }
    ]
  }
}
```

- 返回结构：新建后的 `Strategy`

### `UpdateStrategy`

- 输入：

```json
{
  "input": {
    "id": "s1",
    "version": "v1",
    "name": "demo-strategy-v2",
    "description": "updated",
    "code": "export default {}",
    "params": [],
    "signals": []
  }
}
```

- 字段说明：
  - `id`：策略 ID
  - `version`：基于哪个版本更新
  - 其余字段均为可选覆盖
- 返回结构：更新后的 `Strategy`

### `GenerateStrategy`

- 输入：

```json
{
  "input": {
    "query": "生成一个趋势跟随 BTC 策略"
  }
}
```

- 返回结构：`GenerateStrategyResponse`

### `ActiveStrategy`

- 输入：

```json
{
  "id": "s1"
}
```

- 返回：布尔值

### `InactiveStrategy`

- 输入与返回同 `ActiveStrategy`

### `RunBacktest`

- 输入：

```json
{
  "input": {
    "strategyId": "s1",
    "version": "v1",
    "runType": 1,
    "startTime": 0,
    "endTime": 0,
    "symbols": [
      {
        "exchange": "binance",
        "symbol": "BTCUSDT",
        "baseAssetQty": "0",
        "quoteAssetQty": "10000"
      }
    ],
    "params": "{\"window\":20}",
    "signals": [
      {
        "signalId": "ticker-btc",
        "datasourceId": 1,
        "exchange": "binance",
        "symbol": "BTCUSDT"
      }
    ]
  }
}
```

- 字段说明：
  - `strategy`：可直接内联策略定义，和 `strategyId` 二选一使用更稳妥
  - `strategyId` / `version`：引用已有策略时使用
  - `runType`：回测运行模式，按调用方约定传值
  - `startTime` / `endTime`：回测区间时间戳
  - `symbols`：初始资金和标的配置
  - `params`：策略参数 JSON 字符串
  - `signals`：信号与数据源映射
- 返回结构：`RunBacktestResponse`
