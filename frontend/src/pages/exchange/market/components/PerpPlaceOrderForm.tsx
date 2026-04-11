import { Exchange, MarketType } from '@/global.types';
import type { PlaceOrderParams } from '@/pages/exchange/market/types';
import { Balance, OrderType, Position, PositionSide, estimateOrder } from '@/services/gateway/account';
import { Ticker } from '@/services/gateway/market';
import utils from '@/utils';
import { Button, Col, InputNumber, Modal, Row, Segmented, Slider, Space, Typography } from 'antd';
import React, { useCallback, useEffect, useState } from 'react';

export type PerpPlaceOrderFormProps = {
  loading: boolean;
  exchange: Exchange;
  symbolName: string;
  pricePrecision: number;
  volumePrecision: number;
  balance: Balance | null;
  positions: Position[];
  ticker?: Ticker;
  leverage?: number;
  leverageLoading?: boolean;
  accountId: string | null;
  onLeverageChange?: (value: number) => void | Promise<void>;
  onPlaceOrder: (params: PlaceOrderParams) => void;
  /** 下单成功后递增，用于重置数量/金额 */
  resetKey?: number;
};

const PerpPlaceOrderForm: React.FC<PerpPlaceOrderFormProps> = ({
  loading,
  exchange,
  accountId,
  symbolName,
  pricePrecision,
  volumePrecision,
  balance,
  positions,
  ticker,
  leverage,
  leverageLoading,
  onLeverageChange,
  onPlaceOrder,
  resetKey,
}) => {
  const normalizeSide = (side: string | null | undefined) => String(side || '').toLowerCase();

  const currentLongPos =
    positions.find(
      (p) => p.symbol === symbolName && normalizeSide(p.side) === PositionSide.Long,
    ) || null;
  const currentShortPos =
    positions.find(
      (p) => p.symbol === symbolName && normalizeSide(p.side) === PositionSide.Short,
    ) || null;

  const symbolParsed = utils.market.parseSymbol(symbolName);

  const [orderMode, setOrderMode] = useState<'open' | 'close'>('open');
  const [orderType, setOrderType] = useState<OrderType>(OrderType.Market);
  const [orderPrice, setOrderPrice] = useState<string>('');
  const [orderQty, setOrderQty] = useState<string>('');
  const [orderBaseQty, setOrderBaseQty] = useState<string>('');
  const [orderQtyPercent, setOrderQtyPercent] = useState<number>(0);
  const [priceTouched, setPriceTouched] = useState(false);
  const [leverageModalVisible, setLeverageModalVisible] = useState(false);
  const [tempLeverage, setTempLeverage] = useState<number | undefined>(leverage);
  const [showLeverageInput, setShowLeverageInput] = useState(false);
  const [estimatedLiqPriceLong, setEstimatedLiqPriceLong] = useState<string | null>(null);
  const [estimatedLiqPriceShort, setEstimatedLiqPriceShort] = useState<string | null>(null);

  const onOrderLeverageChange = useCallback(
    (v: number) => {
      const raw = Number.isFinite(v) ? v : 1;
      const clamped = Math.max(1, Math.min(100, Math.round(raw || 1)));
      if (onLeverageChange) {
        void onLeverageChange(clamped);
      }
    },
    [onLeverageChange],
  );

  const getAvailableAmount = useCallback(
    (code: string): number => {
      const walletTypes = utils.market.getWalletTypes(exchange, MarketType.Future);
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
    const availableMargin = getAvailableAmount(symbolParsed.quote);
    if (availableMargin <= 0) return 0;
    const lev = Math.max(1, Number(leverage) || 1);
    return availableMargin * lev;
  }, [balance, symbolParsed.quote, leverage, getAvailableAmount]);

  // 下单成功后，父组件会递增 resetKey，触发这里清空数量/金额相关字段
  useEffect(() => {
    if (resetKey === undefined) return;
    setOrderQty('');
    setOrderBaseQty('');
    setOrderQtyPercent(0);
  }, [resetKey]);

  useEffect(() => {
    if (!accountId || !symbolName) {
      setEstimatedLiqPriceLong(null);
      setEstimatedLiqPriceShort(null);
      return;
    }

    const notionalNum = utils.math.toSafeNumber(orderQty);
    if (!notionalNum || notionalNum <= 0) {
      setEstimatedLiqPriceLong(null);
      setEstimatedLiqPriceShort(null);
      return;
    }

    const priceForCalc =
      orderType === OrderType.Limit && orderPrice
        ? utils.math.toSafeNumber(orderPrice)
        : utils.math.toSafeNumber(ticker?.lastPrice);
    if (!priceForCalc || priceForCalc <= 0) {
      setEstimatedLiqPriceLong(null);
      setEstimatedLiqPriceShort(null);
      return;
    }

    const priceStr = String(priceForCalc);
    const notionalStr = String(notionalNum);

    let cancelled = false;
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const [longResp, shortResp] = await Promise.all([
            estimateOrder({
              accountId,
              symbol: symbolName,
              side: PositionSide.Long,
              isBuy: true,
              orderType: orderType,
              price: priceStr,
              notional: notionalStr,
              leverage,
            }),
            estimateOrder({
              accountId,
              symbol: symbolName,
              side: PositionSide.Short,
              isBuy: false,
              orderType: orderType,
              price: priceStr,
              notional: notionalStr,
              leverage,
            }),
          ]);
          if (cancelled) return;
          setEstimatedLiqPriceLong(longResp?.liquidationPrice || null);
          setEstimatedLiqPriceShort(shortResp?.liquidationPrice || null);
        } catch {
          if (cancelled) return;
          setEstimatedLiqPriceLong(null);
          setEstimatedLiqPriceShort(null);
        }
      })();
    }, 400);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [accountId, symbolName, orderType, orderPrice, orderQty, ticker?.lastPrice, leverage]);

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
    if (orderMode === 'open') {
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
      const longAmt = currentLongPos
        ? Math.abs(utils.math.toSafeNumber(currentLongPos.amount))
        : 0;
      const shortAmt = currentShortPos
        ? Math.abs(utils.math.toSafeNumber(currentShortPos.amount))
        : 0;
      const maxBase = Math.max(longAmt, shortAmt);
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
    orderMode,
    orderQty,
    orderBaseQty,
    getMaxNotional,
    currentLongPos,
    currentShortPos,
  ]);

  const formatPrice = useCallback(
    (value: string | number | null | undefined, empty: string = '--') =>
      utils.math.formatByPrecision(value, pricePrecision, empty),
    [pricePrecision],
  );

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
    const leverageNum = Number(leverage) || 0;
    const orderQtyNum = utils.math.toSafeNumber(orderQty);
    const maxNotional = getMaxNotional();
    const maxOpen =
      priceForCalc > 0 && maxNotional > 0 ? maxNotional / priceForCalc : 0;
    const marginForOrder =
      leverageNum > 0 && orderQtyNum > 0 ? orderQtyNum / leverageNum : 0;

    const renderOpenInfo = (liqPrice?: string | null) => (
      <>
        <div>
          可开 {formatVolume(maxOpen)} {symbolParsed.base}
        </div>
        <div>
          保证金 {marginForOrder > 0 ? formatPrice(marginForOrder) : '0'} {symbolParsed.quote}
        </div>
        <div>强平价格 {liqPrice ? formatPrice(liqPrice) : '--'}</div>
      </>
    );

    const renderCloseInfo = (pos?: Position | null) => {
      const rawAmount = pos ? utils.math.toSafeNumber(pos.amount) : 0;
      const absAmount = Math.abs(rawAmount);
      return (
        <div>
          可平 {absAmount > 0 ? formatVolume(absAmount) : '--'}{' '}
          {symbolParsed.base}
        </div>
      );
    };

    const longSide = orderMode === 'open' ? 'open_long' : 'close_long';
    const shortSide = orderMode === 'open' ? 'open_short' : 'close_short';

    return (
      <div style={{ display: 'flex', gap: 8 }}>
        <div
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            gap: 4,
          }}
        >
          <Button
            type="primary"
            loading={loading}
            danger={longSide === 'close_long'}
            onClick={() => {
              const finalPrice =
                orderType === OrderType.Limit && orderPrice
                  ? utils.math.toSafeNumber(orderPrice)
                  : utils.math.toSafeNumber(ticker?.lastPrice);
              const isOpen = orderMode === 'open';
              let baseQtyVal = 0;
              let notionalStr = '0';

              if (isOpen) {
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
                isBuy: orderMode === 'open',
                orderType,
                price: String(finalPrice || 0),
                baseQty: String(baseQtyVal || 0),
                notional: notionalStr,
                leverage: leverage ?? 0,
              });
            }}
          >
            {orderMode === 'open' ? '开多' : '平多'}
          </Button>
          <div style={{ fontSize: 11, color: '#8c8c8c', lineHeight: 1.4 }}>
            {orderMode === 'open'
              ? renderOpenInfo(
                estimatedLiqPriceLong ?? currentLongPos?.liquidationPrice,
              )
              : renderCloseInfo(currentLongPos)}
          </div>
        </div>
        <div
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            gap: 4,
          }}
        >
          <Button
            type="primary"
            loading={loading}
            danger={shortSide === 'open_short'}
            onClick={() => {
              const finalPrice =
                orderType === OrderType.Limit && orderPrice
                  ? utils.math.toSafeNumber(orderPrice)
                  : utils.math.toSafeNumber(ticker?.lastPrice);
              const isOpen = orderMode === 'open';
              let baseQtyVal = 0;
              let notionalStr = '0';

              if (isOpen) {
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
                side: PositionSide.Short,
                isBuy: orderMode !== 'open',
                orderType,
                price: String(finalPrice || 0),
                baseQty: String(baseQtyVal || 0),
                notional: notionalStr,
                leverage: leverage ?? 0,
              });
            }}
          >
            {orderMode === 'open' ? '开空' : '平空'}
          </Button>
          <div style={{ fontSize: 11, color: '#8c8c8c', lineHeight: 1.4 }}>
            {orderMode === 'open'
              ? renderOpenInfo(
                estimatedLiqPriceShort ?? currentShortPos?.liquidationPrice,
              )
              : renderCloseInfo(currentShortPos)}
          </div>
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
    orderMode,
    currentLongPos,
    currentShortPos,
    onPlaceOrder,
    leverage,
    pricePrecision,
    formatPrice,
    formatVolume,
    getAvailableAmount,
  ]);

  return (
    <>
      <div style={{ flex: 1, overflowY: 'auto' }}>
        <Space direction="vertical" style={{ width: '100%' }} size="small">
          <div>
            <Segmented
              value={orderMode}
              onChange={(v) => setOrderMode(v as 'open' | 'close')}
              options={[
                { label: '开仓', value: 'open' },
                { label: '平仓', value: 'close' },
              ]}
              block
            />
          </div>

          <div>
            <Row>
              <Col span={12} style={{ paddingRight: 4 }}>
                <Button disabled style={{ width: '100%' }}>全仓</Button>
              </Col>
              <Col span={12} style={{ paddingLeft: 4 }}>
                <Button
                  style={{ width: '100%' }}
                  loading={leverageLoading}
                  onClick={() => {
                    setTempLeverage(leverage ?? 1);
                    setLeverageModalVisible(true);
                  }}
                >
                  {leverage ? `${leverage}x` : '杠杆'}
                </Button>
              </Col>
            </Row>
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

          <div hidden={orderType !== OrderType.Limit}>
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

          <div>
            <div hidden={orderMode === 'close'}>
              <div style={{ fontSize: 12, color: '#8c8c8c', marginBottom: 4 }}>
                金额
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
            </div>
            <div hidden={orderMode === 'open'}>
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
                  const longAmt = currentLongPos
                    ? Math.abs(utils.math.toSafeNumber(currentLongPos.amount))
                    : 0;
                  const shortAmt = currentShortPos
                    ? Math.abs(utils.math.toSafeNumber(currentShortPos.amount))
                    : 0;
                  const maxBase = Math.max(longAmt, shortAmt);
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
                    const longAmt = currentLongPos
                      ? Math.abs(utils.math.toSafeNumber(currentLongPos.amount))
                      : 0;
                    const shortAmt = currentShortPos
                      ? Math.abs(utils.math.toSafeNumber(currentShortPos.amount))
                      : 0;
                    const maxBase = Math.max(longAmt, shortAmt);
                    if (maxBase <= 0) return;
                    const qty = (maxBase * percent) / 100;
                    setOrderBaseQty(qty > 0 ? String(qty) : '');
                  }}
                />
              </div>
            </div>
          </div>
          <div style={{ fontSize: 12, color: '#8c8c8c', lineHeight: 1.4 }}>
            <Space>
              可用
              {
                utils.math.formatByPrecision(
                  getAvailableAmount(symbolParsed.quote),
                  pricePrecision
                )
              }
              {symbolParsed.quote}
            </Space>
          </div>
        </Space>
      </div>

      <div style={{ position: 'absolute', bottom: 0, left: 0, right: 0, padding: 12 }}>
        {renderFooter()}
      </div>

      <Modal
        title="调整杠杆"
        open={leverageModalVisible}
        onCancel={() => setLeverageModalVisible(false)}
        confirmLoading={!!leverageLoading}
        footer={[
          <Button key="submit" type="primary" style={{ width: '100%' }} onClick={() => {
            if (tempLeverage === undefined || !Number.isFinite(tempLeverage)) {
              return;
            }
            onOrderLeverageChange(tempLeverage);
            setLeverageModalVisible(false);
          }}>
            确认
          </Button>,
        ]}
        centered
      >
        <div style={{ marginBottom: 12, fontSize: 12, color: '#8c8c8c' }}>杠杆</div>
        <Row justify="center">
          <div
            style={{
              width: '50%',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '8px 12px',
              borderRadius: 8,
              border: '1px solid #bfbfbf',
              marginBottom: 16,
            }}
          >
            <Button
              size="small"
              type="text"
              onClick={() => {
                setTempLeverage((prev) => {
                  const base = Number.isFinite(prev as number) && (prev as number) > 0 ? (prev as number) : 1;
                  return Math.max(1, base - 1);
                });
              }}
            >
              -
            </Button>
            <div style={{ fontSize: 18, fontWeight: 500 }} onClick={() => setShowLeverageInput(true)}>
              {showLeverageInput ? (
                <InputNumber
                  autoFocus
                  size="small"
                  style={{ width: 50 }}
                  variant="borderless"
                  controls={false}
                  min={1}
                  max={100}
                  step={1}
                  precision={0}
                  value={tempLeverage ? tempLeverage : leverage ? leverage : 1}
                  onBlur={() => setShowLeverageInput(false)}
                />
              ) : (
                <span>{tempLeverage ? `${tempLeverage}` : leverage ? `${leverage}` : '1'}</span>
              )}
              <span>x</span>
            </div>
            <Button
              size="small"
              type="text"
              onClick={() => {
                setTempLeverage((prev) => {
                  const base = Number.isFinite(prev as number) && (prev as number) > 0 ? (prev as number) : 1;
                  return Math.min(100, base + 1);
                });
              }}
            >
              +
            </Button>
          </div>
        </Row>
        <Slider
          min={1}
          max={100}
          step={1}
          value={Math.min(100, Math.max(1, tempLeverage ?? leverage ?? 1))}
          marks={{ 1: '1x', 25: '25x', 50: '50x', 75: '75x', 100: '100x' }}
          onChange={(v) => {
            const val = Array.isArray(v) ? v[0] : v;
            const clamped = Math.min(100, Math.max(1, val));
            setTempLeverage(clamped);
          }}
        />
        <div style={{ marginTop: 16, fontSize: 12, color: '#8c8c8c', lineHeight: 1.6 }}>
          <div>
            · 当前杠杆倍数将影响<Typography.Text type="warning" style={{ fontSize: 12, fontWeight: 500 }}> 持仓和新开仓订单 </Typography.Text>的风险水平。
          </div>
          <div>· 杠杆越高，强平风险越大，请根据自身风险承受能力谨慎选择。</div>
        </div>
      </Modal>
    </>
  );
};

export default PerpPlaceOrderForm;
