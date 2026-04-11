import { CodeEditor, CompositeTags, ReadonlyWrapper } from '@/components';
import { EditIndicator } from '@/global.types';
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  ExperimentOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import {
  ModalForm,
  ProCard,
  ProDescriptions,
  ProForm,
  ProFormDigit,
  ProFormSelect,
  ProFormSwitch,
  ProFormText,
  ProList,
} from '@ant-design/pro-components';
import {
  Button,
  Card,
  Col,
  Divider,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Tag,
  Typography,
  message,
} from 'antd';
import React, { useEffect, useRef, useState } from 'react';
import { testPrompt } from '../service';
import useStyles from '../style.style';
import { CompletionMetadata, CompletionUsage, LlmMessage, LlmPrompt, PlatformType } from '../types';
import { getRoleAvatar } from './LlmRole';
import PromptMessageModal from './PromptMessageModal';

const { TextArea } = Input;
const { Text } = Typography;

type TestResult = {
  success: boolean;
  result?: string;
  error?: string;
  duration?: number;
  usage?: CompletionUsage;
  metadata?: CompletionMetadata;
};

type PromptModalProps = {
  mode: 'new' | 'edit' | 'readonly';
  open: boolean;
  value?: LlmPrompt;
  onOpenChange: (open: boolean) => void;
  onFinish?: (formData: LlmPrompt) => Promise<boolean | void> | void;
};

const defaultValues = {
  name: '',
  model: '',
  platform: '',
  providers: [],
  variants: [],
  enabled: true,
  config: {},
  messages: [],
  timeout: 300,
  weight: 100,
};

const DefaultVariants = ['default', 'free', 'downgrade'];

const PromptModal: React.FC<PromptModalProps> = ({ mode, open, value, onOpenChange, onFinish }) => {
  const { styles } = useStyles();

  const [form] = Form.useForm();

  const [messages, setMessages] = useState<readonly LlmMessage[]>([]);
  const [providers, setProviders] = useState<string[]>([]);

  const platformValue = Form.useWatch('platform', form);

  const [messageEditIndicator, setMessageEditIndicator] = useState<EditIndicator<LlmMessage>>({
    value: null,
    index: -1,
    open: false,
  });

  // 测试相关状态
  const [testVariablesOpen, setTestVariablesOpen] = useState(false);
  const [testVariables, setTestVariables] = useState<string>('{}');
  const [testVariablesStatus, setTestVariablesStatus] = useState<'error' | 'warning' | undefined>();
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [testResultModalOpen, setTestResultModalOpen] = useState(false);
  const [testResultTabActiveKey, setTestResultTabActiveKey] = useState<string>('variables');
  const [loadingModalOpen, setLoadingModalOpen] = useState(false);
  const [loadingStartTime, setLoadingStartTime] = useState<number>(0);
  const [elapsedSeconds, setElapsedSeconds] = useState<number>(0);
  const timerRef = useRef<NodeJS.Timeout | null>(null);

  const readonly = mode === 'readonly';

  const onMessagesChange = (messages: LlmMessage[]) => {
    setMessages(messages);
    form.setFieldsValue({ messages: messages });
    form.validateFields(['messages']);
  };

  const onProvidersChange = (providers: string[]) => {
    setProviders(providers);
    form.setFieldsValue({ providers: providers });
    form.validateFields(['providers']);
  };

  useEffect(() => {
    if (!open) {
      return;
    }
    form.setFieldsValue({
      ...(mode === 'new' ? defaultValues : {}),
      ...(value || {}),
    });
    setMessages(value?.messages || []);
    setProviders(value?.providers || []);
  }, [value, mode, open]);

  // 计时器效果
  useEffect(() => {
    if (loadingModalOpen && loadingStartTime > 0) {
      timerRef.current = setInterval(() => {
        setElapsedSeconds(Math.floor((Date.now() - loadingStartTime) / 1000));
      }, 1000);
    } else {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
      setElapsedSeconds(0);
    }
    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
      }
    };
  }, [loadingModalOpen, loadingStartTime]);

  // JSON 验证
  const validateJson = (jsonStr: string): boolean => {
    try {
      const parsed = JSON.parse(jsonStr);
      if (typeof parsed !== 'object' || Array.isArray(parsed)) {
        return false;
      }
      return true;
    } catch {
      return false;
    }
  };

  // 测试函数
  const handleTest = async () => {
    if (!validateJson(testVariables)) {
      message.error('请输入有效的 JSON 对象');
      setTestVariablesStatus('error');
      return;
    }

    setTesting(true);
    const startTime = Date.now();
    setLoadingModalOpen(true);
    setLoadingStartTime(startTime);
    setTestVariablesStatus(undefined);

    try {
      const variablesJsonStr = JSON.stringify(JSON.parse(testVariables));

      const formValues = form.getFieldsValue();

      const res = await testPrompt({
        sceneId: value?.sceneId || '',
        byPrompt: formValues,
        variables: variablesJsonStr,
      });

      setLoadingModalOpen(false);
      setLoadingStartTime(0);

      if (!res.errors && res.data?.Result) {
        const result = res.data.Result;
        setTestResult({
          success: result.success || false,
          result: result.result || '',
          error: result.error,
          duration: result.duration,
          usage: result.usage,
          metadata: result.metadata,
        });
        setTestResultModalOpen(true);
        setTestVariablesOpen(false);
        message.success('测试完成');
      } else {
        setTestResult({
          success: false,
          error: res.errors?.[0]?.message || '测试失败',
        });
        setTestResultModalOpen(true);
        setTestVariablesOpen(false);
        message.error('测试失败');
      }
    } catch (error: any) {
      setLoadingModalOpen(false);
      setLoadingStartTime(0);
      setTestResult({
        success: false,
        error: error.message || '测试失败',
      });
      setTestResultModalOpen(true);
      setTestVariablesOpen(false);
      message.error('测试失败: ' + error.message);
    } finally {
      setTesting(false);
    }
  };

  return (
    <>
      <ModalForm<LlmPrompt>
        form={form}
        title={`${mode === 'new' ? '新建' : mode === 'edit' ? '编辑' : '查看'} LLM Prompt`}
        width="760px"
        open={open}
        layout="vertical"
        onOpenChange={onOpenChange}
        readonly={readonly}
        submitter={{
          render: (props, defaultDoms) => {
            const buttons = [
              <Button
                key="test"
                icon={<ExperimentOutlined />}
                color="orange"
                variant="solid"
                onClick={async () => {
                  const valid = await form.validateFields([
                    'model',
                    'providers',
                    'platform',
                    'messages',
                    'config',
                    'timeout',
                  ]);
                  if (!valid) {
                    return;
                  }
                  setTestVariables('{}');
                  setTestVariablesStatus(undefined);
                  setTestVariablesOpen(true);
                }}
              >
                测试
              </Button>,
            ];
            if (!readonly) {
              buttons.push(...defaultDoms);
            }
            return buttons;
          },
        }}
        modalProps={{
          centered: true,
          footer: null,
        }}
        style={{
          paddingTop: 20,
        }}
        onFinish={async (values) => {
          if (onFinish) {
            return await onFinish(values);
          }
          return true;
        }}
      >
        <Row justify="space-between" style={{ width: '100%' }}>
          <Col span={12}>
            <ProFormText
              name="name"
              label="Name"
              width="md"
              rules={[
                { required: true, message: '请输入 Name' },
                { max: 100, message: '最多 100 个字符' },
              ]}
              tooltip="用于识别 Prompt，使用有意义的名称，尽量不要重复"
              fieldProps={{
                count: {
                  show: true,
                  max: 100,
                },
              }}
            />
          </Col>
          <Col span={6}>
            <ProFormDigit
              name="timeout"
              label="Timeout (秒)"
              width={140}
              rules={[{ required: true, message: '请输入 Timeout', type: 'number', min: 0 }]}
            />
          </Col>
          <Col span={6}>
            <ProFormSwitch name="enabled" label="Enabled" width="md" rules={[{ required: true }]} />
          </Col>
        </Row>

        <Row justify="space-between" style={{ width: '100%' }}>
          <Col span={12}>
            <ProFormText
              name="model"
              label="Model"
              width="md"
              disabled={readonly || mode === 'edit'}
              rules={[
                { required: true, message: '请输入 Model' },
                { max: 200, message: '最多 200 个字符' },
              ]}
              fieldProps={{
                count: {
                  show: true,
                  max: 200,
                },
              }}
            />
          </Col>
          <Col span={6}>
            <ProFormSelect
              name="platform"
              label="Platform"
              width={140}
              disabled={readonly || mode === 'edit'}
              rules={[{ required: true, message: '请选择 Platform' }]}
              options={Object.values(PlatformType).map((platform) => ({
                label: platform,
                value: platform,
              }))}
            />
          </Col>
          <Col span={6}>
            <ProFormDigit
              name="weight"
              label="Weight"
              width="xs"
              max={100}
              min={0}
              rules={[
                { required: true, message: '请输入 Weight', type: 'number', min: 0, max: 100 },
              ]}
              tooltip="用于AB测试分流，0-100，越大权重越高"
            />
          </Col>
        </Row>

        <ProForm.Item
          name="providers"
          label="Providers"
          hidden={platformValue !== PlatformType.OPENROUTER}
          tooltip="仅 platform:openrouter 支持"
          rules={[
            {
              validator: async (_: any, value: string[]) => {
                if (value && value.length > 5) {
                  return Promise.reject(new Error('At most 5 providers'));
                }
                if (value && value.some((v) => v.length > 50)) {
                  return Promise.reject(new Error('单个 Provider 不超过 50 个字符'));
                }
              },
            },
          ]}
        >
          <Select mode="multiple" style={{ display: 'none' }} />
          <CompositeTags
            value={providers}
            maxLength={5}
            readonly={readonly || mode === 'edit'}
            draggable={true}
            plusTagLabel="New Provider"
            onChange={onProvidersChange}
          />
        </ProForm.Item>

        <ProForm.Item
          name="variants"
          label="Variants"
          tooltip="用于通过 scene_key:variant 精确匹配 Prompt"
          rules={[
            {
              validator: async (_: any, value: string[]) => {
                if (value && value.length > 3) {
                  return Promise.reject(new Error('At most 3 variants'));
                }
                if (value && value.some((v) => v.length > 20)) {
                  return Promise.reject(new Error('单个 Variant 不超过 20 个字符'));
                }
                if (value && value.some((v) => !/^[a-zA-Z0-9_]+$/.test(v))) {
                  return Promise.reject(new Error('只能包含大小写字母、数字和下划线'));
                }
              },
            },
          ]}
        >
          <ReadonlyWrapper readonly={readonly}>
            <Select
              mode="tags"
              style={{ width: '50%' }}
              options={DefaultVariants.map((variant) => ({ label: variant, value: variant }))}
            />
          </ReadonlyWrapper>
        </ProForm.Item>

        <ProForm.Item name="messages" label="Messages" width="md" rules={[{ required: true }]}>
          <ProList<LlmMessage>
            rowKey={(_, index) => index?.toString() || ''}
            dataSource={messages as unknown as LlmMessage[]}
            showActions="hover"
            metas={{
              avatar: {
                dataIndex: 'role',
                render: (_, row) => {
                  return getRoleAvatar(row);
                },
              },
              description: {
                dataIndex: 'content',
                render: (_, row) => {
                  return (
                    <Typography.Paragraph type="secondary" ellipsis={{ rows: 5 }} copyable>
                      {row.content}
                    </Typography.Paragraph>
                  );
                },
              },
              actions: {
                render: (text, row, index) => [
                  <Button
                    type="link"
                    key="view"
                    disabled={readonly || mode === 'edit'}
                    icon={<EditOutlined />}
                    onClick={() => {
                      setMessageEditIndicator({
                        mode: 'edit',
                        open: true,
                        value: row,
                        index: index,
                      });
                    }}
                  />,
                  <Button
                    type="link"
                    key="copy"
                    disabled={readonly || mode === 'edit'}
                    icon={<CopyOutlined />}
                    onClick={() => {
                      setMessageEditIndicator({
                        mode: 'new',
                        open: true,
                        value: row,
                        index: -1,
                      });
                    }}
                  />,
                  <Button
                    type="link"
                    key="delete"
                    disabled={readonly || mode === 'edit'}
                    icon={<DeleteOutlined />}
                    onClick={() => {
                      onMessagesChange(messages.filter((_, i) => i !== index));
                    }}
                  />,
                ],
              },
            }}
          />
          {!readonly && mode !== 'edit' && (
            <>
              <Divider size="small" />
              <Button
                block
                type="dashed"
                icon={<PlusOutlined />}
                onClick={() => {
                  setMessageEditIndicator({
                    mode: 'new',
                    open: true,
                    value: null,
                    index: -1,
                  });
                }}
              >
                添加一行
              </Button>
            </>
          )}
        </ProForm.Item>

        <ProForm.Item name="config" layout="horizontal" required>
          <div className={styles.required} style={{ marginBottom: 10 }}>
            Config
          </div>
          <ProCard bordered>
            <Row align="middle">
              <Col span={12} className={styles.lastChildNoBottomMargin}>
                <Form.Item
                  name={['config', 'temperature']}
                  label="Temperature"
                  layout="horizontal"
                  labelAlign="right"
                  labelCol={{ span: 12 }}
                  wrapperCol={{ span: 12 }}
                  tooltip="Controls the randomness of the model’s output. Higher values (closer to 1) make output more random, while lower values make the output more deterministic."
                >
                  <ReadonlyWrapper readonly={readonly || mode === 'edit'}>
                    <InputNumber min={0} max={2} step={0.1} />
                  </ReadonlyWrapper>
                </Form.Item>
                <Form.Item
                  name={['config', 'maxTokens']}
                  label="Max Tokens"
                  layout="horizontal"
                  labelAlign="right"
                  labelCol={{ span: 12 }}
                  wrapperCol={{ span: 12 }}
                >
                  <ReadonlyWrapper readonly={readonly || mode === 'edit'}>
                    <InputNumber min={0} step={1} />
                  </ReadonlyWrapper>
                </Form.Item>
              </Col>
              <Col span={12} className={styles.lastChildNoBottomMargin}>
                <Form.Item
                  name={['config', 'topP']}
                  label="Top P"
                  layout="horizontal"
                  labelAlign="right"
                  labelCol={{ span: 12 }}
                  wrapperCol={{ span: 12 }}
                  tooltip="Specifies the diversity of the model’s output. Similar to temperature but more precise."
                >
                  <ReadonlyWrapper readonly={readonly || mode === 'edit'}>
                    <InputNumber min={0.1} max={1} step={0.1} />
                  </ReadonlyWrapper>
                </Form.Item>
                <Form.Item
                  name={['config', 'maxCompletionTokens']}
                  label="Max Completion Tokens"
                  layout="horizontal"
                  labelAlign="right"
                  labelCol={{ span: 12 }}
                  wrapperCol={{ span: 12 }}
                >
                  <ReadonlyWrapper readonly={readonly || mode === 'edit'}>
                    <InputNumber min={0} step={1} />
                  </ReadonlyWrapper>
                </Form.Item>
              </Col>
            </Row>
          </ProCard>
        </ProForm.Item>
      </ModalForm>

      <PromptMessageModal
        open={messageEditIndicator.open}
        value={messageEditIndicator.value || undefined}
        onOpenChange={(open) => {
          setMessageEditIndicator((prev) => ({
            ...prev,
            open: open,
          }));
        }}
        onFinish={async (values) => {
          if (messageEditIndicator.index === -1) {
            onMessagesChange([...messages, values]);
          } else {
            onMessagesChange(
              messages.map((message, index) =>
                index === messageEditIndicator.index ? { ...message, ...values } : message,
              ),
            );
          }
          setMessageEditIndicator((prev) => ({
            open: false,
            value: null,
            index: -1,
          }));
          return true;
        }}
      />

      {/* 测试参数输入 Modal */}
      <Modal
        title="测试参数"
        open={testVariablesOpen}
        onCancel={() => setTestVariablesOpen(false)}
        onOk={handleTest}
        confirmLoading={testing}
        okText="测试"
        width={600}
        centered
      >
        <Space direction="vertical" style={{ width: '100%', marginTop: 10 }} size="middle">
          <CodeEditor
            value={testVariables}
            onChange={setTestVariables}
            placeholder='请输入 JSON 对象，例如: {"key": "value"}'
            onBlur={() => {
              if (!validateJson(testVariables)) {
                setTestVariablesStatus('error');
              } else {
                setTestVariablesStatus(undefined);
              }
            }}
          />
        </Space>
      </Modal>

      {/* 加载 Modal */}
      <Modal title="测试进行中" open={loadingModalOpen} closable={false} footer={null} centered>
        <Space direction="vertical" style={{ width: '100%' }} align="center">
          <Spin size="large" />
          <Text>测试中，请稍候...</Text>
          <Text type="secondary">已用时: {elapsedSeconds} 秒</Text>
        </Space>
      </Modal>

      {/* 测试结果 Modal */}
      <Modal
        title="测试结果"
        width={800}
        open={testResultModalOpen}
        footer={null}
        onCancel={() => setTestResultModalOpen(false)}
      >
        <Space direction="vertical" style={{ width: '100%', marginTop: 10 }} size="middle">
          <ProDescriptions column={2} size="small">
            <ProDescriptions.Item label="状态">
              <Tag color={testResult?.success ? 'green' : 'red'}>
                {testResult?.success ? '成功' : '失败'}
              </Tag>
            </ProDescriptions.Item>
            {testResult?.duration && (
              <ProDescriptions.Item label="耗时">{testResult.duration} ms</ProDescriptions.Item>
            )}
            {testResult?.error && (
              <ProDescriptions.Item label="错误信息" ellipsis tooltip={testResult.error}>
                <Text type="danger">{testResult.error}</Text>
              </ProDescriptions.Item>
            )}
            {testResult?.metadata && (
              <>
                <ProDescriptions.Item label="Model">
                  {testResult.metadata.model}
                </ProDescriptions.Item>
                <ProDescriptions.Item label="Provider">
                  {testResult.metadata.provider}
                </ProDescriptions.Item>
              </>
            )}
          </ProDescriptions>
          {testResult?.usage && (
            <>
              <Row>
                <Col span={24}>
                  <Typography.Title level={5}>Usage</Typography.Title>
                </Col>
              </Row>
              <Row gutter={16} justify="space-evenly">
                <Col span={8}>
                  <Card variant="borderless">
                    <Statistic title="Total Tokens" value={testResult.usage.totalTokens} />
                  </Card>
                </Col>
                <Col span={8}>
                  <Card variant="borderless">
                    <Statistic title="Prompt Tokens" value={testResult.usage.promptTokens} />
                  </Card>
                </Col>
                <Col span={8}>
                  <Card variant="borderless">
                    <Statistic
                      title="Completion Tokens"
                      value={testResult.usage.completionTokens}
                    />
                  </Card>
                </Col>
              </Row>
            </>
          )}
          <ProCard
            bordered
            tabs={{
              activeKey: testResultTabActiveKey,
              items: [
                {
                  label: `Variables`,
                  key: 'variables',
                  children: <CodeEditor height="300px" value={testVariables} readonly />,
                },
                {
                  label: `Result`,
                  key: 'result',
                  children: (
                    <CodeEditor
                      language="text"
                      height="300px"
                      value={testResult?.result || testResult?.error || ''}
                      readonly
                    />
                  ),
                },
              ],
              onChange: (key) => {
                setTestResultTabActiveKey(key);
              },
            }}
          />
        </Space>
      </Modal>
    </>
  );
};

export default PromptModal;
