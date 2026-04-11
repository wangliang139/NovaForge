# NovaForge 前端

你是一位精通 React、TypeScript、Ant Design Pro、GraphQL 和现代前端开发的专家。

## 项目概述

本目录为 **NovaForge** 的桌面端 Web 界面，基于 Ant Design Pro 与 UmiJS，通过 GraphQL 与同仓库的 **server** 单体后端通信。功能覆盖账户、交易所、文档、项目、Meme 分析等模块（以实际页面为准）。

## 技术栈

- **语言**: TypeScript 5.8+
- **框架**: React 18.3+, UmiJS 4.4
- **UI 组件**: Ant Design 5.26, Ant Design Pro 2.8
- **数据获取**: Apollo Client 3.13, GraphQL
- **状态管理**: UmiJS 内置状态管理
- **路由**: UmiJS 路由
- **构建工具**: Umi Max
- **开发工具**: ESLint, Prettier, Jest

## 核心功能模块

### 页面模块

- **Account**: 账户管理、个人设置
- **Calendar**: 日历事件管理
- **Dashboard**: 分析面板、监控面板、工作台
- **Document**: 文档管理
- **Exchange**: 账户管理、套利监控、实时监控
- **Project**: 项目管理、交易对管理
- **Meme**: Meme Token 分析
- **Tools**: 工具和 API 接口

### 组件和工具

- 自定义组件: Footer, HeaderDropdown, RightContent, Tag
- WebSocket 支持: `hooks/websocket.ts`
- 工具函数: datetime, dict, exchange, math 等

## 开发规范

### 文件组织

- 使用小写字母和连字符命名目录（如 `account-center`）
- 通用组件放在 `components` 子目录
- 页面组件放在 `pages` 子目录，按功能模块组织
- 每个页面模块包含 `index.tsx`、`service.ts`、`types.ts` 等文件
- 服务函数放在 `service.ts` 文件中
- 类型定义放在 `types.ts` 文件中
- 页面组件的入口文件是 `index.tsx`
- 样式使用 Style 后缀（如 `Center.style.ts`）

### 组件编写

- 使用函数式组件和 React Hooks
- 优先使用 Ant Design Pro 组件
- 使用 TypeScript 严格模式，确保类型安全
- 遵循 React 最佳实践：单一职责、组件复用

### GraphQL 集成

- 使用 Apollo Client 进行 GraphQL 查询和变更
- GraphQL 定义在 GraphQL schema 中
- 使用自动生成的 TypeScript 类型

### 路径别名

- `@/` 指向 `src/`
- `@@/` 指向 `.umi/`

### 示例代码

```typescript
// 使用 GraphQL Query
import { useQuery } from '@apollo/client';
import { gql } from 'graphql';

const GET_ACCOUNTS = gql`
  query GetAccounts($input: QueryAccountsInput!) {
    Accounts(input: $input) {
      totalCount
      list {
        id
        name
        exchange
        status
      }
    }
  }
`;

function AccountList() {
  const { data, loading, error } = useQuery(GET_ACCOUNTS, {
    variables: {
      input: {
        limit: 20,
        offset: 0,
      },
    },
  });

  if (loading) return <Spin />;
  if (error) return <Alert message={error.message} type="error" />;

  return (
    <ProTable
      columns={columns}
      dataSource={data.Accounts.list}
      pagination={{ total: data.Accounts.totalCount }}
    />
  );
}
```

### 样式规范

- 使用 styled-components 或 CSS-in-JS
- 遵循 Ant Design 设计规范
- 响应式设计，支持移动端和桌面端

### 状态管理

- 使用 UmiJS 的 model 进行全局状态管理
- 组件内状态使用 useState Hook
- 使用 Context API 处理深层传递的状态

### 性能优化

- 使用 React.memo 避免不必要的重渲染
- 使用式加载 (lazy loading) 减少首屏加载时间
- 合理使用 useMemo 和 useCallback

### 错误处理

- 使用 Apollo Client 的错误处理机制
- 在组件中优雅处理错误状态
- 显示用户友好的错误信息

### 表单处理

- 使用 Ant Design 的 Form 组件
- 使用 ProForm 提供的高级表单功能
- 实现表单验证和提交逻辑

### WebSocket 实时更新

- 使用 `hooks/websocket.ts` 提供的 WebSocket Hook
- 在需要实时数据的页面集成 WebSocket
- 处理连接断开和重连逻辑

## 构建和部署

- `pnpm run dev`: 开发模式启动
- `pnpm run build`: 构建生产版本
- `pnpm run lint`: 代码检查
- `pnpm run test`: 运行测试

## 与后端集成

- 通过 Apollo Client 调用 **NovaForge server** 提供的 GraphQL（路径与 `API_URL` 等见 `config/apollo.ts`）
- GraphQL endpoint 配置在 `config/apollo.ts`
- 支持 WebSocket 订阅时，与后端订阅端点一致即可
