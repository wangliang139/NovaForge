import {
  fetchBotSignalStats,
  fetchDocumentStats,
  fetchLlmCompletionStats,
  fetchStreamStats,
  type BotSignalTypeStats,
  type ChannelDocumentCount,
  type DocumentStats,
  type LlmCompletionStats,
  type LlmSceneStats,
  type StreamStatsItem,
} from '@/services/gateway/dashboard';
import { GetSourceText } from '@/services/gateway/document';
import { getExchangeLogo } from '@/utils/market';
import { GridContent } from '@ant-design/pro-components';
import { Avatar, Card, Col, Row, Statistic, Table, Tag } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { FC, useEffect, useMemo, useState } from 'react';

const Monitor: FC = () => {
  const [loading, setLoading] = useState(false);
  const [llmData, setLlmData] = useState<LlmCompletionStats | null>(null);
  const [docData, setDocData] = useState<DocumentStats | null>(null);
  const [botSignalData, setBotSignalData] = useState<{ stats: BotSignalTypeStats[] } | null>(null);
  const [connectorData, setConnectorData] = useState<StreamStatsItem[] | null>(null);

  useEffect(() => {
    const loadData = async () => {
      setLoading(true);
      const [llm, doc, botSignal, stream] = await Promise.all([
        fetchLlmCompletionStats(1),
        fetchDocumentStats(1),
        fetchBotSignalStats(1),
        fetchStreamStats(1),
      ]);
      setLlmData(llm ?? null);
      setDocData(doc ?? null);
      setBotSignalData(botSignal ?? null);
      setConnectorData(stream ?? null);
      setLoading(false);
    };
    loadData();
    const timer = setInterval(loadData, 60000);
    return () => clearInterval(timer);
  }, []);

  const botSignalDataSorted = useMemo(() => {
    const list = [...(botSignalData?.stats ?? [])];
    return list.sort((a, b) => (b.maxLatencyMs ?? 0) - (a.maxLatencyMs ?? 0));
  }, [botSignalData?.stats]);

  const connectorDataSorted = useMemo(() => {
    const list = [...(connectorData ?? [])];
    return list.sort((a, b) => (b.maxLatencyMs ?? 0) - (a.maxLatencyMs ?? 0));
  }, [connectorData]);

  const docChannelData = useMemo(
    () =>
      [...(docData?.channelCounts ?? [])].sort(
        (a, b) => (b.documentCount ?? 0) - (a.documentCount ?? 0),
      ),
    [docData?.channelCounts],
  );

  const sceneColumns: ColumnsType<LlmSceneStats> = [
    { title: 'Scene', dataIndex: 'sceneKey', key: 'sceneKey' },
    { title: '总调用', dataIndex: 'totalCount', key: 'totalCount', align: 'right' },
    { title: '成功', dataIndex: 'successCount', key: 'successCount', align: 'right' },
    { title: '失败', dataIndex: 'failCount', key: 'failCount', align: 'right' },
    {
      title: '成功率',
      dataIndex: 'successRate',
      key: 'successRate',
      align: 'right',
      render: (v: number) => `${v.toFixed(2)}%`,
    },
    {
      title: '平均耗时(ms)',
      dataIndex: 'avgDurationMs',
      key: 'avgDurationMs',
      align: 'right',
      render: (v: number) => v.toFixed(0),
    },
  ];

  return (
    <GridContent>
      <Row gutter={24}>
      <Col xl={24} lg={24} md={24} sm={24} xs={24} style={{ marginBottom: 24 }}>
          <Card title="Bot Signal 监控" variant="borderless">
            <div style={{ marginTop: 0 }}>
              <h4>按 Bot/Stream 统计（1h 窗口）</h4>
              <Table<BotSignalTypeStats>
                loading={loading}
                dataSource={botSignalDataSorted}
                columns={[
                  { title: 'Bot ID', dataIndex: 'botId', key: 'botId', width: 80 },
                  {
                    title: 'Stream',
                    dataIndex: 'stream',
                    key: 'stream',
                    render: (v: string) => <Tag>{v}</Tag>,
                  },
                  {
                    title: '事件数',
                    dataIndex: 'eventCount',
                    key: 'eventCount',
                    align: 'right',
                    sorter: (a, b) => (a.eventCount ?? 0) - (b.eventCount ?? 0),
                  },
                  {
                    title: '平均延迟(ms)',
                    dataIndex: 'avgLatencyMs',
                    key: 'avgLatencyMs',
                    align: 'right',
                    render: (v: number) => v?.toFixed(0) ?? '-',
                    sorter: (a, b) => (a.avgLatencyMs ?? 0) - (b.avgLatencyMs ?? 0),
                  },
                  {
                    title: '最大延迟(ms)',
                    dataIndex: 'maxLatencyMs',
                    key: 'maxLatencyMs',
                    align: 'right',
                    defaultSortOrder: 'descend',
                    render: (v: number) => v?.toFixed(0) ?? '-',
                    sorter: (a, b) => (a.maxLatencyMs ?? 0) - (b.maxLatencyMs ?? 0),
                  },
                ]}
                rowKey={(r) => `${r.botId}-${r.stream}`}
                pagination={false}
                size="small"
                scroll={{ y: 200 }}
              />
            </div>
          </Card>
        </Col>
        <Col xl={24} lg={24} md={24} sm={24} xs={24} style={{ marginBottom: 24 }}>
          <Card title="数据流 Connector 监控" variant="borderless">
            <div style={{ marginTop: 0 }}>
              <h4>按 Exchange/Stream 统计（1h 窗口）</h4>
              <Table<StreamStatsItem>
                loading={loading}
                dataSource={connectorDataSorted}
                columns={[
                  {
                    title: 'Exchange',
                    dataIndex: 'exchange',
                    key: 'exchange',
                    align: 'center',
                    width: 100,
                    render: (v: string) => (
                      <Avatar src={getExchangeLogo(v)} size={24} shape="square" />
                    ),
                  },
                  {
                    title: 'Stream',
                    dataIndex: 'stream',
                    key: 'stream',
                    render: (v: string) => <Tag>{v}</Tag>,
                  },
                  {
                    title: '事件数',
                    dataIndex: 'eventCount',
                    key: 'eventCount',
                    align: 'right',
                    sorter: (a, b) => (a.eventCount ?? 0) - (b.eventCount ?? 0),
                  },
                  {
                    title: '平均延迟(ms)',
                    dataIndex: 'avgLatencyMs',
                    key: 'avgLatencyMs',
                    align: 'right',
                    render: (v: number) => v?.toFixed(0) ?? '-',
                    sorter: (a, b) => (a.avgLatencyMs ?? 0) - (b.avgLatencyMs ?? 0),
                  },
                  {
                    title: '最大延迟(ms)',
                    dataIndex: 'maxLatencyMs',
                    key: 'maxLatencyMs',
                    align: 'right',
                    defaultSortOrder: 'descend',
                    render: (v: number) => v?.toFixed(0) ?? '-',
                    sorter: (a, b) => (a.maxLatencyMs ?? 0) - (b.maxLatencyMs ?? 0),
                  },
                  {
                    title: '重连次数',
                    dataIndex: 'reconnectCount',
                    key: 'reconnectCount',
                    align: 'right',
                    sorter: (a, b) => (a.reconnectCount ?? 0) - (b.reconnectCount ?? 0),
                  },
                ]}
                rowKey={(r) => `${r.exchange}-${r.stream}`}
                pagination={false}
                size="small"
                scroll={{ y: 200 }}
              />
            </div>
          </Card>
        </Col>
        <Col xl={24} lg={24} md={24} sm={24} xs={24} style={{ marginBottom: 24 }}>
          <Card title="LLM 监控" variant="borderless">
            <Row gutter={24}>
              <Col md={6} sm={12} xs={24}>
                <Statistic
                  title="调用成功率"
                  loading={loading}
                  value={llmData?.successRate ?? 0}
                  precision={2}
                  suffix="%"
                />
              </Col>
              <Col md={6} sm={12} xs={24}>
                <Statistic
                  title="失败次数"
                  loading={loading}
                  value={llmData?.failCount ?? 0}
                />
              </Col>
              <Col md={6} sm={12} xs={24}>
                <Statistic
                  title="总调用数"
                  loading={loading}
                  value={llmData?.totalCount ?? 0}
                />
              </Col>
              <Col md={6} sm={12} xs={24}>
                <Statistic
                  title="成功次数"
                  loading={loading}
                  value={llmData?.successCount ?? 0}
                />
              </Col>
            </Row>
            <div style={{ marginTop: 24 }}>
              <h4>按 Scene 统计</h4>
              <Table<LlmSceneStats>
                loading={loading}
                dataSource={llmData?.sceneStats ?? []}
                columns={sceneColumns}
                rowKey="sceneKey"
                pagination={false}
                size="small"
              />
            </div>
          </Card>
        </Col>
        <Col xl={24} lg={24} md={24} sm={24} xs={24} style={{ marginBottom: 24 }}>
          <Card title="市场资讯监控" variant="borderless">
            <Row gutter={24}>
              <Col md={6} sm={12} xs={24}>
                <Statistic
                  title="处理成功率"
                  loading={loading}
                  value={docData?.stats?.successRate ?? 0}
                  precision={2}
                  suffix="%"
                />
              </Col>
              <Col md={6} sm={12} xs={24}>
                <Statistic
                  title="成功数/总数"
                  loading={loading}
                  value={`${docData?.stats?.successCount ?? 0} / ${docData?.stats?.totalCount ?? 0}`}
                />
              </Col>
              <Col md={6} sm={12} xs={24}>
                <Statistic
                  title="发布→入库(秒)"
                  loading={loading}
                  value={docData?.stats?.avgPublishToIngestSec ?? 0}
                  precision={1}
                />
              </Col>
              <Col md={6} sm={12} xs={24}>
                <Statistic
                  title="入库→处理(秒)"
                  loading={loading}
                  value={docData?.stats?.avgIngestToSuccessSec ?? 0}
                  precision={1}
                />
              </Col>
            </Row>
            <div style={{ marginTop: 24 }}>
              <h4>按频道文档数</h4>
              <Table<ChannelDocumentCount>
                loading={loading}
                dataSource={docChannelData}
                columns={[
                  {
                    title: 'Source',
                    dataIndex: 'source',
                    key: 'source',
                    render: (v: string) => GetSourceText(v),
                  },
                  {
                    title: 'Provider',
                    dataIndex: 'provider',
                    key: 'provider',
                    render: (v: string) => <Tag>{v}</Tag>,
                  },
                  {
                    title: '总文档数',
                    dataIndex: 'documentCount',
                    key: 'documentCount',
                    align: 'right',
                  },
                  {
                    title: '成功数',
                    dataIndex: 'successCount',
                    key: 'successCount',
                    align: 'right',
                  },
                ]}
                rowKey={(r) => `${r.source}-${r.provider}`}
                pagination={false}
                size="small"
                scroll={{ y: 200 }}
              />
            </div>
          </Card>
        </Col>
      </Row>
    </GridContent>
  );
};

export default Monitor;
