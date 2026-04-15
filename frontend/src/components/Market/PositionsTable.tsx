import { Position } from '@/services/gateway/account';
import utils from '@/utils';
import { getSideTagInfo, sideFilterOptions } from '@/utils/marketTag';
import type { ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import { Button, Tag } from 'antd';
import Decimal from 'decimal.js';
import React, { useMemo } from 'react';

export type PositionsProTableProps = {
  positions: Position[];
  loading?: boolean;
  /** 垂直滚动高度（主要用于底部小窗口） */
  scrollY?: number;
  /** 是否展示底部汇总行（现金价值合计/未实现盈亏） */
  showSummary?: boolean;
  /** 是否启用按交易对/方向的筛选 */
  enableFilters?: boolean;
  /** 是否启用点击交易对打开 K 线 */
  enableKlineLink?: boolean;
  onOpenKline?: (symbol: string) => void;
  /** 传入则展示“平仓”操作列，由外部处理具体平仓逻辑 */
  onClosePosition?: (position: Position) => void;
  /** 自定义平仓按钮的 disabled/loading 等状态 */
  getCloseButtonProps?: (position: Position) => { disabled?: boolean; loading?: boolean };
};

const toNumber = (value: any) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

const safeDecimal = (raw: any): Decimal => {
  const s = String(raw ?? '')
    .replace(/,/g, '')
    .trim();
  if (!s) return new Decimal(0);
  try {
    const d = new Decimal(s);
    return d.isFinite() ? d : new Decimal(0);
  } catch {
    return new Decimal(0);
  }
};

/** 与单元格展示的字符串一致：按字面小数位数（不剥末尾 0），科学计数法回退到 math 工具推断。 */
const displayFractionDigits = (raw: any): number => {
  const t = String(raw ?? '')
    .replace(/,/g, '')
    .trim();
  if (!t) return 0;
  if (t.includes('e') || t.includes('E')) {
    return utils.math.getDecimalPrecision(t);
  }
  const dot = t.indexOf('.');
  if (dot < 0) return 0;
  return Math.max(0, t.length - dot - 1);
};

const PositionsProTable: React.FC<PositionsProTableProps> = ({
  positions,
  loading = false,
  scrollY,
  showSummary = false,
  enableFilters = false,
  enableKlineLink = false,
  onOpenKline,
  onClosePosition,
  getCloseButtonProps,
}) => {
  const symbolFilters = useMemo(
    () =>
      Array.from(new Set(positions.map((item) => item.symbol).filter(Boolean))).map((symbol) => ({
        text: symbol as string,
        value: symbol as string,
      })),
    [positions],
  );

  const columns: ProColumns<Position>[] = useMemo(() => {
    return [
      {
        title: '交易对',
        dataIndex: 'symbol',
        align: 'center',
        fixed: 'left',
        key: 'symbol',
        width: 160,
        sorter: (a, b) => String(a.symbol || '').localeCompare(String(b.symbol || '')),
        filters: enableFilters ? symbolFilters : undefined,
        onFilter: enableFilters ? (value, record) => record.symbol === value : undefined,
        render: (text: any) => {
          const symbol = String(text || '');
          if (!symbol) return '-';
          if (!enableKlineLink || !onOpenKline) {
            return symbol;
          }
          return (
            <a
              onClick={(e) => {
                e.preventDefault();
                onOpenKline(symbol);
              }}
            >
              <Tag color="blue">{symbol}</Tag>
            </a>
          );
        },
      },
      {
        title: '方向',
        dataIndex: 'side',
        align: 'center',
        fixed: 'left',
        key: 'side',
        width: 80,
        sorter: enableFilters ? (a, b) => String(a.side || '').localeCompare(String(b.side || '')) : undefined,
        filters: enableFilters ? sideFilterOptions : undefined,
        onFilter: enableFilters ? (value, record) => String(record.side || '') === String(value) : undefined,
        render: (text: any) => {
          const info = getSideTagInfo(String(text || ''));
          return <Tag color={info.color}>{info.text}</Tag>;
        },
      },
      {
        title: '杠杆',
        dataIndex: 'leverage',
        key: 'leverage',
        width: 80,
        fixed: 'left',
        align: 'center',
        render: (text: any) => `${text}x`,
      },
      {
        title: '数量',
        width: 120,
        dataIndex: 'amount',
        key: 'amount',
        align: 'right',
      },
      {
        title: '开仓均价',
        width: 160,
        dataIndex: 'entryPrice',
        key: 'entryPrice',
        align: 'right',
      },
      {
        title: '标记价格',
        width: 160,
        dataIndex: 'markPrice',
        key: 'markPrice',
        align: 'right',
      },
      {
        title: '现金价值（USDT）',
        width: 180,
        dataIndex: 'notional',
        key: 'notional',
        align: 'right',
      },
      {
        title: '保证金',
        width: 100,
        dataIndex: 'initialMargin',
        key: 'initialMargin',
        align: 'right',
      },
      {
        title: '强平价格',
        width: 120,
        dataIndex: 'liquidationPrice',
        key: 'liquidationPrice',
        align: 'right',
      },
      {
        title: '未实现盈亏（USDT）',
        width: 180,
        dataIndex: 'unRealizedProfit',
        key: 'unRealizedProfit',
        align: 'right',
        defaultSortOrder: 'ascend',
        sorter: (a, b) => toNumber(a.unRealizedProfit) - toNumber(b.unRealizedProfit),
        render: (text: any) => {
          const value = parseFloat(String(text));
          const color = value >= 0 ? 'green' : 'red';
          return <span style={{ color }}>{text}</span>;
        },
      },
      {
        title: '操作',
        key: 'action',
        fixed: 'right',
        align: 'center',
        hidden: onClosePosition ? false : true,
        width: 160,
        render: (_: any, record: Position) => {
          const btnProps = getCloseButtonProps?.(record) ?? {};
          return (
            <Button
              size="small"
              danger
              onClick={() => {
                onClosePosition?.(record);
              }}
              {...btnProps}
            >
              平仓
            </Button>
          );
        },
      }
    ];
  }, [enableFilters, enableKlineLink, getCloseButtonProps, onClosePosition, onOpenKline, symbolFilters]);

  const renderSummary: NonNullable<React.ComponentProps<typeof ProTable<Position>>['summary']> | undefined = showSummary
    ? (pageData) => {
      if (!pageData || pageData.length === 0) {
        return null;
      }
      const source = pageData.length > 0 ? pageData : positions;
      const totals = source.reduce(
        (acc, item) => {
          acc.netValue += toNumber(item.notional);
          acc.margin += toNumber(item.initialMargin);
          acc.unRealized = acc.unRealized.plus(safeDecimal(item.unRealizedProfit));
          acc.unRealizedFrac = Math.max(acc.unRealizedFrac, displayFractionDigits(item.unRealizedProfit));
          return acc;
        },
        { netValue: 0, margin: 0, unRealized: new Decimal(0), unRealizedFrac: 0 },
      );
      const unRealizedText = totals.unRealized.toFixed(totals.unRealizedFrac);
      const unRealizedNum = totals.unRealized.toNumber();
      return (
        <ProTable.Summary>
          <ProTable.Summary.Row>
            <ProTable.Summary.Cell index={0} colSpan={6} align="right">
              合计
            </ProTable.Summary.Cell>
            <ProTable.Summary.Cell index={1} align="right">
              {totals.netValue !== 0 ? utils.math.formatByPrecision(totals.netValue, 8) : '0'}
            </ProTable.Summary.Cell>
            <ProTable.Summary.Cell index={3} colSpan={2} />
            <ProTable.Summary.Cell index={4} align="right">
              {unRealizedNum >= 0 ? (
                <span style={{ color: '#52c41a' }}>{unRealizedText}</span>
              ) : (
                <span style={{ color: '#ff4d4f' }}>{unRealizedText}</span>
              )}
            </ProTable.Summary.Cell>
            <ProTable.Summary.Cell index={5} colSpan={1} />
          </ProTable.Summary.Row>
        </ProTable.Summary>
      );
    }
    : undefined;

  return (
    <ProTable<Position>
      style={{ marginBottom: 24 }}
      pagination={false}
      search={false}
      loading={loading}
      options={false}
      toolBarRender={false}
      dataSource={positions}
      columns={columns}
      rowKey={(record) => `${record.symbol}-${record.side}`}
      summary={renderSummary}
      scroll={{ y: scrollY }}
    />
  );
};

export default PositionsProTable;

