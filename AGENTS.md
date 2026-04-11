# NovaForge - 加密货币智能交易平台

这是一个多服务微服务平台，专注于加密货币交易、数据分析和智能决策支持。

## 项目架构

### 核心服务
- **llt-data-api**: 数据服务层（Go + gRPC），处理账户、文档、交易所、项目等核心数据
- **llt-trade-api**: 交易服务（Go + gRPC），专注 Meme Token 分析和 Solana 链上交易
- **llt-trade-py**: AI 交易分析服务（Python + FastAPI），提供机器学习驱动的智能分析
- **llt-backoffice-gateway**: API 网关（Go + GraphQL + Gin），统一对外接口
- **llt-strategy-api**: 策略服务（Go + gRPC）
- **frontend**: 前端管理界面（React + TypeScript + UmiJS + Ant Design Pro）
- **common**: 通用接口（Proto）/代码等

### 技术栈概览
- **后端**: Go 1.24+ (gRPC, GraphQL, Gin), Python 3.10.18 (FastAPI, TensorFlow, PyTorch)
- **前端**: React 18, TypeScript 5.8+, Ant Design Pro, Apollo Client, UmiJS 4.4
- **数据存储**: PostgreSQL, Redis, Qdrant (向量数据库), MinIO (对象存储)
- **消息队列**: Apache Kafka
- **可观测性**: OpenTelemetry, Zerolog
- **区块链**: Solana (gagliardetto/solana-go)
- **AI/ML**: OpenAI, Google Gemini, LangChain, TensorFlow, PyTorch
- **API 规范**: Protocol Buffers (gRPC), GraphQL

---

## 构建、测试、Lint 命令

### Go 服务 (llt-data-api, llt-trade-api, ll-backoffice-gateway, llt-strategy-api)

| 命令 | 说明 |
|------|------|
| `make lint` | 运行 golangci-lint 检查 |
| `make lint-fix` | 自动修复 lint 问题 |
| `make build` | 编译应用 (llt-backoffice-gateway) |
| `make gqlgen` | 生成 GraphQL 代码 (llt-backoffice-gateway) |
| `make repo` | 生成 sqlc 数据库代码 |
| `make convert` | 生成 goverter 转换代码 |
| `make proto` | 生成 protobuf 代码 |

**运行单个测试**:
```bash
go test -v ./pkg/entity/... -run TestFunctionName
```

### Python 服务 (llt-trade-py)

使用 **uv** 作为包管理器。

| 命令 | 说明 |
|------|------|
| `uv sync` | 安装依赖 |
| `uv run pytest` | 运行所有测试 |
| `uv run pytest tests/test_file.py::test_func` | 运行单个测试 |
| `uv run pytest tests/ -k "test_name"` | 按名称过滤测试 |
| `uv run pytest tests/ --cov` | 运行测试并生成覆盖率报告 |
| `make proto` | 生成 protobuf 代码 |

### 前端 (frontend)

使用 **pnpm** 作为包管理器。

| 命令 | 说明 |
|------|------|
| `pnpm run lint` | 运行 ESLint + Prettier + TypeScript 检查 |
| `pnpm run lint:fix` | 自动修复 lint 问题 |
| `pnpm run tsc` | TypeScript 类型检查 |
| `pnpm run test` | 运行 Jest 测试 |
| `pnpm run test -- TestName` | 运行单个测试文件 |
| `pnpm run test -t "test name"` | 按测试名称过滤 |
| `pnpm run test:update` | 更新快照 |
| `pnpm run test:coverage` | 运行测试并生成覆盖率 |
| `pnpm run dev` | 开发模式启动 |
| `pnpm run build` | 构建生产版本 |

---

## 代码风格规范

### Go 开发规范

#### 命名约定
- **包名**: 使用小写字母，简短（如 `entity`, `repos`, `converter`）
- **变量/函数**: 使用驼峰命名（camelCase）
- **结构体/接口**: 使用帕斯卡命名（PascalCase）
- **常量**: 使用全大写字母加下划线（如 `MaxRetryCount`）
- **文件**: 使用小写字母加下划线（如 `user_service.go`）

#### 导入规范
- 标准库 → 第三方库 → 项目内部包
- 使用 Go Modules 管理依赖
- 修改依赖后使用 `go get {module}`，依赖服务 API 变更时使用 `go get {module}/api`

#### 代码组织
```
pkg/
├── entity/       # 业务实体层
├── service/      # gRPC 服务实现
├── repos/        # 数据库仓库层 (sqlc)
├── converter/    # 数据转换器
├── sdk/          # 第三方 SDK
└── utils/        # 工具函数
```

#### API 定义
- Proto 文件在 `api/schema` 或 `api/proto` 目录
- 使用 `make proto` 生成 Go 代码
- 数据库 schema 在 `pkg/repos/*/schema.sql`，查询在 `pkg/repos/*/query.sql`
- 使用 `make repo` 生成 sqlc 代码

### TypeScript/React 开发规范

#### 命名约定
- **组件文件**: 使用帕斯卡命名（如 `UserProfile.tsx`）
- **工具函数**: 使用驼峰命名（如 `formatDate.ts`）
- **样式文件**: 使用 `.style.ts` 后缀
- **类型文件**: 使用 `.types.ts` 后缀

#### 代码组织
- 页面组件放在 `pages` 子目录，按功能模块组织
- 每个模块包含 `index.tsx`, `service.ts`, `types.ts`
- 通用组件放在 `components` 子目录
- 使用 `@/` 指向 `src/`，`@@/` 指向 `.umi/`

#### 规范
- 使用 Ant Design Pro 组件库
- 使用 TypeScript 严格模式
- 使用 Apollo Client 进行 GraphQL 查询
- 遵循 ESLint 和 Prettier 配置

### Python 开发规范

#### 命名约定
- **模块/包**: 使用小写字母加下划线（如 `user_service`）
- **类**: 使用帕斯卡命名（如 `UserService`）
- **函数/方法**: 使用小写字母加下划线（如 `get_user`）
- **常量**: 使用全大写字母加下划线（如 `MAX_RETRY`）

#### 代码组织
```
app/
├── dao/          # 数据访问层
├── services/     # 业务服务层
├── routes/       # HTTP 路由
├── data/         # 数据处理
├── libs/         # 库函数
├── sdk/          # 第三方 SDK
└── utils/        # 工具函数
```

#### 规范
- 使用 Pydantic 进行数据验证
- 使用 Type Hints 增强可读性
- 遵循 PEP 8 代码风格
- 使用异步编程（async/await）

---

## 错误处理规范

所有语言遵循以下通用原则：

1. **优先处理错误**: 在函数开始时处理错误条件
2. **早期返回**: 使用守卫子句提前处理前置条件和无效状态
3. **避免深层嵌套**: 使用 if-return 模式，避免不必要的 else
4. **日志记录**: 使用对应语言的日志库（Go: Zerolog, Python: logging, Frontend: console）
5. **错误包装**: 使用语言特定的错误包装机制
6. **用户友好**: 返回用户友好的错误消息

---

## 项目间依赖

- llt-backoffice-gateway 调用其他 API 模块的 gRPC 服务
- llt-trade-py 作为独立的 AI 分析服务
- frontend 通过 GraphQL 与 llt-backoffice-gateway 通信

---

## 调试和开发

- Go 服务在 6060 端口提供 pprof 端点
- 使用 OpenTelemetry 进行分布式追踪
- 查看各服务的 README.md 获取具体配置和启动方式
- 如果修改了多个服务的代码，请按照服务依赖关系，以服务为单位从底层开始修改代码，并依次让用户确认并提交，不要一次性提交所有修改

---

## Dashboard 信息整理

### 一、核心模块划分

| 模块 | 说明 |
|------|------|
| **总览** | 整体资产、收益概览 |
| **账户** | 账户余额、持仓、流水 |
| **交易** | 订单、成交记录 |
| **市场** | 实时行情、K线、深度 |
| **Meme** | 热度排行、飙升检测 |
| **策略/Bot** | 策略管理、机器人状态 |
| **监控** | 异常告警 |

### 二、各模块具体指标

#### 1. 总览 (Overview)
- **总资产**: 各账户权益总和 (Equity)
- **24h 盈亏**: 总未实现盈亏 + 已实现盈亏
- **持仓数**: 活跃持仓数量
- **账户状态**: 在线/离线账户数

#### 2. 账户 (Account)
- 账户列表 (名称、交易所、状态)
- 余额详情 (可用、冻结)
- 持仓列表 (交易对、方向、数量、均价、盈亏)
- 账户流水 (充值、提现、交易)
- 权益曲线 (历史权益变化)

#### 3. 交易 (Trading)
- **订单列表**: 挂单中/已完成/已撤销
- **实时成交**: 最近成交记录
- **手续费统计**: 24h 手续费支出

#### 4. 市场 (Market)
- **热门交易对**: 涨跌幅排行
- **实时行情**: 价格、24h 成交量、涨跌幅
- **K线图表**: 可选时间周期
- **订单簿**: 买卖深度

#### 5. Meme Token (Solana)
- **热门 Token**: 热度排行 Top 10
- **飙升检测**: 快速暴涨的 Token
- **热门 Creator**: 创建者排行
- **热门 Trader**: 交易者排行
- **Token 详情**: MCap、Bonding Curve、流动性

#### 6. 策略与 Bot
- **策略列表**: 名称、类型、状态
- **Bot 状态**: 运行中/已停止
- **回测结果**: 收益率、胜率、夏普比率
- **信号监控**: 触发信号记录

#### 7. 监控 (Monitor)
- **异常告警**: 账户/持仓异常提醒

#### 8. 资讯 (Document)
- **财经日历**: 重要经济事件
- **文档摘要**: AI 生成的资讯摘要

### 三、建议补充的 Widget

| 优先级 | Widget | 数据来源 |
|--------|--------|----------|
| 高 | 资产概览卡片 | `Query.Balance`, `Query.Accounts` |
| 高 | 持仓汇总 | `Query.Positions` |
| 高 | 24h 订单统计 | `Query.Orders` |
| 中 | 热门 Token 排行 | `llt-trade-api` Meme 服务 |
| 中 | Bot 运行状态 | `Query.Bots` |
| 中 | 实时行情 | `Query.Ticker` |
| 低 | 财经日历 | `Query.Calendar` |

### 四、统计指标

| 类别 | 指标 |
|------|------|
| **账户** | 激活数 / 总数 |
| **Bot** | 上线数 / 总数 |
