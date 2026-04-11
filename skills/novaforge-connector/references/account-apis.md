# 账户与交易接口

## 通用约定

- GraphQL 响应包裹结构固定为：

```json
{
  "data": {
    "<FieldName>": {}
  }
}
```

- 所有金额、数量、价格大多使用字符串，按高精度数值处理，不要直接当作浮点数。

## 常用枚举

- `AccountStatus`：`online` | `offline` | `unspecified`
- `WalletType`：`fund` | `trade` | `spot` | `future` | `margin` | `unspecified`
- `MarketType`：`spot` | `future` | `unspecified`
- `MarketSource`：`db` | `remote` | `unspecified`
- `OrderType`：`market` | `limit` | `none`
- `OrderSource`：`user` | `strategy` | `liquidation` | `adl`
- `PositionSide`：`long` | `short`
- `MetricsDimension`：`account` | `symbol` | `unspecified`

## 共享结构

### `AmountLimit`

```json
{
  "amount": "1000",
  "ratio": "0.2"
}
```

- `amount`：绝对额度限制
- `ratio`：相对比例限制

### `AccountConfig`

```json
{
  "maxOrderSize": "1000",
  "maxPositionPerSymbol": { "amount": "5000", "ratio": "0.3" },
  "maxDailyLoss": { "amount": "300", "ratio": "0.05" },
  "maxLeverage": "10",
  "maxOrdersPerMinute": 30,
  "minMaintenanceMarginRatio": "0.05",
  "maxTotalNetExposure": { "amount": "10000", "ratio": "0.8" },
  "maxTotalGrossExposure": { "amount": "15000", "ratio": "1.2" },
  "riskIndexThreshold": "80",
  "riskIndexAction": "reject",
  "cooldownSeconds": 60
}
```

- `maxOrderSize`：单笔最大下单量或名义规模
- `maxPositionPerSymbol`：单标的最大持仓
- `maxDailyLoss`：单日最大亏损
- `maxLeverage`：允许的最大杠杆
- `maxOrdersPerMinute`：每分钟最大下单数
- `minMaintenanceMarginRatio`：最小维持保证金率
- `maxTotalNetExposure`：总净敞口上限
- `maxTotalGrossExposure`：总毛敞口上限
- `riskIndexThreshold`：风险指数阈值
- `riskIndexAction`：超阈值后的动作
- `cooldownSeconds`：风控冷却时间

### `Account`

```json
{
  "id": "1",
  "name": "main",
  "exchange": "binance",
  "apiKey": "masked",
  "apiSecret": "masked",
  "passphrase": "",
  "tags": ["prod"],
  "status": "online",
  "algorithm": "hmac",
  "accountType": "real",
  "config": {},
  "createdAt": 0,
  "updatedAt": 0,
  "stats": {
    "notional": "0",
    "unRealizedProfit": "0",
    "notional24HChange": "0"
  },
  "riskIndex": "35"
}
```

- `id`：账户 ID
- `name`：账户名
- `exchange`：交易所
- `apiKey` / `apiSecret` / `passphrase`：凭证字段，读取时谨慎处理
- `tags`：标签
- `status`：在线状态
- `algorithm`：签名算法
- `accountType`：真实盘或模拟盘
- `config`：账户风控配置
- `stats`：账户统计快照
- `riskIndex`：当前风险指数，通常按字符串返回

### `Equity`

```json
{
  "id": 1,
  "accountId": "1",
  "ts": 0,
  "notional": "1000",
  "unRealizedProfit": "12.5",
  "createdAt": 0
}
```

- `ts`：权益时间点
- `notional`：净值或名义权益
- `unRealizedProfit`：未实现盈亏

### `Asset`

```json
{
  "code": "USDT",
  "balance": "1000",
  "locked": "50",
  "notional": "1000",
  "unRealizedProfit": "0",
  "walletType": "future",
  "updatedTs": 0
}
```

- `code`：资产代码
- `balance`：总余额
- `locked`：冻结金额
- `notional`：折算后的名义价值
- `unRealizedProfit`：资产维度未实现盈亏
- `walletType`：钱包类型

### `Balance`

```json
{
  "notional": "1000",
  "unRealizedProfit": "12.5",
  "notional24HChange": "30",
  "assets": []
}
```

- `notional`：总权益或总名义价值
- `unRealizedProfit`：总未实现盈亏
- `notional24HChange`：24 小时权益变化
- `assets`：资产明细数组，元素结构见 `Asset`

### `Position`

```json
{
  "symbol": "BTCUSDT",
  "side": "long",
  "isolated": true,
  "amount": "0.01",
  "entryPrice": "60000",
  "markPrice": "60500",
  "liquidationPrice": "50000",
  "notional": "605",
  "leverage": 10,
  "initialMargin": "60.5",
  "maintMargin": "6.05",
  "unRealizedProfit": "5",
  "updatedTs": 0
}
```

- `amount`：持仓数量
- `entryPrice`：开仓均价
- `markPrice`：标记价格
- `liquidationPrice`：预估强平价
- `notional`：名义价值
- `leverage`：杠杆
- `initialMargin` / `maintMargin`：初始保证金 / 维持保证金

### `Order`

```json
{
  "accountId": "1",
  "botId": 0,
  "exchange": "binance",
  "symbol": "BTCUSDT",
  "clientOrderId": "abc",
  "orderId": "123",
  "drivedOrderId": "",
  "side": "long",
  "isBuy": true,
  "orderType": "limit",
  "algoType": "none",
  "source": "user",
  "price": "60000",
  "originalQty": "0.01",
  "executedQty": "0",
  "originalQuoteQty": "600",
  "executedQuoteQty": "0",
  "avgPrice": "0",
  "priceWorkingType": "",
  "priceMode": "",
  "status": "new",
  "timeInForce": "GTC",
  "reduceOnly": false,
  "closePosition": false,
  "postOnly": false,
  "priceProtect": false,
  "conditions": [],
  "isWorking": true,
  "workingTs": 0,
  "rejectReason": "",
  "createdTs": 0,
  "updatedTs": 0,
  "finishedTs": 0,
  "locked": "0",
  "lockedAsset": "USDT",
  "fee": "0",
  "feeAsset": "USDT",
  "realizedPnl": "0",
  "pnlAsset": "USDT"
}
```

- `clientOrderId`：客户端订单号
- `orderId`：交易所订单号
- `drivedOrderId`：派生订单 ID
- `side`：方向标签
- `isBuy`：是否买入
- `orderType`：订单类型
- `algoType`：算法单类型
- `source`：订单来源
- `originalQty` / `executedQty`：原始数量 / 已成交数量
- `originalQuoteQty` / `executedQuoteQty`：按计价资产表示的数量
- `avgPrice`：成交均价
- `status`：订单状态
- `reduceOnly` / `closePosition`：仅减仓 / 平仓标记
- `conditions`：条件单信息
- `locked` / `lockedAsset`：锁定资金及币种
- `fee` / `feeAsset`：手续费及币种
- `realizedPnl` / `pnlAsset`：已实现盈亏及币种

### `RiskEvent`

```json
{
  "id": 1,
  "accountId": "1",
  "exchange": "binance",
  "rule": "max_daily_loss",
  "riskIndex": "92",
  "payloadJson": "{}",
  "createdAt": 0
}
```

- `rule`：触发的风控规则名
- `riskIndex`：触发时风险指数
- `payloadJson`：补充细节，按 JSON 字符串解析

### `AccountMetrics`

```json
{
  "accountId": "1",
  "dimension": "account",
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

- `dimension`：账户级或标的级
- `cagr`：复合年化收益率
- `sharpe` / `sortino` / `calmar`：风险收益指标
- `maxDrawdown`：最大回撤
- `timeUnderWaterSeconds`：净值回撤区间累计时长
- `winRate`：胜率
- `profitFactor`：盈亏比
- `rollingSharpe`：滚动夏普
- `avgSlippageBps`：平均滑点，单位 bps
- `feeRatio`：手续费占比
- `maxConsecutiveLoss`：最大连续亏损次数
- `symbols`：标的级指标数组

## 接口

### `Accounts`

- 输入：

```json
{
  "input": {
    "limit": 20,
    "offset": 0,
    "id": "1",
    "name": "main",
    "exchange": "binance",
    "tags": ["prod"],
    "status": "online",
    "createdAtStart": 0,
    "createdAtEnd": 0
  }
}
```

- 字段说明：
  - `limit` / `offset`：分页
  - `id` / `name`：精确或名称筛选
  - `exchange`：交易所筛选
  - `tags`：标签筛选
  - `status`：状态筛选
  - `createdAtStart` / `createdAtEnd`：创建时间区间
- 返回：

```json
{
  "data": {
    "Accounts": {
      "totalCount": 0,
      "list": []
    }
  }
}
```

- `list` 元素结构：`Account`

### `Equitys`

- 输入：

```json
{
  "input": {
    "accountId": "1",
    "range": "7d"
  }
}
```

- 字段说明：
  - `accountId`：账户 ID
  - `range`：时间范围字符串，常见如 `1d`、`7d`、`30d`
- 返回：`Equity[]`

### `Leverage`

- 输入：

```json
{
  "accountId": "1",
  "symbol": "BTCUSDT"
}
```

- 返回：整数杠杆值

### `AccountMetrics`

- 输入：

```json
{
  "input": {
    "accountId": "1",
    "dimension": "account",
    "symbol": "BTCUSDT",
    "startTs": 0,
    "endTs": 0
  }
}
```

- `symbol`：仅在标的级分析时使用
- 返回结构：`AccountMetrics`

### `RiskEvents`

- 输入：

```json
{
  "input": {
    "accountId": "1",
    "limit": 20,
    "offset": 0
  }
}
```

- 返回：`RiskEvent[]`

### `Balance`

- 输入：

```json
{
  "input": {
    "accountId": "1",
    "walletType": "future",
    "asset": "USDT",
    "withNotional": true,
    "source": "remote"
  }
}
```

- 字段说明：
  - `walletType`：钱包过滤
  - `asset`：只看某个币种
  - `withNotional`：是否要求返回折算名义价值
  - `source`：从缓存还是远端获取
- 返回结构：`Balance`

### `Positions`

- 输入：

```json
{
  "input": {
    "accountId": "1",
    "marketType": "future",
    "symbol": "BTCUSDT",
    "source": "remote"
  }
}
```

- 返回：`Position[]`

### `Orders`

- 输入：

```json
{
  "input": {
    "accountId": "1",
    "symbol": "BTCUSDT",
    "orderType": "limit",
    "orderSource": "user",
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
    "Orders": {
      "totalCount": 0,
      "list": []
    }
  }
}
```

- `list` 元素结构：`Order`

### `OnlineAccount`

- 输入：

```json
{
  "id": "1"
}
```

- 返回结构：`Account`

### `OfflineAccount`

- 权限：`M`
- 输入与返回同 `OnlineAccount`

### `RefreshAccountSnapshots`

- 输入：

```json
{
  "accountId": "1"
}
```

- 返回：布尔值，表示是否成功触发刷新

### `PlaceOrder`

- 输入：

```json
{
  "input": {
    "accountId": "1",
    "symbol": "BTCUSDT",
    "side": "long",
    "isBuy": true,
    "orderType": "limit",
    "price": "60000",
    "quantity": "0.01",
    "timeInForce": "GTC",
    "reduceOnly": false,
    "closePosition": false
  }
}
```

- 字段说明：
  - `side`：仓位方向标签
  - `isBuy`：买卖方向
  - `orderType`：市价或限价
  - `price`：限价单通常必填
  - `quantity`：基础资产数量
  - `timeInForce`：例如 `GTC`
  - `reduceOnly`：仅减仓
  - `closePosition`：按平仓意图下单
- 返回：

```json
{
  "data": {
    "PlaceOrder": {
      "orderId": "123",
      "clientOrderId": "abc"
    }
  }
}
```

### `CancelOrder`

- 输入：

```json
{
  "input": {
    "accountId": "1",
    "symbol": "BTCUSDT",
    "clientOrderId": "abc",
    "orderId": "123"
  }
}
```

- 至少应提供 `clientOrderId` 或 `orderId` 之一
- 返回：布尔值

### `SetLeverage`

- 输入：

```json
{
  "accountId": "1",
  "symbol": "BTCUSDT",
  "leverage": 10
}
```

- 返回：设置后的杠杆整数值

### `UpdateAccountRiskConfig`

- 输入：

```json
{
  "input": {
    "accountId": "1",
    "maxOrderSize": "1000",
    "maxPositionPerSymbol": { "amount": "5000", "ratio": "0.3" },
    "maxDailyLoss": { "amount": "300", "ratio": "0.05" },
    "maxLeverage": "10",
    "maxOrdersPerMinute": 30,
    "minMaintenanceMarginRatio": "0.05",
    "maxTotalNetExposure": { "amount": "10000", "ratio": "0.8" },
    "maxTotalGrossExposure": { "amount": "15000", "ratio": "1.2" },
    "riskIndexThreshold": "80",
    "riskIndexAction": "reject",
    "cooldownSeconds": 60
  }
}
```

- 返回结构：更新后的 `Account`
