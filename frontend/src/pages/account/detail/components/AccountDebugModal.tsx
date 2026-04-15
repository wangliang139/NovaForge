import {
  EventFlowStream,
  EventRecord,
  queryAccountEventFlow,
} from '@/services/gateway/account';
import utils from '@/utils';
import { ClockCircleOutlined, PlayCircleOutlined, SyncOutlined } from '@ant-design/icons';
import { Button, Divider, Empty, Flex, List, message, Modal, Space, Tag, theme, Typography } from 'antd';
import dayjs from 'dayjs';
import type { FC } from 'react';
import { useEffect, useRef, useState } from 'react';

interface AccountDebugModalProps {
  accountId: string;
  visible: boolean;
  onClose: () => void;
}

const AccountDebugModal: FC<AccountDebugModalProps> = ({ accountId, visible, onClose }) => {
  const { token } = theme.useToken();
  const [polling, setPolling] = useState(false);
  const [sessionStartTs, setSessionStartTs] = useState<number>(0);
  const [currentStartId, setCurrentStartId] = useState<string | undefined>();
  const [nowTs, setNowTs] = useState<number>(() => Date.now());
  const [events, setEvents] = useState<EventRecord[]>([]);
  const [selectedEvent, setSelectedEvent] = useState<EventRecord | null>(null);
  const pollingInFlightRef = useRef(false);

  const handleStart = () => {
    const now = Date.now();
    setSessionStartTs(now);
    setCurrentStartId(undefined);
    setPolling(true);
  };

  const handlePause = () => {
    setPolling(false);
  };

  const handleClear = () => {
    setEvents([]);
    setSelectedEvent(null);
  };

  const pollEvents = async () => {
    if (!accountId || !polling) return;
    if (pollingInFlightRef.current) return;
    pollingInFlightRef.current = true;

    const now = Date.now();
    const tenMinutes = 10 * 60 * 1000;

    try {

      // 后端的 eventflow.Query 在 StartTs 为空时可能触发 nil 指针，所以前端始终传 startTsMs
      const startTsMs = currentStartId ? undefined : (sessionStartTs || now);
      const startId = currentStartId;

      const result = await queryAccountEventFlow(accountId, EventFlowStream.All, startTsMs, startId, 200);
      if (!result) return;

      if (result.events.length > 0) {
        setEvents((prev) => {
          const seen = new Set(prev.map((e) => e.id));
          const next = [...prev];
          for (const e of result.events) {
            if (!seen.has(e.id)) {
              seen.add(e.id);
              next.push(e);
            }
          }
          return next;
        });
      }

      // 推进游标：nextId = lastId + 1；无新事件时 nextId=0，此时不推进
      if (result?.nextId && result.nextId.length > 0 && Number(result.nextId) > 0) {
        setCurrentStartId((prev) => result.nextId);
      }
    } catch (err) {
      message.error(`加载调试事件失败：${err}`);
    } finally {
      pollingInFlightRef.current = false;
    }
  };

  useEffect(() => {
    if (!polling) return;

    const timer = setInterval(() => {
      pollEvents();
    }, 2000);

    return () => clearInterval(timer);
  }, [polling, sessionStartTs, currentStartId, accountId]);

  useEffect(() => {
    if (!polling) return;

    setNowTs(Date.now());
    const timer = setInterval(() => {
      setNowTs(Date.now());
    }, 1000);

    return () => clearInterval(timer);
  }, [polling, sessionStartTs]);

  const handleClose = () => {
    setPolling(false);
    setEvents([]);
    setSelectedEvent(null);
    onClose();
  };

  const renderEventList = (events: EventRecord[]) => {
    if (events.length === 0) {
      return (
        <Flex justify="center" align="center" style={{ height: '100%', width: '100%' }}>
          <Empty description="暂无事件" />
        </Flex>
      );
    }
    const sortedEvents = [...events].sort((a, b) => Number(b.id) - Number(a.id));

    return (
      <List
        size="small"
        dataSource={sortedEvents}
        renderItem={(item) => (
          <List.Item
            key={item.id}
            style={{
              cursor: 'pointer',
              backgroundColor: selectedEvent?.id === item.id ? token.colorPrimaryBg : undefined,
            }}
            onClick={() => setSelectedEvent(item)}
          >
            <List.Item.Meta
              title={
                <Flex justify="space-between" align="center">
                  <Space>
                    <Tag color={item.stream === 'account_raw' ? 'blue' : 'green'}>{item.eventKind}</Tag>
                    <Typography.Text style={{ fontSize: 12 }} type="secondary">
                      {dayjs(item.tsMs).format('HH:mm:ss.SSS')}
                    </Typography.Text>
                    <Typography.Text style={{ fontSize: 12 }} type="warning">
                      {dayjs(item.receiveAtMs).format('HH:mm:ss.SSS')}
                    </Typography.Text>
                    <Typography.Text style={{ fontSize: 12 }} type="success">
                      {dayjs(item.publishAtMs).format('HH:mm:ss.SSS')}
                    </Typography.Text>
                  </Space>
                  <Space size={4}>
                    <ClockCircleOutlined style={{ fontSize: 12 }} />
                    <Typography.Text style={{ fontSize: 12 }} type="secondary">
                      {item.ingestAtMs - item.tsMs}ms
                    </Typography.Text>
                  </Space>
                </Flex>
              }
              description={
                <Typography.Text ellipsis style={{ fontSize: 12 }}>
                  {item.topic}
                </Typography.Text>
              }
            />
          </List.Item>
        )}
      />
    );
  };

  const renderEventDetail = () => {
    if (!selectedEvent) return null;

    return (
      <Space direction="vertical" style={{ width: '100%' }} size="small">
        <div>
          <Typography.Text strong>ID: </Typography.Text>
          <Typography.Text copyable>{selectedEvent.id}</Typography.Text>
        </div>
        <div>
          <Typography.Text strong>类型: </Typography.Text>
          <Tag color={selectedEvent.stream === 'account_raw' ? 'blue' : 'green'}>
            {selectedEvent.eventKind}
          </Tag>
        </div>
        <div>
          <Typography.Text strong>生成时间: </Typography.Text>
          <Typography.Text type="secondary">
            {dayjs(selectedEvent.tsMs).format('YYYY-MM-DD HH:mm:ss.SSS')}
          </Typography.Text>
        </div>
        <div>
          <Typography.Text strong>接收时间: </Typography.Text>
          <Typography.Text type="warning">
            {dayjs(selectedEvent.receiveAtMs).format('YYYY-MM-DD HH:mm:ss.SSS')}
          </Typography.Text>
        </div>
        <div>
          <Typography.Text strong>发布时间: </Typography.Text>
          <Typography.Text type="success">
            {dayjs(selectedEvent.publishAtMs).format('YYYY-MM-DD HH:mm:ss.SSS')}
          </Typography.Text>
        </div>
        <div>
          <Typography.Text strong>交易所: </Typography.Text>
          <Typography.Text>{utils.market.getExchangeTitle(selectedEvent.exchange)}</Typography.Text>
        </div>
        <Divider style={{ margin: '8px 0' }} />
        <div>
          <Typography.Text strong>Payload:</Typography.Text>
          <pre
            style={{
              marginTop: 8,
              padding: 8,
              backgroundColor: token.colorFillAlter,
              borderRadius: 4,
              fontSize: 11,
              maxHeight: 280,
              overflowY: 'auto',
            }}
          >
            {JSON.stringify(JSON.parse(selectedEvent.payloadJson), null, 2)}
          </pre>
        </div>
      </Space>
    );
  };

  const rawCount = events.filter((e) => e.stream === 'account_raw').length;
  const procCount = events.filter((e) => e.stream === 'account').length;

  return (
    <Modal
      title="账户事件监控"
      open={visible}
      onCancel={handleClose}
      width={1200}
      footer={[
        <Flex key="footer" justify="space-between" align="center" style={{ width: '100%' }}>
          <Space direction="horizontal" style={{ width: '100%' }} size="large">
            {polling && (
              <Space>
                <ClockCircleOutlined style={{ color: token.colorPrimary }} />
                <Typography.Text type="secondary" >
                  {` 已运行 ${Math.floor((nowTs - sessionStartTs) / 1000)}s`}
                </Typography.Text>
              </Space>
            )}
          </Space>
          <Space>
            <Button danger onClick={handleClear} disabled={events.length === 0 && !selectedEvent}>
              清空
            </Button>
            {!polling ? (
              <Button type="primary" onClick={handleStart} icon={<PlayCircleOutlined />}>
                开始
              </Button>
            ) : (
              <Button onClick={handlePause} icon={<SyncOutlined spin />}>暂停</Button>
            )}
          </Space>
        </Flex>,
      ]}
    >
      <Flex gap={12} style={{ width: '100%', height: '65vh', minHeight: 520, marginTop: 16 }}>
        <div
          style={{
            flex: '0 0 520px',
            minWidth: 420,
            border: `1px solid ${token.colorBorder}`,
            borderRadius: 4,
            overflow: 'hidden',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <Flex
            justify="space-between"
            align="center"
            style={{
              padding: '8px 12px',
              borderBottom: `1px solid ${token.colorBorderSecondary}`,
              background: token.colorBgLayout,
            }}
          >
            <Typography.Text strong>事件列表</Typography.Text>
            <Space size={8}>
              <Typography.Text type="secondary">{events.length} 条</Typography.Text>
              <Tag color="blue">Ingested {rawCount}</Tag>
              <Tag color="green">Published {procCount}</Tag>
            </Space>
          </Flex>
          <div style={{ flex: 1, overflowY: 'auto' }}>{renderEventList(events)}</div>
        </div>
        <div
          style={{
            flex: '1 1 0',
            minWidth: 420,
            border: `1px solid ${token.colorBorder}`,
            borderRadius: 4,
            overflow: 'hidden',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <Flex
            justify="space-between"
            align="center"
            style={{
              padding: '8px 12px',
              borderBottom: `1px solid ${token.colorBorderSecondary}`,
              background: token.colorBgLayout,
            }}
          >
            <Typography.Text strong>事件详情</Typography.Text>
          </Flex>
          <div style={{ flex: 1, overflowY: 'auto', padding: 12 }}>
            {selectedEvent ? (
              renderEventDetail()
            ) : (
              <Flex justify="center" align="center" style={{ height: '100%', width: '100%' }}>
                <Empty description="请选择一条事件查看详情" />
              </Flex>
            )}
          </div>
        </div>
      </Flex>
    </Modal>
  );
};

export default AccountDebugModal;
