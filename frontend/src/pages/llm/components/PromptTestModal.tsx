import { CodeEditor } from '@/components';
import { ExperimentOutlined } from '@ant-design/icons';
import { ProCard, ProDescriptions, ProList } from '@ant-design/pro-components';
import {
  Button,
  Card,
  Col,
  Descriptions,
  Empty,
  Input,
  List,
  Modal,
  Radio,
  Row,
  Space,
  Spin,
  Statistic,
  Tag,
  Typography,
  message,
} from 'antd';
import dayjs from 'dayjs';
import React, { useEffect, useRef, useState } from 'react';
import { getLlmScene, testPrompt } from '../service';
import {
  CompletionMetadata,
  CompletionUsage,
  LlmMessage,
  LlmPlatformColor,
  LlmPrompt,
  LlmScene,
  PlatformType,
} from '../types';
import { getRoleAvatar } from './LlmRole';

const { TextArea } = Input;
const { Text } = Typography;

type TestResult = {
  id: string;
  promptId: string;
  promptName: string;
  variables: string;
  status: 'loading' | 'success' | 'error';
  result?: string;
  error?: string;
  duration?: number;
  usage?: CompletionUsage;
  metadata?: CompletionMetadata;
  createdAt: number;
};

type PromptTestModalProps = {
  open: boolean;
  scene?: LlmScene;
  onOpenChange: (open: boolean) => void;
};

const PromptTestModal: React.FC<PromptTestModalProps> = ({ open, scene, onOpenChange }) => {
  const [loadingPrompts, setLoadingPrompts] = useState(false);
  const [prompts, setPrompts] = useState<LlmPrompt[]>([]);

  const [selectedPrompt, setSelectedPrompt] = useState<LlmPrompt | null>(null);
  const [variables, setVariables] = useState<string>('{}');
  const [variablesStatus, setVariablesStatus] = useState<'error' | 'warning' | undefined>();
  const [testResults, setTestResults] = useState<TestResult[]>([]);
  const [testing, setTesting] = useState(false);

  const [loadingModalOpen, setLoadingModalOpen] = useState(false);
  const [loadingStartTime, setLoadingStartTime] = useState<number>(0);

  const [resultModalOpen, setResultModalOpen] = useState(false);
  const [result, setResult] = useState<TestResult | null>(null);
  const [resultTabActiveKey, setResultTabActiveKey] = useState<string>('variables');

  const [elapsedSeconds, setElapsedSeconds] = useState<number>(0);
  const timerRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    if (open && scene?.id) {
      loadPrompts();
    } else {
      setPrompts([]);
      setSelectedPrompt(null);
      setVariables('{}');
      setTestResults([]);
    }
  }, [open, scene?.id]);

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

  const loadPrompts = async () => {
    if (!scene?.id) return;
    setLoadingPrompts(true);
    try {
      const resp = await getLlmScene(scene.id, true);
      if (!resp.errors && resp.prompts) {
        setPrompts(resp.prompts);
        if (resp.prompts.length > 0 && !selectedPrompt?.id) {
          setSelectedPrompt(resp.prompts[0]);
        }
      }
    } catch (error) {
      message.error('加载 Prompts 失败');
    } finally {
      setLoadingPrompts(false);
    }
  };

  const handlePromptSelect = (promptId: string) => {
    const prompt = prompts.find((p) => p.id === promptId);
    setSelectedPrompt(prompt || null);
  };

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

  const handleTest = async () => {
    if (!selectedPrompt?.id) {
      message.warning('请先选择一个 Prompt');
      return;
    }

    if (!validateJson(variables)) {
      message.error('请输入有效的 JSON 对象');
      return;
    }

    const testId = `test_${Date.now()}`;
    const testResult: TestResult = {
      id: testId,
      promptId: selectedPrompt?.id || '',
      promptName: selectedPrompt?.name || '',
      variables,
      status: 'loading',
      createdAt: Date.now(),
    };

    setTestResults((prev) => [...prev, testResult]);
    setTesting(true);
    const startTime = Date.now();
    setLoadingModalOpen(true);
    setLoadingStartTime(startTime);

    try {
      const variablesJsonStr = JSON.stringify(JSON.parse(variables));

      const res = await testPrompt({
        sceneId: scene?.id || '',
        byPromptId: selectedPrompt?.id || '',
        variables: variablesJsonStr,
      });

      setLoadingModalOpen(false);
      setLoadingStartTime(0);

      if (!res.errors && res.data.Result) {
        const result = res.data.Result;
        const updatedResult: TestResult = {
          ...testResult,
          status: 'success',
          result: result.result || '',
          error: result.error,
          duration: result.duration,
          usage: result.usage,
          metadata: result.metadata,
        };
        setTestResults((prev) => prev.map((r) => (r.id === testId ? updatedResult : r)));
        message.success('测试完成');
      } else {
        const updatedResult: TestResult = {
          ...testResult,
          status: 'error',
          error: res.errors?.[0]?.message || '测试失败',
        };
        setTestResults((prev) => prev.map((r) => (r.id === testId ? updatedResult : r)));
        message.error('测试失败');
      }
    } catch (error: any) {
      const elapsedTime = Date.now() - startTime;
      setLoadingModalOpen(false);
      setLoadingStartTime(0);
      const updatedResult: TestResult = {
        ...testResult,
        status: 'error',
        error: error.message || '测试失败',
        duration: elapsedTime,
      };
      setTestResults((prev) => prev.map((r) => (r.id === testId ? updatedResult : r)));
      message.error('测试失败: ' + error.message);
    } finally {
      setTesting(false);
    }
  };

  return (
    <>
      <Modal
        title="测试 Prompt"
        open={open}
        onCancel={() => onOpenChange(false)}
        width={1000}
        footer={null}
      >
        <Space direction="vertical" style={{ width: '100%' }} size="small">
          {/* Prompt List */}
          <ProCard title="Prompt 列表" size="small">
            <ProList
              loading={loadingPrompts}
              style={{ maxHeight: '240px', overflowY: 'auto' }}
              dataSource={prompts}
              renderItem={(prompt) => (
                <List.Item>
                  <Radio
                    checked={selectedPrompt?.id === prompt.id}
                    onChange={() => prompt.id && handlePromptSelect(prompt.id)}
                    style={{ width: '100%' }}
                  >
                    <Space>
                      <Tag color={LlmPlatformColor[prompt.platform as PlatformType] || '#default'}>
                        {prompt.platform}
                      </Tag>
                      <Text strong>{prompt.name}</Text>
                      <Text type="secondary">{prompt.model}</Text>
                    </Space>
                  </Radio>
                </List.Item>
              )}
            />
          </ProCard>

          {/* Prompt Info */}
          <ProCard title="Prompt 信息" size="small" hidden={!selectedPrompt}>
            <Descriptions column={3} size="small">
              <Descriptions.Item label="ID">{selectedPrompt?.id}</Descriptions.Item>
              <Descriptions.Item label="Name">{selectedPrompt?.name}</Descriptions.Item>
              <Descriptions.Item label="Platform">{selectedPrompt?.platform}</Descriptions.Item>
              <Descriptions.Item label="Model">{selectedPrompt?.model}</Descriptions.Item>
              <Descriptions.Item label="Timeout">{selectedPrompt?.timeout} 秒</Descriptions.Item>
              <Descriptions.Item label="Weight">{selectedPrompt?.weight}</Descriptions.Item>
              {selectedPrompt?.providers && selectedPrompt?.providers.length > 0 && (
                <Descriptions.Item label="Providers">
                  {selectedPrompt?.providers.map((p, i) => (
                    <Tag key={i}>{p}</Tag>
                  ))}
                </Descriptions.Item>
              )}
            </Descriptions>
            <div style={{ marginTop: 16 }}>
              <Row style={{ marginBottom: 6 }}>
                <Text strong>Config:</Text>
              </Row>
              <Descriptions column={2} size="small">
                {selectedPrompt?.config && (
                  <>
                    <Descriptions.Item label="Temperature">
                      {selectedPrompt?.config?.temperature ?? '-'}
                    </Descriptions.Item>
                    <Descriptions.Item label="Top P">
                      {selectedPrompt?.config?.topP ?? '-'}
                    </Descriptions.Item>
                    <Descriptions.Item label="Max Tokens">
                      {selectedPrompt?.config?.maxTokens ?? '-'}
                    </Descriptions.Item>
                    <Descriptions.Item label="Max Completion Tokens">
                      {selectedPrompt?.config?.maxCompletionTokens ?? '-'}
                    </Descriptions.Item>
                  </>
                )}
              </Descriptions>
            </div>
            <div style={{ marginTop: 16 }}>
              <Row style={{ marginBottom: 6 }}>
                <Text strong>Messages:</Text>
              </Row>
              <ProList<LlmMessage>
                rowKey={(_, index) => index?.toString() || ''}
                dataSource={selectedPrompt?.messages || []}
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
                }}
              />
            </div>
          </ProCard>

          {/* Test Results */}
          <ProCard title="测试结果" size="small">
            {testResults.length === 0 ? (
              <Empty description="暂无测试结果" />
            ) : (
              <List
                style={{ maxHeight: '400px', overflowY: 'auto' }}
                dataSource={testResults}
                renderItem={(result) => (
                  <List.Item
                    actions={[
                      <Button
                        type="link"
                        onClick={() => {
                          setResult(result);
                          setResultModalOpen(true);
                        }}
                      >
                        查看详情
                      </Button>,
                    ]}
                  >
                    <List.Item.Meta
                      avatar={
                        <Tag
                          color={
                            result.status === 'success'
                              ? 'green'
                              : result.status === 'error'
                              ? 'red'
                              : 'blue'
                          }
                        >
                          {result.status === 'success'
                            ? '成功'
                            : result.status === 'error'
                            ? '失败'
                            : '进行中'}
                        </Tag>
                      }
                      title={
                        <Space>
                          <Text>{result.promptName}</Text>
                          {result.duration && <Text type="secondary">{result.duration} ms</Text>}
                        </Space>
                      }
                      description={
                        <Text type="secondary" ellipsis>
                          {dayjs(result.createdAt).format('YYYY-MM-DD HH:mm:ss')}
                        </Text>
                      }
                    />
                  </List.Item>
                )}
              />
            )}
          </ProCard>

          {/* Variables Input */}
          <ProCard
            title="测试参数"
            size="small"
            tooltip={'格式：JSON 对象，例如: {`{"key": "value", "name": "test"}`}'}
          >
            <CodeEditor
              language="json"
              height="300px"
              value={variables}
              onChange={setVariables}
              placeholder='请输入 JSON 对象，例如: {"key": "value"}'
              onBlur={() => {
                if (!validateJson(variables)) {
                  setVariablesStatus('error');
                } else {
                  setVariablesStatus(undefined);
                }
              }}
            />
          </ProCard>

          {/* Test Button */}
          <div style={{ textAlign: 'right' }}>
            <Button
              type="primary"
              icon={<ExperimentOutlined />}
              onClick={handleTest}
              loading={testing}
              disabled={!selectedPrompt}
            >
              测试
            </Button>
          </div>
        </Space>
      </Modal>

      {/* Loading Modal */}
      <Modal title="测试进行中" open={loadingModalOpen} closable={false} footer={null} centered>
        <Space direction="vertical" style={{ width: '100%' }} align="center">
          <Spin size="large" />
          <Text>测试中，请稍候...</Text>
          <Text type="secondary">已用时: {elapsedSeconds} 秒</Text>
        </Space>
      </Modal>

      <Modal
        title={`测试结果详情`}
        width={800}
        open={resultModalOpen}
        footer={null}
        onCancel={() => setResultModalOpen(false)}
      >
        <Space direction="vertical" style={{ width: '100%', marginTop: 10 }} size="middle">
          <ProDescriptions column={2} size="small">
            <ProDescriptions.Item label="Prompt">{result?.promptName}</ProDescriptions.Item>
            <ProDescriptions.Item label="耗时">{result?.duration} ms</ProDescriptions.Item>
            <ProDescriptions.Item label="状态">
              <Tag
                color={
                  result?.status === 'success'
                    ? 'green'
                    : result?.status === 'error'
                    ? 'red'
                    : 'blue'
                }
              >
                {result?.status === 'success'
                  ? '成功'
                  : result?.status === 'error'
                  ? '失败'
                  : '进行中'}
              </Tag>
            </ProDescriptions.Item>
            {result?.error && (
              <ProDescriptions.Item label="错误信息" ellipsis tooltip={result?.error}>
                <Text type="danger">{result?.error}</Text>
              </ProDescriptions.Item>
            )}
            {result?.metadata && (
              <>
                <ProDescriptions.Item label="Model">{result?.metadata.model}</ProDescriptions.Item>
                <ProDescriptions.Item label="Provider">
                  {result?.metadata.provider}
                </ProDescriptions.Item>
              </>
            )}
          </ProDescriptions>
          {result?.usage && (
            <>
              <Row>
                <Col span={24}>
                  <Typography.Title level={5}>Usage</Typography.Title>
                </Col>
              </Row>
              <Row gutter={16} justify="space-evenly">
                <Col span={8}>
                  <Card variant="borderless">
                    <Statistic title="Total Tokens" value={result?.usage.totalTokens} />
                  </Card>
                </Col>
                <Col span={8}>
                  <Card variant="borderless">
                    <Statistic title="Prompt Tokens" value={result?.usage.promptTokens} />
                  </Card>
                </Col>
                <Col span={8}>
                  <Card variant="borderless">
                    <Statistic title="Completion Tokens" value={result?.usage.completionTokens} />
                  </Card>
                </Col>
              </Row>
            </>
          )}
          <ProCard
            bordered
            tabs={{
              activeKey: resultTabActiveKey,
              items: [
                {
                  label: `Variables`,
                  key: 'variables',
                  children: (
                    <CodeEditor language="json" height="300px" value={result?.variables} readonly />
                  ),
                },
                {
                  label: `Result`,
                  key: 'result',
                  children: (
                    <CodeEditor language="text" height="300px" value={result?.result} readonly />
                  ),
                },
              ],
              onChange: (key) => {
                setResultTabActiveKey(key);
              },
            }}
          ></ProCard>
        </Space>
      </Modal>
    </>
  );
};

export default PromptTestModal;
