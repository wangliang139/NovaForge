import { Form, Input, Space, Button, Typography, message } from 'antd';
import React, { useCallback, useEffect, useState } from 'react';
import { fetchTavilyConfig, saveTavilyConfig } from './llm-tools_api';

const LlmToolsView: React.FC = () => {
  const [form] = Form.useForm<{ tavilyApiKey: string }>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const cfg = await fetchTavilyConfig();
      form.setFieldsValue({ tavilyApiKey: cfg.tavilyApiKey });
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
        AI 工具配置：目前支持 Tavily Web Search。留空则默认不启用该工具。
      </Typography.Paragraph>
      <Form
        form={form}
        layout="vertical"
        onFinish={async (values) => {
          setSaving(true);
          try {
            await saveTavilyConfig({
              tavilyApiKey: values.tavilyApiKey?.trim() ?? '',
            });
            message.success('已保存');
            await load();
          } catch (e) {
            message.error(e instanceof Error ? e.message : '保存失败');
          } finally {
            setSaving(false);
          }
        }}
      >
        <Typography.Text strong style={{ display: 'block', marginBottom: 6 }}>
          Tavily API Key
        </Typography.Text>
        <Form.Item name="tavilyApiKey">
          <Input.Password
            autoComplete="off"
          />
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

export default LlmToolsView;

