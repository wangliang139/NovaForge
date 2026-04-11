import { CodeEditor } from '@/components';
import {
  GenerateStrategyResponse,
  generateStrategy,
} from '@/services/gateway/strategy';
import { RobotOutlined, UserOutlined } from '@ant-design/icons';
import { Button, Card, Empty, Input, List, Modal, Space, Typography, message } from 'antd';
import { useRef, useState } from 'react';

const { TextArea } = Input;

type AIStrategyGeneratorProps = {
  readonly?: boolean;
  onApplyCode: (code: string) => void;
};

type ChatMessage = {
  role: 'user' | 'assistant';
  content: string;
  id: string;
  createdAt: number;
};

const AIStrategyGenerator = ({ readonly = false, onApplyCode }: AIStrategyGeneratorProps) => {
  const [prompt, setPrompt] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [streamingCode, setStreamingCode] = useState('');
  const [generating, setGenerating] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const codeBufferRef = useRef('');

  const handleGenerate = async () => {
    if (readonly) {
      return;
    }
    const currentPrompt = prompt.trim();
    if (!currentPrompt) {
      message.warning('请先输入策略需求描述');
      return;
    }

    const userMessage: ChatMessage = {
      id: `msg_user_${Date.now()}`,
      role: 'user',
      content: currentPrompt,
      createdAt: Date.now(),
    };
    setMessages([userMessage]);
    setPrompt('');
    setStreamingCode('');
    codeBufferRef.current = '';
    setGenerating(true);

    try {
      const resp = (await generateStrategy({ query: currentPrompt })) as GenerateStrategyResponse | undefined;
      if (!resp?.content) {
        throw new Error('未生成有效代码');
      }

      codeBufferRef.current = resp.content;
      setStreamingCode(resp.content);
      setMessages([
        userMessage,
        {
          id: `msg_ai_${Date.now()}`,
          role: 'assistant',
          content: resp.content,
          createdAt: Date.now(),
        },
      ]);
      setPreviewOpen(true);
    } catch (error: any) {
      message.error(error?.message || 'AI 生成失败，请稍后重试');
    } finally {
      setGenerating(false);
    }
  };

  const resetConversation = () => {
    setMessages([]);
    setStreamingCode('');
    setPrompt('');
    setPreviewOpen(false);
    setGenerating(false);
    codeBufferRef.current = '';
  };

  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Card size="small" title="本轮输入与输出" extra={<Typography.Text type="secondary">当前仅支持单轮生成</Typography.Text>}>
        {messages.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description="输入策略需求，例如：做一个 15m RSI + EMA 趋势跟随策略，支持 BTC/USDT:FUTURE"
          />
        ) : (
          <List
            dataSource={messages}
            renderItem={(item) => (
              <List.Item>
                <List.Item.Meta
                  avatar={item.role === 'assistant' ? <RobotOutlined /> : <UserOutlined />}
                  title={item.role === 'assistant' ? 'AI' : '你'}
                  description={
                    <Typography.Paragraph style={{ marginBottom: 0, whiteSpace: 'pre-wrap' }} ellipsis={item.role === 'assistant' ? { rows: 4 } : false}>
                      {item.content}
                    </Typography.Paragraph>
                  }
                />
              </List.Item>
            )}
          />
        )}
      </Card>

      <Card
        size="small"
        title="需求输入"
        extra={
          <Button onClick={resetConversation} disabled={generating || readonly}>
            清空会话
          </Button>
        }
      >
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <TextArea
            rows={5}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="描述你希望 AI 生成的策略逻辑、交易对、周期、风控规则等"
            disabled={generating || readonly}
          />
          <Space>
            <Button type="primary" loading={generating} onClick={handleGenerate} disabled={readonly}>
              {generating ? '生成中...' : '生成策略代码'}
            </Button>
          </Space>
        </Space>
      </Card>

      {(generating || streamingCode) && (
        <Card size="small" title="实时生成预览">
          <CodeEditor language="javascript" height="260px" value={streamingCode} readonly />
        </Card>
      )}

      <Modal
        title="AI 生成代码预览"
        open={previewOpen}
        width={1000}
        destroyOnHidden
        onCancel={() => setPreviewOpen(false)}
        footer={[
          <Button key="close" onClick={() => setPreviewOpen(false)}>
            关闭
          </Button>,
          <Button
            key="apply"
            type="primary"
            disabled={readonly}
            onClick={() => {
              onApplyCode(codeBufferRef.current);
              setPreviewOpen(false);
            }}
          >
            应用到编辑器
          </Button>,
        ]}
      >
        <CodeEditor language="javascript" height="500px" value={codeBufferRef.current} readonly />
      </Modal>
    </Space>
  );
};

export default AIStrategyGenerator;
