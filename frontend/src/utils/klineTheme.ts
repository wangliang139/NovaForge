import { useModel } from '@umijs/max';
import { useMemo } from 'react';

export type KlineTheme = 'light' | 'dark';

/** 与全局布局主题（navTheme）对齐，供 lightweight-charts 等 K 线组件使用 */
export function useKlineTheme(): KlineTheme {
  const { initialState } = useModel('@@initialState');
  return useMemo(
    () => (initialState?.settings?.navTheme === 'realDark' ? 'dark' : 'light'),
    [initialState?.settings?.navTheme],
  );
}

export type LightweightKlineThemeTokens = {
  layout: { background: { color: string }; textColor: string };
  grid: { vertLines: { color: string }; horzLines: { color: string } };
  rightPriceScale: { borderColor: string };
  volumeUp: string;
  volumeDown: string;
  headerMuted: string;
  ohlc: string;
  volumeLabel: string;
  markerTooltip: {
    background: string;
    border: string;
    boxShadow: string;
  };
};

export const LIGHTWEIGHT_KLINE_THEME: Record<KlineTheme, LightweightKlineThemeTokens> = {
  light: {
    layout: { background: { color: '#ffffff' }, textColor: '#333333' },
    grid: { vertLines: { color: '#eeeeee' }, horzLines: { color: '#eeeeee' } },
    rightPriceScale: { borderColor: '#cccccc' },
    volumeUp: '#26a69a',
    volumeDown: '#ef5350',
    headerMuted: '#868e9b',
    ohlc: '#cf1322',
    volumeLabel: '#ff7300',
    markerTooltip: {
      background: '#ffffff',
      border: '1px solid #e8e8e8',
      boxShadow: '0 2px 10px rgba(0, 0, 0, 0.08)',
    },
  },
  dark: {
    layout: { background: { color: '#151517' }, textColor: '#d9d9d9' },
    grid: { vertLines: { color: '#292929' }, horzLines: { color: '#292929' } },
    rightPriceScale: { borderColor: '#434343' },
    volumeUp: '#26a69a',
    volumeDown: '#ef5350',
    headerMuted: '#929aa5',
    ohlc: '#ff7875',
    volumeLabel: '#ffa940',
    markerTooltip: {
      background: '#1c1c1f',
      border: '1px solid #292929',
      boxShadow: '0 4px 16px rgba(0, 0, 0, 0.45)',
    },
  },
};
