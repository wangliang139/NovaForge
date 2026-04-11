import { createStyles } from 'antd-style';

const useStyles = createStyles(({ token }) => {
  return {
    marketTabs: {
      '.ant-tabs-nav': {
        marginBottom: 0,
      },
    }
  };
});

export default useStyles;
