import { PageContainer } from '@ant-design/pro-components';
import { Card, Spin, Typography } from 'antd';
import mermaid from 'mermaid';
import React, { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import rehypeSlug from 'rehype-slug';
import remarkGfm from 'remark-gfm';
import useStyles from './style.style';

const StrategyGuide: React.FC = () => {
  const { styles } = useStyles();
  const [loading, setLoading] = useState<boolean>(true);
  const [content, setContent] = useState<string>('');
  const [error, setError] = useState<string>('');
  const mermaidInitialized = useRef(false);

  const scrollToHash = (hash: string) => {
    if (!hash) {
      return;
    }

    const id = decodeURIComponent(hash.replace(/^#/, ''));
    const headings = Array.from(
      document.querySelectorAll<HTMLElement>('h1[id], h2[id], h3[id], h4[id], h5[id], h6[id]'),
    );
    const candidates = headings.filter((heading) => {
      if (heading.id === id) {
        return true;
      }

      if (!heading.id.startsWith(`${id}-`)) {
        return false;
      }

      return /^\d+$/.test(heading.id.slice(id.length + 1));
    });
    const isTocHeading = (heading: HTMLElement) =>
      Array.from(heading.querySelectorAll<HTMLAnchorElement>('a[href^="#"]')).some((anchor) => {
        const href = anchor.getAttribute('href');

        return href ? decodeURIComponent(href) === `#${id}` : false;
      });
    const element = candidates.find((heading) => !isTocHeading(heading)) ?? document.getElementById(id);

    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  };

  useEffect(() => {
    // 初始化 mermaid
    if (!mermaidInitialized.current) {
      mermaid.initialize({
        startOnLoad: true,
        theme: 'default',
        securityLevel: 'loose',
        fontFamily: 'ui-sans-serif, system-ui, sans-serif',
      });
      mermaidInitialized.current = true;
    }

    // 加载静态 markdown 文件
    fetch('/strategy-guide.md')
      .then((response) => {
        if (!response.ok) {
          throw new Error('Failed to load strategy guide');
        }
        return response.text();
      })
      .then((text) => {
        setContent(text);
        setLoading(false);
      })
      .catch((err) => {
        console.error('Failed to load guide:', err);
        setError('无法加载策略开发手册，请稍后重试');
        setLoading(false);
      });
  }, []);

  // 渲染 mermaid 图表后的回调
  useEffect(() => {
    if (content && !loading) {
      // 使用 setTimeout 确保 DOM 已渲染
      setTimeout(() => {
        mermaid.run({
          querySelector: '.mermaid',
        });
        
        // 如果 URL 中有锚点，自动滚动到对应位置
        const hash = window.location.hash;
        if (hash) {
          scrollToHash(hash);
        }
      }, 100);
    }
  }, [content, loading]);

  if (loading) {
    return (
      <PageContainer>
        <Card>
          <div style={{ textAlign: 'center', padding: '50px 0' }}>
            <Spin size="large" tip="加载中..." />
          </div>
        </Card>
      </PageContainer>
    );
  }

  if (error) {
    return (
      <PageContainer>
        <Card>
          <div style={{ textAlign: 'center', padding: '50px 0' }}>
            <Typography.Text type="danger">{error}</Typography.Text>
          </div>
        </Card>
      </PageContainer>
    );
  }

  return (
    <PageContainer
      title="策略开发手册"
      subTitle="自定义策略开发指南"
    >
      <Card className={styles.guideCard}>
        <div className={styles.markdownContent}>
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            rehypePlugins={[rehypeSlug, rehypeHighlight]}
            components={{
              code(props) {
                const { node, className, children, ...rest } = props;
                const match = /language-(\w+)/.exec(className || '');
                const language = match ? match[1] : '';
                const isInline = !className;

                // 处理 mermaid 代码块
                if (language === 'mermaid' && !isInline) {
                  return (
                    <div className="mermaid" style={{ textAlign: 'center', margin: '20px 0' }}>
                      {String(children).replace(/\n$/, '')}
                    </div>
                  );
                }

                // 其他代码块使用默认渲染
                return (
                  <code className={className} {...rest}>
                    {children}
                  </code>
                );
              },
              // 处理锚点链接跳转
              a(props) {
                const { href, children, ...rest } = props;
                
                // 如果是锚点链接（以 # 开头）
                if (href && href.startsWith('#')) {
                  return (
                    <a
                      {...rest}
                      href={href}
                      onClick={(e) => {
                        e.preventDefault();
                        scrollToHash(href);
                      }}
                    >
                      {children}
                    </a>
                  );
                }
                
                // 外部链接在新窗口打开
                if (href && (href.startsWith('http://') || href.startsWith('https://'))) {
                  return (
                    <a href={href} target="_blank" rel="noopener noreferrer" {...rest}>
                      {children}
                    </a>
                  );
                }
                
                // 其他链接正常处理
                return <a href={href} {...rest}>{children}</a>;
              },
            }}
          >
            {content}
          </ReactMarkdown>
        </div>
      </Card>
    </PageContainer>
  );
};

export default StrategyGuide;
