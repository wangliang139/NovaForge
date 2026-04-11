import { Exchange, MarketType } from '@/global.types';
import type { PlaceOrderParams } from '@/pages/exchange/market/types';
import type { Balance, Position } from '@/services/gateway/account';
import type { Ticker } from '@/services/gateway/market';
import utils from '@/utils';
import React from 'react';
import PerpPlaceOrderForm from './PerpPlaceOrderForm';
import SpotPlaceOrderForm from './SpotPlaceOrderForm';

export type PlaceOrderFormProps = {
  loading: boolean;
  symbolName: string;
  pricePrecision: number;
  volumePrecision: number;
  balance: Balance | null;
  positions: Position[];
  ticker?: Ticker;
  leverage?: number;
  leverageLoading?: boolean;
  accountId: string | null;
  exchange: Exchange;
  onLeverageChange?: (value: number) => void | Promise<void>;
  onPlaceOrder: (params: PlaceOrderParams) => void;
  /** 下单成功后递增，用于通知子表单重置数量/金额 */
  resetKey?: number;
};

const PlaceOrderForm: React.FC<PlaceOrderFormProps> = (props) => {
  const symbolParsed = utils.market.parseSymbol(props.symbolName);
  const isPerp = symbolParsed.type === MarketType.Future;

  if (isPerp) {
    return (
      <PerpPlaceOrderForm
        loading={props.loading}
        exchange={props.exchange}
        accountId={props.accountId}
        symbolName={props.symbolName}
        pricePrecision={props.pricePrecision}
        volumePrecision={props.volumePrecision}
        balance={props.balance}
        positions={props.positions}
        ticker={props.ticker}
        leverage={props.leverage}
        leverageLoading={props.leverageLoading}
        onLeverageChange={props.onLeverageChange}
        onPlaceOrder={props.onPlaceOrder}
        resetKey={props.resetKey}
      />
    );
  }

  return (
    <SpotPlaceOrderForm
      loading={props.loading}
      exchange={props.exchange}
      symbolName={props.symbolName}
      pricePrecision={props.pricePrecision}
      volumePrecision={props.volumePrecision}
      balance={props.balance}
      ticker={props.ticker}
      onPlaceOrder={props.onPlaceOrder}
      resetKey={props.resetKey}
    />
  );
};

export default PlaceOrderForm;
