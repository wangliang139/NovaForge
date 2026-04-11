import { EditIndicator } from '@/global.types';
import { Channel, createChannel, DocumentCatalogOptions, queryChannels, updateChannel } from '@/services/gateway/document';
import { PlusOutlined } from '@ant-design/icons';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import { Button, Dropdown, message, Tag } from 'antd';
import { MenuInfo } from 'rc-menu/lib/interface';
import React, { useRef, useState } from 'react';
import ChannelModal from './components/ChannelModal';
import ExtractTestModal from './components/ExtractTestModal';

const ChannelList: React.FC = () => {
  const actionRef = useRef<ActionType>();
  const [channelEditIndicator, setChannelEditIndicator] = useState<EditIndicator<Channel>>({
    mode: 'new',
    open: false,
  });
  const [testModalState, setTestModalState] = useState<{
    open: boolean;
    channel?: Channel;
  }>({
    open: false,
  });

  const handleDisabledToggle = async (id: string, enabled: boolean) => {
    try {
      await updateChannel({
        id,
        enabled,
      });
      message.success(`${enabled ? '启用' : '禁用'}成功`);
      actionRef.current?.reload();
    } catch (error: any) {
      message.error(error.message || '操作失败');
    }
  };

  const handleMenuClick = async (e: MenuInfo, row: Channel) => {
    switch (e.key) {
      case 'edit':
        setChannelEditIndicator({
          mode: 'edit',
          open: true,
          value: row,
        });
        break;
      case 'enable':
        await handleDisabledToggle(row.id, true);
        break;
      case 'disable':
        await handleDisabledToggle(row.id, false);
        break;
      case 'test':
        setTestModalState({
          open: true,
          channel: row,
        });
        break;
    }
  };

  const renderMenus = (row: Channel) => {
    let menus = [{ key: 'edit', label: '修改' }];

    if (row.enabled) {
      menus.push({ key: 'disable', label: '禁用' });
    } else {
      menus.push({ key: 'enable', label: '启用' });
    }
    menus.push({ key: 'test', label: '测试' });

    return [
      <Dropdown.Button
        type="primary"
        key="view"
        arrow={true}
        menu={{
          items: menus,
          onClick: async (e: MenuInfo) => {
            await handleMenuClick(e, row);
          },
        }}
        onClick={() => {
          setChannelEditIndicator({
            mode: 'readonly',
            open: true,
            value: row,
          });
        }}
        children={'查看'}
      />,
    ];
  };

  const columns: ProColumns<Channel>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 100,
      render: (dom, entity) => {
        return (
          <a
            onClick={() => {
              setChannelEditIndicator({
                mode: 'readonly',
                open: true,
                value: entity,
              });
            }}
          >
            {dom}
          </a>
        );
      },
    },
    {
      title: 'Name',
      dataIndex: 'name',
      width: 150,
    },
    {
      title: 'Title',
      dataIndex: 'title',
      width: 200,
      search: false,
    },
    {
      title: 'Broadcast',
      dataIndex: 'broadcast',
      width: 100,
      search: false,
      render: (_, record) => (record.broadcast ? <Tag color="green">Yes</Tag> : <Tag>No</Tag>),
    },
    {
      title: 'Source',
      dataIndex: 'source',
      width: 150,
    },
    {
      title: 'Catalog',
      dataIndex: 'catalog',
      width: 150,
      valueType: 'select',
      fieldProps: {
        options: DocumentCatalogOptions,
      },
    },
    {
      title: 'Enabled',
      dataIndex: 'enabled',
      width: 100,
      initialValue: 'true',
      valueType: 'select',
      hideInTable: true,
      valueEnum: {
        true: { text: '已启用', status: 'Success' },
        false: { text: '已禁用', status: 'Error' },
      },
    },
    {
      title: 'Enabled',
      dataIndex: 'enabled',
      width: 100,
      hideInSearch: true,
      valueEnum: {
        true: { text: '已启用', status: 'Success' },
        false: { text: '已禁用', status: 'Error' },
      },
    },
    {
      title: 'Created At',
      dataIndex: 'createdAt',
      width: 180,
      search: false,
      render: (_, record) => new Date(record.createdAt * 1000).toLocaleString(),
    },
    {
      title: 'Updated At',
      dataIndex: 'updatedAt',
      width: 140,
      search: false,
      hideInTable: true,
      render: (_, record) => new Date(record.updatedAt * 1000).toLocaleString(),
    },
    {
      title: '操作',
      width: 100,
      fixed: 'right',
      search: false,
      render: (_, record) => renderMenus(record),
    },
  ];

  return (
    <>
      <ProTable<Channel>
        columns={columns}
        actionRef={actionRef}
        form={{ span: 4 }}
        request={async (params) => {
          const { current = 1, pageSize = 20, id, name, source, catalog, enabled } = params;
          const offset = (current - 1) * pageSize;

          const result = await queryChannels({
            limit: pageSize,
            offset,
            id: id || undefined,
            name: name || undefined,
            source: source || undefined,
            catalog: catalog || undefined,
            enabled: enabled !== undefined ? enabled === 'true' : undefined,
          });

          return {
            data: result.list,
            success: true,
            total: result.totalCount,
          };
        }}
        rowKey="id"
        pagination={{
          defaultPageSize: 20,
          showSizeChanger: true,
        }}
        search={{
          labelWidth: 'auto',
        }}
        dateFormatter="string"
        headerTitle="文档频道列表"
        toolBarRender={() => [
          <Button
            key="button"
            icon={<PlusOutlined />}
            onClick={() => {
              setChannelEditIndicator({
                mode: 'new',
                open: true,
              });
            }}
            type="primary"
          >
            新建
          </Button>,
        ]}
      />

      <ChannelModal
        mode={channelEditIndicator.mode || 'new'}
        open={channelEditIndicator.open}
        value={channelEditIndicator.value || undefined}
        onOpenChange={(open) =>
          setChannelEditIndicator((prev) => ({
            ...prev,
            open,
          }))
        }
        onFinish={async (values: Channel) => {
          try {
            if (channelEditIndicator.mode === 'new') {
              await createChannel(values);
              message.success('创建成功');
            } else if (channelEditIndicator.mode === 'edit') {
              await updateChannel(values);
              message.success('更新成功');
            }
            actionRef.current?.reload();
            return true;
          } catch (error: any) {
            return false;
          }
        }}
      />

      {testModalState.channel && (
        <ExtractTestModal
          open={testModalState.open}
          value={
            testModalState.channel.extractCfg
              ? JSON.stringify(testModalState.channel.extractCfg, null, 2)
              : undefined
          }
          onOpenChange={(open) =>
            setTestModalState((prev) => ({
              ...prev,
              open,
            }))
          }
        />
      )}
    </>
  );
};

export default ChannelList;
