import { CopyOutlined } from '@ant-design/icons';
import { javascript } from '@codemirror/lang-javascript';
import { json } from '@codemirror/lang-json';
import CodeMirror, { BasicSetupOptions } from '@uiw/react-codemirror';
import { Button, message } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import './CodeEditor.less';
import useStyles from './style.style';

export type CodeEditorProps = {
  value?: string;
  language?: string;
  height?: string;
  width?: string;
  placeholder?: string;
  readonly?: boolean;
  copyable?: boolean;

  basicSetup?: BasicSetupOptions;

  style?: React.CSSProperties;

  onChange?: (value: string) => void;
  onBlur?: (value: string) => void;
};

const defaultStyle: React.CSSProperties = {
  fontFamily: '"Fira Code", monospace',
  fontSize: 12,
  borderRadius: 6,
};

export default function CodeEditor({
  value,
  language = 'json',
  width,
  height,
  placeholder,
  readonly = false,
  copyable = true,
  basicSetup,
  onChange,
  onBlur,
  style,
}: CodeEditorProps) {
  const [code, setCode] = useState(value || '');
  const { styles } = useStyles();

  useEffect(() => {
    setCode(value || '');
  }, [value]);

  // 根据 readonly 设置 basicSetup 默认值:
  const computedBasicSetup = useMemo(() => {
    if (readonly) {
      return {
        ...{
          lineNumbers: false,
          highlightActiveLine: false,
          highlightActiveLineGutter: false,
          foldGutter: false,
        },
        ...basicSetup,
      };
    } else {
      return {
        ...{
          lineNumbers: true,
          highlightActiveLine: true,
          highlightActiveLineGutter: true,
          foldGutter: true,
        },
        ...basicSetup,
      };
    }
  }, [basicSetup, readonly]);

  const extensions = useMemo(() => {
    switch (language) {
      case 'json':
        return [json()];
      case 'javascript':
        return [javascript()];
      default:
        return [];
    }
  }, [language]);

  const mergedStyle = useMemo(() => {
    return {
      ...defaultStyle,
      ...style,
    };
  }, [style]);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code ?? '');
      message.success('已复制！');
    } catch (err) {
      message.error(`复制失败：${err}`);
    }
  };

  return (
    <div
      className={styles.codeEditor}
      style={{ position: 'relative' }}
      onKeyDownCapture={(e) => {
        if (e.key === 'Escape') {
          e.stopPropagation();
        }
      }}
    >
      <Button
        hidden={!copyable || !code}
        icon={<CopyOutlined />}
        onClick={handleCopy}
        size="small"
        type="text"
        style={{
          position: 'absolute',
          right: '12px',
          top: '10px',
          background: 'rgba(255,255,255,0.1)',
          color: '#4096ff',
          cursor: 'pointer',
          transition: 'all 0.3s ease',
          border: '1px solid rgba(255,255,255,0.3)',
          borderRadius: '6px',
          zIndex: 1,
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

      <CodeMirror
        value={code}
        height={height || '400px'}
        width={width}
        extensions={extensions}
        editable={!readonly}
        readOnly={readonly}
        onChange={(value) => {
          setCode(value);
          if (onChange) {
            onChange(value);
          }
        }}
        onUpdate={(viewUpdate) => {
          if (!viewUpdate.view.hasFocus) {
            if (onBlur) {
              onBlur(code);
            }
          }
        }}
        placeholder={placeholder}
        basicSetup={computedBasicSetup}
        style={mergedStyle}
        theme="dark"
      />
    </div>
  );
}
