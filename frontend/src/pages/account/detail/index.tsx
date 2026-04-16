import { EllipsisMiddleText } from '@/components';
import AssetsTable from '@/components/Market/AssetsTable';
import { KlineChartPro } from '@/components/Market/KlineChartPro';
import LedgersTable from '@/components/Market/LedgersTable';
import OrdersTable from '@/components/Market/OrdersTable';
import PositionsTable from '@/components/Market/PositionsTable';
import { api } from '@/services/gateway';
import type { RiskEvent } from '@/services/gateway/account';
import {
  Account,
  AccountConfig,
  AccountEquity,
  AccountInfo,
  AccountMultiBotDetails,
  AccountType,
  Asset,
  cancelOrder,
  getBalance,
  getLedgers,
  getOrders,
  getPositions,
  Ledger,
  offlineAccount,
  onlineAccount,
  Order,
  OrderSource,
  OrderStatus,
  OrderType,
  placeOrder,
  Position,
  PositionSide,
  queryAccount,
  queryAccountInfo,
  queryAccountMetrics,
  queryAccountMultiBotDetails,
  queryEquitys,
  queryRiskEvents,
  refreshAccountSnapshots,
  updateAccountRiskConfig,
} from '@/services/gateway/account';
import type { MarketInfo } from '@/services/gateway/market';
import utils from '@/utils';
import { getSideTagInfo, getWalletTypeTagInfo } from '@/utils/marketTag';
import {
  AlertOutlined,
  BugOutlined,
  CaretRightOutlined,
  DesktopOutlined,
  InfoCircleOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  SyncOutlined
} from '@ant-design/icons';
import {
  PageContainer,
  ProDescriptions,
  ProForm,
  ProFormDigit,
  ProFormText,
} from '@ant-design/pro-components';
import { history, useParams } from '@umijs/max';
import {
  Button,
  Card,
  Col,
  Dropdown,
  Empty,
  Flex,
  message,
  Modal,
  Row,
  Segmented,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import { MenuInfo } from 'rc-menu/lib/interface';
import type { FC, ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { LineChart, ResponsiveContainer, Line as RLine, XAxis, YAxis } from 'recharts';
import AccountDebugModal from './components/AccountDebugModal';
import AccountMetricsCard from './components/AccountMetricsCard';
import useStyles from './style.style';

const CLOSE_ORDER_TERMINAL_STATUSES = new Set<string>([
  OrderStatus.Done,
  OrderStatus.Canceled,
  OrderStatus.Rejected,
  OrderStatus.Expired,
]);

function sleepMs(ms: number) {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, ms);
  });
}

/** 轮询订单状态，直至进入终结态或超时（最多约 10s）。 */
async function waitUntilCloseOrderSettled(
  accountId: string,
  orderId: string,
  symbol: string,
): Promise<void> {
  const deadline = Date.now() + 10_000;
  const intervalMs = 400;
  while (Date.now() < deadline) {
    try {
      const response = await getOrders({
        accountId,
        symbol,
        includeFinished: true,
        page: 1,
        size: 50,
      });
      const order = response?.list?.find((o) => o.orderId === orderId);
      if (order?.status && CLOSE_ORDER_TERMINAL_STATUSES.has(order.status)) {
        return;
      }
    } catch {
      // 单次查询失败不中断，继续轮询直到超时
    }
    await sleepMs(intervalMs);
  }
}

function renderAccountTypeTag(accountType: AccountType) {
  const typeMap: Record<AccountType, { text: string; color: string }> = {
    [AccountType.Real]: { text: '真实账户', color: 'blue' },
    [AccountType.Virtual]: { text: '虚拟账户', color: 'green' },
    [AccountType.VirtualSub]: { text: '虚拟子账户', color: 'orange' },
    [AccountType.Unspecified]: { text: '未指定', color: 'default' },
  };
  const typeInfo = typeMap[accountType] ?? typeMap[AccountType.Unspecified];
  return <Tag color={typeInfo.color}>{typeInfo.text}</Tag>;
}

const AccountDetail: FC = () => {
  const { styles } = useStyles();
  const { id } = useParams<{ id: string }>();
  const [account, setAccount] = useState<Account | null>(null);
  const [assets, setAssets] = useState<Asset[]>([]);
  const [positions, setPositions] = useState<Position[]>([]);
  const [ledgers, setLedgers] = useState<Ledger[]>([]);
  const [orders, setOrders] = useState<Order[]>([]);
  const [accountInfo, setAccountInfo] = useState<AccountInfo | null>(null);
  const [assetsLoading, setAssetsLoading] = useState(false);
  const [positionsLoading, setPositionsLoading] = useState(false);
  const [closingPositionKey, setClosingPositionKey] = useState<string | null>(null);
  const [ordersLoading, setOrdersLoading] = useState(false);
  const [syncLoading, setSyncLoading] = useState(false);
  const [equityModalOpen, setEquityModalOpen] = useState(false);
  const [equityRange, setEquityRange] = useState('7d');
  const [equityLoading, setEquityLoading] = useState(false);
  const [equityPoints, setEquityPoints] = useState<AccountEquity[]>([]);
  const [dailyEquityLoading, setDailyEquityLoading] = useState(false);
  const [dailyEquityPoints, setDailyEquityPoints] = useState<AccountEquity[]>([]);
  const [metricsLoading, setMetricsLoading] = useState(false);
  const [accountMetrics, setAccountMetrics] =
    useState<Awaited<ReturnType<typeof queryAccountMetrics>>>(null);
  const [orderPagination, setOrderPagination] = useState({
    current: 1,
    pageSize: 10,
    total: 0,
  });
  const [onlyOnTheWay, setOnlyOnTheWay] = useState(false);
  const [orderFilters, setOrderFilters] = useState<{
    symbol?: string;
    orderType?: OrderType;
    orderSource?: OrderSource;
  }>({});
  const [ledgersLoading, setLedgersLoading] = useState(false);
  const [accountInfoLoading, setAccountInfoLoading] = useState(false);
  const [ledgerPagination, setLedgerPagination] = useState({ current: 1, pageSize: 10, total: 0 });
  const [debugModalVisible, setDebugModalVisible] = useState(false);
  const [riskConfig, setRiskConfig] = useState<AccountConfig | null>(null);
  const [riskFormVisible, setRiskFormVisible] = useState(false);
  const [statusOperating, setStatusOperating] = useState(false);
  const [loading, setLoading] = useState(false);
  const [reloading, setReloading] = useState(false);
  const [klineVisible, setKlineVisible] = useState(false);
  const [klineSymbol, setKlineSymbol] = useState<string>('');
  const [klineMarketInfo, setKlineMarketInfo] = useState<MarketInfo | null>(null);
  const [cancelingOrderId, setCancelingOrderId] = useState<string | null>(null);
  const [riskEvents, setRiskEvents] = useState<RiskEvent[]>([]);
  const [riskEventsLoading, setRiskEventsLoading] = useState(false);
  const [riskModalOpen, setRiskModalOpen] = useState(false);
  const [multiBotModalOpen, setMultiBotModalOpen] = useState(false);
  const [multiBotLoading, setMultiBotLoading] = useState(false);
  const [multiBotDetails, setMultiBotDetails] = useState<AccountMultiBotDetails | null>(null);
  const [maxPositionPerSymbolMode, setMaxPositionPerSymbolMode] = useState<'amount' | 'ratio'>(
    'amount',
  );
  const [maxDailyLossMode, setMaxDailyLossMode] = useState<'amount' | 'ratio'>('amount');
  const [maxTotalNetExposureMode, setMaxTotalNetExposureMode] = useState<'amount' | 'ratio'>(
    'amount',
  );
  const [maxTotalGrossExposureMode, setMaxTotalGrossExposureMode] = useState<'amount' | 'ratio'>(
    'amount',
  );
  const [riskForm] = ProForm.useForm();

  const latestRiskEvent = useMemo(() => {
    if (!riskEvents.length) return null;
    return [...riskEvents].sort((a, b) => b.createdAt - a.createdAt)[0];
  }, [riskEvents]);

  const openKlineModal = (symbol: string) => {
    const ex = account?.exchange;
    if (!ex) {
      message.error('账户交易所缺失，无法查询 K 线');
      return;
    }
    if (!symbol) return;
    setKlineVisible(true);
    setKlineSymbol(symbol);
    setKlineMarketInfo(null);
  };

  useEffect(() => {
    if (!klineVisible || !klineSymbol) return;
    const ex = account?.exchange;
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
  }, [klineVisible, klineSymbol, account?.exchange]);

  const toNumber = (value: any) => {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  };

  const formatRiskIndex = (value?: string | number | null) => {
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) {
      return '0.0';
    }
    return parsed.toFixed(1);
  };

  const formatRatioAsPercent = (ratio?: string) => {
    const value = Number(ratio);
    if (!Number.isFinite(value)) {
      return '-';
    }
    return `${(value * 100).toFixed(2)}%`;
  };

  const decideLimitMode = (amount?: any, ratio?: any): 'amount' | 'ratio' => {
    const hasAmount = amount !== undefined && amount !== null && amount !== '';
    const hasRatio = ratio !== undefined && ratio !== null && ratio !== '';
    if (hasRatio) return 'ratio';
    if (hasAmount) return 'amount';
    return 'amount';
  };

  useEffect(() => {
    if (!riskFormVisible || !riskConfig) return;
    setMaxPositionPerSymbolMode(
      decideLimitMode(
        riskConfig.maxPositionPerSymbol?.amount,
        riskConfig.maxPositionPerSymbol?.ratio,
      ),
    );
    setMaxDailyLossMode(
      decideLimitMode(riskConfig.maxDailyLoss?.amount, riskConfig.maxDailyLoss?.ratio),
    );
    setMaxTotalNetExposureMode(
      decideLimitMode(
        riskConfig.maxTotalNetExposure?.amount,
        riskConfig.maxTotalNetExposure?.ratio,
      ),
    );
    setMaxTotalGrossExposureMode(
      decideLimitMode(
        riskConfig.maxTotalGrossExposure?.amount,
        riskConfig.maxTotalGrossExposure?.ratio,
      ),
    );
  }, [riskFormVisible, riskConfig]);

  const orderSymbolFilters = useMemo(
    () =>
      Array.from(new Set(orders.map((item) => item.symbol).filter(Boolean))).map((symbol) => ({
        text: symbol as string,
        value: symbol as string,
      })),
    [orders],
  );

  const orderSourceFilters = useMemo(
    () => [
      { text: '用户', value: OrderSource.User },
      { text: '策略', value: OrderSource.Strategy },
      { text: '强平', value: OrderSource.Liquidation },
      { text: 'ADL', value: OrderSource.Adl },
    ],
    [],
  );

  useEffect(() => {
    if (id) {
      loadAccountInfo();
      loadAccountExtraInfo();
      loadAccountBalance();
      loadAccountPositions();
      loadAccountLedgers(1, 10);
      setOrders([]);
      setOrderPagination({ current: 1, pageSize: 10, total: 0 });
      setOnlyOnTheWay(false);
      setOrderFilters({});
      loadAccountOrders({}, { page: 1, pageSize: 10 });
      loadRiskEvents();
      setEquityPoints([]);
      setDailyEquityPoints([]);
      setAccountMetrics(null);
      loadDailyEquity();
    }
  }, [id]);

  useEffect(() => {
    if (equityModalOpen && id) {
      loadAccountEquity(equityRange);
      const days = equityRange === '1d' ? 1 : equityRange === '7d' ? 7 : 30;
      loadAccountMetrics(days);
    }
  }, [equityModalOpen, equityRange, id]);

  const loadAccountInfo = async () => {
    try {
      const res = await queryAccount(String(id));
      if (res && res.list && res.list.length > 0) {
        const acc = res.list[0] as any;
        setAccount(acc);
        setRiskConfig(acc.config);
      }
    } catch (err) {
      message.error(`加载账户信息失败：${err}`);
    }
  };

  const loadRiskEvents = async () => {
    try {
      if (!id) return;
      setRiskEventsLoading(true);
      const events = await queryRiskEvents(id, { limit: 50, offset: 0 });
      setRiskEvents(events || []);
    } catch (err) {
      message.error(`加载风控事件失败：${err}`);
    } finally {
      setRiskEventsLoading(false);
    }
  };

  const loadAccountMultiBotDetails = async () => {
    try {
      if (!id) return;
      setMultiBotLoading(true);
      const res = await queryAccountMultiBotDetails(id);
      setMultiBotDetails(res || { subAccounts: [], assetAllocations: [], positionAllocations: [] });
    } catch (err) {
      message.error(`加载子账户信息失败：${err}`);
    } finally {
      setMultiBotLoading(false);
    }
  };

  const loadAccountExtraInfo = async () => {
    try {
      if (!id) return;
      setAccountInfoLoading(true);
      const res = await queryAccountInfo(id);
      if (res) {
        setAccountInfo(res);
      }
    } catch (err) {
      message.error(`加载账户附加信息失败：${err}`);
    } finally {
      setAccountInfoLoading(false);
    }
  };

  const loadAccountBalance = async () => {
    try {
      if (!id) return;
      setAssetsLoading(true);
      const balance = await getBalance(id);
      if (balance && balance.assets) {
        setAssets(balance.assets);
      }
    } catch (err) {
      message.error(`加载资产信息失败：${err}`);
    } finally {
      setAssetsLoading(false);
    }
  };

  const loadAccountEquity = async (range: string) => {
    try {
      if (!id) return;
      setEquityLoading(true);
      const response = await queryEquitys(id, range);
      setEquityPoints(response || []);
    } catch (err) {
      message.error(`加载收益曲线失败：${err}`);
    } finally {
      setEquityLoading(false);
    }
  };

  const loadDailyEquity = async () => {
    try {
      if (!id) return;
      setDailyEquityLoading(true);
      const response = await queryEquitys(id, '1d');
      setDailyEquityPoints(response || []);
    } catch (err) {
      message.error(`加载日收益曲线失败：${err}`);
    } finally {
      setDailyEquityLoading(false);
    }
  };

  const loadAccountMetrics = async (days: number) => {
    try {
      if (!id) return;
      setMetricsLoading(true);
      const endTs = Math.floor(Date.now() / 1000);
      const startTs = endTs - days * 24 * 3600;
      const response = await queryAccountMetrics(id, 'account', { startTs, endTs });
      setAccountMetrics(response ?? null);
    } catch (err) {
      message.error(`加载绩效数据失败：${err}`);
    } finally {
      setMetricsLoading(false);
    }
  };

  const loadAccountPositions = async () => {
    try {
      if (!id) return;
      setPositionsLoading(true);
      const positions = await getPositions(id);
      if (positions) {
        setPositions(positions);
      }
    } catch (err) {
      message.error(`加载仓位信息失败：${err}`);
    } finally {
      setPositionsLoading(false);
    }
  };

  const loadAccountOrders = async (
    filters: {
      symbol?: string;
      orderType?: OrderType;
      orderSource?: OrderSource;
    },
    options?: { page?: number; pageSize?: number; onlyOnTheWay?: boolean },
  ) => {
    if (!id) {
      return;
    }
    const page = options?.page ?? orderPagination.current;
    const pageSize = options?.pageSize ?? orderPagination.pageSize;
    const isOnlyOnTheWay = options?.onlyOnTheWay ?? onlyOnTheWay;
    try {
      setOrdersLoading(true);
      const response = await getOrders({
        accountId: id,
        symbol: filters.symbol,
        orderType: filters.orderType,
        orderSource: filters.orderSource,
        includeFinished: !isOnlyOnTheWay,
        page,
        size: pageSize,
      });
      const list = response?.list || [];
      setOrders(list);
      setOrderPagination({
        current: page,
        pageSize,
        total: response?.totalCount ?? 0,
      });
    } catch (err) {
      message.error(`加载订单信息失败：${err}`);
    } finally {
      setOrdersLoading(false);
    }
  };

  const loadAccountLedgers = async (page: number, pageSize: number) => {
    try {
      if (!id) return;
      setLedgersLoading(true);
      const endTs = dayjs().valueOf();
      const startTs = dayjs().subtract(7, 'day').valueOf();
      const response = await getLedgers(id, startTs, endTs, pageSize, page);
      if (response) {
        const list = response.list || [];
        setLedgers(list);
        setLedgerPagination({ current: page, pageSize, total: response.totalCount ?? 0 });
      }
    } catch (err) {
      message.error(`加载资金流水失败：${err}`);
    } finally {
      setLedgersLoading(false);
    }
  };

  const handleCancelOrder = (order: Order) => {
    if (!order || !order.orderId) return;
    const orderId = order.orderId;
    Modal.confirm({
      title: '确认撤单',
      okText: '确认撤单',
      okType: 'danger',
      cancelText: '取消',
      content: (
        <div>
          确认撤销订单 {orderId}
          {order.symbol ? `（${order.symbol}）` : ''} 吗？
        </div>
      ),
      onOk: async () => {
        setCancelingOrderId(orderId);
        try {
          const symbol = order.symbol;
          const clientOrderId = order.clientOrderId || '';
          const orderIdVal = order.orderId;
          await cancelOrder(id!, symbol, clientOrderId, orderIdVal);
          message.success('撤单成功');
          await loadAccountOrders(orderFilters, {
            page: orderPagination.current,
            pageSize: orderPagination.pageSize,
            onlyOnTheWay,
          });
        } catch (err) {
          message.error(`撤单失败：${err}`);
        } finally {
          setCancelingOrderId(null);
        }
      },
    });
  };

  const handleSyncSnapshots = async () => {
    if (!id || syncLoading) {
      return;
    }
    setSyncLoading(true);
    try {
      const success = await refreshAccountSnapshots(id);
      if (success) {
        message.success('同步成功');
        await Promise.all([
          loadAccountBalance(),
          loadAccountPositions(),
          loadAccountOrders(orderFilters, {
            page: orderPagination.current,
            pageSize: orderPagination.pageSize,
            onlyOnTheWay,
          }),
        ]);
      } else {
        message.error('同步失败');
      }
    } catch (err) {
      message.error(`同步失败：${err}`);
    } finally {
      setSyncLoading(false);
    }
  };

  const reloadAll = async () => {
    if (!id || reloading) return;
    setReloading(true);
    try {
      await Promise.all([
        loadAccountInfo(),
        loadAccountExtraInfo(),
        loadAccountBalance(),
        loadAccountPositions(),
        loadAccountLedgers(ledgerPagination.current, ledgerPagination.pageSize),
        loadAccountOrders(orderFilters, {
          page: orderPagination.current,
          pageSize: orderPagination.pageSize,
          onlyOnTheWay,
        }),
        loadRiskEvents(),
        loadDailyEquity(),
        ...(equityModalOpen
          ? [
            loadAccountEquity(equityRange),
            loadAccountMetrics(equityRange === '1d' ? 1 : equityRange === '7d' ? 7 : 30),
          ]
          : []),
      ]);
    } finally {
      setReloading(false);
    }
  };

  const dailyEquityChartData = [...dailyEquityPoints]
    .sort((a, b) => a.ts - b.ts)
    .map((point) => ({
      time: dayjs(point.ts).format('YYYY-MM-DD HH:mm'),
      value: toNumber(point.notional),
    }));

  const handleClosePosition = (record: Position) => {
    const rowKey = `${record.symbol}-${record.side}`;
    const sideLabel =
      record.side === PositionSide.Long
        ? '多'
        : record.side === PositionSide.Short
          ? '空'
          : String(record.side || '-');
    const amountNum = Number(record.amount);
    const baseDisabled = !id || !Number.isFinite(amountNum) || amountNum <= 0;
    const busyOtherRow = closingPositionKey !== null && closingPositionKey !== rowKey;
    if (baseDisabled || busyOtherRow) {
      return;
    }
    Modal.confirm({
      title: '确认平仓',
      okText: '确认平仓',
      okType: 'danger',
      cancelText: '取消',
      content: (
        <div>
          将以市价平仓：{record.symbol}（{sideLabel}），数量 {record.amount}。
          <br />
          是否继续？
        </div>
      ),
      onOk: async () => {
        if (!id) return;
        setClosingPositionKey(rowKey);
        try {
          const gqlSide =
            record.side === PositionSide.Long ? PositionSide.Long : PositionSide.Short;
          if (!gqlSide) {
            message.error('仓位方向不合法，无法平仓');
            return;
          }
          const res = await placeOrder({
            accountId: id,
            symbol: record.symbol,
            side: gqlSide,
            isBuy: record.side === PositionSide.Short,
            orderType: OrderType.Market,
            quantity: String(record.amount),
            reduceOnly: true,
          });
          if (res?.orderId) {
            message.success('平仓下单成功');
            await waitUntilCloseOrderSettled(id, res.orderId, record.symbol);
            await Promise.all([
              loadAccountPositions(),
              loadAccountOrders(orderFilters, {
                page: orderPagination.current,
                pageSize: orderPagination.pageSize,
                onlyOnTheWay,
              }),
            ]);
            return;
          }
          message.error('平仓失败');
        } catch (err) {
          message.error(`平仓失败：${err}`);
        } finally {
          setClosingPositionKey(null);
        }
      },
    });
  };

  if (!account) {
    return (
      <PageContainer>
        <Card loading={true} />
      </PageContainer>
    );
  }

  const accountIsOnline = account.status === 'online';
  const debugDisabled = !accountIsOnline;

  const renderAccountInfoValue = (value?: ReactNode) => {
    if (accountInfoLoading || !accountInfo) {
      return '-';
    }
    if (value === undefined || value === null || value === '') {
      return '-';
    }
    return value;
  };

  const renderAccountPermission = (enabled?: boolean) => {
    if (accountInfoLoading || !accountInfo) {
      return '-';
    }
    return <Tag color={enabled ? 'green' : 'default'}>{enabled ? '已开启' : '未开启'}</Tag>;
  };

  const multiBotSubAccounts = multiBotDetails?.subAccounts || [];
  const multiBotAssetRows = multiBotDetails?.assetAllocations || [];
  const multiBotPositionRows = multiBotDetails?.positionAllocations || [];

  const onClickButtonGroup = async ({ key }: MenuInfo) => {
    if (!id) return;
    if (key === 'online' || key === 'offline') {
      const nextAction = accountIsOnline ? '下线' : '上线';
      const doToggle = async () => {
        setStatusOperating(true);
        try {
          const resp = accountIsOnline ? await offlineAccount(id) : await onlineAccount(id);
          const errMsg = resp?.errors?.[0]?.message;
          if (errMsg) {
            return;
          }
          message.success(`账户已${nextAction}`);
          await reloadAll();
        } catch (err) {
          message.error(`账户${nextAction}失败：${err}`);
        } finally {
          setStatusOperating(false);
        }
      };
      if (accountIsOnline) {
        Modal.confirm({
          title: '确认下线账户',
          content: '下线后将停止账户相关数据订阅；若账户被运行中的 Bot 绑定，将无法下线。',
          okText: '确认下线',
          okType: 'danger',
          cancelText: '取消',
          onOk: doToggle,
        });
        return;
      }
      doToggle();
    }
    if (key === 'offline') {
      const resp = await offlineAccount(id);
      if (!resp.errors) {
        message.success('账户已下线');
        await reloadAll();
      }
    }
    if (key === 'sync') {
      await handleSyncSnapshots();
    }
  };

  const renderButtonGroup = () => {
    const items = [
      {
        key: 'online',
        label: (
          <Space style={{ color: accountIsOnline ? '#999' : '#52c41a' }}>
            <CaretRightOutlined />
            上线
          </Space>
        ),
        disabled: accountIsOnline,
      },
      {
        key: 'offline',
        label: (
          <Space style={{ color: !accountIsOnline ? '#999' : '#fa8c16' }}>
            <PoweroffOutlined />
            下线
          </Space>
        ),
        danger: true,
        disabled: !accountIsOnline,
      },
      {
        key: 'sync',
        label: (
          <Space style={{ color: '#1677ff' }}>
            <SyncOutlined /> 同步
          </Space>
        ),
      },
    ];
    return (
      <Dropdown.Button
        menu={{ items: items, onClick: onClickButtonGroup }}
        onClick={reloadAll}
        loading={reloading}
        disabled={reloading}
      >
        <ReloadOutlined /> 刷新
      </Dropdown.Button>
    );
  };

  const renderDailyEquityChart = () => {
    return (
      <Card
        style={{ width: '45%', cursor: 'pointer', marginRight: 20 }}
        styles={{ body: { paddingLeft: 10, paddingRight: 10, paddingTop: 10, paddingBottom: 10 } }}
        onClick={() => setEquityModalOpen(true)}
      >
        <ResponsiveContainer width="100%" height={60}>
          <LineChart
            data={dailyEquityChartData}
            style={{ cursor: 'pointer' }}
            margin={{ top: 4, right: 8, left: 0, bottom: 0 }}
          >
            <XAxis dataKey="time" hide />
            <YAxis hide domain={['auto', 'auto']} />
            <RLine type="monotone" dataKey="value" stroke="#1677ff" strokeWidth={2} dot={false} />
          </LineChart>
        </ResponsiveContainer>
      </Card>
    );
  };

  return (
    <PageContainer>
      <Flex justify="space-between" style={{ marginBottom: 16 }}>
        <Card
          variant="borderless"
          style={{ width: '24%', height: 100 }}
          styles={{ body: { padding: 0 } }}
        >
          <Flex justify="space-between" align="center" style={{ height: 100 }}>
            <div style={{ marginLeft: 20 }}>
              <Statistic
                title="总资产估值"
                value={`${toNumber(account.stats?.notional).toFixed(2)}`}
                precision={2}
                suffix={<Typography.Text type="secondary">USDT</Typography.Text>}
              />
            </div>
          </Flex>
        </Card>
        <Card
          variant="borderless"
          style={{ width: '24%', height: 100 }}
          styles={{ body: { padding: 0 } }}
        >
          <Flex justify="space-between" align="center" style={{ height: 100 }}>
            <div style={{ marginLeft: 20 }}>
              <Statistic
                title="未实现收益"
                value={`${toNumber(account.stats?.unRealizedProfit) >= 0 ? '+' : ''}${toNumber(
                  account.stats?.unRealizedProfit,
                ).toFixed(2)}`}
                valueStyle={{
                  color: toNumber(account.stats?.unRealizedProfit) >= 0 ? '#388e3c' : '#d32f2f',
                }}
                precision={2}
                suffix={<Typography.Text type="secondary">USDT</Typography.Text>}
              />
            </div>
          </Flex>
        </Card>
        <Card
          variant="borderless"
          style={{ width: '24%', height: 100 }}
          styles={{ body: { padding: 0 } }}
        >
          <Flex justify="space-between" align="center" style={{ height: 100 }}>
            <div style={{ width: 160, marginLeft: 20 }}>
              <Statistic
                title="24小时变动"
                value={`${toNumber(account.stats?.notional24HChange) >= 0 ? '+' : ''}${toNumber(
                  account.stats?.notional24HChange,
                ).toFixed(2)}`}
                valueStyle={{
                  color: toNumber(account.stats?.notional24HChange) >= 0 ? '#388e3c' : '#d32f2f',
                }}
                precision={2}
                suffix={<Typography.Text type="secondary">USDT</Typography.Text>}
              />
            </div>
            {dailyEquityChartData.length > 0 && renderDailyEquityChart()}
          </Flex>
        </Card>
        <Card
          variant="borderless"
          style={{ width: '24%', height: 100 }}
          styles={{ body: { padding: 0 } }}
        >
          <Flex justify="space-between" align="center" style={{ height: 100 }}>
            <div style={{ width: 160, marginLeft: 20 }}>
              <Statistic
                title="风险指数"
                value={formatRiskIndex(account?.riskIndex)}
                valueStyle={{
                  color:
                    toNumber(account?.riskIndex) < 30
                      ? '#388e3c'
                      : toNumber(account?.riskIndex) < 60
                        ? '#f57c00'
                        : '#d32f2f',
                }}
                suffix={<Typography.Text type="secondary">/ 100</Typography.Text>}
              />
            </div>
          </Flex>
        </Card>
      </Flex>
      <Card variant="borderless">
        <ProDescriptions
          title={
            <Space size={8}>
              <span>基本信息</span>
              {latestRiskEvent && (
                <Typography.Link type="warning" onClick={() => setRiskModalOpen(true)}>
                  最新风控：{latestRiskEvent.rule}
                  {latestRiskEvent.createdAt
                    ? `（${dayjs.unix(latestRiskEvent.createdAt).format('MM-DD HH:mm')}）`
                    : ''}
                </Typography.Link>
              )}
            </Space>
          }
          extra={
            <Space>
              <span>
                <Button
                  type="primary"
                  variant="outlined"
                  disabled={debugDisabled}
                  onClick={() => setDebugModalVisible(true)}
                  icon={<BugOutlined />}
                >
                  调试
                </Button>
              </span>
              <Button
                color="danger"
                variant="outlined"
                icon={<AlertOutlined />}
                onClick={() => setRiskModalOpen(true)}
              >
                风控
              </Button>
              <Button
                color="cyan"
                variant="outlined"
                disabled={!account.exchange || !id}
                icon={<DesktopOutlined />}
                onClick={() => {
                  if (!account.exchange || !id) return;
                  const params = new URLSearchParams({
                    exchange: String(account.exchange),
                    accountId: id,
                  });
                  history.push(`/exchange/market?${params.toString()}`);
                }}
              >
                终端
              </Button>
              {account.multiBotMode && (
                <Button
                  color="purple"
                  variant="outlined"
                  onClick={async () => {
                    setMultiBotModalOpen(true);
                    await loadAccountMultiBotDetails();
                  }}
                >
                  子账户
                </Button>
              )}
              {renderButtonGroup()}
            </Space>
          }
          style={{
            marginBottom: 32,
          }}
        >
          <ProDescriptions.Item label="ID" copyable>
            {account.id}
          </ProDescriptions.Item>
          <ProDescriptions.Item label="账户名称">{account.name}</ProDescriptions.Item>
          <ProDescriptions.Item label="交易所">
            <Space size={4}>
              <img
                alt={account.exchange}
                style={{ display: 'inline', marginLeft: 0, paddingBottom: 2 }}
                width={16}
                src={utils.market.getExchangeLogo(account.exchange)}
              />
              {utils.market.getExchangeTitle(account.exchange)}
            </Space>
          </ProDescriptions.Item>
          <ProDescriptions.Item label="交易所 UID" copyable>
            {renderAccountInfoValue(accountInfo?.uid)}
          </ProDescriptions.Item>
          {account.accountType === AccountType.VirtualSub && (
            <ProDescriptions.Item label="父账户 ID">
              {account.parentAccountId ? (
                <Typography.Link
                  onClick={() => history.push(`/account/${account.parentAccountId}`)}
                >
                  {account.parentAccountId}
                </Typography.Link>
              ) : (
                '-'
              )}
            </ProDescriptions.Item>
          )}
          <ProDescriptions.Item label="账户类型">
            {renderAccountTypeTag(account.accountType)}
          </ProDescriptions.Item>
          <ProDescriptions.Item label="多 Bot 模式">
            {account.multiBotMode ? '是' : '否'}
          </ProDescriptions.Item>
          <ProDescriptions.Item label="状态">
            <Tag color={account.status === 'online' ? 'green' : 'default'}>
              {account.status === 'online' ? '在线' : '离线'}
            </Tag>
          </ProDescriptions.Item>
          <ProDescriptions.Item label="标签">
            {account.tags && account.tags.length > 0 ? (
              <Space size={0}>
                {account.tags.map((tag) => (
                  <Tag key={tag}>{tag}</Tag>
                ))}
              </Space>
            ) : (
              '无'
            )}
          </ProDescriptions.Item>
          <ProDescriptions.Item label="创建时间">
            {account.createdAt >= 0
              ? dayjs.unix(account.createdAt).format('YYYY-MM-DD HH:mm:ss')
              : '-'}
          </ProDescriptions.Item>
          <ProDescriptions.Item label="现货权限">
            {renderAccountPermission(accountInfo?.isSpotEnabled)}
          </ProDescriptions.Item>
          <ProDescriptions.Item label="合约权限">
            {renderAccountPermission(accountInfo?.isFutureEnabled)}
          </ProDescriptions.Item>
        </ProDescriptions>
        <div className={styles.title}>资产列表</div>
        <AssetsTable
          assets={assets}
          loading={assetsLoading}
          showSummary
          accountId={id}
        />
        <div className={styles.title} style={{ marginTop: 32 }}>
          仓位列表
        </div>
        <PositionsTable
          positions={positions}
          loading={positionsLoading}
          scrollY={240}
          showSummary
          enableFilters
          enableKlineLink
          onOpenKline={openKlineModal}
          onClosePosition={handleClosePosition}
          accountId={id}
          exchange={account.exchange}
          getCloseButtonProps={(record) => {
            const rowKey = `${record.symbol}-${record.side}`;
            const amountNum = Number(record.amount);
            const baseDisabled = !id || !Number.isFinite(amountNum) || amountNum <= 0;
            const busyOtherRow = closingPositionKey !== null && closingPositionKey !== rowKey;
            return {
              disabled: baseDisabled || busyOtherRow,
              loading: closingPositionKey === rowKey,
            };
          }}
        />
        <div className={styles.title} style={{ marginTop: 32 }}>
          订单列表 <Tooltip title="双击订单行可查看订单详情"><InfoCircleOutlined style={{ marginLeft: 4 }} /></Tooltip>
        </div>
        <OrdersTable
          dataSource={orders}
          loading={ordersLoading}
          pagination={{
            current: orderPagination.current,
            pageSize: orderPagination.pageSize,
            total: orderPagination.total,
            showSizeChanger: true,
          }}
          enableKlineLink
          onOpenKline={openKlineModal}
          symbolFilters={orderSymbolFilters}
          symbolFilterValue={orderFilters.symbol}
          sourceFilters={orderSourceFilters}
          sourceFilterValue={orderFilters.orderSource}
          enableOnlyOnTheWayFilter
          onlyOnTheWay={onlyOnTheWay}
          showConditionsColumn
          onCancelOrder={handleCancelOrder}
          getCancelButtonProps={(order) => ({
            loading: cancelingOrderId === order.orderId,
          })}
          renderOrderDetailExtra={(order) =>
            !!order.allocations?.length && (
              <Card size="small" title="子账户分摊信息">
                <Table
                  rowKey={(record) => record.accountId}
                  size="small"
                  pagination={false}
                  dataSource={order.allocations}
                  columns={[
                    { title: '分摊账户', dataIndex: 'accountId' },
                    {
                      title: '分摊比例',
                      dataIndex: 'ratio',
                      align: 'right',
                      render: (value: string) => formatRatioAsPercent(value),
                    },
                  ]}
                />
              </Card>
            )
          }
          onChange={(pagination: any, filter: any) => {
            const selectedSymbol = Array.isArray(filter?.symbol) ? filter?.symbol[0] : undefined;
            const selectedOrderType = Array.isArray(filter?.orderType)
              ? filter?.orderType[0]
              : undefined;
            const selectedSource = Array.isArray(filter?.source) ? filter?.source[0] : undefined;
            const isOnlyOnTheWay = Array.isArray(filter?.status) && filter?.status.length > 0;
            const nextFilters = {
              symbol: selectedSymbol as string | undefined,
              orderType: selectedOrderType as OrderType | undefined,
              orderSource: selectedSource as OrderSource | undefined,
              onlyOnTheWay: isOnlyOnTheWay as boolean,
            };
            const filtersChanged =
              nextFilters.symbol !== orderFilters.symbol ||
              nextFilters.orderType !== orderFilters.orderType ||
              nextFilters.orderSource !== orderFilters.orderSource ||
              onlyOnTheWay !== isOnlyOnTheWay;
            const nextPage = pagination?.current || 1;
            const nextPageSize = pagination?.pageSize || orderPagination.pageSize;
            if (filtersChanged) {
              setOrderFilters(nextFilters);
              setOrders([]);
              setOrderPagination({
                current: 1,
                pageSize: nextPageSize,
                total: orderPagination.total,
              });
              if (onlyOnTheWay !== isOnlyOnTheWay) {
                setOnlyOnTheWay(isOnlyOnTheWay);
              }
              loadAccountOrders(nextFilters, {
                page: 1,
                pageSize: nextPageSize,
                onlyOnTheWay: isOnlyOnTheWay,
              });
              return;
            }
            if (nextPage !== orderPagination.current || nextPageSize !== orderPagination.pageSize) {
              setOrderPagination((prev) => ({
                ...prev,
                current: nextPage,
                pageSize: nextPageSize,
              }));
              loadAccountOrders(orderFilters, { page: nextPage, pageSize: nextPageSize });
            }
          }}
        />
        <div className={styles.title} style={{ marginTop: 32 }}>
          资金流水（最近7天）
        </div>
        <LedgersTable
          mode="account"
          style={{
            marginBottom: 16,
          }}
          pagination={{
            current: ledgerPagination.current,
            pageSize: ledgerPagination.pageSize,
            total: ledgerPagination.total,
            onChange: (page: number, pageSize: number) => {
              loadAccountLedgers(page, pageSize);
            },
          }}
          loading={ledgersLoading}
          dataSource={ledgers}
        />
      </Card>

      <Modal
        title="子账户信息"
        open={multiBotModalOpen}
        onCancel={() => setMultiBotModalOpen(false)}
        footer={null}
        width={1000}
        destroyOnHidden
      >
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Card title="子账户列表" size="small">
            <Table
              rowKey="accountId"
              loading={multiBotLoading}
              pagination={false}
              dataSource={multiBotSubAccounts}
              columns={[
                { title: '账户ID', dataIndex: 'accountId', width: 200 },
                { title: '账户名称', dataIndex: 'name' },
                {
                  title: '创建时间',
                  dataIndex: 'createdAt',
                  width: 200,
                  render: (value: number) =>
                    value >= 0 ? dayjs.unix(value).format('YYYY-MM-DD HH:mm:ss') : '-',
                },
              ]}
            />
          </Card>
          <Card title="资金分配表" size="small">
            <Table
              rowKey={(record) => `${record.asset}-${record.walletType}`}
              loading={multiBotLoading}
              pagination={false}
              scroll={{ x: true }}
              dataSource={multiBotAssetRows}
              columns={[
                { title: '币种', dataIndex: 'asset', align: 'center', width: 100 },
                {
                  title: '钱包类型',
                  dataIndex: 'walletType',
                  width: 120,
                  align: 'center',
                  render: (walletType: string) => {
                    const info = getWalletTypeTagInfo(walletType, { withWalletSuffix: true });
                    return <Tag color={info.color}>{info.text}</Tag>;
                  },
                },
                { title: '总资金', dataIndex: 'parentTotal', width: 120 },
                {
                  title: '未分配',
                  dataIndex: 'unallocated',
                  width: 120,
                  render: (value: string, record: any) => {
                    const unallocatedValue = Number(value);
                    const parentTotalValue = Number(record.parentTotal);
                    const ratio =
                      Number.isFinite(unallocatedValue) &&
                        Number.isFinite(parentTotalValue) &&
                        parentTotalValue > 0
                        ? unallocatedValue / parentTotalValue
                        : 0;
                    return (
                      <div>
                        <div>{value || '0'}</div>
                        <Typography.Text type="secondary">{`${(ratio * 100).toFixed(2)}%`}</Typography.Text>
                      </div>
                    );
                  },
                },
                ...multiBotSubAccounts.map((sub) => ({
                  title: <EllipsisMiddleText suffixCount={10} style={{ minWidth: 100 }}>{sub.name || sub.accountId}</EllipsisMiddleText>,
                  key: `sub-${sub.accountId}`,
                  render: (_: unknown, record: any) => {
                    const allocation = record.subAllocations?.find(
                      (item: any) => item.accountId === sub.accountId,
                    );
                    const amount = allocation?.amount || '0';
                    const amountValue = Number(amount);
                    const parentTotalValue = Number(record.parentTotal);
                    const ratio =
                      Number.isFinite(amountValue) &&
                        Number.isFinite(parentTotalValue) &&
                        parentTotalValue > 0
                        ? amountValue / parentTotalValue
                        : 0;
                    return (
                      <div>
                        <div>{amount}</div>
                        <Typography.Text type="secondary">{`${(ratio * 100).toFixed(2)}%`}</Typography.Text>
                      </div>
                    );
                  },
                })),
              ]}
            />
          </Card>
          <Card title="仓位分配表" size="small">
            <Table
              rowKey={(record) => `${record.side}-${record.symbol}`}
              loading={multiBotLoading}
              pagination={false}
              scroll={{ x: true }}
              dataSource={multiBotPositionRows}
              columns={[
                {
                  title: '交易对', dataIndex: 'symbol', width: 180,
                  align: 'center',
                  render: (value: string) => <Tag color="blue">{value}</Tag>
                },
                {
                  title: '方向',
                  dataIndex: 'side',
                  width: 80,
                  align: 'center',
                  render: (side: string) => {
                    const info = getSideTagInfo(side);
                    return <Tag color={info.color}>{info.text}</Tag>;
                  },
                },
                { title: '父账户总仓位', dataIndex: 'parentTotal', width: 120 },
                {
                  title: '未分配仓位',
                  dataIndex: 'unallocated',
                  width: 120,
                  render: (value: string, record: any) => {
                    const unallocatedValue = Number(value);
                    const parentTotalValue = Number(record.parentTotal);
                    const ratio =
                      Number.isFinite(unallocatedValue) &&
                        Number.isFinite(parentTotalValue) &&
                        parentTotalValue > 0
                        ? unallocatedValue / parentTotalValue
                        : 0;
                    return (
                      <div>
                        <div>{value || '0'}</div>
                        <Typography.Text type="secondary">{`${(ratio * 100).toFixed(2)}%`}</Typography.Text>
                      </div>
                    );
                  },
                },
                ...multiBotSubAccounts.map((sub) => ({
                  title: <EllipsisMiddleText suffixCount={10} style={{ minWidth: 100 }}>{sub.name || sub.accountId}</EllipsisMiddleText>,
                  key: `pos-sub-${sub.accountId}`,
                  render: (_: unknown, record: any) => {
                    const allocation = record.subAllocations?.find(
                      (item: any) => item.accountId === sub.accountId,
                    );
                    const amount = allocation?.amount || '0';
                    const amountValue = Number(amount);
                    const parentTotalValue = Number(record.parentTotal);
                    const ratio =
                      Number.isFinite(amountValue) &&
                        Number.isFinite(parentTotalValue) &&
                        parentTotalValue > 0
                        ? amountValue / parentTotalValue
                        : 0;
                    return (
                      <div>
                        <div>{amount}</div>
                        <Typography.Text type="secondary">{`${(ratio * 100).toFixed(2)}%`}</Typography.Text>
                      </div>
                    );
                  },
                })),
              ]}
            />
          </Card>
        </Space>
      </Modal>

      <Modal
        title="账户风控"
        open={riskModalOpen}
        onCancel={() => setRiskModalOpen(false)}
        footer={null}
        width={900}
        destroyOnHidden
      >
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Card
            title="风控配置"
            size="small"
            extra={
              <Button type="link" onClick={() => setRiskFormVisible(true)}>
                编辑
              </Button>
            }
          >
            <Flex vertical gap={8}>
              <Typography.Text type="secondary">
                仅展示已配置的限额；未配置的规则视为「未限制」。
              </Typography.Text>
              <Flex wrap="wrap" gap={24}>
                <div>
                  <Typography.Text strong>单笔订单限额</Typography.Text>
                  <br />
                  <Typography.Text>
                    {riskConfig?.maxOrderSize ? `${riskConfig.maxOrderSize} USDT` : '未限制'}
                  </Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>单标持仓限额</Typography.Text>
                  <br />
                  <Typography.Text>
                    {riskConfig?.maxPositionPerSymbol?.amount ||
                      riskConfig?.maxPositionPerSymbol?.ratio
                      ? [
                        riskConfig.maxPositionPerSymbol?.amount
                          ? `${riskConfig.maxPositionPerSymbol.amount} USDT`
                          : null,
                        riskConfig.maxPositionPerSymbol?.ratio
                          ? `${Number(riskConfig.maxPositionPerSymbol.ratio) * 100}% Equity`
                          : null,
                      ]
                        .filter(Boolean)
                        .join(' / ')
                      : '未限制'}
                  </Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>日亏损限额</Typography.Text>
                  <br />
                  <Typography.Text>
                    {riskConfig?.maxDailyLoss?.amount || riskConfig?.maxDailyLoss?.ratio
                      ? [
                        riskConfig.maxDailyLoss?.amount
                          ? `${riskConfig.maxDailyLoss.amount} USDT`
                          : null,
                        riskConfig.maxDailyLoss?.ratio
                          ? `${Number(riskConfig.maxDailyLoss.ratio) * 100}% Equity`
                          : null,
                      ]
                        .filter(Boolean)
                        .join(' / ')
                      : '未限制'}
                  </Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>最大杠杆</Typography.Text>
                  <br />
                  <Typography.Text>{riskConfig?.maxLeverage || '未限制'}</Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>下单频率限制</Typography.Text>
                  <br />
                  <Typography.Text>
                    {riskConfig?.maxOrdersPerMinute
                      ? `${riskConfig.maxOrdersPerMinute} 单/分钟`
                      : '未限制'}
                  </Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>维持保证金率下限</Typography.Text>
                  <br />
                  <Typography.Text>
                    {riskConfig?.minMaintenanceMarginRatio
                      ? `${Number(riskConfig.minMaintenanceMarginRatio) * 100}%`
                      : '未限制'}
                  </Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>净敞口限额</Typography.Text>
                  <br />
                  <Typography.Text>
                    {riskConfig?.maxTotalNetExposure?.amount ||
                      riskConfig?.maxTotalNetExposure?.ratio
                      ? [
                        riskConfig.maxTotalNetExposure?.amount
                          ? `${riskConfig.maxTotalNetExposure.amount} USDT`
                          : null,
                        riskConfig.maxTotalNetExposure?.ratio
                          ? `${Number(riskConfig.maxTotalNetExposure.ratio) * 100}% Equity`
                          : null,
                      ]
                        .filter(Boolean)
                        .join(' / ')
                      : '未限制'}
                  </Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>总敞口限额</Typography.Text>
                  <br />
                  <Typography.Text>
                    {riskConfig?.maxTotalGrossExposure?.amount ||
                      riskConfig?.maxTotalGrossExposure?.ratio
                      ? [
                        riskConfig.maxTotalGrossExposure?.amount
                          ? `${riskConfig.maxTotalGrossExposure.amount} USDT`
                          : null,
                        riskConfig.maxTotalGrossExposure?.ratio
                          ? `${Number(riskConfig.maxTotalGrossExposure.ratio) * 100}% Equity`
                          : null,
                      ]
                        .filter(Boolean)
                        .join(' / ')
                      : '未限制'}
                  </Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>风险指数阈值</Typography.Text>
                  <br />
                  <Typography.Text>
                    {riskConfig?.riskIndexThreshold || '未限制'}
                    {riskConfig?.riskIndexAction ? `（动作：${riskConfig.riskIndexAction}）` : ''}
                  </Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>冷静期</Typography.Text>
                  <br />
                  <Typography.Text>
                    {riskConfig?.cooldownSeconds
                      ? `${riskConfig.cooldownSeconds} 秒（全平后在该时间内禁止加仓）`
                      : '未限制'}
                  </Typography.Text>
                </div>
              </Flex>
            </Flex>
          </Card>

          <Card title="风控事件" size="small">
            {riskEventsLoading ? (
              <Typography.Text type="secondary">加载中...</Typography.Text>
            ) : riskEvents.length === 0 ? (
              <Typography.Text type="secondary">暂无风控事件</Typography.Text>
            ) : (
              <Space direction="vertical" style={{ width: '100%' }}>
                {riskEvents.map((evt) => (
                  <Flex
                    key={evt.id}
                    align="flex-start"
                    justify="space-between"
                    style={{ borderBottom: '1px dashed #f0f0f0', padding: '4px 0' }}
                  >
                    <div>
                      <Typography.Text strong>{evt.rule}</Typography.Text>
                      {evt.riskIndex && (
                        <Typography.Text type="secondary" style={{ marginLeft: 8 }}>
                          风险指数：{evt.riskIndex}
                        </Typography.Text>
                      )}
                      <br />
                      <Typography.Text type="secondary">
                        {dayjs.unix(evt.createdAt).format('YYYY-MM-DD HH:mm:ss')}
                      </Typography.Text>
                      {evt.payloadJson && (
                        <>
                          <br />
                          <Typography.Text type="secondary">
                            {evt.payloadJson.length > 200
                              ? `${evt.payloadJson.slice(0, 200)}...`
                              : evt.payloadJson}
                          </Typography.Text>
                        </>
                      )}
                    </div>
                    <Tag>{evt.exchange}</Tag>
                  </Flex>
                ))}
              </Space>
            )}
          </Card>
        </Space>
      </Modal>

      <Modal
        title="编辑风控配置"
        open={riskFormVisible}
        onCancel={() => setRiskFormVisible(false)}
        footer={null}
        width={900}
        destroyOnHidden
      >
        <ProForm
          layout="vertical"
          form={riskForm}
          initialValues={{
            maxOrderSize: riskConfig?.maxOrderSize,
            maxPositionPerSymbolAmount: riskConfig?.maxPositionPerSymbol?.amount,
            maxPositionPerSymbolRatio: riskConfig?.maxPositionPerSymbol?.ratio,
            maxDailyLossAmount: riskConfig?.maxDailyLoss?.amount,
            maxDailyLossRatio: riskConfig?.maxDailyLoss?.ratio,
            maxLeverage: riskConfig?.maxLeverage,
            maxOrdersPerMinute: riskConfig?.maxOrdersPerMinute,
            minMaintenanceMarginRatio: riskConfig?.minMaintenanceMarginRatio,
            maxTotalNetExposureAmount: riskConfig?.maxTotalNetExposure?.amount,
            maxTotalNetExposureRatio: riskConfig?.maxTotalNetExposure?.ratio,
            maxTotalGrossExposureAmount: riskConfig?.maxTotalGrossExposure?.amount,
            maxTotalGrossExposureRatio: riskConfig?.maxTotalGrossExposure?.ratio,
            riskIndexThreshold: riskConfig?.riskIndexThreshold,
            riskIndexAction: riskConfig?.riskIndexAction,
            cooldownSeconds: riskConfig?.cooldownSeconds ?? undefined,
          }}
          onFinish={async (values: any) => {
            if (!id) return;
            if (maxPositionPerSymbolMode === 'amount') {
              values.maxPositionPerSymbolRatio = undefined;
            } else {
              values.maxPositionPerSymbolAmount = undefined;
            }
            if (maxDailyLossMode === 'amount') {
              values.maxDailyLossRatio = undefined;
            } else {
              values.maxDailyLossAmount = undefined;
            }
            if (maxTotalNetExposureMode === 'amount') {
              values.maxTotalNetExposureRatio = undefined;
            } else {
              values.maxTotalNetExposureAmount = undefined;
            }
            if (maxTotalGrossExposureMode === 'amount') {
              values.maxTotalGrossExposureRatio = undefined;
            } else {
              values.maxTotalGrossExposureAmount = undefined;
            }
            try {
              const payload: any = {
                accountId: id,
              };
              if (values.maxOrderSize) payload.maxOrderSize = String(values.maxOrderSize);
              if (values.maxLeverage) payload.maxLeverage = String(values.maxLeverage);
              if (values.maxOrdersPerMinute)
                payload.maxOrdersPerMinute = Number(values.maxOrdersPerMinute);
              if (values.minMaintenanceMarginRatio)
                payload.minMaintenanceMarginRatio = String(values.minMaintenanceMarginRatio);
              if (values.riskIndexThreshold)
                payload.riskIndexThreshold = String(values.riskIndexThreshold);
              if (values.riskIndexAction) payload.riskIndexAction = String(values.riskIndexAction);
              if (values.cooldownSeconds !== undefined && values.cooldownSeconds !== null) {
                payload.cooldownSeconds = Number(values.cooldownSeconds);
              }

              const toAmountLimitInput = (amount?: any, ratio?: any) => {
                const hasAmount = amount !== undefined && amount !== null && amount !== '';
                const hasRatio = ratio !== undefined && ratio !== null && ratio !== '';
                if (!hasAmount && !hasRatio) return undefined;
                return {
                  amount: hasAmount ? String(amount) : undefined,
                  ratio: hasRatio ? String(ratio) : undefined,
                };
              };

              const mps = toAmountLimitInput(
                values.maxPositionPerSymbolAmount,
                values.maxPositionPerSymbolRatio,
              );
              const mdl = toAmountLimitInput(values.maxDailyLossAmount, values.maxDailyLossRatio);
              const mtn = toAmountLimitInput(
                values.maxTotalNetExposureAmount,
                values.maxTotalNetExposureRatio,
              );
              const mtg = toAmountLimitInput(
                values.maxTotalGrossExposureAmount,
                values.maxTotalGrossExposureRatio,
              );

              if (mps) payload.maxPositionPerSymbol = mps;
              if (mdl) payload.maxDailyLoss = mdl;
              if (mtn) payload.maxTotalNetExposure = mtn;
              if (mtg) payload.maxTotalGrossExposure = mtg;

              const updated = await updateAccountRiskConfig(payload);
              if (updated) {
                message.success('风控配置已更新');
                setRiskConfig(updated.config ?? null);
              }
              setRiskFormVisible(false);
            } catch (err) {
              message.error(`更新风控配置失败：${err}`);
            }
          }}
        >
          <Row>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>单笔订单限额 (USDT)</span>
                </Flex>
                <ProFormDigit
                  name="maxOrderSize"
                  fieldProps={{ precision: 2 }}
                  colProps={{ span: 12 }}
                />
              </Row>
            </Col>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>最大杠杆</span>
                </Flex>
                <ProFormDigit
                  name="maxLeverage"
                  fieldProps={{ precision: 2 }}
                  colProps={{ span: 12 }}
                />
              </Row>
            </Col>
          </Row>

          <Row>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>下单频率限制 (单/分钟)</span>
                </Flex>
                <ProFormDigit name="maxOrdersPerMinute" min={0} colProps={{ span: 12 }} />
              </Row>
            </Col>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>维持保证金率下限 (0-1)</span>
                </Flex>
                <ProFormDigit
                  name="minMaintenanceMarginRatio"
                  fieldProps={{ precision: 4, step: 0.0001 }}
                  colProps={{ span: 12 }}
                />
              </Row>
            </Col>
          </Row>

          <Row>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>单标持仓限额</span>
                  <Segmented
                    size="small"
                    value={maxPositionPerSymbolMode}
                    onChange={(val) => {
                      const mode = val as 'amount' | 'ratio';
                      setMaxPositionPerSymbolMode(mode);
                      if (mode === 'amount') {
                        riskForm.setFieldValue('maxPositionPerSymbolRatio', undefined);
                      } else {
                        riskForm.setFieldValue('maxPositionPerSymbolAmount', undefined);
                      }
                    }}
                    options={[
                      { label: '按数量', value: 'amount' },
                      { label: '按比例', value: 'ratio' },
                    ]}
                  />
                </Flex>
                {maxPositionPerSymbolMode === 'amount' ? (
                  <ProFormDigit
                    name="maxPositionPerSymbolAmount"
                    fieldProps={{ precision: 2 }}
                    colProps={{ span: 24 }}
                  />
                ) : (
                  <ProFormDigit
                    name="maxPositionPerSymbolRatio"
                    fieldProps={{ precision: 4, step: 0.0001 }}
                    colProps={{ span: 24 }}
                  />
                )}
              </Row>
            </Col>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>日亏损限额</span>
                  <Segmented
                    size="small"
                    value={maxDailyLossMode}
                    onChange={(val) => {
                      const mode = val as 'amount' | 'ratio';
                      setMaxDailyLossMode(mode);
                      if (mode === 'amount') {
                        riskForm.setFieldValue('maxDailyLossRatio', undefined);
                      } else {
                        riskForm.setFieldValue('maxDailyLossAmount', undefined);
                      }
                    }}
                    options={[
                      { label: '按数量', value: 'amount' },
                      { label: '按比例', value: 'ratio' },
                    ]}
                  />
                </Flex>
                {maxDailyLossMode === 'amount' ? (
                  <ProFormDigit
                    name="maxDailyLossAmount"
                    fieldProps={{ precision: 2 }}
                    colProps={{ span: 24 }}
                  />
                ) : (
                  <ProFormDigit
                    name="maxDailyLossRatio"
                    fieldProps={{ precision: 4, step: 0.0001 }}
                    colProps={{ span: 24 }}
                  />
                )}
              </Row>
            </Col>
          </Row>

          <Row>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>净敞口限额</span>
                  <Segmented
                    size="small"
                    value={maxTotalNetExposureMode}
                    onChange={(val) => {
                      const mode = val as 'amount' | 'ratio';
                      setMaxTotalNetExposureMode(mode);
                      if (mode === 'amount') {
                        riskForm.setFieldValue('maxTotalNetExposureRatio', undefined);
                      } else {
                        riskForm.setFieldValue('maxTotalNetExposureAmount', undefined);
                      }
                    }}
                    options={[
                      { label: '按数量', value: 'amount' },
                      { label: '按比例', value: 'ratio' },
                    ]}
                  />
                </Flex>
                {maxTotalNetExposureMode === 'amount' ? (
                  <ProFormDigit
                    name="maxTotalNetExposureAmount"
                    fieldProps={{ precision: 2 }}
                    colProps={{ span: 24 }}
                  />
                ) : (
                  <ProFormDigit
                    name="maxTotalNetExposureRatio"
                    fieldProps={{ precision: 4, step: 0.0001 }}
                    colProps={{ span: 24 }}
                  />
                )}
              </Row>
            </Col>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>总敞口限额</span>
                  <Segmented
                    size="small"
                    value={maxTotalGrossExposureMode}
                    onChange={(val) => {
                      const mode = val as 'amount' | 'ratio';
                      setMaxTotalGrossExposureMode(mode);
                      if (mode === 'amount') {
                        riskForm.setFieldValue('maxTotalGrossExposureRatio', undefined);
                      } else {
                        riskForm.setFieldValue('maxTotalGrossExposureAmount', undefined);
                      }
                    }}
                    options={[
                      { label: '按数量', value: 'amount' },
                      { label: '按比例', value: 'ratio' },
                    ]}
                  />
                </Flex>
                {maxTotalGrossExposureMode === 'amount' ? (
                  <ProFormDigit
                    name="maxTotalGrossExposureAmount"
                    fieldProps={{ precision: 2 }}
                    colProps={{ span: 24 }}
                  />
                ) : (
                  <ProFormDigit
                    name="maxTotalGrossExposureRatio"
                    fieldProps={{ precision: 4, step: 0.0001 }}
                    colProps={{ span: 24 }}
                  />
                )}
              </Row>
            </Col>
          </Row>

          <Row>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>风险指数阈值</span>
                </Flex>
                <ProFormDigit
                  name="riskIndexThreshold"
                  fieldProps={{ precision: 1, step: 0.1 }}
                  colProps={{ span: 8 }}
                />
              </Row>
            </Col>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>风险指数动作 (如 close_and_sell)</span>
                </Flex>
                <ProFormText name="riskIndexAction" colProps={{ span: 10 }} />
              </Row>
            </Col>
          </Row>

          <Row>
            <Col span={12}>
              <Row style={{ display: 'block', padding: '0 8px' }}>
                <Flex align="center" justify="space-between" style={{ marginBottom: 10 }}>
                  <span style={{ fontWeight: 'bold' }}>冷静期（秒）</span>
                </Flex>
                <ProFormDigit
                  name="cooldownSeconds"
                  min={0}
                  fieldProps={{ precision: 0 }}
                  colProps={{ span: 6 }}
                />
              </Row>
            </Col>
          </Row>
        </ProForm>
      </Modal>

      <Modal
        title="收益曲线"
        open={equityModalOpen}
        onCancel={() => setEquityModalOpen(false)}
        footer={null}
        width={900}
        destroyOnHidden
      >
        <AccountMetricsCard
          loading={metricsLoading}
          equityLoading={equityLoading}
          equityPoints={equityPoints}
          equityRange={equityRange}
          onEquityRangeChange={setEquityRange}
          metrics={accountMetrics}
          periodDays={equityRange === '1d' ? 1 : equityRange === '7d' ? 7 : 30}
        />
      </Modal>

      <AccountDebugModal
        accountId={id || ''}
        visible={debugModalVisible}
        onClose={() => setDebugModalVisible(false)}
      />

      <Modal
        open={klineVisible}
        title={
          <Space style={{ paddingLeft: 16, paddingRight: 16 }}>
            <span>K 线图：</span>
            {account?.exchange && (
              <img
                alt={klineSymbol}
                style={{ display: 'inline', marginLeft: 0, paddingBottom: 2 }}
                width={16}
                src={utils.market.getExchangeLogo(account.exchange)}
              />
            )}
            <Tag color="blue">{klineSymbol || '-'}</Tag>
          </Space>
        }
        styles={{ content: { paddingLeft: 0, paddingRight: 0, paddingTop: 10, paddingBottom: 10 } }}
        width={1100}
        destroyOnHidden
        onCancel={() => {
          setKlineVisible(false);
          setKlineSymbol('');
          setKlineMarketInfo(null);
        }}
        footer={null}
      >
        {klineSymbol && account?.exchange ? (
          <KlineChartPro
            exchange={String(account.exchange)}
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
  );
};

export default AccountDetail;
