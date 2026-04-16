import AssetHistoryModal from '@/components/Market/AssetHistoryModal';
import type { Asset } from '@/services/gateway/account';
import utils from '@/utils';
import {
  getWalletTypeTagInfo,
  walletTypeFilterOptions,
} from '@/utils/marketTag';
import type { ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import { Tag } from 'antd';
import React from 'react';

export type AssetsTableProps = {
  assets: Asset[];
  loading?: boolean;
  /** 是否展示底部汇总行（现金价值合计） */
  showSummary?: boolean;
  /** 传入后支持双击行查看资产快照历史曲线 */
  accountId?: string|null;
};

const assetColumns: ProColumns<Asset>[] = [
  {
    title: '币种',
    dataIndex: 'code',
    align: 'center',
    key: 'code',
    width: 100,
    sorter: (a, b) => String(a.code || '').localeCompare(String(b.code || '')),
  },
  {
    title: '钱包类型',
    dataIndex: 'walletType',
    key: 'walletType',
    align: 'center',
    width: 120,
    sorter: (a, b) => String(a.walletType || '').localeCompare(String(b.walletType || '')),
    filters: walletTypeFilterOptions,
    onFilter: (value, record) => record.walletType === value,
    render: (text: any) => {
      const info = getWalletTypeTagInfo(String(text || ''));
      return <Tag color={info.color}>{info.text}</Tag>;
    },
  },
  {
    title: '余额',
    dataIndex: 'balance',
    key: 'balance',
    align: 'right',
    render: (text: any) => {
      const value = utils.math.toSafeNumber(text);
      return value !== 0 ? utils.math.formatByPrecision(value, 8) : '0';
    },
  },
  {
    title: '可用',
    key: 'available',
    align: 'right',
    render: (_: any, record: Asset) => {
      const balance = parseFloat(record.balance as any);
      const locked = parseFloat(record.locked as any);
      const available = (Number.isNaN(balance) ? 0 : balance) - (Number.isNaN(locked) ? 0 : locked);
      return available !== 0 ? utils.math.formatByPrecision(available, 8) : '0';
    },
  },
  {
    title: '冻结',
    dataIndex: 'locked',
    key: 'locked',
    align: 'right',
    tooltip: '冻结资产=仓位保证金+订单冻结/占用资金',
    render: (text: any) => {
      return utils.math.formatByPrecision(text, 8);
    },
  },
  {
    title: '均价 (USDT)',
    key: 'avgPrice',
    align: 'right',
    tooltip: '按现金价值/余额计算的平均持仓价格',
    render: (_: any, record: Asset) => {
      const balance = parseFloat(record.balance as any);
      const notional = parseFloat(record.notional as any);
      const b = Number.isNaN(balance) ? 0 : balance;
      const n = Number.isNaN(notional) ? 0 : notional;
      if (b <= 0 || !Number.isFinite(n) || n === 0) {
        return '-';
      }
      const avg = n / b;
      return avg !== 0 ? utils.math.formatByPrecision(avg, 8) : '0';
    },
  },
  {
    title: '现金价值 (USDT)',
    dataIndex: 'notional',
    key: 'notional',
    align: 'right',
    defaultSortOrder: 'descend',
    sorter: (a, b) => {
      const left = parseFloat(a.notional as any);
      const right = parseFloat(b.notional as any);
      return (Number.isNaN(left) ? 0 : left) - (Number.isNaN(right) ? 0 : right);
    },
    render: (text: any) => {
      const value = parseFloat(text);
      return value !== 0 ? utils.math.formatByPrecision(text, 2) : '0';
    },
  },
];

const AssetsTable: React.FC<AssetsTableProps> = ({
  assets,
  loading = false,
  showSummary = false,
  accountId = null,
}) => {
  const [historyOpen, setHistoryOpen] = React.useState(false);
  const [pickedAsset, setPickedAsset] = React.useState<Asset | null>(null);

  const visibleAssets = React.useMemo(() => {
    return assets.filter((asset) => {
      const notional = parseFloat(asset.notional as any);
      return Number.isFinite(notional) && notional >= 0.01;
    });
  }, [assets]);

  const renderSummary: NonNullable<React.ComponentProps<typeof ProTable<Asset>>['summary']> = (pageData) => {
    if (!showSummary) {
      return null;
    }
    if (!pageData || pageData.length === 0) {
      return null;
    }
    const source = pageData.length > 0 ? pageData : assets;
    const total = source.reduce((acc, item) => {
      const value = parseFloat(item.notional as any);
      return acc + (Number.isNaN(value) ? 0 : value);
    }, 0);
    return (
      <ProTable.Summary>
        <ProTable.Summary.Row>
          <ProTable.Summary.Cell index={0} colSpan={5} align="right">
            现金价值合计 (USDT)
          </ProTable.Summary.Cell>
          <ProTable.Summary.Cell index={1} colSpan={2} align="right">
            {total > 0 ? utils.math.formatByPrecision(total, 2) : '0'}
          </ProTable.Summary.Cell>
        </ProTable.Summary.Row>
      </ProTable.Summary>
    );
  };

  return (
    <>
      <ProTable<Asset>
        style={{ marginBottom: 24 }}
        pagination={false}
        search={false}
        loading={loading}
        options={false}
        toolBarRender={false}
        dataSource={visibleAssets}
        columns={assetColumns}
        rowKey={(record) => `${record.code}-${record.walletType}`}
        summary={showSummary ? renderSummary : undefined}
        onRow={
          accountId
            ? (record) => ({
                onDoubleClick: () => {
                  setPickedAsset(record);
                  setHistoryOpen(true);
                },
              })
            : undefined
        }
      />
      {accountId && pickedAsset ? (
        <AssetHistoryModal
          open={historyOpen}
          onClose={() => {
            setHistoryOpen(false);
            setPickedAsset(null);
          }}
          accountId={accountId}
          asset={pickedAsset}
        />
      ) : null}
    </>
  );
};

export default AssetsTable;

