import type { AccountProfitRankItem } from '@/services/gateway/dashboard';
import utils from '@/utils';
import { getExchangeLogo } from '@/utils/market';
import { Avatar, Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';

type AccountProfitRankTableProps = {
  loading: boolean;
  data: AccountProfitRankItem[];
};

const AccountProfitRankTable: React.FC<AccountProfitRankTableProps> = ({ loading, data }) => {
  const formatNum = (v: string) => {
    const n = parseFloat(v);
    if (Number.isNaN(n)) return '-';
    const prefix = n >= 0 ? '+' : '';
    return `${prefix}${n.toFixed(4)}`;
  };

  const sortByNotional24HChange = (a: AccountProfitRankItem, b: AccountProfitRankItem) => {
    const va = a.notional24HChange ? parseFloat(a.notional24HChange) : 0;
    const vb = b.notional24HChange ? parseFloat(b.notional24HChange) : 0;
    return vb - va;
  };

  const columns: ColumnsType<AccountProfitRankItem> = [
    {
      title: '账户',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '交易所',
      dataIndex: 'exchange',
      key: 'exchange',
      align: 'center',
      width: 100,
      render: (exchange: string) => (
        <Avatar src={getExchangeLogo(exchange)} size={24} shape="square" />
      ),
    },
    {
      title: '名义价值',
      dataIndex: 'notional',
      key: 'notional',
      align: 'right',
      render: (v: string) => utils.math.formatByPrecision(v, 2),
    },
    {
      title: '未实现盈亏',
      dataIndex: 'unRealizedProfit',
      key: 'unRealizedProfit',
      align: 'right',
      render: (v: string) => {
        const n = parseFloat(v);
        return <span style={{ color: n >= 0 ? '#52c41a' : '#ff4d4f' }}>{formatNum(v)}</span>;
      },
    },
    {
      title: '24h 收益',
      dataIndex: 'notional24HChange',
      key: 'notional24HChange',
      align: 'right',
      render: (v: string) => {
        const n = parseFloat(v);
        return <span style={{ color: n >= 0 ? '#52c41a' : '#ff4d4f' }}>{formatNum(v)}</span>;
      },
    },
  ];

  return (
    <Table<AccountProfitRankItem>
      loading={loading}
      dataSource={data.sort(sortByNotional24HChange)}
      columns={columns}
      rowKey="id"
      pagination={false}
      size="small"
    />
  );
};

export default AccountProfitRankTable;
