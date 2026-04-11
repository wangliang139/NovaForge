import { createStyles } from 'antd-style';

const useStyles = createStyles(({ token }) => {
  return {
    guideCard: {
      '.ant-card-body': {
        padding: '24px',
      },
    },
    markdownContent: {
      lineHeight: '1.8',
      fontSize: '15px',
      color: token.colorText,

      // 标题样式
      'h1': {
        fontSize: '32px',
        fontWeight: '600',
        marginTop: '24px',
        marginBottom: '16px',
        paddingBottom: '12px',
        borderBottom: `2px solid ${token.colorBorder}`,
      },
      'h2': {
        fontSize: '28px',
        fontWeight: '600',
        marginTop: '32px',
        marginBottom: '16px',
        paddingBottom: '8px',
        borderBottom: `1px solid ${token.colorBorderSecondary}`,
      },
      'h3': {
        fontSize: '24px',
        fontWeight: '600',
        marginTop: '24px',
        marginBottom: '12px',
      },
      'h4': {
        fontSize: '20px',
        fontWeight: '600',
        marginTop: '20px',
        marginBottom: '12px',
      },
      'h5': {
        fontSize: '18px',
        fontWeight: '600',
        marginTop: '16px',
        marginBottom: '10px',
      },
      'h6': {
        fontSize: '16px',
        fontWeight: '600',
        marginTop: '16px',
        marginBottom: '10px',
      },

      // 段落样式
      'p': {
        marginBottom: '16px',
      },

      // 列表样式
      'ul, ol': {
        paddingLeft: '24px',
        marginBottom: '16px',
      },
      'li': {
        marginBottom: '8px',
      },

      // 代码块样式
      'pre': {
        backgroundColor: token.colorBgLayout,
        border: `1px solid ${token.colorBorder}`,
        borderRadius: token.borderRadius,
        padding: '16px',
        overflow: 'auto',
        marginBottom: '16px',
        fontSize: '14px',
      },
      'code': {
        backgroundColor: token.colorBgLayout,
        padding: '2px 6px',
        borderRadius: '4px',
        fontSize: '14px',
        fontFamily: 'Consolas, Monaco, "Courier New", monospace',
      },
      'pre code': {
        backgroundColor: 'transparent',
        padding: '0',
      },

      // 表格样式
      'table': {
        width: '100%',
        borderCollapse: 'collapse',
        marginBottom: '16px',
        fontSize: '14px',
      },
      'th, td': {
        border: `1px solid ${token.colorBorder}`,
        padding: '8px 12px',
        textAlign: 'left',
      },
      'th': {
        backgroundColor: token.colorBgLayout,
        fontWeight: '600',
      },
      'tr:nth-child(even)': {
        backgroundColor: token.colorBgContainer,
      },

      // 引用样式
      'blockquote': {
        borderLeft: `4px solid ${token.colorPrimary}`,
        paddingLeft: '16px',
        marginLeft: '0',
        marginBottom: '16px',
        color: token.colorTextSecondary,
        fontStyle: 'italic',
      },

      // 链接样式
      'a': {
        color: token.colorPrimary,
        textDecoration: 'none',
        '&:hover': {
          textDecoration: 'underline',
        },
      },

      // 分隔线样式
      'hr': {
        border: 'none',
        borderTop: `1px solid ${token.colorBorder}`,
        marginTop: '24px',
        marginBottom: '24px',
      },

      // 图片样式
      'img': {
        maxWidth: '100%',
        height: 'auto',
        borderRadius: token.borderRadius,
      },

      // Mermaid 图表样式
      '.mermaid': {
        backgroundColor: token.colorBgContainer,
        padding: '24px',
        borderRadius: token.borderRadius,
        marginTop: '16px',
        marginBottom: '24px',
        border: `1px solid ${token.colorBorder}`,
        overflow: 'auto',
        textAlign: 'center',
        
        // Mermaid SVG 样式
        'svg': {
          maxWidth: '100%',
          height: 'auto',
        },
      },
    },
  };
});

export default useStyles;
