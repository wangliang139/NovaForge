import defaultAvatar from '@/assets/image/avatar.png';
import bgLayout from '@/assets/image/bg-layout.png';
import { AvatarDropdown, Footer, Question } from '@/components';
import { NewsTicker } from '@/components/Header/NewsTicker';
import { BgColorsOutlined, BookOutlined, MoonOutlined, SunOutlined, SyncOutlined } from '@ant-design/icons';
import type { Settings as LayoutSettings } from '@ant-design/pro-components';
import { SettingDrawer } from '@ant-design/pro-components';
import { ApolloProvider } from '@apollo/client';
import type { RequestConfig, RunTimeLayoutConfig } from '@umijs/max';
import { history, RuntimeAntdConfig } from '@umijs/max';
import type { MenuProps } from 'antd';
import { Dropdown, Space } from 'antd';
import type { ReactNode } from 'react';
import apollo from '../config/apollo';
import defaultSettings from '../config/defaultSettings';
import { errorConfig } from './requestErrorConfig';
import { currentUser as queryCurrentUser } from './services/ant-design-pro/api';

const isDev = process.env.NODE_ENV === 'development';
const THEME_MODE_STORAGE_KEY = 'novaforge-theme-mode';

type ThemeMode = 'system' | 'light' | 'dark';

function readThemeModeFromStorage(): ThemeMode {
  if (typeof window === 'undefined') return 'system';
  const raw = window.localStorage.getItem(THEME_MODE_STORAGE_KEY);
  if (raw === 'system' || raw === 'light' || raw === 'dark') {
    return raw;
  }
  return 'system';
}

function isSystemDark(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function resolveNavTheme(mode: ThemeMode): NonNullable<LayoutSettings['navTheme']> {
  if (mode === 'dark') return 'realDark';
  if (mode === 'light') return 'light';
  return isSystemDark() ? 'realDark' : 'light';
}

function applyThemeToSettings(
  mode: ThemeMode,
  settings?: Partial<LayoutSettings>,
): Partial<LayoutSettings> {
  return {
    ...(settings ?? (defaultSettings as Partial<LayoutSettings>)),
    navTheme: resolveNavTheme(mode),
  };
}

let systemThemeListenerBound = false;

function bindSystemThemeListener(
  setInitialState: (updater: (state: any) => any) => void,
) {
  if (systemThemeListenerBound || typeof window === 'undefined' || !window.matchMedia) return;
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
  const onChange = () => {
    setInitialState((prev: any) => {
      if (!prev || prev.themeMode !== 'system') return prev;
      return {
        ...prev,
        settings: applyThemeToSettings('system', prev.settings),
      };
    });
  };
  mediaQuery.addEventListener('change', onChange);
  systemThemeListenerBound = true;
}

/** 根容器包裹 ApolloProvider，使 layout header 中的 useSubscription 等能访问 client */
export function rootContainer(container: ReactNode) {
  return <ApolloProvider client={apollo}>{container}</ApolloProvider>;
}

/**
 * @see  https://umijs.org/zh-CN/plugins/plugin-initial-state
 * */
export async function getInitialState(): Promise<{
  settings?: Partial<LayoutSettings>;
  themeMode: ThemeMode;
  currentUser?: API.CurrentUser;
  loading?: boolean;
  fetchUserInfo?: () => Promise<API.CurrentUser | undefined>;
}> {
  const fetchUserInfo = async () => {
    try {
      const msg = await queryCurrentUser({
        skipErrorHandler: true,
      });
      return msg.data;
    } catch {
      return undefined;
    }
  };
  const themeMode = readThemeModeFromStorage();
  const isLoginPage =
    typeof window !== 'undefined' && window.location.pathname === '/user/login';
  const currentUser = isLoginPage ? undefined : await fetchUserInfo();
  return {
    fetchUserInfo,
    currentUser,
    themeMode,
    settings: applyThemeToSettings(themeMode, defaultSettings as Partial<LayoutSettings>),
  };
}

export const antd: RuntimeAntdConfig = (memo) => {
  // memo.theme ??= {};
  // memo.theme.algorithm = theme.compactAlgorithm; // 配置 antd5 的预设 dark 算法
  memo.appConfig = {
    message: {
      // 配置 message 最大显示数，超过限制时，最早的消息会被自动关闭
      maxCount: 3,
      top: 100,
      duration: 2,
      rtl: true,
      prefixCls: 'my-message',
    },
  };
  return memo;
};

/** 头像：接口未返回或外链失败时使用站点 logo，避免空白 */
function layoutAvatarSrc(currentUser?: API.CurrentUser): string {
  const url = currentUser?.avatar?.trim();
  if (url) {
    return url;
  }
  return '/logo.svg';
}

// ProLayout 支持的api https://procomponents.ant.design/components/layout
export const layout: RunTimeLayoutConfig = ({ initialState, setInitialState }) => {
  bindSystemThemeListener(setInitialState);

  const onThemeMenuClick: MenuProps['onClick'] = ({ key }) => {
    const mode = key as ThemeMode;
    if (mode !== 'system' && mode !== 'light' && mode !== 'dark') return;
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(THEME_MODE_STORAGE_KEY, mode);
    }
    setInitialState((prev) => ({
      ...prev,
      themeMode: mode,
      settings: applyThemeToSettings(mode, prev?.settings),
    }));
  };

  const themeMenuItems: MenuProps['items'] = [
    { key: 'system', label: '跟随系统', icon: <SyncOutlined /> },
    { key: 'light', label: '浅色主题', icon: <SunOutlined /> },
    { key: 'dark', label: '暗色主题', icon: <MoonOutlined /> },
  ];

  return {
    headerContentRender: () => {
      if (history.location.pathname !== '/exchange/market') return null;
      return <NewsTicker />;
    },
    actionsRender: () => [
      <Space
        key="guide"
        size={3}
        style={{
          height: 36,
          cursor: 'pointer',
        }}
        onClick={() => {
          history.push('/guide');
        }}
      >
        <BookOutlined />
      </Space>,
      <Dropdown
        key="theme"
        trigger={['hover']}
        menu={{
          items: themeMenuItems,
          onClick: onThemeMenuClick,
          selectedKeys: [initialState?.themeMode ?? 'system'],
        }}
      >
        <Space
          size={3}
          style={{
            height: 36,
            cursor: 'pointer',
          }}
        >
          <BgColorsOutlined />
        </Space>
      </Dropdown>,
      <Question key="doc" height={36} />,
    ],
    avatarProps: {
      // src: layoutAvatarSrc(initialState?.currentUser),
      // title: <AvatarName />,
      src: defaultAvatar,
      render: (_, avatarChildren) => {
        return <AvatarDropdown>{avatarChildren}</AvatarDropdown>;
      },
    },
    waterMarkProps: {
      content: initialState?.currentUser?.name,
    },
    footerRender: () => {
      if (history.location.pathname.startsWith('/chat')) return null;
      return <Footer />;
    },
    bgLayoutImgList: [
      {
        src: bgLayout,
        left: 85,
        bottom: 100,
        height: '303px',
      },
      {
        src: bgLayout,
        bottom: -68,
        right: -45,
        height: '303px',
      },
      {
        src: bgLayout,
        bottom: 0,
        left: 0,
        width: '331px',
      },
    ],
    links: isDev
      ? [
        // <Link key="openapi" to="/umi/plugin/openapi" target="_blank">
        //   <LinkOutlined/>
        //   <span>OpenAPI 文档</span>
        // </Link>,
      ]
      : [],
    menuHeaderRender: undefined,
    // 自定义 403 页面
    // unAccessible: <div>unAccessible</div>,
    // 增加一个 loading 的状态
    childrenRender: (children) => {
      // if (initialState?.loading) return <PageLoading />;
      return (
        <>
          {children}
          {isDev && (
            <SettingDrawer
              disableUrlParams
              enableDarkTheme
              settings={initialState?.settings}
              onSettingChange={(settings) => {
                setInitialState((preInitialState: any) => ({
                  ...preInitialState,
                  settings,
                }));
              }}
            />
          )}
        </>
      );
    },
    ...initialState?.settings,
  };
};

/**
 * @name request 配置，可以配置错误处理
 * 它基于 axios 和 ahooks 的 useRequest 提供了一套统一的网络请求和错误处理方案。
 * @doc https://umijs.org/docs/max/request#配置
 */
export const request: RequestConfig = {
  baseURL: API_URL,
  ...errorConfig,
};
