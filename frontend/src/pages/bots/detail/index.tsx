import AssetsTable from '@/components/Market/AssetsTable';
import { KlineChartPro } from '@/components/Market/KlineChartPro';
import LedgersTable from '@/components/Market/LedgersTable';
import OrdersTable from '@/components/Market/OrdersTable';
import PositionsTable from '@/components/Market/PositionsTable';
import { Exchange } from '@/global.types';
import StrategyModal from '@/pages/strategy/components/StrategyModal';
import { api } from '@/services/gateway';
import { AccountEquity, Asset, Position } from '@/services/gateway/account';
import { MarketInfo } from '@/services/gateway/market';
import {
  Bot,
  BotLog,
  BotState,
  BotStatus,
  BotStatusOptions,
  calculateBotHealth,
  queryBot,
  queryBotBalance,
  queryBotEquity,
  queryBotLedger,
  queryBotLogs,
  queryBotMetrics,
  queryBotOrders,
  queryBotPositions,
  queryBotState,
  queryStrategy,
  startBot,
  stopBot,
  Strategy,
} from '@/services/gateway/strategy';
import utils from '@/utils';
import {
  BarChartOutlined,
  BugOutlined,
  DesktopOutlined,
  DollarOutlined,
  FallOutlined,
  LineChartOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RiseOutlined,
  TrophyOutlined,
} from '@ant-design/icons';
import { PageContainer, ProDescriptions } from '@ant-design/pro-components';
import { Helmet, history, useParams } from '@umijs/max';
import {
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Empty,
  Flex,
  List,
  message,
  Modal,
  Row,
  Segmented,
  Select,
  Space,
  Spin,
  Statistic,
  Tag,
  theme,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  CartesianGrid,
  Line,
  LineChart,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  XAxis,
  YAxis,
} from 'recharts';
import BotDebugModal from './components/BotDebugModal';

const toMsIfSeconds = (ts: number) => (ts < 1e12 ? ts * 1000 : ts);

const BotDetailPage: React.FC = () => {
  const { token } = theme.useToken();
  const { id } = useParams<{ id: string }>();
  const botId = Number(id);

  const [bot, setBot] = useState<Bot | null>(null);
  const [botState, setBotState] = useState<BotState | null>(null);
  const [loading, setLoading] = useState(false);

  const [balance, setBalance] = useState<{
    notional: string;
    unRealizedProfit?: string;
    notional24HChange?: string;
  } | null>(null);
  const [assets, setAssets] = useState<Asset[]>([]);
  const [assetsLoading, setAssetsLoading] = useState(false);

  const [positions, setPositions] = useState<Position[]>([]);
  const [positionsLoading, setPositionsLoading] = useState(false);

  const [equityRange, setEquityRange] = useState<'1d' | '7d' | '30d'>('7d');
  const [equityLoading, setEquityLoading] = useState(false);
  const [equity, setEquity] = useState<AccountEquity[]>([]);
  const [metricsLoading, setMetricsLoading] = useState(false);
  const [botMetrics, setBotMetrics] = useState<Awaited<ReturnType<typeof queryBotMetrics>>>(null);

  const [debugVisible, setDebugVisible] = useState(false);
  const [operating, setOperating] = useState(false);
  const botIsRunning = bot?.status === 'running';
  const debugDisabled = !botIsRunning;

  const [klineVisible, setKlineVisible] = useState(false);
  const [klineSymbol, setKlineSymbol] = useState<string>('');
  const [klineMarketInfo, setKlineMarketInfo] = useState<MarketInfo | null>(null);

  const [logs, setLogs] = useState<BotLog[]>([]);
  const [logsNextCursor, setLogsNextCursor] = useState<string | undefined>();
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsHasMore, setLogsHasMore] = useState(true);
  const [logsLevelFilter, setLogsLevelFilter] = useState<string>('');
  const [selectedLog, setSelectedLog] = useState<BotLog | null>(null);
  const [logDetailVisible, setLogDetailVisible] = useState(false);

  const [strategyModalOpen, setStrategyModalOpen] = useState(false);
  const [strategyDetail, setStrategyDetail] = useState<Strategy | null>(null);
  const [strategyLoading, setStrategyLoading] = useState(false);

  const LOG_LEVEL_OPTIONS = [
    { label: '全部', value: '' },
    { label: 'Error', value: 'error' },
    { label: 'Warn', value: 'warn' },
    { label: 'Info', value: 'info' },
    { label: 'Debug', value: 'debug' },
  ];
  const logsScrollRef = useRef<HTMLDivElement>(null);
  const logsSentinelRef = useRef<HTMLDivElement>(null);
  const logsLoadingRef = useRef(false);

  const loadLogs = useCallback(
    async (cursor?: string) => {
      if (!Number.isFinite(botId) || botId <= 0 || logsLoadingRef.current) return;
      logsLoadingRef.current = true;
      setLogsLoading(true);
      try {
        const resp = await queryBotLogs({
          botId,
          limit: 50,
          cursor,
          level: logsLevelFilter || undefined,
        });
        const list = resp?.list || [];
        if (cursor) {
          setLogs((prev) => [...prev, ...list]);
        } else {
          setLogs(list);
        }
        setLogsNextCursor(resp?.nextCursor);
        setLogsHasMore(!!resp?.nextCursor && list.length > 0);
      } catch (err) {
        message.error(`加载 Bot 日志失败：${err}`);
        setLogsHasMore(false);
      } finally {
        logsLoadingRef.current = false;
        setLogsLoading(false);
      }
    },
    [botId, logsLevelFilter],
  );

  const handleOpenStrategyModal = useCallback(async () => {
    if (!bot?.strategyId) {
      message.warning('该实例未关联策略');
      return;
    }
    if (strategyLoading) return;
    setStrategyLoading(true);
    try {
      const data = await queryStrategy(bot.strategyId);
      if (!data) {
        message.error('未查询到策略详情');
        return;
      }
      setStrategyDetail(data);
      setStrategyModalOpen(true);
    } catch (err) {
      message.error(`加载策略详情失败：${err}`);
    } finally {
      setStrategyLoading(false);
    }
  }, [bot?.strategyId, strategyLoading]);

  useEffect(() => {
    if (!Number.isFinite(botId) || botId <= 0) return;
    setLogs([]);
    setLogsNextCursor(undefined);
    setLogsHasMore(true);
    loadLogs();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [botId, logsLevelFilter]);

  useEffect(() => {
    if (!logsSentinelRef.current || !logsScrollRef.current || !logsHasMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && logsHasMore && !logsLoadingRef.current) {
          loadLogs(logsNextCursor);
        }
      },
      { root: logsScrollRef.current, rootMargin: '100px', threshold: 0 },
    );
    observer.observe(logsSentinelRef.current);
    return () => observer.disconnect();
  }, [logsHasMore, logsNextCursor, loadLogs]);

  const openKlineModal = (symbol: string) => {
    const ex = bot?.exchange as Exchange | undefined;
    if (!ex) {
      message.error('Bot 交易所缺失，无法查询 K 线');
      return;
    }
    if (!symbol) return;

    setKlineVisible(true);
    setKlineSymbol(symbol);
    setKlineMarketInfo(null);
  };

  useEffect(() => {
    if (!klineVisible || !klineSymbol) return;
    const ex = bot?.exchange;
    if (!ex) return;

    let cancelled = false;
    api
      .queryMarket({ exchange: String(ex), symbol: klineSymbol })
      .then((resp) => {
        if (!cancelled) setKlineMarketInfo((resp as MarketInfo) || null);
      })
      .catch(() => {
        if (!cancelled) setKlineMarketInfo(null);
      });
    return () => {
      cancelled = true;
    };
  }, [klineVisible, klineSymbol, bot?.exchange]);

  const loadBot = async () => {
    if (!Number.isFinite(botId) || botId <= 0) {
      setBot(null);
      return;
    }
    setLoading(true);
    try {
      const data = await queryBot(botId);
      setBot(data || null);
      // 同时加载 bot 状态
      try {
        const state = await queryBotState(botId);
        setBotState(state || null);
      } catch {
        setBotState(null);
      }
    } catch (err) {
      message.error(`加载 Bot 信息失败：${err}`);
      setBot(null);
    } finally {
      setLoading(false);
    }
  };

  const loadBalance = async () => {
    if (!Number.isFinite(botId) || botId <= 0) return;
    setAssetsLoading(true);
    try {
      const resp = await queryBotBalance(botId);
      setBalance(
        resp
          ? {
              notional: resp.notional,
              unRealizedProfit: resp.unRealizedProfit,
              notional24HChange: resp.notional24HChange,
            }
          : null,
      );
      setAssets(resp?.assets || []);
    } catch (err) {
      message.error(`加载资产失败：${err}`);
      setBalance(null);
      setAssets([]);
    } finally {
      setAssetsLoading(false);
    }
  };

  const loadPositions = async () => {
    if (!Number.isFinite(botId) || botId <= 0) return;
    setPositionsLoading(true);
    try {
      const list = await queryBotPositions(botId);
      setPositions(list || []);
    } catch (err) {
      message.error(`加载仓位失败：${err}`);
      setPositions([]);
    } finally {
      setPositionsLoading(false);
    }
  };

  const loadEquity = async () => {
    if (!Number.isFinite(botId) || botId <= 0) return;
    setEquityLoading(true);
    try {
      const endTs = dayjs().valueOf();
      const rangeMs =
        equityRange === '1d'
          ? 24 * 60 * 60 * 1000
          : equityRange === '30d'
          ? 30 * 24 * 60 * 60 * 1000
          : 7 * 24 * 60 * 60 * 1000;
      const startTs = endTs - rangeMs;
      const resp = await queryBotEquity(botId, startTs, endTs);
      setEquity(resp?.list || []);
    } catch (err) {
      message.error(`加载收益曲线失败：${err}`);
      setEquity([]);
    } finally {
      setEquityLoading(false);
    }
  };

  const loadBotMetrics = async () => {
    if (!Number.isFinite(botId) || botId <= 0) return;
    setMetricsLoading(true);
    try {
      const endTs = Math.floor(Date.now() / 1000);
      const rangeSec =
        equityRange === '1d' ? 24 * 3600 : equityRange === '30d' ? 30 * 24 * 3600 : 7 * 24 * 3600;
      const startTs = endTs - rangeSec;
      const resp = await queryBotMetrics(botId, 'account', { startTs, endTs });
      setBotMetrics(resp ?? null);
    } catch (err) {
      message.error(`加载绩效数据失败：${err}`);
      setBotMetrics(null);
    } finally {
      setMetricsLoading(false);
    }
  };

  const reloadAll = async () => {
    await Promise.all([
      loadBot(),
      loadBalance(),
      loadPositions(),
      loadEquity(),
      loadBotMetrics(),
      loadLogs(),
    ]);
  };

  useEffect(() => {
    if (!id) return;
    setBotMetrics(null);
    reloadAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  useEffect(() => {
    loadEquity();
    loadBotMetrics();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [equityRange]);

  const formattedEquityData = useMemo(() => {
    if (!equity.length) return [];
    const times = equity.map((p) => toMsIfSeconds(p.ts));
    const format = 'MM-DD HH:mm';
    return equity
      .map((point) => ({
        ts: point.ts,
        value: parseFloat(point.notional),
        time: dayjs(toMsIfSeconds(point.ts)).format(format),
      }))
      .sort((a, b) => a.ts - b.ts);
  }, [equity]);

  const equityYDomain = useMemo(() => {
    if (!formattedEquityData.length) return undefined;
    let min = Number.POSITIVE_INFINITY;
    let max = Number.NEGATIVE_INFINITY;
    for (const p of formattedEquityData) {
      if (!Number.isFinite(p.value)) continue;
      if (p.value < min) min = p.value;
      if (p.value > max) max = p.value;
    }
    if (!Number.isFinite(min) || !Number.isFinite(max)) return undefined;
    const range = max - min;
    const padding = range === 0 ? Math.abs(min) * 0.1 || 1 : range * 0.1;
    return [min - padding, max + padding] as [number, number];
  }, [formattedEquityData]);

  const pageTitle = bot?.name ? `${bot.name}` : id ? `实例 ${id}` : '实例详情';

  if (!Number.isFinite(botId) || botId <= 0) {
    return (
      <PageContainer title="实例详情">
        <Card>BotId 不合法</Card>
      </PageContainer>
    );
  }

  if (loading && !bot) {
    return (
      <PageContainer title={id ? `实例 ${id}` : '实例详情'}>
        <Card loading />
      </PageContainer>
    );
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle} - 策略实例</title>
      </Helmet>
      <PageContainer title={pageTitle}>
        <Row gutter={[16, 16]} justify="space-between" style={{ marginBottom: 16 }}>
          <Col span={6}>
            <Card variant="borderless" style={{ height: 100 }} styles={{ body: { padding: 0 } }}>
              <Flex
                justify="space-between"
                align="center"
                style={{ height: 100, padding: '0 20px' }}
              >
                <Statistic
                  title="健康度"
                  value={
                    botState
                      ? (() => {
                          const { score } = calculateBotHealth(botState);
                          return score;
                        })()
                      : '-'
                  }
                  valueStyle={{
                    color: botState
                      ? (() => {
                          const { level } = calculateBotHealth(botState);
                          return level === 'excellent'
                            ? '#388e3c'
                            : level === 'good'
                            ? '#1976d2'
                            : level === 'fair'
                            ? '#ed6c02'
                            : '#d32f2f';
                        })()
                      : undefined,
                  }}
                  suffix={botState ? '/ 100' : ''}
                />
              </Flex>
            </Card>
          </Col>
          <Col span={6}>
            <Card variant="borderless" style={{ height: 100 }} styles={{ body: { padding: 0 } }}>
              <Flex justify="space-between" align="center" style={{ height: 100 }}>
                <div style={{ marginLeft: 20 }}>
                  <Statistic
                    title="总资产估值"
                    value={`${Number(balance?.notional || 0).toFixed(2)}`}
                    precision={2}
                    suffix={<Typography.Text type="secondary">USDT</Typography.Text>}
                  />
                </div>
              </Flex>
            </Card>
          </Col>
          <Col span={6}>
            <Card variant="borderless" style={{ height: 100 }} styles={{ body: { padding: 0 } }}>
              <Flex justify="space-between" align="center" style={{ height: 100 }}>
                <div style={{ marginLeft: 20 }}>
                  <Statistic
                    title="未实现收益"
                    value={`${Number(balance?.unRealizedProfit || 0) >= 0 ? '+' : ''}${Number(
                      balance?.unRealizedProfit || 0,
                    ).toFixed(2)}`}
                    valueStyle={{
                      color: Number(balance?.unRealizedProfit || 0) >= 0 ? '#388e3c' : '#d32f2f',
                    }}
                    precision={2}
                    suffix={<Typography.Text type="secondary">USDT</Typography.Text>}
                  />
                </div>
              </Flex>
            </Card>
          </Col>
          <Col span={6}>
            <Card variant="borderless" style={{ height: 100 }} styles={{ body: { padding: 0 } }}>
              <Flex justify="space-between" align="center" style={{ height: 100 }}>
                <div style={{ marginLeft: 20 }}>
                  <Statistic
                    title="24小时变动"
                    value={`${Number(balance?.notional24HChange || 0) >= 0 ? '+' : ''}${Number(
                      balance?.notional24HChange || 0,
                    ).toFixed(2)}`}
                    valueStyle={{
                      color: Number(balance?.notional24HChange || 0) >= 0 ? '#388e3c' : '#d32f2f',
                    }}
                    precision={2}
                    suffix={<Typography.Text type="secondary">USDT</Typography.Text>}
                  />
                </div>
              </Flex>
            </Card>
          </Col>
        </Row>

        <Card variant="borderless">
          <ProDescriptions
            title="基本信息"
            extra={
              <Space>
                <Tooltip title={debugDisabled ? 'Bot 未启动，无法调试' : '打开调试面板'}>
                  <span>
                    <Button
                      type="primary"
                      disabled={debugDisabled}
                      onClick={() => setDebugVisible(true)}
                      icon={<BugOutlined />}
                    >
                      调试
                    </Button>
                  </span>
                </Tooltip>
                {bot?.exchange && bot?.accountId && String(bot.accountId) !== '0' && (
                  <Button
                    color="cyan"
                    variant="outlined"
                    icon={<DesktopOutlined />}
                    onClick={() => {
                      const params = new URLSearchParams({
                        exchange: String(bot.exchange),
                        accountId: String(bot.accountId),
                      });
                      history.push(`/exchange/market?${params.toString()}`);
                    }}
                  >
                    终端
                  </Button>
                )}
                <Button
                  color={bot?.status === 'running' ? 'danger' : 'primary'}
                  variant="outlined"
                  icon={
                    bot?.status === 'running' ? <PauseCircleOutlined /> : <PlayCircleOutlined />
                  }
                  loading={operating}
                  disabled={!bot}
                  onClick={async () => {
                    if (!bot) return;
                    setOperating(true);
                    try {
                      if (bot.status === 'running') {
                        const resp = await stopBot(bot.id);
                        if (!resp.errors) {
                          message.success('Bot 已停止');
                          await loadBot();
                        } else {
                          message.error(resp.errors[0]?.message || '停止失败');
                        }
                      } else {
                        const resp = await startBot(bot.id);
                        if (!resp.errors) {
                          message.success('Bot 已启动');
                          await loadBot();
                        } else {
                          message.error(resp.errors[0]?.message || '启动失败');
                        }
                      }
                    } finally {
                      setOperating(false);
                    }
                  }}
                >
                  {bot?.status === 'running' ? '停止' : '启动'}
                </Button>
                <Button onClick={reloadAll} icon={<ReloadOutlined />} loading={loading}>
                  刷新
                </Button>
              </Space>
            }
            column={3}
            dataSource={bot || ({} as Bot)}
          >
            <ProDescriptions.Item label="ID" copyable>
              {bot?.id}
            </ProDescriptions.Item>
            <ProDescriptions.Item label="名称">{bot?.name || '-'}</ProDescriptions.Item>
            <ProDescriptions.Item label="策略">
              <Space>
                {bot?.strategyId ? (
                  <Typography.Link onClick={handleOpenStrategyModal}>
                    {bot?.strategyName || bot.strategyId}
                  </Typography.Link>
                ) : (
                  <span>-</span>
                )}
                {bot?.strategyVer && (
                  <span>
                    (<Typography.Text type="success">{bot.strategyVer}</Typography.Text>)
                  </span>
                )}
              </Space>
            </ProDescriptions.Item>
            <ProDescriptions.Item label="模式">
              {bot?.mode ? <Tag>{bot.mode === 'paper' ? '模拟盘' : '实盘'}</Tag> : '-'}
            </ProDescriptions.Item>
            <ProDescriptions.Item label="状态">
              {bot?.status ? (
                <Tag
                  color={
                    bot.status === BotStatus.Running
                      ? 'green'
                      : bot.status === BotStatus.Error
                      ? 'red'
                      : 'default'
                  }
                >
                  {BotStatusOptions.find((x) => x.value === bot.status)?.label ?? bot.status}
                </Tag>
              ) : (
                '-'
              )}
            </ProDescriptions.Item>
            {botState && (
              <ProDescriptions.Item label="运行状态">
                <Space>
                  <Tag
                    color={
                      botState.executorStatus === 'running' && botState.jsRunnerStatus === 'running'
                        ? 'green'
                        : 'red'
                    }
                  >
                    {botState.executorStatus === 'running' && botState.jsRunnerStatus === 'running'
                      ? '正常'
                      : '异常'}
                  </Tag>
                  {(() => {
                    const { score, level } = calculateBotHealth(botState);
                    const healthColor =
                      level === 'excellent'
                        ? 'green'
                        : level === 'good'
                        ? 'blue'
                        : level === 'fair'
                        ? 'orange'
                        : 'red';
                    return <Tag color={healthColor}>健康度: {score}</Tag>;
                  })()}
                </Space>
              </ProDescriptions.Item>
            )}
            <ProDescriptions.Item label="交易所">
              {bot?.exchange ? (
                <Space size={4}>
                  <img
                    alt={bot.exchange}
                    style={{ display: 'inline', marginLeft: 0, paddingBottom: 2 }}
                    width={16}
                    src={utils.market.getExchangeLogo(bot.exchange)}
                  />
                  {utils.market.getExchangeTitle(bot.exchange)}
                </Space>
              ) : (
                '-'
              )}
            </ProDescriptions.Item>
            <ProDescriptions.Item label="账户ID" copyable>
              {bot?.accountId && String(bot.accountId) !== '0' ? (
                <Typography.Link
                  onClick={() => {
                    history.push(`/account/${bot.accountId}`);
                  }}
                >
                  {bot.accountId}
                </Typography.Link>
              ) : (
                '-'
              )}
            </ProDescriptions.Item>
            <ProDescriptions.Item label="创建时间">
              {bot?.createdAt ? dayjs(bot.createdAt * 1000).format('YYYY-MM-DD HH:mm:ss') : '-'}
            </ProDescriptions.Item>
          </ProDescriptions>

          {bot?.errorMessage && (
            <Tag
              color="red"
              style={{ marginBottom: 0, marginTop: 8, display: 'block', padding: '8px' }}
            >
              错误信息: {bot.errorMessage}
            </Tag>
          )}

          {botState?.runErr && (
            <Tag
              color="red"
              style={{ marginBottom: 0, marginTop: 8, display: 'block', padding: '8px' }}
            >
              运行错误: {botState.runErr}
            </Tag>
          )}

          <Divider style={{ marginTop: 24, marginBottom: 24 }} />

          <Typography.Title level={5} style={{ marginBottom: 8 }}>
            收益曲线
          </Typography.Title>
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            <Segmented
              options={[
                { label: '一天', value: '1d' },
                { label: '一周', value: '7d' },
                { label: '一月', value: '30d' },
              ]}
              value={equityRange}
              onChange={(v) => setEquityRange(v as any)}
            />
            <div style={{ height: 360 }}>
              {equityLoading ? (
                <Card loading />
              ) : formattedEquityData.length > 0 ? (
                <ResponsiveContainer width="100%" height={360}>
                  <LineChart
                    data={formattedEquityData}
                    margin={{ top: 5, right: 30, left: 20, bottom: 5 }}
                  >
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis
                      dataKey="time"
                      tick={{ fontSize: 12 }}
                      angle={-45}
                      textAnchor="end"
                      height={60}
                    />
                    <YAxis
                      tick={{ fontSize: 12 }}
                      tickFormatter={(value: number) => value.toFixed(2)}
                      domain={equityYDomain ?? ['auto', 'auto']}
                      allowDataOverflow
                    />
                    <RechartsTooltip
                      labelStyle={{ color: '#d89614' }}
                      formatter={(value: number) => Number(value).toFixed(2)}
                      labelFormatter={(_label, payload) => {
                        const ts = payload?.[0]?.payload?.ts as number | undefined;
                        return `时间: ${
                          ts ? dayjs(toMsIfSeconds(ts)).format('YYYY-MM-DD HH:mm:ss') : '-'
                        }`;
                      }}
                    />
                    <Line
                      type="monotone"
                      dataKey="value"
                      stroke="#1890ff"
                      strokeWidth={2}
                      dot={false}
                      name="收益"
                    />
                  </LineChart>
                </ResponsiveContainer>
              ) : (
                <Empty description="暂无收益曲线数据" />
              )}
            </div>
            <Spin spinning={metricsLoading}>
              <Descriptions column={3} style={{ marginTop: 16, marginLeft: 32, marginRight: 32 }}>
                <Descriptions.Item
                  label={
                    <Tooltip title="复合年化增长率">
                      <span>
                        <RiseOutlined
                          style={{
                            marginRight: 4,
                            color:
                              botMetrics?.cagr != null && botMetrics.cagr >= 0
                                ? '#52c41a'
                                : '#ff4d4f',
                          }}
                        />
                        年化收益率 (CAGR)
                      </span>
                    </Tooltip>
                  }
                >
                  <Typography.Text
                    style={{
                      color:
                        botMetrics?.cagr != null && botMetrics.cagr >= 0
                          ? '#52c41a'
                          : botMetrics?.cagr != null
                          ? '#ff4d4f'
                          : undefined,
                    }}
                  >
                    {botMetrics?.cagr != null
                      ? `${botMetrics.cagr >= 0 ? '+' : ''}${(botMetrics.cagr * 100).toFixed(2)}%`
                      : '-'}
                  </Typography.Text>
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="从峰值到谷底的最大跌幅">
                      <span>
                        <FallOutlined style={{ marginRight: 4, color: '#faad14' }} />
                        最大回撤
                      </span>
                    </Tooltip>
                  }
                >
                  <Typography.Text style={{ color: '#faad14' }}>
                    {botMetrics?.maxDrawdown != null
                      ? `${(botMetrics.maxDrawdown * 100).toFixed(2)}%`
                      : '-'}
                  </Typography.Text>
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="风险调整后收益">
                      <span>
                        <LineChartOutlined style={{ marginRight: 4 }} />
                        夏普比率
                      </span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.sharpe != null ? botMetrics.sharpe.toFixed(2) : '-'}
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="下行风险调整">
                      <span>
                        <BarChartOutlined style={{ marginRight: 4 }} />
                        索提诺比率
                      </span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.sortino != null ? botMetrics.sortino.toFixed(2) : '-'}
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="CAGR / 最大回撤">
                      <span>
                        <TrophyOutlined style={{ marginRight: 4 }} />
                        卡玛比率
                      </span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.calmar != null ? botMetrics.calmar.toFixed(2) : '-'}
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="20 日滚动">
                      <span>滚动夏普</span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.rollingSharpe != null ? botMetrics.rollingSharpe.toFixed(2) : '-'}
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="盈利交易占比">
                      <span>
                        <TrophyOutlined style={{ marginRight: 4 }} />
                        胜率
                      </span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.winRate != null ? `${(botMetrics.winRate * 100).toFixed(2)}%` : '-'}
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="总盈利 / 总亏损">
                      <span>
                        <DollarOutlined style={{ marginRight: 4 }} />
                        盈亏比
                      </span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.profitFactor != null ? botMetrics.profitFactor.toFixed(2) : '-'}
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="手续费 / 总盈亏">
                      <span>手续费占比</span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.feeRatio != null
                    ? `${(botMetrics.feeRatio * 100).toFixed(2)}%`
                    : '-'}
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="限价单">
                      <span>平均滑点 (bps)</span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.avgSlippageBps != null ? botMetrics.avgSlippageBps.toFixed(2) : '-'}
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="笔数">
                      <span>最大连续亏损</span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.maxConsecutiveLoss != null
                    ? String(botMetrics.maxConsecutiveLoss)
                    : '-'}
                </Descriptions.Item>
                <Descriptions.Item
                  label={
                    <Tooltip title="最长水下时间">
                      <span>回撤时长 (秒)</span>
                    </Tooltip>
                  }
                >
                  {botMetrics?.timeUnderWaterSeconds != null
                    ? String(botMetrics.timeUnderWaterSeconds)
                    : '-'}
                </Descriptions.Item>
              </Descriptions>
            </Spin>
          </Space>

          <Divider style={{ marginTop: 24, marginBottom: 24 }} />

          <Typography.Title level={5} style={{ marginBottom: 8 }}>
            资金列表
          </Typography.Title>
          <AssetsTable assets={assets} loading={assetsLoading} />

          <Typography.Title level={5} style={{ marginTop: 24, marginBottom: 8 }}>
            仓位列表
          </Typography.Title>
          <PositionsTable
            positions={positions}
            loading={positionsLoading}
            enableKlineLink
            onOpenKline={openKlineModal}
            exchange={bot?.exchange}
            accountId={bot?.accountId}
          />

          <Typography.Title level={5} style={{ marginTop: 24, marginBottom: 8 }}>
            订单列表
          </Typography.Title>
          <OrdersTable
            enableKlineLink
            onOpenKline={openKlineModal}
            pagination={{ showSizeChanger: true, pageSize: 10 }}
            request={async (params: any) => {
              const current = params.current || 1;
              const pageSize = params.pageSize || 10;
              try {
                const result = await queryBotOrders(botId, current, pageSize);
                const total = result?.totalCount || 0;
                return { data: result?.list || [], total, success: true };
              } catch (error) {
                return { data: [], total: 0, success: false };
              }
            }}
          />

          <Typography.Title level={5} style={{ marginTop: 24, marginBottom: 8 }}>
            资金流水
          </Typography.Title>
          <LedgersTable
            mode="bot"
            pagination={{ showSizeChanger: true, pageSize: 10 }}
            request={async (params: any) => {
              const current = params.current || 1;
              const pageSize = params.pageSize || 10;
              const startTs = 0;
              const endTs = dayjs().valueOf();
              try {
                const result = await queryBotLedger(botId, startTs, endTs, current, pageSize);
                const total = result?.totalCount || 0;
                return { data: result?.list || [], total, success: true };
              } catch (error) {
                return { data: [], total: 0, success: false };
              }
            }}
          />

          <Flex justify="space-between" align="center" style={{ marginTop: 24, marginBottom: 8 }}>
            <Typography.Title level={5} style={{ margin: 0 }}>
              历史日志
            </Typography.Title>
            <Select
              value={logsLevelFilter}
              onChange={setLogsLevelFilter}
              options={LOG_LEVEL_OPTIONS}
              style={{ width: 120 }}
            />
          </Flex>
          <div
            ref={logsScrollRef}
            style={{
              maxHeight: 400,
              overflowY: 'auto',
              border: `1px solid ${token.colorBorderSecondary}`,
              borderRadius: 8,
            }}
          >
            {logs.length === 0 && logsLoading ? (
              <div style={{ padding: 48, textAlign: 'center' }}>
                <Spin size="small" />
                <Typography.Text type="secondary" style={{ marginLeft: 8 }}>
                  加载中...
                </Typography.Text>
              </div>
            ) : logs.length === 0 ? (
              <Empty description="暂无日志" style={{ padding: 48 }} />
            ) : (
              <>
                <List
                  size="small"
                  dataSource={logs}
                  renderItem={(item) => (
                    <List.Item
                      style={{
                        padding: '8px 16px',
                        borderBottom: `1px solid ${token.colorBorderSecondary}`,
                        cursor: 'pointer',
                      }}
                      onDoubleClick={() => {
                        setSelectedLog(item);
                        setLogDetailVisible(true);
                      }}
                    >
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: 8,
                          width: '100%',
                          minWidth: 0,
                          overflow: 'hidden',
                        }}
                      >
                        <Tag
                          style={{ flexShrink: 0 }}
                          color={
                            item.level === 'error'
                              ? 'red'
                              : item.level === 'warn'
                              ? 'orange'
                              : item.level === 'info'
                              ? 'blue'
                              : 'default'
                          }
                        >
                          {item.level}
                        </Tag>
                        <Typography.Text type="secondary" style={{ fontSize: 12, flexShrink: 0 }}>
                          {dayjs(toMsIfSeconds(item.ts)).format('YYYY-MM-DD HH:mm:ss')}
                        </Typography.Text>
                        <Typography.Text ellipsis style={{ fontSize: 12, flex: 1, minWidth: 0 }}>
                          {item.message || '-'}
                        </Typography.Text>
                      </div>
                    </List.Item>
                  )}
                />
                {logsHasMore && (
                  <div ref={logsSentinelRef} style={{ padding: 12, textAlign: 'center' }}>
                    {logsLoading ? <Spin size="small" /> : null}
                  </div>
                )}
              </>
            )}
          </div>
        </Card>

        <BotDebugModal
          botId={botId}
          visible={debugVisible}
          onClose={() => setDebugVisible(false)}
        />

        <StrategyModal
          mode="readonly"
          open={strategyModalOpen}
          value={strategyDetail || undefined}
          onOpenChange={(open) => {
            setStrategyModalOpen(open);
            if (!open) {
              setStrategyDetail(null);
            }
          }}
        />

        <Modal
          title="日志详情"
          open={logDetailVisible}
          onCancel={() => {
            setLogDetailVisible(false);
            setSelectedLog(null);
          }}
          footer={null}
          width={560}
        >
          {selectedLog && (
            <Space direction="vertical" style={{ width: '100%' }} size="small">
              <div>
                <Typography.Text strong>ID: </Typography.Text>
                <Typography.Text copyable>{selectedLog.id}</Typography.Text>
              </div>
              <div>
                <Typography.Text strong>级别: </Typography.Text>
                <Tag
                  color={
                    selectedLog.level === 'error'
                      ? 'red'
                      : selectedLog.level === 'warn'
                      ? 'orange'
                      : selectedLog.level === 'info'
                      ? 'blue'
                      : 'default'
                  }
                >
                  {selectedLog.level}
                </Tag>
              </div>
              <div>
                <Typography.Text strong>时间: </Typography.Text>
                <Typography.Text type="secondary">
                  {dayjs(toMsIfSeconds(selectedLog.ts)).format('YYYY-MM-DD HH:mm:ss.SSS')}
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
          )}
        </Modal>

        <Modal
          open={klineVisible}
          title={
            <Space style={{ paddingLeft: 16, paddingRight: 16 }}>
              <span>K 线图：</span>
              <img
                alt={klineSymbol}
                style={{ display: 'inline', marginLeft: 0, paddingBottom: 2 }}
                width={16}
                src={utils.market.getExchangeLogo(bot?.exchange as Exchange)}
              />
              <Tag color="blue">{klineSymbol || '-'}</Tag>
            </Space>
          }
          styles={{
            content: { paddingLeft: 0, paddingRight: 0, paddingTop: 10, paddingBottom: 10 },
          }}
          width={1100}
          destroyOnHidden
          onCancel={() => {
            setKlineVisible(false);
            setKlineSymbol('');
            setKlineMarketInfo(null);
          }}
          footer={null}
        >
          {klineSymbol && bot?.exchange ? (
            <KlineChartPro
              exchange={String(bot.exchange)}
              symbol={klineSymbol}
              height={520}
              pricePrecision={klineMarketInfo?.pricePrecision ?? 6}
              volumePrecision={klineMarketInfo?.baseAssetPrecision ?? 2}
            />
          ) : (
            <Empty description="缺少交易对或交易所" />
          )}
        </Modal>
      </PageContainer>
    </>
  );
};

export default BotDetailPage;
