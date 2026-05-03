import { AccountType, deleteAccount, queryAccount } from '@/services/gateway/account';
import {
  Bot,
  BotMode,
  BotStatus,
  deleteBot,
  queryBots,
  QueryBotsParams,
  startBot,
  stopBot,
  upgradeBot,
} from '@/services/gateway/strategy';
import { getExchangeTitle } from '@/utils/market';
import { history } from '@@/exports';
import {
  ArrowUpOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { ActionType, PageContainer, ProColumns, ProTable } from '@ant-design/pro-components';
import { Outlet, useMatch } from '@umijs/max';
import { Button, Checkbox, Dropdown, message, Modal, Space, Tag, Typography } from 'antd';
import dayjs from 'dayjs';
import { MenuInfo } from 'rc-menu/es/interface';
import React, { useRef, useState } from 'react';
import BotModal from './components/BotModal';

const BotsComponent: React.FC = () => {
  const actionRef = useRef<ActionType>();
  const matchDetail = useMatch({ path: '/bot/:id', end: true });

  const [botModalOpen, setBotModalOpen] = useState(false);
  const [editingBot, setEditingBot] = useState<Bot | null>(null);

  // 子路由 /bot/:id 时渲染详情页出口，避免两级导航
  if (matchDetail) {
    return <Outlet />;
  }

  const handleMenuClick = async (e: MenuInfo, row: Bot, index: number) => {
    switch (e.key) {
      case 'upgrade':
        Modal.confirm({
          title: '确认升级 Bot？',
          content:
            '升级会先停止当前 Bot、更新策略版本到最新并重新启动。升级后可能启动失败，请确认是否继续。',
          okText: '确认升级',
          cancelText: '取消',
          onOk: async () => {
            const resp = await upgradeBot(row.id);
            if (!resp.errors) {
              const result = resp.data?.Result;
              if (result?.success) {
                message.success(result?.message || 'Bot 升级完成');
              } else {
                message.warning(result?.message || 'Bot 已升级，但启动失败');
              }
              actionRef.current?.reload();
            } else {
              message.error(resp.errors[0]?.message || '升级失败');
            }
          },
        });
        break;
      case 'start':
        const startResp = await startBot(row.id);
        if (!startResp.errors) {
          message.success('Bot 已启动');
          actionRef.current?.reload();
        } else {
          message.error(startResp.errors[0]?.message || '启动失败');
        }
        break;
      case 'stop':
        const stopResp = await stopBot(row.id);
        if (!stopResp.errors) {
          message.success('Bot 已停止');
          actionRef.current?.reload();
        } else {
          message.error(stopResp.errors[0]?.message || '停止失败');
        }
        break;
      case 'delete':
        {
          let accountType: AccountType | undefined;
          try {
            const accountRes = await queryAccount(row.accountId);
            accountType = accountRes?.list?.[0]?.accountType;
          } catch (_) {
            // ignore account query failure, keep delete bot flow available.
          }
          const canCascadeDeleteAccount =
            accountType === AccountType.Virtual || accountType === AccountType.VirtualSub;
          let deleteRelatedAccount = false;

          Modal.confirm({
            title: '确认删除',
            content: (
              <Space direction="vertical" size={12}>
                <span>{`确定要删除 Bot「${row.name}」吗？此操作不可恢复。`}</span>
                {canCascadeDeleteAccount && (
                  <Checkbox
                    onChange={(e) => {
                      deleteRelatedAccount = e.target.checked;
                    }}
                  >
                    同时删除关联模拟账户/虚拟子账户
                  </Checkbox>
                )}
              </Space>
            ),
            okText: '删除',
            okType: 'danger',
            cancelText: '取消',
            onOk: async () => {
              const deleteResp = await deleteBot(row.id);
              if (deleteResp.errors?.length) {
                message.error(deleteResp.errors[0]?.message || '删除失败');
                return;
              }

              if (deleteRelatedAccount && canCascadeDeleteAccount) {
                const accountDeleteResp = await deleteAccount(row.accountId);
                if (accountDeleteResp.errors?.length) {
                  message.warning(
                    `Bot 已删除，但关联账户删除失败：${accountDeleteResp.errors[0]?.message || 'unknown error'}`,
                  );
                  actionRef.current?.reload();
                  return;
                }
              }

              message.success('Bot 已删除');
              actionRef.current?.reload();
            },
          });
        }
        break;
      case 'detail':
        history.push(`/bot/${row.id}`);
        break;
      case 'edit':
        setEditingBot(row);
        setBotModalOpen(true);
        break;
      default:
        break;
    }
  };

  const columns: ProColumns<Bot>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
      ellipsis: true,
      render: (dom, entity, index) => {
        return (
          <a
            onClick={() => {
              history.push(`/bot/${entity.id}`);
            }}
          >
            {dom}
          </a>
        );
      },
    },
    {
      title: '名称',
      dataIndex: 'name',
      ellipsis: true,
    },
    {
      title: '策略',
      dataIndex: 'strategyName',
      hideInSearch: true,
      render: (_, record) => {
        return (
          <Space>
            <span>{record.strategyName || record.strategyId}</span>
          </Space>
        );
      },
    },
    {
      title: '交易所',
      dataIndex: 'exchange',
      width: 100,
      align: 'center',
      render: (_, record) => <Tag>{getExchangeTitle(record.exchange) || record.exchange}</Tag>,
    },
    {
      title: '账户',
      dataIndex: 'accountId',
      width: 200,
      align: 'center',
      render: (_, record) => (
        <Typography.Link
          copyable
          onClick={() => {
            history.push(`/account/${record.accountId}`);
          }}
        >
          {record.accountId || record.accountId}
        </Typography.Link>
      ),
    },
    {
      title: '模式',
      dataIndex: 'mode',
      width: 100,
      align: 'center',
      valueType: 'select',
      filters: true,
      onFilter: true,
      valueEnum: {
        [BotMode.Live]: { text: '实盘', status: 'Processing' },
        [BotMode.Paper]: { text: '模拟盘', status: 'Warning' },
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      align: 'center',
      valueType: 'select',
      filters: true,
      onFilter: true,
      valueEnum: {
        [BotStatus.Stopped]: { text: '已停止', status: 'Default' },
        [BotStatus.Running]: { text: '运行中', status: 'Success' },
        [BotStatus.Error]: { text: '错误', status: 'Error' },
      },
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      width: 180,
      align: 'center',
      valueType: 'dateTime',
      hideInSearch: true,
      render: (_, record) => dayjs(record.createdAt * 1000).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '创建时间',
      dataIndex: 'createdAtRange',
      valueType: 'dateTimeRange',
      colSize: 2,
      ellipsis: true,
      hideInTable: true,
      hideInForm: true,
      hideInDescriptions: true,
      fieldProps: {
        allowClear: true,
      },
    },
    {
      title: '操作',
      valueType: 'option',
      fixed: 'right',
      width: 130,
      render: (_, record, index) => {
        const items = [];

        if (record.status === BotStatus.Stopped || record.status === BotStatus.Error) {
          items.push({
            key: 'start',
            label: '启动',
            icon: <PlayCircleOutlined />,
          });
        }

        if (record.status === BotStatus.Running) {
          items.push({
            key: 'stop',
            label: '停止',
            icon: <PauseCircleOutlined />,
          });
        }

        if (record.upgradable) {
          items.push({
            key: 'upgrade',
            label: <span style={{ color: '#faad14' }}>升级</span>,
            icon: <ArrowUpOutlined style={{ color: '#faad14' }} />,
            style: { color: '#faad14' },
          });
        }

        items.push({
          key: 'edit',
          label: '修改',
          icon: <EditOutlined />,
        });

        items.push({
          key: 'delete',
          label: '删除',
          icon: <DeleteOutlined />,
          danger: true,
        });

        return (
          <Dropdown.Button
            type="primary"
            arrow
            menu={{
              items,
              onClick: (e) => handleMenuClick(e, record, index),
            }}
            onClick={() => {
              history.push(`/bot/${record.id}`);
            }}
          >
            <Space size={10}>
              <EyeOutlined /> 详情
            </Space>
          </Dropdown.Button>
        );
      },
    },
  ];

  return (
    <PageContainer>
      <ProTable<Bot, QueryBotsParams>
        columns={columns}
        actionRef={actionRef}
        cardBordered
        request={async (params = {}, sort, filter) => {
          const result = await queryBots(params);
          return {
            data: result?.list || [],
            total: result?.totalCount || 0,
            success: true,
          };
        }}
        editable={{
          type: 'multiple',
        }}
        columnsState={{
          persistenceKey: 'pro-table-bots',
          persistenceType: 'localStorage',
        }}
        rowKey="id"
        search={{
          labelWidth: 'auto',
        }}
        options={{
          setting: {
            listsHeight: 400,
          },
        }}
        pagination={{
          pageSize: 10,
          showSizeChanger: true,
        }}
        dateFormatter="string"
        headerTitle="Bot 列表"
        toolBarRender={() => [
          <Button
            key="button"
            icon={<PlusOutlined />}
            onClick={() => {
              setBotModalOpen(true);
            }}
            type="primary"
          >
            新建
          </Button>,
        ]}
      />

      <BotModal
        open={botModalOpen}
        onOpenChange={(open) => {
          setBotModalOpen(open);
          if (!open) {
            setEditingBot(null);
          }
        }}
        bot={editingBot}
        onSuccess={() => {
          setBotModalOpen(false);
          setEditingBot(null);
          actionRef.current?.reload();
        }}
      />
    </PageContainer>
  );
};

export default BotsComponent;
