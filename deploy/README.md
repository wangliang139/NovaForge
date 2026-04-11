# 数据库部署

本目录提供 **NovaForge** 单体后端（`server/pkg/repos`）相关的 **PostgreSQL** 等一体化部署脚本，由历史 schema 合并整理而来，便于本机或单机环境一次性初始化。

## deploy.sql（PostgreSQL）

- **用途**：在 PostgreSQL 上创建应用所需的表、枚举与索引。
- **依赖**：若使用向量列，需安装 [pgvector](https://github.com/pgvector/pgvector)。若使用 document 表的 BM25 全文检索，需单独安装 pg_bm25 并在脚本中按需取消注释。
- **执行**：
  ```bash
  psql "postgresql://user:pass@host:5432/dbname?sslmode=disable" -f deploy/deploy.sql
  ```
- **幂等**：枚举类型已做“存在则跳过”处理，表与索引使用 `IF NOT EXISTS`，可重复执行。

## ClickHouse 表

`deploy.sql` 末尾以注释形式保留了 **事件流**（`trade.event_flow`）与 **Bot 信号流**（`trade.bot_signal_flow`）的 ClickHouse DDL。  
这两张表需在 ClickHouse 实例上单独执行，不能通过上述 PostgreSQL 脚本创建。

## 与 server 服务 schema 的同步

- 日常开发以 `server/pkg/repos/*/schema.sql` 为准，由 sqlc 生成代码。
- 修改表结构后，请同步更新本目录下的 `deploy.sql`，保证新环境与迁移脚本与运行时代码一致。
