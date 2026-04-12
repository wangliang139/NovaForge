import { EditIndicator, Exchange } from '@/global.types';
import { Account, AccountStatus, AccountType, createAccount, deleteAccount, offlineAccount, onlineAccount, queryAccounts, QueryAccountsParams, updateAccount } from '@/services/gateway/account';
import utils from '@/utils';
import { enumToOptions } from '@/utils/dict';
import { history } from '@@/exports';
import { Outlet, useMatch } from '@umijs/max';
import { PlusOutlined } from '@ant-design/icons';
import {
  ActionType,
  PageContainer,
  ProColumns,
  ProFormInstance,
  ProList,
} from '@ant-design/pro-components';
import { Button, Card, Dropdown, Flex, message, Modal, Space, Statistic, Tag, Typography } from 'antd';
import dayjs from 'dayjs';
import { MenuInfo } from 'rc-menu/es/interface';
import React, { useRef, useState } from 'react';
import AccountModal from './components/AccountModal';

const AccountsComponent: React.FC = () => {
  const matchDetail = useMatch({ path: '/account/:id', end: true });
  /** 新建窗口的弹窗 */
  const actionRef = useRef<ActionType>();
  const formRef = useRef<ProFormInstance>();

  const [accountIndicator, setAccountIndicator] = useState<EditIndicator<Account>>({
    mode: 'new',
    open: false,
  });
  const [summaryStats, setSummaryStats] = useState({
    notional: 0,
    unRealizedProfit: 0,
    notional24HChange: 0,
  });

  // 子路由 /account/:id 时渲染详情页出口（放在所有 hooks 之后，避免 Rendered fewer hooks）
  if (matchDetail) {
    return <Outlet />;
  }

  const toNumber = (value?: string) => {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  };

  const handleMenuClick = async (e: MenuInfo, row: Account) => {
    if (e.key === 'online') {
      const resp = await onlineAccount(row.id);
      if (!resp.errors) {
        message.success(`Account online successfully!`);
        actionRef.current?.reload();
      }
    } else if (e.key === 'offline') {
      const resp = await offlineAccount(row.id);
      if (!resp.errors) {
        message.success(`Account offline successfully!`);
        actionRef.current?.reload();
      }
    } else if (e.key === 'delete') {
      Modal.confirm({
        title: '确认删除该账户？',
        content: `删除后将无法恢复`,
        okText: '确定',
        okType: 'danger',
        cancelText: '取消',
        onOk: async () => {
          await handleDelete(row);
        },
      });
    } else {
      setAccountIndicator({
        mode: 'edit',
        open: true,
        value: row,
      });
    }
  };

  const handleDelete = async (row: Account) => {
    const hide = message.loading('正在删除');
    try {
      const resp = await deleteAccount(row.id);
      if (!resp.errors && resp.data?.Result) {
        message.success('删除成功');
        actionRef.current?.reload();
        return;
      }
      message.error('删除失败');
    } finally {
      hide();
    }
  };

  const exchangeOptions = enumToOptions(Exchange);
  const getExchangeLabel = (exchange: Exchange) => {
    return exchangeOptions.find((option) => option.value === exchange)?.label ?? exchange;
  };

  const renderStatusTag = (status?: string) => {
    const statusMap: Record<string, { text: string; color: string }> = {
      [AccountStatus.Online]: { text: '在线', color: 'green' },
      [AccountStatus.Offline]: { text: '离线', color: 'default' },
      [AccountStatus.Unspecified]: { text: '未指定', color: 'default' },
    };
    const statusInfo = statusMap[status || ''] || statusMap[AccountStatus.Unspecified];
    return <Tag color={statusInfo.color}>{statusInfo.text}</Tag>;
  };

  const renderAccountTypeTag = (accountType: AccountType) => {
    const typeMap: Record<string, { text: string; color: string }> = {
      [AccountType.Real]: { text: '真实账户', color: 'blue' },
      [AccountType.Virtual]: { text: '虚拟账户', color: 'green' },
      [AccountType.VirtualSub]: { text: '虚拟子账户', color: 'orange' },
      [AccountType.Unspecified]: { text: '未指定', color: 'default' },
    };
    const typeInfo = typeMap[accountType] || typeMap[AccountType.Unspecified];
    return <Tag color={typeInfo.color}>{typeInfo.text}</Tag>;
  };

  const handleCopy = async (text: string) => {
    const value = text ?? '';
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
      } else {
        const textarea = document.createElement('textarea');
        textarea.value = value;
        textarea.setAttribute('readonly', 'true');
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        const copied = document.execCommand('copy');
        document.body.removeChild(textarea);
        if (!copied) {
          throw new Error('浏览器不支持复制');
        }
      }
      message.success('已复制！');
    } catch (err) {
      message.error(`复制失败：${err}`);
    }
  };

  return (
    <PageContainer>
      <Flex justify="space-between" style={{ marginBottom: 16 }}>
        <Card variant="borderless" style={{ width: '30%' }}>
          <Statistic
            title="总资产估值"
            value={`${summaryStats.notional.toFixed(2)}`}
            precision={2}
            suffix={<Typography.Text type="secondary">USDT</Typography.Text>}
          />
        </Card>
        <Card variant="borderless" style={{ width: '30%' }}>
          <Statistic
            title="未实现收益"
            value={`${summaryStats.unRealizedProfit >= 0 ? '+' : ''}${summaryStats.unRealizedProfit.toFixed(2)}`}
            valueStyle={{ color: summaryStats.unRealizedProfit >= 0 ? '#388e3c' : '#d32f2f' }}
            precision={2}
            suffix={<Typography.Text type="secondary">USDT</Typography.Text>}
          />
        </Card>
        <Card variant="borderless" style={{ width: '30%' }}>
          <Statistic
            title="24小时变动"
            value={`${summaryStats.notional24HChange >= 0 ? '+' : ''}${summaryStats.notional24HChange.toFixed(2)}`}
            valueStyle={{ color: summaryStats.notional24HChange >= 0 ? '#388e3c' : '#d32f2f' }}
            precision={2}
            suffix={<Typography.Text type="secondary">USDT</Typography.Text>}
          />
        </Card>
      </Flex>
      <ProList<Account, API.PageParams>
        actionRef={actionRef}
        formRef={formRef}
        form={{ span: 6 }}
        rowKey={(record) => record.id}
        search={{
          filterType: 'light',
          // labelWidth: 'auto',
          // showHiddenNum: true,
        }}
        toolBarRender={() => [
          <Button
            type="primary"
            key="primary"
            onClick={() => {
              setAccountIndicator({
                mode: 'new',
                open: true,
                value: null,
              });
            }}
          >
            <PlusOutlined /> 新建
          </Button>,
        ]}
        request={async (params: QueryAccountsParams) => {
          const res = await queryAccounts(params);
          const list = (res?.list || []) as Account[];
          let summaryList = list;
          if (res?.totalCount && res.totalCount > list.length) {
            const summaryRes = await queryAccounts({
              ...params,
              current: 1,
              pageSize: res.totalCount,
            });
            summaryList = (summaryRes?.list || []) as Account[];
          }
          const totals = summaryList.reduce(
            (acc: { notional: number; unRealizedProfit: number; notional24HChange: number }, item: Account) => {
              acc.notional += toNumber(item.stats?.notional);
              acc.unRealizedProfit += toNumber(item.stats?.unRealizedProfit);
              acc.notional24HChange += toNumber(item.stats?.notional24HChange);
              return acc;
            },
            { notional: 0, unRealizedProfit: 0, notional24HChange: 0 },
          );
          setSummaryStats(totals);
          return {
            data: list,
            total: res?.totalCount,
            success: true,
          };
        }}
        pagination={{
          showSizeChanger: true,
        }}
        grid={{
          gutter: 16,
          xs: 1,
          sm: 2,
          md: 2,
          lg: 3,
          xl: 3,
          xxl: 4,
        }}
        showActions="hover"
        metas={{
          title: {
            dataIndex: 'name',
            render: (_: React.ReactNode, row: Account) => (
              <Space size={4} align="baseline">
                <img
                  alt={row.exchange}
                  style={{ display: 'inline', marginLeft: 0, paddingBottom: 2 }}
                  width={16}
                  src={utils.market.getExchangeLogo(row.exchange)}
                />
                <span>{row.name || row.id}</span>
              </Space>
            ),
            search: false,
          },
          description: {
            search: false,
            render: (_: React.ReactNode, row: Account) => (
              <Space size={[4, 4]} wrap>
                <Tag color="blue">{getExchangeLabel(row.exchange)}</Tag>
                {renderAccountTypeTag(row.accountType)}
              </Space>
            ),
          },
          content: {
            search: false,
            render: (_: React.ReactNode, row: Account) => (
              <div style={{ width: '100%' }}>
                <Space direction="vertical" size={6} style={{ textAlign: 'right', float: 'right' }}>
                  <Typography.Title level={4}>
                    {toNumber(row.stats?.notional).toFixed(2)}
                  </Typography.Title>
                  <Typography.Text type={toNumber(row.stats?.notional24HChange) >= 0 ? 'success' : 'danger'}>
                    {toNumber(row.stats?.notional24HChange) >= 0 ? '+' : ''}
                    {toNumber(row.stats?.notional24HChange).toFixed(2)}
                  </Typography.Text>
                </Space>
                <Space direction="vertical" size={4} style={{ float: 'left' }}>
                  <div>
                    ID: <a onClick={(e) => {
                      e.stopPropagation();
                      handleCopy(row.id);
                    }}>{row.id}</a>
                  </div>
                  <div>
                    {renderStatusTag(row.status)}
                    {renderAccountTypeTag(row.accountType)}
                  </div>
                  {row.tags?.length ? (
                    <Space size={0}>
                      {row.tags.map((tag) => (
                        <Tag key={tag}>{tag}</Tag>
                      ))}
                    </Space>
                  ) : (
                    <span>暂无标签</span>
                  )}
                  <div style={{ marginTop: 10 }}>
                    创建时间：{row.createdAt >= 0 ? dayjs.unix(row.createdAt).format('YYYY-MM-DD HH:mm:ss') : '-'}
                  </div>
                </Space>
              </div>
            ),
          },
          actions: {
            render: (_: React.ReactNode, row: Account) => {
              const menus = [];
              if (row.status === AccountStatus.Online) {
                menus.push({ key: 'offline', label: 'Offline' });
              } else {
                menus.push({ key: 'online', label: 'Online' });
              }
              menus.push({ key: 'delete', label: '删除', danger: true });
              return [
                <Dropdown.Button menu={{ items: menus, onClick: async (e: MenuInfo) => { await handleMenuClick(e, row); } }}
                  onClick={() => {
                    setAccountIndicator({
                      mode: 'edit',
                      open: true,
                      value: row,
                    });
                  }}>编辑</Dropdown.Button>
              ];
            },
          },
          id: {
            title: 'ID',
            dataIndex: 'id',
          },
          exchange: {
            title: 'Exchange',
            dataIndex: 'exchange',
            valueType: 'select',
            fieldProps: {
              options: enumToOptions(Exchange),
            },
          },
          type: {
            title: 'Type',
            dataIndex: 'accountType',
            valueType: 'select',
            fieldProps: {
              options: enumToOptions(AccountType, 'Unspecified'),
            },
            render: (dom, row) => {
              return renderAccountTypeTag(row.accountType);
            },
          },
        }}
        dateFormatter={'number'}
        onItem={(row: any) => {
          return {
            onClick: () => {
              // window.open(`/account/${row.id}`, '_blank');
              history.push(`/account/${row.id}`);
            },
          };
        }}
      />
      <AccountModal
        key={accountIndicator.value?.id || accountIndicator.mode}
        mode={accountIndicator.mode || 'new'}
        open={accountIndicator.open}
        value={accountIndicator.value || undefined}
        onOpenChange={(open) => {
          if (open) {
            setAccountIndicator((prev) => ({ ...prev, open: true }));
            return;
          }
          setAccountIndicator({
            mode: 'new',
            open: false,
            value: null,
          });
        }}
        onFinish={async (value) => {
          if (accountIndicator.mode === 'readonly') {
            return true;
          }
          const hide = message.loading('正在保存');
          const res =
            accountIndicator.mode === 'edit'
              ? await updateAccount({
                ...(accountIndicator.value || {}),
                ...value,
              }).finally(() => hide())
              : await createAccount(value).finally(() => hide());
          if (!res.errors) {
            message.success(accountIndicator.mode === 'edit' ? '配置成功' : '添加成功');
            actionRef.current?.reload();
            return true;
          }
          return false;
        }}
      />
    </PageContainer>
  );
};

export default AccountsComponent;
