import { Trade } from '@/services/gateway/market';
import { InfoCircleOutlined } from '@ant-design/icons';
import { Space, Table, Tooltip, Typography } from 'antd';
import dayjs from 'dayjs';
import React from 'react';
import utils from '@/utils';

const tableTitle = (title: string) => (
  <Typography.Text style={{ fontSize: 12 }}>{title}</Typography.Text>
);

export type RecentTradesTableProps = {
  trades: Trade[];
  quoteAsset: string;
  monoPriceStyle: React.CSSProperties;
  pricePrecision: number;
  volumePrecision: number;
  scrollY?: number;
};

const formatPrice = (value: string | number | null | undefined, precision?: number, empty: string = '--') => {
  if (value === null || value === undefined) return empty;
  const rawStr = String(value).replace(/,/g, '').trim();
  if (!rawStr) return empty;
  const n = Number(rawStr);
  if (!Number.isFinite(n)) return empty;
  if (!Number.isFinite(precision as number) || (precision as number) < 0) return rawStr;
  return n.toFixed(precision as number);
};

const RecentTradesTable: React.FC<RecentTradesTableProps> = ({
  trades,
  quoteAsset,
  monoPriceStyle,
  pricePrecision,
  volumePrecision,
  scrollY = 462,
}) => (
  <Table<Trade>
    size="small"
    pagination={false}
    rowKey={(row) => `${row.tradeId}-${row.ts}`}
    onRow={() => ({ style: { lineHeight: '6px', fontSize: 12 } })}
    dataSource={trades}
    scroll={{ y: scrollY }}
    columns={[
      {
        title: tableTitle(`价格`),
        dataIndex: 'price',
        align: 'left',
        render: (v, row) => (
          <span
            style={{
              ...monoPriceStyle,
              color: row.isBuy ? '#52c41a' : '#ff4d4f',
            }}
          >
            {formatPrice(v, pricePrecision)}
          </span>
        ),
      },
      {
        // title: tableTitle(`数量（${quoteAsset || '--'}）`),
        title: (<Space>
          <Typography.Text style={{ fontSize: 12 }}>数量</Typography.Text>
          <Tooltip title={`计价币种：${quoteAsset || '--'}`}><InfoCircleOutlined /></Tooltip>
        </Space>),
        dataIndex: 'size',
        align: 'right',
        render: (_v, row) => utils.math.formatByPrecision(row?.size, volumePrecision),
      },
      {
        title: tableTitle('时间'),
        dataIndex: 'ts',
        width: 70,
        align: 'right',
        render: (v) => dayjs(v).format('HH:mm:ss'),
      },
    ]}
  />
);

export default RecentTradesTable;
