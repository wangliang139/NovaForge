import { MarketType } from '@/global.types';
import { PositionSide } from '@/services/gateway/account';
import utils from '@/utils';

export type TradeSideRecord = {
  symbol: string;
  side?: string;
  isBuy?: boolean;
};

/** 订单/成交方向文案：现货为买入/卖出，合约为开多/平多/开空/平空 */
export const getTradeSideLabel = (record: TradeSideRecord): string => {
  const formtedSymbol = utils.market.parseSymbol(record.symbol);
  if (formtedSymbol.type === MarketType.Future) {
    if (record.side === PositionSide.Long) {
      return record.isBuy ? '开多' : '平多';
    }
    if (record.side === PositionSide.Short) {
      return record.isBuy ? '平空' : '开空';
    }
  } else {
    return record.isBuy ? '买入' : '卖出';
  }
  return '-';
};

/** 合约开仓成交（开多 / 开空），此类成交无已实现盈亏展示意义 */
export const isOpeningTradeSide = (record: TradeSideRecord): boolean => {
  const formtedSymbol = utils.market.parseSymbol(record.symbol);
  if (formtedSymbol.type !== MarketType.Future) {
    return false;
  }
  if (record.side === PositionSide.Long && record.isBuy) {
    return true;
  }
  if (record.side === PositionSide.Short && record.isBuy === false) {
    return true;
  }
  return false;
};

export const getTradeSideColor = (record: TradeSideRecord): string => {
  if (record.side === PositionSide.Long) {
    return record.isBuy ? 'green' : 'orange';
  }
  if (record.side === PositionSide.Short) {
    return record.isBuy ? 'orange' : 'red';
  }
  return 'default';
};
