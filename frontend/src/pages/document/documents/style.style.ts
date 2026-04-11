import { createStyles } from 'antd-style';

const useStyles = createStyles(({ token }) => {
  return {
    htmlContent: {
      width: '100%',
      overflowWrap: 'break-word',
      wordBreak: 'break-all',
      img: {
        maxWidth: '100% !important',
        height: 'auto !important',
        display: 'block',
        margin: '8px auto',
        borderRadius: '4px',
      },
      video: {
        maxWidth: '100% !important',
        height: 'auto !important',
        display: 'block',
        margin: '8px auto',
        borderRadius: '4px',
      },
    },
    contentCard: {
      marginTop: 20,
      '.ant-card-body': {
        padding: 0,
      },
    },
  };
});

export default useStyles;
