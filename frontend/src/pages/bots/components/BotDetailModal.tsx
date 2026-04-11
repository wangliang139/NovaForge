import {
  AccountEquity,
  Asset,
  Ledger,
  Order,
  Position,
  PositionSide,
  WalletType,
} from '@/services/gateway/account';
import {
  Bot,
  BotLog,
  queryBotBalance,
  queryBotEquity,
  queryBotLedger,
  queryBotLogs,
  queryBotOrders,
  queryBotPositions,
} from '@/services/gateway/strategy';
import { ProDescriptions, ProTable, type ProColumns } from '@ant-design/pro-components';
import { Button, Empty, Modal, Space, Tabs, Tag, Tooltip, Typography } from 'antd';
import dayjs from 'dayjs';
import React, { useEffect, useMemo, useState } from 'react';
import {
  CartesianGrid,
  Line,
  LineChart,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  XAxis,
  YAxis,
} from 'recharts';

type BotDetailModalProps = {
  bot: Bot | null;
  open: boolean;
  onClose: () => void;
};

const BotDetailModal: React.FC<BotDetailModalProps> = ({ bot, open, onClose }) => {
  const [equity, setEquity] = useState<AccountEquity[]>([]);
  const [equityRange, setEquityRange] = useState<'1d' | '7d' | '30d'>('7d');
  const [assets, setAssets] = useState<Asset[]>([]);
  const [assetsLoading, setAssetsLoading] = useState(false);
  const [positions, setPositions] = useState<Position[]>([]);
  const [positionsLoading, setPositionsLoading] = useState(false);
  const [logs, setLogs] = useState<BotLog[]>([]);
  const [logsCursor, setLogsCursor] = useState<string | undefined>(undefined);
  const [logsLoading, setLogsLoading] = useState(false);
  const [equityLoading, setEquityLoading] = useState(false);
  const [orderTotal, setOrderTotal] = useState(0);
  const [ledgerTotal, setLedgerTotal] = useState(0);
  const toMsIfSeconds = (ts: number) => (ts < 1e12 ? ts * 1000 : ts);

  useEffect(() => {
    if (open && bot) {
      loadLogs();
    }
  }, [open, bot]);

  useEffect(() => {
    if (open && bot) {
      loadData();
    }
  }, [open, bot, equityRange]);

  useEffect(() => {
    if (open && bot) {
      loadAssets();
    }
  }, [open, bot?.id]);

  useEffect(() => {
    if (open && bot) {
      loadPositions();
    }
  }, [open, bot?.id]);

  const loadAssets = async () => {
    const botId = Number(bot?.id);
    if (!bot || Number.isNaN(botId)) {
      setAssets([]);
      return;
    }
    setAssetsLoading(true);
    try {
      const balance = await queryBotBalance(botId);
      setAssets(balance?.assets || []);
    } catch (error) {
      console.error('加载资产信息失败', error);
      setAssets([]);
    } finally {
      setAssetsLoading(false);
    }
  };

  const loadPositions = async () => {
    const botId = Number(bot?.id);
    if (!bot || Number.isNaN(botId)) {
      setPositions([]);
      return;
    }
    setPositionsLoading(true);
    try {
      const list = await queryBotPositions(botId);
      setPositions(list || []);
    } catch (error) {
      console.error('加载仓位信息失败', error);
      setPositions([]);
    } finally {
      setPositionsLoading(false);
    }
  };

  const loadData = async () => {
    if (!bot) return;
    const botId = Number(bot.id);
    if (Number.isNaN(botId)) {
      setEquity([]);
      return;
    }
    setEquityLoading(true);
    try {
      const endTs = dayjs().valueOf();
      const rangeMs =
        equityRange === '1d' ? 24 * 60 * 60 * 1000 : equityRange === '30d' ? 30 * 24 * 60 * 60 * 1000 : 7 * 24 * 60 * 60 * 1000;
      const startTs = endTs - rangeMs;
      const equityResult = await queryBotEquity(botId, startTs, endTs);
      setEquity(equityResult?.list || []);
    } catch (error) {
      console.error('加载 Bot 数据失败', error);
    } finally {
      setEquityLoading(false);
    }
  };

  const loadLogs = async (cursor?: string) => {
    if (!bot) return;
    const botId = Number(bot.id);
    if (Number.isNaN(botId)) return;
    setLogsLoading(true);
    try {
      const result = await queryBotLogs({
        botId,
        limit: 50,
        cursor,
      });
      if (cursor) {
        setLogs((prev) => [...prev, ...(result?.list || [])]);
      } else {
        setLogs(result?.list || []);
      }
      setLogsCursor(result?.nextCursor);
    } catch (error) {
      console.error('加载 Bot 日志失败', error);
    } finally {
      setLogsLoading(false);
    }
  };

  const assetColumns: ProColumns<Asset>[] = [
    {
      title: '币种',
      dataIndex: 'code',
      width: 120,
    },
    {
      title: '钱包类型',
      dataIndex: 'walletType',
      width: 120,
      render: (_: React.ReactNode, record: Asset) => {
        const value = String(record.walletType || '');
        const typeMap: Record<string, { text: string; color: string }> = {
          [WalletType.Spot]: { text: '现货', color: 'blue' },
          [WalletType.Future]: { text: '合约', color: 'orange' },
          [WalletType.Fund]: { text: '资金', color: 'green' },
          [WalletType.Trade]: { text: '交易', color: 'purple' },
          [WalletType.Margin]: { text: '杠杆', color: 'red' },
        };
        const info = typeMap[value] || { text: value || '-', color: 'default' };
        return <Tag color={info.color}>{info.text}</Tag>;
      },
    },
    {
      title: '余额',
      dataIndex: 'balance',
      width: 140,
      align: 'right',
      render: (_: React.ReactNode, record: Asset) => {
        const value = parseFloat(String(record.balance ?? '0'));
        return Number.isFinite(value) && value !== 0 ? value.toFixed(8) : '0';
      },
    },
    {
      title: '可用',
      width: 140,
      align: 'right',
      render: (_: React.ReactNode, record: Asset) => {
        const balance = parseFloat(String(record.balance ?? '0'));
        const locked = parseFloat(String(record.locked ?? '0'));
        const b = Number.isFinite(balance) ? balance : 0;
        const l = Number.isFinite(locked) ? locked : 0;
        const available = b - l;
        return available !== 0 ? available.toFixed(8) : '0';
      },
    },
    {
      title: '冻结',
      dataIndex: 'locked',
      width: 140,
      align: 'right',
      tooltip: '直接展示 assets.locked（不按订单/仓位计算）',
      render: (_: React.ReactNode, record: Asset) => {
        const value = parseFloat(String(record.locked ?? '0'));
        return Number.isFinite(value) && value > 0 ? value.toFixed(8) : '0';
      },
    },
    {
      title: '现金价值 (USDT)',
      dataIndex: 'notional',
      width: 160,
      align: 'right',
      render: (_: React.ReactNode, record: Asset) => {
        const value = parseFloat(String(record.notional ?? '0'));
        return Number.isFinite(value) && value !== 0 ? value.toFixed(2) : '0';
      },
    },
  ];

  const positionColumns: ProColumns<Position>[] = [
    {
      title: '交易对',
      dataIndex: 'symbol',
      width: 140,
      ellipsis: true,
    },
    {
      title: '方向',
      dataIndex: 'side',
      width: 90,
      align: 'center',
      render: (_: React.ReactNode, record: Position) => {
        const side = record.side;
        const isLong = side === PositionSide.Long;
        const color = isLong ? 'green' : 'red';
        const label = side === PositionSide.Long ? '多' : side === PositionSide.Short ? '空' : side || '-';
        return <Tag color={color}>{label}</Tag>;
      },
    },
    {
      title: '杠杆',
      dataIndex: 'leverage',
      width: 90,
      align: 'center',
      render: (_: React.ReactNode, record: Position) => `${record.leverage || 0}x`,
    },
    {
      title: '数量',
      dataIndex: 'amount',
      width: 120,
      align: 'right',
      render: (_: React.ReactNode, record: Position) => {
        const value = parseFloat(String(record.amount ?? '0'));
        return Number.isFinite(value) && value !== 0 ? value.toFixed(6) : '0';
      },
    },
    {
      title: '开仓均价',
      dataIndex: 'entryPrice',
      width: 120,
      align: 'right',
      render: (_: React.ReactNode, record: Position) => {
        const value = parseFloat(String(record.entryPrice ?? '0'));
        return Number.isFinite(value) && value !== 0 ? value.toFixed(6) : '-';
      },
    },
    {
      title: '标记价格',
      dataIndex: 'markPrice',
      width: 120,
      align: 'right',
      render: (_: React.ReactNode, record: Position) => {
        const value = parseFloat(String(record.markPrice ?? '0'));
        return Number.isFinite(value) && value !== 0 ? value.toFixed(6) : '-';
      },
    },
    {
      title: '保证金',
      dataIndex: 'initialMargin',
      width: 120,
      align: 'right',
      render: (_: React.ReactNode, record: Position) => {
        const value = parseFloat(String(record.initialMargin ?? '0'));
        return Number.isFinite(value) && value !== 0 ? value.toFixed(6) : '0';
      },
    },
    {
      title: '强平价格',
      dataIndex: 'liquidationPrice',
      width: 120,
      align: 'right',
      render: (_: React.ReactNode, record: Position) => {
        const value = parseFloat(String(record.liquidationPrice ?? '0'));
        return Number.isFinite(value) && value !== 0 ? value.toFixed(6) : '-';
      },
    },
    {
      title: '现金价值 (USDT)',
      dataIndex: 'notional',
      width: 150,
      align: 'right',
      render: (_: React.ReactNode, record: Position) => {
        const value = parseFloat(String(record.notional ?? '0'));
        return Number.isFinite(value) && value !== 0 ? value.toFixed(2) : '0';
      },
    },
    {
      title: '未实现盈亏 (USDT)',
      dataIndex: 'unRealizedProfit',
      width: 160,
      align: 'right',
      render: (_: React.ReactNode, record: Position) => {
        const value = parseFloat(String(record.unRealizedProfit ?? '0'));
        if (!Number.isFinite(value) || value === 0) return '-';
        return <span style={{ color: value >= 0 ? 'green' : 'red' }}>{value.toFixed(2)}</span>;
      },
    },
  ];

  const orderColumns: ProColumns<Order>[] = [
    {
      title: 'ID',
      dataIndex: 'orderId',
      width: 200,
      ellipsis: true,
    },
    {
      title: '交易对',
      dataIndex: 'symbol',
      width: 120,
    },
    {
      title: '方向',
      dataIndex: 'side',
      width: 80,
      render: (_: React.ReactNode, record: Order) => {
        const side = record.side;
        return <Tag color={side === PositionSide.Long ? 'green' : 'red'}>{side}</Tag>;
      },
    },
    {
      title: '类型',
      dataIndex: 'orderType',
      width: 100,
    },
    {
      title: '价格',
      dataIndex: 'price',
      width: 120,
      render: (_: React.ReactNode, record: Order) =>
        record.price ? parseFloat(record.price).toFixed(6) : '-',
    },
    {
      title: '数量',
      dataIndex: 'quantity',
      width: 120,
      render: (_: React.ReactNode, record: Order) =>
        record.originalQty ? parseFloat(record.originalQty).toFixed(6) : '-',
    },
    {
      title: '已成交',
      dataIndex: 'executedQty',
      width: 120,
      render: (_: React.ReactNode, record: Order) =>
        record.executedQty ? parseFloat(record.executedQty).toFixed(6) : '0',
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (_: React.ReactNode, record: Order) => {
        const status = record.status;
        const colorMap: Record<string, string> = {
          NEW: 'blue',
          PENDING: 'blue',
          PARTIAL_DONE: 'orange',
          DONE: 'green',
          CANCELED: 'default',
          REJECTED: 'red',
          EXPIRED: 'default',
        };
        return <Tag color={colorMap[status] || 'default'}>{status}</Tag>;
      },
    },
    {
      title: '创建时间',
      dataIndex: 'createdTs',
      width: 180,
      render: (_: React.ReactNode, record: Order) =>
        dayjs(toMsIfSeconds(record.createdTs)).format('YYYY-MM-DD HH:mm:ss'),
    },
  ];

  const ledgerColumns: ProColumns<Ledger>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: '资产',
      dataIndex: 'asset',
      width: 100,
    },
    {
      title: '类型',
      dataIndex: 'type',
      width: 150,
    },
    {
      title: 'Total 变化',
      dataIndex: 'totalDelta',
      width: 120,
      render: (_: React.ReactNode, record: Ledger) => {
        const delta = record.totalDelta;
        if (!delta || delta === '0') return '-';
        const num = parseFloat(delta);
        return <span style={{ color: num > 0 ? 'green' : 'red' }}>{num.toFixed(6)}</span>;
      },
    },
    {
      title: 'Frozen 变化',
      dataIndex: 'frozenDelta',
      width: 120,
      render: (_: React.ReactNode, record: Ledger) => {
        const delta = record.frozenDelta;
        if (!delta || delta === '0') return '-';
        const num = parseFloat(delta);
        return <span style={{ color: num > 0 ? 'orange' : 'blue' }}>{num.toFixed(6)}</span>;
      },
    },
    {
      title: '时间',
      dataIndex: 'ts',
      width: 180,
      render: (_: React.ReactNode, record: Ledger) =>
        dayjs(toMsIfSeconds(record.ts)).format('YYYY-MM-DD HH:mm:ss'),
    },
  ];

  const logColumns: ProColumns<BotLog>[] = [
    {
      title: '时间',
      dataIndex: 'ts',
      width: 180,
      render: (_: React.ReactNode, record: BotLog) =>
        dayjs(toMsIfSeconds(record.ts)).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '级别',
      dataIndex: 'level',
      width: 80,
      align: 'center',
      render: (_: React.ReactNode, record: BotLog) => {
        const level = record.level?.toLowerCase();
        const colorMap: Record<string, string> = {
          debug: 'default',
          info: 'blue',
          warn: 'orange',
          error: 'red',
        };
        return <Tag color={colorMap[level] || 'default'}>{level || '-'}</Tag>;
      },
    },
    {
      title: '消息',
      dataIndex: 'message',
      ellipsis: true,
      render: (_: React.ReactNode, record: BotLog) => <Tooltip title={record.message}>{record.message || '-'}</Tooltip>,
    },
  ];

  const formattedEquityData = useMemo(() => {
    if (!equity.length) {
      return [];
    }
    const times = equity.map((p) => toMsIfSeconds(p.ts));
    const minTs = Math.min(...times);
    const maxTs = Math.max(...times);
    const duration = dayjs(maxTs).diff(dayjs(minTs), 'second');
    let format = 'MM-DD HH:mm:ss';
    if (duration > 60 * 5) {
      format = 'MM-DD HH:mm';
    }
    return equity
      .map((point) => ({
        ts: point.ts,
        value: parseFloat(point.notional),
        time: dayjs(toMsIfSeconds(point.ts)).format(format),
      }))
      .sort((a, b) => a.ts - b.ts);
  }, [equity]);

  const equityYDomain = useMemo(() => {
    if (!formattedEquityData.length) {
      return undefined;
    }
    let min = Number.POSITIVE_INFINITY;
    let max = Number.NEGATIVE_INFINITY;
    for (const p of formattedEquityData) {
      if (!Number.isFinite(p.value)) continue;
      if (p.value < min) min = p.value;
      if (p.value > max) max = p.value;
    }
    if (!Number.isFinite(min) || !Number.isFinite(max)) {
      return undefined;
    }
    const range = max - min;
    const padding = range === 0 ? Math.abs(min) * 0.1 || 1 : range * 0.1;
    return [min - padding, max + padding] as [number, number];
  }, [formattedEquityData]);

  return (
    <Modal
      title={`Bot 详情 - ${bot?.id}`}
      open={open}
      onCancel={onClose}
      width={1200}
      footer={null}
      destroyOnHidden
    >
      {bot && (
        <>
          <ProDescriptions
            column={3}
            dataSource={bot}
            columns={[
              {
                title: 'ID',
                dataIndex: 'id',
                copyable: true,
              },
              {
                title: '策略',
                dataIndex: 'strategyName',
                render: (_, record) => (<Space>
                  <span>{record.strategyName || record.strategyId}</span>
                  <span>(<Typography.Text type="success">{record.strategyVer}</Typography.Text>)</span>
                </Space>),
              },
              {
                title: '模式',
                dataIndex: 'mode',
                valueEnum: {
                  paper: { text: '模拟盘', status: 'Default' },
                  live: { text: '实盘', status: 'Processing' },
                },
              },
              {
                title: '状态',
                dataIndex: 'status',
                valueEnum: {
                  stopped: { text: '已停止', status: 'Default' },
                  running: { text: '运行中', status: 'Success' },
                  error: { text: '错误', status: 'Error' },
                },
              },
              {
                title: '交易所',
                dataIndex: 'exchange',
              },
              {
                title: '账户 ID',
                dataIndex: 'accountId',
              },
              {
                title: '创建时间',
                dataIndex: 'createdAt',
                render: (_, record) => dayjs(record.createdAt * 1000).format('YYYY-MM-DD HH:mm:ss'),
              },
            ]}
          />

          {bot.errorMessage && (
            <Tag color="red" style={{ marginBottom: 0, marginTop: 8, display: 'block', padding: '8px' }}>
              错误信息: {bot.errorMessage}
            </Tag>
          )}

          <Tabs
            style={{ marginTop: 16 }}
            items={[
              {
                key: 'equity',
                label: '收益曲线',
                children: (
                  <>
                    <div style={{ marginBottom: 12 }}>
                      <Tabs
                        size="small"
                        activeKey={equityRange}
                        onChange={(key) => setEquityRange(key as '1d' | '7d' | '30d')}
                        items={[
                          { key: '1d', label: '1天' },
                          { key: '7d', label: '7天' },
                          { key: '30d', label: '30天' },
                        ]}
                      />
                    </div>
                    <div style={{ height: 400 }}>
                      {equityLoading ? (
                        <Empty description="加载中..." />
                      ) : formattedEquityData.length > 0 ? (
                        <ResponsiveContainer width="100%" height={400}>
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
                              height={80}
                            />
                            <YAxis
                              tick={{ fontSize: 12 }}
                              tickFormatter={(value: number) => value.toFixed(2)}
                              domain={equityYDomain ?? ['auto', 'auto']}
                              allowDataOverflow
                            />
                            <RechartsTooltip
                              formatter={(value: number) => Number(value).toFixed(2)}
                              labelFormatter={(_label, payload) => {
                                const ts = payload?.[0]?.payload?.ts as number | undefined;
                                return `时间: ${ts ? dayjs(toMsIfSeconds(ts)).format('YYYY-MM-DD HH:mm:ss') : '-'}`;
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
                  </>
                ),
              },
              {
                key: 'positions',
                label: `仓位 (${positions.length})`,
                children: (
                  <ProTable
                    columns={positionColumns}
                    rowKey={(record) => `${record.symbol}-${record.side}`}
                    search={false}
                    options={false}
                    pagination={false}
                    loading={positionsLoading}
                    dataSource={positions}
                    scroll={{ x: 1200 }}
                  />
                ),
              },
              {
                key: 'orders',
                label: `订单 (${orderTotal})`,
                children: (
                  <ProTable
                    columns={orderColumns}
                    rowKey={(record) => record.orderId || record.clientOrderId}
                    search={false}
                    options={false}
                    pagination={{ showSizeChanger: true }}
                    request={async (params) => {
                      const botId = Number(bot?.id);
                      if (!bot || Number.isNaN(botId)) {
                        setOrderTotal(0);
                        return { data: [], total: 0, success: true };
                      }
                      const current = params.current || 1;
                      const pageSize = params.pageSize || 10;
                      try {
                        const result = await queryBotOrders(botId, current, pageSize);
                        const total = result?.totalCount || 0;
                        setOrderTotal(total);
                        return { data: result?.list || [], total, success: true };
                      } catch (error) {
                        console.error('加载 Bot 订单失败', error);
                        setOrderTotal(0);
                        return { data: [], total: 0, success: false };
                      }
                    }}
                  />
                ),
              },
              {
                key: 'assets',
                label: `资金列表 (${assets.length})`,
                children: (
                  <ProTable
                    columns={assetColumns}
                    rowKey={(record) => `${record.code}-${record.walletType}`}
                    search={false}
                    options={false}
                    pagination={false}
                    loading={assetsLoading}
                    dataSource={assets}
                  />
                ),
              },
              {
                key: 'ledger',
                label: `资金流水 (${ledgerTotal})`,
                children: (
                  <ProTable
                    columns={ledgerColumns}
                    rowKey="id"
                    search={false}
                    options={false}
                    pagination={{ showSizeChanger: true }}
                    request={async (params) => {
                      const botId = Number(bot?.id);
                      if (!bot || Number.isNaN(botId)) {
                        setLedgerTotal(0);
                        return { data: [], total: 0, success: true };
                      }
                      const current = params.current || 1;
                      const pageSize = params.pageSize || 10;
                      const startTs = 0;
                      const endTs = dayjs().valueOf();
                      try {
                        const result = await queryBotLedger(botId, startTs, endTs, current, pageSize);
                        const total = result?.totalCount || 0;
                        setLedgerTotal(total);
                        return { data: result?.list || [], total, success: true };
                      } catch (error) {
                        console.error('加载资金流水失败', error);
                        setLedgerTotal(0);
                        return { data: [], total: 0, success: false };
                      }
                    }}
                  />
                ),
              },
              {
                key: 'logs',
                label: `日志 (${logs.length})`,
                children: (
                  <>
                    <ProTable
                      columns={logColumns}
                      dataSource={logs}
                      rowKey="id"
                      search={false}
                      options={false}
                      pagination={false}
                      loading={logsLoading}
                      scroll={{ x: 1200 }}
                    />
                    <div style={{ textAlign: 'center', marginTop: 12 }}>
                      <Button
                        onClick={() => loadLogs(logsCursor)}
                        disabled={!logsCursor || logsLoading}
                      >
                        {logsCursor ? '加载更多' : '暂无更多'}
                      </Button>
                    </div>
                  </>
                ),
              },
            ]}
          />
        </>
      )}
    </Modal>
  );
};

export default BotDetailModal;
