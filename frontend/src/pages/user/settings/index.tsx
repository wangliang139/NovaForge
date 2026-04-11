import { GridContent } from '@ant-design/pro-components';
import { useSearchParams } from '@umijs/max';
import { Menu } from 'antd';
import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import ApiKeysView from './components/apikeys';
import BindingView from './components/binding';
import LlmView from './components/llm';
import ProxyView from './components/proxy';
import SecurityView from './components/security';
import NewsSourceView from './components/news-source';
import TelegramView from './components/telegram';
import useStyles from './style.style';

const TAB_QUERY = 'tab';

const MENU_TAB_KEYS = ['security', 'telegram', 'newsSource', 'llm', 'proxy', 'apikeys'] as const;
type MenuTabKey = (typeof MENU_TAB_KEYS)[number];

type SettingsStateKeys = MenuTabKey | 'binding';

function tabFromSearch(tabParam: string | null): MenuTabKey {
  if (tabParam && (MENU_TAB_KEYS as readonly string[]).includes(tabParam)) {
    return tabParam as MenuTabKey;
  }
  return 'security';
}

const Settings: React.FC = () => {
  const { styles } = useStyles();
  const [searchParams, setSearchParams] = useSearchParams();
  const selectKey = useMemo(
    () => tabFromSearch(searchParams.get(TAB_QUERY)),
    [searchParams],
  ) as SettingsStateKeys;

  useEffect(() => {
    const raw = searchParams.get(TAB_QUERY);
    if (raw !== null && !(MENU_TAB_KEYS as readonly string[]).includes(raw)) {
      const next = new URLSearchParams(searchParams);
      next.delete(TAB_QUERY);
      setSearchParams(next, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  const menuMap: Record<string, React.ReactNode> = {
    security: '安全设置',
    telegram: '消息推送',
    newsSource: '资讯来源',
    llm: 'AI 大脑',
    proxy: '代理设置',
    apikeys: 'API 密钥',
  };

  const [mode, setMode] = useState<'inline' | 'horizontal'>('inline');
  const dom = useRef<HTMLDivElement>();

  const resize = useCallback(() => {
    requestAnimationFrame(() => {
      if (!dom.current) {
        return;
      }
      let nextMode: 'inline' | 'horizontal' = 'inline';
      const { offsetWidth } = dom.current;
      if (dom.current.offsetWidth < 641 && offsetWidth > 400) {
        nextMode = 'horizontal';
      }
      if (window.innerWidth < 768 && offsetWidth > 400) {
        nextMode = 'horizontal';
      }
      setMode(nextMode);
    });
  }, []);

  useLayoutEffect(() => {
    if (dom.current) {
      window.addEventListener('resize', resize);
      resize();
    }
    return () => {
      window.removeEventListener('resize', resize);
    };
  }, [resize]);

  const handleMenuClick = useCallback(
    ({ key }: { key: string }) => {
      const next = new URLSearchParams(searchParams);
      next.set(TAB_QUERY, key);
      setSearchParams(next, { replace: false });
    },
    [searchParams, setSearchParams],
  );

  const getMenu = () => {
    return Object.keys(menuMap).map((item) => ({ key: item, label: menuMap[item] }));
  };

  const renderChildren = () => {
    switch (selectKey) {
      case 'security':
        return <SecurityView />;
      case 'apikeys':
        return <ApiKeysView />;
      case 'binding':
        return <BindingView />;
      case 'telegram':
        return <TelegramView />;
      case 'newsSource':
        return <NewsSourceView />;
      case 'llm':
        return <LlmView />;
      case 'proxy':
        return <ProxyView />;
      default:
        return null;
    }
  };

  const title = menuMap[selectKey];

  return (
    <GridContent>
      <div
        className={styles.main}
        ref={(ref) => {
          if (ref) {
            dom.current = ref;
          }
        }}
      >
        <div className={styles.leftMenu}>
          <Menu
            mode={mode}
            selectedKeys={[selectKey]}
            onClick={handleMenuClick}
            items={getMenu()}
          />
        </div>
        <div className={styles.right}>
          <div className={styles.title}>{title}</div>
          {renderChildren()}
        </div>
      </div>
    </GridContent>
  );
};
export default Settings;
