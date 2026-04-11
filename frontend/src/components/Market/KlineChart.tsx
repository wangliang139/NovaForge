import { Kline } from '@/services/gateway/market';
import { Col, Row, Space, Typography } from 'antd';
import dayjs from 'dayjs';
import {
  CandlestickData,
  createChart,
  CrosshairMode,
  HistogramData,
  IChartApi,
  ISeriesApi,
  SeriesMarker,
  UTCTimestamp,
} from 'lightweight-charts';
import React, { useEffect, useMemo, useRef, useState } from 'react';

export type KlineMarker = {
  /** 毫秒时间戳（推荐：与 Kline.openTs 一致）；如传入秒级时间戳，也可通过 tsIsSeconds 配置 */
  ts: number;
  /** true=买入，false=卖出（可选） */
  isBuy?: boolean;
  /** 轻量标注文本（显示在图上） */
  text?: string;
  /** 订单/信号自定义数据，透传给 tooltip renderer */
  payload?: Record<string, any>;
};

interface KlineChartProps {
  data: Kline[];
  height?: number;
  precision?: number;
  /** 外部传入点位标记（买入/卖出/信号等） */
  markers?: KlineMarker[];
  /** markers.ts 是否为秒级时间戳；默认 false（毫秒） */
  tsIsSeconds?: boolean;
  /**
   * 自定义点位 tooltip 渲染函数（浮层显示在点位旁）
   * - marker: 当前点位
   * - ctx.kline: 当前时间对应的 K 线（若存在）
   */
  renderMarkerTooltip?: (marker: KlineMarker, ctx: { kline?: Kline }) => React.ReactNode;
}

export const KlineChart: React.FC<KlineChartProps> = ({
  data,
  height = 400,
  precision = 6,
  markers,
  tsIsSeconds = false,
  renderMarkerTooltip,
}) => {
  const DEFAULT_BAR_SPACING = 8;
  const DEFAULT_MIN_BAR_SPACING = 2;
  const DEFAULT_INIT_VISIBLE_BARS = 120;

  const clampPrecision = (p: number) => {
    const n = Math.floor(Number(p));
    if (!Number.isFinite(n)) return 6;
    return Math.max(0, Math.min(12, n));
  };
  const priceFormat = useMemo(() => {
    const p = clampPrecision(precision);
    const minMove = p <= 0 ? 1 : Number((1 / Math.pow(10, p)).toFixed(p));
    return { type: 'price' as const, precision: p, minMove };
  }, [precision]);

  const containerRef = useRef<HTMLDivElement>(null);
  const overlayRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const candleSeriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null);
  const volumeSeriesRef = useRef<ISeriesApi<'Histogram'> | null>(null);
  const markerMapRef = useRef<Map<number, KlineMarker[]>>(new Map());
  const timeToKlineMapRef = useRef<Map<number, Kline>>(new Map());
  const renderMarkerTooltipRef = useRef<KlineChartProps['renderMarkerTooltip']>(undefined);
  const userInteractedRef = useRef(false);
  const hasAutoFittedRef = useRef(false);

  const [active, setActive] = useState<{
    timeMs: number;
    open: number;
    high: number;
    low: number;
    close: number;
    volume?: number;
  } | null>(null);
  const [latest, setLatest] = useState<{
    timeMs: number;
    open: number;
    high: number;
    low: number;
    close: number;
    volume?: number;
  } | null>(null);
  const [markerTip, setMarkerTip] = useState<{
    x: number;
    y: number;
    marker: KlineMarker;
    kline?: Kline;
  } | null>(null);

  const shown = active ?? latest;

  const formatVolume = (v: number) => {
    if (!Number.isFinite(v)) return '-';
    const abs = Math.abs(v);
    if (abs > 100000000) return `${(v / 100000000).toFixed(2)} 亿`;
    if (abs > 10000) return `${(v / 10000).toFixed(2)} 万`;
    return v.toFixed(2);
  };

  const normalizeKlines = (klines: Kline[]): CandlestickData[] => {
    if (!Array.isArray(klines) || klines.length === 0) return [];

    // 去重（同一根 K 线可能会重复推送），并保证按时间升序
    const byTime = new Map<number, CandlestickData>();
    for (const k of klines) {
      const t = Math.floor(Number(k?.openTs) / 1000);
      const open = Number(k?.open);
      const high = Number(k?.high);
      const low = Number(k?.low);
      const close = Number(k?.close);

      if (!Number.isFinite(t) || t <= 0) continue;
      if (
        !Number.isFinite(open) ||
        !Number.isFinite(high) ||
        !Number.isFinite(low) ||
        !Number.isFinite(close)
      ) {
        continue;
      }

      byTime.set(t, {
        time: t as UTCTimestamp,
        open,
        high,
        low,
        close,
      });
    }

    return Array.from(byTime.entries())
      .sort(([a], [b]) => a - b)
      .map(([, v]) => v);
  };

  const clampVisibleRange = (
    range: any,
    candleData: CandlestickData[],
  ): { from: any; to: any } | null => {
    if (!range || !Array.isArray(candleData) || candleData.length === 0) return null;
    const minT = Number((candleData[0] as any)?.time);
    const maxT = Number((candleData[candleData.length - 1] as any)?.time);
    if (!Number.isFinite(minT) || !Number.isFinite(maxT)) return null;

    const from = (range as any).from;
    const to = (range as any).to;
    if (typeof from !== 'number' || typeof to !== 'number') {
      // BusinessDay 等情况不做 clamp，直接透传
      return range;
    }
    const nextFrom = Math.min(Math.max(from, minT), maxT);
    const nextTo = Math.min(Math.max(to, minT), maxT);
    if (!Number.isFinite(nextFrom) || !Number.isFinite(nextTo)) return null;
    return { from: nextFrom, to: nextTo };
  };

  const volumeMap = useMemo(() => {
    const map = new Map<number, number>();
    for (const k of data || []) {
      const t = Math.floor(Number(k?.openTs) / 1000);
      const v = Number(k?.quoteVolume ?? k?.volume);
      if (!Number.isFinite(t) || t <= 0) continue;
      if (!Number.isFinite(v)) continue;
      map.set(t, v);
    }
    return map;
  }, [data]);

  const timeToKlineMap = useMemo(() => {
    const map = new Map<number, Kline>();
    for (const k of data || []) {
      const t = Math.floor(Number(k?.openTs) / 1000);
      if (!Number.isFinite(t) || t <= 0) continue;
      map.set(t, k);
    }
    return map;
  }, [data]);

  const klineTimeRanges = useMemo(() => {
    // 按 openTs 升序，便于将 marker.ts 对齐到所在的 K 线区间（openTs <= ts < closeTs）
    const list: Array<{ openMs: number; closeMs: number; openSec: number }> = [];
    for (const k of data || []) {
      const openMs = Number(k?.openTs);
      const closeMs = Number(k?.closeTs);
      if (!Number.isFinite(openMs) || !Number.isFinite(closeMs)) continue;
      const openSec = Math.floor(openMs / 1000);
      if (!Number.isFinite(openSec) || openSec <= 0) continue;
      list.push({ openMs, closeMs, openSec });
    }
    list.sort((a, b) => a.openMs - b.openMs);
    return list;
  }, [data]);

  const markerMap = useMemo(() => {
    const map = new Map<number, KlineMarker[]>();
    const alignToKlineOpenSec = (markerMs: number) => {
      // 二分查找：最后一个 openMs <= markerMs
      let l = 0;
      let r = klineTimeRanges.length - 1;
      let idx = -1;
      while (l <= r) {
        const mid = (l + r) >> 1;
        const v = klineTimeRanges[mid].openMs;
        if (v <= markerMs) {
          idx = mid;
          l = mid + 1;
        } else {
          r = mid - 1;
        }
      }
      if (idx >= 0) {
        const hit = klineTimeRanges[idx];
        if (markerMs <= hit.closeMs) return hit.openSec;
      }
      return Math.floor(markerMs / 1000);
    };

    for (const m of markers || []) {
      const raw = Number(m?.ts);
      if (!Number.isFinite(raw)) continue;
      const markerMs = tsIsSeconds ? raw * 1000 : raw;
      const tSec = alignToKlineOpenSec(markerMs);
      if (!Number.isFinite(tSec) || tSec <= 0) continue;
      const list = map.get(tSec) || [];
      list.push(m);
      map.set(tSec, list);
    }
    return map;
  }, [data, markers, tsIsSeconds, klineTimeRanges]);

  // 关键：让 subscribeCrosshairMove 的回调始终读到“最新”的 markerMap / klineMap / renderer
  useEffect(() => {
    markerMapRef.current = markerMap;
    timeToKlineMapRef.current = timeToKlineMap;
    renderMarkerTooltipRef.current = renderMarkerTooltip;
    // renderer 被移除时，确保浮层立刻隐藏
    if (!renderMarkerTooltip) {
      setMarkerTip(null);
    }
  }, [markerMap, timeToKlineMap, renderMarkerTooltip]);

  const normalizeVolumes = (klines: Kline[]): HistogramData[] => {
    if (!Array.isArray(klines) || klines.length === 0) return [];

    const byTime = new Map<number, HistogramData>();
    for (const k of klines) {
      const t = Math.floor(Number(k?.openTs) / 1000);
      const open = Number(k?.open);
      const close = Number(k?.close);
      // 优先展示 quoteVolume（成交额），缺失则回退 volume（成交量）
      const v = Number(k?.quoteVolume ?? k?.volume);

      if (!Number.isFinite(t) || t <= 0) continue;
      if (!Number.isFinite(v)) continue;

      byTime.set(t, {
        time: t as UTCTimestamp,
        value: v,
        color:
          Number.isFinite(open) && Number.isFinite(close) && close >= open ? '#26a69a' : '#ef5350',
      });
    }

    return Array.from(byTime.entries())
      .sort(([a], [b]) => a - b)
      .map(([, v]) => v);
  };

  // 1️⃣ 初始化 chart（只执行一次）
  useEffect(() => {
    if (!containerRef.current) return;

    const getWidth = () => {
      const el = containerRef.current;
      if (!el) return 1;
      const w = el.clientWidth || el.getBoundingClientRect().width;
      return Math.max(1, Math.floor(w));
    };

    const chart = createChart(containerRef.current, {
      width: getWidth(),
      height,
      layout: {
        background: { color: '#ffffff' },
        textColor: '#333',
      },
      // 关键：让十字线价格标签显示“鼠标所在价格”（而不是吸附到K线 close）
      crosshair: {
        mode: CrosshairMode.Normal,
      },
      // 关键：让时间轴/十字线时间标签按“本地时区”展示
      localization: {
        locale: 'zh-CN',
        timeFormatter: (time: any) => {
          // UTCTimestamp（秒）-> 本地时区
          if (typeof time === 'number' && Number.isFinite(time)) {
            return dayjs(time * 1000).format('YYYY/MM/DD HH:mm:ss');
          }
          // BusinessDay：{ year, month, day }（兜底）
          if (time && typeof time === 'object' && 'year' in time && 'month' in time && 'day' in time) {
            return dayjs(new Date(time.year, time.month - 1, time.day)).format('YYYY/MM/DD');
          }
          return String(time ?? '');
        },
      },
      grid: {
        vertLines: { color: '#eee' },
        horzLines: { color: '#eee' },
      },
      timeScale: {
        timeVisible: true,
        secondsVisible: false,
        // 默认不要把单根/少量 K 线“拉满全屏”（会导致 bar/candle 看起来是最大宽度）
        barSpacing: DEFAULT_BAR_SPACING,
        minBarSpacing: DEFAULT_MIN_BAR_SPACING,
        // 缩放时固定“右侧”（通常是最新时间一侧），表现为从左往右缩放
        fixRightEdge: true,
        rightOffset: 0,
        tickMarkFormatter: (time: any) => {
          if (typeof time === 'number' && Number.isFinite(time)) {
            // tick 常用更短格式
            return dayjs(time * 1000).format('MM/DD HH:mm');
          }
          if (time && typeof time === 'object' && 'year' in time && 'month' in time && 'day' in time) {
            return `${time.month}/${time.day}`;
          }
          return '';
        },
      },
      rightPriceScale: {
        borderColor: '#ccc',
      },
    });

    const candleSeries = chart.addCandlestickSeries({
      upColor: '#26a69a',
      downColor: '#ef5350',
      wickUpColor: '#26a69a',
      wickDownColor: '#ef5350',
      borderVisible: false,
      priceFormat,
    });

    // 成交量（底部直方图）
    const volumeSeries = chart.addHistogramSeries({
      priceFormat: { type: 'volume' },
      priceScaleId: '',
      lastValueVisible: false,
      priceLineVisible: false,
    });
    // 上方价格图留更多空间，底部留给成交量
    candleSeries.priceScale().applyOptions({
      scaleMargins: { top: 0.08, bottom: 0.28 },
    });
    volumeSeries.priceScale().applyOptions({
      scaleMargins: { top: 0.78, bottom: 0 },
    });

    chartRef.current = chart;
    candleSeriesRef.current = candleSeries;
    volumeSeriesRef.current = volumeSeries;

    // 只要用户有过缩放/拖拽/触摸等交互，就不要再自动重置 timeScale
    const el = containerRef.current;
    const markInteracted = () => {
      userInteractedRef.current = true;
    };
    el.addEventListener('wheel', markInteracted, { passive: true });
    el.addEventListener('mousedown', markInteracted);
    el.addEventListener('touchstart', markInteracted, { passive: true });

    // 顶部常驻信息栏：十字线移动时更新 active，移出后恢复为 latest
    const onCrosshairMove = (param: any) => {
      const t = param?.time as UTCTimestamp | undefined;
      if (!t || !param?.point) {
        setActive(null);
        setMarkerTip(null);
        return;
      }
      const candle = (param.seriesData as any)?.get?.(candleSeries) as CandlestickData | undefined;
      if (!candle) {
        setActive(null);
        setMarkerTip(null);
        return;
      }
      const vol = (param.seriesData as any)?.get?.(volumeSeries) as HistogramData | undefined;
      const v = typeof (vol as any)?.value === 'number' ? (vol as any).value : undefined;
      setActive({
        timeMs: Number(t) * 1000,
        open: (candle as any).open,
        high: (candle as any).high,
        low: (candle as any).low,
        close: (candle as any).close,
        volume: v,
      });

      // 点位浮层：鼠标靠近箭头附近才显示
      const renderer = renderMarkerTooltipRef.current;
      if (!renderer) {
        setMarkerTip(null);
        return;
      }

      const point = param.point as { x: number; y: number };
      const timeScale: any = (chart as any).timeScale?.();
      const priceToCoordinate: any = (candleSeries as any).priceToCoordinate?.bind(candleSeries);
      const timeToCoordinate: any = timeScale?.timeToCoordinate?.bind(timeScale);
      if (typeof priceToCoordinate !== 'function' || typeof timeToCoordinate !== 'function') {
        setMarkerTip(null);
        return;
      }

      const visibleRange: any = timeScale?.getVisibleRange?.();
      const fromT = typeof visibleRange?.from === 'number' ? visibleRange.from : undefined;
      const toT = typeof visibleRange?.to === 'number' ? visibleRange.to : undefined;

      const hitRadius = 8; // px：鼠标距离箭头的命中半径
      const yOffset = 12; // px：箭头与蜡烛 high/low 的偏移

      let best:
        | {
            dist2: number;
            x: number;
            y: number;
            marker: KlineMarker;
            kline?: Kline;
          }
        | undefined;

      for (const [timeSec, ms] of markerMapRef.current.entries()) {
        // 只检测当前可视区间附近的点，避免过多遍历
        if (typeof fromT === 'number' && timeSec < fromT) continue;
        if (typeof toT === 'number' && timeSec > toT) continue;

        const x = timeToCoordinate(timeSec as any);
        if (x == null) continue;

        const kline = timeToKlineMapRef.current.get(timeSec);
        if (!kline) continue;

        const high = Number(kline.high);
        const low = Number(kline.low);
        if (!Number.isFinite(high) || !Number.isFinite(low)) continue;

        for (const m of ms) {
          const isBuy = m?.isBuy !== false;
          const basePrice = isBuy ? low : high;
          const baseY = priceToCoordinate(basePrice);
          if (baseY == null) continue;
          const y = isBuy ? baseY + yOffset : baseY - yOffset;

          const dx = point.x - x;
          const dy = point.y - y;
          const dist2 = dx * dx + dy * dy;
          if (dist2 > hitRadius * hitRadius) continue;

          if (!best || dist2 < best.dist2) {
            best = { dist2, x, y, marker: m, kline };
          }
        }
      }

      if (!best) {
        setMarkerTip(null);
        return;
      }

      setMarkerTip({
        x: best.x,
        y: best.y,
        marker: best.marker,
        kline: best.kline,
      });
    };

    chart.subscribeCrosshairMove(onCrosshairMove);

    // resize
    const resizeObserver = new ResizeObserver((entries) => {
      const { width } = entries[0].contentRect;
      chart.applyOptions({ width });
    });

    resizeObserver.observe(containerRef.current);

    return () => {
      chart.unsubscribeCrosshairMove(onCrosshairMove);
      el.removeEventListener('wheel', markInteracted as any);
      el.removeEventListener('mousedown', markInteracted as any);
      el.removeEventListener('touchstart', markInteracted as any);
      resizeObserver.disconnect();
      chart.remove();
    };
  }, [height, priceFormat]);

  // 1.1️⃣ precision 变化时同步更新 Y 轴/价格标签格式
  useEffect(() => {
    const series = candleSeriesRef.current as any;
    if (!series || typeof series.applyOptions !== 'function') return;
    series.applyOptions({ priceFormat });
  }, [priceFormat]);

  // 2️⃣ 设置完整 K 线数据
  useEffect(() => {
    if (!candleSeriesRef.current) return;
    const chart = chartRef.current;
    const timeScale = chart?.timeScale?.();

    const candleData = normalizeKlines(data);

    // 如果用户已经缩放/拖拽过，则尽量保持当前可视区间，避免“缩放后马上弹回”
    const prevRangeRaw = userInteractedRef.current && timeScale ? (timeScale as any).getVisibleRange?.() : null;
    const prevRange = prevRangeRaw ? clampVisibleRange(prevRangeRaw, candleData) : null;

    candleSeriesRef.current.setData(candleData);

    const volumeSeries = volumeSeriesRef.current;
    if (volumeSeries) {
      const volumeData = normalizeVolumes(data);
      volumeSeries.setData(volumeData);
    }

    // latest：默认显示最新一根 K 线（用于“常驻信息栏”）
    const last = candleData[candleData.length - 1] as any;
    if (last && typeof last.time === 'number') {
      const tSec = Number(last.time);
      const vol = volumeMap.get(tSec);
      setLatest({
        timeMs: tSec * 1000,
        open: Number(last.open),
        high: Number(last.high),
        low: Number(last.low),
        close: Number(last.close),
        volume: typeof vol === 'number' ? vol : undefined,
      });
    } else {
      setLatest(null);
    }

    // data 被清空通常意味着“切换品种/周期”，此时允许重新自动定位
    if (!candleData || candleData.length === 0) {
      userInteractedRef.current = false;
      hasAutoFittedRef.current = false;
      return;
    }

    if (!timeScale) return;

    if (userInteractedRef.current && prevRange && typeof (timeScale as any).setVisibleRange === 'function') {
      // setData 可能触发内部 recalculation；下一帧恢复可视区间更稳
      requestAnimationFrame(() => {
        try {
          (timeScale as any).setVisibleRange(prevRange);
        } catch (e) {
          // ignore
        }
      });
      return;
    }

    // 未交互：首次加载设置默认 barSpacing，并定位到最近 N 根，避免“bar 默认最大宽度”
    if (!hasAutoFittedRef.current) {
      hasAutoFittedRef.current = true;
      if (typeof (timeScale as any).applyOptions === 'function') {
        (timeScale as any).applyOptions({
          barSpacing: DEFAULT_BAR_SPACING,
          minBarSpacing: DEFAULT_MIN_BAR_SPACING,
        });
      }

      if (typeof (timeScale as any).scrollToRealTime === 'function') {
        (timeScale as any).scrollToRealTime();
        return;
      }

      // 兜底：按时间范围显示最近 N 根
      const len = candleData.length;
      const from = (candleData[Math.max(0, len - DEFAULT_INIT_VISIBLE_BARS)] as any)?.time;
      const to = (candleData[len - 1] as any)?.time;
      if (
        typeof from === 'number' &&
        typeof to === 'number' &&
        typeof (timeScale as any).setVisibleRange === 'function'
      ) {
        try {
          (timeScale as any).setVisibleRange({ from, to });
        } catch (e) {
          // ignore
        }
      }
      return;
    }
    if (typeof (timeScale as any).scrollToRealTime === 'function') {
      (timeScale as any).scrollToRealTime();
    }
  }, [data]);

  // 2.1️⃣ 设置标记点位（买入/卖出/信号等）
  useEffect(() => {
    const series = candleSeriesRef.current as any;
    if (!series) return;

    const list: Array<SeriesMarker<UTCTimestamp>> = [];
    for (const [t, ms] of markerMap.entries()) {
      for (const m of ms) {
        const isBuy = m?.isBuy !== false;
        list.push({
          time: t as UTCTimestamp,
          position: isBuy ? 'belowBar' : 'aboveBar',
          color: isBuy ? '#26a69a' : '#ef5350',
          shape: isBuy ? 'arrowUp' : 'arrowDown',
          text: m?.text,
        } as any);
      }
    }

    // 关键：lightweight-charts 要求 markers 按 time 升序，否则缩放/滚动后可能出现“部分 marker 不渲染”
    // 同一时间点再做一次稳定排序，尽量让渲染结果可预期（先卖后买/按文本）
    list.sort((a: any, b: any) => {
      const ta = Number(a?.time);
      const tb = Number(b?.time);
      if (ta !== tb) return ta - tb;
      const pa = a?.position === 'aboveBar' ? 0 : 1; // 卖(above)优先
      const pb = b?.position === 'aboveBar' ? 0 : 1;
      if (pa !== pb) return pa - pb;
      const sa = String(a?.text ?? '');
      const sb = String(b?.text ?? '');
      return sa.localeCompare(sb);
    });

    // lightweight-charts v4：setMarkers 在 series 上（类型可能没完全暴露，所以用 any）
    if (typeof series.setMarkers === 'function') {
      series.setMarkers(list);
    }
  }, [markerMap]);

  return (
    <>
      <Row style={{ marginBottom: 10, color: '#868E9B' }}>
        <Col span={24}>
          <Space>
            <span>{shown ? dayjs(shown.timeMs).format('YYYY/MM/DD HH:mm:ss') : '-'}</span>
            <span>
              开:{' '}
              <span style={{ color: 'red' }}>{shown ? shown.open.toFixed(precision) : '-'}</span>
            </span>
            <span>
              高:{' '}
              <span style={{ color: 'red' }}>{shown ? shown.high.toFixed(precision) : '-'}</span>
            </span>
            <span>
              低: <span style={{ color: 'red' }}>{shown ? shown.low.toFixed(precision) : '-'}</span>
            </span>
            <span>
              收:{' '}
              <span style={{ color: 'red' }}>{shown ? shown.close.toFixed(precision) : '-'}</span>
            </span>
            <span>
              成交额:{' '}
              <Typography.Text style={{ color: '#ff7300' }}>
                {shown && typeof shown.volume === 'number' ? formatVolume(shown.volume) : '-'}
              </Typography.Text>
            </span>
          </Space>
        </Col>
      </Row>
      <div style={{ position: 'relative', width: '100%' }}>
        <div
          ref={containerRef}
          style={{ width: '100%' }}
          onDoubleClick={() => chartRef.current?.timeScale().fitContent()}
        />

        {/* 点位旁浮层 tooltip（仅在提供 renderMarkerTooltip 时生效） */}
        {markerTip && renderMarkerTooltip ? (
          <div
            ref={overlayRef}
            style={{
              position: 'absolute',
              left: markerTip.x + 12,
              top: Math.max(0, markerTip.y - 10),
              zIndex: 3,
              pointerEvents: 'none',
              background: '#fff',
              border: '1px solid #e8e8e8',
              borderRadius: 6,
              padding: '8px 10px',
              boxShadow: '0 2px 10px rgba(0,0,0,0.08)',
              maxWidth: 600,
            }}
          >
            {renderMarkerTooltip(markerTip.marker, { kline: markerTip.kline })}
          </div>
        ) : null}
      </div>
    </>
  );
};
