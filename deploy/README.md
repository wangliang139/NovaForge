# 数据库部署

本目录包含从 **llt-data-api** 与 **llt-strategy-api** 各 `pkg/repos/*/schema.sql` 整理合并后的统一部署脚本。

## deploy.sql（PostgreSQL）

- **用途**：在 PostgreSQL 上一次性创建两服务所需的表、枚举与索引。
- **依赖**：需安装 [pgvector](https://github.com/pgvector/pgvector) 扩展（`document.embedding` 向量列）。  
  若使用 document 表的 BM25 全文检索，需单独安装 pg_bm25 并在脚本中取消对应索引的注释后执行。
- **执行**：
  ```bash
  psql "postgresql://user:pass@host:5432/dbname?sslmode=disable" -f deploy/deploy.sql
  ```
- **幂等**：脚本中枚举类型已做“存在则跳过”处理，表与索引均使用 `IF NOT EXISTS`，可重复执行。

## ClickHouse 表

`deploy.sql` 文件末尾以注释形式保留了 **llt-data-api** 的 `trade.event_flow` 与 **llt-strategy-api** 的 `trade.bot_signal_flow` 的 ClickHouse DDL。  
这两张表需在 ClickHouse 实例上单独执行，不能通过上述 PostgreSQL 脚本创建。

## 与 server 服务 schema 的同步

- 日常开发仍以 server 服务内 `pkg/repos/*/schema.sql` 为准，由 sqlc 生成代码。
- 修改了某个表的 schema 后，请同步更新本目录下的 `deploy.sql`，以便新环境或迁移时与服务定义一致。
