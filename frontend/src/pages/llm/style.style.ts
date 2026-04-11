import { createStyles } from 'antd-style';

const useStyles = createStyles(({ token }) => {
  return {
    required: {
      '&::before': {
        display: 'inline-block',
        marginInlineEnd: '4px',
        color: token.colorError,
        fontSize: '14px',
        lineHeight: '1',
        fontFamily: 'SimSun, sans-serif',
        content: `"*"`,
      },
    },
    lastChildNoBottomMargin: {
      '*:last-child': {
        marginBottom: 0,
      },
    },
    proFromLayoutNested: {
      '&.ant-form-item-horizontal': {
        flexDirection: 'row',
        '.ant-form-item': {
          '.ant-form-item-row': {
            flexDirection: 'row',
          },
        },
      },
    },
    clickable: {
      cursor: 'pointer',
      color: token.colorPrimary,
      '&:hover': {
        color: token.colorPrimaryHover,
      },
      '&:active': {
        color: token.colorPrimaryActive,
      },
    },
  };
});

export default useStyles;
