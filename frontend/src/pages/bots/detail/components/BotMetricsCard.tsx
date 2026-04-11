import type { BotMetricsResponse } from '@/services/gateway/strategy';
import {
  BarChartOutlined,
  FallOutlined,
  RiseOutlined,
  LineChartOutlined,
  DollarOutlined,
  TrophyOutlined,
} from '@ant-design/icons';
import { Card, Col, Row, Statistic, Typography } from 'antd';
import type { FC } from 'react';

type BotMetricsCardProps = {
  loading?: boolean;
  metrics: BotMetricsResponse | null;
  /** 区间天数，用于展示 */
  periodDays?: number;
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

const BotMetricsCard: FC<BotMetricsCardProps> = ({
  loading,
  metrics,
  periodDays = 30,
}) => {
  const m = metrics;
  const totalReturnColor = m?.cagr != null && m.cagr >= 0 ? '#52c41a' : '#ff4d4f';
  const maxDrawdownColor = '#faad14';

  return (
    <Card variant="borderless" loading={loading} title={`绩效指标（近${periodDays}日）`}>
      <Row gutter={[16, 16]} justify="space-around">
        <Col xs={12} sm={8} md={6}>
          <Statistic
            title={
              <span>
                <RiseOutlined style={{ marginRight: 4, color: totalReturnColor }} />
                年化收益率 (CAGR)
              </span>
            }
            value={m ? formatPct(m.cagr) : '-'}
            valueStyle={{ color: m?.cagr != null ? totalReturnColor : undefined }}
          />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            复合年化增长率
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic
            title={
              <span>
                <FallOutlined style={{ marginRight: 4, color: maxDrawdownColor }} />
                最大回撤
              </span>
            }
            value={m ? formatPctAbs(m.maxDrawdown) : '-'}
            valueStyle={{ color: maxDrawdownColor }}
          />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            从峰值到谷底的最大跌幅
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic
            title={
              <span>
                <LineChartOutlined style={{ marginRight: 4 }} />
                夏普比率
              </span>
            }
            value={m ? formatNum(m.sharpe) : '-'}
          />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            风险调整后收益
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic
            title={
              <span>
                <BarChartOutlined style={{ marginRight: 4 }} />
                索提诺比率
              </span>
            }
            value={m ? formatNum(m.sortino) : '-'}
          />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            下行风险调整
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic title="卡玛比率" value={m ? formatNum(m.calmar) : '-'} />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            CAGR / 最大回撤
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic title="滚动夏普" value={m ? formatNum(m.rollingSharpe) : '-'} />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            20 日滚动
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic
            title={
              <span>
                <TrophyOutlined style={{ marginRight: 4 }} />
                胜率
              </span>
            }
            value={m ? formatPctAbs(m.winRate) : '-'}
          />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            盈利交易占比
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic title="盈亏比" value={m ? formatNum(m.profitFactor) : '-'} />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            总盈利 / 总亏损
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic
            title={
              <span>
                <DollarOutlined style={{ marginRight: 4 }} />
                手续费占比
              </span>
            }
            value={m ? formatPctAbs(m.feeRatio) : '-'}
          />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            手续费 / 总盈亏
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic title="平均滑点 (bps)" value={m ? formatNum(m.avgSlippageBps) : '-'} />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            限价单
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic title="最大连续亏损" value={m ? formatInt(m.maxConsecutiveLoss) : '-'} />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            笔数
          </Typography.Text>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Statistic title="回撤时长 (秒)" value={m ? formatInt(m.timeUnderWaterSeconds) : '-'} />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            最长水下时间
          </Typography.Text>
        </Col>
      </Row>
    </Card>
  );
};

export default BotMetricsCard;
