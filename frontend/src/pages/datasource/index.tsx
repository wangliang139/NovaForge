import { CodeEditor } from '@/components';
import { Exchange } from '@/global.types';
import { DataSource, QueryDatasourcesParams, SignalType, createDatasource, deleteDatasource, queryDatasources } from '@/services/gateway/strategy';
import { enumToOptions } from '@/utils/dict';
import { CodeOutlined, PlusOutlined } from '@ant-design/icons';
import {
  ActionType,
  PageContainer,
  ProColumns,
  ProDescriptions,
  ProDescriptionsItemProps,
  ProFormInstance,
  ProTable,
} from '@ant-design/pro-components';
import { Button, Card, Drawer, Modal, message } from 'antd';
import dayjs from 'dayjs';
import React, { useRef, useState } from 'react';
import CreateModal from './components/CreateModal';

const DatasourcesComponent: React.FC = () => {
  const actionRef = useRef<ActionType>();
  const formRef = useRef<ProFormInstance>();
  const [openCreateModal, setOpenCreateModal] = useState<boolean>(false);
  const [openViewModal, setOpenViewModal] = useState<boolean>(false);
  const [currentRow, setCurrentRow] = useState<DataSource>();

  const columns: ProColumns<DataSource>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
      render: (dom, entity) => {
        return (
          <a
            onClick={() => {
              setCurrentRow(entity);
              setOpenViewModal(true);
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
      hideInSearch: true,
      ellipsis: true,
    },
    {
      title: '类型',
      dataIndex: 'type',
      width: 80,
      valueType: 'select',
      valueEnum: {
        [SignalType.Kline]: { text: 'K线' },
        [SignalType.Trade]: { text: '交易' },
        [SignalType.Depth]: { text: '深度' },
        [SignalType.Ticker]: { text: 'Ticker' },
        [SignalType.Social]: { text: '社交' },
        [SignalType.Timer]: { text: '定时器' },
        [SignalType.Order]: { text: '订单' },
        [SignalType.Position]: { text: '持仓' },
        [SignalType.Balance]: { text: '资金' },
        [SignalType.Risk]: { text: '风险' },
        [SignalType.System]: { text: '系统' },
      },
    },
    {
      title: '描述',
      dataIndex: 'description',
      hideInSearch: true,
      hideInTable: true,
    },
    {
      title: '交易所',
      dataIndex: 'exchange',
      valueType: 'select',
      width: 80,
      fieldProps: {
        options: enumToOptions(Exchange),
      },
    },
    {
      title: '交易对',
      dataIndex: 'symbol',
    },
    {
      title: '开始时间',
      dataIndex: 'startTs',
      valueType: 'dateTime',
      ellipsis: true,
      render: (_, record) => dayjs.unix(record.startTs).format('YYYY-MM-DD HH:mm:ss'),
      hideInSearch: true,
    },
    {
      title: '结束时间',
      dataIndex: 'endTs',
      valueType: 'dateTime',
      ellipsis: true,
      render: (_, record) => dayjs.unix(record.endTs).format('YYYY-MM-DD HH:mm:ss'),
      hideInSearch: true,
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      valueType: 'dateTime',
      ellipsis: true,
      render: (_, record) => dayjs.unix(record.createdAt).format('YYYY-MM-DD HH:mm:ss'),
      hideInSearch: true,
    },
    {
      title: '更新时间',
      dataIndex: 'updatedAt',
      valueType: 'dateTime',
      render: (_, record) => dayjs.unix(record.updatedAt).format('YYYY-MM-DD HH:mm:ss'),
      hideInSearch: true,
      hideInTable: true,
    },
    {
      title: '操作',
      valueType: 'option',
      width: 100,
      hideInDescriptions: true,
      render: (_, record) => [
        <a
          key="view"
          onClick={() => {
            setCurrentRow(record);
            setOpenViewModal(true);
          }}
        >
          详情
        </a>,
        <a
          key="delete"
          onClick={() => {
            Modal.confirm({
              title: '确认删除',
              content: `确定要删除数据源 "${record.name || record.id}" 吗？此操作不可恢复。`,
              okText: '确认',
              okType: 'danger',
              cancelText: '取消',
              onOk: async () => {
                const resp = await deleteDatasource(record.id);
                if (!resp.errors) {
                  message.success('删除成功');
                  actionRef.current?.reload();
                }
              },
            });
          }}
        >
          删除
        </a>,
      ],
    },
  ];

  return (
    <PageContainer>
      <ProTable<DataSource, QueryDatasourcesParams>
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
              setOpenCreateModal(true);
            }}
          >
            <PlusOutlined /> 新建
          </Button>,
        ]}
        request={async (params: QueryDatasourcesParams) => {
          const res = await queryDatasources(params);
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

      <CreateModal
        open={openCreateModal}
        onOpenChange={setOpenCreateModal}
        onFinish={async (value) => {
          const hide = message.loading('正在保存');
          let res = await createDatasource(value).finally(() => hide());
          if (!res.errors) {
            message.success('添加成功');
            if (actionRef.current) {
              actionRef.current.reload();
            }
            return true;
          }
          return false;
        }}
      />

      <Drawer
        open={openViewModal}
        onClose={() => {
          setOpenViewModal(false);
          setCurrentRow(undefined);
        }}
        title="数据源详情"
        width={600}
      >
        <ProDescriptions<DataSource>
          title={currentRow?.name}
          column={1}
          dataSource={currentRow}
          columns={columns as ProDescriptionsItemProps<DataSource>[]}
        />
        {currentRow?.props && (
          <Card
            // className={styles.contentCard}
            variant="borderless"
            title={
              <>
                <CodeOutlined /> 附加属性
              </>
            }
            style={{ marginTop: 20, padding: 0 }}
            styles={{ body: { padding: 0 } }}
          >
            <CodeEditor
              value={currentRow?.props || ''}
              language="json"
              readonly={true}
              height="200px"
            />
          </Card>
        )}
      </Drawer>
    </PageContainer>
  );
};

export default DatasourcesComponent;
