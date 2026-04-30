import { CodeEditor } from '@/components';
import { EditIndicator } from '@/global.types';
import {
  SignalDefinition,
  Strategy,
  StrategyParam,
} from '@/services/gateway/strategy';
import utils from '@/utils';
import {
  CompressOutlined,
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
  ExpandOutlined,
  LeftOutlined,
  PlusOutlined,
  RightOutlined,
} from '@ant-design/icons';
import type { ProColumns } from '@ant-design/pro-components';
import {
  ProForm,
  ProFormText,
  ProList,
  ProTable
} from '@ant-design/pro-components';
import {
  Button,
  Card,
  Col,
  Collapse,
  Divider,
  Flex,
  Form,
  Input,
  message,
  Modal,
  Row,
  Segmented,
  Space,
  Tag,
  theme,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkGfm from 'remark-gfm';
import ParamModal from './ParamModal';
import SignalModal from './SignalModal';
import StrategyChatPanel, { StrategyPatch } from './StrategyChatPanel';

type StrategyFormProps = {
  mode?: 'new' | 'edit' | 'readonly';
  value?: Strategy;
  hideSubmitter?: boolean;
  onFinish?: (formData: {
    name?: string;
    description?: string;
    code?: string;
    params?: StrategyParam[];
    signals?: SignalDefinition[];
  }) => Promise<boolean | void> | void;
  onStrategyChange?: (strategy: Partial<Strategy>) => void;
};

export type StrategyFormRef = {
  submit: () => void;
};

const DefaultCode = `function onInit() {
  console.log('策略执行中...')
}

function onSignal(signal) {
  var payload = JSON.stringify(signal.payload);
  console.log('onSignal', signal.type, payload);
}`;

const StrategyForm = forwardRef<StrategyFormRef, StrategyFormProps>((props, ref) => {
  const { mode, value, onFinish, onStrategyChange, hideSubmitter = false } = props;
  const { token } = theme.useToken();
  const [form] = Form.useForm();
  const [code, setCode] = useState(DefaultCode);
  const [codeFullscreenOpen, setCodeFullscreenOpen] = useState(false);
  const [chatPanelVisible, setChatPanelVisible] = useState(true);
  const [params, setParams] = useState<readonly StrategyParam[]>([]);
  const [signals, setSignals] = useState<readonly SignalDefinition[]>([]);
  const [editTab, setEditTab] = useState<string>('code');

  const [paramEditIndicator, setParamEditIndicator] = useState<EditIndicator<StrategyParam>>({
    value: null,
    index: -1,
    open: false,
  });

  const [signalEditIndicator, setSignalEditIndicator] = useState<EditIndicator<SignalDefinition>>({
    value: null,
    index: -1,
    open: false,
  });

  const readonly = mode === 'readonly';
  const isEditMode = mode === 'edit' || mode === 'new';
  const isSyncingFromValueRef = useRef(false);

  // 构建策略对象并通知父组件
  const notifyStrategyChange = () => {
    // 避免在从外部 value 同步到表单时形成更新闭环
    if (isSyncingFromValueRef.current) {
      return;
    }
    if (onStrategyChange) {
      const formData = form.getFieldsValue();
      const strategy: Partial<Strategy> = {
        id: value?.id || '',
        name: formData.name || '',
        description: formData.description || '',
        code: code,
        version: value?.version || '1',
        status: value?.status,
        params: params as StrategyParam[],
        signals: signals as SignalDefinition[],
        createdAt: value?.createdAt || 0,
        updatedAt: value?.updatedAt || 0,
      };
      onStrategyChange(strategy);
    }
  };

  const handleCodeChange = (value: string) => {
    setCode(value);
    form.setFieldsValue({ code: value });
    // 代码变化时通知策略变化
    setTimeout(() => notifyStrategyChange(), 0);
  };

  const getCurrentStrategySnapshot = (): Partial<Strategy> => {
    const formData = form.getFieldsValue();
    return {
      id: value?.id || '',
      name: formData.name || '',
      description: formData.description || '',
      code,
      version: value?.version || '1',
      status: value?.status,
      params: params as StrategyParam[],
      signals: signals as SignalDefinition[],
      createdAt: value?.createdAt || 0,
      updatedAt: value?.updatedAt || 0,
    };
  };

  const handleApplyStrategyPatch = (patch: StrategyPatch) => {
    setCode(patch.code);
    setParams(patch.params);
    setSignals(patch.signals);
    form.setFieldsValue({
      name: patch.name,
      description: patch.description,
      code: patch.code,
      params: patch.params,
      signals: patch.signals,
    });
    onStrategyChange?.({
      ...getCurrentStrategySnapshot(),
      name: patch.name,
      description: patch.description,
      code: patch.code,
      params: patch.params,
      signals: patch.signals,
    });
  };

  const onParamsChange = (newParams: StrategyParam[]) => {
    setParams(newParams);
    form.setFieldsValue({ params: newParams });
    form.validateFields(['params']);
    // 通知策略变化
    setTimeout(() => notifyStrategyChange(), 0);
  };

  const onSignalsChange = (newSignals: SignalDefinition[]) => {
    // 根据排序自动生成ID（从1开始）
    const signalsWithAutoId = newSignals.map((signal, index) => ({
      ...signal,
      id: (index + 1).toString(),
    }));
    setSignals(signalsWithAutoId);
    form.setFieldsValue({ signals: signalsWithAutoId });
    form.validateFields(['signals']);
    // 通知策略变化
    setTimeout(() => notifyStrategyChange(), 0);
  };

  useEffect(() => {
    if (!value) {
      return;
    }
    // 仅在策略 ID 或更新时间变化时，从外部 value 同步到表单，
    // 避免在编辑同一个策略时每次父组件 setState 都覆盖正在输入的内容。
    isSyncingFromValueRef.current = true;
    const paramsValue = value.params || [];
    const signalsValue = value.signals || [];
    signalsValue.sort((a, b) => {
      const aIndex = parseInt(a.id);
      const bIndex = parseInt(b.id);
      return aIndex - bIndex;
    });
    setCode(value.code || DefaultCode);
    setParams(paramsValue);
    setSignals(signalsValue);
    form.setFieldsValue({
      name: value.name,
      description: value.description,
      code: value.code || DefaultCode,
      params: paramsValue,
      signals: signalsValue,
      id: value.id,
      version: value.version,
      createdAt: value.createdAt ? dayjs.unix(value.createdAt).format('YYYY-MM-DD HH:mm:ss') : '',
      updatedAt: value.updatedAt ? dayjs.unix(value.updatedAt).format('YYYY-MM-DD HH:mm:ss') : '',
    });
    // 本次仅负责把外部 value 同步到表单，不向上通知变化，避免死循环
    setTimeout(() => {
      isSyncingFromValueRef.current = false;
    }, 0);
    // 说明：
    // - 依赖 value.id：切换到另一条策略时需要完整同步
    // - 依赖 value.updatedAt：保存后后端返回的新数据需要覆盖到表单（版本号等）
    // - 不依赖整个 value 对象，避免父组件在 onStrategyChange 中 setState 导致正在编辑时被立即重置
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value?.id, value?.updatedAt]);

  // 使用 Form.useWatch 监听 name 和 description 变化
  const nameValue = Form.useWatch('name', form);
  const descriptionValue = Form.useWatch('description', form);

  useEffect(() => {
    if (nameValue !== undefined || descriptionValue !== undefined) {
      notifyStrategyChange();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nameValue, descriptionValue]);

  useImperativeHandle(ref, () => ({
    submit: () => {
      form.submit();
    },
  }));

  const handleFinish = async (formData: any) => {
    if (onFinish) {
      const data = {
        ...formData,
        code,
        params: params as StrategyParam[],
        signals: signals as SignalDefinition[],
      };
      const success = await onFinish(data);
      if (success === false) {
        return false;
      }
    }
    // 新建模式下，保存成功后重置表单和本地状态；编辑/只读模式不自动重置
    if (mode === 'new') {
      setCode(DefaultCode);
      setParams([]);
      setSignals([]);
      form.resetFields();
    }
    return true;
  };

  return (
    <>
      <ProForm form={form} labelAlign="right" onFinish={handleFinish} submitter={false}>
        <Row>
          <Col span={20}>
            <ProForm.Item
              name="name"
              rules={[
                {
                  required: true,
                  message: '名称是必填项',
                },
              ]}
            >
              <Input
                addonBefore={<Typography.Text type="secondary">策略名称</Typography.Text>}
                disabled={readonly}
                size="middle"
                width="100%"
              />
            </ProForm.Item>
          </Col>
          <Col span={4}>
            <Flex justify="flex-end">
              <Space>
                <Button
                  type="primary"
                  onClick={() => form.submit()}
                  style={{ marginRight: 8 }}
                  disabled={readonly}
                >保存</Button>
              </Space>
            </Flex>
          </Col>
        </Row>

        {mode !== 'new' && <ProForm.Group >
          <ProFormText name="id" label="ID" width="lg" readonly />
          <ProFormText name="version" label="版本号" width="sm" readonly />
          <ProFormText name="createdAt" label="创建时间" width="lg" readonly />
          <ProFormText name="updatedAt" label="更新时间" width="lg" readonly />
        </ProForm.Group>}

        <Card
          style={{ marginBottom: 24 }}
          styles={{ body: { padding: 2 } }}
          title={
            <Row justify="space-between">
              <Segmented<string>
                options={[
                  { label: '代码', value: 'code' },
                  { label: '描述', value: 'description' },
                ]}
                onChange={(value) => {
                  setEditTab(value);
                }}
              />
              <div hidden={editTab !== 'code'}>
                <Button
                  type="default"
                  icon={<ExpandOutlined />}
                  disabled={readonly}
                  onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    setCodeFullscreenOpen(true);
                  }}
                >
                  全屏
                </Button>
              </div>
            </Row>
          }
        >
          {editTab === 'description' && (
            <Row>
              <Col span={12}>
                <ProForm.Item name="description" noStyle>
                  <Input.TextArea
                    size="large"
                    style={{ height: 400 }}
                    disabled={readonly}
                  />
                </ProForm.Item>
              </Col>
              <Col span={12}>
                <Typography.Paragraph
                  type="secondary"
                  style={{
                    border: '1px solid #d9d9d9',
                    borderRadius: 6,
                    padding: '8px 12px',
                    height: 400,
                    marginBottom: 0,
                    cursor: 'pointer',
                    overflow: 'auto',
                  }}
                  onClick={() => { }}
                >
                  <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
                    {descriptionValue || ''}
                  </ReactMarkdown>
                </Typography.Paragraph>
              </Col>
            </Row>
          )}
          {editTab === 'code' && (
            <div style={{ display: 'flex', height: 400 }}>
              {/* Code editor */}
              <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                <CodeEditor
                  language="javascript"
                  height="400px"
                  value={code}
                  readonly={readonly}
                  onChange={handleCodeChange}
                />
              </div>

              {/* Toggle button */}
              {!readonly && (
                <div
                  style={{
                    width: 20,
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    background: token.colorFillQuaternary,
                    borderLeft: `1px solid ${token.colorBorderSecondary}`,
                    borderRight: chatPanelVisible ? `1px solid ${token.colorBorderSecondary}` : undefined,
                    cursor: 'pointer',
                    userSelect: 'none',
                    flexShrink: 0,
                  }}
                  onClick={() => setChatPanelVisible((v) => !v)}
                >
                  {chatPanelVisible ? (
                    <RightOutlined style={{ fontSize: 10, color: token.colorTextTertiary }} />
                  ) : (
                    <LeftOutlined style={{ fontSize: 10, color: token.colorTextTertiary }} />
                  )}
                </div>
              )}

              {/* Chat panel */}
              {!readonly && chatPanelVisible && (
                <div style={{ flex: 1, minWidth: 0 }}>
                  <StrategyChatPanel
                    getCurrentStrategy={getCurrentStrategySnapshot}
                    onApplyStrategy={handleApplyStrategyPatch}
                  />
                </div>
              )}
            </div>
          )}
        </Card>

        <ProForm.Item width="md">
          <Collapse
            defaultActiveKey={['params']}
            style={{ marginBottom: 0 }}
            items={[
              {
                key: 'params',
                label: '参数配置',
                children: (
                  <ProForm.Item name="params" noStyle>
                    <ProTable<StrategyParam>
                      rowKey={(_, index) => index?.toString() || ''}
                      dataSource={params as unknown as StrategyParam[]}
                      search={false}
                      options={false}
                      pagination={false}
                      toolBarRender={false}
                      size="small"
                      columns={
                        [
                          {
                            title: '名称',
                            dataIndex: 'name',
                            render: (_, row) => (
                              <Flex justify="space-between">
                                <Space size={8}>
                                  <span>{row.name}</span>
                                  {row.description && (
                                    <Tooltip title={row.description}>
                                      <ExclamationCircleOutlined style={{ color: '#1890ff' }} />
                                    </Tooltip>
                                  )}
                                </Space>
                                <Space size={6} wrap>
                                  {row.required && <Tag color="red">必填</Tag>}
                                  {row.type && <Tag>{row.type}</Tag>}
                                </Space>
                              </Flex>
                            ),
                          },
                          {
                            title: '默认值',
                            dataIndex: 'default',
                            width: 200,
                            render: (_, row) => (
                              <Typography.Paragraph
                                type="secondary"
                                ellipsis={{ rows: 2 }}
                                copyable={!!row.default && row.default.length > 0}
                                style={{ marginBottom: 0 }}
                              >
                                <span>{row.default || '-'}</span>
                              </Typography.Paragraph>
                            ),
                          },
                          {
                            title: '操作',
                            valueType: 'option',
                            width: 80,
                            render: (_, row, index) => [
                              <Button
                                type="link"
                                key="edit"
                                icon={<EditOutlined />}
                                disabled={readonly}
                                onClick={() => {
                                  setParamEditIndicator({
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
                                disabled={readonly}
                                onClick={() => {
                                  setParamEditIndicator({
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
                                disabled={readonly}
                                onClick={() => {
                                  onParamsChange(params.filter((_, i) => i !== index));
                                }}
                              />,
                            ],
                          },
                        ] as ProColumns<StrategyParam>[]
                      }
                    />

                    <>
                      <Divider size="small" />
                      <Button
                        block
                        type="dashed"
                        icon={<PlusOutlined />}
                        disabled={readonly}
                        onClick={() => {
                          setParamEditIndicator({
                            mode: 'new',
                            open: true,
                            value: null,
                            index: -1,
                          });
                        }}
                      >
                        添加参数
                      </Button>
                    </>
                  </ProForm.Item>
                ),
              },
            ]}
          />
        </ProForm.Item>
        <ProForm.Item>
          <Form.Item
            name="signals"
            rules={[{ required: true, message: '信号定义是必填项' }]}
            noStyle
          />
          <Collapse
            defaultActiveKey={['signals']}
            style={{ marginBottom: 0 }}
            items={[
              {
                key: 'signals',
                label: '信号定义',
                forceRender: true,
                children: (
                  <>
                    <ProList<SignalDefinition>
                      rowKey={(_, index) => index?.toString() || ''}
                      dataSource={signals as unknown as SignalDefinition[]}
                      showActions="hover"
                      metas={{
                        title: {
                          dataIndex: 'id',
                          render: (_, row) => (
                            <Space>
                              <span>{row.id}</span>
                              {row.type && <Tag color="blue">{row.type}</Tag>}
                              {row.scope && <Tag color="purple">{row.scope}</Tag>}
                            </Space>
                          ),
                        },
                        description: {
                          render: (_, row) => (
                            <>
                              {row.symbol && (
                                <span>
                                  {row.exchange && (
                                    <img
                                      alt={row.exchange}
                                      width={24}
                                      src={utils.market.getExchangeLogo(row.exchange)}
                                      style={{
                                        display: 'inline',
                                        width: 18,
                                        height: 18,
                                        verticalAlign: 'middle',
                                        marginRight: 4,
                                        marginLeft: 4,
                                      }}
                                    />
                                  )}
                                  {row.symbol}
                                </span>
                              )}
                            </>
                          ),
                        },
                        content: {
                          dataIndex: 'props',
                        },
                        actions: {
                          render: (text, row, index) => [
                            <Button
                              type="link"
                              key="edit"
                              icon={<EditOutlined />}
                              disabled={readonly}
                              onClick={() => {
                                setSignalEditIndicator({
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
                              disabled={readonly}
                              onClick={() => {
                                // 复制时计算下一个ID（当前信号数量+1）
                                const nextId = (signals.length + 1).toString();
                                setSignalEditIndicator({
                                  mode: 'new',
                                  open: true,
                                  value: {
                                    ...row,
                                    id: nextId,
                                  },
                                  index: -1,
                                });
                              }}
                            />,
                            <Button
                              type="link"
                              key="delete"
                              danger
                              icon={<DeleteOutlined />}
                              disabled={readonly}
                              onClick={() => {
                                onSignalsChange(signals.filter((_, i) => i !== index));
                              }}
                            />,
                          ],
                        },
                      }}
                    />
                    <>
                      <Divider size="small" />
                      <Button
                        block
                        type="dashed"
                        icon={<PlusOutlined />}
                        disabled={readonly}
                        onClick={() => {
                          // 计算下一个ID（当前信号数量+1）
                          const nextId = (signals.length + 1).toString();
                          setSignalEditIndicator({
                            mode: 'new',
                            open: true,
                            value: {
                              id: nextId,
                            } as SignalDefinition,
                            index: -1,
                          });
                        }}
                      >
                        添加信号
                      </Button>
                    </>
                  </>
                ),
              },
            ]}
          />
        </ProForm.Item>
      </ProForm>

      <Modal
        open={codeFullscreenOpen}
        onCancel={() => setCodeFullscreenOpen(false)}
        closable={false}
        footer={null}
        width="100%"
        style={{ top: 0, paddingBottom: 0 }}
        styles={{ body: { padding: 0 } }}
        destroyOnHidden={true}
      >
        <div style={{ height: 'calc(100vh - 55px)', display: 'flex', flexDirection: 'column' }}>
          <div
            style={{
              padding: '8px 12px',
              borderBottom: '1px solid #f0f0f0',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              flex: '0 0 auto',
            }}
          >
            <Typography.Title level={5}>代码编辑器</Typography.Title>
            <Button
              icon={<CompressOutlined />}
              onClick={() => setCodeFullscreenOpen(false)}
              type="default"
              style={{ marginBottom: 8 }}
            >
              退出全屏
            </Button>
          </div>
          <div style={{ flex: '1 1 auto' }}>
            <CodeEditor
              language="javascript"
              height="calc(100vh - 120px)"
              value={code}
              onChange={handleCodeChange}
            />
          </div>
        </div>
      </Modal>

      <ParamModal
        open={paramEditIndicator.open}
        value={paramEditIndicator.value || undefined}
        readonly={paramEditIndicator.mode === 'readonly'}
        onOpenChange={(open) => {
          setParamEditIndicator((prev) => ({
            ...prev,
            open: open,
          }));
        }}
        onFinish={async (values) => {
          if (paramEditIndicator.mode === 'readonly') {
            return true;
          }

          // 检查参数名是否重复
          const isDuplicate = params.some(
            (param, index) =>
              param.name === values.name &&
              (paramEditIndicator.index === -1 || index !== paramEditIndicator.index),
          );

          if (isDuplicate) {
            message.error(`参数名 "${values.name}" 已存在，请使用其他名称`);
            return false;
          }

          if (paramEditIndicator.index === -1) {
            onParamsChange([...params, values]);
          } else {
            onParamsChange(
              params.map((param, index) =>
                index === paramEditIndicator.index ? { ...param, ...values } : param,
              ),
            );
          }
          return true;
        }}
      />

      <SignalModal
        open={signalEditIndicator.open}
        value={signalEditIndicator.value || undefined}
        readonly={signalEditIndicator.mode === 'readonly'}
        onOpenChange={(open) => {
          setSignalEditIndicator((prev) => ({
            ...prev,
            open: open,
          }));
        }}
        onFinish={async (values) => {
          if (signalEditIndicator.mode === 'readonly') {
            return true;
          }
          const signalData: SignalDefinition = {
            id: values.id,
            type: values.type,
            exchange: values.exchange,
            symbol: values.symbol,
            scope: values.scope,
            props: values.props,
          };
          if (signalEditIndicator.index === -1) {
            onSignalsChange([...signals, signalData]);
          } else {
            onSignalsChange(
              signals.map((signal, index) =>
                index === signalEditIndicator.index ? signalData : signal,
              ),
            );
          }
          return true;
        }}
      />
    </>
  );
});

StrategyForm.displayName = 'StrategyForm';

export default StrategyForm;
