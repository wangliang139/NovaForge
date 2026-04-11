---
name: novaforge
description: NovaForge 桌面端访问说明：通过 GraphQL 查询或操作账户、策略、Bot。用户提到账户、下单、持仓、策略、回测、Bot、GraphQL 或需要用本目录下 `scripts/cli.js` 调用后端时使用。
license: Proprietary
compatibility: 需要 Node.js 18+；环境变量见 `scripts/cli.js` 内 `graphql` 子命令帮助（`NOVAFORGE_API_KEY` 等）。
metadata:
  domain: novaforge
  transport: graphql
---

# NovaForge 连接器技能

## 何时使用

- 用户要查询或修改 NovaForge 的账户、订单、持仓、策略、回测或 Bot 数据。
- 用户明确要走 GraphQL，或需要复用本目录下的 `scripts/cli.js`。
- 用户只描述业务对象时，先按对象类型选择对应参考文档，不要把三份参考文档全部读入上下文。

## 默认流程

1. 在仓库中进入 `skills/novaforge-connector/`，优先使用 `node scripts/cli.js graphql ...`，不要手写原始 HTTP。
2. 先判断任务属于账户、策略还是 Bot，再只读取对应的参考文档。
3. 先执行只读查询，拿到 `accountId`、`strategyId`、`botId` 等标识，再执行 mutation。
4. 执行 mutation 前，先核对必填字段、枚举值和目标资源。
5. 返回结果时，优先摘录关键字段，不要把大段原始 JSON 全量贴给用户。

## 参考文档选择

- 账户、余额、持仓、订单、风控、杠杆：读 `references/account-apis.md`
- 策略、策略生成、激活停用、回测：读 `references/strategy-apis.md`
- Bot、Bot 状态、Bot 资产、启动停止升级删除：读 `references/bot-apis.md`

只有在字段或枚举不清楚时，再展开读取对应文档的详细片段。

## 常用命令

在 `skills/novaforge-connector/` 目录下：

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

## MCP 工具与 GraphQL 字段

MCP 工具注册名与 GraphQL 根字段的对应关系以运行时代码为准，见：

- `server/pkg/gateway/mcp/tools/account.go`
- `server/pkg/gateway/mcp/tools/strategy.go`
- `server/pkg/gateway/mcp/tools/bot.go`

各文件中的 `mcp.AddTool` 第一个参数为工具名，Description 中标明对应的 GraphQL 操作。

## 变更类操作检查单

- [ ] 已定位目标资源 ID
- [ ] 已确认使用的字段名和枚举值
- [ ] 已核对是否需要先读当前状态
- [ ] 已避免泄露敏感凭证字段
- [ ] 已在结果中返回关键状态或错误信息
