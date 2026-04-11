import {
  activeStrategy,
  deleteStrategy,
  inactiveStrategy,
  queryStrategies,
  QueryStrategiesParams,
  Strategy,
  StrategyStatus,
} from '@/services/gateway/strategy';
import {
  CheckCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import {
  ActionType,
  PageContainer,
  ProColumns,
  ProFormInstance,
  ProTable,
} from '@ant-design/pro-components';
import { history } from '@umijs/max';
import { Button, Dropdown, message, Modal, Space, Tag } from 'antd';
import dayjs from 'dayjs';
import { MenuInfo } from 'rc-menu/es/interface';
import React, { useRef } from 'react';

const StrategiesComponent: React.FC = () => {
  const actionRef = useRef<ActionType>();
  const formRef = useRef<ProFormInstance>();

  const handleMenuClick = async (e: MenuInfo, row: Strategy) => {
    switch (e.key) {
      case 'backtest':
        history.push(`/strategy/${row.id}?tab=backtest`);
        break;
      case 'active':
        const activeResp = await activeStrategy(row.id);
        if (!activeResp.errors) {
          message.success('策略已激活');
          actionRef.current?.reload();
        }
        break;
      case 'inactive':
        const inactiveResp = await inactiveStrategy(row.id);
        if (!inactiveResp.errors) {
          message.success('策略已停用');
          actionRef.current?.reload();
        }
        break;
      case 'delete':
        Modal.confirm({
          title: '确认删除',
          content: `确定要删除策略「${row.name}」吗？此操作不可恢复。`,
          okText: '删除',
          okType: 'danger',
          cancelText: '取消',
          onOk: async () => {
            const deleteResp = await deleteStrategy(row.id);
            if (deleteResp.errors?.length > 0) {
              return;
            }
            message.success('策略已删除');
            actionRef.current?.reload();
          },
        });
        break;
      default:
        break;
    }
  };

  const columns: ProColumns<Strategy>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 180,
      render: (dom, entity) => {
        return (
          <a
            onClick={() => {
              history.push(`/strategy/${entity.id}`);
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
    },
    {
      title: '描述',
      dataIndex: 'description',
      ellipsis: true,
      hideInSearch: true,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      align: 'center',
      valueType: 'select',
      valueEnum: {
        [StrategyStatus.Draft]: { text: '草稿', status: 'Default' },
        [StrategyStatus.Active]: { text: '激活', status: 'Success' },
        [StrategyStatus.Inactive]: { text: '停用', status: 'Warning' },
      },
      render: (_, record) => {
        const statusMap = {
          [StrategyStatus.Draft]: { text: '草稿', color: 'default' },
          [StrategyStatus.Active]: { text: '激活', color: 'success' },
          [StrategyStatus.Inactive]: { text: '停用', color: 'warning' },
        };
        const status =
          statusMap[record.status as keyof typeof statusMap] || statusMap[StrategyStatus.Draft];
        return <Tag color={status.color}>{status.text}</Tag>;
      },
    },
    {
      title: '版本',
      dataIndex: 'version',
      width: 120,
      hideInTable: true,
      hideInSearch: true,
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      valueType: 'dateTime',
      sorter: true,
      hideInSearch: true,
      render: (_, record) => dayjs.unix(record.createdAt).format('YYYY-MM-DD HH:mm:ss'),
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
      title: '更新时间',
      dataIndex: 'updatedAt',
      valueType: 'dateTime',
      sorter: true,
      hideInSearch: true,
      render: (_, record) => dayjs.unix(record.updatedAt).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '操作',
      valueType: 'option',
      hideInDescriptions: true,
      width: 130,
      render: (_, record) => {
        const menus = [
          { key: 'backtest', label: '回测', icon: <PlayCircleOutlined /> },
          {
            key: 'active',
            label: '激活',
            icon: <CheckCircleOutlined />,
            disabled: record.status === StrategyStatus.Active,
          },
          {
            key: 'inactive',
            label: '停用',
            icon: <PauseCircleOutlined />,
            disabled: record.status !== StrategyStatus.Active,
          },
          { key: 'delete', label: '删除', icon: <DeleteOutlined />, danger: true },
        ];
        return [
          <Dropdown.Button
            type="primary"
            key="edit"
            arrow={true}
            menu={{
              items: menus,
              onClick: async (e: MenuInfo) => {
                await handleMenuClick(e, record);
              },
            }}
            onClick={() => {
              history.push(`/strategy/${record.id}`);
            }}
          >
            <Space size={10}>
              <EditOutlined /> 编辑
            </Space>
          </Dropdown.Button>,
        ];
      },
    },
  ];

  return (
    <PageContainer>
      <ProTable<Strategy, QueryStrategiesParams>
        actionRef={actionRef}
        formRef={formRef}
        rowKey={(record) => record.id}
        search={{
          labelWidth: 'auto',
          showHiddenNum: true,
        }}
        toolBarRender={() => [
          <Button
            type="primary"
            key="primary"
            onClick={() => {
              history.push('/strategy/new');
            }}
          >
            <PlusOutlined /> 新建
          </Button>,
        ]}
        request={async (params: any) => {
          const queryParams: QueryStrategiesParams = {
            id: params.id,
            current: params.current,
            pageSize: params.pageSize,
            name: params.name,
            status: params.status,
          };
          if (params.createdAtRange && Array.isArray(params.createdAtRange)) {
            queryParams.createdAtStart = dayjs(params.createdAtRange[0]).unix();
            queryParams.createdAtEnd = dayjs(params.createdAtRange[1]).unix();
          }
          const res = await queryStrategies(queryParams);
          return {
            data: res.list,
            total: res.totalCount,
            success: true,
          };
        }}
        pagination={{
          showSizeChanger: true,
        }}
        columns={columns}
        dateFormatter={'number'}
      />
    </PageContainer>
  );
};

export default StrategiesComponent;
