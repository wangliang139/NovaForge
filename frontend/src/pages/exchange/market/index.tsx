import AssetsTable from '@/components/Market/AssetsTable';
import { KlineChartPro } from '@/components/Market/KlineChartPro';
import LedgersTable from '@/components/Market/LedgersTable';
import { Orderbook } from '@/components/Market/Orderbook';
import OrdersTable from '@/components/Market/OrdersTable';
import PositionsTable from '@/components/Market/PositionsTable';
import { Exchange, MarketType } from '@/global.types';
import { useSubscriptionWithReconnect } from '@/hooks/useReconnectSubscription';
import RecentTradesTable from '@/pages/exchange/components/RecentTradesTable';
import type { PlaceOrderParams } from '@/pages/exchange/market/types';
import { api } from '@/services/gateway';
import {
  AccountInfo,
  Balance,
  cancelOrder,
  estimateOrder,
  EstimateOrderResult,
  getBalance,
  getLedgers,
  getLeverage,
  getOrders,
  getPositions,
  LedgersConnection,
  Order,
  OrdersConnection,
  OrderStatus,
  OrderType,
  placeOrder,
  Position,
  PositionSide,
  queryAccountInfo,
  setLeverage,
} from '@/services/gateway/account';
import {
  Depth,
  DepthLevel,
  FundingRate,
  getOrderBook,
  IndexComponent,
  IndexPrice,
  LeverageBracket,
  MarketInfo,
  MarkPrice,
  OpenInterest,
  StreamEvent,
  Ticker,
  Trade,
} from '@/services/gateway/market';
import { SUB_STREAM } from '@/services/gateway/subscription';
import utils from '@/utils';
import { LoadingOutlined } from '@ant-design/icons';
import { PageContainer, ProDescriptions } from '@ant-design/pro-components';
import { useApolloClient } from '@apollo/client';
import { useSearchParams } from '@umijs/max';
import {
  Alert,
  Card,
  Empty,
  message,
  Modal,
  Segmented,
  Space,
  Table,
  Tabs,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import PlaceOrderForm from './components/PlaceOrderForm';
import SymbolTicker from './components/SymbolTicker';
import useStyles from './style.style';

const terminalStreamOrderStatuses = new Set<string>([
  OrderStatus.Done,
  OrderStatus.Canceled,
  OrderStatus.Rejected,
  OrderStatus.Expired,
]);

function mergePositionsByUpdatedTs(prev: Position[], incoming: Position[]): Position[] {
  type PositionMetricsField =
    | 'markPrice'
    | 'liquidationPrice'
    | 'notional'
    | 'initialMargin'
    | 'maintMargin'
    | 'unRealizedProfit';
  const metricsFields: PositionMetricsField[] = [
    'markPrice',
    'liquidationPrice',
    'notional',
    'initialMargin',
    'maintMargin',
    'unRealizedProfit',
  ];
  const isZeroLike = (v: unknown) => {
    const n = Number(String(v ?? '').replace(/,/g, '').trim());
    return Number.isFinite(n) ? Math.abs(n) < 1e-12 : true;
  };
  const mergeOne = (base: Position, patch: Position): Position => {
    if (Number(patch.amount) <= 0) return patch;
    const next = { ...patch };
    for (const field of metricsFields) {
      if (isZeroLike(next[field]) && !isZeroLike(base[field])) {
        next[field] = base[field];
      }
    }
    return next;
  };

  const map = new Map<string, Position>();
  for (const p of prev) {
    map.set(`${p.symbol}\0${p.side}`, p);
  }
  for (const p of incoming) {
    const k = `${p.symbol}\0${p.side}`;
    const cur = map.get(k);
    if (!cur || p.updatedTs >= cur.updatedTs) {
      map.set(k, cur ? mergeOne(cur, p) : p);
    }
  }
  return [...map.values()].filter((p) => Number(p.amount) > 0);
}

type MarketRulesFull = {
  maxOrderNum?: number;
  minPrice?: string;
  maxPrice?: string;
  tickSize?: string;
  minQuantity?: string;
  maxQuantity?: string;
  lotSize?: string;
  minNotional?: string;
  maxNotional?: string;
};

type MarketOrderTypeFull = {
  orderType: string;
  rules?: MarketRulesFull;
};

type MarketFull = {
  exchange: string;
  symbol: string;
  status: string;
  baseAssetPrecision?: number;
  quoteAssetPrecision?: number;
  pricePrecision?: number;
  rules?: MarketRulesFull;
  supportOrderTypes?: MarketOrderTypeFull[];
};

const TOP_LAYOUT_GAP = 16;
const ORDERBOOK_MIN_WIDTH = 260;
const ORDER_FORM_MIN_WIDTH = 200;
const ORDERBOOK_RATIO = 0.25;
const ORDER_FORM_RATIO = 0.2;

const isEmptyRuleValue = (v: unknown) =>
  v === undefined || v === null || v === '' || v === 0 || v === '0';

const mergeRules = (
  base?: MarketRulesFull,
  override?: MarketRulesFull,
): MarketRulesFull | undefined => {
  if (!base && !override) return undefined;
  const merged: MarketRulesFull = {};
  const keys: (keyof MarketRulesFull)[] = [
    'maxOrderNum',
    'minPrice',
    'maxPrice',
    'tickSize',
    'minQuantity',
    'maxQuantity',
    'lotSize',
    'minNotional',
    'maxNotional',
  ];
  keys.forEach((k) => {
    const ov = override?.[k];
    const bv = base?.[k];
    const v = !isEmptyRuleValue(ov) ? ov : bv;
    if (isEmptyRuleValue(v)) return;
    (merged as any)[k] = v as any;
  });
  return merged;
};

const ruleColumns = [
  { title: '最大挂单数量', dataIndex: 'maxOrderNum' },
  { title: '最小价格', dataIndex: 'minPrice' },
  { title: '最大价格', dataIndex: 'maxPrice' },
  { title: '价格步长', dataIndex: 'tickSize' },
  { title: '最小数量', dataIndex: 'minQuantity' },
  { title: '最大数量', dataIndex: 'maxQuantity' },
  { title: '数量步长', dataIndex: 'lotSize' },
  { title: '最小订单价值', dataIndex: 'minNotional' },
  { title: '最大订单价值', dataIndex: 'maxNotional' },
];

const mergeDepthSide = (
  prev: DepthLevel[] = [],
  delta: DepthLevel[] = [],
  isBid: boolean,
  maxLevels = 10000,
): DepthLevel[] => {
  if (!delta || delta.length === 0) {
    return prev;
  }

  const levelMap = new Map<string, { priceRaw: string; size: string }>();

  for (const item of prev || []) {
    const price = item?.price ?? '';
    const sizeNum = Number(item?.size ?? '');
    if (sizeNum <= 0) continue;
    levelMap.set(price, { priceRaw: price, size: String(item.size) });
  }

  for (const item of delta || []) {
    const price = item?.price ?? '';
    const sizeNum = Number(item?.size ?? '');
    if (sizeNum <= 0) {
      levelMap.delete(price);
    } else {
      levelMap.set(price, { priceRaw: price, size: String(item.size) });
    }
  }

  const merged = Array.from(levelMap.values()).map((v) => ({ price: v.priceRaw, size: v.size }));
  merged.sort((a, b) => {
    const pa = Number(a.price);
    const pb = Number(b.price);
    return isBid ? pb - pa : pa - pb;
  });
  return merged.slice(0, maxLevels);
};

const DEFAULT_MARKET_SYMBOL = 'BTC/USDT:FUTURE';

function parseExchangeFromSearchParams(sp: URLSearchParams): Exchange {
  const raw = sp.get('exchange');
  if (!raw) return Exchange.Binance;
  const normalized = raw.toLowerCase();
  const exchangeValues = Object.values(Exchange);
  const matched = exchangeValues.find((v) => v.toLowerCase() === normalized);
  return matched ? (matched as Exchange) : Exchange.Binance;
}

function initialSymbolFromSearchParams(sp: URLSearchParams): string {
  const s = sp.get('symbol')?.trim();
  return s || DEFAULT_MARKET_SYMBOL;
}

function initialAccountIdFromSearchParams(sp: URLSearchParams): string | null {
  const id = sp.get('accountId')?.trim();
  return id || null;
}

const MarketPage: React.FC = () => {
  const { styles } = useStyles();
  const apolloClient = useApolloClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const [symbolName, setSymbolName] = useState<string>(() =>
    initialSymbolFromSearchParams(searchParams),
  );
  const [exchange, setExchange] = useState<Exchange>(() =>
    parseExchangeFromSearchParams(searchParams),
  );
  const [trades, setTrades] = useState<Trade[]>([]);
  const [ticker, setTicker] = useState<Ticker>();
  const [markPrice, setMarkPrice] = useState<MarkPrice>();
  const [fundingRate, setFundingRate] = useState<FundingRate | null>(null);
  const [fundingRates7d, setFundingRates7d] = useState<FundingRate[]>([]);
  const [fundingRates7dLoading, setFundingRates7dLoading] = useState(false);
  const [indexPrice, setIndexPrice] = useState<IndexPrice | null>(null);
  const [indexComponent, setIndexComponent] = useState<IndexComponent | null>(null);
  const [indexComponentLoading, setIndexComponentLoading] = useState(false);
  const [openInterest, setOpenInterest] = useState<OpenInterest | null>(null);
  const [depth, setDepth] = useState<Depth>();
  const [marketInfo, setMarketInfo] = useState<MarketInfo | null>(null);
  const [marketLoading, setMarketLoading] = useState(false);
  const [rightTab, setRightTab] = useState<'depth' | 'trade'>('depth');
  const [leftTab, setLeftTab] = useState<'chart' | 'info' | 'trading'>('chart');
  const [infoTab, setInfoTab] = useState<'coin' | 'params' | 'leverage' | 'funding' | 'index'>(
    'params',
  );
  const [leverageBracket, setLeverageBracket] = useState<LeverageBracket | null>(null);

  // 账户相关状态（首屏与 URL 一致，避免默认交易对/交易所先拉一轮再切 URL）
  const [selectedAccountId, setSelectedAccountId] = useState<string | null>(() =>
    initialAccountIdFromSearchParams(searchParams),
  );

  const [accountInfo, setAccountInfo] = useState<AccountInfo | null>(null);

  // 下单表单状态
  const [orderLoading, setOrderLoading] = useState(false);
  const [orderFormResetKey, setOrderFormResetKey] = useState(0);

  // 底部 Tabs 状态
  const [bottomTab, setBottomTab] = useState<string>('positions');
  const [activatedTabs, setActivatedTabs] = useState<Set<string>>(new Set());
  const [topRowWidth, setTopRowWidth] = useState(0);

  // 数据状态
  const [positions, setPositions] = useState<Position[]>([]);
  const [openOrders, setOpenOrders] = useState<Order[]>([]);
  const [historyOrders, setHistoryOrders] = useState<OrdersConnection | null>(null);
  const [balance, setBalance] = useState<Balance | null>(null);
  const [ledgers, setLedgers] = useState<LedgersConnection | null>(null);
  const [positionsLoading, setPositionsLoading] = useState(false);
  const [openOrdersLoading, setOpenOrdersLoading] = useState(false);
  const [historyOrdersLoading, setHistoryOrdersLoading] = useState(false);
  const [balanceLoading, setBalanceLoading] = useState(false);
  const [ledgersLoading, setLedgersLoading] = useState(false);

  // 快捷下单浮层状态
  const [quickOrderOpen, setQuickOrderOpen] = useState(true);

  const skip = !symbolName;
  const symbolParsed = useMemo(() => utils.market.parseSymbol(symbolName), [symbolName]);
  const isPerp = symbolParsed.type === MarketType.Future;

  /** 信息 Tab 内按需请求：减少首屏 / 停留在图表时的接口消耗 */
  const infoPanelActive = leftTab === 'info';
  const fundingHistoryNeeded = infoPanelActive && infoTab === 'funding' && !skip && isPerp;
  const leverageInfoNeeded = infoPanelActive && infoTab === 'leverage' && !skip && isPerp;
  const indexComponentNeeded = infoPanelActive && infoTab === 'index' && !skip && isPerp;

  const [symbolLeverage, setSymbolLeverage] = useState<number | null>(null);
  const [symbolLeverageLoading, setSymbolLeverageLoading] = useState(false);

  const filteredAssets = useMemo(() => balance?.assets || [], [balance, isPerp]);

  const tradeBufferRef = useRef<Trade[]>([]);
  const tradeFlushTimerRef = useRef<number | null>(null);
  const marketReqIdRef = useRef<number>(0);
  /** SymbolTicker 确认时已拉取的 Market，供 loadMarketInfo 消费以避免重复 queryMarket */
  const marketInfoSeedRef = useRef<{ exchange: string; symbol: string; info: MarketInfo } | null>(null);
  const perpMetricsReqIdRef = useRef<number>(0);
  const perpMetricsLastErrAtRef = useRef<number>(0);
  const fundingHistoryReqIdRef = useRef<number>(0);
  const fundingHistoryLastErrAtRef = useRef<number>(0);
  const leverageBracketReqIdRef = useRef<number>(0);
  const leverageBracketLastErrAtRef = useRef<number>(0);
  const leverageBracketLoadedKeyRef = useRef<string>('');
  const indexComponentReqIdRef = useRef<number>(0);
  const indexComponentLastErrAtRef = useRef<number>(0);
  const tickerLastAtRef = useRef(0);
  const markPriceLastAtRef = useRef(0);
  const depthLastAtRef = useRef(0);
  const depthLastSeqIdRef = useRef<number | null>(null);
  const depthSnapshotLoadingRef = useRef(false);
  const depthSnapshotLastErrAtRef = useRef(0);
  const depthSnapshotLastAtRef = useRef(0);

  /** 最近一次全量拉取中的最大 updatedTs，用于与账户流 eventTs/行级时间戳去重 */
  const fullPollPositionsMaxTsRef = useRef(0);
  const fullPollOpenOrdersMaxTsRef = useRef(0);
  const fullPollBalanceMaxTsRef = useRef(0);
  const positionsFastRefreshLastAtRef = useRef(0);
  const symbolLeverageStreamTsRef = useRef(0);
  const topLayoutRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const el = topLayoutRef.current;
    if (!el || typeof ResizeObserver === 'undefined') return;

    const updateWidth = (width: number) => {
      setTopRowWidth((prev) => (prev === width ? prev : width));
    };

    updateWidth(el.getBoundingClientRect().width);

    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) return;
      updateWidth(entry.contentRect.width);
    });

    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const topLayout = useMemo(() => {
    const width = Math.max(topRowWidth, 0);
    const hasOrderForm = Boolean(selectedAccountId);

    if (!hasOrderForm) {
      const availableWidth = Math.max(width - TOP_LAYOUT_GAP, 0);
      const showOrderbook = availableWidth * ORDERBOOK_RATIO >= ORDERBOOK_MIN_WIDTH;
      return {
        showOrderbook,
        showOrderForm: false,
        chartFlex: showOrderbook ? 3 : 1,
        orderbookFlex: showOrderbook ? 1 : 0,
        orderFormFlex: 0,
      };
    }

    const threePanelWidth = Math.max(width - TOP_LAYOUT_GAP * 2, 0);
    const canShowAll =
      threePanelWidth * ORDERBOOK_RATIO >= ORDERBOOK_MIN_WIDTH &&
      threePanelWidth * ORDER_FORM_RATIO >= ORDER_FORM_MIN_WIDTH;
    if (canShowAll) {
      return {
        showOrderbook: true,
        showOrderForm: true,
        chartFlex: 2.2,
        orderbookFlex: 1,
        orderFormFlex: 0.8,
      };
    }

    const twoPanelWidth = Math.max(width - TOP_LAYOUT_GAP, 0);
    const canShowOrderFormOnly = twoPanelWidth * ORDER_FORM_RATIO >= ORDER_FORM_MIN_WIDTH;
    if (canShowOrderFormOnly) {
      return {
        showOrderbook: false,
        showOrderForm: true,
        chartFlex: 4,
        orderbookFlex: 0,
        orderFormFlex: 1,
      };
    }

    return {
      showOrderbook: false,
      showOrderForm: false,
      chartFlex: 1,
      orderbookFlex: 0,
      orderFormFlex: 0,
    };
  }, [selectedAccountId, topRowWidth]);

  const handleTickerSelectionConfirm = useCallback(
    ({
      exchange: nextExchange,
      symbol: nextSymbol,
      accountId: nextAccountId,
      marketInfo: nextMarketInfo,
    }: {
      exchange: Exchange;
      symbol: string;
      accountId: string | null;
      marketInfo?: MarketInfo | null;
    }) => {
      if (nextMarketInfo) {
        marketInfoSeedRef.current = {
          exchange: nextExchange,
          symbol: nextSymbol,
          info: nextMarketInfo,
        };
      } else {
        marketInfoSeedRef.current = null;
      }
      setExchange(nextExchange);
      setSymbolName(nextSymbol);
      setSelectedAccountId(nextAccountId);

      const next = new URLSearchParams(window.location.search);
      next.set('exchange', nextExchange);
      if (nextSymbol) {
        next.set('symbol', nextSymbol);
      } else {
        next.delete('symbol');
      }
      if (nextAccountId) {
        next.set('accountId', nextAccountId);
      } else {
        next.delete('accountId');
      }
      setSearchParams(next, { replace: true });
    },
    [setSearchParams],
  );

  const fundingRateText = useMemo(() => {
    if (!isPerp) return '--';
    const raw = String(fundingRate?.fundingRate ?? '')
      .replace(/,/g, '')
      .trim();
    if (!raw) return '--';
    const n = Number(raw);
    if (!Number.isFinite(n)) return raw;
    const pct = n * 100;
    const sign = pct > 0 ? '+' : '';
    const txt = utils.math.formatByPrecision(pct, 6).replace(/\.?0+$/, '');
    return `${sign}${txt}%`;
  }, [fundingRate?.fundingRate, isPerp]);

  useSubscriptionWithReconnect<{ Stream: StreamEvent }>(SUB_STREAM, {
    variables: {
      input: {
        type: 'ticker',
        exchange,
        symbol: symbolName,
      },
    },
    skip,
    ignoreResults: true,
    onData: ({ data }) => {
      const event = data.data?.Stream;
      if (!event) return;
      const now = Date.now();
      switch (event.type) {
        case 'ticker': {
          if (now - tickerLastAtRef.current < 120) break;
          tickerLastAtRef.current = now;
          if (event.ticker) setTicker(event.ticker);
          break;
        }
        case 'mark_price': {
          if (now - markPriceLastAtRef.current < 120) break;
          markPriceLastAtRef.current = now;
          if (event.markPrice) setMarkPrice(event.markPrice);
          break;
        }
        case 'trade': {
          if (!event.trade) break;
          tradeBufferRef.current.unshift(event.trade);
          if (tradeFlushTimerRef.current != null) break;
          tradeFlushTimerRef.current = window.setTimeout(() => {
            tradeFlushTimerRef.current = null;
            const pending = tradeBufferRef.current;
            tradeBufferRef.current = [];
            if (pending.length === 0) return;
            setTrades((prev) => {
              const merged = [...pending, ...prev];
              const seen = new Set<string>();
              const next: Trade[] = [];
              for (const item of merged) {
                const key = `${item.tradeId}-${item.ts}`;
                if (seen.has(key)) continue;
                seen.add(key);
                next.push(item);
                if (next.length >= 100) break;
              }
              return next;
            });
          }, 120);
          break;
        }
        case 'depth': {
          depthLastAtRef.current = now;
          if (!event.depth) break;
          const d = event.depth;

          d.bids.forEach((bid) => {
            bid.ts = d.ts;
            bid.seqId = d.seqId;
          });
          d.asks.forEach((ask) => {
            ask.ts = d.ts;
            ask.seqId = d.seqId;
          });

          // 先按增量正常更新本地订单簿（不阻塞）
          setDepth((prev) => ({
            bids: mergeDepthSide(prev?.bids ?? [], d.bids, true),
            asks: mergeDepthSide(prev?.asks ?? [], d.asks, false),
            ts: d.ts,
            seqId: d.seqId,
            prevSeqId: d.prevSeqId,
          }));

          // 根据 seqId/prevSeqId 判断是否需要重新拉取快照：
          // 1）当前是首个深度事件；2）prevSeqId 与上一个事件的 seqId 不一致
          const lastSeqId = depthLastSeqIdRef.current;
          const isFirstEvent = lastSeqId == null;
          const hasGap =
            !isFirstEvent &&
            typeof d.prevSeqId === 'number' &&
            typeof lastSeqId === 'number' &&
            d.prevSeqId !== lastSeqId;

          const shouldReloadByTime = now - depthSnapshotLastAtRef.current >= 3000;

          if ((isFirstEvent || hasGap) && !depthSnapshotLoadingRef.current && shouldReloadByTime) {
            depthSnapshotLastAtRef.current = now;
            depthSnapshotLoadingRef.current = true;
            void (async () => {
              console.log('reload depth snapshot');
              try {
                const snapshot = await getOrderBook(exchange, symbolName, 1000);
                if (!snapshot) {
                  console.error('get order book snapshot failed');
                  return;
                }
                snapshot.bids.forEach((bid) => {
                  bid.ts = snapshot.ts;
                  bid.seqId = snapshot.seqId;
                });
                snapshot.asks.forEach((ask) => {
                  ask.ts = snapshot.ts;
                  ask.seqId = snapshot.seqId;
                });
                setDepth((prev) => {
                  if (!prev) {
                    return snapshot;
                  }
                  // 清理过期数据
                  let bids = prev.bids.filter(
                    (bid) =>
                      (snapshot.ts > 0 && bid.ts && bid.ts > snapshot.ts) ||
                      (bid.seqId && bid.seqId > snapshot.seqId),
                  );
                  let asks = prev.asks.filter(
                    (ask) =>
                      (snapshot.ts > 0 && ask.ts && ask.ts > snapshot.ts) ||
                      (ask.seqId && ask.seqId > snapshot.seqId),
                  );
                  // 合并数据
                  const mergedBids = mergeDepthSide(snapshot.bids ?? [], bids ?? [], true);
                  const mergedAsks = mergeDepthSide(snapshot.asks ?? [], asks ?? [], false);
                  return {
                    bids: mergedBids,
                    asks: mergedAsks,
                    ts: Math.max(Number(snapshot.ts || 0), Number(prev.ts || 0)),
                    seqId: prev.seqId ?? snapshot.seqId,
                    prevSeqId: prev.prevSeqId ?? snapshot.prevSeqId,
                  } as Depth;
                });
              } catch (e: any) {
                const msg = e?.message || '加载订单簿快照失败';
                const nowTs = Date.now();
                if (nowTs - depthSnapshotLastErrAtRef.current > 8000) {
                  depthSnapshotLastErrAtRef.current = nowTs;
                  message.error(msg);
                }
              } finally {
                depthSnapshotLoadingRef.current = false;
              }
            })();
          }

          // 记录当前事件的 seqId，供下一次 prevSeqId 校验使用
          depthLastSeqIdRef.current = d.seqId;
          break;
        }
      }
    },
    onError: (e) => {
      const msg = (e as any)?.message || String(e);
      if (msg) message.error(`行情订阅失败：${msg}`);
    },
  });
  useSubscriptionWithReconnect<{ Stream: StreamEvent }>(SUB_STREAM, {
    variables: { input: { type: 'trade', exchange, symbol: symbolName } },
    skip,
    ignoreResults: true,
    onData: ({ data }) => {
      const event = data.data?.Stream;
      if (!event?.trade) return;
      const now = Date.now();
      tradeBufferRef.current.unshift(event.trade);
      if (tradeFlushTimerRef.current != null) return;
      tradeFlushTimerRef.current = window.setTimeout(() => {
        tradeFlushTimerRef.current = null;
        const pending = tradeBufferRef.current;
        tradeBufferRef.current = [];
        if (pending.length === 0) return;
        setTrades((prev) => {
          const merged = [...pending, ...prev];
          const seen = new Set<string>();
          const next: Trade[] = [];
          for (const item of merged) {
            const key = `${item.tradeId}-${item.ts}`;
            if (seen.has(key)) continue;
            seen.add(key);
            next.push(item);
            if (next.length >= 100) break;
          }
          return next;
        });
      }, 120);
    },
    onError: (e) => {
      const msg = (e as any)?.message || String(e);
      if (msg) message.error(`行情订阅失败：${msg}`);
    },
  });
  useSubscriptionWithReconnect<{ Stream: StreamEvent }>(SUB_STREAM, {
    variables: { input: { type: 'depth', exchange, symbol: symbolName } },
    skip,
    ignoreResults: true,
    onData: ({ data }) => {
      const event = data.data?.Stream;
      if (!event?.depth) return;
      const now = Date.now();
      depthLastAtRef.current = now;
      const d = event.depth;
      d.bids.forEach((bid) => {
        bid.ts = d.ts;
        bid.seqId = d.seqId;
      });
      d.asks.forEach((ask) => {
        ask.ts = d.ts;
        ask.seqId = d.seqId;
      });
      setDepth((prev) => ({
        bids: mergeDepthSide(prev?.bids ?? [], d.bids, true),
        asks: mergeDepthSide(prev?.asks ?? [], d.asks, false),
        ts: d.ts,
        seqId: d.seqId,
        prevSeqId: d.prevSeqId,
      }));
      const lastSeqId = depthLastSeqIdRef.current;
      const isFirstEvent = lastSeqId == null;
      const hasGap =
        !isFirstEvent &&
        typeof d.prevSeqId === 'number' &&
        typeof lastSeqId === 'number' &&
        d.prevSeqId !== lastSeqId;
      const shouldReloadByTime = now - depthSnapshotLastAtRef.current >= 3000;
      if ((isFirstEvent || hasGap) && !depthSnapshotLoadingRef.current && shouldReloadByTime) {
        depthSnapshotLastAtRef.current = now;
        depthSnapshotLoadingRef.current = true;
        void (async () => {
          try {
            const snapshot = await getOrderBook(exchange, symbolName, 1000);
            if (!snapshot) return;
            snapshot.bids.forEach((bid) => {
              bid.ts = snapshot.ts;
              bid.seqId = snapshot.seqId;
            });
            snapshot.asks.forEach((ask) => {
              ask.ts = snapshot.ts;
              ask.seqId = snapshot.seqId;
            });
            setDepth((prev) => {
              if (!prev) return snapshot;
              const mergedBids = mergeDepthSide(snapshot.bids ?? [], prev.bids ?? [], true);
              const mergedAsks = mergeDepthSide(snapshot.asks ?? [], prev.asks ?? [], false);
              return {
                bids: mergedBids,
                asks: mergedAsks,
                ts: Math.max(Number(snapshot.ts || 0), Number(prev.ts || 0)),
                seqId: prev.seqId ?? snapshot.seqId,
                prevSeqId: prev.prevSeqId ?? snapshot.prevSeqId,
              } as Depth;
            });
          } catch (e: any) {
            const nowTs = Date.now();
            if (nowTs - depthSnapshotLastErrAtRef.current > 8000) {
              depthSnapshotLastErrAtRef.current = nowTs;
              message.error(e?.message || '加载订单簿快照失败');
            }
          } finally {
            depthSnapshotLoadingRef.current = false;
          }
        })();
      }
      depthLastSeqIdRef.current = d.seqId;
    },
    onReconnectSuccess: () => {
      message.success('订单簿重连成功');
      setDepth(undefined);
    },
    onError: (e) => {
      const msg = (e as any)?.message || String(e);
      if (msg) message.error(`行情订阅失败：${msg}`);
    },
  });
  useSubscriptionWithReconnect<{ Stream: StreamEvent }>(SUB_STREAM, {
    variables: { input: { type: 'mark_price', exchange, symbol: symbolName } },
    skip: skip || !isPerp,
    ignoreResults: true,
    onData: ({ data }) => {
      const event = data.data?.Stream;
      if (!event?.markPrice) return;
      const now = Date.now();
      if (now - markPriceLastAtRef.current < 120) return;
      markPriceLastAtRef.current = now;
      setMarkPrice(event.markPrice);
    },
    onError: (e) => {
      const msg = (e as any)?.message || String(e);
      if (msg) message.error(`行情订阅失败：${msg}`);
    },
  });

  /**
   * 账户私有流：在下方 useEffect 已通过 HTTP 完成仓位/余额/委托等首屏全量加载后，此处仅做增量合并与对齐。
   * （当前委托另在选中账户时立即拉一次全量，不依赖 Tab。）
   */
  const fastRefreshPositions = useCallback(async () => {
    if (!selectedAccountId || !isPerp) return;
    const now = Date.now();
    // 避免成交/部分成交高频回报导致的重复全量请求
    if (now - positionsFastRefreshLastAtRef.current < 1200) return;
    positionsFastRefreshLastAtRef.current = now;
    try {
      const data = await getPositions(selectedAccountId);
      const rows = (data || []).filter((p) => Number(p.amount) > 0);
      const rowMax = rows.reduce((m, p) => Math.max(m, Number(p.updatedTs) || 0), 0);
      fullPollPositionsMaxTsRef.current = Math.max(fullPollPositionsMaxTsRef.current, rowMax);
      setPositions(rows);
    } catch (err) {
      console.error('Failed to fast refresh positions:', err);
    }
  }, [isPerp, selectedAccountId]);

  useSubscriptionWithReconnect<{ Stream: StreamEvent }>(SUB_STREAM, {
    variables: {
      input: {
        type: 'account',
        account: selectedAccountId!,
        exchange,
      },
    },
    skip: !selectedAccountId,
    ignoreResults: true,
    onData: ({ data }) => {
      const event = data.data?.Stream;
      if (!event || event.type !== 'account') return;
      const ets = Number(event.eventTs) || 0;

      if (event.balanceSnapshot && ets >= fullPollBalanceMaxTsRef.current) {
        const aid = selectedAccountId;
        if (aid) {
          void getBalance(aid).then((bal) => {
            if (!bal) return;
            const assets = bal.assets || [];
            fullPollBalanceMaxTsRef.current = assets.reduce(
              (m, a) => Math.max(m, Number(a.updatedTs) || 0),
              0,
            );
            setBalance(bal);
          });
        }
      }

      if (event.positionSnapshot != null) {
        if (ets < fullPollPositionsMaxTsRef.current) return;
        const next = (event.positionSnapshot.positions || []).filter((p) => Number(p.amount) > 0);
        const rowMax = next.reduce((m, p) => Math.max(m, Number(p.updatedTs) || 0), 0);
        fullPollPositionsMaxTsRef.current = Math.max(fullPollPositionsMaxTsRef.current, ets, rowMax);
        setPositions(next);
      }

      if (event.positionsUpdate?.positions?.length) {
        if (ets < fullPollPositionsMaxTsRef.current) return;
        const inc = event.positionsUpdate.positions!;
        const rowMax = inc.reduce((m, p) => Math.max(m, Number(p.updatedTs) || 0), 0);
        fullPollPositionsMaxTsRef.current = Math.max(fullPollPositionsMaxTsRef.current, ets, rowMax);
        setPositions((prev) => mergePositionsByUpdatedTs(prev, inc));
        if (String(event.positionsUpdate.reason || '').toUpperCase() === 'FILL') {
          void fastRefreshPositions();
        }
      }

      if (event.order) {
        const o = event.order;
        setOpenOrders((prev) => {
          if (terminalStreamOrderStatuses.has(o.status) || !o.isWorking) {
            return prev.filter((x) => x.orderId !== o.orderId);
          }
          const idx = prev.findIndex((x) => x.orderId === o.orderId);
          if (idx < 0) {
            fullPollOpenOrdersMaxTsRef.current = Math.max(
              fullPollOpenOrdersMaxTsRef.current,
              ets,
              Number(o.updatedTs) || 0,
            );
            return [...prev, o];
          }
          if (o.updatedTs < prev[idx].updatedTs) {
            return prev;
          }
          fullPollOpenOrdersMaxTsRef.current = Math.max(
            fullPollOpenOrdersMaxTsRef.current,
            ets,
            Number(o.updatedTs) || 0,
          );
          const next = [...prev];
          next[idx] = o;
          return next;
        });
      }

      if (event.symbolLeverage && isPerp && event.symbolLeverage.symbol === symbolName) {
        const sl = event.symbolLeverage;
        if (sl.updatedTs < symbolLeverageStreamTsRef.current) return;
        symbolLeverageStreamTsRef.current = sl.updatedTs;
        setSymbolLeverage(sl.leverage > 0 ? sl.leverage : 10);
      }

    },
    onError: (e) => {
      const msg = (e as any)?.message || String(e);
      if (msg) message.error(`账户流订阅失败：${msg}`);
    },
  });

  const loadMarketInfo = useCallback(async () => {
    if (!symbolName) {
      setMarketInfo(null);
      setMarketLoading(false);
      return;
    }
    const seed = marketInfoSeedRef.current;
    if (seed && seed.exchange === exchange && seed.symbol === symbolName) {
      marketInfoSeedRef.current = null;
      ++marketReqIdRef.current;
      setMarketInfo(seed.info);
      setMarketLoading(false);
      return;
    }
    const reqId = ++marketReqIdRef.current;
    setMarketLoading(true);
    try {
      const resp = (await api.queryMarket({ exchange, symbol: symbolName })) as MarketInfo | null;
      if (reqId !== marketReqIdRef.current) return;
      setMarketInfo(resp || null);
    } catch (e: any) {
      if (reqId !== marketReqIdRef.current) return;
      setMarketInfo(null);
      message.error(e?.message || '加载 Market 信息失败');
    } finally {
      if (reqId === marketReqIdRef.current) {
        setMarketLoading(false);
      }
    }
  }, [exchange, symbolName]);

  useEffect(() => {
    setTrades([]);
    setTicker(undefined);
    setMarkPrice(undefined);
    setFundingRate(null);
    setFundingRates7d([]);
    setFundingRates7dLoading(false);
    setIndexPrice(null);
    setIndexComponent(null);
    setIndexComponentLoading(false);
    setOpenInterest(null);
    setDepth(undefined);
    depthLastSeqIdRef.current = null;
    depthSnapshotLoadingRef.current = false;
    depthSnapshotLastErrAtRef.current = 0;
    depthSnapshotLastAtRef.current = 0;
    setLeverageBracket(null);
    leverageBracketLoadedKeyRef.current = '';
    indexComponentReqIdRef.current += 1;
    fundingHistoryReqIdRef.current += 1;
    leverageBracketReqIdRef.current += 1;
    tradeBufferRef.current = [];
    if (tradeFlushTimerRef.current != null) {
      window.clearTimeout(tradeFlushTimerRef.current);
      tradeFlushTimerRef.current = null;
    }
  }, [exchange, symbolName]);

  // 组件卸载时清理 Trade flush timer
  useEffect(() => {
    return () => {
      if (tradeFlushTimerRef.current != null) {
        window.clearTimeout(tradeFlushTimerRef.current);
        tradeFlushTimerRef.current = null;
      }
    };
  }, []);

  // 资金费率 / 合约持仓量：轮询拉取（仅合约）
  useEffect(() => {
    setFundingRate(null);
    setOpenInterest(null);
    setIndexPrice(null);
    const reqId = ++perpMetricsReqIdRef.current;

    if (skip || !isPerp) return;

    let disposed = false;
    const fetchOnce = async () => {
      if (disposed) return;
      try {
        const [fr, oi, idx] = await Promise.all([
          api.queryFundingRate({ exchange, symbol: symbolName }),
          api.queryOpenInterest({ exchange, symbol: symbolName }),
          api.queryIndexPrice({ exchange, symbol: symbolName }),
        ]);
        if (disposed) return;
        if (reqId !== perpMetricsReqIdRef.current) return;
        setFundingRate(fr || null);
        setOpenInterest(oi || null);
        setIndexPrice((idx as IndexPrice) || null);
      } catch (e: any) {
        if (disposed) return;
        if (reqId !== perpMetricsReqIdRef.current) return;
        const now = Date.now();
        if (now - perpMetricsLastErrAtRef.current > 8000) {
          perpMetricsLastErrAtRef.current = now;
          message.error(e?.message || '加载资金费率/合约持仓量失败');
        }
      }
    };

    fetchOnce();
    const timer = window.setInterval(fetchOnce, 5000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [exchange, symbolName, skip, isPerp]);

  // 资金费率历史：最近 7 天（仅合约；进入「信息 → 资金费率」时再拉取）
  useEffect(() => {
    if (!fundingHistoryNeeded) return;

    const reqId = ++fundingHistoryReqIdRef.current;
    let disposed = false;
    setFundingRates7dLoading(true);

    const fetchOnce = async () => {
      try {
        const endTime = Date.now();
        const startTime = endTime - 7 * 24 * 60 * 60 * 1000;
        const list = (await api.queryFundingRates({
          exchange,
          symbol: symbolName,
          startTime,
          endTime,
          limit: 200,
        })) as FundingRate[];

        if (disposed) return;
        if (reqId !== fundingHistoryReqIdRef.current) return;
        const sorted = (list || []).slice().sort((a, b) => (b.ts ?? 0) - (a.ts ?? 0));
        setFundingRates7d(sorted);
      } catch (e: any) {
        if (disposed) return;
        if (reqId !== fundingHistoryReqIdRef.current) return;
        const now = Date.now();
        if (now - fundingHistoryLastErrAtRef.current > 8000) {
          fundingHistoryLastErrAtRef.current = now;
          message.error(e?.message || '加载资金费率历史失败');
        }
      } finally {
        if (!disposed && reqId === fundingHistoryReqIdRef.current) {
          setFundingRates7dLoading(false);
        }
      }
    };

    fetchOnce();
    return () => {
      disposed = true;
    };
  }, [exchange, symbolName, fundingHistoryNeeded]);

  // 杠杆档位：仅合约（进入「信息 → 杠杆与保证金」且标记价格就绪后再拉取）
  useEffect(() => {
    if (!leverageInfoNeeded) return;

    const key = `${exchange}-${symbolName}-${selectedAccountId ?? ''}`;

    const isZeroLike = (v: string) => {
      const raw = String(v ?? '')
        .replace(/,/g, '')
        .trim();
      if (!raw) return true;
      const n = Number(raw);
      if (!Number.isFinite(n)) return false;
      return n === 0;
    };

    const mp1 = String(markPrice?.markPrice ?? '').trim();
    const mp = (mp1 && !isZeroLike(mp1) ? mp1 : '').replace(/,/g, '').trim();
    if (!mp) {
      if (leverageBracketLoadedKeyRef.current !== key) {
        setLeverageBracket(null);
      }
      return;
    }

    // 避免 markPrice 高频变化导致重复请求：每个 symbol/exchange/账户 只拉一次（需要时可手动刷新）
    if (leverageBracketLoadedKeyRef.current === key) return;
    leverageBracketLoadedKeyRef.current = key;

    setLeverageBracket(null);
    const reqId = ++leverageBracketReqIdRef.current;
    let disposed = false;

    api
      .queryLeverageBracket({
        exchange,
        symbol: symbolName,
        markPrice: mp,
        accountId: selectedAccountId ?? undefined,
      })
      .then((resp) => {
        if (reqId !== leverageBracketReqIdRef.current) return;
        setLeverageBracket((resp as any) || null);
      })
      .catch((e: any) => {
        if (reqId !== leverageBracketReqIdRef.current) return;
        leverageBracketLoadedKeyRef.current = '';
        if (!disposed && Date.now() - leverageBracketLastErrAtRef.current > 8000) {
          leverageBracketLastErrAtRef.current = Date.now();
          message.error(e?.message || '加载杠杆档位失败');
        }
      });

    return () => {
      disposed = true;
    };
  }, [
    exchange,
    symbolName,
    markPrice?.markPrice,
    leverageInfoNeeded,
    selectedAccountId,
  ]);

  // 指数构成：仅合约（进入「信息 → 指数信息」时再拉取）
  useEffect(() => {
    if (!indexComponentNeeded) return;

    const reqId = ++indexComponentReqIdRef.current;
    let disposed = false;
    setIndexComponentLoading(true);
    api
      .queryIndexComponent({ exchange, symbol: symbolName })
      .then((resp) => {
        if (disposed) return;
        if (reqId !== indexComponentReqIdRef.current) return;
        setIndexComponent((resp as any) || null);
      })
      .catch((e: any) => {
        if (disposed) return;
        if (reqId !== indexComponentReqIdRef.current) return;
        const now = Date.now();
        if (now - indexComponentLastErrAtRef.current > 8000) {
          indexComponentLastErrAtRef.current = now;
          message.error(e?.message || '加载指数构成失败');
        }
      })
      .finally(() => {
        if (!disposed && reqId === indexComponentReqIdRef.current) {
          setIndexComponentLoading(false);
        }
      });
    return () => {
      disposed = true;
    };
  }, [exchange, symbolName, indexComponentNeeded]);

  // exchange / symbol 变化后加载 Market 信息（tickSize/lotSize/precision）
  useEffect(() => {
    loadMarketInfo();
  }, [loadMarketInfo]);

  const tickSize = marketInfo?.rules?.tickSize;
  const quoteAsset = symbolParsed.quote || '';
  const pricePrecision = useMemo(() => {
    const pp = marketInfo?.pricePrecision;
    if (Number.isFinite(pp as number) && (pp as number) >= 0) return pp as number;
    const step = String(tickSize ?? '').trim();
    if (!step) return 4;
    return Math.min(12, Math.max(0, utils.math.getDecimalPrecision(step)));
  }, [marketInfo?.pricePrecision, tickSize]);
  const volumePrecision = useMemo(() => {
    const vp = marketInfo?.baseAssetPrecision;
    if (Number.isFinite(vp as number) && (vp as number) >= 0) return vp as number;
    return 2;
  }, [marketInfo?.baseAssetPrecision]);

  /** K 线图：避免在默认精度下先渲染再跳变，需 Market 返回可用的价格与数量精度来源 */
  const chartPrecisionReady = useMemo(() => {
    if (!symbolName || marketLoading || !marketInfo) return false;
    const pp = marketInfo.pricePrecision;
    const hasPricePrecision = Number.isFinite(pp as number) && (pp as number) >= 0;
    const tick = String(marketInfo.rules?.tickSize ?? '').trim();
    const hasTickSize = Boolean(tick);
    if (!hasPricePrecision && !hasTickSize) return false;
    const vp = marketInfo.baseAssetPrecision;
    return Number.isFinite(vp as number) && (vp as number) >= 0;
  }, [symbolName, marketLoading, marketInfo]);

  const monoPriceStyle = useMemo(
    () =>
    ({
      fontFamily:
        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
      fontVariantNumeric: 'tabular-nums',
    } as React.CSSProperties),
    [],
  );

  const toSafeNumber = useCallback((value: string | number | null | undefined): number => {
    const rawStr = String(value ?? '')
      .replace(/,/g, '')
      .trim();
    if (!rawStr) return 0;
    const n = Number(rawStr);
    return Number.isFinite(n) ? n : 0;
  }, []);

  const formatPrice = useCallback(
    (value: string | number | null | undefined, empty: string = '--') =>
      utils.math.formatByPrecision(value, pricePrecision, empty),
    [pricePrecision],
  );

  const formatVolume = useCallback(
    (value: string | number | null | undefined, empty: string = '--') =>
      utils.math.formatByPrecision(value, volumePrecision, empty),
    [volumePrecision],
  );

  const formatQuoteQty = useCallback(
    (
      price: string | number | null | undefined,
      baseSize: string | number | null | undefined,
      empty: string = '--',
    ) => {
      const pRaw =
        price === null || price === undefined ? '' : String(price).replace(/,/g, '').trim();
      const sRaw =
        baseSize === null || baseSize === undefined
          ? ''
          : String(baseSize).replace(/,/g, '').trim();
      if (!pRaw || !sRaw) return empty;
      const p = Number(pRaw);
      const s = Number(sRaw);
      if (!Number.isFinite(p) || !Number.isFinite(s)) return empty;
      const v = p * s;
      return utils.math.formatKMB(v, { digits: 2, empty });
    },
    [],
  );

  const chartLiqPrice = useMemo<number | null>(() => {
    if (!isPerp) return null;
    const candidates = (positions || [])
      .filter((p) => p.symbol === symbolName)
      .map((p) => Number(String(p.liquidationPrice ?? '').replace(/,/g, '').trim()))
      .filter((v) => Number.isFinite(v) && v > 0);
    if (candidates.length === 0) return null;
    return candidates[0] ?? null;
  }, [isPerp, positions, symbolName]);

  // 当前合约的 symbol 级别杠杆
  useEffect(() => {
    symbolLeverageStreamTsRef.current = 0;
    if (!selectedAccountId || !isPerp) {
      setSymbolLeverage(null);
      return;
    }

    let disposed = false;

    const fetchLeverage = async () => {
      try {
        setSymbolLeverageLoading(true);
        const lev = await getLeverage(selectedAccountId, symbolName);
        if (disposed) return;
        setSymbolLeverage(lev > 0 ? lev : 10);
      } catch (e: any) {
        if (disposed) return;
        message.error(e?.message || '加载杠杆失败');
        setSymbolLeverage((prev) => (prev && prev > 0 ? prev : 10));
      } finally {
        if (!disposed) {
          setSymbolLeverageLoading(false);
        }
      }
    };

    fetchLeverage();

    return () => {
      disposed = true;
    };
  }, [selectedAccountId, symbolName, isPerp]);

  // 当 selectedAccountId 变更时，加载 accountInfo，并重置账户相关数据
  useEffect(() => {
    fullPollPositionsMaxTsRef.current = 0;
    fullPollOpenOrdersMaxTsRef.current = 0;
    fullPollBalanceMaxTsRef.current = 0;
    symbolLeverageStreamTsRef.current = 0;
    if (!selectedAccountId) {
      setAccountInfo(null);
      setBalance(null);
      setPositions([]);
      setOpenOrders([]);
      return;
    }

    const loadAccountData = async () => {
      try {
        const info = await queryAccountInfo(selectedAccountId);
        setAccountInfo(info);
      } catch (err) {
        console.error('Failed to load account info:', err);
        message.error('加载账户信息失败');
      }
    };

    loadAccountData();
  }, [selectedAccountId]);

  // 选中账户后立刻全量拉取当前委托（与 Tab 无关）；首屏数据以 HTTP 为准，账户 WS 仅增量更新
  useEffect(() => {
    if (!selectedAccountId) return;
    setOpenOrders([]);
    let disposed = false;
    void (async () => {
      try {
        const result = await getOrders({
          accountId: selectedAccountId,
          includeFinished: false,
        });
        if (disposed) return;
        const list = result?.list || [];
        fullPollOpenOrdersMaxTsRef.current = list.reduce(
          (m, o) => Math.max(m, Number(o.updatedTs) || 0),
          0,
        );
        setOpenOrders(list);
      } catch (err) {
        if (!disposed) {
          console.error('Failed to load open orders (initial full fetch):', err);
        }
      }
    })();
    return () => {
      disposed = true;
    };
  }, [selectedAccountId]);

  // 根据市场类型自动设置 bottomTab，并激活对应 Tab
  useEffect(() => {
    if (!selectedAccountId) return;
    const key = isPerp ? 'positions' : 'assets';
    setBottomTab(key);
    setActivatedTabs((prev) => new Set([...prev, key]));
  }, [isPerp, selectedAccountId]);

  // 轮询仓位数据（合约）：首次立即全量 + 10s 对齐；与账户 WS 增量叠加
  useEffect(() => {
    if (!selectedAccountId || !isPerp) return;

    let disposed = false;
    const fetchOnce = async () => {
      if (disposed) return;
      try {
        setPositionsLoading(true);
        const data = await getPositions(selectedAccountId);
        if (!disposed) {
          const rows = (data || []).filter((p) => Number(p.amount) > 0);
          fullPollPositionsMaxTsRef.current = rows.reduce(
            (m, p) => Math.max(m, Number(p.updatedTs) || 0),
            0,
          );
          setPositions(rows);
        }
      } catch (err) {
        console.error('Failed to load positions:', err);
      } finally {
        if (!disposed) setPositionsLoading(false);
      }
    };

    fetchOnce();
    const timer = window.setInterval(fetchOnce, 10000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [selectedAccountId, isPerp]);

  // 轮询账户资产：首次立即全量 + 10s 对齐；与账户 WS 增量叠加
  useEffect(() => {
    if (!selectedAccountId) {
      return;
    }

    let disposed = false;
    const fetchOnce = async () => {
      if (disposed) return;
      try {
        setBalanceLoading(true);
        const bal = await getBalance(selectedAccountId);
        if (!disposed) {
          const assets = bal?.assets || [];
          fullPollBalanceMaxTsRef.current = assets.reduce(
            (m, a) => Math.max(m, Number(a.updatedTs) || 0),
            0,
          );
          setBalance(bal);
        }
      } catch (err) {
        if (!disposed) {
          console.error('Failed to load balance:', err);
        }
      } finally {
        if (!disposed) {
          setBalanceLoading(false);
        }
      }
    };

    fetchOnce();
    const timer = window.setInterval(fetchOnce, 10000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [selectedAccountId]);

  // 当前委托 Tab 激活时 10s 轮询对齐（首屏全量已由选中账户时的 effect 拉取）
  useEffect(() => {
    if (!selectedAccountId || bottomTab !== 'open_orders') return;

    let disposed = false;
    const fetchOnce: () => Promise<void> = async () => {
      if (disposed) return;
      try {
        setOpenOrdersLoading(true);
        const result = await getOrders({
          accountId: selectedAccountId,
          includeFinished: false,
        });
        if (!disposed) {
          const list = result?.list || [];
          fullPollOpenOrdersMaxTsRef.current = list.reduce(
            (m, o) => Math.max(m, Number(o.updatedTs) || 0),
            0,
          );
          setOpenOrders(list);
        }
      } catch (err) {
        console.error('Failed to load open orders:', err);
      } finally {
        if (!disposed) setOpenOrdersLoading(false);
      }
    };

    fetchOnce();
    const timer = window.setInterval(fetchOnce, 10000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [selectedAccountId, bottomTab, symbolName]);

  // 历史委托：当对应 Tab 激活时以 10s 间隔轮询
  useEffect(() => {
    if (
      !selectedAccountId ||
      bottomTab !== 'history_orders' ||
      !activatedTabs.has('history_orders')
    ) {
      return;
    }

    let disposed = false;
    const fetchOnce = async () => {
      if (disposed) return;
      try {
        setHistoryOrdersLoading(true);
        const result = await getOrders({
          accountId: selectedAccountId,
          includeFinished: true,
        });
        if (!disposed) {
          setHistoryOrders(result);
        }
      } catch (err) {
        console.error('Failed to load history orders:', err);
      } finally {
        if (!disposed) setHistoryOrdersLoading(false);
      }
    };

    fetchOnce();
    const timer = window.setInterval(fetchOnce, 10000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [selectedAccountId, bottomTab, activatedTabs]);

  // 资金流水：当对应 Tab 激活时以 10s 间隔轮询
  useEffect(() => {
    if (!selectedAccountId || bottomTab !== 'ledgers' || !activatedTabs.has('ledgers')) return;

    let disposed = false;
    const fetchOnce = async () => {
      if (disposed) return;
      try {
        setLedgersLoading(true);
        const endTs = Date.now();
        const startTs = endTs - 7 * 24 * 60 * 60 * 1000; // 近 7 天
        const result = await getLedgers(selectedAccountId, startTs, endTs);
        if (!disposed) {
          setLedgers(result);
        }
      } catch (err) {
        console.error('Failed to load ledgers:', err);
      } finally {
        if (!disposed) setLedgersLoading(false);
      }
    };

    fetchOnce();
    const timer = window.setInterval(fetchOnce, 10000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [selectedAccountId, bottomTab, activatedTabs]);

  const getAvailableAmount = useCallback(
    (asset?: { balance: string; locked: string } | null): number => {
      if (!asset) return 0;
      const total = toSafeNumber(asset.balance);
      const locked = toSafeNumber(asset.locked);
      const available = total - locked;
      return available > 0 ? available : 0;
    },
    [toSafeNumber],
  );

  // 快捷键处理
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!selectedAccountId) return;
      if (e.altKey && e.key === 'b') {
        e.preventDefault();
        setQuickOrderOpen(true);
      } else if (e.altKey && e.key === 's') {
        e.preventDefault();
        setQuickOrderOpen(true);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [selectedAccountId]);

  const renderTableTitle = useCallback((title: string) => {
    return <Typography.Text style={{ fontSize: 12 }}>{title}</Typography.Text>;
  }, []);

  const isOpenOrder = useCallback((side: PositionSide, isBuy: boolean) => {
    return (side === PositionSide.Long && isBuy) || (side === PositionSide.Short && !isBuy);
  }, []);

  // 处理下单（notional 表示成交额，单位为 quote），提交前弹出确认弹窗
  const handlePlaceOrder = useCallback(
    async (params: PlaceOrderParams) => {
      if (!selectedAccountId) {
        message.error('请先选择账户');
        return;
      }

      const notional = Number(params.notional); // 成交额（quote）
      const baseQty = Number(params.baseQty);
      let price = Number(params.price);
      let leverage = Number(params.leverage);
      const isOpen = isOpenOrder(params.side, params.isBuy);

      if (!isPerp) {
        leverage = 1;
      }

      if (!leverage || leverage <= 0) {
        message.error('合约杠杆倍数必须大于 0');
        return;
      }

      if (!notional || notional <= 0) {
        message.error('成交额必须大于 0');
        return;
      }

      if (!baseQty || baseQty <= 0) {
        message.error('数量必须大于 0');
        return;
      }

      if (params.orderType === OrderType.Limit && (!price || price <= 0)) {
        message.error('限价单价格必须大于 0');
        return;
      }
      if (price === 0) {
        message.error('获取最新价格为 0');
        return;
      }

      const quoteQty = notional > 0 ? notional : baseQty * price;

      const rules = marketInfo?.rules as MarketRulesFull | undefined;
      if (rules?.minQuantity && baseQty > 0) {
        if (baseQty < Number(rules.minQuantity)) {
          message.error(`数量不能小于 ${rules.minQuantity}`);
          return;
        }
      }

      if (rules?.minNotional) {
        if (notional < Number(rules.minNotional)) {
          message.error(`订单价值不能小于 ${rules.minNotional}`);
          return;
        }
      }

      // 检查下单额度是否超过可用资金
      if (isOpen) {
        const accountAsset = balance?.assets?.find((a) => a.code === symbolParsed.quote);
        let availableQuote = getAvailableAmount(accountAsset);
        if (isPerp) {
          availableQuote = availableQuote * leverage;
        }
        if (quoteQty > Number(availableQuote)) {
          message.error('下单数量超过可用资金');
          return;
        }
      } else {
        if (isPerp) {
          // 合约平仓：检查当前方向仓位是否足够
          const currentPos = positions.find(
            (p) => p.symbol === symbolName && String(p.side || '').toLowerCase() === params.side,
          );
          const posAmount = currentPos ? Number(currentPos.amount) || 0 : 0;
          if (!currentPos || posAmount <= 0) {
            message.error('当前无可平仓位');
            return;
          }
          if (Number(baseQty) > posAmount) {
            message.error('下单数量超过当前可平仓位');
            return;
          }
        } else {
          const accountAsset = balance?.assets?.find((a) => a.code === symbolParsed.base);
          let availableBase = getAvailableAmount(accountAsset);
          if (availableBase < Number(baseQty)) {
            message.error('下单数量超过可用资金');
            return;
          }
        }
      }

      // 成交额（quote）
      const notionalText = formatPrice(notional, '--');

      // 合约：保证金 / 强平价格（强平价格通过后端预估接口获取）
      let contractMargin: string | number | null = null;
      if (isPerp) {
        const leverageNum = Number(leverage);
        contractMargin = formatPrice(notional / leverageNum, '--');
      }

      const doPlaceOrder = async () => {
        try {
          setOrderLoading(true);

          let side: PositionSide = params.side;
          let isBuy: boolean = params.isBuy;
          let reduceOnly = false;

          await placeOrder({
            accountId: selectedAccountId,
            symbol: symbolName,
            side,
            isBuy,
            orderType: params.orderType,
            price: params.orderType === OrderType.Limit ? String(price) : undefined,
            quantity: String(baseQty),
            timeInForce: 'GTC',
            reduceOnly,
          });

          message.success('下单成功');

          // 通知下单表单重置数量/金额
          setOrderFormResetKey((prev) => prev + 1);

          // 刷新委托列表和余额
          if (selectedAccountId) {
            const [orders, bal] = await Promise.all([
              getOrders({
                accountId: selectedAccountId,
                symbol: symbolName,
                includeFinished: false,
              }),
              getBalance(selectedAccountId),
            ]);
            setOpenOrders(orders?.list || []);
            setBalance(bal);
          }
        } catch (err: any) {
          message.error(`下单失败：${err?.message || '未知错误'}`);
        } finally {
          setOrderLoading(false);
        }
      };

      // 计算确认弹窗中需要展示的关键信息
      const orderSideLabel = (() => {
        if (isPerp) {
          switch (params.side) {
            case PositionSide.Long:
              return params.isBuy ? '开多' : '平多';
            case PositionSide.Short:
              return params.isBuy ? '平空' : '开空';
          }
        }
        return params.isBuy ? '买入' : '卖出';
      })();

      const orderTypeLabel = params.orderType === OrderType.Limit ? '限价单' : '市价单';

      // 确认弹窗内容渲染函数：支持在预估结果返回后进行刷新
      const renderConfirmContent = (estimate?: EstimateOrderResult | null) => (
        <div style={{ fontSize: 13, lineHeight: 1.6 }}>
          <div>
            <Typography.Text type="secondary">交易对：</Typography.Text>
            <Typography.Text>{symbolName}</Typography.Text>
          </div>
          <div>
            <Typography.Text type="secondary">订单方向：</Typography.Text>
            <Space>
              <Typography.Text>{orderSideLabel}</Typography.Text>
              <Typography.Text hidden={!isPerp}>
                {params.leverage ? `${params.leverage}x` : '—'}
              </Typography.Text>
            </Space>
          </div>
          <div>
            <Typography.Text type="secondary">订单类型：</Typography.Text>
            <Typography.Text>{orderTypeLabel}</Typography.Text>
          </div>

          {isPerp ? (
            <>
              <div>
                <Typography.Text type="secondary">金额：</Typography.Text>
                <Typography.Text>
                  {notionalText} {symbolParsed.quote}
                </Typography.Text>
              </div>
              <div>
                <Typography.Text type="secondary">数量：</Typography.Text>
                <Typography.Text>
                  {formatVolume(baseQty) ?? '—'} {symbolParsed.base}
                </Typography.Text>
              </div>
              <div hidden={!isOpen}>
                <Typography.Text type="secondary">保证金：</Typography.Text>
                <Typography.Text>
                  {contractMargin ?? '—'} {contractMargin ? symbolParsed.quote : ''}
                </Typography.Text>
              </div>
              <div hidden={!isOpen}>
                <Typography.Text type="secondary">强平价格：</Typography.Text>
                <Typography.Text>
                  {estimate?.liquidationPrice ? formatPrice(estimate.liquidationPrice) : '—'}
                </Typography.Text>
              </div>
            </>
          ) : (
            <>
              <div>
                <Typography.Text type="secondary">{params.isBuy ? '买入' : '卖出'}：</Typography.Text>
                <Typography.Text>
                  {formatVolume(baseQty) ?? '—'} {symbolParsed.base}
                </Typography.Text>
              </div>
              <div>
                <Typography.Text type="secondary">{params.isBuy ? '卖出' : '买入'}：</Typography.Text>
                <Typography.Text>
                  {formatVolume(quoteQty) ?? '—'} {symbolParsed.quote}
                </Typography.Text>
              </div>
            </>
          )}
          <div>
            <Typography.Text type="secondary">手续费：</Typography.Text>
            <Typography.Text>
              {estimate?.fee ? `${formatVolume(estimate.fee)} ${estimate.feeAsset}` : '—'}
            </Typography.Text>
          </div>
          <div hidden={!isPerp || isOpen}>
            <Typography.Text type="secondary">预估盈亏：</Typography.Text>
            <Typography.Text>
              {estimate?.expectedPnl
                ? `${formatVolume(estimate.expectedPnl)} ${symbolParsed.quote}`
                : '—'}
            </Typography.Text>
          </div>
        </div>
      );

      // 先立即弹出确认弹窗，同时并发请求后端预估接口，返回后更新强平价格并解除按钮 loading
      const modal = Modal.confirm({
        title: '确认下单',
        content: renderConfirmContent(undefined),
        okText: '确认下单',
        cancelText: '取消',
        okButtonProps: {
          loading: true,
        },
        onOk: () => {
          void doPlaceOrder();
        },
      });

      // 调用预估接口
      void (async () => {
        try {
          const resp = await estimateOrder({
            accountId: selectedAccountId,
            symbol: symbolName,
            side: params.side,
            isBuy: params.isBuy,
            orderType: params.orderType,
            price: String(price),
            notional: String(notional),
            leverage: params.leverage,
          });
          const estimate = resp || undefined;
          modal.update({
            content: renderConfirmContent(estimate),
            okButtonProps: {
              loading: false,
            },
          });
        } catch (e) {
          console.error('estimateOrder failed', e);
          modal.update({
            okButtonProps: {
              loading: false,
            },
          });
        }
      })();
    },
    [
      selectedAccountId,
      symbolName,
      marketInfo,
      isPerp,
      toSafeNumber,
      formatPrice,
      formatVolume,
      volumePrecision,
      positions,
      balance,
      symbolParsed,
      exchange,
    ],
  );

  // 处理撤单（弹出确认弹窗）
  const handleCancelOrder = useCallback(
    (order: Order) => {
      if (!selectedAccountId) return;

      const symbol = order.symbol;
      const clientOrderId = order.clientOrderId || '';
      const orderId = order.orderId;

      Modal.confirm({
        title: '确认撤单',
        content: `确认要撤销该委托单 ${orderId}（${symbol}）吗？`,
        okText: '确认',
        cancelText: '取消',
        async onOk() {
          try {
            await cancelOrder(selectedAccountId, symbol, clientOrderId, orderId);
            message.success('撤单成功');

            // 刷新委托列表
            const result = await getOrders({
              accountId: selectedAccountId,
              symbol: symbolName,
              includeFinished: false,
            });
            setOpenOrders(result?.list || []);
          } catch (err: any) {
            message.error(`撤单失败：${err?.message || '未知错误'}`);
          }
        },
      });
    },
    [selectedAccountId, symbolName],
  );

  // 处理平仓
  const handleClosePosition = useCallback(
    async (position: Position) => {
      if (!selectedAccountId) return;

      Modal.confirm({
        title: '平仓确认',
        content: `确认市价平仓 ${position.symbol} ${position.side === PositionSide.Long ? '多头' : '空头'
          } ${position.amount} ${symbolParsed.base}？`,
        okText: '确认',
        cancelText: '取消',
        onOk: async () => {
          console.log(position);
          try {
            const side = position.side;
            const isBuy = side === PositionSide.Short;

            await placeOrder({
              accountId: selectedAccountId,
              symbol: position.symbol,
              side,
              isBuy,
              orderType: OrderType.Market,
              quantity: position.amount,
              reduceOnly: true,
              closePosition: true,
            });

            message.success('平仓成功');

            // 刷新仓位和委托
            const [pos, orders] = await Promise.all([
              getPositions(selectedAccountId),
              getOrders({
                accountId: selectedAccountId,
                symbol: position.symbol,
                includeFinished: false,
              }),
            ]);
            setPositions((pos || []).filter((p) => Number(p.amount) > 0));
            setOpenOrders(orders?.list || []);
          } catch (err: any) {
            message.error(`平仓失败：${err?.message || '未知错误'}`);
          }
        },
      });
    },
    [selectedAccountId],
  );

  return (
    <PageContainer title={false}>
      <SymbolTicker
        exchange={exchange}
        symbol={symbolName}
        pricePrecision={pricePrecision}
        accountId={selectedAccountId}
        ticker={ticker}
        markPrice={markPrice}
        indexPrice={indexPrice ?? undefined}
        fundingRate={fundingRate ?? undefined}
        openInterest={openInterest ?? undefined}
        onSelectionConfirm={handleTickerSelectionConfirm}
      />

      <div ref={topLayoutRef} style={{ display: 'flex', gap: TOP_LAYOUT_GAP, flexWrap: 'nowrap' }}>
        <div style={{ flex: `${topLayout.chartFlex} 1 0`, minWidth: 0 }}>
          <Card style={{ marginBottom: 16, height: 590 }} styles={{ body: { padding: 0 } }}>
            <Tabs
              className={styles.marketTabs}
              activeKey={leftTab}
              onChange={(key) => setLeftTab(key as any)}
              tabBarStyle={{ paddingLeft: 20, paddingRight: 20 }}
              style={{ padding: 0, margin: 0 }}
              destroyOnHidden={false}
              items={[
                {
                  key: 'chart',
                  label: '图表',
                  forceRender: true,
                  children: (
                    <div style={{ position: 'relative', height: 530 }}>
                      {!chartPrecisionReady && (
                        <div
                          style={{
                            position: 'absolute',
                            inset: 0,
                            zIndex: 2,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            padding: 24,
                            background: 'var(--ant-color-bg-container, #ffffff)',
                          }}
                        >
                          {marketLoading ? (
                            <Typography.Text type="secondary">
                              <LoadingOutlined /> 加载交易参数…
                            </Typography.Text>
                          ) : !marketInfo ? (
                            <Empty description="暂无 Market 信息" />
                          ) : (
                            <Empty description="缺少价格或数量精度，无法显示图表" />
                          )}
                        </div>
                      )}
                      <KlineChartPro
                        dataReady={chartPrecisionReady}
                        height={530}
                        visible={leftTab === 'chart'}
                        exchange={exchange}
                        symbol={symbolName}
                        accountId={selectedAccountId ?? undefined}
                        pricePrecision={pricePrecision}
                        volumePrecision={volumePrecision}
                        positions={positions}
                        liqPrice={chartLiqPrice}
                        openOrders={openOrders}
                      />
                    </div>
                  ),
                },
                {
                  key: 'info',
                  label: '信息',
                  children: (
                    <div style={{ padding: '12px 20px 0 20px' }}>
                      <Segmented
                        options={[
                          { label: renderTableTitle('币种信息'), value: 'coin', disabled: true },
                          { label: renderTableTitle('交易参数'), value: 'params' },
                          { label: renderTableTitle('杠杆与保证金'), value: 'leverage' },
                          { label: renderTableTitle('资金费率'), value: 'funding' },
                          { label: renderTableTitle('指数信息'), value: 'index' },
                        ]}
                        value={infoTab}
                        onChange={(v) => setInfoTab(v as any)}
                      />
                      <div style={{ marginTop: 12 }}>
                        {infoTab === 'params' && (
                          <div>
                            {marketLoading ? (
                              <div style={{ padding: '12px 0' }}>
                                <LoadingOutlined />{' '}
                                <Typography.Text type="secondary">加载中...</Typography.Text>
                              </div>
                            ) : null}
                            {!marketLoading && !marketInfo ? (
                              <Empty description="暂无 Market 信息" />
                            ) : null}
                            {marketInfo ? (
                              <div>
                                <div style={{ marginBottom: 32 }}>
                                  <div
                                    style={{
                                      display: 'flex',
                                      alignItems: 'center',
                                      justifyContent: 'space-between',
                                      marginBottom: 8,
                                    }}
                                  >
                                    <Typography.Title level={5}>基础信息</Typography.Title>
                                  </div>
                                  <ProDescriptions<MarketFull>
                                    column={4}
                                    size="small"
                                    colon={false}
                                    layout="vertical"
                                    dataSource={marketInfo as any}
                                    columns={[
                                      { title: '基础资产精度', dataIndex: 'baseAssetPrecision' },
                                      { title: '报价资产精度', dataIndex: 'quoteAssetPrecision' },
                                      { title: '价格精度', dataIndex: 'pricePrecision' },
                                      { title: '交易状态', dataIndex: 'status' },
                                    ]}
                                  />
                                </div>

                                <Typography.Title
                                  level={5}
                                  style={{ marginTop: 18, marginBottom: 8 }}
                                >
                                  过滤规则
                                </Typography.Title>

                                <Tabs
                                  items={[
                                    {
                                      key: 'default',
                                      label: '默认',
                                      children: (
                                        <ProDescriptions
                                          style={{ marginTop: 16 }}
                                          column={3}
                                          dataSource={
                                            (marketInfo as any as MarketFull)?.rules || {}
                                          }
                                          columns={ruleColumns}
                                          emptyText="-"
                                        />
                                      ),
                                    },
                                    ...(
                                      ((marketInfo as any as MarketFull)?.supportOrderTypes ||
                                        []) as MarketOrderTypeFull[]
                                    ).map((ot, idx) => {
                                      const merged = mergeRules(
                                        (marketInfo as any as MarketFull)?.rules,
                                        ot.rules,
                                      );
                                      return {
                                        key: `${ot.orderType || 'orderType'}-${idx}`,
                                        label: ot.orderType || `OrderType-${idx + 1}`,
                                        children: (
                                          <ProDescriptions
                                            style={{ marginTop: 16 }}
                                            column={3}
                                            dataSource={merged || {}}
                                            columns={ruleColumns}
                                            emptyText="-"
                                          />
                                        ),
                                      };
                                    }),
                                  ]}
                                />
                              </div>
                            ) : null}
                          </div>
                        )}
                        {infoTab === 'leverage' && (
                          <div>
                            {!isPerp ? (
                              <Alert type="warning" showIcon message="当前交易对不是永续合约" />
                            ) : (
                              <>
                                <Table
                                  size="small"
                                  scroll={{ y: 416 }}
                                  pagination={false}
                                  rowKey={(row: any) => String(row?.bracket)}
                                  dataSource={leverageBracket?.brackets ?? []}
                                  columns={[
                                    {
                                      title: renderTableTitle('档位'),
                                      dataIndex: 'bracket',
                                      width: 70,
                                    },
                                    {
                                      title: renderTableTitle('最大杠杆'),
                                      dataIndex: 'maxLeverage',
                                      align: 'right',
                                      render: (v: number) => utils.math.formatByPrecision(v, 2),
                                    },
                                    {
                                      title: renderTableTitle('最小名义价值'),
                                      dataIndex: 'minNotional',
                                      align: 'right',
                                      render: (v: string) => utils.math.formatByPrecision(v, 0),
                                    },
                                    {
                                      title: renderTableTitle('最大名义价值'),
                                      dataIndex: 'maxNotional',
                                      align: 'right',
                                      render: (v: string) => utils.math.formatByPrecision(v, 0),
                                    },
                                    {
                                      title: renderTableTitle('维持保证金速算数'),
                                      dataIndex: 'cum',
                                      align: 'right',
                                    },
                                    {
                                      title: renderTableTitle('维持保证金率'),
                                      dataIndex: 'mmr',
                                      align: 'right',
                                      width: 100,
                                      render: (v: string) => utils.math.digitalToPercent(v),
                                    },
                                  ]}
                                />
                              </>
                            )}
                          </div>
                        )}
                        {infoTab === 'funding' && (
                          <div>
                            {!isPerp ? (
                              <Alert type="warning" showIcon message="当前交易对不是永续合约" />
                            ) : (
                              <>
                                <div style={{ marginBottom: 32 }}>
                                  <div
                                    style={{
                                      display: 'flex',
                                      alignItems: 'center',
                                      justifyContent: 'space-between',
                                      marginBottom: 8,
                                    }}
                                  >
                                    <Typography.Title level={5}>实时资金费率</Typography.Title>
                                    <Space size={8}>
                                      {fundingRates7dLoading ? <LoadingOutlined /> : null}
                                    </Space>
                                  </div>
                                  <ProDescriptions
                                    column={3}
                                    size="small"
                                    colon={false}
                                    layout="vertical"
                                    dataSource={{
                                      fundingRate: fundingRate?.fundingRate,
                                      interestRate: fundingRate?.interestRate,
                                      nextFundingTime: fundingRate?.nextFundingTime,
                                      ts: fundingRate?.ts,
                                    }}
                                    columns={[
                                      {
                                        title: '当前资金费率',
                                        dataIndex: 'fundingRate',
                                        render: () => fundingRateText,
                                      },
                                      {
                                        title: '利率',
                                        dataIndex: 'interestRate',
                                        render: (_, row: any) =>
                                          utils.math.digitalToPercent(row?.interestRate),
                                      },
                                      {
                                        title: '下次结算时间',
                                        dataIndex: 'nextFundingTime',
                                        render: (_, row: any) =>
                                          row?.nextFundingTime
                                            ? dayjs(row.nextFundingTime).format(
                                              'YYYY-MM-DD HH:mm:ss',
                                            )
                                            : '--',
                                      },
                                    ]}
                                  />
                                </div>

                                <div style={{ marginTop: 12 }}>
                                  <div
                                    style={{
                                      display: 'flex',
                                      alignItems: 'center',
                                      justifyContent: 'space-between',
                                      marginBottom: 8,
                                    }}
                                  >
                                    <Typography.Title level={5}>资金费率历史</Typography.Title>
                                    <Space size={8}>
                                      {fundingRates7dLoading ? <LoadingOutlined /> : null}
                                    </Space>
                                  </div>

                                  {fundingRates7d.length === 0 ? (
                                    <Empty description="暂无资金费率历史" />
                                  ) : (
                                    <Table<FundingRate>
                                      size="small"
                                      pagination={false}
                                      scroll={{ y: 260 }}
                                      rowKey={(row) => String(row.ts)}
                                      dataSource={fundingRates7d}
                                      columns={[
                                        {
                                          title: renderTableTitle('时间'),
                                          dataIndex: 'ts',
                                          render: (v) => (
                                            <Typography.Text
                                              type="secondary"
                                              style={{ ...monoPriceStyle, fontSize: 12 }}
                                            >
                                              {v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '--'}
                                            </Typography.Text>
                                          ),
                                        },
                                        {
                                          title: renderTableTitle('资金费率'),
                                          dataIndex: 'fundingRate',
                                          render: (v) => (
                                            <Typography.Text
                                              type="secondary"
                                              style={{ ...monoPriceStyle, fontSize: 12 }}
                                            >
                                              {utils.math.digitalToPercent(v, 8)}
                                            </Typography.Text>
                                          ),
                                        },
                                      ]}
                                    />
                                  )}
                                </div>
                              </>
                            )}
                          </div>
                        )}
                        {infoTab === 'index' && (
                          <div>
                            {!isPerp ? (
                              <Alert type="warning" showIcon message="当前交易对不是永续合约" />
                            ) : (
                              <>
                                {indexComponentLoading ? (
                                  <div style={{ padding: '12px 0' }}>
                                    <LoadingOutlined />{' '}
                                    <Typography.Text type="secondary">加载中...</Typography.Text>
                                  </div>
                                ) : indexComponent?.components?.length ? (
                                  <Table
                                    size="small"
                                    pagination={false}
                                    rowKey={(row: any) => `${row.exchange}-${row.symbol}`}
                                    dataSource={indexComponent.components as any}
                                    columns={[
                                      {
                                        title: renderTableTitle('交易所'),
                                        dataIndex: 'exchange',
                                        render: (v: string) => (
                                          <Typography.Text
                                            type="secondary"
                                            style={{ fontSize: 12 }}
                                          >
                                            {v}
                                          </Typography.Text>
                                        ),
                                      },
                                      {
                                        title: renderTableTitle('交易对'),
                                        dataIndex: 'symbol',
                                        render: (v: string) => (
                                          <Typography.Text
                                            type="secondary"
                                            style={{ fontSize: 12 }}
                                          >
                                            {v}
                                          </Typography.Text>
                                        ),
                                      },
                                      {
                                        title: renderTableTitle('权重'),
                                        dataIndex: 'weight',
                                        render: (v: string) => (
                                          <Typography.Text
                                            type="secondary"
                                            style={{ fontSize: 12 }}
                                          >
                                            {v != null && v !== ''
                                              ? utils.math.digitalToPercent(v, 4)
                                              : '--'}
                                          </Typography.Text>
                                        ),
                                      },
                                    ]}
                                  />
                                ) : !indexComponentLoading ? (
                                  <Empty description="暂无指数构成" />
                                ) : null}
                              </>
                            )}
                          </div>
                        )}
                      </div>
                    </div>
                  ),
                },
                {
                  key: 'trading',
                  label: '交易数据',
                  disabled: true,
                  children: null,
                },
              ]}
            />
          </Card>
        </div>

        {topLayout.showOrderbook && (
          <div
            style={{
              flex: `${topLayout.orderbookFlex} 0 0`,
              minWidth: ORDERBOOK_MIN_WIDTH,
              overflow: 'hidden',
            }}
          >
            {/* 订单簿和最新成交 */}
            <Card styles={{ body: { paddingTop: 0 } }} style={{ height: 590, overflow: 'hidden' }}>
              <Tabs
                activeKey={rightTab}
                onChange={(key) => setRightTab(key as 'depth' | 'trade')}
                style={{ padding: 0 }}
                items={[
                  {
                    key: 'depth',
                    label: '订单簿',
                    children: (
                      <Orderbook
                        resetKey={`${exchange}-${symbolName}`}
                        depth={depth}
                        symbol={symbolName}
                        markPrice={markPrice}
                        pricePrecision={pricePrecision}
                      />
                    ),
                  },
                  {
                    key: 'trade',
                    label: '最新成交',
                    children: (
                      <RecentTradesTable
                        trades={trades}
                        quoteAsset={quoteAsset || symbolParsed.quote || ''}
                        monoPriceStyle={monoPriceStyle}
                        pricePrecision={pricePrecision}
                        volumePrecision={volumePrecision}
                        scrollY={462}
                      />
                    ),
                  },
                ]}
              />
            </Card>
          </div>
        )}

        {/* 下单表单 */}
        {selectedAccountId && topLayout.showOrderForm && (
          <div style={{ flex: `${topLayout.orderFormFlex} 0 0`, minWidth: ORDER_FORM_MIN_WIDTH }}>
            <Card
              styles={{ body: { padding: 12, display: 'flex', flexDirection: 'column' } }}
              style={{ height: 590 }}
            >
              <PlaceOrderForm
                loading={orderLoading}
                exchange={exchange}
                symbolName={symbolName}
                pricePrecision={pricePrecision}
                volumePrecision={volumePrecision}
                balance={balance}
                positions={positions}
                ticker={ticker}
                accountId={selectedAccountId}
                leverage={symbolLeverage ?? undefined}
                leverageLoading={symbolLeverageLoading}
                resetKey={orderFormResetKey}
                onLeverageChange={async (lev) => {
                  if (!selectedAccountId || !isPerp) return;
                  try {
                    setSymbolLeverageLoading(true);
                    const applied = await setLeverage(selectedAccountId, symbolName, lev);
                    setSymbolLeverage(applied);
                    message.success(`杠杆已调整至 ${applied}x`);
                    const data = await getPositions(selectedAccountId);
                    setPositions((data || []).filter((p) => Number(p.amount) > 0));
                  } catch (e: any) {
                  } finally {
                    setSymbolLeverageLoading(false);
                  }
                }}
                onPlaceOrder={handlePlaceOrder}
              />
            </Card>
          </div>
        )}
      </div>

      {/* 底部 Tabs */}
      {selectedAccountId && (
        <Card
          style={{ marginTop: 0, height: 350 }}
          styles={{ body: { paddingTop: 0, paddingBottom: 10 } }}
        >
          <Tabs
            activeKey={bottomTab}
            style={{}}
            onChange={(key) => {
              setBottomTab(key);
              setActivatedTabs((prev) => new Set([...prev, key]));
            }}
            items={[
              ...(isPerp
                ? [
                  {
                    key: 'positions',
                    label: '仓位',
                    children: !activatedTabs.has('positions') ? null : (
                      <PositionsTable
                        scrollY={230}
                        positions={positions}
                        onClosePosition={handleClosePosition}
                        accountId={selectedAccountId ?? null}
                        exchange={exchange}
                      />
                    ),
                  },
                ]
                : []),
              {
                key: 'open_orders',
                label: '当前委托',
                children: !activatedTabs.has('open_orders') ? null : (
                  <OrdersTable
                    scrollY={180}
                    mode="onlyOnTheWay"
                    dataSource={openOrders || []}
                    pagination={{ pageSize: 20, total: openOrders?.length || 0 }}
                    pricePrecision={pricePrecision}
                    onCancelOrder={handleCancelOrder}
                  />
                ),
              },
              {
                key: 'history_orders',
                label: '历史委托',
                children: !activatedTabs.has('history_orders') ? null : (
                  <OrdersTable
                    scrollY={180}
                    dataSource={historyOrders?.list || []}
                    pagination={{ pageSize: 20, total: historyOrders?.totalCount || 0 }}
                    pricePrecision={pricePrecision}
                  />
                ),
              },
              {
                key: 'ledgers',
                label: '资金流水',
                children: !activatedTabs.has('ledgers') ? null : (
                  <LedgersTable
                    scrollY={180}
                    mode="account"
                    pagination={{
                      pageSize: 20,
                      total: ledgers?.totalCount ?? ledgers?.list?.length ?? 0,
                    }}
                    dataSource={ledgers?.list || []}
                  />
                ),
              },
              {
                key: 'assets',
                label: '资产',
                children: !activatedTabs.has('assets') ? null : (
                  <AssetsTable
                    assets={filteredAssets}
                    accountId={selectedAccountId ?? null}
                  />
                ),
              },
            ].filter(Boolean)}
          />
        </Card>
      )}
    </PageContainer>
  );
};

export default MarketPage;
