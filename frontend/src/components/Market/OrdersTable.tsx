import { MarketType } from '@/global.types';
import { Order, OrderCondition, OrderSource as OrderSourceEnum, OrderStatus as OrderStatusEnum, OrderType, OrderType as OrderTypeEnum, PositionSide } from '@/services/gateway/account';
import utils from '@/utils';
import { InfoCircleOutlined } from '@ant-design/icons';
import type { ParamsType, ProColumns, ProTableProps } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import { Button, Descriptions, Empty, Modal, Popover, Row, Space, Switch, Tag, Tooltip, Typography } from 'antd';
import type { FilterDropdownProps } from 'antd/es/table/interface';
import dayjs from 'dayjs';
import React, { useMemo, useState } from 'react';

export type OrdersTableProps = {
  mode?: 'all' | 'onlyOnTheWay' | 'finished';
  /** 数据源（本地控制模式） */
  dataSource?: Order[];
  /** 加载状态（本地控制模式） */
  loading?: boolean;
  pricePrecision?: number;
  volumePrecision?: number;
  /** 分页配置（本地控制模式或服务端模式均可复用） */
  pagination?: ProTableProps<Order, ParamsType>['pagination'];
  /** 表格变更回调（筛选/分页变更时触发，由外层决定如何重新拉取数据） */
  onChange?: ProTableProps<Order, ParamsType>['onChange'];
  /** ProTable request（服务端模式），如 Bot 订单列表 */
  request?: ProTableProps<Order, ParamsType>['request'];
  /** 垂直滚动高度（主要给小窗口使用） */
  scrollY?: number;
  /** 是否启用点击交易对打开 K 线 */
  enableKlineLink?: boolean;
  onOpenKline?: (symbol: string) => void;
  /** 订单 Symbol 筛选配置（账户详情） */
  symbolFilters?: { text: string; value: string }[];
  /** 当前选中的 Symbol，用于保持筛选 UI 状态 */
  symbolFilterValue?: string;
  /** 订单来源筛选配置（账户详情） */
  sourceFilters?: { text: string; value: OrderSourceEnum }[];
  /** 当前选中的来源，用于保持筛选 UI 状态 */
  sourceFilterValue?: OrderSourceEnum;
  /** 是否启用“仅显示在途单”开关（账户详情） */
  enableOnlyOnTheWayFilter?: boolean;
  /** 当前是否“仅显示在途单”，用于保持筛选 UI 状态 */
  onlyOnTheWay?: boolean;
  /** 是否展示条件列（条件单详情） */
  showConditionsColumn?: boolean;
  /** 撤单回调（传入后会显示撤单操作列，仅对在途单可用） */
  onCancelOrder?: (order: Order) => void;
  /** 撤单按钮属性（例如 loading / disabled 控制） */
  getCancelButtonProps?: (order: Order) => { disabled?: boolean; loading?: boolean } | undefined;
  /** 行双击回调（桌面端用于打开详情） */
  onRowDoubleClick?: (order: Order) => void;
  /** 是否启用内置订单详情弹窗（默认 true） */
  enableOrderDetailModal?: boolean;
  /** 订单详情弹窗扩展区域（用于页面自定义内容，如子账户分摊） */
  renderOrderDetailExtra?: (order: Order) => React.ReactNode;
};

const toNumber = (value: any) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

const renderTime = (text: any) => {
  const value = text > 0 ? dayjs.unix(text / 1000).format('YYYY-MM-DD HH:mm:ss') : '-';
  return (
    <Typography.Text type="secondary" style={{ fontVariantNumeric: 'tabular-nums' }}>
      {value}
    </Typography.Text>
  );
};

const getOrderSideLabel = (record: Order) => {
  const formtedSymbol = utils.market.parseSymbol(record.symbol);
  if (formtedSymbol.type === MarketType.Future) {
    if (record.side === PositionSide.Long) {
      return record.isBuy ? '开多' : '平多';
    }
    if (record.side === PositionSide.Short) {
      return record.isBuy ? '平空' : '开空';
    }
  } else {
    return record.isBuy ? '买入' : '卖出';
  }
  return '-';
};

const getOrderSideColor = (record: Order) => {
  if (record.side === PositionSide.Long) {
    return record.isBuy ? 'green' : 'orange';
  }
  if (record.side === PositionSide.Short) {
    return record.isBuy ? 'orange' : 'red';
  }
  return 'default';
};

const orderTypeLabelMap: Record<string, string> = {
  [OrderTypeEnum.Market]: '市价单',
  [OrderTypeEnum.Limit]: '限价单',
  MARKET: '市价单',
  LIMIT: '限价单',
};

const orderSourceLabelMap: Record<string, string> = {
  [OrderSourceEnum.User]: '用户',
  [OrderSourceEnum.Strategy]: '策略',
  [OrderSourceEnum.Liquidation]: '强平',
  [OrderSourceEnum.Adl]: 'ADL',
  USER: '用户',
  STRATEGY: '策略',
  LIQUIDATION: '强平',
  ADL: 'ADL',
};

const orderStatusLabelMap: Record<string, string> = {
  [OrderStatusEnum.New]: '新订单',
  [OrderStatusEnum.Pending]: '待处理',
  [OrderStatusEnum.Working]: '处理中',
  [OrderStatusEnum.PartialDone]: '部分成交',
  [OrderStatusEnum.Done]: '已成交',
  [OrderStatusEnum.Canceled]: '已取消',
  [OrderStatusEnum.Rejected]: '已拒绝',
  [OrderStatusEnum.Expired]: '已过期',
};

const isOnTheWayStatus = (status?: string | OrderStatusEnum) => {
  const value = String(status || '');
  return (
    value === String(OrderStatusEnum.New) ||
    value === String(OrderStatusEnum.Pending) ||
    value === String(OrderStatusEnum.Working) ||
    value === String(OrderStatusEnum.PartialDone)
  );
};

const renderOrderCondition = (order: Order, c: OrderCondition, index: number) => {
  const typeMap: Record<string, string> = {
    NONE: '计划',
    STOP_LOSS: '止损',
    TAKE_PROFIT: '止盈',
  };
  const typeLabel = typeMap[c.triggerType] || c.triggerType;

  const priceWorkingTypeMap: Record<string, string> = {
    LATEST: '最新价',
    MARK: '标记价',
    INDEX: '指数价',
  };

  let orderPrice = c.orderPrice;
  if (orderPrice === '0') {
    orderPrice = '市价';
  }

  let quoteQty = 0;
  if (c.orderPrice !== '0') {
    quoteQty = parseFloat(c.orderPrice) * parseFloat(order.originalQty);
  } else if (c.activationPrice !== '0') {
    quoteQty = parseFloat(c.activationPrice) * parseFloat(order.originalQty);
  }

  const renderConditionRow = (label: string, value: React.ReactNode) => (
    <div key={label} style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
      <div style={{ flex: 1, textAlign: 'left' }}>
        <Typography.Text type="secondary">{label}：</Typography.Text>
      </div>
      <div style={{ flexShrink: 0, textAlign: 'right' }}>{value}</div>
    </div>
  );

  const symbol = utils.market.parseSymbol(order.symbol);
  const quoteQtyRowLabel = `委托数量（${symbol.quote}）`;

  const content = (
    <div style={{ width: 280, display: 'flex', flexDirection: 'column', gap: 6 }}>
      {renderConditionRow('触发类型', typeLabel)}
      {c.activationPrice && parseFloat(c.activationPrice) > 0 && renderConditionRow('激活价格', c.activationPrice)}
      {renderConditionRow('委托价格', orderPrice)}
      {renderConditionRow(quoteQtyRowLabel, quoteQty)}
      {c.callbackRate && parseFloat(c.callbackRate) > 0 &&
        renderConditionRow('回调比例', `${utils.math.formatByPrecision(parseFloat(c.callbackRate) * 100, 2)}%`)}
      {c.callbackDistance && parseFloat(c.callbackDistance) > 0 &&
        renderConditionRow('回调距离', c.callbackDistance)}
      {c.priceWorkingType &&
        renderConditionRow('价格类型', priceWorkingTypeMap[c.priceWorkingType] || c.priceWorkingType)}
      {c.isTrailing && renderConditionRow('追踪止损', <Tag color="blue">是</Tag>)}
      {renderConditionRow(
        '已激活',
        c.activated ? (
          <Tag color="green">
            是 ({dayjs.unix(c.activatedTs / 1000).format('MM-DD HH:mm:ss')})
          </Tag>
        ) : (
          <Tag color="default">否</Tag>
        ),
      )}
    </div>
  );

  return (
    <Popover key={index} content={content} title="条件详情" trigger="hover">
      <Tag color="purple" style={{ fontSize: '11px', cursor: 'pointer' }}>
        {typeLabel}
      </Tag>
    </Popover>
  );
};

export const OrdersTable: React.FC<OrdersTableProps> = ({
  mode = 'all',
  dataSource,
  loading,
  pagination,
  onChange,
  request,
  scrollY,
  enableKlineLink,
  onOpenKline,
  symbolFilters,
  symbolFilterValue,
  sourceFilters,
  sourceFilterValue,
  enableOnlyOnTheWayFilter,
  onlyOnTheWay,
  showConditionsColumn,
  pricePrecision,
  volumePrecision,
  onCancelOrder,
  getCancelButtonProps,
  onRowDoubleClick,
  enableOrderDetailModal = true,
  renderOrderDetailExtra,
}) => {
  const [orderDetailOpen, setOrderDetailOpen] = useState(false);
  const [selectedOrder, setSelectedOrder] = useState<Order | null>(null);

  const openOrderDetail = (order: Order) => {
    setSelectedOrder(order);
    setOrderDetailOpen(true);
  };

  const columns: ProColumns<Order>[] = useMemo(() => {
    return [
      {
        title: '交易对',
        dataIndex: 'symbol',
        align: 'center',
        key: 'symbol',
        width: 150,
        filters: symbolFilters,
        filteredValue: symbolFilterValue ? [symbolFilterValue] : undefined,
        onFilter: symbolFilters ? (value, record) => record.symbol === value : undefined,
        render: (text: any) => {
          const symbol = String(text || '');
          if (!symbol) return '-';
          if (!enableKlineLink || !onOpenKline) {
            return <Tag color="blue">{symbol}</Tag>;
          }
          return (
            <Typography.Link
              onClick={(e) => {
                e.preventDefault();
                onOpenKline(symbol);
              }}
            >
              <Tag color="blue">{symbol}</Tag>
            </Typography.Link>
          );
        },
      },
      {
        title: '订单ID',
        dataIndex: 'orderId',
        ellipsis: true,
        copyable: true,
        key: 'orderId',
        width: 160,
        fixed: 'left',
        render: (text: any, record: Order) => {
          if (record.drivedOrderId && record.drivedOrderId !== '' && record.drivedOrderId !== '0') {
            return (
              <Space>
                <span>{text}</span>
                <Tooltip
                  title={
                    <div style={{ fontSize: '12px', color: '#999' }}>
                      子订单ID：{record.drivedOrderId}
                    </div>
                  }
                >
                  <InfoCircleOutlined style={{ color: '#999' }} />
                </Tooltip>
              </Space>
            );
          }
          return text;
        },
      },
      {
        title: '方向',
        dataIndex: 'side',
        align: 'center',
        key: 'side',
        width: 80,
        fixed: 'left',
        render: (_: any, record: Order) => {
          const label = getOrderSideLabel(record);
          const color = getOrderSideColor(record);
          return <Tag color={color}>{label}</Tag>;
        },
      },
      {
        title: '订单类型',
        dataIndex: 'orderType',
        align: 'center',
        key: 'orderType',
        width: 100,
        render: (text: any, record: Order) => {
          const typeLabel = orderTypeLabelMap[text as string] || text;
          if (record.algoType && record.algoType !== 'none' && record.algoType !== 'unknown') {
            return (
              <Row align="middle" justify="center">
                <span>{typeLabel}</span>
                <Typography.Text type="secondary" style={{ fontSize: '10px', marginTop: 0, paddingTop: 0 }}>
                  {record.algoType}
                </Typography.Text>
              </Row>
            );
          }
          return typeLabel;
        },
      },
      {
        title: '来源',
        dataIndex: 'source',
        align: 'center',
        key: 'source',
        width: 90,
        filters: sourceFilters,
        filteredValue: sourceFilterValue ? [sourceFilterValue] : undefined,
        render: (text: any) => {
          const info = orderSourceLabelMap[text as string] || text;
          const colorMap: Record<string, string> = {
            [OrderSourceEnum.User]: 'default',
            [OrderSourceEnum.Strategy]: 'blue',
            [OrderSourceEnum.Liquidation]: 'red',
            [OrderSourceEnum.Adl]: 'orange',
            USER: 'default',
            STRATEGY: 'blue',
            LIQUIDATION: 'red',
            ADL: 'orange',
          };
          return <Tag color={colorMap[text as string] || 'default'}>{info}</Tag>;
        },
      },
      {
        title: '状态',
        dataIndex: 'status',
        align: 'center',
        key: 'status',
        width: 100,
        filteredValue: enableOnlyOnTheWayFilter && onlyOnTheWay ? ['on'] : undefined,
        filterDropdown: enableOnlyOnTheWayFilter
          ? ({ setSelectedKeys, selectedKeys, confirm }: FilterDropdownProps) => {
            const only = Array.isArray(selectedKeys) && selectedKeys.length > 0;
            return (
              <div style={{ padding: 8 }}>
                <Space>
                  <Typography.Text>仅显示在途单</Typography.Text>
                  <Switch
                    checked={only}
                    onChange={(value) => {
                      setSelectedKeys(value ? ['on'] : []);
                      confirm({ closeDropdown: false });
                    }}
                  />
                </Space>
              </div>
            );
          }
          : undefined,
        onFilter: (value, record) => {
          if (value === 'on') {
            return isOnTheWayStatus(record.status);
          }
          return true;
        },
        render: (text: any, record: Order) => {
          const statusLabel = orderStatusLabelMap[text as string] || text;
          if (record.rejectReason && record.rejectReason !== '') {
            return (
              <Space>
                <Typography.Text>{statusLabel}</Typography.Text>
                {record.status === OrderStatusEnum.Rejected && (
                  <Tooltip title={record.rejectReason}>
                    <InfoCircleOutlined style={{ color: '#fa8c16' }} />
                  </Tooltip>
                )}
              </Space>
            );
          }
          return statusLabel;
        },
      },
      {
        title: '数量',
        dataIndex: 'originalQty',
        key: 'originalQty',
        align: 'right',
        width: 120,
        render: (v: any, row: Order) => {
          return utils.math.formatByPrecision(v, 8);
        },
      },
      {
        title: '价格',
        dataIndex: 'price',
        key: 'price',
        align: 'right',
        width: 120,
        render: (v: any, row: Order) => {
          if (row.orderType === OrderType.Market) {
            return '市价';
          }
          return utils.math.formatByPrecision(v, pricePrecision);
        },
      },
      {
        title: '已成交',
        hidden: mode === 'onlyOnTheWay',
        dataIndex: 'executedQty',
        key: 'executedQty',
        align: 'right',
        width: 120,
        render: (v: any, row: Order) => {
          return utils.math.formatByPrecision(v, 8);
        },
      },
      {
        title: '成交均价',
        hidden: mode === 'onlyOnTheWay',
        dataIndex: 'avgPrice',
        key: 'avgPrice',
        align: 'right',
        width: 120,
        render: (v: any) => utils.math.formatByPrecision(v, pricePrecision),
      },
      {
        title: '成交价值',
        hidden: mode === 'onlyOnTheWay',
        key: 'executedValue',
        align: 'right',
        width: 140,
        render: (_: any, record: Order) => {
          const executedQty = toNumber(record.executedQty);
          const avgPrice = toNumber(record.avgPrice);
          const value = executedQty * avgPrice;
          return value !== 0 ? utils.math.formatByPrecision(value, 4) : '-';
        },
      },
      {
        title: '条件',
        hidden: showConditionsColumn === false,
        key: 'conditions',
        align: 'center',
        width: 160,
        render: (_, record: Order) => {
          if (record.conditions && record.conditions.length > 0) {
            return (
              <Space direction="vertical" size={4}>
                {record.conditions.map((c, index) => renderOrderCondition(record, c, index))}
              </Space>
            );
          }
          return '-';
        },
      },
      {
        title: '剩余占用/冻结',
        dataIndex: 'locked',
        key: 'locked',
        align: 'right',
        width: 160,
        tooltip: (
          <div>
            剩余占用/冻结：
            <br />
            - 未成交部分占用的资产；
            <br />
            - 现货：剩余冻结资产；
            <br />
            - 合约：开仓场景剩余保证金，平仓不占用资产；
          </div>
        ),
        render: (text: any, record: Order) => {
          if (!text || text === '0') {
            return '-';
          }
          const value = toNumber(text);
          const color = value >= 0 ? 'green' : 'red';
          const display = utils.math.formatByPrecision(text, 8);
          const label = record.lockedAsset ? `${display} ${record.lockedAsset}` : display;
          return <span style={{ color }}>{label}</span>;
        },
      },
      {
        title: '手续费',
        hidden: mode === 'onlyOnTheWay',
        dataIndex: 'fee',
        key: 'fee',
        align: 'right',
        width: 160,
        tooltip: '已成交部分的手续费',
        render: (text: any, record: Order) => {
          if (!text || text === '0') {
            return '-';
          }
          const value = toNumber(text);
          const color = value >= 0 ? 'green' : 'red';
          const display = utils.math.formatByPrecision(text, 8);
          const label = record.feeAsset ? `${display} ${record.feeAsset}` : display;
          return <span style={{ color }}>{label}</span>;
        },
      },
      {
        title: '已实现收益',
        hidden: mode === 'onlyOnTheWay',
        dataIndex: 'realizedPnl',
        key: 'realizedPnl',
        align: 'right',
        width: 120,
        tooltip: '合约平仓或现货卖出产生的已实现收益，不含手续费；pnlAsset 表示收益计价资产',
        render: (text: any, record: Order) => {
          if (!text || text === '0') {
            return '-';
          }
          const value = toNumber(text);
          const color = value >= 0 ? 'green' : 'red';
          const display = utils.math.formatByPrecision(text, 8);
          const label = record.pnlAsset ? `${display} ${record.pnlAsset}` : display;
          return <span style={{ color }}>{label}</span>;
        },
      },
      {
        title: '结束时间',
        hidden: mode === 'onlyOnTheWay',
        dataIndex: 'finishedTs',
        key: 'finishedTs',
        width: 170,
        render: renderTime,
      },
      {
        title: '创建时间',
        dataIndex: 'createdTs',
        key: 'createdTs',
        width: 170,
        fixed: 'right',
        render: renderTime,
      },
      {
        title: '操作',
        hidden: mode !== 'onlyOnTheWay',
        key: 'actions',
        align: 'center',
        width: 100,
        fixed: 'right' as const,
        render: (_: any, record: Order) => {
          const baseDisabled = !isOnTheWayStatus(record.status);
          const extra = getCancelButtonProps?.(record) || {};
          const disabled = baseDisabled || !!extra.disabled;
          return (
            <Button
              size="small"
              danger
              disabled={disabled}
              loading={!!extra.loading}
              onClick={() => {
                if (disabled) return;
                onCancelOrder?.(record);
              }}
            >
              撤单
            </Button>
          );
        },
      },
    ];
  }, [
    enableKlineLink,
    onOpenKline,
    symbolFilters,
    symbolFilterValue,
    sourceFilters,
    sourceFilterValue,
    enableOnlyOnTheWayFilter,
    onlyOnTheWay,
    showConditionsColumn,
    onCancelOrder,
    getCancelButtonProps,
    pricePrecision,
  ]);

  const scroll = scrollY ? { x: 'max-content' as const, y: scrollY } : { x: 'max-content' as const };

  return (
    <>
      <ProTable<Order>
        style={{ marginBottom: 24 }}
        pagination={pagination}
        search={false}
        loading={loading}
        options={false}
        toolBarRender={false}
        dataSource={request ? undefined : dataSource}
        request={request}
        columns={columns}
        rowKey={(record) => record.orderId || record.clientOrderId || `${record.orderId}-${record.symbol}-${record.side}`}
        scroll={scroll}
        onChange={onChange}
        onRow={(record) => ({
          onDoubleClick: () => {
            if (enableOrderDetailModal) {
              openOrderDetail(record);
            }
            onRowDoubleClick?.(record);
          },
        })}
      />
      <Modal
        title="订单详情"
        open={orderDetailOpen}
        onCancel={() => {
          setOrderDetailOpen(false);
          setSelectedOrder(null);
        }}
        footer={null}
        width={760}
        destroyOnHidden
      >
        {selectedOrder ? (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions column={2} size="small" bordered>
              <Descriptions.Item label="订单ID">{selectedOrder.orderId || '-'}</Descriptions.Item>
              <Descriptions.Item label="客户端订单ID">{selectedOrder.clientOrderId || '-'}</Descriptions.Item>
              <Descriptions.Item label="交易对">{selectedOrder.symbol || '-'}</Descriptions.Item>
              <Descriptions.Item label="方向">{selectedOrder.isBuy ? '买入' : '卖出'}</Descriptions.Item>
              <Descriptions.Item label="仓位方向">{selectedOrder.side || '-'}</Descriptions.Item>
              <Descriptions.Item label="订单类型">
                {orderTypeLabelMap[selectedOrder.orderType] || selectedOrder.orderType || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="来源">
                {orderSourceLabelMap[selectedOrder.source] || selectedOrder.source || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="状态">
                {orderStatusLabelMap[selectedOrder.status] || selectedOrder.status || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="委托价格">{selectedOrder.price || '-'}</Descriptions.Item>
              <Descriptions.Item label="委托数量">
                {selectedOrder.originalQty
                  ? `${selectedOrder.originalQty} ${utils.market.parseSymbol(selectedOrder.symbol).base || ''}`.trim()
                  : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="已成交数量">
                {selectedOrder.executedQty
                  ? `${selectedOrder.executedQty} ${utils.market.parseSymbol(selectedOrder.symbol).base || ''}`.trim()
                  : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="成交均价">{selectedOrder.avgPrice || '-'}</Descriptions.Item>
              <Descriptions.Item label="创建时间">
                {selectedOrder.createdTs > 0
                  ? dayjs.unix(selectedOrder.createdTs / 1000).format('YYYY-MM-DD HH:mm:ss')
                  : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="结束时间">
                {selectedOrder.finishedTs > 0
                  ? dayjs.unix(selectedOrder.finishedTs / 1000).format('YYYY-MM-DD HH:mm:ss')
                  : '-'}
              </Descriptions.Item>
            </Descriptions>
            {renderOrderDetailExtra?.(selectedOrder)}
          </Space>
        ) : (
          <Empty description="订单详情为空" />
        )}
      </Modal>
    </>
  );
};

export default OrdersTable;

