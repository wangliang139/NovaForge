import { Exchange, MarketType } from '@/global.types';
import {
  Account,
  AccountType,
  AccountUnallocatedAsset,
  queryAccounts,
  queryAccountUnallocatedAssets,
  WalletType,
} from '@/services/gateway/account';
import * as api from '@/services/gateway/api';
import {
  Bot,
  BotMode,
  BotModeOptions,
  createBot,
  CreateBotInput,
  IsMarketSignal,
  ParamType,
  queryBots,
  queryStrategies,
  SignalScope,
  SignalTypeOptions,
  Strategy,
  StrategyParam,
  StrategyStatus,
  updateBot,
  UpdateBotInput,
} from '@/services/gateway/strategy';
import { getExchangeLogo, getExchangeTitle, parseSymbol } from '@/utils/market';
import { getWalletTypeLabel } from '@/utils/marketTag';
import {
  DeleteOutlined,
  InfoCircleOutlined,
  PlusOutlined,
  UsergroupAddOutlined,
  UserOutlined
} from '@ant-design/icons';
import {
  ModalForm,
  ProForm,
  ProFormDigit,
  ProFormSelect,
  ProFormText,
} from '@ant-design/pro-components';
import {
  Button,
  Card,
  Col,
  Collapse,
  Flex,
  Form,
  Input,
  message,
  Modal,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
} from 'antd';
import Decimal from 'decimal.js';
import React, { useEffect, useRef, useState } from 'react';

type BotModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
  bot?: Bot | null;
};

/** 模拟盘与多 Bot 子账户共用：写入 bot config.initialAssets（字段 asset / total / frozen） */
type BotInitialAssetRow = {
  asset: string;
  walletType?: WalletType;
  total?: string | number;
  frozen?: string;
};

const EMPTY_BOT_INITIAL_ASSET_ROW: BotInitialAssetRow = {
  asset: '',
  walletType: undefined,
  total: undefined,
  frozen: '0',
};

type BotRuntimeConfig = {
  signalTimeoutMs?: number;
  aiTimeoutMs?: number;
  maxAITimeoutMs?: number;
};

const DEFAULT_BOT_RUNTIME_CONFIG: Required<BotRuntimeConfig> = {
  signalTimeoutMs: 30000,
  aiTimeoutMs: 15000,
  maxAITimeoutMs: 30000,
};

function coercePositiveInt(value: unknown, fallback: number): number {
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) {
    return fallback;
  }
  return Math.floor(n);
}

function normalizeRuntimeConfig(raw: unknown): Required<BotRuntimeConfig> {
  const r = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
  return {
    signalTimeoutMs: coercePositiveInt(r.signalTimeoutMs, DEFAULT_BOT_RUNTIME_CONFIG.signalTimeoutMs),
    aiTimeoutMs: coercePositiveInt(r.aiTimeoutMs, DEFAULT_BOT_RUNTIME_CONFIG.aiTimeoutMs),
    maxAITimeoutMs: coercePositiveInt(r.maxAITimeoutMs, DEFAULT_BOT_RUNTIME_CONFIG.maxAITimeoutMs),
  };
}

/** 实盘多 Bot 父账户下默认首行（与原先 OKX 类预设一致，可按需再按 exchange 区分） */
const defaultLiveMultiInitialAssetRows = (exchange?: string): BotInitialAssetRow[] => {
  if (exchange === Exchange.Binance || exchange === Exchange.BinanceTest) {
    return [
      { asset: 'USDT', walletType: WalletType.Spot, total: '', frozen: '0' },
    ];
  }
  return [{ asset: 'USDT', walletType: WalletType.Trade, total: '', frozen: '0' }];
}

/** 读表单或历史 JSON 时，把后端/旧数据别名收敛到 BotInitialAssetRow */
function coerceInitialAssetRow(raw: unknown): BotInitialAssetRow {
  if (!raw || typeof raw !== 'object') {
    return { ...EMPTY_BOT_INITIAL_ASSET_ROW };
  }
  const r = raw as Record<string, unknown>;
  return {
    asset: String(r.asset ?? r.code ?? '').trim(),
    walletType: r.walletType as WalletType | undefined,
    total: (r.total ?? r.balance) as BotInitialAssetRow['total'],
    frozen:
      r.frozen !== undefined && r.frozen !== null && r.frozen !== ''
        ? String(r.frozen)
        : '0',
  };
}

const BotModal: React.FC<BotModalProps> = ({ open, onOpenChange, onSuccess, bot }) => {
  const isEdit = !!bot;
  const [form] = Form.useForm();
  const [strategies, setStrategies] = useState<Strategy[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedStrategy, setSelectedStrategy] = useState<Strategy | null>(null);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loadingAccounts, setLoadingAccounts] = useState(false);
  const [occupiedAccountIds, setOccupiedAccountIds] = useState<Set<string>>(new Set());
  const [exchangeSymbols, setExchangeSymbols] = useState<
    Record<string, { label: string; value: string }[]>
  >({});
  const [loadingSymbols, setLoadingSymbols] = useState<Record<string, boolean>>({});
  const [unallocatedRows, setUnallocatedRows] = useState<AccountUnallocatedAsset[]>([]);
  const [loadingUnallocated, setLoadingUnallocated] = useState(false);

  useEffect(() => {
    if (open) {
      loadStrategies();
      loadAccounts();
      if (!isEdit) {
        form.setFieldsValue({
          mode: BotMode.Live,
          paperAccountSource: 'existing',
          exchange: 'binance',
          symbols: [],
          initialAssets: [{ ...EMPTY_BOT_INITIAL_ASSET_ROW }],
          runtime: { ...DEFAULT_BOT_RUNTIME_CONFIG },
        });
      } else {
        form.resetFields();
        setSelectedStrategy(null);
        setExchangeSymbols({});
      }
    } else {
      form.resetFields();
      setSelectedStrategy(null);
      setExchangeSymbols({});
    }
  }, [open, isEdit]);

  useEffect(() => {
    if (isEdit && bot) {
      form.setFieldsValue({
        name: bot.name,
        description: bot.description,
        exchange: bot.exchange,
        symbols: bot.symbols,
        mode: bot.mode,
        accountId: bot.accountId,
      });
    }
    if (isEdit && bot && strategies.length > 0) {
      const strategy = strategies.find((s) => s.id === bot.strategyId);
      if (strategy) {
        setSelectedStrategy(strategy);
        form.setFieldValue('strategyId', bot.strategyId);
        let config: Record<string, any> = {};
        try {
          config = bot.config ? JSON.parse(bot.config) : {};
        } catch (e) {
          console.error('Failed to parse bot config:', e);
        }
        form.setFieldValue('runtime', normalizeRuntimeConfig(config.runtime));
        if (strategy.params && strategy.params.length > 0) {
          const params: Record<string, any> = {};
          strategy.params.forEach((param) => {
            if (config.params && config.params[param.name] !== undefined) {
              const value = config.params[param.name];
              if (param.type === ParamType.Number) {
                params[param.name] = parseFloat(value);
              } else if (param.type === ParamType.Bool) {
                params[param.name] = value === true || value === 'true';
              } else {
                params[param.name] = value;
              }
            } else if (param.default !== undefined && param.default !== null) {
              if (param.type === ParamType.Number) {
                params[param.name] = parseFloat(param.default);
              } else if (param.type === ParamType.Bool) {
                params[param.name] = param.default === 'true';
              } else {
                params[param.name] = param.default;
              }
            }
          });
          form.setFieldValue('params', params);
        }
        if (strategy.signals && strategy.signals.length > 0) {
          const signals = config.signals || [];
          const formSignals = strategy.signals
            .sort((a, b) => Number(a.id) - Number(b.id))
            .map((signal) => {
              const scope = signal.scope;
              const existingSignal = signals.find((s: any) => s.signalId === signal.id);
              if (scope === SignalScope.Symbol) {
                const bindings = (bot.symbols || []).map((symbol) => ({
                  exchange: bot.exchange,
                  symbol: symbol,
                }));
                return {
                  signalId: signal.id,
                  scope: scope,
                  bindings: bindings,
                };
              } else if (scope === SignalScope.Target) {
                return (
                  existingSignal || {
                    signalId: signal.id,
                    scope: scope,
                    datasourceId: 0,
                  }
                );
              } else if (scope === SignalScope.Exchange) {
                return {
                  signalId: signal.id,
                  scope: scope,
                  bindings: [{ exchange: bot.exchange }],
                };
              } else {
                return (
                  existingSignal || {
                    signalId: signal.id,
                    scope: scope,
                    datasourceId: 0,
                  }
                );
              }
            });
          form.setFieldValue('signals', formSignals);
        }
      }
    }
  }, [isEdit, bot, strategies]);

  const exchangeValue = Form.useWatch('exchange', form);
  // 监听 symbols 变化，更新信号绑定结构
  const symbolsValue = Form.useWatch('symbols', form);
  const modeValue = Form.useWatch('mode', form);
  const paperAccountSource = Form.useWatch('paperAccountSource', form) as
    | 'existing'
    | 'new'
    | undefined;
  const accountIdWatch = Form.useWatch('accountId', form);
  const needsLiveSubAllocation =
    !isEdit &&
    modeValue === BotMode.Live &&
    !!(
      accounts.find((a) => String(a.id) === String(accountIdWatch ?? ''))?.multiBotMode
    );
  const prevExchangeRef = useRef<string | undefined>(exchangeValue);
  const getAllowedWalletTypes = (exchange?: string) => {
    if (exchange === Exchange.Binance || exchange === Exchange.BinanceTest) {
      return [WalletType.Spot, WalletType.Future];
    }
    if (exchange === Exchange.OKX || exchange === Exchange.OKXTest) {
      return [WalletType.Trade];
    }
    return [
      WalletType.Spot,
      WalletType.Future,
      WalletType.Fund,
      WalletType.Trade,
      WalletType.Margin,
    ];
  };
  const getSymbolWalletType = (exchange: string | undefined, symbolType: MarketType) => {
    if (exchange === Exchange.Binance || exchange === Exchange.BinanceTest) {
      return symbolType === MarketType.Future ? WalletType.Future : WalletType.Spot;
    }
    if (exchange === Exchange.OKX || exchange === Exchange.OKXTest) {
      return WalletType.Trade;
    }
    return undefined;
  };
  const validateInitialAssetsAgainstSymbols = (assets: BotInitialAssetRow[]) => {
    if (
      !exchangeValue ||
      !symbolsValue ||
      symbolsValue.length === 0 ||
      !assets ||
      assets.length === 0
    ) {
      return true;
    }
    const assetsByWallet: Record<string, Set<string>> = {};
    const allAssets = new Set<string>();
    assets.forEach((item) => {
      const asset = String(item?.asset || '')
        .trim()
        .toUpperCase();
      const walletType = String(item?.walletType || '').trim();
      if (!asset || !walletType) return;
      allAssets.add(asset);
      if (!assetsByWallet[walletType]) {
        assetsByWallet[walletType] = new Set<string>();
      }
      assetsByWallet[walletType].add(asset);
    });

    for (const symbol of symbolsValue as string[]) {
      if (!symbol) continue;
      const parsed = parseSymbol(String(symbol));
      if (!parsed.base || !parsed.quote) {
        return false;
      }
      const walletType = getSymbolWalletType(exchangeValue, parsed.type);
      const assetSet = walletType ? assetsByWallet[walletType] || new Set<string>() : allAssets;
      if (parsed.type === MarketType.Future) {
        if (!assetSet.has(parsed.quote)) {
          return false;
        }
      } else {
        if (!assetSet.has(parsed.base) && !assetSet.has(parsed.quote)) {
          return false;
        }
      }
    }
    return true;
  };

  /** 与后端 allocKey 一致：资产大写 + 钱包类型小写 */
  const multiBotAllocKey = (asset: string, walletType: string) =>
    `${String(asset || '').trim().toUpperCase()}|${String(walletType || '').trim().toLowerCase()}`;

  const walletTypeKey = (w: unknown) => String(w ?? '').trim().toLowerCase();

  const parseInitialAssetTotal = (item: BotInitialAssetRow): Decimal | null => {
    const raw = item?.total;
    if (raw === null || raw === undefined || raw === '') {
      return new Decimal(0);
    }
    try {
      if (typeof raw === 'number' && Number.isFinite(raw)) {
        return new Decimal(raw);
      }
      return new Decimal(String(raw).trim());
    } catch {
      return null;
    }
  };

  /** 用已拉取的未分配列表校验各条 initialAssets（与后端 allocKey 一致） */
  const checkInitialAssetsAgainstUnallocatedRows = (
    fresh: AccountUnallocatedAsset[],
    initialAssets: unknown[],
  ): string | null => {
    const rows = initialAssets.map(coerceInitialAssetRow);
    const allowed = new Map<string, Decimal>();
    for (const row of fresh) {
      const k = multiBotAllocKey(row.asset, walletTypeKey(row.walletType));
      try {
        allowed.set(k, new Decimal(row.unallocated || '0'));
      } catch {
        allowed.set(k, new Decimal(0));
      }
    }
    for (const item of rows) {
      const asset = String(item.asset || '')
        .trim()
        .toUpperCase();
      const walletType = walletTypeKey(item?.walletType);
      if (!asset || !walletType) {
        continue;
      }
      const total = parseInitialAssetTotal(item);
      if (total === null) {
        return '初始资产数量格式不正确';
      }
      if (total.isNegative()) {
        return '初始资产数量不能为负';
      }
      if (total.isZero()) {
        continue;
      }
      const k = multiBotAllocKey(asset, walletType);
      const max = allowed.get(k) ?? new Decimal(0);
      if (total.gt(max)) {
        const [sym, wt] = k.split('|');
        return `初始分配超额：${sym}（${wt}）申请 ${total.toString()}，超过未分配 ${max.toString()}`;
      }
    }
    return null;
  };

  const validateMultiBotInitialAssets = async (
    parentAccountId: string,
    initialAssets: BotInitialAssetRow[],
  ): Promise<boolean> => {
    let fresh: AccountUnallocatedAsset[];
    try {
      fresh = await queryAccountUnallocatedAssets(String(parentAccountId));
    } catch {
      message.error('拉取父账户未分配资产失败，请稍后重试');
      return false;
    }
    const errMsg = checkInitialAssetsAgainstUnallocatedRows(fresh, initialAssets);
    if (errMsg) {
      message.error(errMsg);
      return false;
    }
    return true;
  };

  const walletTypeOptions = getAllowedWalletTypes(exchangeValue).map((value) => {
    return { value, label: getWalletTypeLabel(value) };
  });

  const isAccountOccupiedByOtherBot = (accountId: string) => {
    if (!accountId) return false;
    if (!occupiedAccountIds.has(accountId)) return false;
    return !(isEdit && bot && String(bot.accountId) === String(accountId));
  };

  // 实盘：可选 real 账户；模拟盘：仅支持 virtual 账户
  const liveAccountOptions = accounts
    .filter(
      (acc) =>
        acc.accountType === AccountType.Real &&
        (!exchangeValue || acc.exchange === exchangeValue) &&
        (acc.multiBotMode || !isAccountOccupiedByOtherBot(String(acc.id))),
    )
    .map((acc) => ({
      value: acc.id,
      label: (
        <Space
          size={6}
          style={{ display: 'flex', alignItems: 'center', width: '100%', justifyContent: 'space-between' }}
        >
          <span>
            {acc.name} ({getExchangeTitle(acc.exchange)})
          </span>
          {acc.multiBotMode ? (
            <Tooltip title="多 Bot 共享">
              <UsergroupAddOutlined style={{ color: '#1677ff', fontSize: 14 }} aria-hidden />
            </Tooltip>
          ) : (
            <Tooltip title="单Bot独享">
              <UserOutlined style={{ color: '#1677ff', fontSize: 14 }} aria-hidden />
            </Tooltip>
          )}
        </Space>
      ),
    }));

  useEffect(() => {
    if (!needsLiveSubAllocation || !accountIdWatch) {
      setUnallocatedRows([]);
      return;
    }
    let cancelled = false;
    setLoadingUnallocated(true);
    queryAccountUnallocatedAssets(accountIdWatch)
      .then((rows) => {
        if (!cancelled) {
          setUnallocatedRows(rows || []);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setUnallocatedRows([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoadingUnallocated(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [needsLiveSubAllocation, accountIdWatch]);

  useEffect(() => {
    if (!open || isEdit || !needsLiveSubAllocation || !accountIdWatch || !accounts.length) {
      return;
    }
    form.setFieldsValue({
      initialAssets: defaultLiveMultiInitialAssetRows(accounts.find((a) => String(a.id) === String(accountIdWatch ?? ''))?.exchange),
    });
  }, [accountIdWatch, accounts, needsLiveSubAllocation, open, isEdit, form]);

  const paperAccountOptions = accounts
    .filter(
      (acc) =>
        acc.accountType === AccountType.Virtual &&
        (!exchangeValue || acc.exchange === exchangeValue) &&
        !isAccountOccupiedByOtherBot(String(acc.id)),
    )
    .map((acc) => ({
      label: `${acc.name} (${getExchangeTitle(acc.exchange)})`,
      value: acc.id,
    }));

  useEffect(() => {
    if (!exchangeValue) {
      return;
    }
    loadSymbolsForExchange(exchangeValue);
    if (prevExchangeRef.current && prevExchangeRef.current !== exchangeValue) {
      form.setFieldsValue({
        symbols: [],
        accountId: undefined,
        initialAssets: [{ ...EMPTY_BOT_INITIAL_ASSET_ROW }],
      });
    }
    prevExchangeRef.current = exchangeValue;
  }, [exchangeValue]);

  // 切换模式时，若当前选中的账户不在该模式允许类型内则清空
  useEffect(() => {
    if (isEdit) return;
    const accountId = form.getFieldValue('accountId');
    if (!accountId) return;
    const opts =
      modeValue === BotMode.Live
        ? liveAccountOptions
        : paperAccountSource === 'existing'
          ? paperAccountOptions
          : [];
    const allowed = opts.some((o) => o.value === accountId);
    if (!allowed) {
      form.setFieldsValue({ accountId: undefined });
    }
  }, [isEdit, modeValue, paperAccountSource, liveAccountOptions, paperAccountOptions]);

  useEffect(() => {
    if (isEdit) return;
    if (modeValue === BotMode.Paper && paperAccountSource === 'new') {
      form.setFieldsValue({ accountId: undefined });
    }
  }, [isEdit, modeValue, paperAccountSource, form]);

  useEffect(() => {
    if (!symbolsValue || symbolsValue.length === 0 || !selectedStrategy || !exchangeValue) {
      return;
    }

    const currentSignals = form.getFieldValue('signals') || [];
    const updatedSignals = selectedStrategy.signals.map((signal) => {
      const scope = signal.scope;
      const existingSignal = currentSignals.find((s: any) => s.signalId === signal.id);

      if (scope === SignalScope.Symbol) {
        const bindings = (symbolsValue as any[])
          .filter((symbol: any) => Boolean(symbol))
          .map((symbol: any) => {
            const existingBinding = existingSignal?.bindings?.find(
              (b: any) => b.exchange === exchangeValue && b.symbol === symbol,
            );
            return {
              exchange: exchangeValue,
              symbol: symbol,
              datasourceId: 0, // 仅支持自动
            };
          });
        return {
          signalId: signal.id,
          scope: scope,
          bindings: bindings,
        };
      } else if (scope === SignalScope.Target) {
        return (
          existingSignal || {
            signalId: signal.id,
            scope: scope,
            datasourceId: 0, // 仅支持自动
          }
        );
      } else if (scope === SignalScope.Exchange) {
        const bindings = [exchangeValue].map((exchange: any) => {
          const existingBinding = existingSignal?.bindings?.find(
            (b: any) => b.exchange === exchange,
          );
          return {
            exchange: exchange,
            symbol: existingBinding?.symbol !== undefined ? existingBinding.symbol : undefined,
            datasourceId: 0, // 仅支持自动
          };
        });
        return {
          signalId: signal.id,
          scope: scope,
          bindings: bindings,
        };
      } else {
        return (
          existingSignal || {
            signalId: signal.id,
            scope: scope,
            datasourceId: 0, // 仅支持自动
          }
        );
      }
    });

    form.setFieldsValue({ signals: updatedSignals });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [symbolsValue, selectedStrategy?.signals, modeValue]);

  const loadStrategies = async () => {
    setLoading(true);
    try {
      const result = await queryStrategies({ current: 1, pageSize: 100 });
      // 只显示激活的版本
      const activeStrategies = (result?.list || []).filter(
        (s: Strategy) => s.status === StrategyStatus.Active,
      );
      setStrategies(activeStrategies);
    } catch (error) {
      message.error('加载策略列表失败');
    } finally {
      setLoading(false);
    }
  };

  const loadAccounts = async () => {
    setLoadingAccounts(true);
    try {
      const [result, botsResult] = await Promise.all([
        queryAccounts({ current: 1, pageSize: 1000 }),
        queryBots({ current: 1, pageSize: 2000 }),
      ]);
      setAccounts(result?.list || []);
      const occupied = new Set<string>();
      (botsResult?.list || []).forEach((b: Bot) => {
        const id = String(b?.accountId || '').trim();
        if (id) occupied.add(id);
      });
      setOccupiedAccountIds(occupied);
      if (isEdit && bot) {
        form.setFieldsValue({ accountId: bot.accountId });
      }
    } catch (error) {
      message.error('加载账户列表失败');
    } finally {
      setLoadingAccounts(false);
    }
  };

  const loadSymbolsForExchange = async (exchange: string) => {
    if (!exchange) {
      return;
    }

    if (exchangeSymbols[exchange]) {
      return;
    }

    setLoadingSymbols((prevLoading) => ({ ...prevLoading, [exchange]: true }));
    try {
      const markets = await api.queryMarkets({ exchange });
      if (markets && markets.length > 0) {
        const symbolOptions = markets.map((market: any) => ({
          label: market.symbol,
          value: market.symbol,
        }));
        setExchangeSymbols((prevSymbols) => ({
          ...prevSymbols,
          [exchange]: symbolOptions,
        }));
      } else {
        setExchangeSymbols((prevSymbols) => ({
          ...prevSymbols,
          [exchange]: [],
        }));
      }
    } catch (error) {
      console.error('Failed to load symbols:', error);
      setExchangeSymbols((prevSymbols) => ({
        ...prevSymbols,
        [exchange]: [],
      }));
    } finally {
      setLoadingSymbols((prevLoading) => ({ ...prevLoading, [exchange]: false }));
    }
  };

  const handleSubmit = async (values: any) => {
    if (!selectedStrategy) {
      message.error('请选择策略');
      return false;
    }

    const pickInitialAssets = (): BotInitialAssetRow[] => {
      const fromValues = values.initialAssets;
      const fromForm = form.getFieldValue('initialAssets');
      const raw = Array.isArray(fromValues)
        ? fromValues
        : Array.isArray(fromForm)
          ? fromForm
          : [];
      return raw.map(coerceInitialAssetRow);
    };

    const configMap: Record<string, any> = {};
    const accountForSubmit = accounts.find(
      (a) => String(a.id) === String(values.accountId ?? ''),
    );
    const liveMulti = values.mode === BotMode.Live && !!accountForSubmit?.multiBotMode;

    const createNewPaperAccount =
      values.mode === BotMode.Paper && (values.paperAccountSource || 'existing') === 'new';
    if (!isEdit && (createNewPaperAccount || liveMulti)) {
      const initialAssets = pickInitialAssets();
      if (!initialAssets.length) {
        message.error(
          createNewPaperAccount
            ? '模拟盘需要配置初始资产'
            : '多 Bot 共享父账户需配置子账户初始资产',
        );
        return false;
      }
      if (!validateInitialAssetsAgainstSymbols(initialAssets)) {
        message.error('初始资产和交易对不匹配，缺失相关资产，请检查');
        return false;
      }
      if (liveMulti) {
        const okUnalloc = await validateMultiBotInitialAssets(
          String(values.accountId),
          initialAssets,
        );
        if (!okUnalloc) {
          return false;
        }
      }
      configMap.initialAssets = initialAssets;
    }
    configMap.params = values.params;

    const runtime = normalizeRuntimeConfig(values.runtime ?? form.getFieldValue('runtime'));
    if (runtime.aiTimeoutMs > runtime.maxAITimeoutMs) {
      message.error('AI 默认超时不能大于 AI 最大超时');
      return false;
    }
    if (runtime.maxAITimeoutMs > runtime.signalTimeoutMs) {
      message.error('AI 最大超时不能大于单次 Signal 超时');
      return false;
    }
    configMap.runtime = runtime;

    // 将 signals 放入 config（仅支持自动）
    try {
      const signals: any[] = [];
      if (values.signals && values.signals.length > 0) {
        values.signals.forEach((formSignal: any) => {
          const signal = selectedStrategy.signals.find((s) => s.id === formSignal.signalId);
          if (!signal) return;

          const scope = signal.scope;
          if (scope === SignalScope.Symbol) {
            if (formSignal.bindings) {
              formSignal.bindings.forEach((binding: any) => {
                signals.push({
                  signalId: signal.id,
                  datasourceId: 0, // 仅支持自动
                  exchange: binding.exchange,
                  symbol: binding.symbol,
                });
              });
            }
          } else if (scope === SignalScope.Target) {
            signals.push({
              signalId: signal.id,
              datasourceId: 0, // 仅支持自动
              exchange: signal.exchange,
              symbol: signal.symbol,
            });
          } else if (scope === SignalScope.Exchange) {
            if (formSignal.bindings) {
              formSignal.bindings.forEach((binding: any) => {
                signals.push({
                  signalId: signal.id,
                  datasourceId: 0, // 仅支持自动
                  exchange: binding.exchange,
                  symbol: binding.symbol,
                });
              });
            }
          } else {
            signals.push({
              signalId: signal.id,
              datasourceId: 0, // 仅支持自动
            });
          }
        });
      }
      configMap.signals = signals;
    } catch (e) {
      message.error('信号数据源格式错误');
      return false;
    }

    // 构建 config JSON 字符串
    let configStr: string | undefined;
    if (Object.keys(configMap).length > 0) {
      try {
        configStr = JSON.stringify(configMap);
      } catch (e) {
        message.error('配置格式错误');
        return false;
      }
    }

    // 对于当前后端schema，我们使用第一个symbol的信息（兼容旧接口）
    // 后续后端更新后，可以支持多个symbol
    if (!values.symbols || values.symbols.length === 0) {
      message.error('请至少配置一个交易对');
      return false;
    }

    // 实盘：需选真实/测试网账户；模拟盘：需选模拟账户
    if (values.mode === BotMode.Live && !values.accountId) {
      message.error('实盘模式下必须选择交易账户（真实或测试网）');
      return false;
    }
    if (
      values.mode === BotMode.Paper &&
      (values.paperAccountSource || 'existing') === 'existing' &&
      !values.accountId
    ) {
      message.error('模拟盘（已有账户）必须选择交易账户（仅支持模拟账户）');
      return false;
    }

    const input: CreateBotInput = {
      strategyId: values.strategyId,
      strategyVer: selectedStrategy.version,
      name: String(values.name || '').trim(),
      description: String(values.description || '').trim(),
      mode: values.mode,
      exchange: values.exchange,
      symbols: values.symbols,
      accountId:
        values.mode === BotMode.Paper && (values.paperAccountSource || 'existing') === 'new'
          ? ''
          : values.accountId || '',
      config: configStr || '{}',
    };

    if (isEdit && bot) {
      return new Promise<boolean>((resolve) => {
        Modal.confirm({
          title: '确认修改',
          content: '修改Bot将清空策略运行态数据并重启，请确认？',
          okText: '确认',
          cancelText: '取消',
          onOk: async () => {
            const updateInput: UpdateBotInput = {
              id: bot.id,
              name: String(values.name || '').trim(),
              description: String(values.description || '').trim(),
              symbols: values.symbols,
              config: configStr || '{}',
            };
            const response = await updateBot(updateInput);
            if (!response.errors) {
              message.success('Bot 修改成功');
              onSuccess?.();
              resolve(true);
            } else {
              message.error(response.errors[0]?.message || '修改失败');
              resolve(false);
            }
          },
          onCancel: () => {
            resolve(false);
          },
        });
      });
    }

    const response = await createBot(input);
    if (!response.errors) {
      message.success('Bot 创建成功');
      onSuccess?.();
      return true;
    } else {
      message.error(response.errors[0]?.message || '创建失败');
      return false;
    }
  };

  const renderInput = (param: StrategyParam) => {
    const renderLabel = () => {
      return (
        <Space size={8}>
          <span style={{ fontWeight: 500 }}>{param.name}</span>
          <Tag color="blue">{param.type}</Tag>
          {param.required && <Tag color="red">必填</Tag>}
          {param.description && (
            <Tooltip title={param.description}>
              <InfoCircleOutlined />
            </Tooltip>
          )}
        </Space>
      );
    };
    switch (param.type) {
      case ParamType.Number:
        return (
          <Flex justify="space-between" align="center" gap={12}>
            <div style={{ minWidth: 220 }}>{renderLabel()}</div>
            <div>
              <ProFormDigit
                name={['params', param.name]}
                placeholder={param.description}
                rules={
                  param.required ? [{ required: true, message: `${param.name} 是必填项` }] : []
                }
                fieldProps={{ style: { width: 200 } }}
                noStyle
              />
            </div>
          </Flex>
        );
      case ParamType.Bool:
        return (
          <Flex justify="space-between" align="center" gap={12}>
            <div style={{ minWidth: 220 }}>{renderLabel()}</div>
            <div>
              <ProFormSelect
                name={['params', param.name]}
                options={[
                  { label: '是', value: true },
                  { label: '否', value: false },
                ]}
                placeholder={param.description}
                rules={
                  param.required ? [{ required: true, message: `${param.name} 是必填项` }] : []
                }
                fieldProps={{ style: { width: 200 } }}
                noStyle
              />
            </div>
          </Flex>
        );
      default:
        return (
          <Flex justify="space-between" align="center" gap={12}>
            <div style={{ minWidth: 220 }}>{renderLabel()}</div>
            <div>
              <ProFormText
                name={['params', param.name]}
                placeholder={param.description}
                rules={
                  param.required ? [{ required: true, message: `${param.name} 是必填项` }] : []
                }
                fieldProps={{ style: { width: 200 } }}
                noStyle
              />
            </div>
          </Flex>
        );
    }
  };

  return (
    <ModalForm
      title={isEdit ? '修改 Bot' : '创建 Bot'}
      open={open}
      onOpenChange={onOpenChange}
      form={form}
      modalProps={{
        destroyOnHidden: true,
        width: 850,
      }}
      onFinish={handleSubmit}
      loading={loading}
    >
      <ProForm.Group>
        <ProFormText
          name="name"
          label="名称"
          rules={[
            { required: true, message: '请输入名称' },
            { max: 20, message: '名称最多20个字符' },
          ]}
          width={260}
          fieldProps={{ maxLength: 20, showCount: true }}
        />
        <ProFormText
          name="description"
          label="描述"
          width={400}
          fieldProps={{ maxLength: 200, showCount: true }}
        />
        <ProFormSelect
          name="strategyId"
          label="策略"
          rules={[{ required: true, message: '请选择策略' }]}
          width={400}
          disabled={isEdit}
          options={strategies.map((s) => ({
            label: `${s.name} (${s.version})`,
            value: s.id,
          }))}
          fieldProps={{
            showSearch: true,
            onChange: (value) => {
              const strategy = strategies.find((s) => s.id === value);
              setSelectedStrategy(strategy || null);
              // 初始化策略参数
              if (strategy && strategy.params && strategy.params.length > 0) {
                const params: Record<string, any> = {};
                strategy.params.forEach((param) => {
                  if (param.default !== undefined && param.default !== null) {
                    if (param.type === ParamType.Number) {
                      params[param.name] = parseFloat(param.default);
                    } else if (param.type === ParamType.Bool) {
                      params[param.name] = param.default === 'true';
                    } else {
                      params[param.name] = param.default;
                    }
                  }
                });
                form.setFieldValue('params', params);
              }
              // 初始化信号绑定
              if (strategy && strategy.signals) {
                const initialSignals: any[] = [];
                strategy.signals
                  .sort((a, b) => Number(a.id) - Number(b.id))
                  .forEach((signal) => {
                    const scope = signal.scope;
                    if (scope === SignalScope.Symbol) {
                      initialSignals.push({
                        signalId: signal.id,
                        scope: scope,
                        bindings: [],
                      });
                    } else if (scope === SignalScope.Target) {
                      initialSignals.push({
                        signalId: signal.id,
                        scope: scope,
                        datasourceId: 0,
                      });
                    } else if (scope === SignalScope.Exchange) {
                      initialSignals.push({
                        signalId: signal.id,
                        scope: scope,
                        bindings: [],
                      });
                    } else {
                      initialSignals.push({
                        signalId: signal.id,
                        scope: scope,
                        datasourceId: 0,
                      });
                    }
                  });
                form.setFieldValue('signals', initialSignals);
              }
            },
          }}
        />

        <>
          <ProFormSelect
            name="mode"
            label="模式"
            disabled={isEdit}
            rules={[{ required: true, message: '请选择运行模式' }]}
            width="xs"
            options={BotModeOptions}
            initialValue={BotMode.Live}
          />

          <Form.Item
            label="交易所"
            name={'exchange'}
            rules={[{ required: true, message: '请选择交易所' }]}
          >
            <Select style={{ width: 180 }} disabled={isEdit} placeholder="选择交易所">
              <Select.Option value="binance">
                <img
                  alt={Exchange.Binance}
                  style={{ display: 'inline', marginLeft: 4 }}
                  width={16}
                  src={getExchangeLogo(Exchange.Binance)}
                />{' '}
                Binance
              </Select.Option>
              <Select.Option value="binance_test">
                <img
                  alt={Exchange.BinanceTest}
                  style={{ display: 'inline', marginLeft: 4 }}
                  width={16}
                  src={getExchangeLogo(Exchange.BinanceTest)}
                />{' '}
                Binance Test
              </Select.Option>
              <Select.Option value="okx">
                <img
                  alt={Exchange.OKX}
                  style={{ display: 'inline', marginLeft: 4 }}
                  width={16}
                  src={getExchangeLogo(Exchange.OKX)}
                />{' '}
                OKX
              </Select.Option>
              <Select.Option value="okx_test">
                <img
                  alt={Exchange.OKXTest}
                  style={{ display: 'inline', marginLeft: 4 }}
                  width={16}
                  src={getExchangeLogo(Exchange.OKXTest)}
                />{' '}
                OKX Test
              </Select.Option>
            </Select>
          </Form.Item>
        </>
      </ProForm.Group>

      <ProForm.Item
        label="交易对配置"
        required
        name="symbols"
        dependencies={['exchange']}
        rules={[
          {
            validator: (_: any, value: string[]) => {
              if (!exchangeValue) {
                return Promise.reject(new Error('请先选择交易所'));
              }
              if (!value || value.length === 0) {
                return Promise.reject(new Error('请选择交易对'));
              }
              if (value.length > 10) {
                return Promise.reject(new Error('最多选择10个交易对'));
              }
              const seen = new Set<string>();
              for (const item of value as string[]) {
                const sym = String(item || '').trim();
                if (!sym) continue;
                if (seen.has(sym)) {
                  return Promise.reject(new Error('不允许添加重复的交易对'));
                }
                seen.add(sym);
              }
              return Promise.resolve();
            },
          },
        ]}
      >
        <Select
          mode="multiple"
          style={{ width: '100%' }}
          placeholder={exchangeValue ? '选择交易对' : '请先选择交易所'}
          loading={loadingSymbols[exchangeValue || ''] || false}
          disabled={!exchangeValue}
          showSearch
          filterOption={(input, option) =>
            ((option?.label as string) ?? '').toLowerCase().includes(input.toLowerCase())
          }
          options={exchangeSymbols[exchangeValue || ''] || []}
          onChange={(values) => {
            if (values.length <= 10) {
              return;
            }
            message.warning('最多选择10个交易对');
            form.setFieldValue('symbols', values.slice(0, 10));
          }}
        />
      </ProForm.Item>

      <>
        {modeValue === BotMode.Live && (
          <ProFormSelect
            name="accountId"
            label="交易账户"
            width={300}
            disabled={isEdit}
            rules={[{ required: true, message: '请选择交易账户（实盘支持真实/测试网账户）' }]}
            options={liveAccountOptions}
            fieldProps={{
              showSearch: true,
              loading: loadingAccounts,
              placeholder: '请选择未占用单Bot账户或多Bot账户',
              filterOption: (input, option) => {
                const id = String(option?.value ?? '');
                const acc = accounts.find((a) => a.id === id);
                if (!acc) return false;
                const hay = `${acc.name} ${getExchangeTitle(acc.exchange)}`.toLowerCase();
                return hay.includes(String(input || '').toLowerCase().trim());
              },
            }}
          />
        )}

        {!isEdit && modeValue === BotMode.Paper && (
          <ProFormSelect
            name="paperAccountSource"
            label="模拟账户来源"
            width={220}
            disabled={isEdit}
            initialValue="existing"
            options={[
              { label: '选择已有模拟账户', value: 'existing' },
              { label: '新建模拟账户', value: 'new' },
            ]}
            rules={[{ required: true, message: '请选择模拟账户来源' }]}
          />
        )}

        {modeValue === BotMode.Paper && (paperAccountSource || 'existing') === 'existing' && (
          <ProFormSelect
            name="accountId"
            label="交易账户"
            width={300}
            disabled={isEdit}
            rules={[{ required: true, message: '请选择交易账户（模拟盘仅支持模拟账户）' }]}
            options={paperAccountOptions}
            fieldProps={{
              showSearch: true,
              loading: loadingAccounts,
              placeholder: '请选择未被占用的模拟账户',
              filterOption: (input, option) =>
                ((option?.label as string) ?? '').toLowerCase().includes(input.toLowerCase()),
            }}
          />
        )}

        {needsLiveSubAllocation && (
          <Card
            size="small"
            title="父账户资产概览"
            loading={loadingUnallocated}
            style={{ marginBottom: 12 }}
          >
            <Table<AccountUnallocatedAsset>
              size="small"
              pagination={false}
              rowKey={(r) => `${r.asset}-${r.walletType}`}
              dataSource={unallocatedRows}
              columns={[
                { title: '资产', dataIndex: 'asset' },
                { title: '钱包', dataIndex: 'walletType' },
                { title: '总量', dataIndex: 'parentTotal' },
                { title: '已分配', dataIndex: 'subsAllocated' },
                { title: '可分配', dataIndex: 'unallocated' },
              ]}
            />
          </Card>
        )}

        {((!isEdit &&
          modeValue === BotMode.Paper &&
          (paperAccountSource || 'existing') === 'new') ||
          needsLiveSubAllocation) && (
          <ProForm.Item
            label={needsLiveSubAllocation ? '子账户初始资产（实盘共享）' : '初始资产配置'}
            required
          >
            <Card>
              <Row gutter={16}>
                <Col span={6}>资产：</Col>
                <Col span={4}>钱包类型：</Col>
                <Col span={6}>总量：</Col>
                <Col span={2}></Col>
              </Row>
              <Form.List
                name="initialAssets"
                rules={[
                  {
                    validator: (_, value) => {
                      if (!value || value.length === 0) {
                        return Promise.reject(new Error('请添加初始资产'));
                      }
                      if (!validateInitialAssetsAgainstSymbols(value)) {
                        return Promise.reject(
                          new Error('初始资产和交易对不匹配，缺失相关资产，请检查'),
                        );
                      }
                      const list = ((value || []) as unknown[]).map(coerceInitialAssetRow);
                      const seen = new Set<string>();
                      for (const item of list) {
                        const asset = String(item.asset || '')
                          .trim()
                          .toUpperCase();
                        const walletType = walletTypeKey(item?.walletType);
                        if (!asset || !walletType) {
                          continue;
                        }
                        const key = multiBotAllocKey(asset, walletType);
                        if (seen.has(key)) {
                          return Promise.reject(new Error('资产与钱包类型组合需唯一'));
                        }
                        seen.add(key);
                      }
                      // 用父账户未分配表格同源数据校验，避免在规则里 async 请求导致 Form.List 异常；提交仍会拉接口兜底
                      if (
                        needsLiveSubAllocation &&
                        !loadingUnallocated &&
                        String(form.getFieldValue('accountId') ?? '') ===
                        String(accountIdWatch ?? '')
                      ) {
                        const unallocErr = checkInitialAssetsAgainstUnallocatedRows(
                          unallocatedRows,
                          list,
                        );
                        if (unallocErr) {
                          return Promise.reject(new Error(unallocErr));
                        }
                      }
                      return Promise.resolve();
                    },
                  },
                ]}
              >
                {(fields, { add, remove }, { errors }) => (
                  <>
                    {fields.map(({ key, name, ...restField }) => (
                      <Row key={key} gutter={16} style={{ marginBottom: 8 }}>
                        <Col span={6}>
                          <Form.Item
                            {...restField}
                            name={[name, 'asset']}
                            rules={[{ required: true, message: '请输入资产' }]}
                          >
                            <Input placeholder="USDT" />
                          </Form.Item>
                        </Col>
                        <Col span={4}>
                          <Form.Item
                            {...restField}
                            name={[name, 'walletType']}
                            dependencies={['exchange']}
                            rules={[
                              { required: true, message: '请选择钱包类型' },
                              {
                                validator: (_, value) => {
                                  if (!value || !exchangeValue) {
                                    return Promise.resolve();
                                  }
                                  const allowed = getAllowedWalletTypes(exchangeValue);
                                  if (!allowed.includes(value)) {
                                    return Promise.reject(new Error('该交易所不支持此钱包类型'));
                                  }
                                  return Promise.resolve();
                                },
                              },
                            ]}
                          >
                            <Select options={walletTypeOptions} />
                          </Form.Item>
                        </Col>
                        <Col span={6}>
                          <Form.Item
                            {...restField}
                            name={[name, 'total']}
                            rules={[{ required: true, message: '请输入总量' }]}
                          >
                            <Input type="number" min={0} step="0.0001" placeholder="1000" />
                          </Form.Item>
                        </Col>
                        <Col span={2}>
                          <Button
                            type="link"
                            danger
                            icon={<DeleteOutlined />}
                            onClick={() => {
                              if (fields.length === 1) {
                                form.setFieldValue('initialAssets', [{ ...EMPTY_BOT_INITIAL_ASSET_ROW }]);
                                return;
                              }
                              remove(name);
                            }}
                          >
                            删除
                          </Button>
                        </Col>
                      </Row>
                    ))}
                    <Button
                      type="dashed"
                      block
                      icon={<PlusOutlined />}
                      onClick={() => add({ ...EMPTY_BOT_INITIAL_ASSET_ROW })}
                    >
                      添加资产
                    </Button>
                    <Form.ErrorList errors={errors?.slice(0, 1)} />
                  </>
                )}
              </Form.List>
            </Card>
          </ProForm.Item>
        )}
      </>

      {selectedStrategy && selectedStrategy.params && selectedStrategy.params.length > 0 && (
        <ProForm.Item>
          <Collapse
            items={[
              {
                key: 'params',
                label: '策略参数',
                styles: { body: { paddingBottom: 2 } },
                forceRender: true,
                children: (
                  <div style={{ paddingBottom: 0 }}>
                    {selectedStrategy.params.map((param: StrategyParam) => {
                      return (
                        <div key={param.name} style={{ marginBottom: 16 }}>
                          {renderInput(param)}
                        </div>
                      );
                    })}
                  </div>
                ),
              },
            ]}
          />
        </ProForm.Item>
      )}

      <ProForm.Item>
        <Collapse
          items={[
            {
              key: 'runtime',
              label: '运行时配置',
              styles: { body: { paddingBottom: 2 } },
              forceRender: true,
              children: (
                <Row gutter={16}>
                  <Col span={8}>
                    <ProFormDigit
                      name={['runtime', 'signalTimeoutMs']}
                      label="Signal 超时(ms)"
                      min={1000}
                      fieldProps={{ precision: 0 }}
                      tooltip="单次 onInit/onSignal 的最大执行时间，需要覆盖 AI 调用等待时间"
                      rules={[{ required: true, message: '请输入 Signal 超时' }]}
                    />
                  </Col>
                  <Col span={8}>
                    <ProFormDigit
                      name={['runtime', 'aiTimeoutMs']}
                      label="AI 默认超时(ms)"
                      min={1000}
                      fieldProps={{ precision: 0 }}
                      tooltip="策略未在 ai.complete 中指定 timeoutMs 时使用"
                      rules={[{ required: true, message: '请输入 AI 默认超时' }]}
                    />
                  </Col>
                  <Col span={8}>
                    <ProFormDigit
                      name={['runtime', 'maxAITimeoutMs']}
                      label="AI 最大超时(ms)"
                      min={1000}
                      fieldProps={{ precision: 0 }}
                      tooltip="限制策略在 ai.complete 中可指定的最大 timeoutMs"
                      rules={[
                        { required: true, message: '请输入 AI 最大超时' },
                        {
                          validator: async (_: unknown, value: unknown) => {
                            const signalTimeoutMs = Number(
                              form.getFieldValue(['runtime', 'signalTimeoutMs']),
                            );
                            const aiTimeoutMs = Number(form.getFieldValue(['runtime', 'aiTimeoutMs']));
                            const maxAITimeoutMs = Number(value);
                            if (
                              Number.isFinite(aiTimeoutMs) &&
                              Number.isFinite(maxAITimeoutMs) &&
                              aiTimeoutMs > maxAITimeoutMs
                            ) {
                              throw new Error('AI 默认超时不能大于 AI 最大超时');
                            }
                            if (
                              Number.isFinite(signalTimeoutMs) &&
                              Number.isFinite(maxAITimeoutMs) &&
                              maxAITimeoutMs > signalTimeoutMs
                            ) {
                              throw new Error('AI 最大超时不能大于 Signal 超时');
                            }
                          },
                        },
                      ]}
                    />
                  </Col>
                </Row>
              ),
            },
          ]}
        />
      </ProForm.Item>

      {selectedStrategy && selectedStrategy.signals && selectedStrategy.signals.length > 0 && (
        <ProForm.Item>
          <Collapse
            items={[
              {
                key: 'signals',
                label: '信号数据源',
                styles: { body: { paddingBottom: 2 } },
                forceRender: true,
                children: (
                  <>
                    <Form.List name="signals">
                      {(fields) => (
                        <>
                          {fields.map(({ key, name, ...restField }) => {
                            const binding = form.getFieldValue(['signals', name]);
                            const signal = selectedStrategy.signals.find(
                              (s) => s.id === binding?.signalId,
                            );
                            if (!signal) {
                              return null;
                            }
                            const scope = signal.scope;
                            const isMarketSignal = IsMarketSignal(signal.type);

                            return (
                              <React.Fragment key={key}>
                                <Form.Item noStyle>
                                  <div style={{ marginBottom: 16 }}>
                                    <Row gutter={16} align="middle">
                                      <Col flex="auto">
                                        <Space>
                                          <Tag color="blue">{signal.id}</Tag>
                                          <Tag>
                                            {SignalTypeOptions.find(
                                              (opt) => opt.value === signal.type,
                                            )?.label || signal.type}
                                          </Tag>
                                          {scope && (
                                            <Tag color="purple">
                                              <Space>
                                                {scope === SignalScope.Symbol
                                                  ? 'Symbol'
                                                  : scope === SignalScope.Target
                                                    ? 'Target'
                                                    : scope === SignalScope.Exchange
                                                      ? 'Exchange'
                                                      : 'Strategy'}
                                                <Tooltip
                                                  title={
                                                    scope === SignalScope.Symbol
                                                      ? '该信号针对每个交易对生效'
                                                      : scope === SignalScope.Target
                                                        ? '该信号针对指定交易对生效'
                                                        : scope === SignalScope.Exchange
                                                          ? '该信号针对每个交易所生效'
                                                          : '该信号在整个策略范围内生效'
                                                  }
                                                >
                                                  <InfoCircleOutlined />
                                                </Tooltip>
                                              </Space>
                                            </Tag>
                                          )}
                                          {signal.props && <Tag>{signal.props}</Tag>}
                                        </Space>
                                      </Col>
                                    </Row>
                                    <Form.Item {...restField} name={[name, 'signalId']} noStyle>
                                      <Input hidden />
                                    </Form.Item>
                                    <Form.Item {...restField} name={[name, 'scope']} noStyle>
                                      <Input hidden />
                                    </Form.Item>

                                    {scope === SignalScope.Symbol && (
                                      <div style={{ marginTop: 6 }}>
                                        {binding?.bindings?.map(
                                          (bindingItem: any, bindingIndex: number) => (
                                            <Row
                                              key={bindingIndex}
                                              gutter={16}
                                              align="middle"
                                              style={{ marginBottom: 4 }}
                                            >
                                              <Col span={10}>
                                                <Tag>{getExchangeTitle(bindingItem.exchange)}</Tag>
                                                <Tag>{bindingItem.symbol}</Tag>
                                              </Col>
                                              <Col span={14}>
                                                {isMarketSignal && (
                                                  <Tag color="green">自动（后端加载）</Tag>
                                                )}
                                              </Col>
                                            </Row>
                                          ),
                                        )}
                                      </div>
                                    )}
                                    {scope === SignalScope.Target && (
                                      <div style={{ marginTop: 6 }}>
                                        <Row gutter={16} align="middle">
                                          <Col span={10}>
                                            <Tag>
                                              {getExchangeTitle(signal.exchange as Exchange)}
                                            </Tag>
                                            <Tag>{signal.symbol}</Tag>
                                          </Col>
                                          <Col span={14}>
                                            {isMarketSignal && (
                                              <Tag color="green">自动（后端加载）</Tag>
                                            )}
                                          </Col>
                                        </Row>
                                      </div>
                                    )}
                                    {scope === SignalScope.Exchange && (
                                      <div style={{ marginTop: 6 }}>
                                        {binding?.bindings?.map(
                                          (bindingItem: any, bindingIndex: number) => (
                                            <Row
                                              key={bindingIndex}
                                              gutter={16}
                                              align="middle"
                                              style={{ marginBottom: 4 }}
                                            >
                                              <Col span={10}>
                                                <Tag>{getExchangeTitle(bindingItem.exchange)}</Tag>
                                                {bindingItem.symbol && (
                                                  <Tag>{bindingItem.symbol}</Tag>
                                                )}
                                              </Col>
                                              <Col span={14}>
                                                {isMarketSignal && (
                                                  <Tag color="green">自动（后端加载）</Tag>
                                                )}
                                              </Col>
                                            </Row>
                                          ),
                                        )}
                                      </div>
                                    )}
                                    {scope === SignalScope.Strategy && (
                                      <div style={{ marginTop: 6 }}>
                                        {isMarketSignal && (
                                          <Tag color="green">自动（后端加载）</Tag>
                                        )}
                                      </div>
                                    )}
                                  </div>
                                </Form.Item>
                              </React.Fragment>
                            );
                          })}
                        </>
                      )}
                    </Form.List>
                  </>
                ),
              },
            ]}
          />
        </ProForm.Item>
      )}
    </ModalForm>
  );
};

export default BotModal;
