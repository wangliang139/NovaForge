import { generateStrategy, GenerateStrategyResponse } from '@/services/gateway/strategy';
import { ClearOutlined, RobotOutlined, SendOutlined, UserOutlined } from '@ant-design/icons';
import { Button, Empty, Input, message, Spin, Tooltip, Typography } from 'antd';
import { useEffect, useRef, useState } from 'react';

type ChatMsg = {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  createdAt: number;
};

type StrategyChatPanelProps = {
  code: string;
  readonly?: boolean;
  onApplyCode: (code: string) => void;
};

const StrategyChatPanel = ({ code, readonly = false, onApplyCode }: StrategyChatPanelProps) => {
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [input, setInput] = useState('');
  const [generating, setGenerating] = useState(false);
  const messagesContainerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (messages.length > 0 || generating) {
      const el = messagesContainerRef.current;
      if (el) {
        el.scrollTop = el.scrollHeight;
      }
    }
  }, [messages, generating]);

  const handleSend = async () => {
    if (readonly) return;
    const trimmed = input.trim();
    if (!trimmed) {
      message.warning('请输入调整需求');
      return;
    }

    const userMsg: ChatMsg = {
      id: `u_${Date.now()}`,
      role: 'user',
      content: trimmed,
      createdAt: Date.now(),
    };

    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setGenerating(true);

    const query = `以下是当前策略代码：\n\`\`\`javascript\n${code}\n\`\`\`\n\n请根据以下需求进行调整：${trimmed}`;

    try {
      const resp = (await generateStrategy({ query })) as GenerateStrategyResponse | undefined;
      if (!resp?.content) throw new Error('未生成有效代码');

      const aiMsg: ChatMsg = {
        id: `a_${Date.now()}`,
        role: 'assistant',
        content: resp.content,
        createdAt: Date.now(),
      };
      setMessages((prev) => [...prev, aiMsg]);
    } catch (err: any) {
      message.error(err?.message || 'AI 生成失败，请稍后重试');
    } finally {
      setGenerating(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      handleSend();
    }
  };

  const handleClear = () => {
    setMessages([]);
    setInput('');
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Header */}
      <div
        style={{
          padding: '6px 10px',
          borderBottom: '1px solid #f0f0f0',
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

      {/* Messages */}
      <div ref={messagesContainerRef} style={{ flex: 1, overflow: 'auto', padding: '10px 10px 4px' }}>
        {messages.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                描述你对策略的调整需求，AI 将基于当前代码生成新版本
              </Typography.Text>
            }
            style={{ margin: '32px 0' }}
          />
        ) : (
          messages.map((msg) => (
            <div key={msg.id} style={{ marginBottom: 12 }}>
              {msg.role === 'user' ? (
                <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 6, alignItems: 'flex-start' }}>
                  <div
                    style={{
                      background: '#1677ff',
                      color: '#fff',
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
                  <UserOutlined style={{ color: '#1677ff', marginTop: 6, flexShrink: 0 }} />
                </div>
              ) : (
                <div style={{ display: 'flex', justifyContent: 'flex-start', gap: 6, alignItems: 'flex-start' }}>
                  <RobotOutlined style={{ color: '#722ed1', marginTop: 6, flexShrink: 0 }} />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div
                      style={{
                        background: '#f6f8fa',
                        border: '1px solid #e8e8e8',
                        borderRadius: '2px 10px 10px 10px',
                        overflow: 'hidden',
                      }}
                    >
                      <pre
                        style={{
                          margin: 0,
                          padding: '8px 10px',
                          fontSize: 12,
                          overflow: 'auto',
                          maxHeight: 200,
                          fontFamily: 'monospace',
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-all',
                        }}
                      >
                        {msg.content}
                      </pre>
                    </div>
                    <Button
                      size="small"
                      type="primary"
                      style={{ marginTop: 6 }}
                      onClick={() => onApplyCode(msg.content)}
                      disabled={readonly}
                    >
                      应用到编辑器
                    </Button>
                  </div>
                </div>
              )}
            </div>
          ))
        )}
        {generating && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
            <RobotOutlined style={{ color: '#722ed1' }} />
            <Spin size="small" />
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              生成中...
            </Typography.Text>
          </div>
        )}
      </div>

      {/* Input area */}
      <div
        style={{
          padding: '8px 10px',
          borderTop: '1px solid #f0f0f0',
          flexShrink: 0,
        }}
      >
        <Input.TextArea
          rows={3}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="描述调整需求... (Ctrl+Enter 发送)"
          disabled={generating || readonly}
          style={{ resize: 'none', fontSize: 13 }}
        />
        <div style={{ marginTop: 6, display: 'flex', justifyContent: 'flex-end' }}>
          <Button
            type="primary"
            icon={<SendOutlined />}
            loading={generating}
            onClick={handleSend}
            disabled={readonly}
            size="small"
          >
            发送
          </Button>
        </div>
      </div>
    </div>
  );
};

export default StrategyChatPanel;
