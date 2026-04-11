import { CopyOutlined } from '@ant-design/icons';
import { Button, message } from 'antd';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { atomDark } from 'react-syntax-highlighter/dist/esm/styles/prism';

export type CodeBlockProps = {
  value?: string;
  language: string;
  copyable?: boolean;
  showLineNumbers?: boolean;
  height?: string;
  width?: string;
};

const CodeBlock: React.FC<CodeBlockProps> = ({
  value,
  language,
  copyable = true,
  showLineNumbers = false,
  height,
  width,
}) => {
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(value ?? '');
      message.success('已复制！');
    } catch (err) {
      message.error(`复制失败：${err}`);
    }
  };

  return (
    <div style={{ position: 'relative' }}>
      <Button
        hidden={!copyable || !value}
        icon={<CopyOutlined />}
        onClick={handleCopy}
        size="small"
        type="text"
        style={{
          position: 'absolute',
          right: '18px',
          top: '14px',
          background: 'rgba(255,255,255,0.1)',
          color: '#4096ff',
          cursor: 'pointer',
          transition: 'all 0.3s ease',
          border: '1px solid rgba(255,255,255,0.3)',
          borderRadius: '6px',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.background = 'rgba(255,255,255,0.2)';
          e.currentTarget.style.color = '#69b1ff';
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.background = 'rgba(255,255,255,0.1)';
          e.currentTarget.style.color = '#4096ff';
        }}
      />

      <SyntaxHighlighter
        language={language}
        children={value?.trim() ?? ''}
        style={atomDark}
        showLineNumbers={showLineNumbers}
        customStyle={{
          height: height,
          width: width,
          borderRadius: '6px',
          fontSize: '0.9rem',
          padding: '1rem',
        }}
      />
    </div>
  );
};

export default CodeBlock;
