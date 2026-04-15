import { Exchange, MarketType } from '@/global.types';
import type { PlaceOrderParams } from '@/pages/exchange/market/types';
import { Balance, OrderType, PositionSide } from '@/services/gateway/account';
import { Ticker } from '@/services/gateway/market';
import utils from '@/utils';
import { InfoCircleOutlined } from '@ant-design/icons';
import { Button, InputNumber, Segmented, Slider, Space, Tooltip, Typography } from 'antd';
import React, { useCallback, useEffect, useState } from 'react';

export type SpotPlaceOrderFormProps = {
  loading: boolean;
  exchange: Exchange;
  symbolName: string;
  pricePrecision: number;
  volumePrecision: number;
  balance: Balance | null;
  ticker?: Ticker;
  onPlaceOrder: (params: PlaceOrderParams) => void;
  /** 下单成功后递增，用于重置数量/金额 */
  resetKey?: number;
};

const SpotPlaceOrderForm: React.FC<SpotPlaceOrderFormProps> = ({
  loading,
  exchange,
  symbolName,
  pricePrecision,
  volumePrecision,
  balance,
  ticker,
  onPlaceOrder,
  resetKey,
}) => {
  const symbolParsed = utils.market.parseSymbol(symbolName);

  const [orderSide, setOrderSide] = useState<'buy' | 'sell'>('buy');
  const [orderType, setOrderType] = useState<OrderType>(OrderType.Market);
  const [orderPrice, setOrderPrice] = useState<string>('');
  const [orderQty, setOrderQty] = useState<string>('');
  const [orderBaseQty, setOrderBaseQty] = useState<string>('');
  const [orderQtyPercent, setOrderQtyPercent] = useState<number>(0);
  const [priceTouched, setPriceTouched] = useState(false);

  const getAvailableAmount = useCallback(
    (code: string): number => {
      const walletTypes = utils.market.getWalletTypes(exchange, MarketType.Spot);
      const assets = balance?.assets.filter((a) => walletTypes.includes(a.walletType));
      if (!assets) return 0;
      let available = 0;
      for (const asset of assets) {
        if (asset.code === code) {
          const total = utils.math.toSafeNumber(asset.balance);
          const locked = utils.math.toSafeNumber(asset.locked);
          available += total - locked;
        }
      }
      return available;
    },
    [balance, exchange],
  );

  const getMaxNotional = useCallback((): number => {
    if (!balance) return 0;
    const available = getAvailableAmount(symbolParsed.quote);
    return available > 0 ? available : 0;
  }, [balance, symbolParsed.quote, getAvailableAmount, exchange]);

  // 下单成功后，父组件会递增 resetKey，触发这里清空数量/金额相关字段
  useEffect(() => {
    if (resetKey === undefined) return;
    setOrderQty('');
    setOrderBaseQty('');
    setOrderQtyPercent(0);
  }, [resetKey]);

  useEffect(() => {
    if (orderType !== OrderType.Limit) return;
    if (priceTouched) return;
    if (orderPrice) return;
    if (!ticker) return;
    const last = utils.math.toSafeNumber(ticker.lastPrice);
    if (last <= 0) return;
    setOrderPrice(String(last));
  }, [orderType, orderPrice, ticker, priceTouched]);

  useEffect(() => {
    if (orderSide === 'buy') {
      const qtyNum = utils.math.toSafeNumber(orderQty);
      const maxNotional = getMaxNotional();
      if (maxNotional > 0 && qtyNum > 0) {
        const pct = Math.max(
          0,
          Math.min(100, Math.round((qtyNum / maxNotional) * 100)),
        );
        setOrderQtyPercent(pct);
      } else {
        setOrderQtyPercent(0);
      }
    } else {
      const qtyNum = utils.math.toSafeNumber(orderBaseQty);
      const maxBase = getAvailableAmount(symbolParsed.base);
      if (maxBase > 0 && qtyNum > 0) {
        const pct = Math.max(
          0,
          Math.min(100, Math.round((qtyNum / maxBase) * 100)),
        );
        setOrderQtyPercent(pct);
      } else {
        setOrderQtyPercent(0);
      }
    }
  }, [
    orderSide,
    orderQty,
    orderBaseQty,
    getMaxNotional,
    getAvailableAmount,
    symbolParsed.base,
  ]);

  const formatVolume = useCallback(
    (value: string | number | null | undefined, empty: string = '--') =>
      utils.math.formatByPrecision(value, volumePrecision, empty),
    [volumePrecision],
  );

  const renderFooter = useCallback(() => {
    const priceForCalc =
      orderType === OrderType.Limit && orderPrice
        ? utils.math.toSafeNumber(orderPrice)
        : utils.math.toSafeNumber(ticker?.lastPrice);

    const orderNotional = utils.math.toSafeNumber(orderQty);
    const buyAmountBase =
      orderNotional > 0 && priceForCalc > 0 ? orderNotional / priceForCalc : 0;

    const baseQtyNum = utils.math.toSafeNumber(orderBaseQty);
    const sellAmountBase = baseQtyNum > 0 ? baseQtyNum : 0;
    const sellQuoteProceeds =
      sellAmountBase > 0 && priceForCalc > 0 ? sellAmountBase * priceForCalc : 0;

    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <Button
          type="primary"
          loading={loading}
          danger={orderSide === 'sell'}
          onClick={() => {
            const finalPrice =
              orderType === OrderType.Limit && orderPrice
                ? utils.math.toSafeNumber(orderPrice)
                : utils.math.toSafeNumber(ticker?.lastPrice);
            const isBuy = orderSide === 'buy';
            let baseQtyVal = 0;
            let notionalStr = '0';

            if (isBuy) {
              const notionalNum = utils.math.toSafeNumber(orderQty);
              baseQtyVal =
                finalPrice > 0 && notionalNum > 0
                  ? notionalNum / finalPrice
                  : 0;
              notionalStr = orderQty || '0';
            } else {
              const qtyNum = utils.math.toSafeNumber(orderBaseQty);
              baseQtyVal = qtyNum;
              const notionalNum =
                finalPrice > 0 && qtyNum > 0 ? qtyNum * finalPrice : 0;
              notionalStr = notionalNum ? String(notionalNum) : '0';
            }

            onPlaceOrder({
              side: PositionSide.Long,
              isBuy,
              orderType,
              price: String(finalPrice || 0),
              baseQty: String(baseQtyVal || 0),
              notional: notionalStr,
              leverage: 1,
            });
          }}
        >
          下单
        </Button>
        <div style={{ fontSize: 11, color: '#8c8c8c', lineHeight: 1.4 }}>
          {orderSide === 'buy' ? (
            <>
              <div>
                可买{' '}
                {buyAmountBase > 0
                  ? formatVolume(buyAmountBase)
                  : '--'}{' '}
                {symbolParsed.base}
              </div>
            </>
          ) : (
            <>
              <div>
                可卖{' '}
                {sellQuoteProceeds > 0
                  ? utils.math.formatByPrecision(sellQuoteProceeds, pricePrecision, '--')
                  : '--'}{' '}
                {symbolParsed.quote}
              </div>
            </>
          )}
        </div>
      </div>
    );
  }, [
    loading,
    symbolParsed,
    ticker,
    orderType,
    orderPrice,
    orderQty,
    orderBaseQty,
    orderSide,
    onPlaceOrder,
    formatVolume,
    pricePrecision,
  ]);

  return (
    <>
      <div style={{ flex: 1, overflowY: 'auto' }}>
        <Space direction="vertical" style={{ width: '100%' }} size="small">
          <div>
            <Segmented
              value={orderSide}
              onChange={(v) => setOrderSide(v as 'buy' | 'sell')}
              options={[
                { label: '买入', value: 'buy' },
                { label: '卖出', value: 'sell' },
              ]}
              block
            />
          </div>

          <div>
            <div style={{ fontSize: 12, color: '#8c8c8c', marginBottom: 4 }}>订单类型</div>
            <Segmented
              value={orderType}
              onChange={(v) => {
                const next = v as OrderType;
                setOrderType(next);
                if (next === OrderType.Limit && !orderPrice && ticker) {
                  const last = utils.math.toSafeNumber(ticker.lastPrice);
                  if (last > 0) {
                    setOrderPrice(String(last));
                  }
                }
              }}
              options={[
                { label: '市价', value: OrderType.Market },
                { label: '限价', value: OrderType.Limit },
              ]}
              block
            />
          </div>

          {orderType === OrderType.Limit && (
            <div>
              <div style={{ fontSize: 12, color: '#8c8c8c', marginBottom: 4 }}>价格</div>
              <InputNumber
                value={orderPrice ? Number(orderPrice) : undefined}
                onChange={(v) => {
                  setPriceTouched(true);
                  setOrderPrice(v ? String(v) : '');
                }}
                placeholder="0.00"
                style={{ width: '100%' }}
                precision={pricePrecision}
              />
            </div>
          )}

          <div>
            {orderSide === 'buy' ? (
              <>
                <div style={{ fontSize: 12, color: '#8c8c8c', marginBottom: 4 }}>
                  成交额
                  <Tooltip title="成交额为当前价格乘以数量">
                    <InfoCircleOutlined style={{ marginLeft: 4, color: '#faad14' }} />
                  </Tooltip>
                </div>
                <InputNumber
                  controls={false}
                  min={0}
                  precision={pricePrecision}
                  value={orderQty ? Number(orderQty) : undefined}
                  suffix={
                    <Typography.Text type="secondary">
                      {symbolParsed.quote}
                    </Typography.Text>
                  }
                  onChange={(v) => {
                    const str = v ? String(v) : '';
                    setOrderQty(str);

                    const qtyNum = utils.math.toSafeNumber(str);
                    const maxNotional = getMaxNotional();
                    if (maxNotional > 0 && qtyNum > 0) {
                      const pct = Math.max(
                        0,
                        Math.min(100, Math.round((qtyNum / maxNotional) * 100)),
                      );
                      setOrderQtyPercent(pct);
                    } else {
                      setOrderQtyPercent(0);
                    }
                  }}
                  placeholder="0"
                  style={{ width: '100%' }}
                />
                <div style={{ marginTop: 4, marginLeft: 4, marginRight: 4 }}>
                  <Slider
                    min={0}
                    max={100}
                    step={1}
                    value={orderQtyPercent}
                    included={false}
                    tooltip={{ formatter: (value) => `${value}%` }}
                    marks={{ 0: '', 25: '', 50: '', 75: '', 100: '' }}
                    onChange={(pct) => {
                      const percent = Array.isArray(pct) ? pct[0] : pct;
                      setOrderQtyPercent(percent);
                      const maxNotional = getMaxNotional();
                      if (maxNotional <= 0) return;
                      const qty = (maxNotional * percent) / 100;
                      setOrderQty(qty > 0 ? String(qty) : '');
                    }}
                  />
                </div>
              </>
            ) : (
              <>
                <div style={{ fontSize: 12, color: '#8c8c8c', marginBottom: 4 }}>
                  数量
                </div>
                <InputNumber
                  controls={false}
                  min={0}
                  precision={volumePrecision}
                  value={orderBaseQty ? Number(orderBaseQty) : undefined}
                  suffix={
                    <Typography.Text type="secondary">
                      {symbolParsed.base}
                    </Typography.Text>
                  }
                  onChange={(v) => {
                    const str = v ? String(v) : '';
                    setOrderBaseQty(str);
                    const qtyNum = utils.math.toSafeNumber(str);
                    const maxBase = getAvailableAmount(symbolParsed.base);
                    if (maxBase > 0 && qtyNum > 0) {
                      const pct = Math.max(
                        0,
                        Math.min(100, Math.round((qtyNum / maxBase) * 100)),
                      );
                      setOrderQtyPercent(pct);
                    } else {
                      setOrderQtyPercent(0);
                    }
                  }}
                  placeholder="0"
                  style={{ width: '100%' }}
                />
                <div style={{ marginTop: 4, marginLeft: 4, marginRight: 4 }}>
                  <Slider
                    min={0}
                    max={100}
                    step={1}
                    value={orderQtyPercent}
                    included={false}
                    tooltip={{ formatter: (value) => `${value}%` }}
                    marks={{ 0: '', 25: '', 50: '', 75: '', 100: '' }}
                    onChange={(pct) => {
                      const percent = Array.isArray(pct) ? pct[0] : pct;
                      setOrderQtyPercent(percent);
                      const maxBase = getAvailableAmount(symbolParsed.base);
                      if (maxBase <= 0) return;
                      const qty = (maxBase * percent) / 100;
                      setOrderBaseQty(qty > 0 ? String(qty) : '');
                    }}
                  />
                </div>
              </>
            )}
          </div>
          <div style={{ fontSize: 12, color: '#8c8c8c', lineHeight: 1.4 }}>
            <Space>
              可用
              {
                utils.math.formatByPrecision(
                  getAvailableAmount(orderSide === 'buy' ? symbolParsed.quote : symbolParsed.base),
                  orderSide === 'buy' ? pricePrecision : volumePrecision
                )
              }
              {orderSide === 'buy' ? symbolParsed.quote : symbolParsed.base}
            </Space>
          </div>
        </Space>
      </div>

      <div style={{ position: 'absolute', bottom: 0, left: 0, right: 0, padding: 12 }}>
        {renderFooter()}
      </div>
    </>
  );
};

export default SpotPlaceOrderForm;
