/**
 * @name umi 的路由配置
 * @description 只支持 path,component,routes,redirect,wrappers,name,icon 的配置
 * @param path  path 只支持两种占位符配置，第一种是动态参数 :id 的形式，第二种是 * 通配符，通配符只能出现路由字符串的最后。
 * @param component 配置 location 和 path 匹配后用于渲染的 React 组件路径。可以是绝对路径，也可以是相对路径，如果是相对路径，会从 src/pages 开始找起。
 * @param routes 配置子路由，通常在需要为多个路径增加 layout 组件时使用。
 * @param redirect 配置路由跳转
 * @param wrappers 配置路由组件的包装组件，通过包装组件可以为当前的路由组件组合进更多的功能。 比如，可以用于路由级别的权限校验
 * @param name 配置路由的标题，默认读取国际化文件 menu.ts 中 menu.xxxx 的值，如配置 name 为 login，则读取 menu.ts 中 menu.login 的取值作为标题
 * @param icon 配置路由的图标，取值参考 https://ant.design/components/icon-cn， 注意去除风格后缀和大小写，如想要配置图标为 <StepBackwardOutlined /> 则取值应为 stepBackward 或 StepBackward，如想要配置图标为 <UserOutlined /> 则取值应为 user 或者 User
 * @doc https://umijs.org/docs/guides/routes
 */
export default [
  // 登录等页面置于顶层并设置 layout: false，避免显示侧边栏
  {
    path: '/user/login',
    layout: false,
    hideInMenu: true,
    name: 'login',
    component: './user/login',
  },
  {
    path: '/dashboard',
    name: 'Dashboard',
    icon: 'dashboard',
    routes: [
      {
        path: '/dashboard',
        redirect: '/dashboard/analysis',
      },
      {
        name: 'Analysis',
        icon: 'smile',
        path: '/dashboard/analysis',
        component: './dashboard/analysis',
      },
      {
        name: 'Monitor',
        icon: 'smile',
        path: '/dashboard/monitor',
        component: './dashboard/monitor',
      },
    ],
  },
  {
    path: '/chat',
    name: '聊天',
    icon: 'MessageOutlined',
    component: './chat',
    routes: [
      {
        path: '/chat',
        redirect: '/chat/new',
      },
      {
        path: '/chat/:sessionId',
        name: '聊天',
        component: './chat',
        hideInMenu: true,
      },
      {
        path: '/chat/new',
        name: '聊天',
        component: './chat',
        hideInMenu: true,
      },
    ]
  },
  {
    path: '/exchange/market',
    name: '交易终端',
    icon: 'TransactionOutlined',
    component: './exchange/market',
  },
  {
    path: '/account',
    name: '交易账户',
    icon: 'AccountBookOutlined',
    component: './account',
    routes: [
      {
        path: '/account/:id',
        name: '账户详情',
        component: './account/detail',
        hideInMenu: true,
      },
    ],
  },
  {
    path: '/bot',
    name: 'Bot 实例',
    icon: 'DockerOutlined',
    component: './bots',
    routes: [
      {
        path: '/bot/:id',
        name: 'Detail',
        component: './bots/detail',
        hideInMenu: true,
      },
    ],
  },
  {
    path: '/strategy',
    name: '策略库',
    icon: 'ProductOutlined',
    routes: [
      {
        path: '/strategy',
        component: './strategy',
      },
      {
        path: '/strategy/:id',
        name: '策略详情',
        component: './strategy/detail',
        hideInMenu: true,
      },
    ],
  },
  {
    path: '/datasource',
    name: '数据源',
    icon: 'DatabaseOutlined',
    component: './datasource',
    hideInMenu: true,
  },
  {
    path: '/guide',
    name: 'Guide',
    icon: 'BookOutlined',
    component: './guide',
    hideInMenu: true,
  },
  {
    path: '/document',
    name: '市场资讯',
    icon: 'FileTextOutlined',
    component: './document/documents',
  },
  {
    path: '/calendar',
    name: '财经日历',
    icon: 'CalendarOutlined',
    component: './document/calendar',
  },
  {
    path: '/llm',
    name: 'LLM',
    icon: 'brain',
    component: './llm',
    hideInMenu: true,
  },
  {
    name: 'Exception',
    icon: 'warning',
    path: '/exception',
    hideInMenu: true,
    routes: [
      {
        path: '/exception',
        redirect: '/exception/403',
      },
      {
        name: '403',
        icon: 'smile',
        path: '/exception/403',
        component: './exception/403',
      },
      {
        name: '404',
        icon: 'smile',
        path: '/exception/404',
        component: './exception/404',
      },
      {
        name: '500',
        icon: 'smile',
        path: '/exception/500',
        component: './exception/500',
      },
    ],
  },
  {
    path: '/user/register-result',
    name: 'register-result',
    icon: 'smile',
    component: './user/register-result',
    hideInMenu: true,
  },
  {
    path: '/user/register',
    name: 'register',
    icon: 'smile',
    component: './user/register',
    hideInMenu: true,
  },
  {
    path: '/user/center',
    redirect: '/user',
    hideInMenu: true,
  },
  {
    path: '/user/settings',
    redirect: '/user',
    hideInMenu: true,
  },
  {
    name: '用户中心',
    icon: 'user',
    path: '/user',
    component: './user/settings',
  },
  {
    component: '404',
    path: '/user/*',
    hideInMenu: true,
  },
  {
    path: '/',
    redirect: '/dashboard/analysis',
  },
  {
    component: '404',
    path: '/*',
  },
];
