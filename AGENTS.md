# NovaForge - 加密货币智能交易平台

面向个人 PC 部署的 **单体应用**：Go 服务端与 React 前端同仓，通过 GraphQL 协作，聚焦加密货币交易、策略自动化与数据分析。

## 项目架构

### 组成部分

- **server**：Go 单体服务（GraphQL、gqlgen、策略运行时、行情与交易对接、sqlc 数据访问、定时任务等）。进程入口见 `server/cmd/app`。
- **frontend**：桌面端管理界面（React、TypeScript、UmiJS、Ant Design Pro），经 Apollo Client 调用后端 GraphQL。

### 技术栈概览

- **后端**: Go 1.25+（GraphQL、Gin 等，以仓库内实际依赖为准）、Zerolog、OpenTelemetry（如启用）
- **前端**: React 18、TypeScript 5.8+、Ant Design Pro、Apollo Client、UmiJS 4.4
- **数据存储**: PostgreSQL、Redis 等（以 `server` 配置与 `pkg/repos` 为准）
- **区块链**: Solana（如启用相关功能）
- **API 规范**: GraphQL（对外）；内部另有事件总线、连接器抽象等（见 `server/pkg/strategy`）

---

## 构建、测试、Lint 命令

### Go 后端（在 `server/` 目录）

| 命令 | 说明 |
|------|------|
| `make lint` | 运行 golangci-lint |
| `make lint-fix` | 自动修复 lint 问题 |
| `make build` | 编译单体应用 |
| `make gqlgen` | 生成 GraphQL 代码 |
| `make repo` | 生成 sqlc 数据库代码 |
| `make convert` | 生成 goverter 转换代码 |

**运行单个测试**:

```bash
cd server && go test -v ./pkg/entity/... -run TestFunctionName
```

### 前端（`frontend/`）

使用 **pnpm** 作为包管理器。

| 命令 | 说明 |
|------|------|
| `pnpm run lint` | ESLint + Prettier + TypeScript |
| `pnpm run lint:fix` | 自动修复 |
| `pnpm run tsc` | 仅 TypeScript 检查 |
| `pnpm run test` | Jest |
| `pnpm run dev` | 开发模式 |
| `pnpm run build` | 生产构建 |

---

## 代码风格规范

### Go

- **包名**: 小写、简短（`entity`, `repos`, `converter`）
- **变量/函数**: camelCase；**类型/接口**: PascalCase；**常量**: UPPER_SNAKE（如适用）
- **导入**: 标准库 → 第三方 → 本模块（以 `server/go.mod` 的 module 为根）
- **布局**（`server/pkg/` 为主）: `entity`、`repos`（sqlc）、`converter`、`gateway`、`strategy`、`service` 等

### TypeScript / React

- 页面放在 `pages`，通用组件在 `components`
- `@/` → `src/`，`@@/` → `.umi/`
- 严格模式类型；GraphQL 经 Apollo Client

---

## 错误处理规范

1. 优先处理错误与边界条件  
2. 早返回，少嵌套  
3. 记录日志（后端 Zerolog 等）  
4. 包装错误并返回可理解的失败原因  

---

## 模块关系（单体）

- **frontend** 仅依赖 **server** 暴露的 GraphQL（及可选 WebSocket），不假设独立网关进程。  
- 账户、订单、策略、Bot、行情等能力均在 **server** 进程内通过包边界与依赖注入划分，而非独立微服务仓库。

---

## 调试和开发

- 后端可暴露 pprof、metrics 等（见 `server` 配置与 `Makefile` 中的端口说明）。  
- 数据库初始化可参考 `deploy/README.md` 与 `server/pkg/repos` 下 schema。  
- 修改涉及 GraphQL、SQL 或转换器时，按上文执行对应的 `make` 代码生成命令。

---

## Dashboard 信息整理

（以下为产品能力备忘，数据源对应 GraphQL `Query` / `Mutation` 字段，由单体后端实现。）

### 一、核心模块划分

| 模块 | 说明 |
|------|------|
| **总览** | 资产、收益概览 |
| **账户** | 余额、持仓、流水 |
| **交易** | 订单、成交 |
| **市场** | 行情、K 线、深度 |
| **Meme** | 热度、飙升检测等（如启用） |
| **策略/Bot** | 策略与机器人状态 |
| **监控** | 告警 |

### 二、建议 Widget 与 GraphQL 示例

| 优先级 | Widget | 数据来源示例 |
|--------|--------|----------------|
| 高 | 资产概览 | `Query.Balance`、`Query.Accounts` |
| 高 | 持仓汇总 | `Query.Positions` |
| 高 | 24h 订单统计 | `Query.Orders` |
| 中 | 热门 Token | Meme 相关 Query（如有） |
| 中 | Bot 状态 | `Query.Bots` |
| 中 | 实时行情 | `Query.Ticker` 等 |
| 低 | 财经日历 | `Query.Calendar` |

### 三、统计指标

| 类别 | 指标 |
|------|------|
| **账户** | 激活数 / 总数 |
| **Bot** | 运行中 / 总数 |
