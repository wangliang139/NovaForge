---
name: llt-trade
description: LLT Trade 服务访问工具，提供通过 GraphQL 网关查询或操作账户、策略、Bot 数据的能力。用户提到账户、下单、持仓、策略、回测、Bot、GraphQL 接口或需要用 `scripts/cli.js` 调用 LLT 服务时使用。
license: Proprietary
compatibility: 需要 Node.js 18+、`LLT_API_KEY`，可选 `LLT_BASE_URL`；通过 `node scripts/cli.js graphql` 访问 LLT GraphQL 网关。
metadata:
  domain: llt-trade
  transport: graphql
---

# LLT Trade

## 何时使用

- 用户要查询或修改 LLT Trade 的账户、订单、持仓、策略、回测或 Bot 数据。
- 用户已经明确要走 GraphQL 网关，或需要复用 `skills/scripts/cli.js`。
- 用户只描述业务对象时，先根据对象类型选择对应参考文档，不要把三份参考文档全部读入上下文。

## 默认流程

1. 优先使用 `node scripts/cli.js graphql ...`，不要手写 `curl`、Header 或原始 HTTP 请求。
2. 先判断任务属于账户、策略还是 Bot，再只读取对应的参考文档。
3. 先执行只读查询，拿到 `accountId`、`strategyId`、`botId` 等标识，再执行 mutation。
4. 执行 mutation 前，先核对必填字段、枚举值和目标资源，再发请求。
5. 返回结果时，优先摘录关键字段，不要把大段原始 JSON 全量贴给用户。

## 参考文档选择

- 账户、余额、持仓、订单、风控、杠杆：读 `references/account-apis.md`
- 策略、策略生成、激活停用、回测：读 `references/strategy-apis.md`
- Bot、Bot 状态、Bot 资产、启动停止升级删除：读 `references/bot-apis.md`

只有在字段或枚举不清楚时，再展开读取对应文档的详细片段。

## 常用命令

查询时默认使用：

```bash
node scripts/cli.js graphql \
  --query 'query Accounts { Accounts { id name status } }' \
  --operation-name Accounts
```

查询较长时，优先使用文件输入：

```bash
node scripts/cli.js graphql \
  --query-file tmp/query.graphql \
  --variables-file tmp/variables.json \
  --operation-name Accounts
```

## Gotchas

- GraphQL 响应通常包在 `data.<FieldName>` 下，先取目标字段，再解释业务含义。
- 金额、价格、数量大量以字符串返回，按高精度数值处理，不要默认转浮点。
- 凭证字段如 `apiKey`、`apiSecret`、`passphrase` 可能被返回或部分脱敏，向用户展示时保持克制。
- 变更类操作默认更敏感。若请求含删除、停用、下单、撤单、升级等动作，先确认目标 ID 和关键参数。
- 当任务只需要某一类对象时，不要同时读取三份参考文档，避免无关上下文干扰。

## 接口索引

### 账户与交易

参考：`references/account-apis.md`

| MCP 工具 | GraphQL 字段 |
|---|---|
| `llt_accounts` | `Accounts` |
| `llt_equitys` | `Equitys` |
| `llt_account_leverage` | `Leverage` |
| `llt_account_metrics` | `AccountMetrics` |
| `llt_risk_events` | `RiskEvents` |
| `llt_balance` | `Balance` |
| `llt_positions` | `Positions` |
| `llt_orders` | `Orders` |
| `llt_online_account` | `OnlineAccount` |
| `llt_offline_account` | `OfflineAccount` |
| `llt_refresh_account_snapshots` | `RefreshAccountSnapshots` |
| `llt_place_order` | `PlaceOrder` |
| `llt_cancel_order` | `CancelOrder` |
| `llt_set_leverage` | `SetLeverage` |
| `llt_update_account_risk_config` | `UpdateAccountRiskConfig` |

### 策略与回测

参考：`references/strategy-apis.md`

| MCP 工具 | GraphQL 字段 |
|---|---|
| `llt_strategies` | `Strategies` |
| `llt_strategy` | `Strategy` |
| `llt_create_strategy` | `CreateStrategy` |
| `llt_update_strategy` | `UpdateStrategy` |
| `llt_generate_strategy` | `GenerateStrategy` |
| `llt_active_strategy` | `ActiveStrategy` |
| `llt_inactive_strategy` | `InactiveStrategy` |
| `llt_run_backtest` | `RunBacktest` |

### Bot

参考：`references/bot-apis.md`

| MCP 工具 | GraphQL 字段 |
|---|---|
| `llt_bots` | `Bots` |
| `llt_bot` | `Bot` |
| `llt_bot_balance` | `BotBalance` |
| `llt_bot_positions` | `BotPositions` |
| `llt_bot_state` | `BotState` |
| `llt_bot_orders` | `BotOrders` |
| `llt_bot_ledger` | `BotLedger` |
| `llt_bot_equity` | `BotEquity` |
| `llt_bot_metrics` | `BotMetrics` |
| `llt_create_bot` | `CreateBot` |
| `llt_update_bot` | `UpdateBot` |
| `llt_start_bot` | `StartBot` |
| `llt_stop_bot` | `StopBot` |
| `llt_upgrade_bot` | `UpgradeBot` |
| `llt_delete_bot` | `DeleteBot` |

## 变更类操作检查单

- [ ] 已定位目标资源 ID
- [ ] 已确认使用的字段名和枚举值
- [ ] 已核对是否需要先读当前状态
- [ ] 已避免泄露敏感凭证字段
- [ ] 已在结果中返回关键状态或错误信息
