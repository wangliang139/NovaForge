import { createStyles } from 'antd-style';

/** 账户列表 ProList / CheckCard 标题区需在 flex 头里可收缩，否则长文案撑破卡片 */
const useAccountListStyles = createStyles(() => ({
  proListScope: {
    '.ant-pro-checkcard-header': {
      minWidth: 0,
    },
    '.ant-pro-checkcard-header-left': {
      flex: 1,
      minWidth: 0,
      overflow: 'hidden',
    },
    '.ant-pro-checkcard-title': {
      minWidth: 0,
      maxWidth: '100%',
    },
    '.ant-list-item-meta-title': {
      display: 'block',
      minWidth: 0,
      maxWidth: '100%',
      overflow: 'hidden',
    },
  },
}));

export default useAccountListStyles;
