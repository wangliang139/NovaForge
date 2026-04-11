import { EditIndicator } from '@/global.types';
import {
  Channel,
  createChannel,
  DocumentCatalogOptions,
  queryChannels,
  updateChannel,
} from '@/services/gateway/document';
import { PlusOutlined } from '@ant-design/icons';
import type { ActionType } from '@ant-design/pro-components';
import { ProList } from '@ant-design/pro-components';
import {
  Button,
  Card,
  Dropdown,
  Form,
  Input,
  message,
  Modal,
  Row,
  Space,
  Switch,
  Tag,
  Typography,
} from 'antd';
import { MenuInfo } from 'rc-menu/lib/interface';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  fetchNewsCollectorConfig,
  saveNewsCollectorConfig,
  type TelegramNewsCollectorStored,
} from './api';
import ChannelModal from './components/ChannelModal';
import ExtractTestModal from './components/ExtractTestModal';
import TelegramAuthTool from './components/TelegramAuth';

type NewsCollectorForm = {
  enabled: boolean;
  appId: string;
  appHash: string;
  session: string;
};

const NewsSourceView: React.FC = () => {
  const actionRef = useRef<ActionType>();
  const [collectorForm] = Form.useForm<NewsCollectorForm>();
  const [collectorLoading, setCollectorLoading] = useState(true);
  const [collectorSaving, setCollectorSaving] = useState(false);
  const [authModalOpen, setAuthModalOpen] = useState(false);
  const [authModalKey, setAuthModalKey] = useState(0);

  const watchedAppId = Form.useWatch('appId', collectorForm);
  const watchedAppHash = Form.useWatch('appHash', collectorForm);

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

  const storedToForm = useCallback((s: TelegramNewsCollectorStored): NewsCollectorForm => {
    return {
      enabled: s.enabled,
      appId: s.app_id,
      appHash: s.app_hash,
      session: s.session,
    };
  }, []);

  const loadCollector = useCallback(async () => {
    setCollectorLoading(true);
    try {
      const stored = await fetchNewsCollectorConfig();
      const vals = storedToForm(stored);
      collectorForm.setFieldsValue(vals);
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败');
    } finally {
      setCollectorLoading(false);
    }
  }, [collectorForm, storedToForm]);

  useEffect(() => {
    loadCollector();
  }, [loadCollector]);

  const persistCollector = useCallback(
    async (partial?: Partial<NewsCollectorForm>) => {
      const cur = collectorForm.getFieldsValue();
      const merged: NewsCollectorForm = {
        enabled: partial?.enabled ?? cur.enabled ?? true,
        appId: partial?.appId ?? cur.appId ?? '',
        appHash: partial?.appHash ?? cur.appHash ?? '',
        session: partial?.session ?? cur.session ?? '',
      };
      const payload: TelegramNewsCollectorStored = {
        enabled: merged.enabled,
        app_id: merged.appId.trim(),
        app_hash: merged.appHash.trim(),
        session: merged.session.trim(),
      };
      await saveNewsCollectorConfig(payload);
      collectorForm.setFieldsValue(merged);
    },
    [collectorForm],
  );

  const handleSaveCollector = async () => {
    const values = await collectorForm.validateFields();
    setCollectorSaving(true);
    try {
      await persistCollector(values);
      message.success(
        values.enabled ? '资讯采集配置已保存（已开启采集）' : '资讯采集配置已保存（已关闭采集）',
      );
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败');
    } finally {
      setCollectorSaving(false);
    }
  };

  const openAuthModal = () => {
    const { appId, appHash } = collectorForm.getFieldsValue();
    if (!appId?.trim() || !appHash?.trim()) {
      message.warning('请先填写 App ID 与 App Hash');
      return;
    }
    setAuthModalKey((k) => k + 1);
    setAuthModalOpen(true);
  };

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

  const catalogLabel = (catalog: string) =>
    DocumentCatalogOptions.find((o) => o.value === catalog)?.label ?? catalog;

  const parseEnabledFilter = (raw: unknown): boolean | undefined => {
    if (raw === undefined || raw === '' || raw === null) {
      return undefined;
    }
    if (raw === true || raw === 'true') {
      return true;
    }
    if (raw === false || raw === 'false') {
      return false;
    }
    return undefined;
  };

  return (
    <>
      <Typography.Paragraph type="secondary">
        使用 Telegram 官方 MTProto 应用（my.telegram.org 申请 App ID / App
        Hash）登录账号并生成 Session，供资讯采集使用。
      </Typography.Paragraph>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>

        <Card loading={collectorLoading}>
          <Form form={collectorForm} layout="vertical">
            <Typography.Title level={5} style={{ marginTop: 0 }}>
              资讯采集配置
            </Typography.Title>
            <Form.Item
              label="开启采集"
              name="enabled"
              valuePropName="checked"
              tooltip="关闭后将停止 Telegram 资讯采集监听；开启后按下方配置尝试启动"
            >
              <Switch checkedChildren="开启" unCheckedChildren="关闭" />
            </Form.Item>
            <Form.Item label="App ID" name="appId">
              <Input autoComplete="off" placeholder="数字 App ID" />
            </Form.Item>
            <Form.Item label="App Hash" name="appHash">
              <Input.Password autoComplete="off" placeholder="App Hash" />
            </Form.Item>
            <Form.Item label="Session（授权成功后填入表单，可手动粘贴）" name="session">
              <Input.TextArea
                autoComplete="off"
                rows={3}
                placeholder="点击「授权」在弹窗内登录后自动填入，或粘贴已有 session"
                style={{ fontFamily: 'monospace' }}
              />
            </Form.Item>

            <Row justify="start" gutter={8}>
              <Space>
                <Button type="primary" loading={collectorSaving} onClick={handleSaveCollector}>
                  保存
                </Button>
                <Button onClick={openAuthModal}>重新授权</Button>
              </Space>
            </Row>
          </Form>

          <Modal
            title="Telegram 账号授权"
            open={authModalOpen}
            onCancel={() => setAuthModalOpen(false)}
            footer={null}
            destroyOnClose
            width={560}
          >
            <TelegramAuthTool
              key={authModalKey}
              embedded
              appId={String(watchedAppId ?? '')}
              appHash={String(watchedAppHash ?? '')}
              onSessionObtained={(session) => {
                collectorForm.setFieldValue('session', session);
                setAuthModalOpen(false);
                message.success('Session 已写入表单，请点击「保存」同步到服务端');
              }}
            />
          </Modal>
        </Card>

        <Card styles={{ body: { padding: 0 } }}>
          <ProList<Channel>
            actionRef={actionRef}
            rowKey="id"
            headerTitle="资讯频道列表"
            form={{ span: 6, initialValues: { enabled: 'true' } }}
            search={{
              labelWidth: 'auto',
              filterType: 'light',
              // labelWidth: 'auto',
              // showHiddenNum: true,
            }}
            pagination={{
              defaultPageSize: 10,
              showSizeChanger: false,
            }}
            showActions="hover"
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
                enabled: parseEnabledFilter(enabled),
              });

              return {
                data: result.list,
                success: true,
                total: result.totalCount,
              };
            }}
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
            metas={{
              title: {
                dataIndex: 'name',
                search: false,
                render: (_, row) => (
                  <Typography.Link
                    onClick={() => {
                      setChannelEditIndicator({
                        mode: 'readonly',
                        open: true,
                        value: row,
                      });
                    }}
                  >
                    {row.name}
                  </Typography.Link>
                ),
              },
              subTitle: {
                search: false,
                render: (_, row) => (
                  <Tag>{row.source}</Tag>
                ),
              },
              description: {
                search: false,
                render: (_, row) => (
                  <Space size={[4, 4]} wrap>
                    <Typography.Text type="secondary">
                      {row.title}
                    </Typography.Text>
                  </Space>
                ),
              },
              content: {
                search: false,
                render: (_, row) => (
                  <Space size={[4, 4]} wrap>
                    {row.enabled ? <Tag color="success">已启用</Tag> : <Tag color="default">已禁用</Tag>}
                    {row.broadcast ? <Tag color="green">Broadcast</Tag> : <Tag>No broadcast</Tag>}
                    <Tag>{catalogLabel(row.catalog)}</Tag>
                  </Space>
                ),
              },
              actions: {
                search: false,
                render: (_, row) => (
                  <Space direction="vertical" size={2}>
                    <Typography.Text type="secondary">
                      {new Date(row.createdAt * 1000).toLocaleString()}
                    </Typography.Text>
                  </Space>
                ),
              },
              extra: {
                search: false,
                render: (_: any, row: any) => renderMenus(row),
              },
              catalog: {
                title: 'Catalog',
                dataIndex: 'catalog',
                valueType: 'select',
                fieldProps: {
                  options: DocumentCatalogOptions,
                },
              },
              enabled: {
                title: 'Enabled',
                dataIndex: 'enabled',
                valueType: 'select',
                valueEnum: {
                  true: { text: '已启用', status: 'Success' },
                  false: { text: '已禁用', status: 'Error' },
                },
              },
            }}
          />
        </Card>

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
      </Space>
    </>
  );
};

export default NewsSourceView;
