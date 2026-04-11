import { Kline } from '@/services/gateway/market';
import type { Chart, KLineData } from 'klinecharts';
import { dispose, init } from 'klinecharts';
import React, { useEffect, useRef, useState } from 'react';

export type KlineMarker = {
  ts: number;
  isBuy?: boolean;
  text?: string;
  payload?: Record<string, any>;
};

interface KlineChart2Props {
  symbol: string;
  period: string;
  data: Kline[];
  height?: number;
  precision?: number;
  markers?: KlineMarker[];
  tsIsSeconds?: boolean;
  renderMarkerTooltip?: (marker: KlineMarker, ctx: { kline?: Kline }) => React.ReactNode;
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
    const volume = Number(k?.quoteVolume ?? k?.volume);
    if (!Number.isFinite(ts) || ts <= 0) continue;
    if (!Number.isFinite(open) || !Number.isFinite(high) || !Number.isFinite(low) || !Number.isFinite(close)) continue;
    byTime.set(ts, { timestamp: ts, open, high, low, close, volume: Number.isFinite(volume) ? volume : undefined });
  }
  return Array.from(byTime.entries())
    .sort(([a], [b]) => a - b)
    .map(([, v]) => v);
}

function periodToSpan(period: string): number {
  const n = Number(period.replace(/[smhd]/g, ''));
  if (!Number.isFinite(n)) return 1;
  return Math.max(1, Math.min(1000, n));
}

function periodToSpanType(period: string): 'second' | 'minute' | 'hour' | 'day' {
  if (period.endsWith('s')) return 'second';
  if (period.endsWith('m')) return 'minute';
  if (period.endsWith('h')) return 'hour';
  if (period.endsWith('d')) return 'day';
  return 'minute';
}

export const KlineChart2: React.FC<KlineChart2Props> = ({
  symbol,
  period,
  data,
  height = 400,
  precision = 6,
}) => {
  const clampPrecision = (p: number) => {
    const n = Math.floor(Number(p));
    if (!Number.isFinite(n)) return 6;
    return Math.max(0, Math.min(12, n));
  };
  const pricePrecision = clampPrecision(precision);
  const volumePrecision = 2;

  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<Chart | null>(null);
  const dataRef = useRef<Kline[]>([]);

  const [crosshairInfo, setCrosshairInfo] = useState<{
    timeMs: number;
    open: number;
    high: number;
    low: number;
    close: number;
    volume?: number;
  } | null>(null);

  const [latestInfo, setLatestInfo] = useState<{
    timeMs: number;
    open: number;
    high: number;
    low: number;
    close: number;
    volume?: number;
  } | null>(null);

  // 初始化图表
  useEffect(() => {
    if (!containerRef.current) return;
    const el = containerRef.current;
    const chart = init(el, {
      locale: 'zh-CN',
      layout: [
        { type: 'candle' },
        { type: 'indicator', content: ['VOL'] },
      ],
    });
    if (!chart) return;

    dataRef.current = data;
    chart.setSymbol({
      ticker: symbol,
      pricePrecision,
      volumePrecision,
    });
    chart.setPeriod({ span: periodToSpan(period), type: periodToSpanType(period) });
    chart.setDataLoader({
      getBars: (params) => {
        const list = klinesToKLineData(dataRef.current);
        params.callback(list, false);
      },
    });

    const onCrosshair: (data: unknown) => void = (payload) => {
      const crosshair = payload as { kLineData?: KLineData };
      const k = crosshair?.kLineData;
      if (k) {
        setCrosshairInfo({
          timeMs: k.timestamp,
          open: k.open,
          high: k.high,
          low: k.low,
          close: k.close,
          volume: k.volume,
        });
      } else {
        setCrosshairInfo(null);
      }
    };

    chart.subscribeAction('onCrosshairChange', onCrosshair);

    const ro = new ResizeObserver(() => chart.resize());
    ro.observe(el);

    chartRef.current = chart;
    return () => {
      chart.unsubscribeAction('onCrosshairChange', onCrosshair);
      ro.disconnect();
      dispose(el);
      chartRef.current = null;
    };
  }, [height]);

  // 周期变化时更新图表
  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) return;
    chart.setPeriod({ span: periodToSpan(period), type: periodToSpanType(period) });
  }, [period]);

  // 精度变化时更新
  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) return;
    chart.setSymbol({
      ticker: symbol,
      pricePrecision,
      volumePrecision,
    });
  }, [pricePrecision]);

  // 数据变化时刷新
  useEffect(() => {
    dataRef.current = data;
    const chart = chartRef.current;
    if (!chart) return;
    chart.resetData();

    const list = klinesToKLineData(data);
    const last = list[list.length - 1];
    if (last) {
      setLatestInfo({
        timeMs: last.timestamp,
        open: last.open,
        high: last.high,
        low: last.low,
        close: last.close,
        volume: last.volume,
      });
    } else {
      setLatestInfo(null);
    }
  }, [data]);

  return (
    <>
      <div style={{ position: 'relative', width: '100%' }}>
        <div
          ref={containerRef}
          style={{ width: '100%', height: height ? `${height}px` : 400 }}
        />
      </div>
    </>
  );
};
