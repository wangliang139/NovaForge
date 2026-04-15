import { PositionSide, WalletType } from '@/services/gateway/account';

type TagInfo = {
  text: string;
  color: string;
};

export const walletTypeLabelMap: Record<string, string> = {
  [WalletType.Unspecified]: '未指定',
  [WalletType.Fund]: '资金',
  [WalletType.Trade]: '交易',
  [WalletType.Spot]: '现货',
  [WalletType.Future]: '合约',
  [WalletType.Margin]: '杠杆',
};

export const walletTypeColorMap: Record<string, string> = {
  [WalletType.Unspecified]: 'default',
  [WalletType.Fund]: 'green',
  [WalletType.Trade]: 'purple',
  [WalletType.Spot]: 'blue',
  [WalletType.Future]: 'orange',
  [WalletType.Margin]: 'red',
};

export const sideLabelMap: Record<string, string> = {
  [PositionSide.Long]: '多',
  [PositionSide.Short]: '空',
};

export const sideColorMap: Record<string, string> = {
  [PositionSide.Long]: 'green',
  [PositionSide.Short]: 'red',
};

export const getWalletTypeLabel = (walletType?: string, withWalletSuffix = false): string => {
  const text = walletTypeLabelMap[walletType || ''] || walletType || '-';
  if (!withWalletSuffix || text === '-' || text === '未指定') {
    return text;
  }
  return `${text}`;
};

export const getWalletTypeTagInfo = (
  walletType?: string,
  options?: { withWalletSuffix?: boolean },
): TagInfo => ({
  text: getWalletTypeLabel(walletType, options?.withWalletSuffix),
  color: walletTypeColorMap[walletType || ''] || 'default',
});

export const getSideTagInfo = (side?: string): TagInfo => ({
  text: sideLabelMap[side || ''] || side || '-',
  color: sideColorMap[side || ''] || 'default',
});

export const walletTypeFilterOptions = [
  WalletType.Spot,
  WalletType.Future,
  WalletType.Fund,
  WalletType.Trade,
  WalletType.Margin,
].map((value) => ({
  text: getWalletTypeLabel(value),
  value,
}));

export const sideFilterOptions = [PositionSide.Long, PositionSide.Short].map((value) => ({
  text: getSideTagInfo(value).text,
  value,
}));
