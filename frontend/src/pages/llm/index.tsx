import { EditIndicator } from '@/global.types';
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  ExperimentOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import {
  ActionType,
  PageContainer,
  ProColumns,
  ProFormInstance,
  ProTable,
} from '@ant-design/pro-components';
import { Button, Dropdown, message, Modal, Space } from 'antd';
import dayjs from 'dayjs';
import { MenuInfo } from 'rc-menu/lib/interface';
import React, { useRef, useState } from 'react';
import PromptTestModal from './components/PromptTestModal';
import LlmSceneModal from './components/SceneModal';
import {
  createLlmScene,
  deleteLlmScene,
  getLlmScene,
  queryLlmScenes,
  updateLlmScene,
} from './service';
import { LlmScene, QueryLlmScenesParams } from './types';

const LlmSceneComponent: React.FC = () => {
  const actionRef = useRef<ActionType>();
  const formRef = useRef<ProFormInstance>();

  /** LLM Scene 弹窗 */
  const [sceneEditIndicator, setSceneEditIndicator] = useState<EditIndicator<LlmScene>>({
    mode: 'new',
    open: false,
    value: null,
    index: -1,
  });

  /** Prompt 测试弹窗 */
  const [testModalOpen, setTestModalOpen] = useState(false);
  const [testScene, setTestScene] = useState<LlmScene>();

  const buildUpdatePayload = (row: LlmScene, overrides?: Partial<LlmScene>) => {
    const v = { ...row, ...(overrides || {}) } as LlmScene;
    return {
      id: v.id,
      key: v.key,
      name: v.name,
      description: v.description,
      config: v.config,
      messages: v.messages || [],
      timeout: v.timeout,
      responseFormat: v.responseFormat,
      enabled: v.enabled,
    };
  };

  const buildCreatePayload = (values: LlmScene) => {
    return {
      key: values.key,
      name: values.name,
      description: values.description,
      timeout: values.timeout,
      messages: values.messages,
      config: values.config,
      responseFormat: values.responseFormat,
      enabled: values.enabled,
    };
  };

  const handleViewDetail = async (row: LlmScene, index: number) => {
    const resp = await getLlmScene(row.id, true);
    if (!resp.errors && resp.scene) {
      setSceneEditIndicator({
        mode: 'readonly',
        open: true,
        value: resp.scene,
        index: index,
      });
    }
  };

  const handleMenuClick = async (e: MenuInfo, row: LlmScene) => {
    switch (e.key) {
      case 'copy':
        setSceneEditIndicator({
          mode: 'new',
          open: true,
          value: row,
          index: -1,
        });
        break;
      case 'test':
        setTestScene(row);
        setTestModalOpen(true);
        break;
      case 'delete':
        Modal.confirm({
          title: '确认删除',
          content: `确定要删除场景 "${row.key}" 吗？`,
          okText: '确定',
          okType: 'danger',
          cancelText: '取消',
          onOk: async () => {
            const hide = message.loading('正在删除');
            try {
              const res = await deleteLlmScene(row.id);
              hide();
              if (!res.errors && res.data?.Result) {
                message.success('删除成功');
                if (actionRef.current) {
                  actionRef.current?.reload();
                }
              } else {
                message.error('删除失败');
              }
            } catch (error) {
              hide();
              message.error('删除失败');
            }
          },
        });
        break;
      default:
        break;
    }
  };

  const columns: ProColumns<LlmScene>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 100,
      hideInSearch: true,
      render: (dom, entity, index) => {
        return (
          <a
            onClick={() => {
              handleViewDetail(entity, index);
            }}
          >
            {dom}
          </a>
        );
      },
    },
    {
      title: 'Key',
      dataIndex: 'key',
    },
    {
      title: 'Name',
      dataIndex: 'name',
    },
    {
      title: 'Description',
      dataIndex: 'description',
      ellipsis: true,
      hideInSearch: true,
      hideInTable: true,
    },
    {
      title: 'Timeout',
      dataIndex: 'timeout',
      width: 100,
      hideInSearch: true,
    },
    {
      title: 'Enabled',
      dataIndex: 'enabled',
      width: 100,
      valueType: 'select',
      valueEnum: {
        true: {
          text: 'Enabled',
          status: 'Success',
        },
        false: {
          text: 'Disabled',
          status: 'Default',
        },
      },
    },
    {
      title: 'CreatedAt',
      dataIndex: 'createdAt',
      ellipsis: true,
      hideInSearch: true,
      width: 160,
      renderText: (text) => (text >= 0 ? dayjs.unix(text).format('YYYY-MM-DD HH:mm:ss') : ''),
    },
    {
      title: 'UpdatedAt',
      dataIndex: 'updatedAt',
      ellipsis: true,
      hideInSearch: true,
      width: 160,
      renderText: (text) => (text >= 0 ? dayjs.unix(text).format('YYYY-MM-DD HH:mm:ss') : ''),
    },
    {
      title: 'Operation',
      valueType: 'option',
      hideInDescriptions: true,
      width: 130,
      render: (dom, row, index) => {
        let menus = [
          { key: 'copy', label: '复制', icon: <CopyOutlined /> },
          { key: 'test', label: '测试', icon: <ExperimentOutlined /> },
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
                await handleMenuClick(e, row);
              },
            }}
            onClick={async () => {
              setSceneEditIndicator({
                value: row,
                open: true,
                mode: 'edit',
                index: index,
              });
            }}
          >
            <Space size={10}>
              <EditOutlined /> Edit
            </Space>
          </Dropdown.Button>,
        ];
      },
    },
  ];

  return (
    <PageContainer>
      <ProTable<LlmScene, QueryLlmScenesParams>
        actionRef={actionRef}
        formRef={formRef}
        form={{ span: 5 }}
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
              setSceneEditIndicator({
                mode: 'new',
                open: true,
                index: -1,
              });
            }}
          >
            <PlusOutlined /> 新建
          </Button>,
        ]}
        request={async (params: QueryLlmScenesParams) => {
          const res = await queryLlmScenes(params);
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

      <LlmSceneModal
        mode={sceneEditIndicator.mode || 'new'}
        open={sceneEditIndicator.open}
        value={sceneEditIndicator.value || undefined}
        onOpenChange={(open) => {
          setSceneEditIndicator((prev) => ({
            ...prev,
            open: open,
          }));
        }}
        onFinish={async (value) => {
          if (sceneEditIndicator.mode === 'readonly') {
            return true;
          }
          const hide = message.loading('正在保存');
          let res = null;
          if (sceneEditIndicator.mode === 'new') {
            const params = buildCreatePayload(value);
            res = await createLlmScene(params);
          } else {
            const params = buildUpdatePayload(sceneEditIndicator.value as LlmScene, value);
            res = await updateLlmScene(params as any);
          }
          hide();
          if (!res.errors) {
            message.success('保存成功');
            if (actionRef.current) {
              actionRef.current?.reload();
            }
            return true;
          }
          return false;
        }}
      />

      <PromptTestModal
        open={testModalOpen}
        scene={testScene || undefined}
        onOpenChange={(open) => {
          setTestModalOpen(open);
        }}
      />
    </PageContainer>
  );
};

export default LlmSceneComponent;
