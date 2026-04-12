import { queryPushConfig, updatePushConfig, type PushConfigRecord } from '@/services/gateway/telegram';
import { Button, Divider, Form, Input, Radio, Row, Space, Switch, Typography, message } from 'antd';
import React, { useCallback, useEffect, useState } from 'react';

type TelegramFormValues = {
  provider: 'telegram' | 'feishu';
  telegramBotToken: string;
  telegramChannelId: string;
  pushDocumentEnabled: boolean;
  pushAlarmEnabled: boolean;
  pushTradeEnabled: boolean;
  feishuWebhookUrl: string;
  feishuSecret: string;
  feishuKeyword: string;
};

const TelegramView: React.FC = () => {
  const [form] = Form.useForm<TelegramFormValues>();
  const [fullRow, setFullRow] = useState<PushConfigRecord | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const applyRowToForm = useCallback(
    (row: PushConfigRecord) => {
      setFullRow(row);
      form.setFieldsValue({
        telegramBotToken: row.telegramBotToken,
        provider: (row.provider as 'telegram' | 'feishu') || 'telegram',
        telegramChannelId: row.telegramChannelId,
        pushDocumentEnabled: row.pushDocumentEnabled,
        pushAlarmEnabled: row.pushAlarmEnabled,
        pushTradeEnabled: row.pushTradeEnabled,
        feishuWebhookUrl: row.feishuWebhookUrl,
        feishuSecret: row.feishuSecret,
        feishuKeyword: row.feishuKeyword,
      });
    },
    [form],
  );

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const row = await queryPushConfig();
      applyRowToForm(row);
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }, [applyRowToForm]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <>
      <Typography.Paragraph type="secondary">
        Telegram 推送：使用独立的推送 Bot 将摘要、告警、订单等发到指定 Chat。
      </Typography.Paragraph>
      <Form
        form={form}
        initialValues={{
          provider: 'telegram',
          pushDocumentEnabled: true,
          pushAlarmEnabled: true,
          pushTradeEnabled: true,
        }}
        onFinish={async (values) => {
          if (!fullRow) {
            message.error('配置未加载');
            return;
          }
          setSaving(true);
          try {
            const row = await updatePushConfig({
              ...fullRow,
              provider: values.provider,
              telegramBotToken: values.telegramBotToken?.trim() ?? '',
              telegramChannelId: values.telegramChannelId?.trim() ?? '',
              pushDocumentEnabled: values.pushDocumentEnabled,
              pushAlarmEnabled: values.pushAlarmEnabled,
              pushTradeEnabled: values.pushTradeEnabled,
              feishuWebhookUrl: values.feishuWebhookUrl?.trim() ?? '',
              feishuSecret: values.feishuSecret ?? '',
              feishuKeyword: values.feishuKeyword?.trim() ?? '',
            });
            message.success('已保存');
            applyRowToForm(row);
          } catch (e) {
            message.error(e instanceof Error ? e.message : '保存失败');
          } finally {
            setSaving(false);
          }
        }}
      >
        <Row justify="space-between">
          <div>
            <Typography.Text strong style={{ display: 'block' }}>
              推送渠道
            </Typography.Text>
            <Typography.Text type="secondary" style={{ display: 'block' }}>
              飞书 / Telegram 二选一（全局）
            </Typography.Text>
          </div>
          <Form.Item name="provider" style={{ margin: 6, padding: 0 }}>
            <Radio.Group
              options={[
                { label: 'Telegram', value: 'telegram' },
                { label: '飞书', value: 'feishu' },
              ]}
              optionType="button"
              buttonStyle="solid"
            />
          </Form.Item>
        </Row>
        <Divider size="small" />
        <Form.Item noStyle shouldUpdate={(prev, cur) => prev.provider !== cur.provider}>
          {({ getFieldValue }) =>
            getFieldValue('provider') === 'telegram' ? (
              <>
                <Row justify="space-between">
                  <div>
                    <Typography.Text strong style={{ display: 'block' }}>
                      Telegram Bot Token
                    </Typography.Text>
                    <Typography.Text type="secondary" style={{ display: 'block' }}>
                      从 @BotFather 获取；留空并保存将删除
                    </Typography.Text>
                  </div>
                  <Form.Item name="telegramBotToken" style={{ width: '30%', margin: 6, padding: 0 }}>
                    <Input.Password autoComplete="off" placeholder="推送 Bot Token" />
                  </Form.Item>
                </Row>
                <Divider size="small" />
                <Row justify="space-between">
                  <div>
                    <Typography.Text strong style={{ display: 'block' }}>
                    Telegram 群组 ID
                    </Typography.Text>
                    <Typography.Text type="secondary" style={{ display: 'block' }}>
                      整数，可为负数（群组）；留空并保存将删除
                    </Typography.Text>
                  </div>
                  <Form.Item name="telegramChannelId" style={{ width: '30%', margin: 6, padding: 0 }}>
                    <Input placeholder="整数，可为负数（群组）；留空并保存将删除" />
                  </Form.Item>
                </Row>
              </>
            ) : (
              <>
                <Row justify="space-between">
                  <div>
                    <Typography.Text strong style={{ display: 'block' }}>
                      飞书 Webhook URL
                    </Typography.Text>
                    <Typography.Text type="secondary" style={{ display: 'block' }}>
                      飞书机器人 Webhook 全链接
                    </Typography.Text>
                  </div>
                  <Form.Item name="feishuWebhookUrl" style={{ width: '45%', margin: 6, padding: 0 }}>
                    <Input autoComplete="off" placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..." />
                  </Form.Item>
                </Row>
                <Divider size="small" />
                <Row justify="space-between">
                  <div>
                    <Typography.Text strong style={{ display: 'block' }}>
                      飞书 Secret
                    </Typography.Text>
                    <Typography.Text type="secondary" style={{ display: 'block' }}>
                      机器人启用签名校验时填写
                    </Typography.Text>
                  </div>
                  <Form.Item name="feishuSecret" style={{ width: '45%', margin: 6, padding: 0 }}>
                    <Input.Password autoComplete="off" placeholder="可选" />
                  </Form.Item>
                </Row>
                <Divider size="small" />
                <Row justify="space-between">
                  <div>
                    <Typography.Text strong style={{ display: 'block' }}>
                      飞书关键词
                    </Typography.Text>
                    <Typography.Text type="secondary" style={{ display: 'block' }}>
                      机器人启用关键词校验时填写
                    </Typography.Text>
                  </div>
                  <Form.Item name="feishuKeyword" style={{ width: '45%', margin: 6, padding: 0 }}>
                    <Input autoComplete="off" placeholder="可选" />
                  </Form.Item>
                </Row>
              </>
            )
          }
        </Form.Item>
        <Divider size="small" />
        <Row justify="space-between">
          <div>
            <Typography.Text strong style={{ display: 'block' }}>
              文档推送
            </Typography.Text>
            <Typography.Text type="secondary" style={{ display: 'block' }}>
              开启后，文档摘要将发往推送渠道
            </Typography.Text>
          </div>
          <Form.Item name="pushDocumentEnabled" valuePropName="checked" style={{ margin: 6, padding: 0 }}>
            <Switch checkedChildren="开" unCheckedChildren="关" />
          </Form.Item>
        </Row>
        <Divider size="small" />
        <Row justify="space-between">
          <div>
            <Typography.Text strong style={{ display: 'block' }}>
              告警推送
            </Typography.Text>
            <Typography.Text type="secondary" style={{ display: 'block' }}>
              开启后，服务/策略/价格告警将发往推送渠道
            </Typography.Text>
          </div>
          <Form.Item name="pushAlarmEnabled" valuePropName="checked" style={{ margin: 6, padding: 0 }}>
            <Switch checkedChildren="开" unCheckedChildren="关" />
          </Form.Item>
        </Row>
        <Divider size="small" />
        <Row justify="space-between">
          <div>
            <Typography.Text strong style={{ display: 'block' }}>
              交易推送
            </Typography.Text>
            <Typography.Text type="secondary" style={{ display: 'block' }}>
              开启后，交易（订单）将发往推送渠道
            </Typography.Text>
          </div>
          <Form.Item name="pushTradeEnabled" valuePropName="checked" style={{ margin: 6, padding: 0 }}>
            <Switch checkedChildren="开" unCheckedChildren="关" />
          </Form.Item>
        </Row>
        <Divider size="small" />
        <Form.Item style={{ marginTop: 24 }}>
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

export default TelegramView;
