import { CodeBlock } from '@/components';
import { queryLlmProviderConfig } from '@/services/gateway/llm';
import { createStrategy, SignalScope, SignalType } from '@/services/gateway/strategy';
import { CheckCircleOutlined, CloseCircleOutlined, DeleteOutlined, EditOutlined, EllipsisOutlined, LoadingOutlined, OpenAIOutlined, PlusOutlined, ReloadOutlined, SaveOutlined, SendOutlined } from '@ant-design/icons';
import { history, useParams } from '@umijs/max';
import {
  Alert,
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Dropdown,
  Flex,
  Input,
  message,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Tabs,
  theme,
  Typography,
} from 'antd';
import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { flushSync } from 'react-dom';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkGfm from 'remark-gfm';
import './index.less';
import { deleteChatSession, generateSessionTitleByFirstTurn, getChatSession, listChatModels, listChatSessions, openUnifiedChatStream, updateChatSessionTitle } from './service';
import type { ChatDeltaEvent, ChatDialog, ChatModelOption, ChatPart, ChatSession, ChatSessionDetail, GeneratedStrategyPayload } from './types';

const { TextArea } = Input;

const FALLBACK_DEFAULT_MODEL = 'minimax/minimax-m2.5';

/** 距底部小于此值视为「在底部」，恢复流式跟随时自动滚到底 */
const SCROLL_BOTTOM_THRESHOLD_PX = 64;

const isMessageListNearBottom = (el: HTMLDivElement) =>
  el.scrollHeight - el.scrollTop - el.clientHeight <= SCROLL_BOTTOM_THRESHOLD_PX;

// 过滤终端/工具输出中夹带的 ANSI 控制序列与不可见控制字符，避免在 UI 末尾出现类似 "[e~[" 的乱码尾巴。
const sanitizeStreamText = (text?: string) => {
  if (!text) {
    return '';
  }
  return text
    .replace(/\u001b\[[0-9;?]*[ -/]*[@-~]/g, '')
    .replace(/\u001b[@-_]/g, '')
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, '');
};

const mergeStreamingPart = (parts: ChatPart[], incoming: ChatPart) => {
  if (incoming.append && parts.length > 0) {
    const last = parts[parts.length - 1];
    if (last.type === incoming.type && last.blockId === incoming.blockId) {
      return [
        ...parts.slice(0, -1),
        {
          ...last,
          text: `${last.text || ''}${incoming.text || ''}`,
        },
      ];
    }
  }
  return [...parts, incoming];
};

/** 将 parts 中同一 toolCallId 的 tool_call + tool_result 合并为 tool_invocation */
const consolidateParts = (parts: ChatPart[]): ChatPart[] => {
  const result: ChatPart[] = [];
  const toolMap = new Map<string, ChatPart>();

  for (const part of parts) {
    if (part.type === 'tool_call' && part.toolCallId) {
      const existing = toolMap.get(part.toolCallId);
      if (existing) {
        if (part.status !== undefined) existing.status = part.status;
        if (part.arguments !== undefined) existing.arguments = part.arguments;
        if (part.toolName) existing.toolName = part.toolName;
      } else {
        const merged: ChatPart = {
          type: 'tool_invocation',
          toolCallId: part.toolCallId,
          toolName: part.toolName,
          status: part.status,
          arguments: part.arguments,
          hasResult: false,
        };
        toolMap.set(part.toolCallId, merged);
        result.push(merged);
      }
    } else if (part.type === 'tool_result' && part.toolCallId) {
      const existing = toolMap.get(part.toolCallId);
      if (existing) {
        existing.result = part.result;
        existing.message = part.message;
        existing.format = part.format;
        existing.hasResult = true;
        if (part.status !== undefined) existing.status = part.status;
        if (part.toolName) existing.toolName = part.toolName;
      } else {
        const merged: ChatPart = {
          type: 'tool_invocation',
          toolCallId: part.toolCallId,
          toolName: part.toolName,
          status: part.status,
          result: part.result,
          message: part.message,
          format: part.format,
          hasResult: true,
        };
        toolMap.set(part.toolCallId, merged);
        result.push(merged);
      }
    } else {
      result.push(part);
    }
  }

  return result;
};

const normalizeGeneratedStrategy = (raw: any): GeneratedStrategyPayload | undefined => {
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
  const code = typeof strategy.code === 'string' ? strategy.code : '';
  const description = typeof strategy.description === 'string' ? strategy.description.trim() : '';
  if (!name || !code.trim() || !description) {
    return undefined;
  }
  const params = Array.isArray(strategy.params) ? strategy.params : [];
  const signals = Array.isArray(strategy.signals) ? strategy.signals : [];
  return {
    name,
    code,
    description,
    params,
    signals,
  };
};

const StrategySkillResultCard: React.FC<{ strategy: GeneratedStrategyPayload }> = ({ strategy }) => {
  const [creating, setCreating] = useState(false);

  const handleCreate = async () => {
    if (creating) {
      return;
    }
    setCreating(true);
    try {
      const params = (strategy.params || []).map((item) => ({
        name: item.name,
        description: item.description || '',
        type: item.type as any,
        required: !!item.required,
        default: item.default,
      }));
      const signals = (strategy.signals || []).map((item) => ({
        id: item.id,
        type: item.type as SignalType,
        scope: (item.scope || SignalScope.Strategy) as SignalScope,
        exchange: item.exchange as any,
        symbol: item.symbol,
        props: item.props ? JSON.stringify(item.props) : undefined,
      }));
      const resp = await createStrategy({
        name: strategy.name,
        description: strategy.description,
        code: strategy.code,
        params: params as any,
        signals: signals as any,
      });
      if (resp?.errors?.length) {
        throw new Error(resp.errors[0]?.message || '创建策略失败');
      }
      const created = resp?.data?.Result;
      if (!created?.id) {
        throw new Error('创建策略失败：未返回策略ID');
      }
      message.success('策略创建成功');
      history.push(`/strategy/${created.id}`);
    } catch (error: any) {
      message.error(error?.message || '创建策略失败');
    } finally {
      setCreating(false);
    }
  };

  return (
    <Card
      size="small"
      title="策略生成结果"
      extra={
        <Button type="primary" loading={creating} onClick={handleCreate} size="small">
          <SaveOutlined /> 保存策略
        </Button>
      }
    >
      <Tabs
        items={[
          {
            key: 'overview',
            label: '策略信息',
            children: (
              <Descriptions size="small" column={1} labelStyle={{ width: 100 }} bordered>
                <Descriptions.Item label="策略名称">{strategy.name}</Descriptions.Item>
                <Descriptions.Item label="策略描述">{strategy.description}</Descriptions.Item>
              </Descriptions>
            ),
          },
          {
            key: 'code',
            label: '策略代码',
            children: <CodeBlock language="javascript" value={strategy.code} />,
          },
          {
            key: 'params',
            label: `策略参数 (${strategy.params?.length || 0})`,
            children: (
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                {JSON.stringify(strategy.params || [], null, 2)}
              </pre>
            ),
          },
          {
            key: 'signals',
            label: `策略信号 (${strategy.signals?.length || 0})`,
            children: (
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                {JSON.stringify(strategy.signals || [], null, 2)}
              </pre>
            ),
          },
        ]}
      />
    </Card>
  );
};

const toolCallPreStyle: React.CSSProperties = {
  margin: '4px 0 0',
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-all',
  fontSize: 12,
  lineHeight: '1.5',
};

const ToolCallBlock: React.FC<{ part: ChatPart; partIdx: number; isStreaming?: boolean }> = ({ part, partIdx, isStreaming }) => {
  const { token } = theme.useToken();
  const [bodyExpanded, setBodyExpanded] = useState(false);
  const isError = part.status === 'error';
  // 只有真正在流式输出时才视为"执行中"；对话结束后即使没有 result 也不显示 loading
  const isPending = !part.hasResult && !!isStreaming;

  const argsText = JSON.stringify(part.arguments || {}, null, 2);
  const hasArgs = argsText !== '{}';
  const resultText =
    part.result !== undefined
      ? typeof part.result === 'string'
        ? part.result
        : JSON.stringify(part.result, null, 2)
      : '';
  const generatedStrategy = normalizeGeneratedStrategy(part.result);
  const hasBody = hasArgs || part.hasResult;

  let statusIcon: React.ReactNode;
  if (isPending) {
    statusIcon = <LoadingOutlined style={{ color: token.colorTextSecondary }} />;
  } else if (isError) {
    statusIcon = <CloseCircleOutlined style={{ color: token.colorError }} />;
  } else {
    statusIcon = <CheckCircleOutlined style={{ color: token.colorSuccess }} />;
  }

  return (
    <Card
      key={partIdx}
      size="small"
      style={{ background: token.colorFillAlter, borderColor: token.colorBorderSecondary }}
      title={
        <Space size={6}>
          {statusIcon}
          <Typography.Text style={{ fontWeight: 500 }}>{part.toolName || '未知工具'}</Typography.Text>
          {isPending && (
            <Typography.Text type="secondary" style={{ fontWeight: 400, fontSize: 12 }}>
              执行中...
            </Typography.Text>
          )}
        </Space>
      }
      extra={
        hasBody ? (
          <Button
            type="link"
            size="small"
            style={{ padding: 0, height: 'auto' }}
            onClick={() => setBodyExpanded((v) => !v)}
          >
            {bodyExpanded ? '收起' : '展开'}
          </Button>
        ) : undefined
      }
      styles={!bodyExpanded && hasBody ? { body: { display: 'none', padding: 0 } } : undefined}
    >
      {bodyExpanded && (
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          {hasArgs && (
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 11 }}>参数</Typography.Text>
              <pre style={toolCallPreStyle}>{argsText}</pre>
            </div>
          )}
          {part.hasResult && (
            <>
              {hasArgs && <Divider style={{ margin: '4px 0' }} />}
              {isError ? (
                <Alert type="error" showIcon message={part.message || '工具执行失败'} />
              ) : generatedStrategy ? (
                <StrategySkillResultCard strategy={generatedStrategy} />
              ) : (
                <div>
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}>结果</Typography.Text>
                  <pre style={toolCallPreStyle}>{resultText}</pre>
                </div>
              )}
            </>
          )}
        </Space>
      )}
    </Card>
  );
};

const eventToPart = (event: ChatDeltaEvent): ChatPart | null => {
  const delta = event.delta || {};
  // 兼容 snake_case（部分网关/序列化）与 camelCase
  const toolCallId = (delta.toolCallId ?? delta.tool_call_id) as string | undefined;
  const toolName = (delta.toolName ?? delta.tool_name) as string | undefined;
  switch (event.type) {
    case 'text':
      return { type: 'text', text: sanitizeStreamText(delta.text || ''), append: !!delta.append };
    case 'code':
      return {
        type: 'code',
        text: sanitizeStreamText(delta.text || ''),
        language: delta.language || 'text',
        blockId: delta.blockId,
        append: !!delta.append,
      };
    case 'thinking':
      return {
        type: 'thinking',
        text: sanitizeStreamText(delta.text || ''),
        blockId: delta.blockId,
        collapsed: true,
        append: !!delta.append,
      };
    case 'tool_call':
      return {
        type: 'tool_call',
        toolCallId,
        toolName,
        arguments: delta.arguments,
        status: delta.status,
      };
    case 'tool_result':
      return {
        type: 'tool_result',
        toolCallId,
        toolName,
        result: delta.result,
        message: delta.message,
        format: delta.format,
        status: delta.status,
      };
    case 'interactive':
      return {
        type: 'interactive',
        component: delta.component,
        actionId: delta.actionId,
        props: delta.props,
      };
    case 'error':
      return {
        type: 'error',
        code: delta.code,
        message: delta.message,
      };
    default:
      return null;
  }
};

const renderPart = (part: ChatPart, idx: number, token: ReturnType<typeof theme.useToken>['token'], isStreaming?: boolean) => {
  switch (part.type) {
    case 'code':
      return <CodeBlock key={idx} language={part.language || 'text'} value={part.text || ''} />;
    case 'thinking':
      return (
        <Card
          key={idx}
          size="small"
          style={{ background: token.colorFillAlter, borderColor: token.colorBorderSecondary }}
          title="思考过程"
        >
          <Typography.Paragraph style={{ marginBottom: 0, whiteSpace: 'pre-wrap' }}>{part.text}</Typography.Paragraph>
        </Card>
      );
    case 'tool_invocation':
      return <ToolCallBlock key={part.toolCallId || idx} part={part} partIdx={idx} isStreaming={isStreaming} />;
    case 'tool_call':
      return <ToolCallBlock key={part.toolCallId || idx} part={{ ...part, type: 'tool_invocation', hasResult: false }} partIdx={idx} isStreaming={isStreaming} />;
    case 'tool_result':
      return <ToolCallBlock key={part.toolCallId || idx} part={{ ...part, type: 'tool_invocation', hasResult: true }} partIdx={idx} isStreaming={isStreaming} />;
    case 'interactive':
      return (
        <Alert
          key={idx}
          type="info"
          showIcon
          message="交互组件"
          description={<pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{JSON.stringify(part.props || {}, null, 2)}</pre>}
        />
      );
    case 'error':
      return <Alert key={idx} type="error" showIcon message={part.message || '生成失败'} />;
    case 'text':
    default:
      return (
        <div key={idx} className="chat-markdown-body">
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
            {part.text || ''}
          </ReactMarkdown>
        </div>
      );
  }
};

export default function ChatPage() {
  const { token } = theme.useToken();
  // 单一路由 /chat/:sessionId：草稿为 sessionId=new，避免与真实 id 拆成两条路由导致 replace 时组件卸载、abort 打断 SSE
  const params = useParams<{ sessionId?: string }>();
  const routeSegment = params.sessionId ?? 'new';
  const isNewChatRoute = routeSegment === 'new';
  const urlSessionId = isNewChatRoute ? undefined : routeSegment;

  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [detail, setDetail] = useState<ChatSessionDetail>();
  const [prompt, setPrompt] = useState('');
  const [loading, setLoading] = useState(false);
  const [sending, setSending] = useState(false);
  const [streamingDialogId, setStreamingDialogId] = useState<string>();
  const [streamingParts, setStreamingParts] = useState<ChatPart[]>([]);
  const [streamingStatus, setStreamingStatus] = useState<string>('pending');
  const abortRef = useRef<AbortController | null>(null);
  const messageListScrollRef = useRef<HTMLDivElement>(null);
  const stickMessageListToBottomRef = useRef(true);
  const userPickedModelRef = useRef(false);
  const prevSessionIdRef = useRef<string | undefined>(undefined);
  const prevIsNewChatRouteRef = useRef(false);
  const skipNextRouteDetailLoadRef = useRef(false);
  const [modelOptions, setModelOptions] = useState<ChatModelOption[]>([]);
  const [settingsDefaultModel, setSettingsDefaultModel] = useState<string>(FALLBACK_DEFAULT_MODEL);
  const [selectedModel, setSelectedModel] = useState<string>(FALLBACK_DEFAULT_MODEL);
  const [hoveredSessionId, setHoveredSessionId] = useState<string>();
  const [pendingQuestion, setPendingQuestion] = useState<string>();

  const loadSessions = async () => {
    const rows = await listChatSessions();
    setSessions(rows);
  };

  const loadDetail = async (sessionId: string, options?: { silent?: boolean }) => {
    if (!options?.silent) {
      setLoading(true);
    }
    try {
      const resp = await getChatSession(sessionId);
      setDetail(resp);
    } catch {
      message.error('会话不存在或无权访问');
      history.replace('/chat/new');
    } finally {
      if (!options?.silent) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    loadSessions();
    listChatModels().then(setModelOptions).catch(() => setModelOptions([]));
    queryLlmProviderConfig()
      .then((row) => {
        console.log('row', row);
        setSettingsDefaultModel(row.defaultModel?.trim() || FALLBACK_DEFAULT_MODEL);
      })
      .catch(() => {
        setSettingsDefaultModel(FALLBACK_DEFAULT_MODEL);
      });
    return () => abortRef.current?.abort();
  }, []);

  useEffect(() => {
    const enteredDraft = isNewChatRoute && !prevIsNewChatRouteRef.current;
    prevIsNewChatRouteRef.current = isNewChatRoute;
    if (isNewChatRoute) {
      setLoading(false);
      if (enteredDraft) {
        setDetail(undefined);
      }
      return;
    }
    if (!urlSessionId) {
      return;
    }
    if (skipNextRouteDetailLoadRef.current) {
      skipNextRouteDetailLoadRef.current = false;
      return;
    }
    // 首轮发送后 history.replace 到同一会话时，避免 loadDetail 再次 setLoading 打断流式 UI
    if (detail?.session?.id === urlSessionId) {
      return;
    }
    void loadDetail(urlSessionId);
  }, [isNewChatRoute, urlSessionId, detail?.session?.id]);

  useEffect(() => {
    stickMessageListToBottomRef.current = true;
  }, [isNewChatRoute, urlSessionId]);

  useEffect(() => {
    const sid = urlSessionId;
    if (sid !== prevSessionIdRef.current) {
      userPickedModelRef.current = false;
      prevSessionIdRef.current = sid;
    }
    if (!userPickedModelRef.current) {
      setSelectedModel(settingsDefaultModel);
    }
  }, [settingsDefaultModel, urlSessionId]);

  const dialogs = useMemo(() => detail?.dialogs || [], [detail]);

  const modelSelectOptions = useMemo(() => {
    const ids = new Set(modelOptions.map((o) => o.id));
    const opts = modelOptions.map((o) => ({ label: o.id, value: o.id }));
    if (selectedModel && !ids.has(selectedModel)) {
      opts.unshift({ label: selectedModel, value: selectedModel });
    }
    return opts;
  }, [modelOptions, selectedModel]);
  const turns = useMemo(() => {
    const grouped = new Map<string, { dialogId: string; question?: ChatDialog; answer?: ChatDialog }>();
    dialogs.forEach((dialog) => {
      const current = grouped.get(dialog.dialogId) || { dialogId: dialog.dialogId };
      if (dialog.role === 'question') {
        current.question = dialog;
      } else {
        current.answer = dialog;
      }
      grouped.set(dialog.dialogId, current);
    });
    return Array.from(grouped.values()).sort((a, b) => {
      const aSeq = Math.min(a.question?.seq || Number.MAX_SAFE_INTEGER, a.answer?.seq || Number.MAX_SAFE_INTEGER);
      const bSeq = Math.min(b.question?.seq || Number.MAX_SAFE_INTEGER, b.answer?.seq || Number.MAX_SAFE_INTEGER);
      return aSeq - bSeq;
    });
  }, [dialogs]);

  useLayoutEffect(() => {
    if (loading || !stickMessageListToBottomRef.current) {
      return;
    }
    const el = messageListScrollRef.current;
    if (!el) {
      return;
    }
    const scrollToEnd = () => {
      el.scrollTop = el.scrollHeight;
    };
    scrollToEnd();
    requestAnimationFrame(scrollToEnd);
  }, [loading, turns, streamingParts, streamingDialogId, streamingStatus]);

  const runChatStream = async (
    body: { sessionId?: string; dialogId?: string; regenerate?: boolean; content?: string },
    opts?: { replaceUrlOnReady?: boolean },
  ): Promise<string | undefined> => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setStreamingDialogId(undefined);
    setStreamingParts([]);
    setStreamingStatus('streaming');

    let capturedSessionId = body.sessionId;

    try {
      await openUnifiedChatStream(
        {
          ...body,
          model: selectedModel || undefined,
        },
        async (event) => {
          if (event.type === 'ready') {
            const sid = (event.delta?.sessionId as string) || event.sessionId;
            const did = (event.delta?.dialogId as string) || event.dialogId;
            if (sid) {
              capturedSessionId = sid;
              // 先拉取会话，使 turns 中出现本轮答案，再处理后续 SSE；否则 isStreaming 不成立，节点事件无法按帧展示。
              await loadDetail(sid, { silent: true });
              // 本轮 user 消息已进入 detail.turns，清除发送占位，否则会与 turns 内 question 重复（出现 question / answer / question）。
              setPendingQuestion(undefined);
            }
            if (did) {
              setStreamingDialogId(did);
            }
            if (opts?.replaceUrlOnReady && sid) {
              // 仅改 URL，不触发这一轮 route effect 的 detail 重拉。
              skipNextRouteDetailLoadRef.current = true;
              history.replace(`/chat/${sid}`);
            }
            return;
          }
          if (event.type === 'started') {
            setStreamingStatus('streaming');
            return;
          }
          if (event.type === 'done') {
            setStreamingStatus('completed');
            return;
          }
          if (event.type === 'error') {
            setStreamingStatus('error');
          }
          const next = eventToPart(event);
          if (next) {
            // 同一次 TCP chunk 内多条 SSE 会在同一同步循环里连续触发 onEvent；不用 flushSync 时 React 18 会合并为一次渲染，表现为「结束后才一次性出现」。
            if (next.type === 'text' && next.append) {
              setStreamingParts((prev) => mergeStreamingPart(prev, next));
            } else {
              flushSync(() => {
                setStreamingParts((prev) => mergeStreamingPart(prev, next));
              });
            }
          }
        },
        controller.signal,
      );
    } finally {
      setStreamingDialogId(undefined);
      setStreamingStatus('pending');
      setPendingQuestion(undefined);
      await loadSessions();
      if (capturedSessionId) {
        await loadDetail(capturedSessionId, { silent: true });
      }
    }
    return capturedSessionId;
  };

  const handleSend = async () => {
    const content = prompt.trim();
    if (!content) {
      message.warning('请输入问题');
      return;
    }
    if (!isNewChatRoute && !urlSessionId) {
      message.warning('请先选择会话');
      history.replace('/chat/new');
      return;
    }
    setSending(true);
    stickMessageListToBottomRef.current = true;
    try {
      setPendingQuestion(content);
      setPrompt('');
      if (isNewChatRoute) {
        const createdSessionId = await runChatStream({ content }, { replaceUrlOnReady: true });
        if (createdSessionId) {
          try {
            const nextTitle = (await generateSessionTitleByFirstTurn(createdSessionId)).trim();
            if (nextTitle) {
              setSessions((prev) => prev.map((item) => (item.id === createdSessionId ? { ...item, title: nextTitle } : item)));
              setDetail((prev) => {
                if (!prev || prev.session?.id !== createdSessionId) {
                  return prev;
                }
                return {
                  ...prev,
                  session: {
                    ...prev.session,
                    title: nextTitle,
                  },
                };
              });
            }
          } catch (error) {
            console.warn('generate session title failed', error);
          }
        }
      } else {
        await runChatStream({ sessionId: urlSessionId!, content });
      }
    } catch (error: any) {
      message.error(error?.message || '发送失败');
    } finally {
      setSending(false);
    }
  };

  const handleRegenerate = async (dialog: ChatDialog) => {
    if (!urlSessionId) {
      return;
    }
    setSending(true);
    stickMessageListToBottomRef.current = true;
    try {
      await runChatStream({
        sessionId: urlSessionId,
        dialogId: dialog.id,
        regenerate: true,
      });
    } catch (error: any) {
      message.error(error?.message || '重新生成失败');
    } finally {
      setSending(false);
    }
  };

  const handleDeleteSession = (sessionId: string) => {
    Modal.confirm({
      title: '删除会话',
      content: '删除后将无法在会话列表中查看该会话，是否继续？',
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        const isDeletingCurrent = sessionId === urlSessionId;
        if (isDeletingCurrent && streamingDialogId) {
          abortRef.current?.abort();
          setStreamingDialogId(undefined);
          setStreamingStatus('pending');
          setStreamingParts([]);
        }
        await deleteChatSession(sessionId);
        await loadSessions();
        if (isDeletingCurrent) {
          setPrompt('');
          setDetail(undefined);
          history.replace('/chat/new');
        }
        message.success('会话已删除');
      },
    });
  };

  const handleRenameSession = (session: ChatSession) => {
    let draftTitle = session.title || '';
    Modal.confirm({
      title: '重命名会话',
      content: <Input defaultValue={draftTitle} maxLength={128} autoFocus onChange={(e) => (draftTitle = e.target.value)} />,
      okText: '保存',
      cancelText: '取消',
      onOk: async () => {
        const nextTitle = (await updateChatSessionTitle(session.id, draftTitle)).trim();
        if (!nextTitle) {
          throw new Error('标题不能为空');
        }
        setSessions((prev) => prev.map((item) => (item.id === session.id ? { ...item, title: nextTitle } : item)));
        setDetail((prev) => {
          if (!prev || prev.session.id !== session.id) {
            return prev;
          }
          return {
            ...prev,
            session: {
              ...prev.session,
              title: nextTitle,
            },
          };
        });
        message.success('会话标题已更新');
      },
    });
  };

  const composer = (
    <div
      style={{
        border: `1px solid ${token.colorBorder}`,
        borderRadius: token.borderRadiusLG,
        background: token.colorBgContainer,
        padding: 12,
        boxShadow: token.boxShadowTertiary,
      }}
    >
      <Space direction="vertical" size={8} style={{ width: '100%' }}>
        <TextArea
          autoSize={{ minRows: 3, maxRows: 3 }}
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== 'Enter' || e.shiftKey) {
              return;
            }
            if (e.nativeEvent.isComposing) {
              return;
            }
            e.preventDefault();
            if (sending || streamingDialogId) {
              return;
            }
            void handleSend();
          }}
          placeholder="有问题，尽管问"
          disabled={sending || !!streamingDialogId}
          variant="borderless"
          style={{ padding: 0, resize: 'none', background: 'transparent' }}
        />
        <Flex justify="space-between" align="center" wrap="wrap" gap={8}>
          <Select
            showSearch
            optionFilterProp="label"
            style={{ width: 260, maxWidth: '100%', flexShrink: 1, color: 'red' }}
            options={modelSelectOptions}
            value={selectedModel || undefined}
            labelRender={(option) => (
              <Typography.Text style={{ color: token.colorTextSecondary }}>{option.label}</Typography.Text>
            )}
            onChange={(v) => {
              userPickedModelRef.current = true;
              setSelectedModel(v ?? '');
            }}
            disabled={sending || !!streamingDialogId}
            allowClear
          />
          <Button type="primary" icon={<SendOutlined />} loading={sending} disabled={!!streamingDialogId} onClick={handleSend}>
            发送
          </Button>
        </Flex>
      </Space>
    </div>
  );

  return (
    <Flex
      style={{
        height: 'calc(100vh - 140px)',
        minHeight: 0,
        gap: 16,
        padding: 8,
        boxSizing: 'border-box',
        overflow: 'hidden',
      }}
    >
      <Card
        title="会话"
        style={{ width: 280, flexShrink: 0, height: '100%', display: 'flex', flexDirection: 'column' }}
        extra={
          <Button
            icon={<PlusOutlined />}
            onClick={() => {
              abortRef.current?.abort();
              setPrompt('');
              setDetail(undefined);
              history.push('/chat/new');
            }}
          >
            新建
          </Button>
        }
        styles={{
          body: {
            padding: '4px 0 8px 0',
            flex: 1,
            minHeight: 0,
            display: 'flex',
            overflow: 'hidden',
          },
        }}
      >
        <div className="chat-session-scroll" style={{ flex: 1, minHeight: 0, overflow: 'auto' }}>
          <Flex vertical>
            {sessions.map((item) => {
              const isActive = item.id === urlSessionId;
              const isHover = item.id === hoveredSessionId;
              const rowBackground = isHover
                ? (isActive ? token.colorFillSecondary : token.colorFillTertiary)
                : isActive
                  ? token.colorFillAlter
                  : undefined;
              const rowTextColor = isActive ? 'var(--ant-color-text)' : 'var(--ant-color-text-secondary)';
              return (
                <Row
                  key={item.id}
                  wrap={false}
                  align="middle"
                  style={{
                    cursor: 'pointer',
                    padding: '6px 6px 6px 12px',
                    margin: '1px 0px 1px 10px',
                    borderRadius: token.borderRadiusLG,
                    background: rowBackground,
                    color: rowTextColor,
                    transition: 'background 0.2s ease',
                  }}
                  onMouseEnter={() => setHoveredSessionId(item.id)}
                  onMouseLeave={() => setHoveredSessionId(undefined)}
                  onClick={() => history.push(`/chat/${item.id}`)}
                >
                  <Col flex="auto" style={{ minWidth: 0 }}>
                    <Typography.Text ellipsis={{ tooltip: true }} style={{ color: rowTextColor }}>
                      {item.title || '未命名会话'}
                    </Typography.Text>
                  </Col>
                  <Col flex="none">
                    <Dropdown
                      trigger={['click']}
                      menu={{
                        items: [
                          {
                            key: 'rename',
                            label: (
                              <span>
                                <EditOutlined /> 重命名
                              </span>
                            ),
                          },
                          {
                            key: 'delete',
                            label: (
                              <span style={{ color: token.colorError }}>
                                <DeleteOutlined /> 删除
                              </span>
                            ),
                          },
                        ],
                        onClick: ({ key, domEvent }) => {
                          domEvent.stopPropagation();
                          if (key === 'rename') {
                            handleRenameSession(item);
                            return;
                          }
                          if (key === 'delete') {
                            handleDeleteSession(item.id);
                          }
                        },
                      }}
                    >
                      <Button
                        type="text"
                        size="small"
                        icon={<EllipsisOutlined />}
                        style={{
                          opacity: isHover ? 1 : 0,
                          pointerEvents: isHover ? 'auto' : 'none',
                          transition: 'opacity 0.2s ease',
                        }}
                        onClick={(e) => {
                          e.stopPropagation();
                        }}
                      />
                    </Dropdown>
                  </Col>
                </Row>
              );
            })}
          </Flex>
        </div>
      </Card>

      <Card
        title={detail?.session?.title || '聊天'}
        style={{ flex: 1, minWidth: 0, height: '100%', display: 'flex', flexDirection: 'column' }}
        styles={{
          body: {
            flex: 1,
            minHeight: 0,
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          },
        }}
      >
        {loading ? (
          <Flex align="center" justify="center" style={{ flex: 1, minHeight: 0 }}>
            <Spin />
          </Flex>
        ) : (
          <Flex vertical gap={16} style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
            <div
              className="chat-message-scroll"
              ref={messageListScrollRef}
              onScroll={() => {
                const el = messageListScrollRef.current;
                if (!el) {
                  return;
                }
                stickMessageListToBottomRef.current = isMessageListNearBottom(el);
              }}
              style={{ flex: 1, minHeight: 0, overflow: 'auto', paddingBottom: 16, paddingRight: 16 }}
            >
              {turns.length === 0 && !pendingQuestion ? (
                <Flex align="center" justify="center" style={{ minHeight: '100%' }}>
                  <Typography.Text style={{ fontSize: 26, fontWeight: 400, color: token.colorTextDescription }}>
                    准备好了，随时开始
                  </Typography.Text>
                </Flex>
              ) : (
                <Space direction="vertical" size={32} style={{ width: '100%' }}>
                  {turns.map((turn) => {
                    const answer = turn.answer;
                    const isStreaming = answer?.id === streamingDialogId;
                    const answerParts =
                      isStreaming && streamingParts.length > 0
                        ? streamingParts
                        : answer?.parts && answer.parts.length > 0
                          ? answer.parts
                          : answer?.contentText
                            ? [{ type: 'text', text: sanitizeStreamText(answer.contentText) }]
                            : [];

                    return (
                      <Space key={turn.dialogId} direction="vertical" size={32} style={{ width: '100%' }}>
                        {turn.question ? (
                          <Flex justify="flex-end">
                            <Card
                              size="small"
                              style={{
                                width: 'min(820px, 100%)',
                                background: token.colorFillTertiary,
                                borderColor: token.colorBorder,
                              }}
                            >
                              <Typography.Paragraph style={{ marginBottom: 0, whiteSpace: 'pre-wrap' }}>
                                {turn.question.contentText}
                              </Typography.Paragraph>
                            </Card>
                          </Flex>
                        ) : null}

                        {answer ? (
                          <Card
                            size="small"
                            title={
                              <Space wrap>
                                <OpenAIOutlined />
                                <span>AI</span>
                                {answer.model ? (
                                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                                    {answer.model}
                                  </Typography.Text>
                                ) : null}
                              </Space>
                            }
                            extra={
                              answer.canRegenerate && !streamingDialogId ? (
                                <Button type={'text'} size="small" icon={<ReloadOutlined />} onClick={() => handleRegenerate(answer)} />
                              ) : null
                            }
                          >
                            <Space direction="vertical" size={12} style={{ width: '100%' }}>
                              {consolidateParts(answerParts).map((part, idx) => renderPart(part, idx, token, isStreaming))}
                              {isStreaming && streamingStatus === 'streaming' ? (
                                <Space size={8}><LoadingOutlined /><Typography.Text type="secondary">生成中...</Typography.Text></Space>
                              ) : null}
                              {answer.errorMessage ? <Alert type="error" showIcon message={answer.errorMessage} /> : null}
                            </Space>
                          </Card>
                        ) : null}
                      </Space>
                    );
                  })}
                  {pendingQuestion ? (
                    <Flex justify="flex-end">
                      <Card
                        size="small"
                        style={{
                          width: 'min(820px, 100%)',
                          background: token.colorFillTertiary,
                          borderColor: token.colorBorder,
                        }}
                      >
                        <Typography.Paragraph style={{ marginBottom: 0, whiteSpace: 'pre-wrap' }}>{pendingQuestion}</Typography.Paragraph>
                      </Card>
                    </Flex>
                  ) : null}
                </Space>
              )}
            </div>
            <div style={{ flexShrink: 0 }}>{composer}</div>
          </Flex>
        )}
      </Card>
    </Flex>
  );
}
