import { MarketType } from '@/global.types';
import { PositionSide } from '@/services/gateway/account';
import {
  Bot,
  BotLog,
  BotSignalRecord,
  BotState,
  calculateBotHealth,
  EventKindOptions,
  queryBot,
  queryBotLogs,
  queryBotSignalFlow,
  queryBotState,
  queryStrategy,
  SignalScope,
  SignalTypeOptions,
  Strategy,
} from '@/services/gateway/strategy';
import utils from '@/utils';
import {
  ClockCircleOutlined,
  InfoCircleTwoTone,
  PlayCircleOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import {
  Alert,
  Button,
  Card,
  Col,
  Divider,
  Empty,
  Flex,
  List,
  Modal,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  theme,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import type { FC } from 'react';
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';

interface BotDebugModalProps {
  botId: number;
  visible: boolean;
  onClose: () => void;
}

const LIST_ITEM_STYLE_BASE = { cursor: 'pointer' as const };
const LIST_ITEM_STYLE_SELECTED = { ...LIST_ITEM_STYLE_BASE, backgroundColor: 'var(--ant-color-primary-bg)' };
const TEXT_STYLE_12 = { fontSize: 12 };

const EventListItem = memo(function EventListItem({
  item,
  isSelected,
  onSelect,
  onOpenDetail,
}: {
  item: BotSignalRecord;
  isSelected: boolean;
  onSelect: (item: BotSignalRecord) => void;
  onOpenDetail: (item: BotSignalRecord) => void;
}) {
  return (
    <List.Item
      style={isSelected ? LIST_ITEM_STYLE_SELECTED : LIST_ITEM_STYLE_BASE}
      onClick={() => onSelect(item)}
      onDoubleClick={() => onOpenDetail(item)}
    >
      <List.Item.Meta
        title={
          <Flex justify="space-between" align="center">
            <Space size={8} wrap>
              <Tag color="green">{item.eventKind}</Tag>
              <Typography.Text style={TEXT_STYLE_12} type="secondary">
                {dayjs(item.tsMs).format('HH:mm:ss.SSS')}
              </Typography.Text>
              <Typography.Text style={TEXT_STYLE_12} type="warning">
                {dayjs(item.receiveAtMs).format('HH:mm:ss.SSS')}
              </Typography.Text>
              <Typography.Text style={TEXT_STYLE_12} type="success">
                {dayjs(item.ingestAtMs).format('HH:mm:ss.SSS')}
              </Typography.Text>
            </Space>
            <Space size={4}>
              <ClockCircleOutlined style={TEXT_STYLE_12} />
              <Typography.Text style={TEXT_STYLE_12} type="secondary">
                {item.ingestAtMs - item.tsMs}ms
              </Typography.Text>
            </Space>
          </Flex>
        }
        description={
          <Typography.Text ellipsis style={TEXT_STYLE_12}>
            {item.topic || '-'}
          </Typography.Text>
        }
      />
    </List.Item>
  );
});

const LogListItem = memo(function LogListItem({
  item,
  isSelected,
  onSelect,
  onOpenDetail,
}: {
  item: BotLog;
  isSelected: boolean;
  onSelect: (item: BotLog) => void;
  onOpenDetail: (item: BotLog) => void;
}) {
  const levelColor =
    item.level === 'error' ? 'red' : item.level === 'warn' ? 'orange' : item.level === 'info' ? 'blue' : 'default';
  return (
    <List.Item
      style={isSelected ? LIST_ITEM_STYLE_SELECTED : LIST_ITEM_STYLE_BASE}
      onClick={() => onSelect(item)}
      onDoubleClick={() => onOpenDetail(item)}
    >
      <List.Item.Meta
        title={
          <Flex justify="space-between" align="center">
            <Space size={8} wrap>
              <Tag color={levelColor}>{item.level}</Tag>
              <Typography.Text style={TEXT_STYLE_12} type="secondary">
                {dayjs(item.ts).format('HH:mm:ss.SSS')}
              </Typography.Text>
            </Space>
          </Flex>
        }
        description={
          <Typography.Text ellipsis style={TEXT_STYLE_12}>
            {item.message || '-'}
          </Typography.Text>
        }
      />
    </List.Item>
  );
});

const BotDebugModal: FC<BotDebugModalProps> = ({ botId, visible, onClose }) => {
  const { token } = theme.useToken();
  const [polling, setPolling] = useState(false);
  // 仅用于 footer「已运行」展示，仅在点击「开始」时设置
  const [runStartedAt, setRunStartedAt] = useState<number>(0);
  // 数据拉取起点，每次轮询结束后刷新，避免累积导致轮询过重
  const [dataStartTs, setDataStartTs] = useState<number>(0);
  const [signalStartId, setSignalStartId] = useState<string | undefined>();
  const [nowTs, setNowTs] = useState<number>(() => Date.now());

  const [events, setEvents] = useState<BotSignalRecord[]>([]);
  const [selectedEvent, setSelectedEvent] = useState<BotSignalRecord | null>(null);
  const [eventKindFilter, setEventKindFilter] = useState<string[]>([]);

  const [logs, setLogs] = useState<BotLog[]>([]);
  const [selectedLog, setSelectedLog] = useState<BotLog | null>(null);

  const [portfolio, setPortfolio] = useState<BotState | null>(null);
  const [portfolioLoading, setPortfolioLoading] = useState(false);
  const [portfolioErr, setPortfolioErr] = useState<string | null>(null);

  const [bot, setBot] = useState<Bot | null>(null);
  const [strategy, setStrategy] = useState<Strategy | null>(null);
  const [strategyLoading, setStrategyLoading] = useState(false);

  const [detailType, setDetailType] = useState<'signal' | 'log' | null>(null);

  const pollingInFlightRef = useRef(false);
  const portfolioInFlightRef = useRef(false);
  const pollOnceRef = useRef<() => Promise<void>>(() => Promise.resolve());

  const fetchPortfolio = useCallback(async (options?: { silent?: boolean }) => {
    if (!botId) return;
    if (portfolioInFlightRef.current) return;
    portfolioInFlightRef.current = true;
    if (!options?.silent) {
      setPortfolioLoading(true);
      setPortfolioErr(null);
    }
    try {
      const resp = await queryBotState(botId);
      if (resp) {
        setPortfolio(resp);
      }
    } catch (e: any) {
      if (!options?.silent) {
        setPortfolioErr(e?.message || '获取 bot state 失败');
      }
    } finally {
      if (!options?.silent) {
        setPortfolioLoading(false);
      }
      portfolioInFlightRef.current = false;
    }
  }, [botId]);

  const handleStart = useCallback(() => {
    const now = Date.now();
    setRunStartedAt(now);
    setDataStartTs(now);
    setSignalStartId(undefined);
    setEvents([]);
    setLogs([]);
    setSelectedEvent(null);
    setSelectedLog(null);
    setEventKindFilter([]);
    setPolling(true);
    fetchPortfolio({ silent: true });
  }, [fetchPortfolio]);

  const handlePause = useCallback(() => {
    setPolling(false);
  }, []);

  const handleClear = useCallback(() => {
    // 清空后不应再把历史数据重新“拉回来”
    // - 重置数据起点：下一次轮询从“清空时刻”开始拉取
    // - 重置 signal 游标：让 signal flow 从新的起点重新建立游标
    // 不重置 eventKindFilter，保留用户选择的事件类型筛选
    const now = Date.now();
    setDataStartTs(now);
    setSignalStartId(undefined);
    setEvents([]);
    setLogs([]);
    setSelectedEvent(null);
    setSelectedLog(null);
  }, []);

  const handleSelectEvent = useCallback((item: BotSignalRecord) => {
    setSelectedEvent(item);
  }, []);
  const handleOpenEventDetail = useCallback((item: BotSignalRecord) => {
    setSelectedEvent(item);
    setDetailType('signal');
  }, []);
  const handleSelectLog = useCallback((item: BotLog) => {
    setSelectedLog(item);
  }, []);
  const handleOpenLogDetail = useCallback((item: BotLog) => {
    setSelectedLog(item);
    setDetailType('log');
  }, []);

  const pollOnce = async () => {
    if (!botId || !polling) return;
    if (pollingInFlightRef.current) return;
    pollingInFlightRef.current = true;

    try {
      const now = Date.now();

      // ---- signals ----
      const startTsMs = signalStartId ? undefined : dataStartTs || now;
      const startId = signalStartId ? Number(signalStartId) : undefined;
      const sigResp = await queryBotSignalFlow({
        botId,
        startTsMs,
        startId: Number.isFinite(startId) ? startId : undefined,
        limit: 200,
      });
      if (sigResp?.events?.length) {
        const next: BotSignalRecord[] = [];
        for (const e of sigResp.events) {
          if (!e?.id) continue;
          if (eventKindFilter.length > 0 && !eventKindFilter.includes(e.eventKind ?? '')) continue;
          next.unshift(e);
        }
        if (next.length) {
          setEvents((prev) => [...next, ...prev].slice(0, 500));
        }
      }
      if (sigResp?.nextId && Number(sigResp.nextId) > 0) {
        setSignalStartId(sigResp.nextId);
      }

      // ---- logs ----
      const logResp = await queryBotLogs({
        botId,
        limit: 200,
        startTs: dataStartTs || now,
        endTs: now,
      });
      if (logResp?.list?.length) {
        const nextLogs: BotLog[] = [];
        for (const l of logResp.list) {
          if (!l?.id) continue;
          nextLogs.push(l);
        }
        if (nextLogs.length) {
          setLogs((prev) => [...prev, ...nextLogs].slice(-500));
        }
      }

      // ---- portfolio ----
      await fetchPortfolio({ silent: true });

      // 每次轮询结束后刷新起点，下次只拉增量，避免数据累积导致轮询过重
      setDataStartTs(Date.now());
    } finally {
      pollingInFlightRef.current = false;
    }
  };

  pollOnceRef.current = pollOnce;

  useEffect(() => {
    if (!polling) return;
    const timer = setInterval(() => {
      pollOnceRef.current?.();
    }, 2000);
    return () => clearInterval(timer);
  }, [polling, botId]);

  useEffect(() => {
    if (!polling) return;
    setNowTs(Date.now());
    const timer = setInterval(() => setNowTs(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [polling, runStartedAt]);

  useEffect(() => {
    if (!visible) return;
    fetchPortfolio();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, botId]);

  // 打开调试弹窗时拉取 Bot 及其策略
  useEffect(() => {
    if (!visible || !botId) {
      setBot(null);
      setStrategy(null);
      return;
    }
    let cancelled = false;
    setStrategyLoading(true);
    setBot(null);
    setStrategy(null);
    queryBot(botId)
      .then((b) => {
        if (!b || cancelled) return;
        setBot(b);
        if (!b.strategyId) return;
        return queryStrategy(b.strategyId);
      })
      .then((s) => {
        if (!cancelled && s) setStrategy(s);
      })
      .finally(() => {
        if (!cancelled) setStrategyLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [visible, botId]);

  const handleClose = useCallback(() => {
    setPolling(false);
    handleClear();
    setPortfolio(null);
    setPortfolioErr(null);
    setPortfolioLoading(false);
    setBot(null);
    setStrategy(null);
    setRunStartedAt(0);
    setDataStartTs(0);
    setSignalStartId(undefined);
    onClose();
  }, [handleClear, onClose]);

  // Bot 配置：解析 params 和 config.signals
  const botParams = useMemo(() => {
    if (!bot?.config) return null;
    try {
      console.log('bot.config', bot.config);
      const parsed = JSON.parse(bot.config) as Record<string, unknown>;
      return parsed.params ? parsed.params : null;
    } catch {
      return null;
    }
  }, [bot?.config]);

  const botSignals = useMemo(() => {
    if (!bot?.config) return null;
    try {
      console.log('bot.config', bot.config);
      const parsed = JSON.parse(bot.config) as {
        signals?: Array<{ signalId: string; exchange?: string; symbol?: string }>;
      };
      return parsed.signals?.length ? parsed.signals : null;
    } catch {
      return null;
    }
  }, [bot?.config]);

  const eventKindOptions = useMemo(() => {
    return EventKindOptions;
  }, []);

  const sortedLogs = useMemo(() => {
    // 按时间倒排，最新在最上
    return [...logs].sort((a, b) => (b.ts || 0) - (a.ts || 0));
  }, [logs]);

  const portfolioAssets = useMemo(() => {
    const list = portfolio?.portfolio?.assets || [];
    // 轮询刷新时，后端返回顺序可能变化；这里固定排序，避免 UI 顺序"跳动"
    return [...list].sort((a, b) => {
      const ax = String(a?.exchange ?? '');
      const bx = String(b?.exchange ?? '');
      if (ax !== bx) return ax.localeCompare(bx);

      const aw = String(a?.walletType ?? '');
      const bw = String(b?.walletType ?? '');
      if (aw !== bw) return aw.localeCompare(bw);

      const aa = String(a?.asset ?? '');
      const ba = String(b?.asset ?? '');
      return aa.localeCompare(ba);
    });
  }, [portfolio]);

  const portfolioPositions = useMemo(() => {
    const list = portfolio?.portfolio?.positions || [];
    // 轮询刷新时固定排序，避免顺序错乱
    return [...list].sort((a, b) => {
      const ax = String(a?.exchange ?? '');
      const bx = String(b?.exchange ?? '');
      if (ax !== bx) return ax.localeCompare(bx);

      const am = String(a?.marketType ?? '');
      const bm = String(b?.marketType ?? '');
      if (am !== bm) return am.localeCompare(bm);

      const as = String(a?.symbol ?? '');
      const bs = String(b?.symbol ?? '');
      if (as !== bs) return as.localeCompare(bs);

      const aside = String(a?.side ?? '');
      const bside = String(b?.side ?? '');
      return aside.localeCompare(bside);
    });
  }, [portfolio]);

  const renderSignalList = useCallback(() => {
    if (events.length === 0) {
      return (
        <Flex justify="center" align="center" style={{ height: '100%', width: '100%' }}>
          <Empty description="暂无事件" />
        </Flex>
      );
    }
    return (
      <List
        size="small"
        dataSource={events}
        renderItem={(item) => (
          <EventListItem
            key={item.id.toString()}
            item={item}
            isSelected={selectedEvent?.id === item.id}
            onSelect={handleSelectEvent}
            onOpenDetail={handleOpenEventDetail}
          />
        )}
      />
    );
  }, [events, selectedEvent?.id, handleSelectEvent, handleOpenEventDetail]);

  const simplifySymbolName = useCallback((symbol: string) => {
    return symbol.split(':')[0];
  }, []);

  const renderSignalDetail = () => {
    if (!selectedEvent) return null;
    let payload: string = selectedEvent.payloadJson || '';
    try {
      payload = JSON.stringify(JSON.parse(payload), null, 2);
    } catch {
      // ignore
    }

    return (
      <Space direction="vertical" style={{ width: '100%' }} size="small">
        <div>
          <Typography.Text strong>ID: </Typography.Text>
          <Typography.Text copyable>{selectedEvent.id}</Typography.Text>
        </div>
        <div>
          <Typography.Text strong>Bot: </Typography.Text>
          <Typography.Text>{selectedEvent.botId}</Typography.Text>
        </div>
        <div>
          <Typography.Text strong>账户: </Typography.Text>
          <Typography.Text copyable>{selectedEvent.accountId || '-'}</Typography.Text>
        </div>
        <div>
          <Typography.Text strong>交易所: </Typography.Text>
          <Typography.Text>{selectedEvent.exchange || '-'}</Typography.Text>
        </div>
        <div>
          <Typography.Text strong>类型: </Typography.Text>
          <Tag color="blue">{selectedEvent.stream}</Tag>
          <Tag color="green">{selectedEvent.eventKind}</Tag>
        </div>
        <div>
          <Typography.Text strong>时间: </Typography.Text>
          <Typography.Text type="secondary">
            {dayjs(selectedEvent.tsMs).format('YYYY-MM-DD HH:mm:ss.SSS')}
          </Typography.Text>
        </div>
        <div>
          <Typography.Text strong>接收: </Typography.Text>
          <Typography.Text type="warning">
            {dayjs(selectedEvent.receiveAtMs).format('YYYY-MM-DD HH:mm:ss.SSS')}
          </Typography.Text>
        </div>
        <div>
          <Typography.Text strong>入库: </Typography.Text>
          <Typography.Text type="success">
            {dayjs(selectedEvent.ingestAtMs).format('YYYY-MM-DD HH:mm:ss.SSS')}
          </Typography.Text>
        </div>
        <div>
          <Typography.Text strong>Topic: </Typography.Text>
          <Typography.Text>{selectedEvent.topic || '-'}</Typography.Text>
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
              maxHeight: 320,
              overflowY: 'auto',
            }}
          >
            {payload}
          </pre>
        </div>
      </Space>
    );
  };

  const renderLogList = useCallback(() => {
    if (sortedLogs.length === 0) {
      return (
        <Flex justify="center" align="center" style={{ height: '100%', width: '100%' }}>
          <Empty description="暂无日志" />
        </Flex>
      );
    }
    return (
      <List
        size="small"
        dataSource={sortedLogs}
        renderItem={(item) => (
          <LogListItem
            key={item.id}
            item={item}
            isSelected={selectedLog?.id === item.id}
            onSelect={handleSelectLog}
            onOpenDetail={handleOpenLogDetail}
          />
        )}
      />
    );
  }, [sortedLogs, selectedLog?.id, handleSelectLog, handleOpenLogDetail]);

  const renderLogDetail = () => {
    if (!selectedLog) return null;
    return (
      <Space direction="vertical" style={{ width: '100%' }} size="small">
        <div>
          <Typography.Text strong>ID: </Typography.Text>
          <Typography.Text copyable>{selectedLog.id}</Typography.Text>
        </div>
        <div>
          <Typography.Text strong>级别: </Typography.Text>
          <Tag>{selectedLog.level}</Tag>
        </div>
        <div>
          <Typography.Text strong>时间: </Typography.Text>
          <Typography.Text type="secondary">
            {dayjs(selectedLog.ts).format('YYYY-MM-DD HH:mm:ss.SSS')}
          </Typography.Text>
        </div>
        <Divider style={{ margin: '8px 0' }} />
        <div>
          <Typography.Text strong>消息:</Typography.Text>
          <pre
            style={{
              marginTop: 8,
              padding: 8,
              backgroundColor: token.colorFillAlter,
              borderRadius: 4,
              fontSize: 12,
              maxHeight: 320,
              overflowY: 'auto',
              whiteSpace: 'pre-wrap',
            }}
          >
            {selectedLog.message || '-'}
          </pre>
        </div>
      </Space>
    );
  };

  const modalFooter = useMemo(
    () => [
      <Flex key="footer" justify="space-between" align="center" style={{ width: '100%' }}>
        <Space direction="horizontal" style={{ width: '100%' }} size="large">
          {polling && (
            <Space>
              <ClockCircleOutlined style={{ color: token.colorPrimary }} />
              <Typography.Text type="secondary">
                {` 已运行 ${Math.floor((nowTs - runStartedAt) / 1000)}s`}
              </Typography.Text>
            </Space>
          )}
        </Space>
        <Space>
          <Button
            danger
            onClick={handleClear}
            disabled={events.length === 0 && logs.length === 0 && !selectedEvent && !selectedLog}
          >
            清空
          </Button>
          {!polling ? (
            <Button type="primary" onClick={handleStart} icon={<PlayCircleOutlined />}>
              开始
            </Button>
          ) : (
            <Button onClick={handlePause} icon={<SyncOutlined spin />}>
              暂停
            </Button>
          )}
        </Space>
      </Flex>,
    ],
    [
      polling,
      nowTs,
      runStartedAt,
      events.length,
      logs.length,
      selectedEvent,
      selectedLog,
      handleClear,
      handleStart,
      handlePause,
    ],
  );

  return (
    <Modal
      title={`Bot 调试`}
      open={visible}
      onCancel={handleClose}
      width={1200}
      destroyOnHidden
      footer={modalFooter}
    >
      <Flex vertical gap={12} style={{ width: '100%' }}>
        {/* Bot 运行状态 */}
        {portfolio && (
          <Card
            size="small"
            title="Bot 运行状态"
            style={{ width: '100%' }}
            styles={{ header: { padding: '8px 12px', background: token.colorBgLayout } }}
          >
            {(() => {
              const { score, level } = calculateBotHealth(portfolio);
              const healthColor =
                level === 'excellent'
                  ? 'green'
                  : level === 'good'
                    ? 'blue'
                    : level === 'fair'
                      ? 'orange'
                      : 'red';
              return (
                <>
                  <Row gutter={[16, 16]}>
                    <Col span={4}>
                      <Statistic
                        title="健康度"
                        value={score}
                        valueStyle={{ color: healthColor }}
                        suffix="/ 100"
                      />
                    </Col>
                    <Col span={3}>
                      <Statistic
                        title="Executor"
                        value={
                          portfolio.executorStatus
                            ? utils.text.toUpperFirstLetter(portfolio.executorStatus)
                            : '-'
                        }
                        valueStyle={{
                          color:
                            portfolio.executorStatus === 'running'
                              ? token.colorSuccess
                              : token.colorError,
                        }}
                      />
                    </Col>
                    <Col span={3}>
                      <Statistic
                        title="JS Runner"
                        value={
                          portfolio.jsRunnerStatus
                            ? utils.text.toUpperFirstLetter(portfolio.jsRunnerStatus)
                            : '-'
                        }
                        valueStyle={{
                          color:
                            portfolio.jsRunnerStatus === 'running'
                              ? token.colorSuccess
                              : token.colorError,
                        }}
                      />
                    </Col>
                    <Col span={4}>
                      <Statistic
                        title="Signal 平均耗时"
                        value={
                          portfolio.signalAvgDurationMs !== undefined &&
                            portfolio.signalAvgDurationMs !== null
                            ? portfolio.signalAvgDurationMs
                            : '-'
                        }
                        suffix={
                          portfolio.signalAvgDurationMs !== undefined &&
                            portfolio.signalAvgDurationMs !== null
                            ? 'ms'
                            : ''
                        }
                      />
                    </Col>
                    <Col span={4}>
                      <Statistic
                        title="Signal 平均延迟"
                        value={
                          portfolio.signalAvgLatencyMs !== undefined &&
                            portfolio.signalAvgLatencyMs !== null
                            ? portfolio.signalAvgLatencyMs
                            : '-'
                        }
                        suffix={
                          portfolio.signalAvgLatencyMs !== undefined &&
                            portfolio.signalAvgLatencyMs !== null
                            ? 'ms'
                            : ''
                        }
                      />
                    </Col>
                    <Col span={6}>
                      <Statistic
                        title="最新信号时间"
                        value={
                          portfolio.lastSignalTs
                            ? dayjs(portfolio.lastSignalTs).format('MM-DD HH:mm:ss.SSS')
                            : '-'
                        }
                      />
                    </Col>
                  </Row>
                  {portfolio.runErr && (
                    <Row gutter={[16, 8]} style={{ marginTop: 16 }}>
                      <Col span={24}>
                        <Alert message={portfolio.runErr} type="error" showIcon />
                      </Col>
                    </Row>
                  )}
                </>
              );
            })()}
          </Card>
        )}
        {/* 策略参数与信号数据源（调试用） */}
        <Card
          size="small"
          title="Bot 配置"
          loading={strategyLoading}
          style={{ width: '100%' }}
          styles={{ header: { padding: '8px 12px', background: token.colorBgLayout } }}
        >
          {!strategyLoading && !bot && (
            <Typography.Text type="secondary">无法加载 Bot 信息</Typography.Text>
          )}
          {!strategyLoading && bot && (
            <Row gutter={[24, 16]}>
              <Col span={8}>
                <Typography.Text strong style={{ display: 'block', marginBottom: 8 }}>
                  策略参数
                </Typography.Text>
                {botParams && Object.keys(botParams).length > 0 ? (
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                    {Object.entries(botParams).map(([name, value]) => (
                      <Tag key={name}>
                        {name}
                        <Typography.Text type="secondary" style={{ marginLeft: 4, fontSize: 12 }}>
                          = {String(value)}
                        </Typography.Text>
                      </Tag>
                    ))}
                  </div>
                ) : (
                  <Typography.Text type="secondary">无参数配置</Typography.Text>
                )}
              </Col>
              <Col span={8}>
                <Typography.Text strong style={{ display: 'block', marginBottom: 8 }}>
                  信号数据源
                </Typography.Text>
                {botSignals && botSignals.length > 0 ? (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                    {botSignals.map((bs, idx) => {
                      const sig = strategy?.signals?.find((s) => s.id === bs.signalId);
                      let typeLabel = sig ? String(sig.type) : bs.signalId;
                      if (sig) {
                        for (const g of SignalTypeOptions) {
                          const opts = 'options' in g && Array.isArray(g.options) ? g.options : [g];
                          const found = opts.find(
                            (o: { value?: string }) => 'value' in o && o.value === sig.type,
                          );
                          if (found && typeof found === 'object' && 'label' in found) {
                            typeLabel = (found as { label: string }).label;
                            break;
                          }
                        }
                      }
                      const scopeLabel = sig
                        ? sig.scope === SignalScope.Symbol
                          ? 'Symbol'
                          : sig.scope === SignalScope.Target
                            ? 'Target'
                            : sig.scope === SignalScope.Exchange
                              ? 'Exchange'
                              : 'Strategy'
                        : '-';
                      return (
                        <div
                          key={`${bs.signalId}-${idx}`}
                          style={{ display: 'flex', alignItems: 'center', gap: 2 }}
                        >
                          <Tag color="blue">{bs.signalId}</Tag>
                          <Tag>{typeLabel}</Tag>
                          <Tag color="purple">{scopeLabel}</Tag>
                          {(bs.exchange || bs.symbol) && (
                            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                              {bs.symbol && (
                                <Tag>
                                  <Space>
                                    {bs.exchange && (
                                      <img
                                        alt={bs.exchange}
                                        style={{
                                          display: 'inline',
                                          marginLeft: 0,
                                          paddingBottom: 2,
                                        }}
                                        width={16}
                                        src={utils.market.getExchangeLogo(bs.exchange)}
                                      />
                                    )}
                                    {bs.symbol}
                                  </Space>
                                </Tag>
                              )}
                            </Typography.Text>
                          )}
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <Typography.Text type="secondary">无信号配置</Typography.Text>
                )}
              </Col>
              <Col span={8}>
                <Typography.Text strong style={{ display: 'block', marginBottom: 8 }}>
                  交易对
                </Typography.Text>
                {bot && bot.symbols?.length ? (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        {bot.symbols?.map((s) => (
                          <Tag>
                            <Space>
                              {bot.exchange && (
                                <img
                                  alt={bot.exchange}
                                  style={{
                                    display: 'inline',
                                    marginLeft: 0,
                                    paddingBottom: 2,
                                  }}
                                  width={16}
                                  src={utils.market.getExchangeLogo(bot.exchange)}
                                />
                              )}
                              {s}
                            </Space>
                          </Tag>
                        ))}
                      </Typography.Text>
                    </div>
                  </div>
                ) : (
                  <Typography.Text type="secondary">无信号配置</Typography.Text>
                )}
              </Col>
            </Row>
          )}
        </Card>

        <Row gutter={[16, 16]}>
          <Col span={12}>
            <Card
              size="small"
              style={{
                width: '100%',
                height: 240,
                display: 'flex',
                flexDirection: 'column',
                marginBottom: 6,
              }}
              loading={portfolioLoading}
              styles={{
                header: { padding: '8px 12px', background: token.colorBgLayout },
                body: { padding: 0, flex: 1, overflow: 'hidden' },
              }}
              title={
                <Space size={8}>
                  <Typography.Text strong>Portfolio - 资产</Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {portfolio?.portfolio?.ts
                      ? dayjs(portfolio.portfolio.ts).format('HH:mm:ss')
                      : '-'}
                  </Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {portfolioAssets.length} 条
                  </Typography.Text>
                </Space>
              }
            >
              <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                {portfolioErr && (
                  <div style={{ padding: 8 }}>
                    <Alert type="error" showIcon message={portfolioErr} />
                  </div>
                )}
                <div style={{ flex: 1, overflow: 'hidden' }}>
                  <Table
                    size="small"
                    rowKey={(r) => `${r.exchange}-${r.walletType}-${r.asset}`}
                    dataSource={portfolioAssets}
                    pagination={false}
                    scroll={{ y: 160 }}
                    columns={[
                      {
                        title: '资产',
                        dataIndex: 'asset',
                        align: 'center',
                        width: 80,
                        render: (text) => <Tag>{text}</Tag>,
                      },
                      {
                        title: '钱包',
                        align: 'center',
                        dataIndex: 'walletType',
                        width: 80,
                        render: (text) => (
                          <Tag color="lime">{utils.text.toUpperFirstLetter(text)}</Tag>
                        ),
                      },
                      {
                        title: '可用',
                        dataIndex: 'free',
                        align: 'right',
                        render: (text) => utils.math.formatByPrecision(text, 8),
                      },
                      { title: '冻结', dataIndex: 'frozen', align: 'right', render: (text) => utils.math.formatByPrecision(text, 8) },
                      {
                        title: '时间',
                        dataIndex: 'updatedTs',
                        align: 'center',
                        width: 50,
                        render: (v: number) =>
                          v ? (
                            <Tooltip title={dayjs(v).format('YYYY-MM-DD HH:mm:ss.SSS')}>
                              <InfoCircleTwoTone />
                            </Tooltip>
                          ) : (
                            '-'
                          ),
                      },
                    ]}
                  />
                </div>
              </div>
            </Card>
          </Col>
          <Col span={12}>
            <Card
              size="small"
              style={{ width: '100%', height: 240, display: 'flex', flexDirection: 'column' }}
              loading={portfolioLoading}
              styles={{
                header: { padding: '8px 12px', background: token.colorBgLayout },
                body: { padding: 0, flex: 1, overflow: 'hidden' },
              }}
              title={
                <Space size={8}>
                  <Typography.Text strong>Portfolio - 仓位</Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {portfolioPositions.length} 条
                  </Typography.Text>
                </Space>
              }
            >
              <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                <div style={{ flex: 1, overflow: 'hidden' }}>
                  <Table
                    size="small"
                    rowKey={(r) => `${r.exchange}-${r.marketType}-${r.symbol}-${r.side}`}
                    dataSource={portfolioPositions}
                    pagination={false}
                    scroll={{ y: 160 }}
                    columns={[
                      {
                        title: '标的',
                        dataIndex: 'symbol',
                        align: 'center',
                        width: 120,
                        render: (text) => {
                          const isSpot = text === MarketType.Spot;
                          return (
                            <Tag color={isSpot ? 'blue' : 'orange'}>{simplifySymbolName(text)}</Tag>
                          );
                        },
                      },
                      {
                        title: '方向',
                        align: 'center',
                        dataIndex: 'side',
                        width: 60,
                        render: (text: any) => {
                          const isLong = text === PositionSide.Long;
                          return <Tag color={isLong ? 'green' : 'red'}>{isLong ? '多' : '空'}</Tag>;
                        },
                      },
                      {
                        title: '杠杆',
                        align: 'center',
                        dataIndex: 'leverage',
                        width: 60,
                        render: (v: number) => (v && v > 0 ? `${v}x` : '-'),
                      },
                      {
                        title: '数量',
                        dataIndex: 'qty',
                        render: (text) => utils.math.formatByPrecision(text, 8),
                      },
                      {
                        title: '均价',
                        dataIndex: 'avgPrice',
                        render: (text) => utils.math.formatByPrecision(text, 8),
                      },
                      {
                        title: '时间',
                        dataIndex: 'updatedTs',
                        align: 'center',
                        width: 50,
                        render: (v: number) =>
                          v ? (
                            <Tooltip title={dayjs(v).format('YYYY-MM-DD HH:mm:ss.SSS')}>
                              <InfoCircleTwoTone />
                            </Tooltip>
                          ) : (
                            '-'
                          ),
                      },
                    ]}
                  />
                </div>
              </div>
            </Card>
          </Col>
        </Row>
      </Flex>
      <Flex gap={12} style={{ width: '100%', height: '65vh', minHeight: 520, marginTop: 8 }}>
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
            gap={8}
            style={{
              padding: '8px 12px',
              borderBottom: `1px solid ${token.colorBorderSecondary}`,
              background: token.colorBgLayout,
            }}
          >
            <Typography.Text strong>事件列表</Typography.Text>
            <Space size={8}>
              <Select
                mode="multiple"
                placeholder="事件类型"
                allowClear
                style={{ width: 220 }}
                value={eventKindFilter}
                maxTagCount="responsive"
                onChange={(v) => setEventKindFilter(v ?? [])}
                options={eventKindOptions.map((k) => ({ value: k.value, label: k.label }))}
              />
              <Typography.Text type="secondary">
                {events.length} 条
              </Typography.Text>
            </Space>
          </Flex>
          <div style={{ flex: 1, overflowY: 'auto' }}>{renderSignalList()}</div>
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
            <Typography.Text strong>日志列表</Typography.Text>
            <Typography.Text type="secondary">
              {sortedLogs.length} 条{logs.length >= 500 && ' (最多 500)'}
            </Typography.Text>
          </Flex>
          <div style={{ flex: 1, overflowY: 'auto' }}>{renderLogList()}</div>
        </div>
      </Flex>

      <Modal
        title="事件详情"
        open={detailType === 'signal' && !!selectedEvent}
        footer={null}
        onCancel={() => setDetailType(null)}
        width={800}
        destroyOnHidden
      >
        {selectedEvent && renderSignalDetail()}
      </Modal>

      <Modal
        title="日志详情"
        open={detailType === 'log' && !!selectedLog}
        footer={null}
        onCancel={() => setDetailType(null)}
        width={800}
        destroyOnHidden
      >
        {selectedLog && renderLogDetail()}
      </Modal>
    </Modal>
  );
};

export default BotDebugModal;
