import { Exchange, Symbol } from '@/global.types';
import DelayMonitor from '@/pages/exchange/components/DelayMonitor';
import { calcKlineAvgPrice, SymbolKline } from '@/pages/exchange/types';
import { Kline } from '@/services/gateway/market';
import { getTimeSeconds } from '@/utils/datetime';
import { Col, Empty, Flex, Row, Space, Typography } from 'antd';
import dayjs from 'dayjs';
import Decimal from 'decimal.js';
import React, { useEffect, useRef, useState } from 'react';
import {
  Bar,
  CartesianGrid,
  ComposedChart,
  Line,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

type ExKline = {
  exchange: Exchange;
  kline: SymbolKline | undefined;
};

type SpreadChartProps = {
  symbol?: Symbol | undefined;
  syncId?: string;
  activeTime?: number;
  precision?: number;
  data?: ExKline[];
};

type ComposedKline = {
  time: number;
  // 动态字段：
  // - `${exchange}Price`
  // - `${exchange}Volume`
  // - `${exchange}Kline`
  // exchange 取值来自全局 Exchange enum（如 binance/okx/binance_test/okx_test）
  [key: string]: any;
};

const calcKlineMaxSpread = (kline?: ComposedKline, precision?: number) => {
  if (!kline) return '-';
  const prices = Object.keys(kline)
    .filter((k) => k.endsWith('Price'))
    .map((k) => Number(kline[k]))
    .filter((v) => Number.isFinite(v));
  if (prices.length < 2) return '-';
  const maxPrice = Math.max(...prices);
  const minPrice = Math.min(...prices);
  return new Decimal(maxPrice).sub(new Decimal(minPrice)).toFixed(precision);
};

const SpreadChart = React.memo((props: SpreadChartProps) => {
  const { syncId, symbol, precision, data, activeTime } = props;

  // const [data, setData] = useState<ExKline[] | undefined>(props.data);

  const [latestKline, setLatestKline] = useState<ComposedKline>();
  const [activeKline, setActiveKline] = useState<ComposedKline>();
  const getShowKline = () => (activeKline ? activeKline : latestKline);

  const [dots, setDots] = useState<any[]>([]);

  const [priceTicks, setPriceTicks] = useState<number[]>();
  const [delayByExchange, setDelayByExchange] = useState<Record<string, number>>({});

  const [maxVolume, setMaxVolume] = useState<number>(0);

  const [mousePosition, setMousePosition] = useState<any>();
  const chartRef = useRef<any>();

  const handleMouseMove = (e: any) => {
    if (chartRef.current && e && e.chartY && data) {
      const chart = chartRef.current;
      const yAxis = chart.state.yAxisMap.price;
      const yScale = yAxis.scale;
      const invertedY = yScale.invert(e.chartY);
      setMousePosition({ x: e.chartX, y: e.chartY, price: invertedY });
    } else {
      setMousePosition(undefined);
    }
    if (e.isTooltipActive && data) {
      e.activePayload && setActiveKline(e.activePayload[0].payload);
    } else {
      setActiveKline(undefined);
    }
  };

  useEffect(() => {
    if (mousePosition && data) {
      const chart = chartRef.current;
      const yAxis = chart.state.yAxisMap.price;
      setMousePosition({
        x: mousePosition.x,
        y: mousePosition.y,
        price: yAxis.scale.invert(mousePosition.y),
      });
    }
  }, [priceTicks]);

  useEffect(() => {
    if (mousePosition && chartRef && chartRef.current.state.activePayload) {
      setActiveKline(chartRef.current.state.activePayload[0].payload);
    }
  }, [data]);

  useEffect(() => {
    if (!data || data?.length === 0) {
      setDots([]);
      setActiveKline(undefined);
      setLatestKline(undefined);
      return;
    }

    let minPrice = Infinity;
    let maxPrice = 0;
    let maxVolume = 0;
    const exchanges = data.map((d) => d.exchange);
    const setDelay = (exchange: string, delay: number) => {
      setDelayByExchange((prev) => ({ ...prev, [exchange]: delay }));
    };
    const mergedDots = data.reduce((acc: ComposedKline[], { exchange, kline: sk }) => {
      let klines = sk?.klines;
      if (!klines || klines.length == 0) return acc;
      if (klines.length > 100) {
        klines = klines.slice(-100);
      }
      setDelay(String(exchange), getTimeSeconds() - klines[klines.length - 1].openTs);
      klines?.forEach((kline) => {
        let price = calcKlineAvgPrice(kline);
        if (price < minPrice) {
          minPrice = price;
        }
        if (price > maxPrice) {
          maxPrice = price;
        }
        maxVolume = Math.max(maxVolume, Number(kline.quoteVolume));
        const entry = acc.find((entry) => entry.time === kline.openTs);
        if (entry) {
          entry[`${exchange}Price`] = price;
          entry[`${exchange}Volume`] = Number(kline.quoteVolume);
          entry[`${exchange}Kline`] = kline;
        } else {
          acc.push({
            time: kline.openTs,
            [`${exchange}Price`]: price,
            [`${exchange}Volume`]: Number(kline.quoteVolume),
            [`${exchange}Kline`]: kline,
          });
        }
      });
      return acc;
    }, []);
    mergedDots.sort((a, b) => a.time - b.time);
    setDots(mergedDots);
    setMaxVolume(maxVolume);

    for (let i = mergedDots.length - 1; i >= 0; i--) {
      const ok = exchanges.every((ex) => Boolean(mergedDots[i]?.[`${ex}Kline`]));
      if (ok) {
        setLatestKline(mergedDots[i]);
        break;
      }
    }

    minPrice = minPrice - (maxPrice - minPrice) * 0.5;
    if (minPrice < 0) {
      minPrice = 0;
    }
    maxPrice = maxPrice + (maxPrice - minPrice) * 0.2;
    const gap = (maxPrice - minPrice) / 20;
    const ticks: number[] = [];
    for (let i = 0; i < 21; i++) {
      let p = Number((minPrice + gap * i).toFixed(precision));
      if (p === ticks[ticks.length - 1]) {
        continue;
      }
      ticks.push(p);
    }
    setPriceTicks(ticks);
  }, [data]);

  const CustomTooltip = ({
    active,
    payload,
    label,
  }: {
    active?: any;
    payload?: any;
    label?: any;
  }) => {
    if (active && payload && payload.length) {
      let kline = payload[0].payload;
      return (
        <div style={{ border: '1px solid #CCCCCC', background: '#FFFFFF', padding: '10px' }}>
          <Row>
            <Typography.Text style={{ color: '#868E9B' }}>{`${dayjs
              .unix(label)
              .format('YYYY/MM/DD HH:mm:ss')}`}</Typography.Text>
          </Row>
          {(data || []).map((item) => {
            const ex = String(item.exchange);
            const px = kline?.[`${ex}Price`];
            const vol = kline?.[`${ex}Volume`];
            if (px === undefined && vol === undefined) return null;
            const title = ex.toUpperCase();
            return (
              <React.Fragment key={ex}>
                {px !== undefined && (
                  <Flex justify={'space-between'} align={'center'}>
                    <Typography.Text style={{ color: '#868E9B' }}>{`均价(${title})：`}</Typography.Text>
                    <Typography.Text style={{ color: '#868E9B' }}>
                      {Number(px).toFixed(precision)}
                    </Typography.Text>
                  </Flex>
                )}
                {vol !== undefined && (
                  <Flex justify={'space-between'} align={'center'}>
                    <Typography.Text style={{ color: '#868E9B' }}>{`成交额(${title})：`}</Typography.Text>
                    <Typography.Text style={{ color: '#868E9B' }}>
                      {Number(vol) > 0 ? Number(vol).toFixed(precision) : 0}
                    </Typography.Text>
                  </Flex>
                )}
              </React.Fragment>
            );
          })}
          <Flex justify={'space-between'} align={'center'}>
            <Typography.Text style={{ color: '#389e0d' }}>价差：</Typography.Text>
            <Typography.Text style={{ color: '#389e0d' }}>
              {calcKlineMaxSpread(kline, precision)}
            </Typography.Text>
          </Flex>
        </div>
      );
    }
    return null;
  };

  const KlineTopBar = ({ kline, title }: { kline?: Kline; title: string }) => {
    return (
      <Space>
        <div style={{ width: 70, textAlign: 'right' }}>
          <span>{title}：</span>
        </div>
        <span>{dayjs.unix(kline?.openTs || 0).format('YYYY/MM/DD HH:mm:ss')}</span>
        <span>
          均:{' '}
          <span style={{ color: 'red' }}>
            {kline ? calcKlineAvgPrice(kline).toFixed(precision) : '-'}
          </span>
        </span>
        <span>
          开:{' '}
          <span style={{ color: 'red' }}>
            {kline ? Number(kline?.open).toFixed(precision) : '-'}
          </span>
        </span>
        <span>
          高:{' '}
          <span style={{ color: 'red' }}>
            {kline ? Number(kline?.high).toFixed(precision) : '-'}
          </span>
        </span>
        <span>
          低:{' '}
          <span style={{ color: 'red' }}>
            {kline ? Number(kline?.low).toFixed(precision) : '-'}
          </span>
        </span>
        <span>
          收:{' '}
          <span style={{ color: 'red' }}>
            {kline ? Number(kline?.close).toFixed(precision) : '-'}
          </span>
        </span>
      </Space>
    );
  };

  if (!data) return <Empty />;
  const exchanges = (data || []).map((d) => d.exchange);
  const colorByExchange: Record<string, string> = {
    [Exchange.Binance]: '#ff7300',
    [Exchange.BinanceTest]: '#ffa940',
    [Exchange.OKX]: '#1890ff',
    [Exchange.OKXTest]: '#40a9ff',
  };
  return (
    <>
      {exchanges.map((ex) => (
        <Row
          key={String(ex)}
          style={{ marginBottom: '10px', marginLeft: '16px', color: '#868E9B' }}
        >
          <Col xl={20} lg={24} md={24} sm={24} xs={24}>
            <KlineTopBar
              title={String(ex).toUpperCase()}
              kline={getShowKline()?.[`${ex}Kline`]}
            />
          </Col>
          <Col xl={4} lg={0} md={0} sm={0} xs={0}>
            <DelayMonitor delay={delayByExchange[String(ex)] || 0} />
          </Col>
        </Row>
      ))}
      <div style={{ position: 'relative' }}>
        <ResponsiveContainer width={'100%'} height={250}>
          <ComposedChart
            ref={chartRef}
            syncId={syncId}
            data={dots}
            margin={{ left: 20, top: 10 }}
            onMouseMove={handleMouseMove}
            onMouseLeave={() => {
              setActiveKline(undefined);
              setMousePosition(undefined);
            }}
          >
            <XAxis
              dataKey="time"
              name="Time"
              interval={30}
              tickFormatter={(value: number) => dayjs.unix(value).format('HH:mm:ss')}
            />
            <YAxis
              yAxisId="price"
              dataKey="price"
              type="number"
              orientation="left"
              ticks={priceTicks}
              domain={['dataMin', 'dataMax']}
            />
            <YAxis
              yAxisId={'volume'}
              type="number"
              orientation="right"
              domain={[0, maxVolume * 5]}
              hide
            />
            <Tooltip
              cursor={{ stroke: '#868E9B', strokeDasharray: '3 3' }}
              content={<CustomTooltip />}
            />
            {mousePosition && (
              <>
                <ReferenceLine
                  y={mousePosition?.price}
                  yAxisId="price"
                  stroke="#868E9B"
                  strokeDasharray="3 3"
                />
              </>
            )}
            {!activeKline && activeTime && (
              <>
                <ReferenceLine
                  x={activeTime}
                  yAxisId="price"
                  stroke="#868E9B"
                  strokeDasharray="3 3"
                />
              </>
            )}
            <CartesianGrid stroke="#f5f5f5" />
            {exchanges.map((ex) => {
              const color = colorByExchange[String(ex)] || '#868E9B';
              return (
                <React.Fragment key={String(ex)}>
                  <Line
                    yAxisId="price"
                    dataKey={`${ex}Price`}
                    type="linear"
                    stroke={color}
                    strokeWidth={1.5}
                    dot={false}
                    label={false}
                    isAnimationActive={false}
                  />
                  <Bar
                    yAxisId="volume"
                    dataKey={`${ex}Volume`}
                    barSize={20}
                    fill={color}
                    stackId="a"
                    isAnimationActive={false}
                  />
                </React.Fragment>
              );
            })}
          </ComposedChart>
        </ResponsiveContainer>
        {/*<div
          style={{
            left: 0,
            right: 0,
            top: 0,
            bottom: 0,
            position: 'absolute',
            zIndex: 100,
            pointerEvents: 'none',
            border: '1px solid #D9D9D9',
          }}></div>*/}
        {mousePosition && (
          <div
            style={{
              position: 'absolute',
              background: '#868E9B',
              fontSize: 12,
              top: mousePosition.y - 10,
              left: 0,
              width: chartRef.current.state.offset.left,
              height: 20,
              color: 'white',
              textAlign: 'center',
              pointerEvents: 'none',
            }}
          >
            <div>{mousePosition?.price.toFixed(precision)}</div>
          </div>
        )}
      </div>
    </>
  );
});
export default SpreadChart;
