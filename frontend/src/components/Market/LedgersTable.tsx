import { Ledger, PositionSide } from '@/services/gateway/account';
import utils from '@/utils';
import { getWalletTypeTagInfo } from '@/utils/marketTag';
import { InfoCircleOutlined } from '@ant-design/icons';
import { ParamsType, ProColumns, ProTable, ProTableProps } from '@ant-design/pro-components';
import { Popover, Space, Tag, Typography } from 'antd';
import dayjs from 'dayjs';
import type { FC, ReactNode } from 'react';

const renderTime = (text: any) => {
  const value = text > 0 ? dayjs.unix(text / 1000).format('YYYY-MM-DD HH:mm:ss') : '-';
  return (
    <Typography.Text type="secondary" style={{ fontVariantNumeric: 'tabular-nums' }}>
      {value}
    </Typography.Text>
  );
};

export type LedgerColumnsOptions = {
  showDetailColumn?: boolean;
};

export const buildLedgerColumns = (options: LedgerColumnsOptions = {}): ProColumns<Ledger>[] => {
  const { showDetailColumn = false } = options;

  const typeMap: Record<string, string> = {
    SNAPSHOT: '快照',
    DEPOSIT: '充值',
    WITHDRAW: '提现',
    WITHDRAW_REJECT: '提现失败',
    ORDER: '成交',
    FILL: '成交',
    FUNDING_FEE: '资金费',
    DELIVERED: '交割',
    EXERCISED: '行权',
    TRANSFERRED: '划转',
    LIQUIDATION: '强平',
    CLAW_BACK: '穿仓补偿',
    ADL: '自动减仓',
    ADJUSTMENT: '调整',
    SET_LEVERAGE: '设置杠杆',
    INTEREST_DEDUCTION: '扣息',
    SETTLEMENT: '交割结算',
    INSURANCE_CLEAR: '保险清算',
    ADMIN_DEPOSIT: '管理员充值',
    ADMIN_WITHDRAW: '管理员提现',
    MARGIN_TRANSFER: '保证金划转',
    MARGIN_TYPE_CHANGE: '保证金模式变更',
    ASSET_TRANSFER: '资产划转',
    OPTIONS_PREMIUM_FEE: '期权权利金',
    OPTIONS_SETTLE_PROFIT: '期权结算收益',
    AUTO_EXCHANGE: '自动兑换',
    COIN_SWAP_DEPOSIT: '币本位充值',
    COIN_SWAP_WITHDRAW: '币本位提现',
    FUNDS_FREEZE: '资金冻结',
    FUNDS_UNFREEZE: '资金解冻',
    ORDER_MARGIN_FREEZE: '订单保证金冻结',
    ORDER_MARGIN_UNFREEZE: '订单保证金解冻',
  };

  const columns: ProColumns<Ledger>[] = [
    {
      title: '时间',
      dataIndex: 'ts',
      key: 'ts',
      width: 180,
      align: 'left',
      render: renderTime,
    },
    {
      title: '币种',
      dataIndex: 'asset',
      key: 'asset',
      width: 100,
      align: 'center',
    },
    {
      title: '钱包类型',
      dataIndex: 'walletType',
      key: 'walletType',
      width: 120,
      align: 'center',
      render: (text: any) => {
        const info = getWalletTypeTagInfo(String(text || ''));
        return <Tag color={info.color}>{info.text}</Tag>;
      },
    },
    {
      title: '事件类型',
      dataIndex: 'type',
      key: 'type',
      width: 120,
      align: 'center',
      render: (text: any) => {
        const label = typeMap[String(text)] || text || '-';
        return <Tag>{label}</Tag>;
      },
    },
    {
      title: '是否生效',
      dataIndex: 'isEffective',
      key: 'isEffective',
      align: 'center',
      width: 100,
      render: (text: any) => {
        return <Tag color={text ? 'green' : 'red'}>{text ? '是' : '否'}</Tag>;
      },
    },
    {
      title: '账户余额',
      dataIndex: 'total',
      key: 'total',
      align: 'right',
      render: (text: any, record: Ledger) => {
        const delta = parseFloat(record.totalDelta);
        const value = Number.isFinite(delta) ? delta : 0;
        const color = value === 0 ? '#999' : value > 0 ? 'green' : 'red';
        const display = utils.math.formatByPrecision(text, 8);
        const deltaDisplay = utils.math.formatByPrecision(record.totalDelta, 8);
        return (
          <Space>
            <span>{display}</span>
            <span>
              (
              <span style={{ color }}>{`${value > 0 ? '+' : ''}${deltaDisplay}`}</span>
              )
            </span>
          </Space>
        );
      },
    },
    {
      title: '冻结资金',
      dataIndex: 'frozen',
      key: 'frozen',
      align: 'right',
      render: (text: any, record: Ledger) => {
        const delta = parseFloat(record.frozenDelta);
        const value = Number.isFinite(delta) ? delta : 0;
        const color = value === 0 ? '#999' : value > 0 ? 'green' : 'red';
        const display = utils.math.formatByPrecision(text, 8);
        const deltaDisplay = utils.math.formatByPrecision(record.frozenDelta, 8);
        return (
          <Space>
            <span>{display}</span>
            <span>
              (
              <span style={{ color }}>{`${value > 0 ? '+' : ''}${deltaDisplay}`}</span>
              )
            </span>
          </Space>
        );
      },
    },
  ];

  if (showDetailColumn) {
    const renderDetailRow = (label: string, value: ReactNode) => (
      <div key={label} style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <div style={{ flex: 1, textAlign: 'left' }}>
          <Typography.Text type="secondary">{label}：</Typography.Text>
        </div>
        <div style={{ flexShrink: 0, textAlign: 'right' }}>{value}</div>
      </div>
    );

    columns.push({
      title: '详情',
      dataIndex: 'detail',
      key: 'detail',
      width: 80,
      align: 'center',
      render: (text: any, record: Ledger, index: number) => {
        if (!text || text === '-') {
          return '-';
        }

        let content: ReactNode = null;
        try {
          const detail = JSON.parse(text);
          const rows: ReactNode[] = [];

          if (detail.orderId) {
            rows.push(renderDetailRow('订单', detail.orderId));
          }
          if (detail.symbol) {
            rows.push(renderDetailRow('交易对', detail.symbol));
          }
          if (detail?.symbol?.endsWith(':FUTURE')) {
            if (detail.isBuy && detail.side === PositionSide.Long) {
              rows.push(renderDetailRow('方向', '开多'));
            } else if (detail.isBuy && detail.side === PositionSide.Short) {
              rows.push(renderDetailRow('方向', '开空'));
            } else if (!detail.isBuy && detail.side === PositionSide.Long) {
              rows.push(renderDetailRow('方向', '平多'));
            } else {
              rows.push(renderDetailRow('方向', '平空'));
            }
          } else if (detail.isBuy) {
            rows.push(renderDetailRow('方向', '买'));
          }

          if (detail.fillQty && detail.fillPrice) {
            rows.push(renderDetailRow('成交', `${detail.fillQty} @ ${detail.fillPrice}`));
          }
          if (detail.fee) {
            const feeAsset = detail.feeAsset ? ` ${detail.feeAsset}` : '';
            rows.push(renderDetailRow('手续费', `${detail.fee}${feeAsset}`));
          }
          if (detail.pnl) {
            rows.push(renderDetailRow('收益', detail.pnl));
          }

          if (rows.length === 0) {
            return '-';
          }

          content = (
            <div style={{ width: 260, display: 'flex', flexDirection: 'column', gap: 6 }}>
              {rows}
            </div>
          );
        } catch {
          content = <Typography.Text>{text}</Typography.Text>;
        }

        if (!content) {
          content = <Typography.Text>{text}</Typography.Text>;
        }

        return (
          <Popover key={index} content={content} title="事件详情" trigger="hover">
            <InfoCircleOutlined style={{ cursor: 'pointer' }} />
          </Popover>
        );
      },
    });
  }

  return columns;
};

export type LedgersTableProps = {
  mode?: 'account' | 'bot';
  scrollY?: number;
} & ProTableProps<Ledger, ParamsType>;

const LedgersTable: FC<LedgersTableProps> = ({ mode = 'account', scrollY, ...tableProps }) => {
  const columns = buildLedgerColumns({ showDetailColumn: mode === 'account' });

  return (
    <ProTable<Ledger>
      scroll={{ y: scrollY }}
      columns={columns}
      rowKey={(record) => String(record.id)}
      search={false}
      options={false}
      toolBarRender={false}
      {...tableProps}
    />
  );
};

export default LedgersTable;

