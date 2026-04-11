import bookAllIcon from '@/assets/icon/book_all.png';
import bookAsksIcon from '@/assets/icon/book_asks.png';
import bookBidsIcon from '@/assets/icon/book_bids.png';
import { MarketType } from '@/global.types';
import { Depth, DepthLevel, MarkPrice } from '@/services/gateway/market';
import utils from '@/utils';
import { ArrowDownOutlined, ArrowUpOutlined } from '@ant-design/icons';
import { Avatar, Empty, Flex, Segmented, Select, Space, Table, Typography, theme } from 'antd';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';

export type OrderbookProps = {
  /** 用于在切换交易对/交易所时重置组件内部状态 */
  resetKey: string;
  depth?: Depth;
  symbol: string;
  markPrice?: MarkPrice;
  pricePrecision: number;
};

export const Orderbook: React.FC<OrderbookProps> = ({
  resetKey,
  depth,
  symbol,
  markPrice,
  pricePrecision,
}) => {
  const { token } = theme.useToken();
  const [mode, setMode] = useState<'both' | 'bid' | 'ask'>('both');
  const [gradient, setGradient] = useState<1 | 10 | 100>(1);
  const prevLastPriceRef = useRef<number | null>(null);
  const [lastPriceDir, setLastPriceDir] = useState<'up' | 'down' | 'flat'>('flat');

  const quoteAsset = useMemo(() => {
    return utils.market.parseSymbol(symbol).quote;
  }, [symbol]);
  const isPerp = useMemo(() => {
    return utils.market.parseSymbol(symbol).type === MarketType.Future;
  }, [symbol]);

  const parseFinite = useCallback((value: unknown): number | null => {
    const n = Number(
      String(value ?? '')
        .replace(/,/g, '')
        .trim(),
    );
    if (!Number.isFinite(n)) return null;
    return n;
  }, []);

  const formatByPrecision = useCallback(
    (value: string | number | null | undefined, precision?: number, empty: string = '--') => {
      if (value === null || value === undefined) return empty;
      const rawStr = String(value).replace(/,/g, '').trim();
      if (!rawStr) return empty;
      const n = Number(rawStr);
      if (!Number.isFinite(n)) return empty;
      if (!Number.isFinite(precision as number) || (precision as number) < 0) return rawStr;
      return n.toFixed(precision as number);
    },
    [],
  );

  const formatPrice = useCallback(
    (value: string | number | null | undefined, empty: string = '--') =>
      formatByPrecision(value, pricePrecision, empty),
    [formatByPrecision, pricePrecision],
  );

  const shownMarkPrice = useMemo(() => {
    if (!isPerp) return '';
    const isZeroLike = (v: string) => {
      const raw = String(v ?? '')
        .replace(/,/g, '')
        .trim();
      if (!raw) return true;
      const n = Number(raw);
      if (!Number.isFinite(n)) return false;
      return n === 0;
    };

    const mp = String(markPrice?.markPrice ?? '').trim();
    if (mp && !isZeroLike(mp)) return mp;
    return '';
  }, [isPerp, markPrice?.markPrice]);

  useEffect(() => {
    setMode('both');
    setGradient(1);
    prevLastPriceRef.current = null;
    setLastPriceDir('flat');
  }, [resetKey]);

  const lastPriceColor = useMemo(() => {
    if (lastPriceDir === 'up') return '#52c41a';
    if (lastPriceDir === 'down') return '#ff4d4f';
    return '#8c8c8c';
  }, [lastPriceDir]);

  const monoPriceStyle = useMemo(
    () =>
      ({
        fontFamily:
          'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
        fontVariantNumeric: 'tabular-nums',
      } as React.CSSProperties),
    [],
  );

  const gradientOptions = useMemo(() => {
    const base = Math.pow(10, -pricePrecision);
    const fmt = (val: number, decimals: number) => val.toFixed(Math.max(0, Math.min(12, decimals)));
    const mk = (factor: 1 | 10 | 100) => {
      const step = base * factor;
      const stepDecimals = Math.max(0, pricePrecision - (factor === 1 ? 0 : factor === 10 ? 1 : 2));
      return {
        value: factor,
        label: fmt(step, stepDecimals),
        step,
        stepDecimals,
      };
    };
    return [mk(1), mk(10), mk(100)];
  }, [pricePrecision]);

  const aggregated = useMemo(() => {
    const rawBids = depth?.bids || [];
    const rawAsks = depth?.asks || [];

    const grad = gradientOptions.find((o) => o.value === gradient) ?? gradientOptions[0];
    const step = grad?.step || Math.pow(10, -pricePrecision);
    const aggPriceDecimals = grad?.stepDecimals ?? pricePrecision;
    const eps = 1e-12;
    const fmtBaseSize = (n: number) => n.toFixed(12);

    const aggregateSide = (levels: DepthLevel[], isBid: boolean): DepthLevel[] => {
      if (!levels || levels.length === 0) return [];
      const map = new Map<string, number>();
      for (const lv of levels) {
        const p = Number(
          String(lv?.price ?? '')
            .replace(/,/g, '')
            .trim(),
        );
        const s = Number(
          String(lv?.size ?? '')
            .replace(/,/g, '')
            .trim(),
        );
        if (!Number.isFinite(p) || !Number.isFinite(s)) continue;
        if (p <= 0 || s <= 0) continue;
        const q = p / step;
        const k = isBid ? Math.floor(q + eps) : Math.ceil(q - eps);
        const bucketPrice = k * step;
        if (!Number.isFinite(bucketPrice)) continue;
        const priceKey = bucketPrice.toFixed(aggPriceDecimals);
        const prev = map.get(priceKey) || 0;
        map.set(priceKey, prev + s);
      }
      const out: DepthLevel[] = [];
      for (const [priceKey, sumBase] of map.entries()) {
        if (!Number.isFinite(sumBase) || sumBase <= 0) continue;
        out.push({ price: priceKey, size: fmtBaseSize(sumBase) });
      }
      out.sort((a, b) => {
        const pa = Number(a.price);
        const pb = Number(b.price);
        return isBid ? pb - pa : pa - pb;
      });
      return out;
    };

    const bids = aggregateSide(rawBids, true);
    const asks = aggregateSide(rawAsks, false);
    return { bids, asks };
  }, [depth?.asks, depth?.bids, gradient, gradientOptions, pricePrecision]);

  const depthStats = useMemo(() => {
    const bids = aggregated.bids || [];
    const asks = aggregated.asks || [];
    const topN = 10;
    const sumQuote = (levels: DepthLevel[]) => {
      let total = 0;
      for (const lv of (levels || []).slice(0, topN)) {
        const p = Number(
          String(lv?.price ?? '')
            .replace(/,/g, '')
            .trim(),
        );
        const s = Number(
          String(lv?.size ?? '')
            .replace(/,/g, '')
            .trim(),
        );
        if (!Number.isFinite(p) || !Number.isFinite(s)) continue;
        if (p <= 0 || s <= 0) continue;
        total += p * s;
      }
      return total;
    };
    const bidTotal = sumQuote(bids);
    const askTotal = sumQuote(asks);
    const sum = bidTotal + askTotal;
    const bidPct = sum > 0 ? (bidTotal / sum) * 100 : 50;
    return { bidPct: Math.max(0, Math.min(100, bidPct)) };
  }, [aggregated.asks, aggregated.bids]);

  const tableStyle: React.CSSProperties = useMemo(
    () =>
      ({
        fontSize: 12,
        width: '100%',
        ['--ant-table-cell-padding-block' as any]: '2px',
        ['--ant-table-cell-padding-inline' as any]: '6px',
      } as React.CSSProperties),
    [],
  );
  const noBorderCellStyle: React.CSSProperties = useMemo(() => ({ borderBottom: 'none' }), []);
  const noBorderHeaderStyle: React.CSSProperties = useMemo(() => ({ borderBottom: 'none' }), []);

  const priceTitle = useMemo(
    () => (
      <Typography.Text strong style={{ fontSize: 12, color: token.colorTextSecondary }}>
        价格
      </Typography.Text>
    ),
    [token.colorTextSecondary],
  );
  const qtyTitle = useMemo(
    () => (
      <Typography.Text strong style={{ fontSize: 12, color: token.colorTextSecondary }}>
        数量（{quoteAsset || '--'}）
      </Typography.Text>
    ),
    [quoteAsset, token.colorTextSecondary],
  );

  const asks = aggregated.asks;
  const bids = aggregated.bids;

  const derivedLastPriceNum = useMemo(() => {
    const bid1 = parseFinite(bids?.[0]?.price);
    const ask1 = parseFinite(asks?.[0]?.price);
    if (bid1 === null || ask1 === null) return null;
    const mid = (bid1 + ask1) / 2;
    if (!Number.isFinite(mid)) return null;
    return mid;
  }, [asks, bids, parseFinite]);

  useEffect(() => {
    const lastNum = derivedLastPriceNum;
    if (typeof lastNum !== 'number' || !Number.isFinite(lastNum)) return;
    const prev = prevLastPriceRef.current;
    if (typeof prev === 'number' && Number.isFinite(prev)) {
      if (lastNum > prev) setLastPriceDir('up');
      else if (lastNum < prev) setLastPriceDir('down');
      else setLastPriceDir('flat');
    }
    prevLastPriceRef.current = lastNum;
  }, [derivedLastPriceNum]);

  const asksView = useMemo(() => asks.slice(0, 10).reverse(), [asks]);
  const bidsView = useMemo(() => bids.slice(0, 10), [bids]);

  const rowDepthPct = useMemo(() => {
    const calcLevelQuote = (rows: DepthLevel[]) => {
      const quoteByPrice = new Map<string, number>();
      let max = 0;
      for (const r of rows) {
        const p = parseFinite(r?.price);
        const s = parseFinite(r?.size);
        if (p === null || s === null) continue;
        if (p <= 0 || s <= 0) continue;
        const q = p * s;
        if (!Number.isFinite(q) || q <= 0) continue;
        const priceKey = String(r.price);
        quoteByPrice.set(priceKey, q);
        if (q > max) max = q;
      }
      return { quoteByPrice, max };
    };

    const ask = calcLevelQuote(asksView);
    const bid = calcLevelQuote(bidsView);
    const globalMax = Math.max(ask.max, bid.max);

    const toPct = (quoteByPrice: Map<string, number>) => {
      const pctByPrice = new Map<string, number>();
      if (!(globalMax > 0)) return pctByPrice;
      for (const [price, v] of quoteByPrice.entries()) {
        const pct = (v / globalMax) * 100;
        pctByPrice.set(price, Math.max(0, Math.min(100, pct)));
      }
      return pctByPrice;
    };

    return { asks: toPct(ask.quoteByPrice), bids: toPct(bid.quoteByPrice) };
  }, [asksView, bidsView, parseFinite]);

  const hasBids = bids.length > 0;
  const hasAsks = asks.length > 0;
  const empty =
    (mode === 'both' && !hasBids && !hasAsks) ||
    (mode === 'bid' && !hasBids) ||
    (mode === 'ask' && !hasAsks);

  const bidPct = Number.isFinite(depthStats.bidPct) ? depthStats.bidPct : 50;
  const askPct = 100 - bidPct;

  return (
    <div>
      <Flex justify="space-between" style={{ marginBottom: 8 }}>
        <Segmented
          size="small"
          value={mode}
          onChange={(val) => setMode(val as any)}
          options={[
            {
              label: (
                <Avatar src={bookAllIcon} size={18} style={{ display: 'block', paddingTop: 2 }} />
              ),
              value: 'both',
            },
            {
              label: (
                <Avatar src={bookBidsIcon} size={18} style={{ display: 'block', paddingTop: 2 }} />
              ),
              value: 'bid',
            },
            {
              label: (
                <Avatar src={bookAsksIcon} size={18} style={{ display: 'block', paddingTop: 2 }} />
              ),
              value: 'ask',
            },
          ]}
        />
        <Select
          size="small"
          value={gradient}
          onChange={(val) => setGradient(val as any)}
          style={{ width: 88 }}
          variant="borderless"
          placement="bottomRight"
          options={gradientOptions.map((o) => ({ value: o.value, label: o.label }))}
        />
      </Flex>

      {empty ? (
        <div style={{ padding: '48px 0' }}>
          <Empty description="暂无订单簿数据" />
        </div>
      ) : (
        <>
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '2px 6px',
              fontSize: 12,
              background: token.colorBgContainer,
              borderBottom: `1px solid ${token.colorBorder}`,
              marginBottom: 1,
              marginLeft: 4
            }}
          >
            <div style={{ flex: 1, textAlign: 'left' }}>{priceTitle}</div>
            <div style={{ flex: 1, textAlign: 'right' }}>{qtyTitle}</div>
          </div>
          {mode !== 'bid' && (
            <Table<DepthLevel>
              size="small"
              pagination={false}
              bordered={false}
              locale={{ emptyText: null }}
              rowKey={(row) => `ask-${row.price}`}
              dataSource={asksView}
              onRow={(row) => {
                const pct = rowDepthPct.asks.get(String(row?.price)) ?? 0;
                return {
                  style: {
                    lineHeight: '2px',
                    fontSize: 12,
                    backgroundImage:
                      'linear-gradient(to left, rgba(255,77,79,0.18) 0%, rgba(255,77,79,0.18) 100%)',
                    backgroundRepeat: 'no-repeat',
                    backgroundPosition: 'right center',
                    backgroundSize: `${pct}% 100%`,
                  },
                };
              }}
              style={{ ...tableStyle, border: 'none' }}
              showHeader={false}
              columns={[
                {
                  title: priceTitle,
                  dataIndex: 'price',
                  align: 'left',
                  onHeaderCell: () => ({ style: noBorderHeaderStyle }),
                  onCell: () => ({ style: noBorderCellStyle }),
                  render: (v) => (
                    <span style={{ ...monoPriceStyle, color: '#ff4d4f' }}>{formatPrice(v)}</span>
                  ),
                },
                {
                  title: qtyTitle,
                  dataIndex: 'size',
                  align: 'right',
                  onHeaderCell: () => ({ style: noBorderHeaderStyle }),
                  onCell: () => ({ style: noBorderCellStyle }),
                  render: (_v, row) =>
                    utils.math.formatKMB(Number(row.price) * Number(row.size), { digits: 2 }),
                },
              ]}
            />
          )}
          <div style={{ padding: '4px 6px' }}>
            <Space size={6} align="center">
              <Space size={2} align="center">
                <Typography.Text
                  style={{
                    ...monoPriceStyle,
                    fontSize: 18,
                    fontWeight: 700,
                    color: lastPriceColor,
                  }}
                >
                  {formatPrice(derivedLastPriceNum)}
                </Typography.Text>
                {lastPriceDir === 'up' ? (
                  <ArrowUpOutlined style={{ color: lastPriceColor }} />
                ) : null}
                {lastPriceDir === 'down' ? (
                  <ArrowDownOutlined style={{ color: lastPriceColor }} />
                ) : null}
              </Space>
              {isPerp ? (
                <div style={{ marginTop: 2, fontSize: 12, color: '#8c8c8c' }}>
                  {formatPrice(shownMarkPrice)}
                </div>
              ) : null}
            </Space>
          </div>
          {mode !== 'ask' && (
            <Table<DepthLevel>
              size="small"
              pagination={false}
              bordered={false}
              locale={{ emptyText: null }}
              rowKey={(row) => `bid-${row.price}`}
              dataSource={bidsView}
              onRow={(row) => {
                const pct = rowDepthPct.bids.get(String(row?.price)) ?? 0;
                return {
                  style: {
                    lineHeight: '2px',
                    fontSize: 12,
                    backgroundImage:
                      'linear-gradient(to left, rgba(82,196,26,0.18) 0%, rgba(82,196,26,0.18) 100%)',
                    backgroundRepeat: 'no-repeat',
                    backgroundPosition: 'right center',
                    backgroundSize: `${pct}% 100%`,
                  },
                };
              }}
              style={{ ...tableStyle, border: 'none' }}
              showHeader={false}
              columns={[
                {
                  title: priceTitle,
                  dataIndex: 'price',
                  align: 'left',
                  onHeaderCell: () => ({ style: noBorderHeaderStyle }),
                  onCell: () => ({ style: noBorderCellStyle }),
                  render: (v) => (
                    <span style={{ ...monoPriceStyle, color: '#52c41a' }}>{formatPrice(v)}</span>
                  ),
                },
                {
                  title: qtyTitle,
                  dataIndex: 'size',
                  align: 'right',
                  onHeaderCell: () => ({ style: noBorderHeaderStyle }),
                  onCell: () => ({ style: noBorderCellStyle }),
                  render: (_v, row) =>
                    utils.math.formatKMB(Number(row.price) * Number(row.size), { digits: 2 }),
                },
              ]}
            />
          )}
          <div style={{ marginTop: 10 }}>
            <div
              style={{
                position: 'relative',
                height: 30,
                width: '100%',
                background: '#0b0f14',
                borderRadius: 6,
                overflow: 'hidden',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  inset: 0,
                  width: `${bidPct}%`,
                  background:
                    'linear-gradient(90deg, rgba(82,196,26,0.40) 0%, rgba(82,196,26,0.22) 100%)',
                  clipPath:
                    bidPct >= 99
                      ? undefined
                      : 'polygon(0 0, calc(100% - 12px) 0, 100% 50%, calc(100% - 12px) 100%, 0 100%)',
                }}
              />
              <div
                style={{
                  position: 'absolute',
                  inset: 0,
                  left: `${bidPct}%`,
                  width: `${askPct}%`,
                  background:
                    'linear-gradient(270deg, rgba(255,77,79,0.40) 0%, rgba(255,77,79,0.22) 100%)',
                  clipPath:
                    askPct >= 99
                      ? undefined
                      : 'polygon(0 50%, 12px 0, 100% 0, 100% 100%, 12px 100%)',
                }}
              />

              <div
                style={{
                  position: 'relative',
                  height: '100%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '0 5px',
                  fontSize: 14,
                  fontWeight: 600,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, color: '#52c41a' }}>
                  <div
                    style={{
                      width: 22,
                      height: 22,
                      borderRadius: 4,
                      border: '1px solid rgba(82,196,26,0.9)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontWeight: 700,
                      lineHeight: '22px',
                    }}
                  >
                    B
                  </div>
                  <span style={{ fontSize: 12 }}>{utils.math.formatByPrecision(bidPct, 2)}%</span>
                </div>

                <div style={{ display: 'flex', alignItems: 'center', gap: 10, color: '#ff4d4f' }}>
                  <span style={{ fontSize: 12 }}>{utils.math.formatByPrecision(askPct, 2)}%</span>
                  <div
                    style={{
                      width: 22,
                      height: 22,
                      borderRadius: 4,
                      border: '1px solid rgba(255,77,79,0.9)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontWeight: 700,
                      lineHeight: '22px',
                    }}
                  >
                    S
                  </div>
                </div>
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  );
};
