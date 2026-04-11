import type { DashboardOverview } from '@/services/gateway/dashboard';
import utils from '@/utils';
import { BookOutlined, DollarOutlined, RobotOutlined, TeamOutlined } from '@ant-design/icons';
import { Col, Row, Statistic } from 'antd';
import { ChartCard } from './Charts';

const topColResponsiveProps = {
  xs: 24,
  sm: 12,
  md: 12,
  lg: 12,
  xl: 6,
  style: {
    marginBottom: 24,
  },
};

type IntroduceRowProps = {
  loading: boolean;
  data?: DashboardOverview | null;
  totalAccountNotional?: string;
  totalAccount24hChange?: string;
};

const IntroduceRow: React.FC<IntroduceRowProps> = ({
  loading,
  data,
  totalAccountNotional = '0',
  totalAccount24hChange = '0',
}) => {
  const changeNum = parseFloat(totalAccount24hChange);
  const changeColor = changeNum >= 0 ? '#52c41a' : '#ff4d4f';
  const changePrefix = changeNum >= 0 ? '+' : '';

  return (
    <Row gutter={24}>
      <Col {...topColResponsiveProps}>
        <ChartCard
          variant="borderless"
          loading={loading}
          title="资金概览"
          total={() => (
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, fontSize: 18 }}>
              <Statistic
                title={null}
                value={totalAccountNotional}
                prefix="$"
              />
              <span style={{ color: changeColor, fontSize: 14, flexShrink: 0 }}>
                ({changePrefix}{utils.math.formatByPrecision(totalAccount24hChange, 2)})
              </span>
            </div>
          )}
          contentHeight={46}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <DollarOutlined style={{ fontSize: 20, color: '#faad14' }} />
            <span style={{ color: '#666' }}>总现金价值 / 24h 总收益</span>
          </div>
        </ChartCard>
      </Col>

      <Col {...topColResponsiveProps}>
        <ChartCard
          variant="borderless"
          loading={loading}
          title="交易账户"
          total={() => (
            <Statistic
              title={null}
              value={data?.accountOnline ?? 0}
              suffix={`/ ${data?.accountTotal ?? 0}`}
            />
          )}
          contentHeight={46}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <TeamOutlined style={{ fontSize: 20, color: '#1890ff' }} />
            <span style={{ color: '#666' }}>上线 / 总数</span>
          </div>
        </ChartCard>
      </Col>

      <Col {...topColResponsiveProps}>
        <ChartCard
          variant="borderless"
          loading={loading}
          title="Bot 实例"
          total={() => (
            <Statistic
              title={null}
              value={data?.botRunning ?? 0}
              suffix={`/ ${data?.botTotal ?? 0}`}
            />
          )}
          contentHeight={46}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <RobotOutlined style={{ fontSize: 20, color: '#52c41a' }} />
            <span style={{ color: '#666' }}>运行中 / 总数</span>
          </div>
        </ChartCard>
      </Col>

      <Col {...topColResponsiveProps}>
        <ChartCard
          variant="borderless"
          title="策略库"
          loading={loading}
          total={() => (
            <Statistic
              title={null}
              value={data?.strategyTotal ?? 0}
              suffix="个"
            />
          )}
          contentHeight={46}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <BookOutlined style={{ fontSize: 20, color: '#1890ff' }} />
            <span style={{ color: '#666' }}>策略总数</span>
          </div>
        </ChartCard>
      </Col>

    </Row>
  );
};

export default IntroduceRow;
