# simulate

`simulate` 包提供一个内存内、可回放、可多账户隔离的撮合与账户模拟内核，主要用于：

- 纸面交易（paper）能力承载
- 回放/确定性测试
- 订单簿驱动的近实时模拟撮合

本文档只描述当前代码已支持的能力边界。

## 当前支持的 Feature

### 1) 市场深度维护（L2）

- 支持 `snapshot + delta` 两种更新
- 支持序列号连续性校验（`PrevSeqId`）
- 支持序列断档检测（`ErrSeqGap`）
- 提供最优买卖价读取（`BestBid/BestAsk`）
- 支持拷贝/遍历（用于 shadow execution）

核心类型：

- `MarketDepth`
- `OrderBook`
- `OrderBookLevel`

### 2) 数据协调与重同步

- `Coordinator` 可对接外部 `DataLoader`
- `Bootstrap()` 拉首个快照
- `HandleDelta()` 遇到 `ErrSeqGap` 自动 `PullSnapshot()` 重同步后重试一次
- 重试后仍不连续的增量会被丢弃（防止脏序列污染）

核心类型：

- `Coordinator`
- `DataLoader`

### 3) Shadow Execution（只读撮合模拟）

- 基于当前深度，模拟：
  - 市价买/卖
  - 限价买/卖
- 不修改原始深度（使用 `Clone`）
- 返回 fills、leftover、notional，可用于预估成交

核心函数：

- `SimulateMarketBuy`
- `SimulateMarketSell`
- `SimulateLimitBuy`
- `SimulateLimitSell`
- `AveragePrice`

### 4) SimExchange（撮合 + 账户）

- 统一管理：
  - 交易规则（`Instrument`）
  - 公共深度（`BindDepth`）
  - 用户订单簿（`SimBook`）
  - 账户资产/仓位（`Portfolio`）
- 支持下单、撤单、查单、开放订单列表
- 支持 `OnDepthUpdated` 触发挂单再撮合
- 支持确定性时间注入（`WithNowFn`）
- 订单 ID 自动生成（`o1/o2/...`）

核心类型：

- `SimExchange`
- `SimBook`
- `Portfolio`
- `SimOrder`
- `PlaceOrderRequest/Result`

### 5) 多账户模型（强隔离）

- 原生多账户：
  - 余额按 `accountID` 隔离
  - 仓位按 `accountID + symbol` 隔离
  - 订单簿按 `accountID + symbol` 隔离
- 账户 ID 非法会返回 `ErrInvalidAccount`

### 6) 现货（Spot）能力

- 市价/限价撮合
- 最小数量、最小名义价值校验
- 价格/数量按 tick/step 向下对齐
- taker/maker 手续费（bps）计入资产变化
- 资产扣减/增加在 `Portfolio` 内完成

### 7) 合约（Perp）基础能力

- 一向持仓模型（净仓，`Qty > 0` 多，`Qty < 0` 空）
- `IntentOpen/IntentClose` + `ReduceOnly` 约束
- 杠杆上下限校验（`LeverageMax`）
- 开平仓保证金与已用保证金更新
- 仓位均价、已用保证金、杠杆更新

说明：当前是最小闭环实现，非完整交易所级风控系统。

### 8) 事件泵与回放时间

- `ReplayClock` 提供确定性时间推进
- `EventPump` 支持事件序列化执行：
  - 深度快照
  - 深度增量
  - ticker
  - 下单（可配置下单延迟）
  - 撤单（可配置撤单延迟）
- 同时间事件按提交序号稳定排序，保证可重放

核心类型：

- `ReplayClock`
- `EventPump`
- `SimEvent`

### 9) 行情缓存

- `TickerStore` 维护 symbol -> latest ticker
- 供模拟校验/估算或上层 connector 读取

## 错误语义（部分）

- 深度类：`ErrNotInitialized`、`ErrSeqGap`
- 交易参数类：`ErrInvalidQty`、`ErrInvalidPrice`、`ErrBelowMinQty`、`ErrBelowMinNotional`
- 账户类：`ErrInsufficientBalance`、`ErrInvalidAccount`
- 持仓意图类：`ErrReduceOnly`、`ErrInvalidIntent`、`ErrLeverage`
- 资源类：`ErrUnknownSymbol`、`ErrOrderNotFound`

详见：`errors.go`。

## 与上层 connector 的关系

`simulate` 是模拟撮合内核；`server/pkg/market/connector/simulate` 在其上实现系统标准 `Connector` 接口。

当前落地中：

- Spot 主路径已接入
- 若某些 API 尚未覆盖（如部分 mark/index/funding），connector 层会明确返回 `not implemented` 错误

## 非目标 / 暂未覆盖

- 全量交易所规则（全部 TIF/高级订单语义）
- 完整资金费率结算与强平引擎
- 跨币种复杂保证金组合模型
- 与真实交易所 1:1 的撮合细节完全一致性

这些能力可在当前内核基础上继续扩展。
