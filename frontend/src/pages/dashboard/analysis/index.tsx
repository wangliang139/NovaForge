import { GridContent } from '@ant-design/pro-components';
import { Card, Col, Row } from 'antd';
import { FC, Suspense, useEffect, useState } from 'react';
import AccountProfitRankTable from './components/AccountProfitRankTable';
import BotProfitRankTable from './components/BotProfitRankTable';
import IntroduceRow from './components/IntroduceRow';
import PageLoading from './components/PageLoading';
import { fetchAnalysisData, type AnalysisData } from './service';

const Analysis: FC = () => {
  const [loading,setLoading] = useState(false);
  const [data,setData] = useState<AnalysisData | null>(null);

  useEffect(() => {
    const loadData = async () => {
      setLoading(true);
      const data = await fetchAnalysisData();
      setData(data);
      setLoading(false);
    };
    loadData();
  }, []);

  return (
    <GridContent>
      <Suspense fallback={<PageLoading />}>
        <IntroduceRow
          loading={loading}
          data={data?.overview ?? null}
          totalAccountNotional={data?.totalAccountNotional}
          totalAccount24hChange={data?.totalAccount24hChange}
        />
      </Suspense>

      <Row gutter={24} style={{ marginTop: 24 }}>
      <Col xl={12} lg={24} md={24} sm={24} xs={24}>
          <Card title="账户收益排行" variant="borderless">
            <AccountProfitRankTable loading={loading} data={data?.accountProfitRank ?? []} />
          </Card>
        </Col>
        <Col xl={12} lg={24} md={24} sm={24} xs={24}>
          <Card title="Bot 收益排行" variant="borderless">
            <BotProfitRankTable loading={loading} data={data?.botProfitRank ?? []} />
          </Card>
        </Col>
      </Row>
    </GridContent>
  );
};

export default Analysis;
