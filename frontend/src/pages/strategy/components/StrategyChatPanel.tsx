import { openUnifiedChatStream } from '@/pages/chat/service';
import type { ChatDeltaEvent } from '@/pages/chat/types';
import { SignalDefinition, SignalScope, SignalType, Strategy, StrategyParam } from '@/services/gateway/strategy';
import { ClearOutlined, RobotOutlined, SendOutlined, StopOutlined, UserOutlined } from '@ant-design/icons';
import { Button, Empty, Input, message, Spin, theme, Tooltip, Typography } from 'antd';
import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkGfm from 'remark-gfm';
import '@/pages/chat/index.less';

type ChatMsg = {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  createdAt: number;
  applied?: boolean;
};

export type StrategyPatch = {
  name: string;
  description: string;
  code: string;
  params: StrategyParam[];
  signals: SignalDefinition[];
};

type StrategyChatPanelProps = {
  getCurrentStrategy: () => Partial<Strategy>;
  onApplyStrategy: (strategy: StrategyPatch) => void;
};

const sanitizeStreamText = (text?: string) => {
  if (!text) {
    return '';
  }
  return text
    .replace(/\u001b\[[0-9;?]*[ -/]*[@-~]/g, '')
    .replace(/\u001b[@-_]/g, '')
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, '');
};

const normalizeStrategyPatch = (raw: any): StrategyPatch | undefined => {
  let payload = raw;
  if (typeof payload === 'string') {
    try {
      payload = JSON.parse(payload);
    } catch {
      return undefined;
    }
  }

  const strategy = payload?.strategy && typeof payload.strategy === 'object' ? payload.strategy : payload;
  if (!strategy || typeof strategy !== 'object') {
    return undefined;
  }

  const name = typeof strategy.name === 'string' ? strategy.name.trim() : '';
  const description = typeof strategy.description === 'string' ? strategy.description.trim() : '';
  const code = typeof strategy.code === 'string' ? strategy.code : '';
  if (!name || !description || !code.trim()) {
    return undefined;
  }

  const params = (Array.isArray(strategy.params) ? strategy.params : []).map((item: any) => ({
    name: item?.name || '',
    description: item?.description || '',
    type: item?.type,
    required: !!item?.required,
    default: item?.default,
  })) as StrategyParam[];

  const signals = (Array.isArray(strategy.signals) ? strategy.signals : []).map((item: any, index: number) => ({
    id: item?.id || `${index + 1}`,
    type: item?.type as SignalType,
    scope: (item?.scope || SignalScope.Strategy) as SignalScope,
    exchange: item?.exchange,
    symbol: item?.symbol,
    props: typeof item?.props === 'string' ? item.props : item?.props ? JSON.stringify(item.props) : undefined,
  })) as SignalDefinition[];

  return {
    name,
    description,
    code,
    params,
    signals,
  };
};

const buildStrategyPrompt = (userQuery: string, currentStrategy: Partial<Strategy>) => {
  return `你正在 NovaForge 策略详情页中协助用户分析和修改当前策略。

当前策略 JSON：
\`\`\`json
${JSON.stringify(currentStrategy, null, 2)}
\`\`\`

用户需求：
${userQuery}

处理规则：
1. 如果用户只是询问、解释或分析策略，请直接回答。
2. 如果用户要求修改策略，请调用工具 skill.generate_strategy，并把 query 设置为用户需求，把 current_strategy 设置为上面的当前策略 JSON。
3. 工具返回新策略后，前端会自动应用名称、描述、参数、信号和代码。`;
};

const StrategyChatPanel = ({ getCurrentStrategy, onApplyStrategy }: StrategyChatPanelProps) => {
  const { token } = theme.useToken();
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [input, setInput] = useState('');
  const [generating, setGenerating] = useState(false);
  const [sessionId, setSessionId] = useState<string>();
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController>();
  const appliedEventIdsRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    return () => {
      abortRef.current?.abort();
    };
  }, []);

  useEffect(() => {
    if (messages.length > 0 || generating) {
      const el = messagesContainerRef.current;
      if (el) {
        el.scrollTop = el.scrollHeight;
      }
    }
  }, [messages, generating]);

  const patchAssistantMessage = (assistantId: string, updater: (msg: ChatMsg) => ChatMsg) => {
    setMessages((prev) => prev.map((msg) => (msg.id === assistantId ? updater(msg) : msg)));
  };

  const handleSend = async () => {
    const trimmed = input.trim();
    if (!trimmed) {
      message.warning('请输入问题或调整需求');
      return;
    }

    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    const userMsg: ChatMsg = {
      id: `u_${Date.now()}`,
      role: 'user',
      content: trimmed,
      createdAt: Date.now(),
    };
    const assistantId = `a_${Date.now()}`;

    setMessages((prev) => [
      ...prev,
      userMsg,
      {
        id: assistantId,
        role: 'assistant',
        content: '',
        createdAt: Date.now(),
      },
    ]);
    setInput('');
    setGenerating(true);

    try {
      await openUnifiedChatStream(
        {
          ...(sessionId ? { sessionId } : {}),
          content: buildStrategyPrompt(trimmed, getCurrentStrategy()),
        },
        async (event: ChatDeltaEvent) => {
          if (event.type === 'ready') {
            const sid = (event.delta?.sessionId as string) || event.sessionId;
            if (sid) {
              setSessionId(sid);
            }
            return;
          }

          if (event.type === 'text' || event.type === 'code') {
            const text = sanitizeStreamText(event.delta?.text || '');
            if (!text) {
              return;
            }
            patchAssistantMessage(assistantId, (msg) => ({
              ...msg,
              content: event.delta?.append ? `${msg.content}${text}` : `${msg.content}${msg.content ? '\n' : ''}${text}`,
            }));
            return;
          }

          if (event.type === 'tool_call') {
            const toolName = event.delta?.toolName || event.delta?.tool_name;
            if (toolName === 'skill.generate_strategy') {
              patchAssistantMessage(assistantId, (msg) =>
                msg.content
                  ? msg
                  : {
                      ...msg,
                      content: '正在根据当前策略生成修改方案...',
                    },
              );
            }
            return;
          }

          if (event.type === 'tool_result') {
            if (appliedEventIdsRef.current.has(event.id)) {
              return;
            }
            const patch = normalizeStrategyPatch(event.delta?.result);
            if (!patch) {
              return;
            }
            appliedEventIdsRef.current.add(event.id);
            onApplyStrategy(patch);
            patchAssistantMessage(assistantId, (msg) => ({
              ...msg,
              content: `${msg.content || '已生成策略修改。'}\n\n已自动应用到当前策略表单。`,
              applied: true,
            }));
            message.success('AI 修改已自动应用');
            return;
          }

          if (event.type === 'error') {
            throw new Error(event.delta?.message || 'AI 生成失败');
          }
        },
        controller.signal,
      );
    } catch (err: any) {
      if (err?.name !== 'AbortError') {
        message.error(err?.message || 'AI 生成失败，请稍后重试');
        patchAssistantMessage(assistantId, (msg) =>
          msg.content
            ? msg
            : {
                ...msg,
                content: '生成失败，请稍后重试。',
              },
        );
      }
    } finally {
      setGenerating(false);
    }
  };

  const handleStop = () => {
    abortRef.current?.abort();
    setGenerating(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      handleSend();
    }
  };

  const handleClear = () => {
    abortRef.current?.abort();
    setMessages([]);
    setInput('');
    setSessionId(undefined);
    appliedEventIdsRef.current.clear();
    setGenerating(false);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div
        style={{
          padding: '6px 10px',
          borderBottom: `1px solid ${token.colorBorderSecondary}`,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          flexShrink: 0,
        }}
      >
        <Typography.Text strong style={{ fontSize: 13 }}>
          AI 助手
        </Typography.Text>
        <Tooltip title="清空对话">
          <Button
            type="text"
            size="small"
            icon={<ClearOutlined />}
            onClick={handleClear}
            disabled={generating || messages.length === 0}
          />
        </Tooltip>
      </div>

      <div ref={messagesContainerRef} style={{ flex: 1, overflow: 'auto', padding: '10px 10px 4px' }}>
        {messages.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                询问策略问题，或描述调整需求；AI 将基于当前策略自动修改表单
              </Typography.Text>
            }
            style={{ margin: '56px 0' }}
          />
        ) : (
          messages.map((msg) => (
            <div key={msg.id} style={{ marginBottom: 12 }}>
              {msg.role === 'user' ? (
                <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 6, alignItems: 'flex-start' }}>
                  <div
                    style={{
                      background: token.colorPrimary,
                      color: token.colorTextLightSolid,
                      padding: '6px 10px',
                      borderRadius: '12px 12px 2px 12px',
                      maxWidth: '85%',
                      fontSize: 13,
                      lineHeight: 1.5,
                      wordBreak: 'break-word',
                    }}
                  >
                    {msg.content}
                  </div>
                  <UserOutlined style={{ color: token.colorPrimary, marginTop: 6, flexShrink: 0 }} />
                </div>
              ) : (
                <div style={{ display: 'flex', justifyContent: 'flex-start', gap: 6, alignItems: 'flex-start' }}>
                  <RobotOutlined style={{ color: token.colorPrimary, marginTop: 6, flexShrink: 0 }} />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div
                      style={{
                        background: token.colorBgContainer,
                        border: `1px solid ${token.colorBorderSecondary}`,
                        borderRadius: '2px 10px 10px 10px',
                        overflow: 'hidden',
                      }}
                    >
                      <div
                        className="chat-markdown-body"
                        style={{
                          padding: '8px 10px',
                          color: token.colorText,
                          background: token.colorBgContainer,
                          overflow: 'auto',
                          maxHeight: 200,
                        }}
                      >
                        <ReactMarkdown
                          remarkPlugins={[remarkGfm]}
                          rehypePlugins={[rehypeHighlight]}
                          components={{
                            table: ({ children, ...props }) => (
                              <div className="chat-markdown-table-wrap">
                                <table {...props}>{children}</table>
                              </div>
                            ),
                          }}
                        >
                          {msg.content || '...'}
                        </ReactMarkdown>
                      </div>
                    </div>
                    {msg.applied && (
                      <Typography.Text type="success" style={{ display: 'block', marginTop: 6, fontSize: 12 }}>
                        已自动应用
                      </Typography.Text>
                    )}
                  </div>
                </div>
              )}
            </div>
          ))
        )}
        {generating && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
            <RobotOutlined style={{ color: token.colorPrimary }} />
            <Spin size="small" />
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              生成中...
            </Typography.Text>
          </div>
        )}
      </div>

      <div
        style={{
          padding: '8px 10px',
          borderTop: `1px solid ${token.colorBorderSecondary}`,
          flexShrink: 0,
        }}
      >
        <div style={{ position: 'relative' }}>
          <Input.TextArea
            rows={3}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="询问或描述调整需求... (Ctrl+Enter 发送)"
            disabled={generating}
            style={{
              resize: 'none',
              fontSize: 13,
              paddingRight: 74,
              paddingBottom: 34,
            }}
          />
          {generating ? (
            <Button
              icon={<StopOutlined />}
              onClick={handleStop}
              size="small"
              style={{
                position: 'absolute',
                right: 8,
                bottom: 8,
              }}
            >
              停止
            </Button>
          ) : (
            <Button
              type="primary"
              icon={<SendOutlined />}
              onClick={handleSend}
              size="small"
              style={{
                position: 'absolute',
                right: 8,
                bottom: 8,
              }}
            >
              发送
            </Button>
          )}
        </div>
      </div>
    </div>
  );
};

export default StrategyChatPanel;
