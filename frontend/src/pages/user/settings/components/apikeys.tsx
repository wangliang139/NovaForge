import type { UserApiKeyPermission, UserApiKeyRecord } from '@/services/gateway/apikey';
import {
  createUserApiKey,
  deleteUserApiKey,
  queryUserApiKeyNameAvailable,
  queryUserApiKeys,
} from '@/services/gateway/apikey';
import { CheckCircleOutlined, CloseCircleOutlined, LoadingOutlined, PlusOutlined } from '@ant-design/icons';
import type { ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import {
  Button,
  Checkbox,
  Form,
  Input,
  Modal,
  Popconfirm,
  Space,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import React, { useCallback, useEffect, useMemo, useState } from 'react';

const permLabel: Record<UserApiKeyPermission, string> = {
  query: '查询',
  trade: '交易',
};

type NameCheckState = 'idle' | 'checking' | 'ok' | 'taken' | 'error';

/** 与后端 usersvc.MaxActiveUserAPIKeys 一致 */
const MAX_ACTIVE_API_KEYS = 10;

const ApiKeysView: React.FC = () => {
  const [createOpen, setCreateOpen] = useState(false);
  const [form] = Form.useForm<{ name: string; trade: boolean }>();
  const nameWatch = Form.useWatch('name', form);
  const [list, setList] = useState<UserApiKeyRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [nameCheck, setNameCheck] = useState<NameCheckState>('idle');

  const loadList = useCallback(async () => {
    setLoading(true);
    try {
      const rows = await queryUserApiKeys();
      setList(rows);
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '加载 API 密钥失败');
      setList([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadList();
  }, [loadList]);

  useEffect(() => {
    if (!createOpen) {
      return;
    }
    const raw = typeof nameWatch === 'string' ? nameWatch : '';
    const trimmed = raw.trim();
    if (!trimmed) {
      setNameCheck('idle');
      return;
    }
    setNameCheck('checking');
    const timer = window.setTimeout(async () => {
      try {
        const avail = await queryUserApiKeyNameAvailable(trimmed);
        setNameCheck(avail ? 'ok' : 'taken');
      } catch {
        setNameCheck('error');
      }
    }, 400);
    return () => clearTimeout(timer);
  }, [nameWatch, createOpen]);

  const atKeyLimit = list.length >= MAX_ACTIVE_API_KEYS;

  const nameSuffixIcon = useMemo(() => {
    if (nameCheck === 'checking') {
      return <LoadingOutlined />;
    }
    if (nameCheck === 'ok') {
      return (
        <Tooltip title="名称可用">
          <CheckCircleOutlined style={{ color: '#52c41a' }} />
        </Tooltip>
      );
    }
    if (nameCheck === 'taken') {
      return (
        <Tooltip title="名称已被占用">
          <CloseCircleOutlined style={{ color: '#ff4d4f' }} />
        </Tooltip>
      );
    }
    if (nameCheck === 'error') {
      return (
        <Tooltip title="校验失败，提交时后台仍会校验">
          <CloseCircleOutlined style={{ color: '#faad14' }} />
        </Tooltip>
      );
    }
    return null;
  }, [nameCheck]);

  const columns = useMemo<ProColumns<UserApiKeyRecord>[]>(
    () => [
      { title: '名称', dataIndex: 'name', width: 160 },
      { title: '密钥前缀', dataIndex: 'keyPrefix', ellipsis: true },
      {
        title: '权限',
        dataIndex: 'permissions',
        render: (_, row) => (
          <Space size={4} wrap>
            {(row.permissions || []).map((p) => (
              <Tag key={p}>{permLabel[p] ?? p}</Tag>
            ))}
          </Space>
        ),
      },
      {
        title: '创建时间',
        dataIndex: 'createdAt',
        width: 200,
        render: (_, row) =>
          row.createdAt != null && row.createdAt > 0
            ? new Date(row.createdAt).toLocaleString()
            : '—',
      },
      {
        title: '操作',
        valueType: 'option',
        width: 100,
        render: (_, row) => [
          <Popconfirm
            key="del"
            title="确定删除该密钥？删除后无法恢复。"
            onConfirm={async () => {
              try {
                await deleteUserApiKey(row.id);
                message.success('已删除');
                await loadList();
              } catch (e: unknown) {
                message.error(e instanceof Error ? e.message : '删除失败');
              }
            }}
          >
            <Button type="link" danger size="small">
              删除
            </Button>
          </Popconfirm>,
        ],
      },
    ],
    [loadList],
  );

  return (
    <>
      <Space style={{ marginBottom: 16 }} align="center" wrap>
        <Tooltip
          title={
            atKeyLimit
              ? `已达到上限（${MAX_ACTIVE_API_KEYS} 个生效中），请先删除不再使用的密钥`
              : undefined
          }
        >
        </Tooltip>
        <Typography.Text type="secondary">
          用于 MCP 与 Webhook 鉴权等场景。
          调用 HTTP 时在请求头加入{' '}
          <Typography.Text code>X-API-Key</Typography.Text>。完整密钥仅在创建时显示一次。
        </Typography.Text>
      </Space>
      <ProTable<UserApiKeyRecord>
        rowKey={(r) => String(r.id)}
        search={false}
        options={false}
        pagination={false}
        loading={loading}
        dataSource={list}
        columns={columns}
      />
      <Button
        type="primary"
        icon={<PlusOutlined />}
        disabled={atKeyLimit}
        style={{ marginTop: 16 }}
        onClick={() => {
          form.resetFields();
          setNameCheck('idle');
          setCreateOpen(true);
        }}
      >
        新建 API 密钥
      </Button>

      <Modal
        title="新建 API 密钥"
        open={createOpen}
        destroyOnHidden
        onCancel={() => setCreateOpen(false)}
        okText="创建"
        okButtonProps={{
          disabled: nameCheck === 'checking',
        }}
        onOk={async () => {
          try {
            const v = await form.validateFields();
            const trimmed = v.name.trim();
            if (!trimmed) {
              return;
            }
            if (nameCheck === 'checking') {
              message.warning('请等待名称校验完成');
              return;
            }
            const finalAvail = await queryUserApiKeyNameAvailable(trimmed);
            if (!finalAvail) {
              setNameCheck('taken');
              message.error('名称已被占用，请更换');
              return;
            }
            if (list.length >= MAX_ACTIVE_API_KEYS) {
              message.warning(`最多 ${MAX_ACTIVE_API_KEYS} 个生效中的 API 密钥，请先删除后再创建`);
              return;
            }
            const permissions: UserApiKeyPermission[] = ['query'];
            if (v.trade) {
              permissions.push('trade');
            }
            const { plainSecret, record } = await createUserApiKey(trimmed, permissions);
            setCreateOpen(false);
            form.resetFields();
            setNameCheck('idle');
            await loadList();
            Modal.success({
              width: 560,
              title: '请保存您的密钥',
              content: (
                <div>
                  <p style={{ marginBottom: 8 }}>以下密钥仅显示一次，关闭后将无法再次查看完整内容。</p>
                  <Typography.Paragraph copyable={{ text: plainSecret }} strong>
                    {plainSecret}
                  </Typography.Paragraph>
                  <p style={{ marginTop: 12, color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
                    名称：{record.name}；前缀：{record.keyPrefix}
                  </p>
                </div>
              ),
            });
          } catch (e: unknown) {
            if (e && typeof e === 'object' && 'errorFields' in e) {
              return;
            }
            message.error(e instanceof Error ? e.message : '创建失败');
          }
        }}
      >
        <Form form={form} layout="vertical" initialValues={{ trade: false }}>
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '请输入唯一名称' }]}
            validateStatus={
              nameCheck === 'taken' ? 'error' : nameCheck === 'ok' ? 'success' : undefined
            }
            help={
              nameCheck === 'taken'
                ? '该名称已被占用'
                : nameCheck === 'error'
                  ? '无法完成在线校验，仍可尝试提交（服务端会再次校验）'
                  : undefined
            }
          >
            <Input
              placeholder="便于识别的名称，同一用户下不可重复"
              maxLength={128}
              suffix={
                <span style={{ display: 'inline-flex', alignItems: 'center', minWidth: 14 }}>
                  {nameSuffixIcon}
                </span>
              }
            />
          </Form.Item>
          <Form.Item label="权限">
            <Space direction="vertical">
              <Checkbox checked disabled>
                查询（默认）
              </Checkbox>
              <Form.Item name="trade" valuePropName="checked" noStyle>
                <Checkbox>交易（下单、策略与账户相关变更等，受服务端白名单约束）</Checkbox>
              </Form.Item>
            </Space>
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

export default ApiKeysView;
