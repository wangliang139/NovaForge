import type { AccountEquity, AccountMetricsResponse } from '@/services/gateway/account';
import {
  BarChartOutlined,
  DollarOutlined,
  FallOutlined,
  LineChartOutlined,
  RiseOutlined,
  TrophyOutlined
} from '@ant-design/icons';
import { Card, Descriptions, Row, Segmented, Space, Tooltip, Typography } from 'antd';
import dayjs from 'dayjs';
import type { FC } from 'react';
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip as RTooltip, XAxis, YAxis } from 'recharts';

type AccountMetricsCardProps = {
  loading?: boolean;
  equityLoading?: boolean;
  equityPoints: AccountEquity[];
  equityRange: string;
  onEquityRangeChange: (range: string) => void;
  metrics: AccountMetricsResponse | null;
  /** 区间天数，跟随 equityRange 计算：1d->1, 7d->7, 30d->30 */
  periodDays: number;
};

const toNumber = (value: unknown) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

const formatUsdt = (value?: string | number) => {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return '0.00';
  return parsed.toFixed(2);
};

const formatPct = (v: number | null | undefined) => {
  if (v == null || !Number.isFinite(v)) return '-';
  const prefix = v >= 0 ? '+' : '';
  return `${prefix}${(v * 100).toFixed(2)}%`;
};

const formatPctAbs = (v: number | null | undefined) => {
  if (v == null || !Number.isFinite(v)) return '-';
  return `${(v * 100).toFixed(2)}%`;
};

const formatNum = (v: number | null | undefined) => {
  if (v == null || !Number.isFinite(v)) return '-';
  return v.toFixed(2);
};

const formatInt = (v: number | null | undefined) => {
  if (v == null || !Number.isFinite(v)) return '-';
  return String(v);
};

const CustomTooltip = ({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: Array<{ payload?: { time?: string; value?: number } }>;
  label?: string;
}) => {
  if (active && payload && payload.length) {
    const p = payload[0]?.payload;
    return (
      <div style={{ border: '1px solid #CCCCCC', background: '#FFFFFF', padding: '10px' }}>
        <Row>
          <Typography.Text style={{ color: '#868E9B' }}>{label ?? '-'}</Typography.Text>
        </Row>
        <Row justify="space-between" align="middle">
          <Typography.Text style={{ color: '#868E9B' }}>总资产估值：</Typography.Text>
          <Typography.Text style={{ color: '#868E9B' }}>{formatUsdt(p?.value)}</Typography.Text>
        </Row>
      </div>
    );
  }
  return null;
};

const AccountMetricsCard: FC<AccountMetricsCardProps> = ({
  loading,
  equityLoading,
  equityPoints,
  equityRange,
  onEquityRangeChange,
  metrics,
  periodDays,
}) => {
  const m = metrics;
  const totalReturnColor = m?.cagr != null && m.cagr >= 0 ? '#52c41a' : '#ff4d4f';
  const maxDrawdownColor = '#faad14';

  const chartData = [...equityPoints]
    .sort((a, b) => a.ts - b.ts)
    .map((point) => ({
      time: dayjs(point.ts).format('MM-DD HH:mm'),
      value: toNumber(point.notional),
    }));

  const chartLoading = loading || equityLoading;

  const renderDescLabel = (label: React.ReactNode, tips?: string, icon?: React.ReactNode,): React.ReactNode => {
    return (
      <Typography.Text type="secondary">
        {icon}
        {tips ? <Tooltip title={tips} placement="top">
          {label}
        </Tooltip> : label}
      </Typography.Text>
    )
  }

  return (
    <>
      <Space direction="vertical" style={{ width: '100%' }} size={16}>
        <Segmented
          options={[
            { label: '一天', value: '1d' },
            { label: '一周', value: '7d' },
            { label: '一月', value: '30d' },
          ]}
          value={equityRange}
          onChange={(value) => onEquityRangeChange(value as string)}
        />
        {chartLoading ? (
          <Card loading style={{ minHeight: 200 }} />
        ) : chartData.length > 0 ? (
          <div style={{ width: '100%', height: 360 }}>
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData} margin={{ top: 8, right: 16, left: 8, bottom: 8 }}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="time" tick={{ fontSize: 12 }} minTickGap={24} angle={-45} textAnchor="end" height={60} />
                <YAxis tickFormatter={(value: number) => formatUsdt(value)} domain={['auto', 'auto']} width={80} />
                <RTooltip content={<CustomTooltip />} />
                <Line type="monotone" dataKey="value" stroke="#1677ff" strokeWidth={2} dot={false} />
              </LineChart>
            </ResponsiveContainer>
          </div>
        ) : (
          <Card>暂无数据</Card>
        )}
        <Descriptions title="绩效指标" column={3} >
          <Descriptions.Item label={renderDescLabel('年化收益率 (CAGR)', '复合年化增长率', <RiseOutlined style={{ marginRight: 4, color: totalReturnColor }} />)}>
            <Typography.Text type="secondary" style={{ color: m?.cagr != null ? totalReturnColor : undefined }}>
              {m ? formatPct(m.cagr) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('最大回撤', '从峰值到谷底的最大跌幅', <FallOutlined style={{ marginRight: 4, color: maxDrawdownColor }} />)}>
            <Typography.Text type="secondary" style={{ color: maxDrawdownColor }}>
              {m ? formatPctAbs(m.maxDrawdown) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('夏普比率', '风险调整后收益', <LineChartOutlined style={{ marginRight: 4 }} />)}>
            <Typography.Text type="secondary">
              {m ? formatNum(m.sharpe) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('索提诺比率', '下行风险调整', <BarChartOutlined style={{ marginRight: 4 }} />)}>
            <Typography.Text type="secondary">
              {m ? formatNum(m.sortino) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('卡玛比率', 'CAGR / 最大回撤', <TrophyOutlined style={{ marginRight: 4 }} />)}>
            <Typography.Text type="secondary">
              {m ? formatNum(m.calmar) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('滚动夏普', '20 日滚动')}>
            <Typography.Text type="secondary">
              {m ? formatNum(m.rollingSharpe) : '-'}
            </Typography.Text>
          </Descriptions.Item>

          <Descriptions.Item label={renderDescLabel('胜率', '盈利交易占比', <TrophyOutlined style={{ marginRight: 4 }} />)}>
            <Typography.Text type="secondary">
              {m ? formatPctAbs(m.winRate) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('盈亏比', '总盈利 / 总亏损', <DollarOutlined style={{ marginRight: 4 }} />)}>
            <Typography.Text type="secondary">
              {m ? formatNum(m.profitFactor) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('手续费占比', '手续费 / 总盈亏')}>
            <Typography.Text type="secondary">
              {m ? formatPctAbs(m.feeRatio) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('平均滑点 (bps)', '限价单')}>
            <Typography.Text type="secondary">
              {m ? formatNum(m.avgSlippageBps) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('最大连续亏损', '笔数')}>
            <Typography.Text type="secondary">
              {m ? formatInt(m.maxConsecutiveLoss) : '-'}
            </Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={renderDescLabel('回撤时长 (秒)', '最长水下时间')}>
            <Typography.Text type="secondary">
              {m ? formatInt(m.timeUnderWaterSeconds) : '-'}
            </Typography.Text>
          </Descriptions.Item>
        </Descriptions>
      </Space>
    </>
  );
};

export default AccountMetricsCard;
