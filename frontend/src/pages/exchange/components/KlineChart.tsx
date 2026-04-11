import { calcKlineAvgPrice } from '@/pages/exchange/types';
import { Kline } from '@/services/gateway/market';
import { getTimeMills } from '@/utils/datetime';
import { WifiOutlined } from '@ant-design/icons';
import { Col, Empty, Flex, Row, Space, Typography } from 'antd';
import dayjs from 'dayjs';
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

type KlineChartProps = {
  syncId?: string;
  activeTime?: number;
  precision?: number;
  klines?: Kline[];
  showDelay?: boolean;
  onActiveKlineChange?: (kline: Kline | undefined) => void;
};

const KlineChart = React.memo((props: KlineChartProps) => {

  const { syncId, precision, klines, activeTime, showDelay, onActiveKlineChange } = props;

  const [latestKline, setLatestKline] = useState<Kline>();
  const [activeKline, setActiveKline] = useState<Kline>();
  const [dots, setDots] = useState<any[]>();
  const [priceTicks, setPriceTicks] = useState<number[]>();
  const [delay, setDelay] = useState<number>(0);

  const [mousePosition, setMousePosition] = useState<any>();
  const chartRef = useRef<any>();

  const getShowKline = () => (activeKline ? activeKline : latestKline);

  const handleMouseMove = (e: any) => {
    if (chartRef.current && e && e.chartY && klines) {
      const chart = chartRef.current;
      const yAxis = chart.state.yAxisMap.price;
      const yScale = yAxis.scale;
      const invertedY = yScale.invert(e.chartY);
      setMousePosition({ x: e.chartX, y: e.chartY, price: invertedY });
    } else {
      setMousePosition(undefined);
    }
    if (e.isTooltipActive && klines) {
      e.activePayload && setActiveKline(e.activePayload[0].payload?.kline);
    } else {
      setActiveKline(undefined);
    }
  };

  useEffect(() => {
    onActiveKlineChange && onActiveKlineChange(activeKline);
  }, [activeKline]);

  useEffect(() => {
    if (mousePosition && klines) {
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
      setActiveKline(chartRef.current.state.activePayload[0].payload.kline);
    }
  }, [klines]);

  useEffect(() => {
    if (!klines || klines.length === 0) {
      setDots(undefined);
      return;
    }
    setLatestKline(klines[klines.length - 1]);
    setDelay(getTimeMills() - klines[klines.length - 1].openTs);
    const dots = [];
    let minPrice = Number(klines[0].open);
    let maxPrice = Number(klines[0].open);
    for (let kline of klines) {
      let price = calcKlineAvgPrice(kline);
      if (price < minPrice) {
        minPrice = price;
      }
      if (price > maxPrice) {
        maxPrice = price;
      }
      dots.push({
        time: kline.openTs,
        price: price,
        volume: Number(kline.quoteVolume),
        kline: kline,
      });
    }
    setDots(dots);
    minPrice = minPrice - (maxPrice - minPrice) * 0.5;
    if (minPrice < 0) {
      minPrice = 0;
    }
    maxPrice = maxPrice + (maxPrice - minPrice) * 0.1;
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
  }, [props]);

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
      return (
        <div style={{ border: '1px solid #CCCCCC', background: '#FFFFFF', padding: '10px' }}>
          <Row>
            <Typography.Text style={{ color: '#868E9B' }}>{`${dayjs(label).format(
              'YYYY/MM/DD HH:mm:ss',
            )}`}</Typography.Text>
          </Row>
          <Flex justify={'space-between'} align={'center'}>
            <Typography.Text style={{ color: '#ff7300' }}>均价：</Typography.Text>
            <Typography.Text style={{ color: '#ff7300' }}>
              {Number(payload[0].value).toFixed(precision)}
            </Typography.Text>
          </Flex>
          <Flex justify={'space-between'} align={'center'}>
            <Typography.Text style={{ color: '#ff7300' }}>成交额：</Typography.Text>
            <Typography.Text style={{ color: '#ff7300' }}>{payload[1].value}</Typography.Text>
          </Flex>
        </div>
      );
    }
    return null;
  };

  if (!klines) return <Empty />;
  return (
    <>
      <Row style={{ marginBottom: '10px', marginLeft: '16px', color: '#868E9B' }}>
        <Col xl={20} lg={24} md={24} sm={24} xs={24}>
          <Space>
            <span>{dayjs(getShowKline()?.openTs).format('YYYY/MM/DD HH:mm:ss')}</span>
            <span>
              均:{' '}
              <span style={{ color: 'red' }}>
                {getShowKline() ? calcKlineAvgPrice(getShowKline()).toFixed(precision) : '-'}
              </span>
            </span>
            <span>
              开:{' '}
              <span style={{ color: 'red' }}>
                {getShowKline() ? Number(getShowKline()?.open).toFixed(precision) : '-'}
              </span>
            </span>
            <span>
              高:{' '}
              <span style={{ color: 'red' }}>
                {getShowKline() ? Number(getShowKline()?.high).toFixed(precision) : '-'}
              </span>
            </span>
            <span>
              低:{' '}
              <span style={{ color: 'red' }}>
                {getShowKline() ? Number(getShowKline()?.low).toFixed(precision) : '-'}
              </span>
            </span>
            <span>
              收:{' '}
              <span style={{ color: 'red' }}>
                {getShowKline() ? Number(getShowKline()?.close).toFixed(precision) : '-'}
              </span>
            </span>
          </Space>
        </Col>
        {showDelay && (
          <Col xl={4} lg={0} md={0} sm={0} xs={0} hidden>
            <Space style={{ float: 'right', width: '150px' }}>
              <WifiOutlined
                style={{ color: delay < 2000 ? 'green' : delay < 10000 ? '#faad14' : 'red' }}
              />
              <span>延迟(ms)：{delay}</span>
            </Space>
          </Col>
        )}
      </Row>
      <div style={{ position: 'relative' }}>
        <ResponsiveContainer width={'100%'} height={250}>
          <ComposedChart
            ref={chartRef}
            data={dots}
            syncId={syncId}
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
              tickFormatter={(value: number) => dayjs(value).format('HH:mm:ss')}
            />
            <YAxis
              yAxisId="price"
              type="number"
              orientation="left"
              ticks={priceTicks}
              domain={['dataMin', 'dataMax']}
            />
            <YAxis
              yAxisId="volume"
              type="number"
              orientation="right"
              domain={[0, (dataMax: number) => dataMax * 5]}
              hide
            />
            <Tooltip
              cursor={{ stroke: '#868E9B', strokeDasharray: '3 3' }}
              content={<CustomTooltip />}
              // formatter={(value, name) => {
              //   return name === '均价' ? Number(value).toFixed(precision) : value
              // }}
              // labelFormatter={(label, payload) => {
              //   return dayjs(label).format('YYYY/MM/DD HH:mm:ss')
              // }}
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
            <Line
              yAxisId="price"
              dataKey="price"
              type="linear"
              stroke="#ff7300"
              strokeWidth={1.5}
              dot={false}
              label={false}
              isAnimationActive={false}
            />
            <Bar
              yAxisId="volume"
              dataKey="volume"
              barSize={20}
              fill="#1890ff"
              isAnimationActive={false}
            />
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
export default KlineChart;
