import { api } from '@/services/gateway';
import { getOrders, Order, Position } from '@/services/gateway/account';
import { Kline, StreamEvent } from '@/services/gateway/market';
import { SUB_STREAM } from '@/services/gateway/subscription';
import { useApolloClient } from '@apollo/client';
import { useModel } from '@umijs/max';
import type { AlertItem, AlertItemInput, ChartPro, Datafeed, HisOrder, Period, SymbolInfo } from '@wangliang139/klinecharts-pro';
import { KLineChartPro as KLineChartProClass } from '@wangliang139/klinecharts-pro';
import '@wangliang139/klinecharts-pro/dist/klinecharts-pro.css';
import { message } from 'antd';
import { createStyles } from 'antd-style';
import type { DeepPartial, KLineData, Styles } from 'klinecharts';
import React, { useCallback, useEffect, useMemo, useRef } from 'react';

/** 空闲检测间隔：超过此时间未收到数据会触发重连 */
const IDLE_RECONNECT_MS = 50_000;
/** 错误重连最大次数 */
const MAX_ERROR_RETRY = 10;
/** 历史 K 线请求条数 */
const HISTORY_KLINE_LIMIT = 300;
/** 历史订单按完结时间分页拉取 */
const HISTORY_ORDERS_PAGE_SIZE = 200;
const HISTORY_ORDERS_MAX_PAGES = 25;

type StreamPayload = { data?: { Stream?: StreamEvent } };

type ChartDomBindings = {
  el: HTMLElement;
  wheelHandler: (event: WheelEvent) => void;
  ro: ResizeObserver;
};

export type HisOrdersBridge = {
  accountId?: string;
  merge: (orders: Order[]) => void;
};

function isTerminalOrderStatus(status: string | undefined): boolean {
  if (!status) return false;
  const s = String(status).toLowerCase();
  return s === 'done' || s === 'canceled' || s === 'cancelled' || s === 'rejected' || s === 'expired';
}

function orderToHisOrder(o: Order): HisOrder | null {
  const ts = Number(o.finishedTs);
  if (!Number.isFinite(ts) || ts <= 0) return null;
  const price = Number(o.avgPrice) || Number(o.price);
  const ex = Number(o.executedQty);
  const orig = Number(o.originalQty);
  const size = Number.isFinite(ex) && ex > 0 ? ex : orig;
  if (!Number.isFinite(price) || !Number.isFinite(size) || size <= 0) return null;
  const feeN = Number(o.fee);
  const pnlN = Number(o.realizedPnl);
  const side = o.side === 'long' || o.side === 'short' ? o.side : undefined;
  return {
    id: o.clientOrderId || o.orderId,
    orderId: o.orderId,
    symbol: o.symbol,
    side,
    isBuy: o.isBuy,
    timestamp: ts,
    price,
    size,
    fee: Number.isFinite(feeN) ? feeN : undefined,
    pnl: Number.isFinite(pnlN) ? pnlN : undefined,
  };
}

function hisOrderDedupeKey(o: Order): string {
  const id = String(o.orderId || '').trim();
  if (id) return `oid:${id}`;
  return `cid:${String(o.clientOrderId || '').trim()}`;
}
type SubscriptionHandle = { unsubscribe: () => void };
type ApolloSubscriptionClient = {
  subscribe(options: { query: unknown; variables: object }): { subscribe(handlers: object): SubscriptionHandle };
};

const useStyles = createStyles(() => ({
  klineChartProWrapper: {
    '--klinecharts-pro-primary-color': '#1677ff',
    '--klinecharts-pro-hover-background-color': 'rgba(22, 119, 255, 0.15)',
    '--klinecharts-pro-background-color': '#FFFFFF',
    '--klinecharts-pro-popover-background-color': '#FFFFFF',
    '--klinecharts-pro-text-color': '#051441',
    '--klinecharts-pro-text-second-color': '#76808F',
    '--klinecharts-pro-border-color': '#ebedf1',
    '--klinecharts-pro-selected-color': 'rgba(22, 119, 255, 0.15)',
    '.klinecharts-pro-period-bar > div.symbol': {
      display: 'none !important',
    },
    '&[data-kline-theme="dark"]': {
      '--klinecharts-pro-primary-color': '#1677ff',
      '--klinecharts-pro-hover-background-color': 'rgba(22, 119, 255, 0.15)',
      '--klinecharts-pro-background-color': '#151517',
      '--klinecharts-pro-popover-background-color': '#1c1c1f',
      '--klinecharts-pro-text-color': '#F8F8F8',
      '--klinecharts-pro-text-second-color': '#929AA5',
      '--klinecharts-pro-border-color': '#292929',
      '--klinecharts-pro-selected-color': 'rgba(22, 119, 255, 0.15)',
    },
  },
}));

type KlineTheme = 'light' | 'dark';

function buildKlineCoreStyles(theme: KlineTheme): DeepPartial<Styles> {
  const styles = {
    candle: {
      tooltip: {
        title: { show: false, template: '{ticker} · {period}' },
      },
    }
  };
  if (theme === 'dark') {
    return {
      ...styles,
      separator: {
        color: '#292929',
      },
      grid: {
        horizontal: { color: '#292929' },
        vertical: { color: '#292929' },
      },
      xAxis: {
        axisLine: { color: '#292929' },
        tickText: { color: '#929AA5' },
      },
      yAxis: {
        axisLine: { color: '#292929' },
        tickText: { color: '#929AA5' },
      },
      crosshair: {
        horizontal: { line: { color: '#929AA5' } },
        vertical: { line: { color: '#929AA5' } },
      },
    };
  }
  return {
    ...styles,
    separator: {
      color: '#ebedf1',
    },
    grid: {
      horizontal: { color: '#ebedf1' },
      vertical: { color: '#ebedf1' },
    },
    xAxis: {
      axisLine: { color: '#ebedf1' },
      tickText: { color: '#76808F' },
    },
    yAxis: {
      axisLine: { color: '#ebedf1' },
      tickText: { color: '#76808F' },
    },
    crosshair: {
      horizontal: { line: { color: '#76808F' } },
      vertical: { line: { color: '#76808F' } },
    },
  };
}

export interface KlineChartProProps {
  exchange: string;
  symbol: string;
  /** 有值时加载/订阅该账户在历史 K 线时间范围内的完结订单并绘制 */
  accountId?: string;
  /**
   * 为 false 时不创建/会销毁图表（例如 Market 精度未就绪）。默认 true。
   * 与「仅卸载子树」配合可避免切换 symbol 时整表反复 new。
   */
  dataReady?: boolean;
  height?: number;
  visible?: boolean;
  /** 价格精度，不传则默认 6 */
  pricePrecision?: number;
  volumePrecision?: number;
  positions?: Position[];
  liqPrice?: number | null;
  openOrders?: Order[];
}

function klinesToKLineData(klines: Kline[]): KLineData[] {
  if (!Array.isArray(klines) || klines.length === 0) return [];
  const byTime = new Map<number, KLineData>();
  for (const k of klines) {
    const ts = Number(k?.openTs);
    const open = Number(k?.open);
    const high = Number(k?.high);
    const low = Number(k?.low);
    const close = Number(k?.close);
    const volume = Number(k?.volume);
    const turnover = Number(k?.quoteVolume);
    if (!Number.isFinite(ts) || ts <= 0) continue;
    if (!Number.isFinite(open) || !Number.isFinite(high) || !Number.isFinite(low) || !Number.isFinite(close)) continue;
    byTime.set(ts, {
      timestamp: ts,
      open,
      high,
      low,
      close,
      volume: Number.isFinite(volume) ? volume : undefined,
      turnover: Number.isFinite(turnover) ? turnover : undefined,
    });
  }
  return Array.from(byTime.entries())
    .sort(([a], [b]) => a - b)
    .map(([, v]) => v);
}

/** 单条 K 线转图表数据，避免订阅流中为单条创建数组/Map 的开销 */
function klineToKLineDataItem(k: Kline): KLineData | null {
  const ts = Number(k?.openTs);
  const open = Number(k?.open);
  const high = Number(k?.high);
  const low = Number(k?.low);
  const close = Number(k?.close);
  const volume = Number(k?.volume);
  const turnover = Number(k?.quoteVolume);
  if (!Number.isFinite(ts) || ts <= 0) return null;
  if (!Number.isFinite(open) || !Number.isFinite(high) || !Number.isFinite(low) || !Number.isFinite(close)) return null;
  return {
    timestamp: ts,
    open,
    high,
    low,
    close,
    volume: Number.isFinite(volume) ? volume : undefined,
    turnover: Number.isFinite(turnover) ? turnover : undefined,
  };
}

/**
 * 若事件 interval 与当前选中 period 不一致则跳过并打 warn，返回 true 表示应跳过
 */
function shouldSkipKlineByInterval(
  eventInterval: string | undefined,
  currentPeriod: string | undefined,
): boolean {
  const current = (currentPeriod ?? '').trim();
  const event = String(eventInterval ?? '').trim();
  if (!current || !event) return false;
  if (event === current) return false;
  console.warn('KlineChartPro: 跳过 interval 不匹配的 K 线事件', { eventInterval, currentInterval: currentPeriod });
  return true;
}

/** 将 "1m" | "5m" | "1w" | "1M" 等转为 Pro Period */
function intervalToProPeriod(interval: string): Period {
  const raw = String(interval || '').trim();
  const n = parseInt(raw.replace(/[smhdwM]/g, ''), 10) || 1;
  const num = Math.max(1, Math.min(1000, n));
  const text = interval || `${num}m`;
  if (raw.endsWith('M')) return { span: num, type: 'month', text };
  if (raw.endsWith('s')) return { span: num, type: 'second', text };
  if (raw.endsWith('m')) return { span: num, type: 'minute', text };
  if (raw.endsWith('h')) return { span: num, type: 'hour', text };
  if (raw.endsWith('d')) return { span: num, type: 'day', text };
  if (raw.endsWith('w')) return { span: num, type: 'week', text };
  return { span: num, type: 'minute', text: interval || '1m' };
}

function dtoToChartAlert(item: api.AlertItemDTO): AlertItem | null {
  const price = item.price != null ? Number(item.price) : undefined;
  const percent = item.percent != null ? Number(item.percent) : undefined;
  return {
    id: item.id,
    type: item.type as AlertItem['type'],
    frequency: item.frequency,
    price: Number.isFinite(price) ? price : undefined,
    window: item.window,
    percent: Number.isFinite(percent) ? percent : undefined,
    remark: item.remark,
    symbol: item.symbol,
  };
}

function inputToApiAlertInput(exchange: string, symbol: string, alert: AlertItemInput): api.AddAlertInputDTO {
  return {
    exchange,
    symbol,
    type: alert.type,
    frequency: alert.frequency,
    price: alert.price != null && Number.isFinite(alert.price) ? String(alert.price) : undefined,
    window: alert.window,
    percent: alert.percent != null && Number.isFinite(alert.percent) ? String(alert.percent) : undefined,
    remark: alert.remark,
  };
}

function getAlertErrorMessage(error: unknown, fallback: string): string {
  const raw = (error as Error)?.message || '';
  const msg = String(raw);
  if (msg.includes('ALERT_DUPLICATED')) return '该预警条件已存在，请勿重复添加';
  if (msg.includes('ALERT_LIMIT_PER_SYMBOL_EXCEEDED')) return '该交易对预警已达上限（最多 20 条）';
  if (msg.includes('ALERT_LIMIT_GLOBAL_EXCEEDED')) return '系统预警总数已达上限（最多 200 条）';
  if (msg.includes('ALERT_INVALID_FIELD_COMBINATION')) return '预警参数不合法，请检查价格/窗口/涨跌幅设置';
  if (msg.includes('ALERT_NOT_FOUND')) return '预警不存在或已被删除';
  return msg || fallback;
}

const DEFAULT_PERIODS: Period[] = [
  { span: 1, type: 'minute', text: '1m' },
  { span: 5, type: 'minute', text: '5m' },
  { span: 15, type: 'minute', text: '15m' },
  { span: 1, type: 'hour', text: '1h' },
  { span: 4, type: 'hour', text: '4h' },
  { span: 1, type: 'day', text: '1d' },
  { span: 1, type: 'week', text: '1w' },
  { span: 1, type: 'month', text: '1M' },
];

function createCustomDatafeed(
  snapshotRef: React.MutableRefObject<{ exchange: string; symbol: string; period: string } | null>,
  apolloClientRef: React.MutableRefObject<unknown>,
  subRef: React.MutableRefObject<SubscriptionHandle | null>,
  reconnectRef: React.MutableRefObject<{ reconnect: () => void; cancel: () => void } | null>,
  hisOrdersRef: React.MutableRefObject<HisOrdersBridge>,
): Datafeed {
  let lastReceiveTime = 0;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let isReconnecting = false;
  let currentCallback: ((data: KLineData) => void) | null = null;
  let errorRetryCount = 0;
  /** 同 chart 实例内相同 (exchange,symbol,period,from,to) 的并发 getHistory 合并为一次 queryKline */
  const inflightHistoryByKey = new Map<string, Promise<KLineData[]>>();

  const startReconnectTimer = () => {
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(() => {
      if (!currentCallback) return;
      const diff = Date.now() - lastReceiveTime;
      if (diff >= IDLE_RECONNECT_MS) {
        console.log('KlineChartPro: 长时间未收到数据，尝试重新订阅...');
        isReconnecting = true;
        reconnectRef.current?.reconnect();
      } else {
        startReconnectTimer();
      }
    }, IDLE_RECONNECT_MS);
  };

  const clearReconnectTimer = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    errorRetryCount = 0;
  };

  const scheduleErrorReconnect = () => {
    if (!currentCallback) return;
    if (errorRetryCount >= MAX_ERROR_RETRY) {
      console.warn('KlineChartPro: 订阅多次重试失败，停止自动重连');
      return;
    }
    errorRetryCount += 1;
    const delay = Math.min(1000 * 2 ** (errorRetryCount - 1), IDLE_RECONNECT_MS);
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    console.warn(
      `KlineChartPro: 订阅断开，准备自动重连，第 ${errorRetryCount} 次重试，${delay}ms 后执行`,
    );
    reconnectTimer = setTimeout(() => {
      if (!currentCallback) return;
      isReconnecting = true;
      reconnectRef.current?.reconnect();
    }, delay);
  };

  reconnectRef.current = {
    reconnect: () => {
      if (subRef.current) {
        try {
          subRef.current.unsubscribe();
        } catch (_) { /* ignore */ }
        subRef.current = null;
      }
      if (snapshotRef.current && apolloClientRef.current && currentCallback) {
        const client = apolloClientRef.current as ApolloSubscriptionClient | null;
        if (client) {
          const exchange = snapshotRef.current.exchange;
          const sym = snapshotRef.current.symbol;
          const interval = snapshotRef.current.period;
          const variables = {
            input: { type: 'kline' as const, exchange, symbol: sym, interval },
          };
          const sub = client
            .subscribe({ query: SUB_STREAM, variables })
            .subscribe({
              next: (payload: StreamPayload) => {
                const nextKline = payload?.data?.Stream?.kline;
                if (!nextKline) return;
                if (shouldSkipKlineByInterval(nextKline.interval, snapshotRef.current?.period)) return;
                lastReceiveTime = Date.now();
                const item = klineToKLineDataItem(nextKline);
                if (item) currentCallback?.(item);
              },
              error: (e: unknown) => {
                console.error('KlineChartPro: 重新订阅出错', e);
                scheduleErrorReconnect();
              },
            });
          subRef.current = sub as SubscriptionHandle;
          lastReceiveTime = Date.now();
          errorRetryCount = 0;
          startReconnectTimer();
        }
      }
    },
    cancel: () => {
      clearReconnectTimer();
    },
  };

  return {
    searchSymbols(search?: string): Promise<SymbolInfo[]> {
      const symbol = snapshotRef.current?.symbol;
      if (!symbol) return Promise.resolve([]);
      const sym: SymbolInfo = { ticker: symbol, shortName: symbol };
      if (!search?.trim()) return Promise.resolve([sym]);
      const q = search.trim().toUpperCase();
      return symbol.toUpperCase().includes(q) ? Promise.resolve([sym]) : Promise.resolve([]);
    },
    async getHistoryKLineData(
      symbol: SymbolInfo,
      period: Period,
      from: number,
      to: number,
    ): Promise<KLineData[]> {
      console.log('getHistoryKLineData', symbol, period, from, to);
      if (from >= to) return Promise.resolve([]);
      const exchange = symbol.exchange;
      const sym = symbol.ticker;
      if (!exchange || !sym) return Promise.resolve([]);
      const periodText = period.text || '';

      const key = `${exchange}\0${sym}\0${periodText}\0${from}\0${to}`;
      const inflight = inflightHistoryByKey.get(key);
      if (inflight) {
        return inflight;
      }

      const fetchPromise = (async (): Promise<KLineData[]> => {
        try {
          const history = (await api.queryKline({
            exchange,
            symbol: sym,
            interval: periodText,
            endTime: to,
            limit: HISTORY_KLINE_LIMIT,
          })) as Kline[];
          const list = klinesToKLineData(history);
          if (list.length === 0) return [];

          const filtered = list.filter((d) => d.timestamp >= from && d.timestamp <= to);
          const out = filtered.length > 0 ? filtered : list;

          const bridge = hisOrdersRef.current;
          const aid = bridge.accountId?.trim();
          if (aid && sym) {
            const rangeFrom = Math.floor(Number(from));
            const rangeTo = Math.floor(Number(to));
            void (async () => {
              try {
                const merged: Order[] = [];
                for (let page = 1; page <= HISTORY_ORDERS_MAX_PAGES; page++) {
                  const conn = await getOrders({
                    accountId: aid,
                    symbol: sym,
                    includeFinished: true,
                    finishedStartTsMs: rangeFrom,
                    finishedEndTsMs: rangeTo,
                    page,
                    size: HISTORY_ORDERS_PAGE_SIZE,
                  });
                  const batch = conn?.list ?? [];
                  merged.push(...batch);
                  const total = Number(conn?.totalCount) || 0;
                  if (merged.length >= total || batch.length < HISTORY_ORDERS_PAGE_SIZE) break;
                }
                bridge.merge(merged);
              } catch (err) {
                console.warn('KlineChartPro: 加载历史订单失败', err);
              }
            })();
          }

          return out;
        } catch (e: unknown) {
          message.error((e as Error)?.message || '加载历史K线失败');
          return [];
        } finally {
          inflightHistoryByKey.delete(key);
        }
      })();

      inflightHistoryByKey.set(key, fetchPromise);
      return fetchPromise;
    },
    subscribe(symbol: SymbolInfo, period: Period, callback: (data: KLineData) => void): void {
      const client = apolloClientRef.current as ApolloSubscriptionClient | null;
      if (!client) return;

      if (subRef.current) {
        try {
          subRef.current.unsubscribe();
        } catch (_) { /* ignore */ }
        subRef.current = null;
      }

      currentCallback = callback;
      lastReceiveTime = Date.now();
      isReconnecting = false;
      clearReconnectTimer();

      const exchange = symbol.exchange;
      const sym = symbol.ticker;
      const interval = period.text || '1m';
      if (snapshotRef.current) {
        snapshotRef.current.period = interval;
      }
      const variables = {
        input: { type: 'kline' as const, exchange, symbol: sym, interval },
      };

      const sub = client
        .subscribe({
          query: SUB_STREAM,
          variables,
        })
        .subscribe({
          next: (payload: StreamPayload) => {
            const nextKline = payload?.data?.Stream?.kline;
            if (!nextKline) return;
            if (shouldSkipKlineByInterval(nextKline.interval, snapshotRef.current?.period)) return;
            lastReceiveTime = Date.now();
            if (isReconnecting) {
              console.log('KlineChartPro: 重新订阅成功');
              isReconnecting = false;
            }
            const item = klineToKLineDataItem(nextKline);
            if (item) callback(item);
          },
          error: (e: unknown) => {
            console.error('KlineChartPro: 订阅失败', e);
            message.error((e as Error)?.message || '订阅K线失败');
            scheduleErrorReconnect();
          },
        });

      subRef.current = sub as SubscriptionHandle;

      startReconnectTimer();
    },
    unsubscribe(_symbol: SymbolInfo, _period: Period): void {
      clearReconnectTimer();
      currentCallback = null;
      if (subRef.current) {
        try {
          subRef.current.unsubscribe();
        } catch (e) {
          console.error('KlineChartPro: unsubscribe error', e);
        }
        subRef.current = null;
      }
    },
  };
}

const KlineChartProInner: React.FC<KlineChartProProps> = ({
  exchange,
  symbol,
  accountId,
  dataReady = true,
  height = 500,
  visible = true,
  pricePrecision = 6,
  volumePrecision = 2,
  positions = [],
  liqPrice = null,
  openOrders = [],
}) => {
  const { styles } = useStyles();
  const { initialState } = useModel('@@initialState');
  const apolloClient = useApolloClient();
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<InstanceType<typeof KLineChartProClass> | null>(null);
  const chartDomBindingsRef = useRef<ChartDomBindings | null>(null);
  const pricePrecisionRef = useRef(pricePrecision);
  const volumePrecisionRef = useRef(volumePrecision);
  pricePrecisionRef.current = pricePrecision;
  volumePrecisionRef.current = volumePrecision;
  const apolloClientRef = useRef(apolloClient);
  apolloClientRef.current = apolloClient;
  const klineSubRef = useRef<SubscriptionHandle | null>(null);
  const reconnectRef = useRef<{ reconnect: () => void; cancel: () => void } | null>(null);
  const datafeedRef = useRef<Datafeed | null>(null);
  const snapshotRef = useRef<{ exchange: string; symbol: string; period: string } | null>(null);
  const hisOrdersMapRef = useRef<Map<string, HisOrder>>(new Map());
  const chartSymbolRef = useRef(symbol);
  chartSymbolRef.current = symbol;
  const exchangeRef = useRef(exchange);
  const symbolRef = useRef(symbol);
  exchangeRef.current = exchange;
  symbolRef.current = symbol;

  const reloadAlerts = useCallback(async () => {
    const chart = chartRef.current as unknown as ChartPro | null;
    if (!chart || !exchange || !symbol) return;
    try {
      const list = await api.listAlerts(exchange, symbol);
      const alerts = list
        .map(dtoToChartAlert)
        .filter((v): v is AlertItem => Boolean(v));
      console.log('set alerts', alerts);
      chart.setAlerts(alerts);
    } catch (e: unknown) {
      message.warning((e as Error)?.message || '加载价格预警失败');
    }
  }, [exchange, symbol]);

  const reloadAlertsRef = useRef(reloadAlerts);
  reloadAlertsRef.current = reloadAlerts;

  const mergeHisOrdersBatch = useCallback((orders: Order[]) => {
    const sym = chartSymbolRef.current;
    const m = hisOrdersMapRef.current;
    for (const o of orders) {
      if (o.symbol !== sym) continue;
      const h = orderToHisOrder(o);
      if (!h) continue;
      m.set(hisOrderDedupeKey(o), h);
    }
    console.log('mergeHisOrdersBatch', Array.from(m.values()));
    (chartRef.current as unknown as ChartPro | null)?.setHisOrders(Array.from(m.values()));
  }, []);

  const hisOrdersRef = useRef<HisOrdersBridge>({
    accountId: undefined,
    merge: () => { },
  });
  hisOrdersRef.current.accountId = accountId;
  hisOrdersRef.current.merge = mergeHisOrdersBatch;

  const disposeChartResources = useCallback(() => {
    const bindings = chartDomBindingsRef.current;
    if (bindings) {
      bindings.el.removeEventListener('wheel', bindings.wheelHandler);
      bindings.ro.disconnect();
      chartDomBindingsRef.current = null;
    }
    reconnectRef.current?.cancel();
    reconnectRef.current = null;
    try {
      klineSubRef.current?.unsubscribe();
    } catch (_) {
      /* ignore */
    }
    klineSubRef.current = null;
    if (datafeedRef.current && snapshotRef.current) {
      try {
        datafeedRef.current.unsubscribe(
          { ticker: snapshotRef.current.symbol, shortName: snapshotRef.current.symbol },
          intervalToProPeriod(snapshotRef.current.period),
        );
      } catch (_) {
        /* ignore */
      }
      datafeedRef.current = null;
      snapshotRef.current = null;
    }
    const ch = chartRef.current;
    if (ch) {
      try {
        ch.dispose();
      } catch (_) {
        /* ignore */
      }
      chartRef.current = null;
    }
    if (containerRef.current) {
      while (containerRef.current.firstChild) {
        containerRef.current.removeChild(containerRef.current.firstChild);
      }
    }
  }, []);

  useEffect(() => {
    return () => {
      disposeChartResources();
    };
  }, [disposeChartResources]);

  const klineTheme: KlineTheme = useMemo(
    () => (initialState?.settings?.navTheme === 'realDark' ? 'dark' : 'light'),
    [initialState?.settings?.navTheme],
  );

  /** 切换 exchange/symbol 时复用实例并 setSymbol；dataReady 为 false 时销毁图表（与外层遮罩配合）。不依赖 klineTheme，主题由专用 effect 同步。 */
  useEffect(() => {
    if (!symbol) {
      disposeChartResources();
      return;
    }
    if (!containerRef.current) {
      return;
    }
    if (!dataReady) {
      disposeChartResources();
      return;
    }

    if (!chartRef.current) {
      const initialPeriod = '1m';
      snapshotRef.current = { exchange, symbol, period: initialPeriod };
      const datafeed = createCustomDatafeed(
        snapshotRef,
        apolloClientRef,
        klineSubRef,
        reconnectRef,
        hisOrdersRef,
      );
      datafeedRef.current = datafeed;

      const el = containerRef.current;
      const symbolInfo: SymbolInfo = {
        exchange,
        ticker: symbol,
        name: symbol,
        shortName: symbol,
        pricePrecision: pricePrecisionRef.current,
        volumePrecision: volumePrecisionRef.current,
      };

      const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai';

      const chart = new KLineChartProClass({
        container: el,
        locale: 'zh-CN',
        theme: klineTheme,
        symbol: symbolInfo,
        timezone,
        period: intervalToProPeriod(initialPeriod),
        periods: DEFAULT_PERIODS,
        mainIndicators: ['BOLL'],
        subIndicators: ['VOL', 'MACD'],
        drawingBarVisible: false,
        watermark: '',
        datafeed,
        onAddAlert: async (alert: AlertItemInput) => {
          try {
            const created = await api.addAlert(
              inputToApiAlertInput(exchangeRef.current, symbolRef.current, alert),
            );
            if (!created) return false;
            await reloadAlertsRef.current();
            return true;
          } catch (e: unknown) {
            message.error(getAlertErrorMessage(e, '添加价格预警失败'));
            return false;
          }
        },
        onRemoveAlert: async (alert: AlertItem) => {
          if (!alert.id) return false;
          try {
            const ok = await api.removeAlert(alert.id);
            if (!ok) return false;
            await reloadAlertsRef.current();
            return true;
          } catch (e: unknown) {
            message.error(getAlertErrorMessage(e, '删除价格预警失败'));
            return false;
          }
        },
      });

      chartRef.current = chart;
      chart.setStyles(buildKlineCoreStyles(klineTheme));

      const handleWheelPreventPageScroll = (event: WheelEvent) => {
        event.preventDefault();
      };
      el.addEventListener('wheel', handleWheelPreventPageScroll, { passive: false });

      let rafId: number | null = null;
      const ro = new ResizeObserver(() => {
        if (rafId !== null) cancelAnimationFrame(rafId);
        rafId = requestAnimationFrame(() => {
          rafId = null;
          if (el.clientWidth <= 0 || el.clientHeight <= 0) return;
          try {
            chart.resize();
          } catch (_) {
            /* ignore */
          }
        });
      });
      ro.observe(el);

      chartDomBindingsRef.current = { el, wheelHandler: handleWheelPreventPageScroll, ro };
      return;
    }

    if (snapshotRef.current?.exchange === exchange && snapshotRef.current?.symbol === symbol) {
      return;
    }

    const chart = chartRef.current;
    const periodObj = chart.getPeriod() as Period | null | undefined;
    const periodStr = periodObj?.text ?? snapshotRef.current?.period ?? '1m';
    snapshotRef.current = { exchange, symbol, period: periodStr };

    const cur = chart.getSymbol() as SymbolInfo | null | undefined;
    chart.setSymbol({
      ...(cur ?? {}),
      exchange,
      ticker: symbol,
      name: symbol,
      shortName: symbol,
      pricePrecision: pricePrecisionRef.current,
      volumePrecision: volumePrecisionRef.current,
    });
  }, [exchange, symbol, dataReady, disposeChartResources]);

  /** 必须在 chartRef 就绪后拉取；切换 exchange/symbol 时图表可能不重建，也需重载预警 */
  useEffect(() => {
    if (!chartRef.current || !exchange || !symbol) return;
    void reloadAlerts();
  }, [exchange, symbol, reloadAlerts]);

  useEffect(() => {
    hisOrdersMapRef.current.clear();
    (chartRef.current as unknown as ChartPro | null)?.setHisOrders([]);
  }, [symbol, accountId]);

  useEffect(() => {
    if (!accountId?.trim() || !symbol) return;
    const client = apolloClient;
    const sub = client
      .subscribe({
        query: SUB_STREAM,
        variables: {
          input: { type: 'account' as const, account: accountId, exchange },
        },
      })
      .subscribe({
        next: (payload: { data?: { Stream?: StreamEvent } }) => {
          const ord = payload.data?.Stream?.order;
          if (!ord || ord.symbol !== chartSymbolRef.current) return;
          if (!isTerminalOrderStatus(ord.status)) return;
          mergeHisOrdersBatch([ord]);
        },
        error: (e: unknown) => {
          console.warn('KlineChartPro: 账户订单流订阅错误', e);
        },
      });
    return () => {
      try {
        sub.unsubscribe();
      } catch (_) {
        /* ignore */
      }
    };
  }, [accountId, symbol, exchange, apolloClient, mergeHisOrdersBatch]);

  /** 精度随 market 信息异步到达；与主 effect 的 setSymbol 互补（仅 ticker 未变时避免重复 reset） */
  useEffect(() => {
    const chart = chartRef.current;
    if (!chart || !symbol) return;
    const cur = chart.getSymbol();
    if (
      cur.pricePrecision === pricePrecision &&
      cur.volumePrecision === volumePrecision &&
      cur.ticker === symbol &&
      (cur.exchange ?? '') === (exchange ?? '')
    ) {
      return;
    }
    chart.setSymbol({
      ...cur,
      pricePrecision,
      volumePrecision,
    });
  }, [exchange, symbol, pricePrecision, volumePrecision]);

  /** 明暗主题：setTheme + 样式一次完成（原先拆成两个 effect 会重复 setStyles） */
  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) return;
    chart.setTheme(klineTheme);
    chart.setStyles(buildKlineCoreStyles(klineTheme));
  }, [klineTheme]);

  useEffect(() => {
    let proPositions = positions.map((p) => ({
      side: p.side,
      avgPrice: Number(p.entryPrice),
      size: Number(p.amount),
    }));
    (chartRef.current as ChartPro | null)?.setPositions(proPositions);
  }, [positions]);

  useEffect(() => {
    (chartRef.current as ChartPro | null)?.setLiqPrice(liqPrice);
  }, [liqPrice]);

  useEffect(() => {
    let proOpenOrders = openOrders.map((o) => ({
      id: o.clientOrderId,
      side: o.side as 'long' | 'short',
      isBuy: o.isBuy,
      price: Number(o.price),
      size: Number(o.originalQty),
      orderType: o.orderType as 'limit' | 'market',
    }));
    (chartRef.current as ChartPro | null)?.setOpenOrders(proOpenOrders);
  }, [openOrders]);

  // Tabs 切换回可见状态时补一次 resize，修复隐藏态尺寸为 0 导致副图高度丢失
  useEffect(() => {
    if (!visible) return;
    const chart = chartRef.current;
    const el = containerRef.current;
    if (!chart || !el) return;

    let rafId: number | null = null;
    const timerId = window.setTimeout(() => {
      rafId = requestAnimationFrame(() => {
        rafId = null;
        if (el.clientWidth <= 0 || el.clientHeight <= 0) return;
        try {
          chart.resize();
        } catch (_) {
          // ignore
        }
      });
    }, 0);

    return () => {
      window.clearTimeout(timerId);
      if (rafId !== null) cancelAnimationFrame(rafId);
    };
  }, [visible, height]);

  const wrapperStyle = useMemo(
    () => ({ position: 'relative' as const, width: '100%' }),
    [],
  );
  const containerStyle = useMemo(
    () => ({
      width: '100%',
      height: height ? `${height}px` : '400px',
    }),
    [height],
  );

  return (
    <div className={styles.klineChartProWrapper} style={wrapperStyle} data-kline-theme={klineTheme}>
      <div ref={containerRef} style={containerStyle} />
    </div>
  );
};

export const KlineChartPro = React.memo(KlineChartProInner);
