import { createStyles } from 'antd-style';

const useStyles = createStyles(({ token }) => {
  return {
    codeEditor: {
      // 关键：阻止横向滚动链传递到页面（避免触发浏览器回退）
      overscrollBehaviorX: 'contain',

      '.cm-editor': {
        borderRadius: 'inherit',
        overscrollBehaviorX: 'contain',
      },
      '.cm-scroller': {
        borderRadius: 'inherit',
        overscrollBehaviorX: 'contain',
        overscrollBehaviorY: 'contain',
      },
      '.cm-gutters': {
        borderRadius: 'inherit',
      },
    },
  };
});

export default useStyles;
