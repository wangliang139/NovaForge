import { CodeEditor, EllipsisMiddleTag, EllipsisMiddleText, ReadonlyWrapper } from '@/components';
import { EditIndicator } from '@/global.types';
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  FileSyncOutlined,
  PlusOutlined,
  RightOutlined,
} from '@ant-design/icons';
import {
  ModalForm,
  ProCard,
  ProForm,
  ProFormDigit,
  ProFormSwitch,
  ProFormText,
  ProFormTextArea,
  ProList,
} from '@ant-design/pro-components';
import {
  Avatar,
  Button,
  Col,
  Divider,
  Empty,
  Form,
  Input,
  InputNumber,
  List,
  message,
  Modal,
  Row,
  Select,
  Skeleton,
  Space,
  Switch,
  Tag,
  Typography,
} from 'antd';
import React, { useEffect, useState } from 'react';
import { createLlmPrompt, deleteLlmPrompt, queryLlmPrompts, updateLlmPrompt } from '../service';
import useStyles from '../style.style';
import {
  LlmMessage,
  LlmPlatformColor,
  LlmPrompt,
  LlmResponseFormatType,
  LlmScene,
  PlatformType,
} from '../types';
import JsonSchemaGenerator from './JsonSchemaGenerator';
import { getRoleAvatar } from './LlmRole';
import PromptMessageModal from './PromptMessageModal';
import PromptModal from './PromptModal';

type SceneModalProps = {
  mode: 'new' | 'edit' | 'readonly';
  open: boolean;
  value?: LlmScene;
  onOpenChange: (open: boolean) => void;
  onFinish?: (formData: LlmScene) => Promise<boolean | void> | void;
};

const defaultValues = {
  key: '',
  name: '',
  description: '',
  enabled: false,
  config: {},
  messages: [],
  timeout: 300,
  responseFormat: { type: 'text' },
};

const SceneModal: React.FC<SceneModalProps> = ({ mode, open, value, onOpenChange, onFinish }) => {
  const { styles } = useStyles();

  const [form] = Form.useForm();

  const [messages, setMessages] = useState<readonly LlmMessage[]>([]);
  const [messageEditIndicator, setMessageEditIndicator] = useState<EditIndicator<LlmMessage>>({
    value: null,
    index: -1,
    open: false,
  });

  const [prompts, setPrompts] = useState<readonly LlmPrompt[]>([]);
  const [promptsCollapsed, setPromptsCollapsed] = useState(true);
  const [promptEditIndicator, setPromptEditIndicator] = useState<EditIndicator<LlmPrompt>>({
    mode: 'new',
    open: false,
    value: null,
    index: -1,
  });

  const [jsonSchemaGeneratorOpen, setJsonSchemaGeneratorOpen] = useState(false);

  const responseFormatTypeValue = Form.useWatch(['responseFormat', 'type'], form);

  const readonly = mode === 'readonly';

  const onMessagesChange = (messages: LlmMessage[]) => {
    setMessages(messages);
    form.setFieldsValue({ messages: messages });
    form.validateFields(['messages']);
  };

  const queryPrompts = async () => {
    if (mode === 'new' || !value?.id) {
      return;
    }
    await queryLlmPrompts({ sceneId: value?.id })
      .then((res) => {
        setPrompts(res.list || []);
      })
      .catch((error) => {
        console.error(error);
        message.error('获取 Prompts 失败');
      });
  };

  useEffect(() => {
    if (!open) {
      return;
    }
    form.setFieldsValue({
      ...(value || defaultValues),
      enabled: mode === 'new' ? false : value?.enabled,
    });
    setMessages(value?.messages || []);
    setPrompts([]);
    setPromptsCollapsed(mode === 'edit');
    queryPrompts();
  }, [value, mode, open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    if (responseFormatTypeValue === LlmResponseFormatType.JSON_SCHEMA) {
      let jsonSchema = {
        name: value?.responseFormat?.jsonSchema?.name || form.getFieldValue('key') || value?.key,
        strict:
          value?.responseFormat?.jsonSchema?.strict !== undefined
            ? value?.responseFormat?.jsonSchema?.strict
            : true,
        schema: value?.responseFormat?.jsonSchema?.schema || '',
      };

      if (jsonSchema.schema) {
        jsonSchema.schema = JSON.stringify(JSON.parse(jsonSchema.schema), null, 4);
      }

      form.setFieldsValue({
        responseFormat: {
          jsonSchema: jsonSchema,
        },
      });
    } else {
      form.setFieldsValue({ responseFormat: { jsonSchema: undefined } });
    }
  }, [responseFormatTypeValue, value?.responseFormat, open]);

  return (
    <>
      <ModalForm<LlmScene>
        form={form}
        title={`${mode === 'new' ? '新建' : mode === 'edit' ? '编辑' : '查看'} LLM Scene`}
        width="800px"
        open={open}
        layout="vertical"
        onOpenChange={onOpenChange}
        readonly={readonly}
        submitter={readonly ? false : undefined}
        modalProps={{
          // centered: true,
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
        <ProFormText
          name="key"
          label="Key"
          width="md"
          disabled={mode !== 'new'}
          fieldProps={{
            count: {
              show: true,
              max: 100,
            },
          }}
          rules={[
            { required: true, message: '请输入 Key' },
            { max: 100, message: '最多 100 个字符' },
            { pattern: /^[a-zA-Z0-9_]+$/, message: '只能包含大小写字母、数字和下划线' },
          ]}
          tooltip="全局唯一，最多 100 个字符，只能包含大小写字母、数字和下划线"
        />

        <Row justify="space-between" style={{ width: '100%' }}>
          <Col span={12}>
            <ProFormText
              name="name"
              label="Name"
              width="md"
              fieldProps={{
                count: {
                  show: true,
                  max: 100,
                },
              }}
              rules={[
                { required: true, message: '请输入 Name' },
                { max: 100, message: '最多 100 个字符' },
              ]}
            />
          </Col>
          <Col span={6}>
            <ProFormDigit
              name="timeout"
              label="Timeout (秒)"
              width="xs"
              max={3600}
              rules={[{ required: true, message: '请输入 Timeout', type: 'number', min: 0 }]}
            />
          </Col>
          <Col span={6}>
            <ProFormSwitch
              name="enabled"
              label="Enabled"
              width="md"
              rules={[{ required: true }]}
              disabled={mode === 'new'}
            />
          </Col>
        </Row>

        <ProFormTextArea
          name="description"
          label="Description"
          fieldProps={{
            rows: 2,
            count: {
              show: true,
              max: 1000,
            },
          }}
          rules={[{ max: 1000, message: '最多 1000 个字符' }]}
        />

        <ProForm.Item name="messages" label="Messages" width="md">
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
                    disabled={readonly}
                    icon={<EditOutlined />}
                    onClick={() => {
                      setMessageEditIndicator({
                        mode: 'readonly',
                        open: true,
                        value: row,
                        index: index,
                      });
                    }}
                  />,
                  <Button
                    type="link"
                    key="copy"
                    disabled={readonly}
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
                    disabled={readonly}
                    icon={<DeleteOutlined />}
                    onClick={() => {
                      onMessagesChange(messages.filter((_, i) => i !== index));
                    }}
                  />,
                ],
              },
            }}
          />
          {!readonly && (
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

        <ProForm.Item name="config" layout="horizontal">
          <div style={{ marginBottom: 10 }}>Config</div>
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
                  tooltip="Controls the randomness of the model's output. Higher values (closer to 1) make output more random, while lower values make the output more deterministic."
                >
                  <ReadonlyWrapper readonly={readonly}>
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
                  rules={[
                    {
                      validator: async (_, value) => {
                        const maxCompletionTokens = form.getFieldValue([
                          'config',
                          'maxCompletionTokens',
                        ]);
                        if (value && maxCompletionTokens && value < maxCompletionTokens) {
                          return Promise.reject(
                            new Error('Max Tokens 不能小于 Max Completion Tokens'),
                          );
                        }
                      },
                    },
                  ]}
                >
                  <ReadonlyWrapper readonly={readonly}>
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
                  <ReadonlyWrapper readonly={readonly}>
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
                  rules={[
                    {
                      validator: async (_, value) => {
                        const maxTokens = form.getFieldValue(['config', 'maxTokens']);
                        if (value && maxTokens && value > maxTokens) {
                          return Promise.reject(
                            new Error('Max Completion Tokens 不能大于 Max Tokens'),
                          );
                        }
                      },
                    },
                  ]}
                >
                  <ReadonlyWrapper readonly={readonly}>
                    <InputNumber min={0} step={1} />
                  </ReadonlyWrapper>
                </Form.Item>
              </Col>
            </Row>
          </ProCard>
        </ProForm.Item>

        <ProForm.Item layout="horizontal" required>
          <div className={styles.required} style={{ marginBottom: 10 }}>
            Response Format
          </div>
          <Form.Item
            name={['responseFormat', 'type']}
            label="Type"
            layout="horizontal"
            labelAlign="right"
            labelCol={{ span: 12 }}
            wrapperCol={{ span: 12 }}
            rules={[{ required: true, message: '请选择 Type' }]}
            hidden
          >
            <Select
              options={Object.values(LlmResponseFormatType).map((type) => ({
                label: type,
                value: type,
              }))}
            />
          </Form.Item>
          <ProCard
            bordered
            tabs={{
              activeKey: responseFormatTypeValue,
              items: [
                {
                  label: `Text`,
                  key: LlmResponseFormatType.TEXT,
                  disabled: readonly && responseFormatTypeValue !== LlmResponseFormatType.TEXT,
                  children: (
                    <Empty
                      image={Empty.PRESENTED_IMAGE_SIMPLE}
                      description="No configuration required"
                    />
                  ),
                },
                {
                  label: `JSON Object`,
                  key: LlmResponseFormatType.JSON_OBJECT,
                  disabled:
                    readonly && responseFormatTypeValue !== LlmResponseFormatType.JSON_OBJECT,
                  children: (
                    <Empty
                      image={Empty.PRESENTED_IMAGE_SIMPLE}
                      description="No configuration required"
                    />
                  ),
                },
                {
                  label: `JSON Schema`,
                  key: LlmResponseFormatType.JSON_SCHEMA,
                  disabled:
                    readonly && responseFormatTypeValue !== LlmResponseFormatType.JSON_SCHEMA,
                  children: responseFormatTypeValue == LlmResponseFormatType.JSON_SCHEMA && (
                    <Row>
                      <Col span={20} className={styles.lastChildNoBottomMargin}>
                        <>
                          <Form.Item
                            name={['responseFormat', 'jsonSchema', 'name']}
                            label="Name"
                            layout="horizontal"
                            labelAlign="right"
                            labelCol={{ span: 4 }}
                            wrapperCol={{ span: 20 }}
                            tooltip="The name of the JSON Schema"
                            rules={[
                              { required: true, message: '请输入 JSON Schema Name' },
                              { max: 100, message: '最多 100 个字符' },
                              {
                                pattern: /^[a-zA-Z0-9_]+$/,
                                message: '只能包含大小写字母、数字和下划线',
                              },
                            ]}
                          >
                            <ReadonlyWrapper readonly={readonly}>
                              <Input
                                style={{ width: '50%' }}
                                count={{
                                  show: true,
                                  max: 100,
                                }}
                                placeholder="请输入 JSON Schema Name"
                              />
                            </ReadonlyWrapper>
                          </Form.Item>
                          <Form.Item
                            name={['responseFormat', 'jsonSchema', 'strict']}
                            label="Strict"
                            layout="horizontal"
                            labelAlign="right"
                            labelCol={{ span: 4 }}
                            wrapperCol={{ span: 20 }}
                            rules={[{ required: true, message: '请选择 Strict' }]}
                          >
                            <ReadonlyWrapper readonly={readonly}>
                              <Switch />
                            </ReadonlyWrapper>
                          </Form.Item>
                          <Form.Item
                            label="Schema"
                            layout="horizontal"
                            labelAlign="right"
                            labelCol={{ span: 4 }}
                            wrapperCol={{ span: 16 }}
                            required
                          >
                            <Form.Item
                              name={['responseFormat', 'jsonSchema', 'schema']}
                              noStyle
                              rules={[
                                {
                                  validator: (_rule, value) => {
                                    try {
                                      if (!value) {
                                        return Promise.reject(
                                          new Error('请输入合法的 JSON Schema'),
                                        );
                                      }
                                      if (value.length > 5000) {
                                        return Promise.reject(
                                          new Error('JSON Schema 不能超过 50000 个字符'),
                                        );
                                      }
                                      const json = JSON.parse(value);
                                      if (typeof json !== 'object' || json === null) {
                                        return Promise.reject(
                                          new Error('请输入合法的 JSON Schema'),
                                        );
                                      }
                                      if (Object.keys(json).length === 0) {
                                        return Promise.reject(
                                          new Error('请输入合法的 JSON Schema'),
                                        );
                                      }
                                      if (Array.isArray(json)) {
                                        return Promise.reject(
                                          new Error('请输入合法的 JSON Schema'),
                                        );
                                      }
                                      return Promise.resolve();
                                    } catch (error) {
                                      return Promise.reject(new Error('请输入合法的 JSON Schema'));
                                    }
                                  },
                                },
                              ]}
                            >
                              <CodeEditor
                                height="200px"
                                readonly
                                style={{
                                  marginBottom: 8,
                                }}
                              />
                            </Form.Item>
                            <Button
                              size="small"
                              icon={<FileSyncOutlined />}
                              disabled={readonly}
                              onClick={() => {
                                setJsonSchemaGeneratorOpen(true);
                              }}
                            >
                              反向生成
                            </Button>
                          </Form.Item>
                        </>
                      </Col>
                    </Row>
                  ),
                },
              ],
              onChange: (key) => {
                form.setFieldsValue({ responseFormat: { type: key } });
              },
            }}
          ></ProCard>
        </ProForm.Item>

        {mode !== 'new' && (
          <ProCard
            title="Prompts"
            headerBordered
            bordered
            collapsible
            collapsed={promptsCollapsed}
            extra={
              <RightOutlined
                rotate={!promptsCollapsed ? 90 : undefined}
                onClick={() => {
                  setPromptsCollapsed(!promptsCollapsed);
                }}
              />
            }
          >
            <List<LlmPrompt>
              rowKey={(row) => row.id || ''}
              dataSource={prompts as unknown as LlmPrompt[]}
              renderItem={(row, index) => (
                <List.Item
                  actions={[
                    <Button
                      type="link"
                      key="edit"
                      icon={<EditOutlined />}
                      onClick={() => {
                        setPromptEditIndicator({
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
                      icon={<CopyOutlined />}
                      onClick={() => {
                        setPromptEditIndicator({
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
                      danger
                      icon={<DeleteOutlined />}
                      onClick={async () => {
                        if (!row.id) {
                          message.error('ID 为空，无法删除');
                          return;
                        }
                        const confirmed = await new Promise<boolean>((resolve) => {
                          Modal.confirm({
                            title: '确认删除',
                            content: `确定要删除 Prompt "${row.name}" 吗？`,
                            onOk: () => resolve(true),
                            onCancel: () => resolve(false),
                          });
                        });
                        if (!confirmed) {
                          return;
                        }
                        const hide = message.loading('正在删除');
                        try {
                          const res = await deleteLlmPrompt(row.id);
                          hide();
                          if (!res.errors) {
                            message.success('删除成功');
                            queryPrompts();
                          } else {
                            message.error('删除失败');
                          }
                        } catch (error) {
                          hide();
                          message.error('删除失败');
                        }
                      }}
                    />,
                  ]}
                >
                  <Skeleton avatar title={false} loading={false} active>
                    <List.Item.Meta
                      avatar={
                        <Avatar
                          style={{
                            backgroundColor: LlmPlatformColor[row.platform as PlatformType],
                            color: '#000000A6',
                            fontWeight: 'bold',
                          }}
                          alt={row.platform || ''}
                        >
                          {row.platform || ''}
                        </Avatar>
                      }
                      title={
                        <span
                          onClick={() => {
                            setPromptEditIndicator({
                              mode: 'readonly',
                              open: true,
                              value: row,
                              index: -1,
                            });
                          }}
                        >
                          <EllipsisMiddleText
                            suffixCount={10}
                            children={row.name || ''}
                            className={styles.clickable}
                          />
                        </span>
                      }
                      description={
                        <>
                          <EllipsisMiddleTag suffixCount={15} children={row.model || ''} />
                          <Row style={{ marginTop: 5 }}>
                            <Space size={0}>
                              {row.variants &&
                                row.variants.map((variant: string, index: number) => (
                                  <Tag key={index} color="#e89a3c">
                                    {variant}
                                  </Tag>
                                ))}
                            </Space>
                          </Row>
                        </>
                      }
                    />
                    <Space size={20}>
                      <span>
                        权重：<span>{row.weight}</span>
                      </span>
                      <span>
                        状态：
                        <Switch
                          checked={row.enabled}
                          onChange={async (checked) => {
                            const hide = message.loading('正在更新状态');
                            try {
                              // Only update enabled field
                              const res = await updateLlmPrompt({
                                id: row.id,
                                enabled: checked,
                              });
                              hide();
                              if (!res.errors) {
                                message.success('状态更新成功');
                                queryPrompts();
                              } else {
                                message.error('状态更新失败');
                              }
                            } catch (error) {
                              hide();
                              message.error('状态更新失败');
                            }
                          }}
                        />
                      </span>
                    </Space>
                  </Skeleton>
                </List.Item>
              )}
            />
            <>
              <Divider size="small" />
              <Button
                block
                type="dashed"
                icon={<PlusOutlined />}
                onClick={() => {
                  setPromptEditIndicator({
                    mode: 'new',
                    open: true,
                    value: {
                      sceneId: value?.id,
                      config: value?.config,
                      messages: value?.messages,
                      timeout: value?.timeout,
                    },
                    index: -1,
                  });
                }}
              >
                添加一行
              </Button>
            </>
          </ProCard>
        )}
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
          return true;
        }}
      />

      <PromptModal
        mode={promptEditIndicator.mode || 'new'}
        open={promptEditIndicator.open}
        value={promptEditIndicator.value || undefined}
        onOpenChange={(open) => {
          setPromptEditIndicator((prev) => ({
            ...prev,
            open: open,
          }));
        }}
        onFinish={async (values: LlmPrompt) => {
          if (promptEditIndicator.mode === 'readonly') {
            return true;
          }
          const hide = message.loading('正在保存');
          try {
            let res = null;
            if (promptEditIndicator.mode === 'new') {
              values.sceneId = value?.id;
              res = await createLlmPrompt(values);
            } else {
              values.id = promptEditIndicator.value?.id || '';
              res = await updateLlmPrompt(values);
            }
            message.success('保存成功');
            queryPrompts();
            return true;
          } catch (error: any) {
            console.error(error);
            message.error('保存失败: ' + error.message);
            return false;
          } finally {
            hide();
          }
        }}
      />

      <JsonSchemaGenerator
        open={jsonSchemaGeneratorOpen}
        onOpenChange={setJsonSchemaGeneratorOpen}
        onFinish={async (jsonSchema) => {
          form.setFieldsValue({
            responseFormat: {
              jsonSchema: {
                schema: jsonSchema,
              },
            },
          });
          setJsonSchemaGeneratorOpen(false);
          form.validateFields([['responseFormat', 'jsonSchema', 'schema']]);
          return true;
        }}
      />
    </>
  );
};

export default SceneModal;
