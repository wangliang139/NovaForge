import { OrderType, PositionSide } from '@/services/gateway/account';

export type PlaceOrderParams = {
  isBuy: boolean;
  side: PositionSide;
  orderType: OrderType;
  price: string;
  baseQty: string;
  notional: string;
  leverage: number;
};
