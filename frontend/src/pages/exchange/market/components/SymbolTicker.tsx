import { EllipsisMiddleText } from '@/components';
import { Exchange, MarketType } from '@/global.types';
import { api } from '@/services/gateway';
import { Account, AccountStatus, queryAccounts } from '@/services/gateway/account';
import type {
  FundingRate,
  IndexPrice,
  MarketInfo,
  MarkPrice,
  OpenInterest,
  Ticker,
} from '@/services/gateway/market';
import utils from '@/utils';
import { BankOutlined, CheckOutlined, DollarOutlined, DownOutlined } from '@ant-design/icons';
import { Avatar, Button, Card, Col, Dropdown, message, Row, Select, Space, Tag, theme, Typography } from 'antd';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';

export type PriceChangeInfo = { color: string; text: string };

export type SymbolTickerProps = {
  exchange: Exchange;
  symbol: string;
  accountId: string | null;
  ticker?: Ticker;
  markPrice?: MarkPrice;
  indexPrice?: IndexPrice;
  fundingRate?: FundingRate;
  openInterest?: OpenInterest;
  pricePrecision: number;
  onSelectionConfirm: (selection: {
    exchange: Exchange;
    symbol: string;
    accountId: string | null;
    /** 确认时已拉取的 Market 详情，上游可跳过重复 queryMarket */
    marketInfo?: MarketInfo | null;
  }) => void;
};

const SymbolTicker: React.FC<SymbolTickerProps> = ({
  exchange,
  symbol,
  accountId,
  ticker,
  markPrice,
  indexPrice,
  fundingRate,
  openInterest,
  pricePrecision,
  onSelectionConfirm,
}) => {
  const [symbolPickerOpen, setSymbolPickerOpen] = useState(false);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const { token } = theme.useToken();
  const [accountList, setAccountList] = useState<Account[]>([]);
  const [symbolOptions, setSymbolOptions] = useState<Array<{ label: string; value: string }>>([]);
  const [draftExchange, setDraftExchange] = useState(exchange);
  const [draftAccountId, setDraftAccountId] = useState<string | null>(accountId);
  const [draftSymbol, setDraftSymbol] = useState(symbol);
  const [symbolOptionsLoading, setSymbolOptionsLoading] = useState(false);
  const [selectionSubmitting, setSelectionSubmitting] = useState(false);
  const symbolRequestIdRef = useRef(0);

  const parsedSymbol = useMemo(() => {
    return utils.market.parseSymbol(symbol);
  }, [symbol]);

  const exchangeLogo = useMemo(() => {
    return utils.market.getExchangeLogo(exchange);
  }, [exchange]);

  const isPerp = useMemo(() => {
    return parsedSymbol.type === MarketType.Future;
  }, [parsedSymbol]);

  const exchangeOptions = useMemo(
    () =>
      utils.dict.enumToOptions(Exchange).map((item: { label: string; value: Exchange }) => ({
        label: (
          <Space size={4} align="baseline">
            <Avatar src={utils.market.getExchangeLogo(item.value)} size={16} />
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {utils.market.getExchangeTitle(item.value)}
            </Typography.Text>
          </Space>
        ),
        value: item.value,
      })),
    [],
  );


  const loadSymbolOptions = useCallback(async (targetExchange: Exchange, preferredSymbol: string) => {
    const requestId = symbolRequestIdRef.current + 1;
    symbolRequestIdRef.current = requestId;
    setSymbolOptionsLoading(true);

    try {
      const markets = await api.queryMarkets({ exchange: targetExchange });
      if (symbolRequestIdRef.current !== requestId) return;

      const options: Array<{ label: string; value: string }> =
        markets?.map((m: { symbol: string }) => ({
          label: m.symbol,
          value: m.symbol,
        })) || [];

      setSymbolOptions(options);
      const nextSymbol =
        options.find((item) => item.value === preferredSymbol)?.value || options[0]?.value || '';
      setDraftSymbol(nextSymbol);
    } catch (e: any) {
      if (symbolRequestIdRef.current !== requestId) return;
      setSymbolOptions([]);
      setDraftSymbol('');
      message.error(e?.message || '加载交易对失败');
    } finally {
      if (symbolRequestIdRef.current === requestId) {
        setSymbolOptionsLoading(false);
      }
    }
  }, []);

  const getSymbolName = useCallback((symbol: string) => {
    return symbol.split(':')[0];
  }, []);

  const syncDraftSelection = useCallback(() => {
    setDraftExchange(exchange);
    setDraftAccountId(accountId);
    setDraftSymbol(symbol);
  }, [accountId, exchange, symbol]);

  useEffect(() => {
    if (symbolPickerOpen) return;
    syncDraftSelection();
    void loadSymbolOptions(exchange, symbol);
  }, [exchange, loadSymbolOptions, symbol, symbolPickerOpen, syncDraftSelection]);

  const selectValue = draftAccountId ? `account:${draftAccountId}` : `exchange:${draftExchange}`;
  const hasAccounts = accountList.length > 0;

  // 组件挂载时加载所有账户
  useEffect(() => {
    const loadAccounts = async () => {
      try {
        const result = await queryAccounts({
          status: AccountStatus.Online,
          pageSize: 100,
        });
        setAccountList(result?.list || []);
      } catch (err) {
        console.error('Failed to load accounts:', err);
      }
    };

    loadAccounts();
  }, []);


  const priceChangeInfo = useMemo(() => {
    const lastRaw = String(ticker?.lastPrice ?? '')
      .replace(/,/g, '')
      .trim();
    const openRaw = String(ticker?.open24H ?? '')
      .replace(/,/g, '')
      .trim();
    const last = Number(lastRaw);
    const open = Number(openRaw);

    if (!Number.isFinite(last) || !Number.isFinite(open) || open === 0) return undefined;

    const delta = last - open;
    const pct = (delta / open) * 100;
    const digits = Math.min(12, Math.max(0, pricePrecision));
    const fmt = (v: number) => String(utils.math.formatByPrecision(v, digits)).replace(/\.?0+$/, '');

    const color = delta > 0 ? '#52c41a' : delta < 0 ? '#ff4d4f' : '#8c8c8c';
    const sign = delta > 0 ? '+' : '';
    const pctSign = pct > 0 ? '+' : '';

    return {
      color,
      text: `${sign}${fmt(delta)} (${pctSign}${utils.math.formatByPrecision(pct, 2)}%)`,
    };
  }, [pricePrecision, ticker?.lastPrice, ticker?.open24H]);


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
    const fromTicker = String(ticker?.lastPrice ?? '').trim();
    if (fromTicker && !isZeroLike(fromTicker)) return fromTicker;
    return '';
  }, [isPerp, markPrice?.markPrice, ticker?.lastPrice]);

  const fundingRateText = useMemo(() => {
    if (!isPerp) return '--';
    const raw = String(fundingRate?.fundingRate ?? '')
      .replace(/,/g, '')
      .trim();
    if (!raw) return '--';
    const n = Number(raw);
    if (!Number.isFinite(n)) return raw;
    const pct = n * 100;
    const sign = pct > 0 ? '+' : '';
    const txt = utils.math.formatByPrecision(pct, 6).replace(/\.?0+$/, '');
    return `${sign}${txt}%`;
  }, [fundingRate?.fundingRate, isPerp]);


  const [fundingCountdown, setFundingCountdown] = useState('');
  useEffect(() => {
    if (!isPerp || fundingRate?.nextFundingTime == null) {
      setFundingCountdown('');
      return;
    }
    const nextMs = Number(fundingRate.nextFundingTime);
    if (!Number.isFinite(nextMs)) {
      setFundingCountdown('--');
      return;
    }
    const run = () => {
      const diff = nextMs - Date.now();
      if (diff <= 0) {
        setFundingCountdown('00:00:00');
        return;
      }
      const totalSeconds = Math.floor(diff / 1000);
      const h = Math.floor(totalSeconds / 3600);
      const m = Math.floor((totalSeconds % 3600) / 60);
      const s = totalSeconds % 60;
      setFundingCountdown([h, m, s].map((v) => String(v).padStart(2, '0')).join(':'));
    };
    run();
    const t = window.setInterval(run, 1000);
    return () => window.clearInterval(t);
  }, [isPerp, fundingRate?.nextFundingTime]);


  const openInterestQuoteText = useMemo(() => {
    if (!isPerp) return '--';
    const baseRaw = String(openInterest?.openInterest ?? '')
      .replace(/,/g, '')
      .trim();
    if (!baseRaw) return '--';
    const mpRaw = String(shownMarkPrice ?? '')
      .replace(/,/g, '')
      .trim();
    if (!mpRaw) return '--';
    const base = Number(baseRaw);
    const mp = Number(mpRaw);
    if (!Number.isFinite(base) || !Number.isFinite(mp)) return '--';
    const quote = base * mp;
    const fmt = utils.math.formatWanYi(quote, { digits: 2 });
    return parsedSymbol.quote ? `${fmt} ${parsedSymbol.quote}` : fmt;
  }, [isPerp, openInterest?.openInterest, parsedSymbol, shownMarkPrice]);

  const handleTickerWheel = useCallback(
    (event: React.WheelEvent<HTMLDivElement>) => {
      const el = scrollContainerRef.current;
      if (!el) return;

      const maxScrollLeft = el.scrollWidth - el.clientWidth;
      if (maxScrollLeft <= 0) return;

      const delta =
        Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
      if (!delta) return;

      event.preventDefault();
      event.stopPropagation();

      const nextScrollLeft = Math.max(0, Math.min(el.scrollLeft + delta, maxScrollLeft));
      if (nextScrollLeft !== el.scrollLeft) {
        el.scrollLeft = nextScrollLeft;
      }
    },
    [],
  );

  const handleSymbolPickerOpenChange = useCallback(
    (open: boolean) => {
      setSymbolPickerOpen(open);
      if (open) {
        syncDraftSelection();
        void loadSymbolOptions(exchange, symbol);
        return;
      }

      syncDraftSelection();
    },
    [exchange, loadSymbolOptions, symbol, syncDraftSelection],
  );

  const handleSourceChange = useCallback(
    (val: string) => {
      if (val.startsWith('exchange:')) {
        const nextExchange = val.slice(9) as Exchange;
        setDraftExchange(nextExchange);
        setDraftAccountId(null);
        void loadSymbolOptions(nextExchange, draftSymbol);
        return;
      }

      const nextAccountId = val.slice(8);
      const account = accountList.find((a) => a.id === nextAccountId);
      if (!account) return;

      setDraftAccountId(nextAccountId);
      setDraftExchange(account.exchange);
      void loadSymbolOptions(account.exchange, draftSymbol);
    },
    [accountList, draftSymbol, loadSymbolOptions],
  );

  const handleSelectionConfirm = useCallback(async () => {
    if (symbolOptionsLoading || selectionSubmitting || !draftSymbol) return;

    setSelectionSubmitting(true);
    let marketInfo: MarketInfo | null = null;
    try {
      marketInfo = (await api.queryMarket({
        exchange: draftExchange,
        symbol: draftSymbol,
      })) as MarketInfo | null;
    } catch {
      // 上游仍会走 loadMarketInfo 兜底
    } finally {
      setSelectionSubmitting(false);
    }

    onSelectionConfirm({
      exchange: draftExchange,
      symbol: draftSymbol,
      accountId: draftAccountId,
      marketInfo,
    });
    setSymbolPickerOpen(false);
  }, [
    draftAccountId,
    draftExchange,
    draftSymbol,
    onSelectionConfirm,
    selectionSubmitting,
    symbolOptionsLoading,
  ]);

  return (
    <Card
      style={{ marginBottom: 16, width: '100%' }}
      styles={{ body: { paddingTop: 10, paddingBottom: 10 } }}
    >
      <style>{`
        .symbol-ticker-scroll::-webkit-scrollbar {
          display: none;
        }
      `}</style>
      <div
        style={{
          position: 'relative',
          width: '100%',
          display: 'flex',
          alignItems: 'center',
          gap: 16,
        }}
      >
        <div style={{ width: 250, flexShrink: 0 }}>
          <Dropdown
            trigger={['click']}
            open={symbolPickerOpen}
            onOpenChange={handleSymbolPickerOpenChange}
            popupRender={() => (
              <div
                style={{
                  background: token.colorBgElevated,
                  borderRadius: 8,
                  boxShadow: token.boxShadowSecondary,
                }}
                onClick={(e) => e.stopPropagation()}
              >
                <Row justify="space-around" style={{ gap: 8, padding: 8, marginLeft: 8 }}>
                  <div>
                    <div style={{ color: token.colorTextSecondary, fontSize: 12, marginBottom: 6 }}>
                      <BankOutlined /> 交易所/账户
                    </div>
                    <Select
                      value={selectValue}
                      onChange={handleSourceChange}
                      style={{ width: 250 }}
                      showSearch
                      optionFilterProp="label"
                      options={[
                        {
                          label: '交易所',
                          options: exchangeOptions.map((e) => ({
                            label: e.label,
                            value: `exchange:${e.value}`,
                          })),
                        },
                        ...(hasAccounts
                          ? [
                            {
                              label: '账户',
                              options: accountList.map((acc) => ({
                                label: (
                                  <span
                                    key={acc.id}
                                    style={{
                                      display: 'flex',
                                      alignItems: 'center',
                                      gap: 6,
                                    }}
                                  >
                                    <img
                                      src={utils.market.getExchangeLogo(acc.exchange)}
                                      alt={acc.exchange}
                                      style={{ width: 16, height: 16, borderRadius: 2 }}
                                    />
                                    <span style={{ minWidth: 0, flex: 1, overflow: 'hidden' }}>
                                      <EllipsisMiddleText suffixCount={10}>{acc.name + ' · #' + acc.id.slice(-5)}</EllipsisMiddleText>
                                    </span>
                                  </span>
                                ),
                                value: `account:${acc.id}`,
                              })),
                            },
                          ]
                          : []),
                      ]}
                    />
                  </div>
                  <div style={{ width: 220 }}>
                    <div style={{ color: token.colorTextSecondary, fontSize: 12, marginBottom: 6 }}>
                      <DollarOutlined /> 交易对
                    </div>
                    <Select
                      showSearch
                      loading={symbolOptionsLoading}
                      value={draftSymbol || undefined}
                      options={symbolOptions}
                      onChange={setDraftSymbol}
                      filterOption={(input, option) =>
                        String(option?.value || '').toUpperCase().includes(input.toUpperCase())
                      }
                      style={{ width: 220 }}
                    />
                  </div>
                  <div style={{ width: 36 }}>
                    <div style={{ color: token.colorTextSecondary, fontSize: 12, marginBottom: 6, height: 19 }}>
                    </div>
                    <Button
                      variant="outlined"
                      style={{ color: "#95de64", borderColor: "#d9f7be" }}
                      loading={symbolOptionsLoading || selectionSubmitting}
                      disabled={!draftSymbol}
                      icon={<CheckOutlined />}
                      onClick={handleSelectionConfirm}
                    >
                    </Button>
                  </div>

                </Row>
              </div>
            )}
          >
            <Space style={{ cursor: 'pointer', userSelect: 'none' }} size={8}>
              {exchangeLogo ? (
                <img
                  src={exchangeLogo}
                  alt={String(exchange)}
                  style={{ width: 18, height: 18, borderRadius: 4, objectFit: 'cover' }}
                />
              ) : null}
              <Typography.Title level={4} style={{ margin: 0 }}>
                {getSymbolName(symbol) || '--'}
              </Typography.Title>
              {isPerp ? (
                <Tag color="purple" style={{ marginInlineStart: 0 }}>
                  永续
                </Tag>
              ) : null}
              <DownOutlined
                style={{
                  fontSize: 12,
                  color: '#8c8c8c',
                  transition: 'transform 0.15s ease',
                  transform: symbolPickerOpen ? 'rotate(180deg)' : undefined,
                }}
              />
            </Space>
          </Dropdown>
          {accountId && (
            <div style={{ paddingTop: 0, color: '#8c8c8c' }}>
              <span style={{ minWidth: 0, flex: 1, overflow: 'hidden' }}>
                {(() => {
                  const account = accountList.find((a) => a.id === accountId);
                  if (!account) return null;
                  return (
                    <EllipsisMiddleText suffixCount={10} style={{ fontSize: 11, width: 200, color: '#8c8c8c' }}>{account.name + ' · #' + account.id.slice(-5)}</EllipsisMiddleText>
                  );
                })()}
              </span>
            </div>
          )}
        </div>
        <div
          ref={scrollContainerRef}
          className="symbol-ticker-scroll"
          onWheel={handleTickerWheel}
          style={{
            flex: 1,
            minWidth: 0,
            overflowX: 'auto',
            overflowY: 'hidden',
            overscrollBehaviorX: 'contain',
            overscrollBehaviorY: 'contain',
            scrollbarWidth: 'none',
            msOverflowStyle: 'none',
          }}
        >
          <Row
            gutter={16}
            align="middle"
            style={{ flexWrap: 'nowrap', width: 'max-content', minWidth: '100%' }}
          >
            <Col style={{ width: 130, flexShrink: 0 }}>
              <div
                style={{
                  fontSize: 18,
                  fontWeight: 700,
                  color: priceChangeInfo?.color ?? '#8c8c8c',
                }}
              >
                {utils.math.formatByPrecision(ticker?.lastPrice, pricePrecision)}
              </div>
              <div style={{ fontSize: 12, color: priceChangeInfo?.color ?? '#8c8c8c' }}>
                {priceChangeInfo?.text || '--'}
              </div>
            </Col>
            {isPerp ? (
              <Col style={{ width: 70, flexShrink: 0 }}>
                <div style={{ color: '#8c8c8c', fontSize: 12 }}>标记价格</div>
                <div style={{ fontSize: 12, fontWeight: 500 }}>{utils.math.formatByPrecision(shownMarkPrice, pricePrecision)}</div>
              </Col>
            ) : null}
            {isPerp ? (
              <Col style={{ width: 70, flexShrink: 0 }}>
                <div style={{ color: '#8c8c8c', fontSize: 12 }}>指数价格</div>
                <div style={{ fontSize: 12, fontWeight: 500 }}>
                  {utils.math.formatByPrecision(indexPrice?.indexPrice, pricePrecision)}
                </div>
              </Col>
            ) : null}
            {isPerp ? (
              <Col style={{ width: 160, flexShrink: 0 }}>
                <div style={{ color: '#8c8c8c', fontSize: 12 }}>资金费率 / 倒计时</div>
                <div style={{ fontSize: 12, fontWeight: 500 }}>
                  <Space size={4}>
                    <span style={{ color: '#f3b765' }}>{fundingRateText}</span>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      /
                    </Typography.Text>
                    <span style={{ fontSize: 12 }}>{fundingCountdown || '--'}</span>
                  </Space>
                </div>
              </Col>
            ) : null}
            {isPerp ? (
              <Col style={{ width: 140, flexShrink: 0 }}>
                <div style={{ color: '#8c8c8c', fontSize: 12 }}>合约持仓量（{parsedSymbol.quote}）</div>
                <div style={{ fontSize: 12, fontWeight: 500 }}>{openInterestQuoteText}</div>
              </Col>
            ) : null}
            <Col style={{ width: 100, flexShrink: 0 }}>
              <div style={{ color: '#8c8c8c', fontSize: 12 }}>24 小时最高价</div>
              <div style={{ fontSize: 12, fontWeight: 500 }}>{utils.math.formatByPrecision(ticker?.high24H, pricePrecision)}</div>
            </Col>
            <Col style={{ width: 100, flexShrink: 0 }}>
              <div style={{ color: '#8c8c8c', fontSize: 12 }}>24 小时最低价</div>
              <div style={{ fontSize: 12, fontWeight: 500 }}>{utils.math.formatByPrecision(ticker?.low24H, pricePrecision)}</div>
            </Col>
            <Col style={{ width: 100, flexShrink: 0 }}>
              <div style={{ color: '#8c8c8c', fontSize: 12 }}>24 小时成交量</div>
              <div style={{ fontSize: 12, fontWeight: 500 }}>
                {utils.math.formatWanYi(ticker?.volume24H)}
              </div>
            </Col>
            {Number(ticker?.quoteVolume24H ?? 0) > 0 && (
              <Col style={{ width: 100, flexShrink: 0 }}>
                <div style={{ color: '#8c8c8c', fontSize: 12 }}>24 小时成交额</div>
                <div style={{ fontSize: 12, fontWeight: 500 }}>
                  {utils.math.formatWanYi(ticker?.quoteVolume24H)}
                </div>
              </Col>)}
          </Row>
        </div>
      </div>
    </Card>
  );
};

export default SymbolTicker;
