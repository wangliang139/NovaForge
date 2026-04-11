import { queryNetworkProxyConfig, updateNetworkProxyConfig } from '@/services/gateway/proxy';
import { requestGatewayRestart } from '@/services/gateway/system';
import { useModel } from '@umijs/max';
import { Button, Form, Input, Modal, Space, Typography, message } from 'antd';
import React, { useCallback, useEffect, useState } from 'react';

const ProxyView: React.FC = () => {
  const { initialState } = useModel('@@initialState');
  const canAdmin = initialState?.currentUser?.access === 'admin';

  const [form] = Form.useForm<{ httpProxyUrl: string }>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const row = await queryNetworkProxyConfig();
      form.setFieldsValue({ httpProxyUrl: row.httpProxyUrl });
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }, [form]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <>
      <Typography.Paragraph type="secondary">
        出站 HTTP/HTTPS 代理（例如 <Typography.Text code>http://127.0.0.1:7890</Typography.Text>
        ）。留空表示直连。保存后交易所连接与 Telegram 相关请求将使用该设置。
      </Typography.Paragraph>
      <Form
        form={form}
        layout="vertical"
        onFinish={async (values) => {
          setSaving(true);
          try {
            const row = await updateNetworkProxyConfig({
              httpProxyUrl: values.httpProxyUrl?.trim() ?? '',
            });
            form.setFieldsValue({ httpProxyUrl: row.httpProxyUrl });
            if (canAdmin) {
              message.success('已保存');
              Modal.confirm({
                title: '是否立即重启网关？',
                content:
                  '代理等新配置通常在进程重新启动后生效。重启会中断当前连接；若使用 Docker，请为容器配置自动重启策略（如 restart: unless-stopped），否则服务可能不会自动拉起。',
                okText: '立即重启',
                cancelText: '稍后',
                onOk: async () => {
                  try {
                    await requestGatewayRestart();
                    message.success('已请求重启，请稍候刷新页面');
                  } catch (e) {
                    message.error(e instanceof Error ? e.message : '重启请求失败');
                    return false;
                  }
                },
              });
            } else {
              message.success('已保存。代理生效可能需管理员重启网关。');
            }
          } catch (e) {
            message.error(e instanceof Error ? e.message : '保存失败');
          } finally {
            setSaving(false);
          }
        }}
      >
        <Typography.Text strong style={{ display: 'block', marginBottom: 6 }}>
          代理 URL
        </Typography.Text>
        <Form.Item
          name="httpProxyUrl"
          rules={[
            {
              validator: async (_, v) => {
                const s = (v ?? '').trim();
                if (!s) {
                  return;
                }
                try {
                  const u = new URL(s);
                  if (u.protocol !== 'http:' && u.protocol !== 'https:') {
                    throw new Error();
                  }
                } catch {
                  throw new Error('请输入有效的 http 或 https URL');
                }
              },
            },
          ]}
        >
          <Input autoComplete="off" placeholder="https://proxy.example.com:8080" />
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={saving}>
              保存
            </Button>
            <Button onClick={() => load()} disabled={loading || saving}>
              重新加载
            </Button>
          </Space>
        </Form.Item>
      </Form>
    </>
  );
};

export default ProxyView;
