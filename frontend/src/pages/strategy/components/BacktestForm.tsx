import { Exchange, MarketType } from '@/global.types';
import { WalletType } from '@/services/gateway/account';
import * as api from '@/services/gateway/api';
import {
  BacktestSignal,
  DataSource,
  IsMarketSignal,
  ParamType,
  queryDatasources,
  runBacktest,
  RunBacktestInput,
  RunBacktestResponse,
  SignalDefinition,
  SignalScope,
  SignalType,
  SignalTypeOptions,
  Strategy,
  StrategyParam,
} from '@/services/gateway/strategy';
import utils from '@/utils';
import { getExchangeLogo, parseSymbol } from '@/utils/market';
import { getWalletTypeLabel } from '@/utils/marketTag';
import { DeleteOutlined, InfoCircleOutlined, PlusOutlined } from '@ant-design/icons';
import {
  ProForm,
  ProFormDateTimeRangePicker,
  ProFormDigit,
  ProFormSelect,
  ProFormText,
} from '@ant-design/pro-components';
import {
  Button,
  Card,
  Col,
  Collapse,
  Divider,
  Empty,
  Flex,
  Form,
  Input,
  message,
  Row,
  Select,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import React, { useEffect, useRef, useState } from 'react';
import BacktestResult from './BacktestResult';

type BacktestFormProps = {
  strategy?: Strategy;
  runType?: number; // 0: use strategy object, 1: use strategyId and version
};

type BacktestInitialAssetRow = {
  asset: string;
  walletType?: WalletType;
  total?: string | number;
  frozen?: string;
};

const EMPTY_INITIAL_ASSET_ROW: BacktestInitialAssetRow = {
  asset: '',
  walletType: undefined,
  total: undefined,
  frozen: '0',
};

const defaultBacktestInitialAssets = (exchange?: string): BacktestInitialAssetRow[] => {
  if (exchange === Exchange.Binance || exchange === Exchange.BinanceTest) {
    return [{ asset: 'USDT', walletType: WalletType.Spot, total: '1000', frozen: '0' }];
  }
  return [{ asset: 'USDT', walletType: WalletType.Trade, total: '1000', frozen: '0' }];
};

const getAllowedWalletTypes = (exchange?: string): WalletType[] => {
  if (exchange === Exchange.Binance || exchange === Exchange.BinanceTest) {
    return [WalletType.Spot, WalletType.Future];
  }
  if (exchange === Exchange.OKX || exchange === Exchange.OKXTest) {
    return [WalletType.Trade];
  }
  return [WalletType.Spot, WalletType.Future, WalletType.Fund, WalletType.Trade, WalletType.Margin];
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

const validateInitialAssetsAgainstSymbols = (
  exchange: string | undefined,
  symbols: string[],
  assets: BacktestInitialAssetRow[],
) => {
  if (!exchange || !symbols?.length || !assets?.length) {
    return true;
  }
  const assetsByWallet: Record<string, Set<string>> = {};
  assets.forEach((item) => {
    const asset = String(item?.asset || '')
      .trim()
      .toUpperCase();
    const walletType = String(item?.walletType || '').trim();
    if (!asset || !walletType) return;
    if (!assetsByWallet[walletType]) {
      assetsByWallet[walletType] = new Set<string>();
    }
    assetsByWallet[walletType].add(asset);
  });

  for (const symbol of symbols) {
    if (!symbol) continue;
    const parsed = parseSymbol(String(symbol));
    if (!parsed.base || !parsed.quote) {
      return false;
    }
    const walletType = getSymbolWalletType(exchange, parsed.type);
    const assetSet = walletType ? assetsByWallet[walletType] || new Set<string>() : new Set<string>();
    if (parsed.type === MarketType.Future) {
      if (!assetSet.has(parsed.quote)) {
        return false;
      }
    } else if (!assetSet.has(parsed.base) && !assetSet.has(parsed.quote)) {
      return false;
    }
  }
  return true;
};

// Exchange 级别信号绑定项组件
type ExchangeBindingItemProps = {
  bindingIndex: number;
  bindingItem: any;
  name: number;
  restField: any;
  form: any;
  signal: SignalDefinition;
  isMarketSignal: boolean;
  exchangeSymbols: Record<string, { label: string; value: string }[]>;
  loadingSymbols: Record<string, boolean>;
  datasources: DataSource[];
  loadingDatasources: boolean;
  onLoadSymbols: (exchange: string) => void;
  getFilteredDatasources: (
    signal: SignalDefinition,
    exchange?: string,
    symbol?: string,
  ) => DataSource[];
  buildDataSourceLabel: (ds: DataSource) => string;
};

const ExchangeBindingItem: React.FC<ExchangeBindingItemProps> = ({
  bindingIndex,
  bindingItem,
  name,
  restField,
  form,
  signal,
  isMarketSignal,
  exchangeSymbols,
  loadingSymbols,
  loadingDatasources,
  onLoadSymbols,
  getFilteredDatasources,
  buildDataSourceLabel,
}) => {
  const exchangeValue = bindingItem.exchange;
  const symbolValue =
    Form.useWatch([name, 'bindings', bindingIndex, 'symbol'], form) || bindingItem.symbol;

  // 当交易所变化时，加载对应的交易对列表
  useEffect(() => {
    if (exchangeValue && !exchangeSymbols[exchangeValue]) {
      onLoadSymbols(exchangeValue);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [exchangeValue]);

  if (!isMarketSignal) {
    return (
      <Row gutter={16} align="middle" style={{ marginBottom: 4 }}>
        <Col span={10}>
          <Tag>{utils.market.getExchangeTitle(bindingItem.exchange)}</Tag>
        </Col>
      </Row>
    );
  }

  return (
    <Row gutter={16} align="middle" style={{ marginBottom: 4 }}>
      <Col span={10}>
        <Flex>
          <Tag>{utils.market.getExchangeTitle(bindingItem.exchange)}</Tag>
          <Form.Item
            {...restField}
            name={[name, 'bindings', bindingIndex, 'symbol']}
            style={{ marginBottom: 8 }}
            rules={[{ required: true, message: '请选择交易对' }]}
            noStyle
          >
            <Select
              style={{ width: 180 }}
              placeholder="选择交易对"
              loading={loadingSymbols[exchangeValue || ''] || false}
              disabled={!exchangeValue}
              showSearch
              allowClear
              size="small"
              filterOption={(input, option) =>
                ((option?.label as string) ?? '').toLowerCase().includes(input.toLowerCase())
              }
              onChange={() => {
                // 当 symbol 变化时，清空数据源选择，让用户重新选择
                form.setFieldValue([name, 'bindings', bindingIndex, 'datasourceId'], 0);
              }}
            >
              {(exchangeSymbols[exchangeValue || ''] || [])?.map((option) => (
                <Select.Option key={option.value} value={option.value} label={option.label}>
                  <Tooltip title={option.label}>{option.label}</Tooltip>
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
        </Flex>
      </Col>
      <Col span={14}>
        <Form.Item
          {...restField}
          name={[name, 'bindings', bindingIndex, 'datasourceId']}
          noStyle
          initialValue={0}
        >
          <Select
            style={{ width: '100%' }}
            placeholder="选择数据源"
            loading={loadingDatasources}
            size="small"
          >
            <Select.Option key="auto" value={0}>
              自动（后端加载）
            </Select.Option>
            {getFilteredDatasources(signal, exchangeValue, symbolValue).map((ds) => (
              <Select.Option key={ds.id} value={ds.id}>
                <Tooltip title={buildDataSourceLabel(ds)}>{buildDataSourceLabel(ds)}</Tooltip>
              </Select.Option>
            ))}
          </Select>
        </Form.Item>
      </Col>
    </Row>
  );
};

const BacktestForm: React.FC<BacktestFormProps> = (props) => {
  const { strategy, runType = 0 } = props;
  const [form] = Form.useForm();
  const [datasources, setDatasources] = useState<DataSource[]>([]);
  const [loadingDatasources, setLoadingDatasources] = useState(false);
  // 每个交易所对应的交易对选项
  const [exchangeSymbols, setExchangeSymbols] = useState<
    Record<string, { label: string; value: string }[]>
  >({});
  const [loadingSymbols, setLoadingSymbols] = useState<Record<string, boolean>>({});
  // 回测结果相关状态
  const [backtestResult, setBacktestResult] = useState<RunBacktestResponse | null>(null);
  const [backtestLoading, setBacktestLoading] = useState(false);
  // 用于取消回测请求的 AbortController
  const abortControllerRef = useRef<AbortController | null>(null);
  const exchangeValue = Form.useWatch('exchange', form);
  const prevExchangeRef = useRef<string | undefined>(exchangeValue);
  const walletTypeOptions = getAllowedWalletTypes(exchangeValue).map((value) => ({
    value,
    label: getWalletTypeLabel(value),
  }));

  useEffect(() => {
    // Load datasources
    loadDatasources();
    // Initialize form
    const params: Record<string, any> = {};
    if (strategy?.params && strategy?.params.length > 0) {
      strategy?.params.forEach((param) => {
        if (param.default !== undefined && param.default !== null) {
          if (param.type === ParamType.Number) {
            params[param.name] = parseFloat(param.default);
          } else if (param.type === ParamType.Bool) {
            params[param.name] = param.default === 'true';
          } else {
            params[param.name] = param.default;
          }
        } else {
          params[param.name] = undefined;
        }
      });
    }
    // 初始化信号绑定，根据作用域创建不同的结构
    const initialSignals: any[] = [];
    // signals 按序号排序
    strategy?.signals
      .sort((a, b) => Number(a.id) - Number(b.id))
      .forEach((signal) => {
        const scope = signal.scope;
        if (scope === SignalScope.Symbol) {
          // Symbol 级别：为每个 symbol 创建绑定项（初始时 symbols 还没有，会在 symbols 变化时更新）
          // 这里先创建一个占位符，实际绑定会在 symbols 变化时更新
          initialSignals.push({
            signalId: signal.id,
            scope: scope,
            bindings: [],
          });
        } else if (scope === SignalScope.Target) {
          // Target 级别：针对策略中指定的具体 symbol（固定）
          // 只创建一个绑定项，使用信号定义中的 exchange 和 symbol
          initialSignals.push({
            signalId: signal.id,
            scope: scope,
            datasourceId: 0, // 默认为自动
          });
        } else if (scope === SignalScope.Exchange) {
          // Exchange 级别：为每个交易所创建绑定项（初始时 symbols 还没有，会在 symbols 变化时更新）
          initialSignals.push({
            signalId: signal.id,
            scope: scope,
            bindings: [],
          });
        } else {
          // Strategy 级别：只创建一个绑定项
          initialSignals.push({
            signalId: signal.id,
            scope: scope,
            datasourceId: 0, // 默认为自动
          });
        }
      });

    form.setFieldsValue({
      dateRange: [dayjs().subtract(1, 'day').startOf('day'), dayjs().startOf('day')],
      exchange: Exchange.Binance,
      symbols: ['ETH/USDT:SPOT'],
      initialAssets: defaultBacktestInitialAssets(Exchange.Binance),
      signals: initialSignals,
      params: params,
    });
    setExchangeSymbols({});
    setLoadingSymbols({});
    setBacktestResult(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [strategy]);

  // 组件卸载时取消进行中的回测
  useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
        abortControllerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    if (!exchangeValue) {
      return;
    }
    loadSymbolsForExchange(exchangeValue);
    if (prevExchangeRef.current && prevExchangeRef.current !== exchangeValue) {
      form.setFieldsValue({
        symbols: [],
        initialAssets: defaultBacktestInitialAssets(exchangeValue),
      });
    }
    prevExchangeRef.current = exchangeValue;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [exchangeValue]);

  // 监听 symbols 变化，更新信号绑定结构
  const symbolsValue = Form.useWatch('symbols', form) as string[] | undefined;
  useEffect(() => {
    if (!symbolsValue || symbolsValue.length === 0 || !exchangeValue) {
      return;
    }

    const currentSignals = form.getFieldValue('signals') || [];
    const updatedSignals = strategy?.signals.map((signal) => {
      const scope = signal.scope;
      const existingSignal = currentSignals.find((s: any) => s.signalId === signal.id);

      if (scope === SignalScope.Symbol) {
        const bindings = (symbolsValue as string[])
          .filter((symbol) => symbol)
          .map((symbol) => {
            const existingBinding = existingSignal?.bindings?.find(
              (b: any) => b.exchange === exchangeValue && b.symbol === symbol,
            );
            return {
              exchange: exchangeValue,
              symbol,
              datasourceId:
                existingBinding?.datasourceId !== undefined ? existingBinding.datasourceId : 0,
            };
          });
        return {
          signalId: signal.id,
          scope: scope,
          bindings: bindings,
        };
      } else if (scope === SignalScope.Target) {
        // Target 级别：针对策略中指定的具体 symbol（固定）
        // 只创建一个绑定项，使用信号定义中的 exchange 和 symbol
        return (
          existingSignal || {
            signalId: signal.id,
            scope: scope,
            datasourceId: 0, // 默认为自动
          }
        );
      } else if (scope === SignalScope.Exchange) {
        const existingBinding = existingSignal?.bindings?.find(
          (b: any) => b.exchange === exchangeValue,
        );
        const bindings = [
          {
            exchange: exchangeValue,
            symbol: existingBinding?.symbol !== undefined ? existingBinding.symbol : undefined,
            datasourceId:
              existingBinding?.datasourceId !== undefined ? existingBinding.datasourceId : 0,
          },
        ];
        return {
          signalId: signal.id,
          scope: scope,
          bindings: bindings,
        };
      } else {
        // Strategy 级别：保持原有绑定
        return (
          existingSignal || {
            signalId: signal.id,
            scope: scope,
            datasourceId: 0, // 默认为自动
          }
        );
      }
    });

    form.setFieldsValue({ signals: updatedSignals });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [symbolsValue, strategy?.signals, exchangeValue]);

  const loadDatasources = async () => {
    setLoadingDatasources(true);
    try {
      const res = await queryDatasources({
        current: 1,
        pageSize: 1000,
      });
      if (res?.list) {
        setDatasources(res.list);
      }
    } finally {
      setLoadingDatasources(false);
    }
  };

  const getFilteredDatasources = (
    signal: SignalDefinition,
    exchange?: string,
    symbol?: string,
  ): DataSource[] => {
    return datasources.filter((ds: DataSource) => {
      if (signal.type !== ds.type) return false;
      if (signal.type == SignalType.Kline) {
        if (!ds.props || !signal.props) return false;
        const dsProps = JSON.parse(ds.props);
        const signalProps = JSON.parse(signal.props);
        if (dsProps.interval !== signalProps.interval) return false;
      }
      // 如果提供了 exchange 和 symbol，进行精确匹配（Symbol 级别）
      if (exchange && symbol) {
        if (ds.exchange !== exchange || ds.symbol !== symbol) return false;
      } else if (exchange) {
        // 只提供了 exchange（Exchange 级别），如果也提供了 symbol，则进行精确匹配；否则 symbol 应该为空
        if (ds.exchange !== exchange) return false;
        if (symbol) {
          // 如果提供了 symbol，则必须匹配
          if (ds.symbol !== symbol) return false;
        } else {
          // 如果没有提供 symbol，则数据源的 symbol 应该为空
          if (ds.symbol && ds.symbol !== '') return false;
        }
      } else {
        // Strategy 级别，不进行 exchange/symbol 过滤
        if (signal.exchange && ds.exchange !== signal.exchange) return false;
        if (signal.symbol && ds.symbol !== signal.symbol) return false;
      }
      return true;
    });
  };

  const loadSymbolsForExchange = async (exchange: string) => {
    if (!exchange) {
      return;
    }

    // 如果已经加载过，直接返回
    if (exchangeSymbols[exchange]) {
      return;
    }

    // 使用函数式更新来设置 loading 状态，避免闭包问题
    setLoadingSymbols((prevLoading) => ({ ...prevLoading, [exchange]: true }));
    try {
      const markets = await api.queryMarkets({ exchange });
      if (markets && markets.length > 0) {
        const symbolOptions = markets.map((market: any) => ({
          label: market.symbol,
          value: market.symbol,
        }));
        // 使用函数式更新，确保使用最新的状态
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
    const [startTime, endTime] = values.dateRange || [];
    if (!startTime || !endTime) {
      message.error('请选择时间范围');
      return false;
    }

    // Build params JSON
    const paramsJson = JSON.stringify(values?.params || {});

    // 根据信号作用域生成 BacktestSignal 数组
    const backtestSignals: BacktestSignal[] = [];
    if (values.signals) {
      values.signals.forEach((formSignal: any) => {
        const signal = strategy?.signals.find((s) => s.id === formSignal.signalId);
        if (!signal) return;

        const scope = signal.scope;
        if (scope === SignalScope.Symbol) {
          // Symbol 级别：为每个 symbol 生成一个 BacktestSignal
          if (formSignal.bindings) {
            formSignal.bindings.forEach((binding: any) => {
              // 包括 0（自动）在内的所有值都传递
              if (binding.datasourceId !== undefined && binding.datasourceId !== null) {
                backtestSignals.push({
                  signalId: signal.id,
                  datasourceId: String(binding.datasourceId), // 保持字符串类型以匹配前端类型定义
                  exchange: binding.exchange,
                  symbol: binding.symbol,
                });
              }
            });
          }
        } else if (scope === SignalScope.Target) {
          // Target 级别：只生成一个 BacktestSignal（针对策略中指定的具体 symbol）
          // 包括 0（自动）在内的所有值都传递
          if (formSignal.datasourceId !== undefined && formSignal.datasourceId !== null) {
            backtestSignals.push({
              signalId: signal.id,
              datasourceId: String(formSignal.datasourceId), // 保持字符串类型以匹配前端类型定义
              exchange: signal.exchange,
              symbol: signal.symbol,
            });
          }
        } else if (scope === SignalScope.Exchange) {
          // Exchange 级别：为每个交易所生成一个 BacktestSignal
          if (formSignal.bindings) {
            formSignal.bindings.forEach((binding: any) => {
              // 包括 0（自动）在内的所有值都传递
              if (binding.datasourceId !== undefined && binding.datasourceId !== null) {
                backtestSignals.push({
                  signalId: signal.id,
                  datasourceId: String(binding.datasourceId), // 保持字符串类型以匹配前端类型定义
                  exchange: binding.exchange,
                  symbol: binding.symbol, // 如果选择了 symbol，则传递
                });
              }
            });
          }
        } else {
          // Strategy 级别：只生成一个 BacktestSignal
          // 包括 0（自动）在内的所有值都传递
          if (formSignal.datasourceId !== undefined && formSignal.datasourceId !== null) {
            backtestSignals.push({
              signalId: signal.id,
              datasourceId: String(formSignal.datasourceId), // 保持字符串类型以匹配前端类型定义
            });
          }
        }
      });
    }

    const initialAssets = (values.initialAssets || []).map((row: BacktestInitialAssetRow) => ({
      asset: String(row.asset || '').trim(),
      walletType: row.walletType as string,
      total: String(row.total ?? ''),
      frozen: row.frozen !== undefined && row.frozen !== null ? String(row.frozen) : '0',
    }));

    if (!validateInitialAssetsAgainstSymbols(values.exchange, values.symbols || [], initialAssets)) {
      message.error('初始资产和交易对不匹配，缺失相关资产，请检查');
      return false;
    }

    const input: RunBacktestInput = {
      strategy: runType === 0 ? strategy : undefined,
      strategyId: runType === 1 ? strategy?.id : undefined,
      version: runType === 1 ? strategy?.version : undefined,
      runType: runType,
      startTime: dayjs(startTime).unix(),
      endTime: dayjs(endTime).unix(),
      exchange: values.exchange,
      symbols: (values.symbols || []).filter((sym: string) => sym),
      initialAssets,
      params: paramsJson,
      signals: backtestSignals,
    };

    // 创建新的 AbortController
    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    setBacktestLoading(true);
    try {
      const res = await runBacktest(
        {
          strategy: input.strategy,
          strategyId: input.strategyId,
          version: input.version,
          runType: input.runType,
          startTime: input.startTime,
          endTime: input.endTime,
          exchange: input.exchange,
          symbols: input.symbols,
          initialAssets: input.initialAssets,
          params: input.params,
          signals: input.signals,
        },
        abortController.signal,
      );
      if (!res.errors && res.data?.Result) {
        setBacktestResult(res.data.Result);
        message.success('回测完成');
        return true;
      }
    } catch (error: any) {
      // 如果是用户主动取消，不显示错误消息
      if (error?.name === 'AbortError' || error?.message?.includes('aborted')) {
        message.info('回测已终止');
        return false;
      }
      // 其他错误已经在 errorHandler 中处理
    } finally {
      setBacktestLoading(false);
      abortControllerRef.current = null;
    }
    return false;
  };

  const renderInput = (param: StrategyParam) => {
    const renderLabel = () => {
      return (
        <Space style={{ marginBottom: 8 }}>
          <span style={{ fontWeight: 500 }}>{param.name}</span>
          <Tag color="blue">{param.type}</Tag>
          {param.required && <Tag color="red">必填</Tag>}
          <Typography.Text type="secondary">{param.description}</Typography.Text>
        </Space>
      );
    };
    switch (param.type) {
      case ParamType.Number:
        return (
          <>
            {renderLabel()}
            <ProFormDigit
              name={['params', param.name]}
              placeholder={param.description}
              rules={param.required ? [{ required: true, message: `${param.name} 是必填项` }] : []}
              noStyle
            />
          </>
        );
      case ParamType.Bool:
        return (
          <>
            {renderLabel()}
            <ProFormSelect
              name={['params', param.name]}
              label={renderLabel()}
              options={[
                { label: '是', value: true },
                { label: '否', value: false },
              ]}
              placeholder={param.description}
              rules={param.required ? [{ required: true, message: `${param.name} 是必填项` }] : []}
              noStyle
            />
          </>
        );
      default:
        return (
          <>
            {renderLabel()}
            <ProFormText
              name={['params', param.name]}
              label={renderLabel()}
              placeholder={param.description}
              rules={param.required ? [{ required: true, message: `${param.name} 是必填项` }] : []}
              noStyle
            />
          </>
        );
    }
  };

  const buildDataSourceLabel = (ds: DataSource) => {
    let propsStr = ds.props;
    if (ds.props) {
      try {
        const propsObj = JSON.parse(ds.props);
        const sortedKeys = Object.keys(propsObj).sort();
        propsStr = sortedKeys.map((key) => String(propsObj[key])).join('/');
      } catch (e) {}
    }
    return `${ds.name || ds.id} (${ds.exchange}:${ds.symbol}) - ${ds.type} - ${propsStr}`;
  };

  // 终止回测
  const handleCancelBacktest = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
      setBacktestLoading(false);
      message.info('正在终止回测...');
    }
  };

  const disabledFutureDate = (current: dayjs.Dayjs) => {
    // 禁止选择未来日期（包含今天之后）
    return current && current.isAfter(dayjs(), 'day');
  };

  const disabledFutureTime = (date: dayjs.Dayjs | null) => {
    // 仅当选择“今天”时，限制时分秒不能超过当前时间
    if (!date) return {};
    const curNow = dayjs();
    if (!date.isSame(curNow, 'day')) return {};

    const curHour = curNow.hour();
    const curMinute = curNow.minute();
    const curSecond = curNow.second();

    return {
      disabledHours: () => Array.from({ length: 24 }, (_, h) => h).filter((h) => h > curHour),
      disabledMinutes: (selectedHour: number) => {
        if (selectedHour !== curHour) return [];
        return Array.from({ length: 60 }, (_, m) => m).filter((m) => m > curMinute);
      },
      disabledSeconds: (selectedHour: number, selectedMinute: number) => {
        if (selectedHour !== curHour || selectedMinute !== curMinute) return [];
        return Array.from({ length: 60 }, (_, s) => s).filter((s) => s > curSecond);
      },
    };
  };

  return (
    <ProForm
      form={form}
      onFinish={handleSubmit}
      submitter={{
        render: (props, dom) => {
          // 检查是否有任何交易对正在加载
          const isAnySymbolLoading = Object.values(loadingSymbols).some((loading) => loading);
          // 如果有数据源或交易对正在加载，禁用按钮
          const isDataLoading = loadingDatasources || isAnySymbolLoading;

          return (
            <Space style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              {backtestLoading ? (
                <Button danger onClick={handleCancelBacktest}>
                  终止回测
                </Button>
              ) : (
                <Button
                  type="primary"
                  loading={backtestLoading || isDataLoading}
                  disabled={isDataLoading}
                  onClick={() => props.submit()}
                >
                  开始回测
                </Button>
              )}
            </Space>
          );
        },
      }}
    >
      <div style={{ marginBottom: 16 }}>
        {/* 回测配置区域 */}
        <Card title="回测配置" style={{ marginBottom: 16 }} styles={{ body: { paddingBottom: 0 } }}>
          <ProFormDateTimeRangePicker
            name="dateRange"
            label="时间范围"
            width="lg"
            required
            fieldProps={{
              showNow: true,
              disabledDate: disabledFutureDate,
              disabledTime: (date, type) => disabledFutureTime(date),
            }}
            rules={[
              { required: true, message: '请选择时间范围' },
              {
                validator: async (_, value) => {
                  const [startTime, endTime] = value || [];
                  if (!startTime || !endTime) return Promise.resolve();

                  const s = dayjs(startTime);
                  const e = dayjs(endTime);
                  const curNow = dayjs();

                  if (e.isAfter(curNow)) {
                    return Promise.reject(new Error('结束时间不能超过当前时间'));
                  }
                  if (s.isAfter(e)) {
                    return Promise.reject(new Error('开始时间不能晚于结束时间'));
                  }
                  // 区间不能超过 3 年：要求 end <= start + 3 years
                  if (s.add(3, 'year').isBefore(e)) {
                    return Promise.reject(new Error('时间区间不能超过 3 年'));
                  }
                  return Promise.resolve();
                },
              },
            ]}
          />

          <Form.Item
            label="交易所"
            name="exchange"
            rules={[{ required: true, message: '请选择交易所' }]}
          >
            <Select style={{ width: 220 }} placeholder="选择交易所">
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
                  for (const item of value) {
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

          <ProForm.Item label="初始资产配置" required>
            <Card>
              <Row gutter={16}>
                <Col span={6}>资产</Col>
                <Col span={6}>钱包类型</Col>
                <Col span={6}>总量</Col>
                <Col span={2} />
              </Row>
              <Form.List
                name="initialAssets"
                rules={[
                  {
                    validator: (_, value) => {
                      if (!value || value.length === 0) {
                        return Promise.reject(new Error('请添加初始资产'));
                      }
                      const symbols = (form.getFieldValue('symbols') || []) as string[];
                      if (
                        !validateInitialAssetsAgainstSymbols(
                          form.getFieldValue('exchange'),
                          symbols,
                          value,
                        )
                      ) {
                        return Promise.reject(
                          new Error('初始资产和交易对不匹配，缺失相关资产，请检查'),
                        );
                      }
                      const seen = new Set<string>();
                      for (const item of value as BacktestInitialAssetRow[]) {
                        const asset = String(item?.asset || '')
                          .trim()
                          .toUpperCase();
                        const walletType = String(item?.walletType || '').trim();
                        if (!asset || !walletType) {
                          continue;
                        }
                        const key = `${asset}|${walletType}`;
                        if (seen.has(key)) {
                          return Promise.reject(new Error('资产与钱包类型组合需唯一'));
                        }
                        seen.add(key);
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
                        <Col span={6}>
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
                            onClick={() => remove(name)}
                          />
                        </Col>
                      </Row>
                    ))}
                    <Button
                      type="dashed"
                      block
                      icon={<PlusOutlined />}
                      onClick={() => add({ ...EMPTY_INITIAL_ASSET_ROW })}
                    >
                      添加资产
                    </Button>
                    <Form.ErrorList errors={errors} />
                  </>
                )}
              </Form.List>
            </Card>
          </ProForm.Item>

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
                      {strategy?.params && strategy?.params.length > 0 ? (
                        strategy?.params.map((param: StrategyParam) => {
                          return (
                            <div key={param.name} style={{ marginBottom: 16 }}>
                              {renderInput(param)}
                            </div>
                          );
                        })
                      ) : (
                        <Empty style={{ paddingBottom: 16 }} description="该策略没有参数配置" />
                      )}
                    </div>
                  ),
                },
              ]}
            />
          </ProForm.Item>

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
                      {strategy?.signals && strategy?.signals.length > 0 ? (
                        <Form.List name="signals">
                          {(fields, { add, remove }) => (
                            <>
                              {fields.map(({ key, name, ...restField }, index) => {
                                const binding = form.getFieldValue(['signals', name]);
                                const signal = strategy?.signals.find(
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

                                        {/* 所有 signal 类型都显示 scope 信息，但只有 market signal 需要数据源选择 */}
                                        {/* 显示 scope 相关的信息 */}
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
                                                    <Tag>
                                                      {utils.market.getExchangeTitle(
                                                        bindingItem.exchange,
                                                      )}
                                                    </Tag>
                                                    <Tag>{bindingItem.symbol}</Tag>
                                                  </Col>
                                                  <Col span={14}>
                                                    {isMarketSignal && (
                                                      <Form.Item
                                                        {...restField}
                                                        name={[
                                                          name,
                                                          'bindings',
                                                          bindingIndex,
                                                          'datasourceId',
                                                        ]}
                                                        style={{ marginBottom: 0 }}
                                                        noStyle
                                                        initialValue={0}
                                                      >
                                                        <Select
                                                          style={{ width: '100%' }}
                                                          placeholder="选择数据源"
                                                          loading={loadingDatasources}
                                                          size="small"
                                                        >
                                                          <Select.Option key="auto" value={0}>
                                                            自动（后端加载）
                                                          </Select.Option>
                                                          {getFilteredDatasources(
                                                            signal,
                                                            bindingItem.exchange,
                                                            bindingItem.symbol,
                                                          ).map((ds) => (
                                                            <Select.Option
                                                              key={ds.id}
                                                              value={ds.id}
                                                            >
                                                              <Tooltip
                                                                title={buildDataSourceLabel(ds)}
                                                              >
                                                                {buildDataSourceLabel(ds)}
                                                              </Tooltip>
                                                            </Select.Option>
                                                          ))}
                                                        </Select>
                                                      </Form.Item>
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
                                                  {utils.market.getExchangeTitle(
                                                    signal.exchange as Exchange,
                                                  )}
                                                </Tag>
                                                <Tag>{signal.symbol}</Tag>
                                              </Col>
                                              <Col span={14}>
                                                {isMarketSignal && (
                                                  <Form.Item
                                                    {...restField}
                                                    name={[name, 'datasourceId']}
                                                    noStyle
                                                    initialValue={0}
                                                  >
                                                    <Select
                                                      style={{ width: '100%' }}
                                                      placeholder="选择数据源"
                                                      loading={loadingDatasources}
                                                      size="small"
                                                    >
                                                      <Select.Option key="auto" value={0}>
                                                        自动（后端加载）
                                                      </Select.Option>
                                                      {getFilteredDatasources(
                                                        signal,
                                                        signal.exchange,
                                                        signal.symbol,
                                                      ).map((ds) => (
                                                        <Select.Option key={ds.id} value={ds.id}>
                                                          <Tooltip title={buildDataSourceLabel(ds)}>
                                                            {buildDataSourceLabel(ds)}
                                                          </Tooltip>
                                                        </Select.Option>
                                                      ))}
                                                    </Select>
                                                  </Form.Item>
                                                )}
                                              </Col>
                                            </Row>
                                          </div>
                                        )}
                                        {scope === SignalScope.Exchange && (
                                          <div style={{ marginTop: 6 }}>
                                            {binding?.bindings?.map(
                                              (bindingItem: any, bindingIndex: number) => (
                                                <ExchangeBindingItem
                                                  key={bindingIndex}
                                                  bindingIndex={bindingIndex}
                                                  bindingItem={bindingItem}
                                                  name={name}
                                                  restField={restField}
                                                  form={form}
                                                  signal={signal}
                                                  isMarketSignal={isMarketSignal}
                                                  exchangeSymbols={exchangeSymbols}
                                                  loadingSymbols={loadingSymbols}
                                                  datasources={datasources}
                                                  loadingDatasources={loadingDatasources}
                                                  onLoadSymbols={loadSymbolsForExchange}
                                                  getFilteredDatasources={getFilteredDatasources}
                                                  buildDataSourceLabel={buildDataSourceLabel}
                                                />
                                              ),
                                            )}
                                          </div>
                                        )}
                                        {scope === SignalScope.Strategy && (
                                          <div style={{ marginTop: 6 }}>
                                            {isMarketSignal && (
                                              <Form.Item
                                                {...restField}
                                                name={[name, 'datasourceId']}
                                                style={{ marginBottom: 4 }}
                                                noStyle
                                                initialValue={0}
                                              >
                                                <Select
                                                  style={{ width: 300 }}
                                                  placeholder="选择数据源"
                                                  loading={loadingDatasources}
                                                  size="small"
                                                >
                                                  <Select.Option key="auto" value={0}>
                                                    自动（后端加载）
                                                  </Select.Option>
                                                  {getFilteredDatasources(signal).map((ds) => (
                                                    <Select.Option key={ds.id} value={ds.id}>
                                                      <Tooltip title={buildDataSourceLabel(ds)}>
                                                        {buildDataSourceLabel(ds)}
                                                      </Tooltip>
                                                    </Select.Option>
                                                  ))}
                                                </Select>
                                              </Form.Item>
                                            )}
                                          </div>
                                        )}
                                      </div>
                                    </Form.Item>
                                    {index < fields.length - 1 && (
                                      <Divider style={{ margin: '16px 0' }} />
                                    )}
                                  </React.Fragment>
                                );
                              })}
                            </>
                          )}
                        </Form.List>
                      ) : (
                        <Empty style={{ paddingBottom: 16 }} description="该策略没有信号配置" />
                      )}
                    </>
                  ),
                },
              ]}
            />
          </ProForm.Item>
        </Card>

        {/* 回测结果区域 */}
        <Card title="回测结果">
          <Spin spinning={backtestLoading}>
            {backtestResult ? (
              <BacktestResult value={backtestResult} />
            ) : (
              <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>
                请配置回测参数并点击"开始回测"按钮查看结果
              </div>
            )}
          </Spin>
        </Card>
      </div>
    </ProForm>
  );
};

export default BacktestForm;
