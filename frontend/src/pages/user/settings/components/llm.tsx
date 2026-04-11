import { queryLlmProviderConfig, updateLlmProviderConfig } from '@/services/gateway/llm';
import { Button, Divider, Form, Input, Space, Typography, message } from 'antd';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import LlmToolsView from './llm-tools';

const LlmView: React.FC = () => {
  const [form] = Form.useForm<{ openRouterApiKey: string; defaultModel: string }>();
  const siliconFlowPreserved = useRef('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const applyConfigToForm = useCallback(
    (openRouterApiKey: string, defaultModel: string) => {
      form.setFieldsValue({ openRouterApiKey, defaultModel });
    },
    [form],
  );

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const row = await queryLlmProviderConfig();
      siliconFlowPreserved.current = row.siliconFlowApiKey;
      applyConfigToForm(row.openRouterApiKey, row.defaultModel);
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }, [applyConfigToForm]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <>
      <Typography.Paragraph type="secondary">
        AI 大模型网关 API Key（OpenRouter）。默认模型用于新建聊天会话等场景；留空则使用系统内置模型。
      </Typography.Paragraph>
      <Form
        form={form}
        layout="vertical"
        onFinish={async (values) => {
          setSaving(true);
          try {
            const row = await updateLlmProviderConfig({
              openRouterApiKey: values.openRouterApiKey?.trim() ?? '',
              siliconFlowApiKey: siliconFlowPreserved.current,
              defaultModel: values.defaultModel?.trim() ?? '',
            });
            message.success('已保存');
            siliconFlowPreserved.current = row.siliconFlowApiKey;
            applyConfigToForm(row.openRouterApiKey, row.defaultModel);
          } catch (e) {
            message.error(e instanceof Error ? e.message : '保存失败');
          } finally {
            setSaving(false);
          }
        }}
      >
        <Typography.Text strong style={{ display: 'block', marginBottom: 6 }}>OpenRouter API Key</Typography.Text>
        <Form.Item name="openRouterApiKey">
          <Input.Password autoComplete="off" placeholder="https://openrouter.ai/keys" />
        </Form.Item>
        <Typography.Text strong style={{ display: 'block', marginBottom: 6 }}>默认模型</Typography.Text>
        <Form.Item name="defaultModel">
          <Input autoComplete="off" placeholder="例如 minimax/minimax-m2.5" />
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
      <Divider />
      <Typography.Title level={5} style={{ marginTop: 0 }}>
        工具配置
      </Typography.Title>
      <LlmToolsView />
    </>
  );
};

export default LlmView;
