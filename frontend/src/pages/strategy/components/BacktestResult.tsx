import { MarketType } from '@/global.types';
import LedgersTable from '@/components/Market/LedgersTable';
import OrdersTable from '@/components/Market/OrdersTable';
import { queryKline } from '@/pages/exchange/service';
import { Ledger, OrderSource } from '@/services/gateway/account';
import { Kline } from '@/services/gateway/market';
import {
  ConsoleLog,
  KlineIntervalOptions,
  RunBacktestResponse,
  SignalType,
  SymbolSummary,
  Fill,
  BacktestSymbolSeriesPoint,
} from '@/services/gateway/strategy';
import {
  Card,
  Checkbox,
  Col,
  Descriptions,
  Empty,
  Row,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  theme,
  Typography,
} from 'antd';
import { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { useEffect, useMemo, useState } from 'react';
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  XAxis,
  YAxis,
} from 'recharts';
import { KlineChart, KlineMarker } from '../../../components/Market/KlineChart';
import { getTradeSideColor, getTradeSideLabel, isOpeningTradeSide } from '@/utils/orderSide';
import utils from '@/utils';

type BacktestResultProps = {
  value: RunBacktestResponse;
};

const defaultPrecision = 8;
const maxKlineBars = 1000;

const ASSET_SERIES_COLORS = [
  '#0ea5e9',
  '#10b981',
  '#f59e0b',
  '#e11d48',
  '#64748b',
  '#14b8a6',
];

const symbolSeriesKey = (sp: Pick<BacktestSymbolSeriesPoint, 'exchange' | 'symbol'>) =>
  `${sp.exchange}-${sp.symbol}`;

const chartDataKeyForAsset = (asset: string) => `asset_${asset}`;

const toMsIfSeconds = (ts: number) => (ts < 1e12 ? ts * 1000 : ts);

const safeFixed = (value: string | number | undefined, precision = defaultPrecision) => {
  const n = typeof value === 'number' ? value : Number(value);
  if (!Number.isFinite(n)) return '-';
  return utils.math.formatByPrecision(n, precision);
};

const isNonEmptyString = (v: unknown): v is string => typeof v === 'string' && v.length > 0;

const getSymbolMarketType = (symbol: string): MarketType => {
  const idx = symbol.lastIndexOf(':');
  if (idx >= 0) {
    const t = symbol.slice(idx + 1).toLowerCase();
    if (t === MarketType.Future) return MarketType.Future;
    if (t === MarketType.Spot) return MarketType.Spot;
  }
  return MarketType.Spot;
};

const isSpotSymbolSummary = (s: SymbolSummary) =>
  getSymbolMarketType(s.symbol) === MarketType.Spot;

const isFutureSymbolSummary = (s: SymbolSummary) =>
  getSymbolMarketType(s.symbol) === MarketType.Future;

const intervalToSeconds = (interval: string): number | undefined => {
  const n = Number.parseFloat(interval);
  if (!Number.isFinite(n)) return undefined;
  if (interval.endsWith('s')) return n;
  if (interval.endsWith('m') && !interval.endsWith('M')) return n * 60;
  if (interval.endsWith('h')) return n * 60 * 60;
  if (interval.endsWith('d')) return n * 60 * 60 * 24;
  if (interval.endsWith('w')) return n * 60 * 60 * 24 * 7;
  if (interval.endsWith('M')) return n * 60 * 60 * 24 * 30;
  return undefined;
};

const toSecondsIfMs = (ts: number) => (ts > 1e12 ? Math.floor(ts / 1000) : ts);

const pickAdaptiveInterval = (startTimeSec: number, endTimeSec: number) => {
  const durationSec = Math.max(0, endTimeSec - startTimeSec);
  const targetSec = durationSec / maxKlineBars;
  const candidates = KlineIntervalOptions.map((o) => o.value)
    .map((v) => ({ v, sec: intervalToSeconds(v) ?? Number.POSITIVE_INFINITY }))
    .sort((a, b) => a.sec - b.sec);

  for (const c of candidates) {
    if (c.sec >= targetSec) return c.v;
  }
  return candidates[candidates.length - 1]?.v ?? '1m';
};

const getSignalIntervalForSymbol = (
  value: RunBacktestResponse,
  exchange: string,
  symbol: string,
) => {
  const klineSignals = (value.strategy?.signals || []).filter((s) => s.type === SignalType.Kline);
  if (!klineSignals.length) return undefined;

  const normalizeExchange = (v: unknown) => String(v ?? '').trim().toLowerCase();
  const normalizeSymbol = (v: unknown) => String(v ?? '').trim().toLowerCase();
  const ex = normalizeExchange(exchange);
  const sym = normalizeSymbol(symbol);

  const allowed = new Set(KlineIntervalOptions.map((o) => o.value));
  const parseInterval = (props?: string) => {
    if (!props) return undefined;
    try {
      const obj = JSON.parse(props) as { interval?: string | number };
      const raw = obj?.interval;
      if (raw == null) return undefined;
      const s = String(raw).trim();
      if (allowed.has(s)) return s;
      // 兼容：interval 可能直接给秒数（如 60 / 300）
      const sec = Number(s);
      if (Number.isFinite(sec)) {
        for (const o of KlineIntervalOptions) {
          const optSec = intervalToSeconds(o.value);
          if (optSec === sec) return o.value;
        }
      }
      return undefined;
    } catch {
      return undefined;
    }
  };

  // 评分策略：Target(交易所+交易对) > Symbol(交易对) > Exchange(交易所) > Strategy(全局)
  let best: { score: number; interval?: string } | undefined;
  for (const s of klineSignals) {
    const interval = parseInterval(s.props);
    if (!interval) continue;

    const sEx = normalizeExchange(s.exchange);
    const sSym = normalizeSymbol(s.symbol);

    let score = -1;
    // 优先按 scope 语义匹配；如果 scope 缺失，则按字段是否存在推断
    const scope = (s as any)?.scope as string | undefined;
    const inferred =
      sEx && sSym ? 'target' : sSym ? 'symbol' : sEx ? 'exchange' : 'strategy';
    const effScope = scope || inferred;

    if (effScope === 'target') {
      if (sEx === ex && sSym === sym) score = 400;
    } else if (effScope === 'symbol') {
      if (sSym === sym) score = 300;
    } else if (effScope === 'exchange') {
      if (sEx === ex) score = 200;
    } else {
      score = 100;
    }

    if (score < 0) continue;
    if (!best || score > best.score) best = { score, interval };
  }

  return best?.interval;
};

const BacktestResult: React.FC<BacktestResultProps> = ({ value }) => {
  const { token } = theme.useToken();

  const assetSeriesList = useMemo(() => {
    const assets = new Set<string>();
    for (const point of value?.data?.equity ?? []) {
      for (const ap of point.assetPoints ?? []) {
        if (ap.asset) {
          assets.add(ap.asset);
        }
      }
    }
    return Array.from(assets)
      .sort((a, b) => a.localeCompare(b))
      .map((asset) => ({ key: asset, label: asset }));
  }, [value?.data?.equity]);

  const [visibleAssetSeries, setVisibleAssetSeries] = useState<string[]>([]);

  useEffect(() => {
    setVisibleAssetSeries(assetSeriesList.map((s) => s.key));
  }, [assetSeriesList]);

  const equityTimeFormat = useMemo(() => {
    const duration = dayjs(toMsIfSeconds(value.endTime)).diff(
      dayjs(toMsIfSeconds(value.startTime)),
      'second',
    );
    return duration > 60 * 5 ? 'MM-DD HH:mm' : 'MM-DD HH:mm:ss';
  }, [value.endTime, value.startTime]);

  const formatedEquityData = useMemo(() => {
    if (!value?.data?.equity?.length) {
      return [];
    }
    return value.data.equity.map((point) => {
      const row: Record<string, number | string> = {
        ts: point.ts,
        netValue: parseFloat(point.netValue),
        time: dayjs(toMsIfSeconds(point.ts)).format(equityTimeFormat),
      };
      for (const ap of point.assetPoints ?? []) {
        if (!ap.asset) continue;
        row[chartDataKeyForAsset(ap.asset)] = parseFloat(ap.netValue);
      }
      return row;
    });
  }, [value?.data?.equity, equityTimeFormat]);

  const backtestOrders = useMemo(
    () =>
      (value?.data?.orders ?? []).map((order) => ({
        ...order,
        source: order.source || OrderSource.Strategy,
      })),
    [value?.data?.orders],
  );

  const equityYDomain = useMemo(() => {
    if (!formatedEquityData.length) {
      return undefined;
    }
    let min = Number.POSITIVE_INFINITY;
    let max = Number.NEGATIVE_INFINITY;
    const consider = (v: unknown) => {
      const n = typeof v === 'number' ? v : Number(v);
      if (!Number.isFinite(n)) return;
      if (n < min) min = n;
      if (n > max) max = n;
    };
    for (const p of formatedEquityData) {
      consider(p.netValue);
      for (const assetKey of visibleAssetSeries) {
        consider(p[chartDataKeyForAsset(assetKey)]);
      }
    }
    if (!Number.isFinite(min) || !Number.isFinite(max)) {
      return undefined;
    }
    const range = max - min;
    const padding = range === 0 ? Math.abs(min) * 0.1 || 1 : range * 0.1;
    return [min - padding, max + padding] as [number, number];
  }, [formatedEquityData, visibleAssetSeries]);

  const backtestLedgers = useMemo((): Ledger[] => {
    return (value?.data?.ledgers ?? []).map((item) => ({
      ...item,
      detail: typeof item.detail === 'string' ? item.detail : JSON.stringify(item.detail ?? {}),
    }));
  }, [value?.data?.ledgers]);

  const positionSeriesBySymbolKey = useMemo(() => {
    const map: Record<
      string,
      { ts: number; time: string; posQty: number; avgPx: number }[]
    > = {};
    for (const s of value?.data?.symbols ?? []) {
      if (!isFutureSymbolSummary(s)) continue;
      const key = `${s.exchange}-${s.symbol}`;
      map[key] = [];
    }
    for (const point of value?.data?.equity ?? []) {
      const time = dayjs(toMsIfSeconds(point.ts)).format(equityTimeFormat);
      for (const sp of point.symbolPoints ?? []) {
        const key = symbolSeriesKey(sp);
        if (!map[key]) continue;
        map[key].push({
          ts: point.ts,
          time,
          posQty: parseFloat(sp.posQty),
          avgPx: parseFloat(sp.avgPx),
        });
      }
    }
    return map;
  }, [value?.data?.equity, value?.data?.symbols, equityTimeFormat]);

  const baseAssetSeriesBySymbolKey = useMemo(() => {
    const map: Record<string, { ts: number; time: string; baseQty: number }[]> = {};
    for (const s of value?.data?.symbols ?? []) {
      if (!isSpotSymbolSummary(s)) continue;
      const key = `${s.exchange}-${s.symbol}`;
      map[key] = [];
    }
    for (const point of value?.data?.equity ?? []) {
      const time = dayjs(toMsIfSeconds(point.ts)).format(equityTimeFormat);
      for (const sp of point.symbolPoints ?? []) {
        const key = symbolSeriesKey(sp);
        if (!map[key]) continue;
        map[key].push({
          ts: point.ts,
          time,
          baseQty: parseFloat(sp.baseQty),
        });
      }
    }
    return map;
  }, [value?.data?.equity, value?.data?.symbols, equityTimeFormat]);

  const consoleColumns: ColumnsType<ConsoleLog> = [
    {
      title: '时间',
      dataIndex: 'ts',
      width: 220,
      render: (ts: number) => dayjs(toMsIfSeconds(ts)).format('YYYY-MM-DD HH:mm:ss.SSS'),
    },
    {
      title: '级别',
      dataIndex: 'level',
      width: 80,
      render: (level: string) => {
        const colorMap: Record<string, string> = {
          error: 'red',
          warn: 'orange',
          info: 'blue',
          debug: 'default',
        };
        return <Tag color={colorMap[level.toLowerCase()] || 'default'}>{level}</Tag>;
      },
      filters: [
        {
          text: 'Debug',
          value: 'debug',
        },
        {
          text: 'Info',
          value: 'info',
        },
        {
          text: 'Warn',
          value: 'warn',
        },
        {
          text: 'Error',
          value: 'error',
        },
      ],
      onFilter: (value, record) => record.level.toLowerCase() === (value as string),
    },
    {
      title: '消息',
      dataIndex: 'message',
    },
  ];

  const renderRawFee = (fill?: Fill) => {
    const feeNum = Number(fill?.fee);
    if (!Number.isFinite(feeNum)) return '-';
    const asset = isNonEmptyString(fill?.asset) ? fill.asset : '-';
    return `${safeFixed(feeNum)}(${asset})`;
  };

  const renderRealizedPnl = (fill?: Fill) => {
    if (!fill) return '-';
    if (isOpeningTradeSide(fill)) {
      return <Typography.Text type="secondary">--</Typography.Text>;
    }
    const pnl = Number(fill.realizedPnl);
    if (!Number.isFinite(pnl)) return '-';
    return (
      <span style={{ color: pnl >= 0 ? '#52c41a' : '#ff4d4f' }}>
        {pnl >= 0 ? '+' : ''}
        {safeFixed(pnl)}
      </span>
    );
  };

  const getFeeInBase = (fill?: Fill): { fee: number; asset: string } | undefined => {
    if (!fill) return undefined;
    if (!isNonEmptyString(fill.feeInBase)) return undefined;
    const feeNum = Number(fill.feeInBase);
    if (!Number.isFinite(feeNum)) return undefined;
    const asset = isNonEmptyString(fill.numeraire) ? fill.numeraire : 'USDT';
    return { fee: feeNum, asset };
  };

  const fillColumns: ColumnsType<Fill> = [
    {
      title: '交易所',
      dataIndex: 'exchange',
      width: 100,
    },
    {
      title: '交易对',
      dataIndex: 'symbol',
      width: 150,
    },
    {
      title: '订单ID',
      dataIndex: 'orderId',
      width: 200,
    },
    {
      title: '方向',
      dataIndex: 'side',
      align: 'center',
      width: 80,
      render: (_: unknown, record: Fill) => (
        <Tag color={getTradeSideColor(record)}>{getTradeSideLabel(record)}</Tag>
      ),
    },
    {
      title: '价格',
      dataIndex: 'price',
      width: 120,
      render: (price?: string) => (price ? utils.math.formatByPrecision(price, defaultPrecision) : '-'),
    },
    {
      title: '数量',
      dataIndex: 'qty',
      width: 120,
      render: (qty?: string) => (qty ? utils.math.formatByPrecision(qty, defaultPrecision) : '-'),
    },
    {
      title: '金额',
      dataIndex: 'amount',
      width: 150,
      render: (_: string, record?: Fill) => {
        const qtyNum = Number(record?.qty);
        const priceNum = Number(record?.price);
        if (!Number.isFinite(qtyNum) || !Number.isFinite(priceNum)) return '-';
        return safeFixed(qtyNum * priceNum);
      },
    },
    {
      title: '手续费',
      dataIndex: 'fee',
      width: 160,
      render: (_: string, record?: Fill) => renderRawFee(record),
    },
    {
      title: '已实现盈亏',
      dataIndex: 'realizedPnl',
      width: 150,
      render: (_: string, record?: Fill) => renderRealizedPnl(record),
    },
    {
      title: '时间',
      dataIndex: 'ts',
      width: 220,
      sorter: (a, b) => (a.ts ?? 0) - (b.ts ?? 0),
      defaultSortOrder: 'ascend',
      sortDirections: ['ascend', 'descend'],
      render: (ts?: number) =>
        ts ? dayjs(toMsIfSeconds(ts)).format('YYYY-MM-DD HH:mm:ss.SSS') : '-',
    },
  ];

  const symbolTabItems = useMemo(() => {
    const symbols = value?.data?.symbols || [];
    return symbols.map((s) => {
      const key = `${s.exchange}-${s.symbol}`;
      return {
        key,
        label: (
          <Space size={6}>
            <Tag>{s.exchange}</Tag>
            <span>{s.symbol}</span>
          </Space>
        ),
        meta: s,
      };
    });
  }, [value]);

  const [activeSymbolKey, setActiveSymbolKey] = useState<string | undefined>(
    () => symbolTabItems[0]?.key,
  );
  const [intervalBySymbol, setIntervalBySymbol] = useState<Record<string, string>>({});
  const [klinesBySymbol, setKlinesBySymbol] = useState<Record<string, Kline[]>>({});
  const [loadingBySymbol, setLoadingBySymbol] = useState<Record<string, boolean>>({});
  const [errorBySymbol, setErrorBySymbol] = useState<Record<string, string | undefined>>({});
  const [loadedBySymbol, setLoadedBySymbol] = useState<Record<string, boolean>>({});
  const [lastQueryKeyBySymbol, setLastQueryKeyBySymbol] = useState<Record<string, string>>({});

  useEffect(() => {
    // value 更新（或首次进入）时：初始化 activeSymbolKey + interval 默认值
    const firstKey = symbolTabItems[0]?.key;
    setActiveSymbolKey(firstKey);

    const nextIntervals: Record<string, string> = {};
    for (const item of symbolTabItems) {
      const meta = (item as any).meta as SymbolSummary;
      const fromSignal = getSignalIntervalForSymbol(value, String(meta.exchange), meta.symbol);
      nextIntervals[item.key] =
        fromSignal ?? pickAdaptiveInterval(toSecondsIfMs(value.startTime), toSecondsIfMs(value.endTime));
    }
    setIntervalBySymbol(nextIntervals);

    // 只预加载第一个标的，其余标的在打开 tab 时再加载
    const nextLoaded: Record<string, boolean> = {};
    if (firstKey) nextLoaded[firstKey] = true;
    setLoadedBySymbol(nextLoaded);

    // 切换回测结果时清空缓存，避免展示旧数据
    setKlinesBySymbol({});
    setLoadingBySymbol({});
    setErrorBySymbol({});
    setLastQueryKeyBySymbol({});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value.id]);

  useEffect(() => {
    if (!activeSymbolKey) return;
    const item = symbolTabItems.find((t) => t.key === activeSymbolKey) as any;
    const meta = item?.meta as SymbolSummary | undefined;
    if (!meta) return;
    if (!loadedBySymbol[activeSymbolKey]) return;

    const interval = intervalBySymbol[activeSymbolKey];
    if (!interval) return;

    const queryKey = `${interval}-${value.startTime}-${value.endTime}-${maxKlineBars}`;
    if (lastQueryKeyBySymbol[activeSymbolKey] === queryKey && klinesBySymbol[activeSymbolKey]) {
      return;
    }

    let cancelled = false;
    setLoadingBySymbol((m) => ({ ...m, [activeSymbolKey]: true }));
    setErrorBySymbol((m) => ({ ...m, [activeSymbolKey]: undefined }));

    queryKline({
      symbol: meta.symbol,
      exchange: meta.exchange,
      interval,
      // RunBacktest 返回的 startTime/endTime 为 Unix 毫秒；Kline 接口同样使用毫秒
      startTime: toMsIfSeconds(value.startTime),
      endTime: toMsIfSeconds(value.endTime),
      limit: maxKlineBars,
    })
      .then((res: any) => {
        const list = (res?.Kline || []) as Kline[];
        const normalized = list.map((k) => ({
          ...k,
          openTs: toMsIfSeconds(k.openTs),
          closeTs: toMsIfSeconds(k.closeTs),
        }));
        if (cancelled) return;
        setKlinesBySymbol((m) => ({ ...m, [activeSymbolKey]: normalized }));
        setLastQueryKeyBySymbol((m) => ({ ...m, [activeSymbolKey]: queryKey }));
      })
      .catch((e: any) => {
        if (cancelled) return;
        setErrorBySymbol((m) => ({
          ...m,
          [activeSymbolKey]: e?.message || 'K 线数据加载失败',
        }));
      })
      .finally(() => {
        if (cancelled) return;
        setLoadingBySymbol((m) => ({ ...m, [activeSymbolKey]: false }));
      });

    return () => {
      cancelled = true;
    };
  }, [
    activeSymbolKey,
    intervalBySymbol[activeSymbolKey || ''],
    loadedBySymbol[activeSymbolKey || ''],
    symbolTabItems,
    value.endTime,
    value.startTime,
    lastQueryKeyBySymbol[activeSymbolKey || ''],
    klinesBySymbol[activeSymbolKey || ''],
  ]);

  const ordersBySymbolKey = useMemo(() => {
    const map: Record<string, any[]> = {};
    const orders = value?.data?.orders || [];
    for (const s of value?.data?.symbols || []) {
      const key = `${s.exchange}-${s.symbol}`;
      map[key] = orders.filter((o) => o?.symbol === s.symbol);
    }
    return map;
  }, [value]);

  const fillsBySymbolKey = useMemo(() => {
    const map: Record<string, Fill[]> = {};
    const fills = (value?.data?.fills || []) as Fill[];
    for (const s of value?.data?.symbols || []) {
      const key = `${s.exchange}-${s.symbol}`;
      map[key] = fills.filter((t) => t?.symbol === s.symbol && t?.exchange === s.exchange);
    }
    return map;
  }, [value]);

  const renderSymbolDescriptions = (s: SymbolSummary) => {
    const pnlColor = (pnl: string) => (Number(pnl) >= 0 ? '#52c41a' : '#ff4d4f');
    const isSpot = isSpotSymbolSummary(s);
    const isFuture = isFutureSymbolSummary(s);

    return (
      <Descriptions size="small" bordered column={3} styles={{ label: { width: 120 } }}>
        <Descriptions.Item label="交易对">{s.symbol}</Descriptions.Item>

        {!isFuture && (
          <Descriptions.Item label="初始基础资产">{safeFixed(s.initialBase)}</Descriptions.Item>
        )}
        <Descriptions.Item label="初始计价资产">{safeFixed(s.initialQuote)}</Descriptions.Item>
        <Descriptions.Item label="初始净值">{safeFixed(s.initialNet)}</Descriptions.Item>

        {!isFuture && (
          <Descriptions.Item label="最终基础资产">{safeFixed(s.finalBase)}</Descriptions.Item>
        )}
        <Descriptions.Item label="最终计价资产">{safeFixed(s.finalQuote)}</Descriptions.Item>
        <Descriptions.Item label="最终净值">{safeFixed(s.finalNet)}</Descriptions.Item>

        {isFuture && (
          <Descriptions.Item label="平均价格">{safeFixed(s.avgPrice)}</Descriptions.Item>
        )}
        <Descriptions.Item label="最新价格">{safeFixed(s.lastPrice)}</Descriptions.Item>

        {isFuture && (
          <Descriptions.Item label="持仓数量">{safeFixed(s.positionQty)}</Descriptions.Item>
        )}

        {isSpot ? (
          <Descriptions.Item label="成交次数">{s.longTrades || 0}</Descriptions.Item>
        ) : (
          <>
            <Descriptions.Item label="多仓成交次数">{s.longTrades || 0}</Descriptions.Item>
            <Descriptions.Item label="空仓成交次数">{s.shortTrades || 0}</Descriptions.Item>
          </>
        )}

        {isSpot && (
          <Descriptions.Item label="手续费">{safeFixed(s.feesInBase)}</Descriptions.Item>
        )}

        {isFuture && (
          <>
            <Descriptions.Item label="总盈亏">
              <span style={{ color: pnlColor(s.netPnl), fontWeight: 600 }}>
                {Number(s.netPnl) >= 0 ? '+' : ''}
                {safeFixed(s.netPnl)}
              </span>
            </Descriptions.Item>
            <Descriptions.Item label="已实现净盈亏(含手续费)">
              {(() => {
                const key = `${s.exchange}-${s.symbol}`;
                const fills = fillsBySymbolKey[key] || [];
                const feeInBaseByAsset: Record<string, number> = {};
                for (const t of fills) {
                  const r = getFeeInBase(t);
                  if (!r) continue;
                  feeInBaseByAsset[r.asset] = (feeInBaseByAsset[r.asset] || 0) + r.fee;
                }
                const feeAssets = Object.keys(feeInBaseByAsset);
                const realizedNet =
                  feeAssets.length === 1
                    ? Number(s.realizedPnl) - (feeInBaseByAsset[feeAssets[0]] || 0)
                    : undefined;
                if (typeof realizedNet === 'number' && Number.isFinite(realizedNet)) {
                  return (
                    <span style={{ color: realizedNet >= 0 ? '#52c41a' : '#ff4d4f' }}>
                      {realizedNet >= 0 ? '+' : ''}
                      {safeFixed(realizedNet)}
                    </span>
                  );
                }
                return <Typography.Text type="secondary">-</Typography.Text>;
              })()}
            </Descriptions.Item>
            <Descriptions.Item label="未实现盈亏">
              <span style={{ color: pnlColor(s.unrealizedPnl) }}>
                {Number(s.unrealizedPnl) >= 0 ? '+' : ''}
                {safeFixed(s.unrealizedPnl)}
              </span>
            </Descriptions.Item>
          </>
        )}
      </Descriptions>
    );
  };

  const renderMarkerTooltip = (marker: KlineMarker, ctx: { kline?: Kline }) => {
    return (
      <Space direction="vertical" size={4} style={{ fontSize: 10 }}>
        <Space>
          <span>类型：</span>
          <span>{marker.payload?.isBuy ? '买入' : '卖出'}</span>
        </Space>
        <Space>
          <span>价格：</span>
          <span>{marker.payload?.price}</span>
        </Space>
        <Space>
          <span>数量：</span>
          <span>{marker.payload?.originalQty}</span>
        </Space>
        <Space>
          <span>金额：</span>
          <span>{(marker.payload?.originalQty * marker.payload?.price).toFixed(2)}</span>
        </Space>
        <Space>
          <span>方向：</span>
          <span>{marker.payload?.side}</span>
        </Space>
        <Space>
          <span>时间：</span>
          <span>{dayjs(toMsIfSeconds(marker.payload?.ts)).format('MM-DD,HH:mm:ss')}</span>
        </Space>
      </Space>
    );
  };

  return (
    <Tabs
      items={[
        {
          key: 'summary',
          label: '概览',
          children: (
            <div>
              <Row gutter={16} style={{ marginBottom: 16 }}>
                <Col span={12}>
                  <Card title="资金信息">
                    <Descriptions column={1} size="small">
                      <Descriptions.Item label="初始资金">
                        {utils.math.formatByPrecision(value.initialBalance, defaultPrecision)}
                      </Descriptions.Item>
                      <Descriptions.Item label="最终资金">
                        {utils.math.formatByPrecision(value.finalBalance, defaultPrecision)}
                      </Descriptions.Item>
                      <Descriptions.Item label="总盈亏">
                        <span
                          style={{
                            color: parseFloat(value.totalPnl) >= 0 ? '#52c41a' : '#ff4d4f',
                            fontWeight: 'bold',
                          }}
                        >
                          {parseFloat(value.totalPnl) >= 0 ? '+' : ''}
                          {utils.math.formatByPrecision(value.totalPnl, defaultPrecision)}
                        </span>
                      </Descriptions.Item>
                      <Descriptions.Item label="收益率">
                        <span
                          style={{
                            color: parseFloat(value.totalPnl) >= 0 ? '#52c41a' : '#ff4d4f',
                            fontWeight: 'bold',
                          }}
                        >
                          {(
                            (parseFloat(value.totalPnl) / parseFloat(value.initialBalance)) *
                            100
                          ).toFixed(2)}
                          %
                        </span>
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>
                </Col>
                <Col span={12}>
                  <Card title="交易统计">
                    <Descriptions column={1} size="small">
                      <Descriptions.Item label="总交易次数">{value.totalTrades}</Descriptions.Item>
                      <Descriptions.Item label="盈利次数">{value.winTrades}</Descriptions.Item>
                      <Descriptions.Item label="亏损次数">{value.lossTrades}</Descriptions.Item>
                      <Descriptions.Item label="胜率">
                        {(value.winRate * 100).toFixed(2)}%
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>
                </Col>
              </Row>
              <Row gutter={16}>
                <Col span={12}>
                  <Card title="风险指标">
                    <Descriptions column={1} size="small">
                      <Descriptions.Item label="夏普比率">
                        {value.sharpeRatio.toFixed(defaultPrecision)}
                      </Descriptions.Item>
                      <Descriptions.Item label="最大回撤">
                        <span style={{ color: '#ff4d4f' }}>
                          {(value.maxDrawdown * 100).toFixed(2)}%
                        </span>
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>
                </Col>
                <Col span={12}>
                  <Card title="其他信息">
                    <Descriptions column={1} size="small">
                      <Descriptions.Item label="回测ID">{value.id}</Descriptions.Item>
                      <Descriptions.Item label="创建时间">
                        {dayjs(toMsIfSeconds(value.createdAt)).format('YYYY-MM-DD HH:mm:ss.SSS')}
                      </Descriptions.Item>
                      <Descriptions.Item label="开始时间">
                        {dayjs(toMsIfSeconds(value.startTime)).format('YYYY-MM-DD HH:mm:ss.SSS')}
                      </Descriptions.Item>
                      <Descriptions.Item label="结束时间">
                        {dayjs(toMsIfSeconds(value.endTime)).format('YYYY-MM-DD HH:mm:ss.SSS')}
                      </Descriptions.Item>
                      <Descriptions.Item label="耗时">{value.timeCost}ms</Descriptions.Item>
                    </Descriptions>
                  </Card>
                </Col>
              </Row>
            </div>
          ),
        },
        {
          key: 'equity',
          label: '净值曲线',
          children: (
            <div>
              {assetSeriesList.length > 0 ? (
                <div style={{ marginBottom: 12 }}>
                  <Typography.Text type="secondary" style={{ marginRight: 12 }}>
                    按资产叠加
                  </Typography.Text>
                  <Checkbox.Group
                    value={visibleAssetSeries}
                    onChange={(keys) => setVisibleAssetSeries(keys as string[])}
                  >
                    {assetSeriesList.map((s, idx) => (
                      <Checkbox key={s.key} value={s.key}>
                        <span style={{ color: ASSET_SERIES_COLORS[idx % ASSET_SERIES_COLORS.length] }}>
                          {s.label}
                        </span>
                      </Checkbox>
                    ))}
                  </Checkbox.Group>
                </div>
              ) : null}
              {formatedEquityData.length > 0 ? (
                <ResponsiveContainer width="100%" height={400}>
                  <LineChart
                    data={formatedEquityData}
                    margin={{ top: 5, right: 30, left: 20, bottom: 5 }}
                  >
                    <CartesianGrid strokeDasharray="3 3" stroke={token.colorBorderSecondary} />
                    <XAxis
                      dataKey="time"
                      tick={{ fontSize: 12, fill: token.colorTextSecondary }}
                      angle={-45}
                      textAnchor="end"
                      height={80}
                    />
                    <YAxis
                      tick={{ fontSize: 12, fill: token.colorTextSecondary }}
                      tickFormatter={(v: number) => v.toFixed(4)}
                      domain={equityYDomain ?? ['auto', 'auto']}
                      allowDataOverflow
                    />
                    <RechartsTooltip
                      contentStyle={{
                        backgroundColor: token.colorBgElevated,
                        border: `1px solid ${token.colorBorderSecondary}`,
                        borderRadius: token.borderRadiusLG,
                        boxShadow: token.boxShadowSecondary,
                      }}
                      labelStyle={{ color: token.colorText, fontWeight: 500 }}
                      itemStyle={{ color: token.colorTextSecondary }}
                      formatter={(v: number, name: string) => [v.toFixed(4), name]}
                      labelFormatter={(_label, payload) => {
                        const ts = payload?.[0]?.payload?.ts as number | undefined;
                        return `时间: ${
                          ts ? dayjs(toMsIfSeconds(ts)).format('YYYY-MM-DD HH:mm:ss') : '-'
                        }`;
                      }}
                    />
                    <Legend />
                    <Line
                      type="monotone"
                      dataKey="netValue"
                      stroke={token.colorPrimary}
                      strokeWidth={2}
                      dot={false}
                      name="总净值"
                    />
                    {assetSeriesList.map((s, idx) =>
                      visibleAssetSeries.includes(s.key) ? (
                        <Line
                          key={s.key}
                          type="monotone"
                          dataKey={chartDataKeyForAsset(s.key)}
                          stroke={ASSET_SERIES_COLORS[idx % ASSET_SERIES_COLORS.length]}
                          strokeWidth={1.5}
                          dot={false}
                          strokeDasharray="4 2"
                          name={s.label}
                        />
                      ) : null,
                    )}
                  </LineChart>
                </ResponsiveContainer>
              ) : (
                <Empty description="暂无净值曲线数据" />
              )}
            </div>
          ),
        },
        {
          key: 'symbols',
          label: '交易对详情',
          children: (
            <div>
              {symbolTabItems.length ? (
                <Tabs
                  type="card"
                  activeKey={activeSymbolKey}
                  onChange={(k) => {
                    setActiveSymbolKey(k);
                    setLoadedBySymbol((m) => ({ ...m, [k]: true }));
                  }}
                  items={symbolTabItems.map((item: any) => {
                    const s = item.meta as SymbolSummary;
                    const interval = intervalBySymbol[item.key];
                    const klines = klinesBySymbol[item.key];
                    const loading = loadingBySymbol[item.key];
                    const error = errorBySymbol[item.key];
                    const symbolOrders = ordersBySymbolKey[item.key] || [];
                    const isSpot = isSpotSymbolSummary(s);
                    const isFuture = isFutureSymbolSummary(s);
                    const positionSeries = positionSeriesBySymbolKey[item.key] || [];
                    const baseAssetSeries = baseAssetSeriesBySymbolKey[item.key] || [];
                    const positionYDomain = (() => {
                      if (!positionSeries.length) return undefined;
                      let min = Number.POSITIVE_INFINITY;
                      let max = Number.NEGATIVE_INFINITY;
                      for (const p of positionSeries) {
                        if (Number.isFinite(p.posQty)) {
                          if (p.posQty < min) min = p.posQty;
                          if (p.posQty > max) max = p.posQty;
                        }
                      }
                      if (!Number.isFinite(min) || !Number.isFinite(max)) return undefined;
                      const pad = (max - min) * 0.1 || 1;
                      return [min - pad, max + pad] as [number, number];
                    })();
                    const baseAssetYDomain = (() => {
                      if (!baseAssetSeries.length) return undefined;
                      let min = Number.POSITIVE_INFINITY;
                      let max = Number.NEGATIVE_INFINITY;
                      for (const p of baseAssetSeries) {
                        if (Number.isFinite(p.baseQty)) {
                          if (p.baseQty < min) min = p.baseQty;
                          if (p.baseQty > max) max = p.baseQty;
                        }
                      }
                      if (!Number.isFinite(min) || !Number.isFinite(max)) return undefined;
                      const pad = (max - min) * 0.1 || 1;
                      return [min - pad, max + pad] as [number, number];
                    })();
                    const marks: KlineMarker[] = symbolOrders
                      .map((o: any) => {
                        const ts = o?.createdTs ?? o?.workingTs ?? o?.updatedTs;
                        const price = Number(o?.price ?? o?.avgPrice);
                        if (!Number.isFinite(ts) || !Number.isFinite(price)) return undefined;
                        return {
                          ts: toMsIfSeconds(ts),
                          isBuy: o?.isBuy ?? String(o?.side || '').toUpperCase() === 'BUY',
                          side: o?.side,
                          qty: Number(o?.executedQty ?? o?.originalQty),
                          payload: o,
                        } as KlineMarker;
                      })
                      .filter(Boolean) as KlineMarker[];
                    return {
                      key: item.key,
                      label: item.label,
                      children: (
                        <div>
                          <Card title="概览">{renderSymbolDescriptions(s)}</Card>
                          <Card title="K 线图" style={{ marginTop: 16 }}>
                            <Row justify="end">
                              <Space size={12} align="center">
                                <Typography.Text type="secondary">K 线间隔：</Typography.Text>
                                <Select
                                  style={{ width: 120 }}
                                  value={interval}
                                  options={KlineIntervalOptions}
                                  onChange={(v) =>
                                    setIntervalBySymbol((m) => ({ ...m, [item.key]: v }))
                                  }
                                />
                              </Space>
                            </Row>
                            <div style={{ marginTop: 8 }}>
                              {error ? (
                                <Typography.Text type="danger">{error}</Typography.Text>
                              ) : null}
                              <Spin spinning={!!loading}>
                                <KlineChart
                                  data={klines}
                                  precision={2}
                                  markers={marks}
                                  tsIsSeconds={false}
                                  renderMarkerTooltip={renderMarkerTooltip}
                                />
                              </Spin>
                            </div>
                          </Card>
                          {isSpot ? (
                            <Card title={`资金曲线（${s.base}）`} style={{ marginTop: 16 }}>
                              {baseAssetSeries.length > 0 ? (
                                <ResponsiveContainer width="100%" height={280}>
                                  <LineChart
                                    data={baseAssetSeries}
                                    margin={{ top: 8, right: 24, left: 8, bottom: 8 }}
                                  >
                                    <CartesianGrid
                                      strokeDasharray="3 3"
                                      stroke={token.colorBorderSecondary}
                                    />
                                    <XAxis
                                      dataKey="time"
                                      tick={{ fontSize: 11, fill: token.colorTextSecondary }}
                                      angle={-35}
                                      textAnchor="end"
                                      height={64}
                                    />
                                    <YAxis
                                      tick={{ fontSize: 11, fill: token.colorTextSecondary }}
                                      tickFormatter={(v: number) => v.toFixed(6)}
                                      domain={baseAssetYDomain ?? ['auto', 'auto']}
                                      allowDataOverflow
                                    />
                                    <RechartsTooltip
                                      contentStyle={{
                                        backgroundColor: token.colorBgElevated,
                                        border: `1px solid ${token.colorBorderSecondary}`,
                                        borderRadius: token.borderRadiusLG,
                                      }}
                                      formatter={(v: number, name: string) => [
                                        Number.isFinite(v) ? v.toFixed(6) : '-',
                                        name,
                                      ]}
                                      labelFormatter={(_label, payload) => {
                                        const ts = payload?.[0]?.payload?.ts as number | undefined;
                                        return ts
                                          ? dayjs(toMsIfSeconds(ts)).format('YYYY-MM-DD HH:mm:ss')
                                          : '-';
                                      }}
                                    />
                                    <Legend />
                                    <Line
                                      type="monotone"
                                      dataKey="baseQty"
                                      stroke="#10b981"
                                      strokeWidth={2}
                                      dot={false}
                                      name={s.base}
                                    />
                                  </LineChart>
                                </ResponsiveContainer>
                              ) : (
                                <Empty
                                  description="暂无资金曲线数据"
                                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                                />
                              )}
                            </Card>
                          ) : null}
                          {isFuture ? (
                            <Card title="仓位曲线" style={{ marginTop: 16 }}>
                              {positionSeries.length > 0 ? (
                                <ResponsiveContainer width="100%" height={280}>
                                  <LineChart
                                    data={positionSeries}
                                    margin={{ top: 8, right: 24, left: 8, bottom: 8 }}
                                  >
                                    <CartesianGrid
                                      strokeDasharray="3 3"
                                      stroke={token.colorBorderSecondary}
                                    />
                                    <XAxis
                                      dataKey="time"
                                      tick={{ fontSize: 11, fill: token.colorTextSecondary }}
                                      angle={-35}
                                      textAnchor="end"
                                      height={64}
                                    />
                                    <YAxis
                                      tick={{ fontSize: 11, fill: token.colorTextSecondary }}
                                      tickFormatter={(v: number) => v.toFixed(6)}
                                      domain={positionYDomain ?? ['auto', 'auto']}
                                      allowDataOverflow
                                    />
                                    <RechartsTooltip
                                      contentStyle={{
                                        backgroundColor: token.colorBgElevated,
                                        border: `1px solid ${token.colorBorderSecondary}`,
                                        borderRadius: token.borderRadiusLG,
                                      }}
                                      formatter={(v: number, name: string) => [
                                        Number.isFinite(v) ? v.toFixed(6) : '-',
                                        name,
                                      ]}
                                      labelFormatter={(_label, payload) => {
                                        const ts = payload?.[0]?.payload?.ts as number | undefined;
                                        return ts
                                          ? dayjs(toMsIfSeconds(ts)).format('YYYY-MM-DD HH:mm:ss')
                                          : '-';
                                      }}
                                    />
                                    <Legend />
                                    <Line
                                      type="monotone"
                                      dataKey="posQty"
                                      stroke="#0ea5e9"
                                      strokeWidth={2}
                                      dot={false}
                                      name="持仓数量"
                                    />
                                  </LineChart>
                                </ResponsiveContainer>
                              ) : (
                                <Empty
                                  description="暂无仓位曲线数据"
                                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                                />
                              )}
                            </Card>
                          ) : null}
                        </div>
                      ),
                    };
                  })}
                />
              ) : (
                <Empty description="暂无交易对数据" />
              )}
            </div>
          ),
        },
        {
          key: 'orders',
          label: '订单记录',
          children: (
            <OrdersTable
              dataSource={backtestOrders}
              pagination={{ pageSize: 50 }}
              scrollY={400}
              pricePrecision={defaultPrecision}
              volumePrecision={defaultPrecision}
              showConditionsColumn={false}
            />
          ),
        },
        {
          key: 'fills',
          label: '成交记录',
          children: (
            <Table
              columns={fillColumns}
              dataSource={value.data.fills || []}
              rowKey={(record, index) => record.orderId || record.clientOrderId || `fill-${index}`}
              pagination={{ pageSize: 50 }}
              scroll={{ x: 'max-content', y: 400 }}
              summary={() => {
                const fills = value?.data?.fills || [];
                const feeByAsset: Record<string, number> = {};
                let realizedPnlSum = 0;

                for (const t of fills) {
                  const feeNum = Number(t?.fee);
                  if (Number.isFinite(feeNum)) {
                    const asset = isNonEmptyString(t?.asset) ? t.asset : '-';
                    feeByAsset[asset] = (feeByAsset[asset] || 0) + feeNum;
                  }
                  const pnl = Number(t?.realizedPnl);
                  if (Number.isFinite(pnl) && !isOpeningTradeSide(t)) {
                    realizedPnlSum += pnl;
                  }
                }

                const feeText = Object.entries(feeByAsset)
                  .sort((a, b) => a[0].localeCompare(b[0]))
                  .map(([asset, total]) => `${safeFixed(total)}(${asset})`)
                  .join(' / ');

                return (
                  <Table.Summary fixed>
                    <Table.Summary.Row>
                      <Table.Summary.Cell index={0} colSpan={7}>
                        <Typography.Text strong>合计</Typography.Text>
                      </Table.Summary.Cell>
                      <Table.Summary.Cell index={1}>
                        <Typography.Text>{feeText || '-'}</Typography.Text>
                      </Table.Summary.Cell>
                      <Table.Summary.Cell index={2}>
                        {Number.isFinite(realizedPnlSum) ? (
                          <Typography.Text
                            style={{ color: realizedPnlSum >= 0 ? '#52c41a' : '#ff4d4f' }}
                          >
                            {realizedPnlSum >= 0 ? '+' : ''}
                            {utils.math.formatByPrecision(realizedPnlSum, defaultPrecision)}
                          </Typography.Text>
                        ) : (
                          <Typography.Text>-</Typography.Text>
                        )}
                      </Table.Summary.Cell>
                      <Table.Summary.Cell index={3} />
                    </Table.Summary.Row>
                  </Table.Summary>
                );
              }}
            />
          ),
        },
        {
          key: 'ledgers',
          label: '资金流水',
          children:
            backtestLedgers.length > 0 ? (
              <LedgersTable
                mode="account"
                dataSource={backtestLedgers}
                pagination={{ pageSize: 20, showSizeChanger: true }}
                scrollY={420}
              />
            ) : (
              <Empty description="暂无资金流水" />
            ),
        },
        {
          key: 'console',
          label: '控制台日志',
          children: (
            <Table
              columns={consoleColumns}
              dataSource={value.consoleLogs}
              rowKey={(record, index) => `${record.ts}-${index}`}
              pagination={{ pageSize: 50 }}
              scroll={{ x: 'max-content', y: 400 }}
            />
          ),
        },
      ]}
    />
  );
};

export default BacktestResult;
