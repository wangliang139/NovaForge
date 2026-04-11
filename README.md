# NovaForge

面向个人或单机部署的 **Web3 多交易所交易与管理平台**：同一进程内提供 GraphQL API、策略运行时、行情与交易对接，配套 React 管理界面。

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

## 目录

- [功能概览](#功能概览)
- [技术栈](#技术栈)
- [仓库结构](#仓库结构)
- [环境要求](#环境要求)
- [本地开发](#本地开发)
- [部署](#部署)
  - [1. 准备依赖服务](#1-准备依赖服务)
  - [2. 初始化数据库](#2-初始化数据库)
  - [3. 配置环境变量](#3-配置环境变量)
  - [4. 使用 Docker 部署（推荐）](#4-使用-docker-部署推荐)
  - [5. 手动编译运行](#5-手动编译运行)
- [常用命令](#常用命令)
- [文档与规范](#文档与规范)
- [许可证](#许可证)
- [免责声明](#免责声明)

## 功能概览

- 多交易所账户、订单、持仓与行情聚合（具体能力以 GraphQL 与页面为准）
- 策略 / Bot 运行时、回测与模拟盘相关能力（详见 `server/pkg/strategy` 与 `docs/`）
- 单体架构：前端通过 GraphQL（及可选 WebSocket）与后端通信，无独立 API 网关进程

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.25+、Gin、gqlgen、sqlc、Zerolog 等 |
| 前端 | React 18、TypeScript、UmiJS 4、Ant Design Pro、Apollo Client |
| 数据 | PostgreSQL、Redis；部分能力可选 ClickHouse、NATS（见各模块配置） |

## 仓库结构

| 路径 | 说明 |
|------|------|
| `server/` | Go 单体服务：入口 `cmd/app`，模块路径见 `server/go.mod` |
| `frontend/` | React SPA，包管理使用 **pnpm** |
| `deploy/` | 聚合 DDL（如 `deploy.sql`），用于新环境初始化 |
| `docker/` | 容器内 Nginx 与启动脚本 |
| `docs/` | 策略、指标等技术文档 |
| `skills/novaforge-connector/` | 通过脚本调用 GraphQL 的辅助说明（见目录内 `SKILL.md`） |

## 环境要求

- **本地开发**：Go 1.25+、Node.js 20+、[pnpm](https://pnpm.io/)、PostgreSQL、Redis（及业务所需的其它中间件）
- **Docker 构建**：Docker（建议启用 BuildKit），根目录 `Dockerfile` 默认目标平台为 `linux/amd64`

## 本地开发

1. 克隆仓库并准备数据库、Redis 等（连接串需与后端环境变量一致，通常通过 `postgres`、`redis` 等前缀的配置注入，见 `server/cmd/app` 与相关 `envconfig` 用法）。

2. 启动后端（在 `server/` 目录）：

   ```bash
   cd server
   make build
   # 配置好环境变量后运行生成的二进制，或使用你现有的启动方式
   ```

3. 启动前端（在 `frontend/` 目录）：

   ```bash
   cd frontend
   pnpm install
   pnpm run dev
   ```

4. 开发模式下，前端通过代理访问后端 GraphQL；默认后端地址见 `frontend/config/config.ts` 中的 `API_URL` 与 `frontend/config/proxy.ts`。

## 部署

以下流程适用于 **新环境** 或 **自托管单机**。生产环境请在此基础上补充监控、备份、密钥管理与网络隔离。

### 1. 准备依赖服务

- **PostgreSQL**：应用主库；若使用向量检索等扩展，需按 `deploy/README.md` 安装 [pgvector](https://github.com/pgvector/pgvector) 等可选扩展。
- **Redis**：缓存与会话相关能力（见后端启动时的 Redis 客户端初始化）。
- **可选**：ClickHouse、NATS 等（策略与日志等模块按需启用，环境变量前缀可参考 `server/README.md` 中策略与 ClickHouse 相关小节）。

### 2. 初始化数据库

在目标库上执行聚合脚本（幂等设计，可重复执行）：

```bash
psql "postgresql://user:pass@host:5432/dbname?sslmode=disable" -f deploy/deploy.sql
```

ClickHouse 等脚本若以注释形式附在 `deploy.sql` 末尾，需在对应实例上**单独**执行。更多说明见 [deploy/README.md](deploy/README.md)。

### 3. 配置环境变量

后端通过 `envconfig` 等读取环境变量（例如 `APP_*`、`postgres`、`redis`、各业务 `*_svc` 前缀）。请根据实际部署填写数据库、Redis、加解密密钥、交易所 API 等配置。

使用仓库根目录 **Makefile** 的 `make docker run` 时，会读取 **`secrets/.docker.env`**（需自行创建，勿提交密钥）。该文件路径在 [Makefile](Makefile) 的 `docker-run` 目标中定义；若使用自定义 Docker 命令，请改为你的 `--env-file` 或 `-e` 注入方式。

### 4. 使用 Docker 部署（推荐）

镜像在同一容器内启动 **Go 后端**（监听 `:3000`）与 **Nginx 静态站点**（监听 `:8000`），Nginx 将 `/query`、`/api/` 等路径反代至本机后端。构建与运行入口在仓库根目录：

```bash
# 构建镜像（默认 linux/amd64）
make docker build

# 首次运行前：创建 secrets/.docker.env 并填写环境变量

# 启动容器（示例：见 Makefile 中的端口、健康检查与自定义 bridge 网络）
make docker run
```

**默认对外端口（与 [Makefile](Makefile) 中 `docker-run` 一致）**

| 宿主机端口 | 说明 |
|------------|------|
| 8000 | Web 界面（Nginx） |
| 3000 | 后端 HTTP（按需是否对公网暴露） |
| 8080 | 健康检查（`curl` 探活路径见 Makefile） |
| 4014 | Metrics |

**说明：**

- `make docker run` 默认使用名为 `alva` 的 Docker 网络；若不存在，请先 `docker network create alva`，或修改 Makefile 中 `--network` 为你的网络名。
- Apple Silicon 上若构建失败，可尝试 `docker build --platform linux/arm64`（需与 [Dockerfile](Dockerfile) 注释中的说明对照）。
- 其它容器操作：`make docker stop`、`make docker rm`、`make docker restart`、`make docker upgrade`。

### 5. 手动编译运行

不通过 Docker 时，可分别在 `server/` 与 `frontend/` 执行生产构建，由反向代理将前端静态资源与 `/query` 等同源转发至后端（与 `docker/nginx.conf` 中的路由思想一致）。

```bash
cd server && make build
cd frontend && pnpm install && pnpm run build
```

前端构建产物在 `frontend/dist`。若访问域名与开发环境不同，请在构建前设置合适的 `UMI_ENV` / `API_URL`（见 `frontend/config/config.ts`），避免将 GraphQL 地址写死为不可达的 `localhost`。

## 常用命令

**后端（`server/`）**

| 命令 | 说明 |
|------|------|
| `make lint` / `make lint-fix` | golangci-lint |
| `make build` | 编译 `cmd/app` |
| `make gqlgen` | 修改 GraphQL schema 后生成代码 |
| `make repo` | 修改 SQL 后生成 sqlc |
| `make convert` | goverter 生成 |

**前端（`frontend/`）**

| 命令 | 说明 |
|------|------|
| `pnpm run dev` | 开发服务 |
| `pnpm run build` | 生产构建 |
| `pnpm run lint` | ESLint + Prettier + TypeScript |

## 文档与规范

- 架构与协作约定：[CLAUDE.md](CLAUDE.md)、[AGENTS.md](AGENTS.md)
- 数据库聚合脚本：[deploy/README.md](deploy/README.md)
- 后端长篇设计与策略说明：[server/README.md](server/README.md)

## 许可证

本项目基于 [Apache License 2.0](LICENSE) 发布。

## 免责声明

本软件仅供学习与技术研究使用。加密货币与衍生品交易具有高风险，使用本软件进行实盘交易的一切后果由使用者自行承担；请遵守所在地法律法规，并自行做好安全审计与密钥保管。
