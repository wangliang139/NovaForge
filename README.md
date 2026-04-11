# NovaForge

NovaForge 是多交易所 Web3 交易引擎，支持跨市场策略执行、自动化与信号驱动交易。

## 仓库结构（单体）

- **`server/`**：Go 单体后端（GraphQL、策略运行时、数据访问与任务等）。开发与构建命令见 `server/README.md` 与 `server/Makefile`。
- **`frontend/`**：React / TypeScript 管理界面（UmiJS、Ant Design Pro、Apollo Client）。
- **`deploy/`**：数据库初始化脚本说明见 `deploy/README.md`。
- **`docs/`**：策略与指标等技术说明文档。
- **`skills/novaforge-connector/`**：通过 `scripts/cli.js` 调用 GraphQL 的辅助脚本与 API 参考（见该目录 `SKILL.md`）。

本地开发时通常分别启动 `server` 与 `frontend`，前端通过 GraphQL 访问后端（具体 URL 见前端配置）。
