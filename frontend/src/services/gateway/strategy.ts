import { Exchange, MarketType } from '@/global.types';
import { request } from '@umijs/max';
import dayjs from 'dayjs';
import { AccountEquity, Balance, LedgersConnection, Order, Position, WalletType } from './account';

export enum StrategyStatus {
  Unspecified = 'unspecified',
  Draft = 'draft',
  Active = 'active',
  Inactive = 'inactive',
}

export enum SignalType {
  Unspecified = 'unspecified',
  Kline = 'kline',
  Trade = 'trade',
  Depth = 'depth',
  Ticker = 'ticker',
  MarkPrice = 'mark_price',
  Social = 'social',
  Timer = 'timer',
  Order = 'order',
  Position = 'position',
  Balance = 'balance',
  Risk = 'risk',
  System = 'system',
}

export const EventKindOptions = [
  { label: 'Kline', value: 'kline' },
  { label: 'Trade', value: 'trade' },
  { label: 'Depth', value: 'depth' },
  { label: 'Ticker', value: 'ticker' },
  { label: 'Mark Price', value: 'mark_price' },

  { label: 'Funding Rate', value: 'funding_rate' },
  { label: 'Funding Settlement', value: 'funding_settlement' },

  { label: 'Order', value: 'order_snapshot' },
  { label: 'Fill', value: 'fill' },
  { label: 'Balance Snapshot', value: 'balance_snapshot' },
  { label: 'Balance Changed', value: 'balance_changed' },
  { label: 'Leverage Changed', value: 'leverage_changed' },
  { label: 'Position Snapshot', value: 'position_snapshot' },

  { label: 'Social', value: 'social' },
  { label: 'Timer', value: 'timer' },
  { label: 'System', value: 'system' },
  { label: 'Risk', value: 'risk' },
  { label: 'Test', value: 'test' },
];

export type DataSource = {
  id: string;
  type: SignalType;
  name: string;
  description: string;
  exchange?: Exchange;
  symbol?: string;
  props?: string;
  startTs: number;
  endTs: number;
  createdAt: number;
  updatedAt: number;
};

export type QueryDatasourcesParams = API.PageParams & {
  id?: string;
  name?: string;
  type?: SignalType;
  exchange?: Exchange;
  symbol?: string;
};

export const IsMarketSignal = (signalType: SignalType) => {
  switch (signalType) {
    case SignalType.Kline:
      return true;
    case SignalType.Trade:
      return true;
    case SignalType.Depth:
      return true;
    case SignalType.Ticker:
      return true;
    default:
      return false;
  }
};

export const SignalTypeOptions = [
  {
    label: 'Market',
    title: 'market',
    options: [
      { label: 'Kline', value: SignalType.Kline },
      { label: 'Trade', value: SignalType.Trade },
      { label: 'Depth', value: SignalType.Depth },
      { label: 'Ticker', value: SignalType.Ticker },
      { label: 'Mark Price', value: SignalType.MarkPrice, disabled: true, description: '暂不支持' },
    ],
  },
  {
    label: 'Account',
    title: 'account',
    options: [
      { label: 'Order', value: SignalType.Order, disabled: true },
      { label: 'Position', value: SignalType.Position, disabled: true },
      { label: 'Balance', value: SignalType.Balance, disabled: true },
    ],
  },
  { label: 'Social', value: SignalType.Social },
  { label: 'Timer', value: SignalType.Timer },
  { label: 'Risk', value: SignalType.Risk },
  { label: 'System', value: SignalType.System },
];

export enum SignalScope {
  Unspecified = 'unspecified',
  Strategy = 'strategy', // Strategy 级别：整个策略只需要一个数据源
  Exchange = 'exchange', // Exchange 级别：每个交易所只需要一个数据源
  Symbol = 'symbol', // Symbol 级别：每个 symbol 需要独立数据源（动态，根据回测时的 symbols）
  Target = 'target', // Target 级别：针对策略中指定的具体 symbol（固定）
}

export const SignalScopeOptions = [
  { label: 'Strategy 级别', value: SignalScope.Strategy, description: '整个策略只需要一个数据源' },
  {
    label: 'Exchange 级别',
    value: SignalScope.Exchange,
    disabled: true,
    description: '每个交易所只需要一个数据源',
  },
  {
    label: 'Symbol 级别',
    value: SignalScope.Symbol,
    description: '每个交易对都需要独立的数据源（动态）',
  },
  {
    label: 'Target 级别',
    value: SignalScope.Target,
    description: '针对策略中指定的具体 symbol（固定）',
  },
];

export enum ParamType {
  String = 'string',
  Number = 'number',
  Bool = 'bool',
  Object = 'object',
}

export enum KlineInterval {
  OneSecond = '1s',
  OneMinute = '1m',
  FiveMinutes = '5m',
  FifteenMinutes = '15m',
  ThirtyMinutes = '30m',
  OneHour = '1h',
  FourHours = '4h',
  TwelveHours = '12h',
  OneDay = '1d',
  ThreeDays = '3d',
  OneWeek = '1w',
  OneMonth = '1M',
}

// Kline interval 枚举选项
export const KlineIntervalOptions = [
  { label: '1秒', value: '1s' },
  { label: '1分钟', value: '1m' },
  { label: '5分钟', value: '5m' },
  { label: '15分钟', value: '15m' },
  { label: '30分钟', value: '30m' },
  { label: '1小时', value: '1h' },
  { label: '4小时', value: '4h' },
  { label: '12小时', value: '12h' },
  { label: '1天', value: '1d' },
  { label: '3天', value: '3d' },
  { label: '1周', value: '1w' },
  { label: '1月', value: '1M' },
];

export type StrategyParam = {
  name: string;
  description?: string;
  type: ParamType;
  required: boolean;
  default?: string;
};

export type SignalDefinition = {
  id: string;
  type: SignalType;
  exchange?: Exchange;
  symbol?: string;
  props?: string;
  scope?: SignalScope; // 新增：信号作用域
};

export type Strategy = {
  id: string;
  name: string;
  description: string;
  code: string;
  version: string;
  params: StrategyParam[];
  status: StrategyStatus;
  signals: SignalDefinition[];
  createdAt: number;
  updatedAt: number;
};

export type GenerateStrategyResponse = {
  sessionId: string;
  content: string;
};

export type QueryStrategiesParams = API.PageParams & {
  id?: string;
  name?: string;
  status?: StrategyStatus;
  createdAtStart?: number;
  createdAtEnd?: number;
};

export type BacktestSymbol = {
  exchange: Exchange;
  symbol: string;
  baseAssetQty?: string;
  quoteAssetQty?: string;
};

export type BacktestAsset = {
  asset: string;
  amount: string;
};

export type BacktestExchange = {
  exchange: Exchange;
  symbols: BacktestSymbol[];
  assets?: BacktestAsset[];
};

export type BacktestSignal = {
  signalId: string;
  datasourceId: string;
  exchange?: string;
  symbol?: string;
};

export type RunBacktestInput = {
  strategy?: Strategy;
  strategyId?: string;
  version?: string;
  runType: number;
  startTime: number;
  endTime: number;
  symbols: BacktestSymbol[];
  params?: string;
  signals?: BacktestSignal[];
};

export type EquityPoint = {
  ts: number;
  netValue: string;
};

export type SymbolSummary = {
  exchange: Exchange;
  symbol: string;
  base: string;
  quote: string;
  initialBase: string;
  initialQuote: string;
  finalBase: string;
  finalQuote: string;
  positionQty: string;
  avgPrice: string;
  lastPrice: string;
  initialNet: string;
  finalNet: string;
  realizedPnl: string;
  unrealizedPnl: string;
  netPnl: string;
  longRealizedPnl: string;
  shortRealizedPnl: string;
  longUnrealizedPnl: string;
  shortUnrealizedPnl: string;
  longNetPnl: string;
  shortNetPnl: string;
  longTrades: number;
  shortTrades: number;
};

export type ExSymbol = {
  exchange: string;
  symbol: string;
};

export type Fill = {
  exchange: Exchange;
  symbol: string;
  orderId: string;
  clientOrderId: string;
  side: string;
  isBuy: boolean;
  qty: string;
  price: string;
  fee: string;
  asset: string;
  feeInBase?: string;
  numeraire?: string;
  realizedPnl: string;
  isMaker?: boolean;
  ts: number;
};

export type BacktestResult = {
  symbols: SymbolSummary[];
  equity: EquityPoint[];
  orders?: Order[];
  fills?: Fill[];
  metaJson?: string;
};

export type ConsoleLog = {
  ts: number;
  level: string;
  message: string;
};

export type RunBacktestResponse = {
  id: string;
  strategy: Strategy;
  startTime: number;
  endTime: number;
  initialBalance: string;
  finalBalance: string;
  totalPnl: string;
  totalTrades: number;
  winTrades: number;
  lossTrades: number;
  winRate: number;
  sharpeRatio: number;
  maxDrawdown: number;
  data: BacktestResult;
  createdAt: number;
  timeCost: number;
  consoleLogs: ConsoleLog[];
};

const QUERY_STRATEGIES = `
  query QueryStrategies($input: QueryStrategiesInput!) {
    Result: Strategies(input: $input) {
      totalCount
      list {
        id
        name
        description
        code
        version
        params {
          name
          description
          type
          required
          default
        }
        status
        signals {
          id
          type
          scope
          exchange
          symbol
          props
        }
        createdAt
        updatedAt
      }
    }
  }
`;

const QUERY_STRATEGY = `
  query QueryStrategy($id: String!) {
    Result: Strategy(id: $id) {
      id
      name
      description
      code
      version
      params {
        name
        description
        type
        required
        default
      }
      status
      signals {
        id
        type
        scope
        exchange
        symbol
        props
      }
      createdAt
      updatedAt
    }
  }
`;

const CREATE_STRATEGY = `
  mutation CreateStrategy($input: CreateStrategyInput!) {
    Result: CreateStrategy(input: $input) {
      id
      name
      description
      code
      version
      params {
        name
        description
        type
        required
        default
      }
      status
      signals {
        id
        type
        scope
        exchange
        symbol
        props
      }
      createdAt
      updatedAt
    }
  }
`;

const UPDATE_STRATEGY = `
  mutation UpdateStrategy($input: UpdateStrategyInput!) {
    Result: UpdateStrategy(input: $input) {
      id
      name
      description
      code
      version
      params {
        name
        description
        type
        required
        default
      }
      status
      signals {
        id
        type
        scope
        exchange
        symbol
        props
      }
      createdAt
      updatedAt
    }
  }
`;

const GENERATE_STRATEGY = `
  mutation GenerateStrategy($input: GenerateStrategyInput!) {
    Result: GenerateStrategy(input: $input) {
      sessionId
      content
    }
  }
`;

const DELETE_STRATEGY = `
  mutation DeleteStrategy($id: String!) {
    Result: DeleteStrategy(id: $id)
  }
`;

const ACTIVE_STRATEGY = `
  mutation ActiveStrategy($id: String!) {
    Result: ActiveStrategy(id: $id)
  }
`;

const INACTIVE_STRATEGY = `
  mutation InactiveStrategy($id: String!) {
    Result: InactiveStrategy(id: $id)
  }
`;

export async function queryStrategies(params: QueryStrategiesParams) {
  const pageSize = params.pageSize || 10;
  const input: any = {
    limit: pageSize,
    offset: ((params?.current || 1) - 1) * pageSize,
  };

  if (params.id) input.id = params.id;
  if (params.name) input.name = params.name;
  if (params.status) input.status = params.status;
  if (params.createdAtStart) input.createdAtStart = params.createdAtStart;
  if (params.createdAtEnd) input.createdAtEnd = params.createdAtEnd;

  let response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_STRATEGIES,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result;
}

export async function queryStrategy(id: string) {
  let response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_STRATEGY,
      variables: {
        id,
      },
    }),
  });
  return response.data?.Result;
}

export async function createStrategy(params: {
  name: string;
  description: string;
  code: string;
  params: StrategyParam[];
  signals: SignalDefinition[];
}) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: CREATE_STRATEGY,
      variables: {
        input: params,
      },
    }),
  });
}

export async function updateStrategy(params: {
  id: string;
  version: string;
  name?: string;
  description?: string;
  code?: string;
  params?: StrategyParam[];
  signals?: SignalDefinition[];
}) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: UPDATE_STRATEGY,
      variables: {
        input: params,
      },
    }),
  });
}

export async function generateStrategy(params: { query: string }) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GENERATE_STRATEGY,
      variables: {
        input: {
          query: params.query,
        },
      },
    }),
  });
  return response.data?.Result as GenerateStrategyResponse | undefined;
}

export async function deleteStrategy(id: string) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: DELETE_STRATEGY,
      variables: {
        id,
      },
    }),
  });
}

export async function activeStrategy(id: string) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: ACTIVE_STRATEGY,
      variables: {
        id,
      },
    }),
  });
}

export async function inactiveStrategy(id: string) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: INACTIVE_STRATEGY,
      variables: {
        id,
      },
    }),
  });
}

const RUN_BACKTEST = `
  mutation RunBacktest($input: RunBacktestInput!) {
    Result: RunBacktest(input: $input) {
      id
      strategy {
        id
        name
        description
        code
        version
        params {
          name
          description
          type
          required
          default
        }
        status
        signals {
          id
          type
          exchange
          symbol
          props
        }
        createdAt
        updatedAt
      }
      startTime
      endTime
      initialBalance
      finalBalance
      totalPnl
      totalTrades
      winTrades
      lossTrades
      winRate
      sharpeRatio
      maxDrawdown
      data {
        symbols {
          exchange
          symbol
          base
          quote
          initialBase
          initialQuote
          finalBase
          finalQuote
          positionQty
          avgPrice
          lastPrice
          initialNet
          finalNet
          realizedPnl
          unrealizedPnl
          netPnl
          longRealizedPnl
          shortRealizedPnl
          longUnrealizedPnl
          shortUnrealizedPnl
          longNetPnl
          shortNetPnl
          longTrades
          shortTrades
        }
        equity {
          ts
          netValue: notional
        }
        orders {
          exchange
          symbol
          clientOrderId
          orderId
          side
          isBuy
          orderType
          algoType
          price
          originalQty
          executedQty
          originalQuoteQty
          executedQuoteQty
          avgPrice
          status
          timeInForce
          reduceOnly
          closePosition
          postOnly
          priceProtect
          isWorking
          workingTs
          createdTs
          updatedTs
          finishedTs
        }
        fills {
          exchange
          symbol
          orderId
          clientOrderId
          side
          isBuy
          qty
          price
          fee
          asset: feeAsset
          realizedPnl
          isMaker
          ts
        }
        metaJson
      }
      createdAt
      timeCost
      consoleLogs {
        ts
        level
        message
      }
    }
  }
`;

export async function runBacktest(
  input: {
    strategy?: Partial<Strategy>;
    strategyId?: string;
    version?: string;
    runType: number;
    startTime: number;
    endTime: number;
    symbols: Array<{
      exchange: string;
      symbol: string;
      baseAssetQty?: string;
      quoteAssetQty?: string;
    }>;
    params?: string;
    signals?: Array<{
      signalId: string;
      datasourceId: string;
    }>;
  },
  signal?: AbortSignal,
) {
  if (input.runType === 0 && input.strategy) {
    input.strategy = {
      name: input.strategy.name,
      code: input.strategy.code,
      params: input.strategy.params,
      signals: input.strategy.signals,
    };
  } else {
    input.strategyId = input.strategyId || input.strategy?.id;
    input.version = input.version || input.strategy?.version;
    delete input.strategy;
  }
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: RUN_BACKTEST,
      variables: {
        input,
      },
    }),
    signal,
  });
}

const QUERY_DATASOURCES = `
  query QueryDatasources($input: QueryDatasourcesInput!) {
    Result: Datasources(input: $input) {
      totalCount
      list {
        id
        type
        name
        description
        exchange
        symbol
        props
        startTs
        endTs
        createdAt
        updatedAt
      }
    }
  }
`;

const QUERY_DATASOURCE = `
  query QueryDatasource($id: String!) {
    Result: Datasource(id: $id) {
      id
      type
      name
      description
      exchange
      symbol
      props
      startTs
      endTs
      createdAt
      updatedAt
    }
  }
`;

const CREATE_DATASOURCE = `
  mutation CreateDatasource($input: CreateDatasourceInput!) {
    Result: CreateDatasource(input: $input) {
      id
      type
      name
      description
      exchange
      symbol
      props
      startTs
      endTs
      createdAt
      updatedAt
    }
  }
`;

const DELETE_DATASOURCE = `
  mutation DeleteDatasource($id: String!) {
    Result: DeleteDatasource(id: $id)
  }
`;

export async function queryDatasources(params: QueryDatasourcesParams) {
  const pageSize = params.pageSize || 100;
  const input: any = {
    limit: pageSize,
    offset: ((params?.current || 1) - 1) * pageSize,
    type: params.type,
    exchange: params.exchange,
    symbol: params.symbol,
  };
  Object.keys(input).forEach((key) => {
    if (input[key] === undefined || input[key] === null || input[key] === '') {
      delete input[key];
    }
  });
  let response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_DATASOURCES,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result;
}

export async function queryDatasource(id: string) {
  let response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_DATASOURCE,
      variables: {
        id,
      },
    }),
  });
  return response.data?.Result;
}

export async function createDatasource(params: {
  name: string;
  description: string;
  type: SignalType;
  exchange?: string;
  symbol?: string;
  props?: string;
  startTs: number;
  endTs: number;
}) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: CREATE_DATASOURCE,
      variables: {
        input: params,
      },
    }),
  });
}

export async function deleteDatasource(id: string) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: DELETE_DATASOURCE,
      variables: {
        id,
      },
    }),
  });
}

export enum BotMode {
  Live = 'live',
  Paper = 'paper',
}

export enum BotStatus {
  Stopped = 'stopped',
  Running = 'running',
  Error = 'error',
}

export const BotStatusOptions = [
  { label: '已停止', value: BotStatus.Stopped },
  { label: '运行中', value: BotStatus.Running },
  { label: '错误', value: BotStatus.Error },
];

export type Bot = {
  id: number;
  strategyId: string;
  strategyVer: string;
  strategyName: string;
  upgradable?: boolean;
  name: string;
  description: string;
  mode: BotMode;
  exchange: Exchange;
  symbols: string[];
  accountId: string;
  config: string; // JSON 字符串
  status: BotStatus;
  errorMessage?: string;
  createdAt: number;
  startedAt?: number;
  stoppedAt?: number;
};

export type BotLog = {
  id: string;
  botId: number;
  level: string;
  message: string;
  ts: number;
  createdAt: number;
};

export type BotLogsConnection = {
  list: BotLog[];
  nextCursor?: string;
};

export type BotPortfolioAsset = {
  exchange: Exchange;
  walletType: WalletType;
  asset: string;
  free: string;
  frozen: string;
  updatedTs: number;
};

export type BotPortfolioPosition = {
  exchange: Exchange;
  symbol: string;
  marketType: MarketType;
  side: string;
  qty: string;
  leverage: number;
  avgPrice: string;
  updatedTs: number;
};

export type BotPortfolio = {
  assets: BotPortfolioAsset[];
  positions: BotPortfolioPosition[];
  ts: number;
};

export type BotState = {
  botStatus: BotStatus;
  executorStatus: string;
  runErr?: string;
  jsRunnerStatus: string;
  portfolio?: BotPortfolio;
  signalAvgDurationMs?: number;
  signalAvgLatencyMs?: number;
  lastSignalTs?: number;
};

export type QueryBotStateParams = {
  botId: number;
};

export type QueryBotsParams = API.PageParams & {
  id?: number;
  strategyId?: string;
  mode?: BotMode;
  status?: BotStatus;
  accountId?: string;
  exchange?: string;
  name?: string;
  createdAtRange?: string[];
};

export type CreateBotInput = {
  strategyId: string;
  strategyVer: string;
  name: string;
  description: string;
  mode: BotMode;
  exchange: string;
  symbols: string[];
  accountId: string;
  config: string;
};

export type UpdateBotInput = {
  id: number;
  name: string;
  description: string;
  symbols: string[];
  config: string;
};

export const BotModeOptions = [
  { label: '模拟盘', value: BotMode.Paper, disabled: true },
  { label: '实盘', value: BotMode.Live },
];

const QUERY_BOTS = `
  query QueryBots($input: QueryBotsInput!) {
    Result: Bots(input: $input) {
      totalCount
      list {
        id
        strategyId
        strategyVer
        strategyName
        upgradable
        name
        description
        mode
        exchange
        symbols
        accountId
        config
        status
        errorMessage
        createdAt
        startedAt
        stoppedAt
      }
    }
  }
`;

const QUERY_BOT = `
  query QueryBot($id: Int!) {
    Result: Bot(id: $id) {
      id
      strategyId
      strategyVer
      strategyName
      name
      description
      mode
      exchange
      symbols
      accountId
      config
      status
      errorMessage
      createdAt
      startedAt
      stoppedAt
    }
  }
`;

const CREATE_BOT = `
  mutation CreateBot($input: CreateBotInput!) {
    Result: CreateBot(input: $input) {
      id
      strategyId
      strategyVer
      strategyName
      name
      description
      mode
      exchange
      symbols
      accountId
      config
      status
      errorMessage
      createdAt
      startedAt
      stoppedAt
    }
  }
`;

const UPDATE_BOT = `
  mutation UpdateBot($input: UpdateBotInput!) {
    Result: UpdateBot(input: $input) {
      id
      strategyId
      strategyVer
      strategyName
      name
      description
      mode
      exchange
      symbols
      accountId
      config
      status
      errorMessage
      createdAt
      startedAt
      stoppedAt
    }
  }
`;

const START_BOT = `
  mutation StartBot($id: Int!) {
    Result: StartBot(id: $id)
  }
`;

const STOP_BOT = `
  mutation StopBot($id: Int!) {
    Result: StopBot(id: $id)
  }
`;

const UPGRADE_BOT = `
  mutation UpgradeBot($id: Int!) {
    Result: UpgradeBot(id: $id) {
      success
      message
      bot {
        id
        strategyId
        strategyVer
        status
        errorMessage
      }
    }
  }
`;

const DELETE_BOT = `
  mutation DeleteBot($id: Int!) {
    Result: DeleteBot(id: $id)
  }
`;

const QUERY_BOT_BALANCE = `
  query QueryBotBalance($input: QueryBotBalanceInput!) {
    Result: BotBalance(input: $input) {
      notional
      unRealizedProfit
      notional24HChange
      assets {
        code
        balance
        locked
        notional
        avgPrice
        walletType
        updatedTs
        unRealizedProfit
      }
    }
  }
`;

const QUERY_BOT_POSITIONS = `
  query QueryBotPositions($input: QueryBotPositionsInput!) {
    Result: BotPositions(input: $input) {
      symbol
      side
      isolated
      amount
      entryPrice
      markPrice
      liquidationPrice
      notional
      leverage
      initialMargin
      maintMargin
      unRealizedProfit
      updatedTs
    }
  }
`;

const QUERY_BOT_ORDERS = `
  query QueryBotOrders($input: QueryBotOrdersInput!) {
    Result: BotOrders(input: $input) {
      totalCount
      list {
        accountId
        botId
        exchange
        symbol
        clientOrderId
        orderId
        drivedOrderId
        side
        isBuy
        orderType
        algoType
        source
        status
        price
        originalQty
        executedQty
        avgPrice
        locked
        lockedAsset
        fee
        feeAsset
        realizedPnl
        pnlAsset
        conditions {
          triggerType
          callbackRate
          callbackDistance
          priceWorkingType
          activationPrice
          orderPrice
          isTrailing
          activated
          activatedTs
        }
        rejectReason
        createdTs
        finishedTs
      }
    }
  }
`;

const QUERY_BOT_LEDGER = `
  query QueryBotLedger($input: QueryBotLedgersInput!) {
    Result: BotLedger(input: $input) {
      totalCount
      list {
        id
        accountId
        exchange
        asset
        walletType
        total
        frozen
        totalDelta
        frozenDelta
        type
        detail
        isEffective
        ts
        createdAt
      }
    }
  }
`;

const QUERY_BOT_EQUITY = `
  query QueryBotEquity($botId: Int!, $startTs: Int!, $endTs: Int!) {
    Result: BotEquity(botId: $botId, startTs: $startTs, endTs: $endTs) {
      totalCount
      list {
        id
        accountId
        ts
        notional
        unRealizedProfit
        createdAt
      }
    }
  }
`;

const QUERY_BOT_METRICS = `
  query QueryBotMetrics($input: QueryBotMetricsInput!) {
    Result: BotMetrics(input: $input) {
      accountId
      botId
      dimension
      symbolsFilter
      cagr
      sharpe
      sortino
      maxDrawdown
      timeUnderWaterSeconds
      calmar
      winRate
      profitFactor
      rollingSharpe
      avgSlippageBps
      feeRatio
      maxConsecutiveLoss
      startTs
      endTs
      symbols {
        symbol
        exchange
        cagr
        sharpe
        sortino
        maxDrawdown
        timeUnderWaterSeconds
        calmar
        winRate
        profitFactor
        rollingSharpe
        avgSlippageBps
        feeRatio
        maxConsecutiveLoss
      }
    }
  }
`;

const QUERY_BOT_LOGS = `
  query QueryBotLogs($input: QueryBotLogsInput!) {
    Result: BotLogs(input: $input) {
      list {
        id
        botId
        level
        message
        ts
        createdAt
      }
      nextCursor
    }
  }
`;

export async function queryBots(params: QueryBotsParams) {
  console.log(params);
  const pageSize = params.pageSize || 10;
  const input: any = {
    limit: pageSize,
    offset: ((params?.current || 1) - 1) * pageSize,
  };

  if (params.id) input.id = params.id;
  if (params.strategyId) input.strategyId = params.strategyId;
  if (params.mode) input.mode = params.mode;
  if (params.status) input.status = params.status;
  if (params.accountId) input.accountId = params.accountId;
  if (params.exchange) input.exchange = params.exchange;
  if (params.name) input.name = params.name;
  if (params.createdAtRange) {
    input.createdAtStart = params.createdAtRange[0]
      ? dayjs(params.createdAtRange[0]).unix()
      : undefined;
    input.createdAtEnd = params.createdAtRange[1]
      ? dayjs(params.createdAtRange[1]).unix()
      : undefined;
  }
  console.log(input);

  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOTS,
      variables: { input },
    }),
  });
  return response.data?.Result;
}

export async function queryBot(id: number) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT,
      variables: { id },
    }),
  });
  return response.data?.Result;
}

export async function createBot(input: CreateBotInput) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: CREATE_BOT,
      variables: { input },
    }),
  });
}

export async function updateBot(input: UpdateBotInput) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: UPDATE_BOT,
      variables: { input },
    }),
  });
}

export async function startBot(id: number) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: START_BOT,
      variables: { id },
    }),
  });
}

export async function stopBot(id: number) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: STOP_BOT,
      variables: { id },
    }),
  });
}

export async function upgradeBot(id: number) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: UPGRADE_BOT,
      variables: { id },
    }),
  });
}

export async function deleteBot(id: number) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: DELETE_BOT,
      variables: { id },
    }),
  });
}

export async function queryBotBalance(
  botId: number,
  walletType?: string,
  asset?: string,
  withNotional: boolean = true,
) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_BALANCE,
      variables: {
        input: { botId, walletType, asset, withNotional },
      },
    }),
  });
  return response.data?.Result as Balance | undefined;
}

export async function queryBotPositions(botId: number, marketType?: string, symbol?: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_POSITIONS,
      variables: {
        input: { botId, marketType, symbol },
      },
    }),
  });
  return response.data?.Result as Position[];
}

const QUERY_BOT_STATE = `
  query QueryBotState($input: QueryBotStateInput!) {
    Result: BotState(input: $input) {
      botStatus
      executorStatus
      runErr
      jsRunnerStatus
      portfolio {
        ts
        assets {
          exchange
          walletType
          asset
          free
          frozen
          updatedTs
        }
        positions {
          exchange
          symbol
          marketType
          side
          qty
          leverage
          avgPrice
          updatedTs
        }
      }
      signalAvgDurationMs
      signalAvgLatencyMs
      lastSignalTs
    }
  }
`;

export async function queryBotState(botId: number): Promise<BotState | undefined> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_STATE,
      variables: {
        input: { botId },
      },
    }),
  });
  return response.data?.Result as BotState | undefined;
}

export async function queryBotOrders(
  botId: number,
  page: number = 1,
  size: number = 10,
  input?: {
    symbol?: string;
    orderType?: string;
    includeFinished?: boolean;
  },
) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_ORDERS,
      variables: {
        input: {
          botId,
          symbol: input?.symbol,
          orderType: input?.orderType,
          includeFinished: input?.includeFinished ?? true,
          page,
          size,
        },
      },
    }),
  });
  return response.data?.Result as { totalCount: number; list: Order[] } | undefined;
}

export async function queryBotLedger(
  botId: number,
  startTs: number,
  endTs: number,
  page: number = 1,
  size: number = 10,
  input?: {
    walletType?: string;
    asset?: string;
  },
) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_LEDGER,
      variables: {
        input: {
          botId,
          walletType: input?.walletType,
          asset: input?.asset,
          startTs,
          endTs,
          page,
          size,
        },
      },
    }),
  });
  return response.data?.Result as LedgersConnection | undefined;
}

export async function queryBotEquity(botId: number, startTs: number, endTs: number) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_EQUITY,
      variables: { botId, startTs, endTs },
    }),
  });
  return response.data?.Result as { totalCount: number; list: AccountEquity[] } | undefined;
}

export type BotMetricsSymbolMetrics = {
  symbol: string;
  exchange: string;
  cagr: number;
  sharpe: number;
  sortino: number;
  maxDrawdown: number;
  timeUnderWaterSeconds: number;
  calmar: number;
  winRate: number;
  profitFactor: number;
  rollingSharpe: number;
  avgSlippageBps: number;
  feeRatio: number;
  maxConsecutiveLoss: number;
};

export type BotMetricsResponse = {
  accountId: string;
  botId: number;
  dimension: string;
  symbolsFilter: string;
  cagr: number;
  sharpe: number;
  sortino: number;
  maxDrawdown: number;
  timeUnderWaterSeconds: number;
  calmar: number;
  winRate: number;
  profitFactor: number;
  rollingSharpe: number;
  avgSlippageBps: number;
  feeRatio: number;
  maxConsecutiveLoss: number;
  startTs: number;
  endTs: number;
  symbols: BotMetricsSymbolMetrics[];
};

export async function queryBotMetrics(
  botId: number,
  dimension: 'account' | 'symbol' = 'account',
  options?: { symbol?: string; startTs?: number; endTs?: number },
) {
  const input: Record<string, unknown> = {
    botId,
    dimension,
  };
  if (options?.symbol) input.symbol = options.symbol;
  if (options?.startTs != null) input.startTs = options.startTs;
  if (options?.endTs != null) input.endTs = options.endTs;
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_METRICS,
      variables: { input },
    }),
  });
  return response.data?.Result as BotMetricsResponse | null;
}

export async function queryBotLogs(input: {
  botId: number;
  limit?: number;
  cursor?: string;
  startTs?: number;
  endTs?: number;
  level?: string;
}): Promise<BotLogsConnection | undefined> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_LOGS,
      variables: { input },
    }),
  });
  return response.data?.Result;
}

const QUERY_BOT_SIGNAL_FLOW = `
  query QueryBotSignalFlow($input: QueryBotSignalFlowInput!) {
    Result: BotSignalFlow(input: $input) {
      events {
        id
        botId
        accountId
        exchange
        stream
        topic
        eventKind
        tsMs
        inboundAtMs
        outboundAtMs
        receiveAtMs
        ingestAtMs
        payloadJson
      }
      nextId
    }
  }
`;

export type BotSignalRecord = {
  id: string;
  botId: number;
  accountId: string;
  exchange: string;
  stream: string;
  topic: string;
  eventKind: string;
  tsMs: number;
  inboundAtMs: number;
  outboundAtMs: number;
  receiveAtMs: number;
  ingestAtMs: number;
  payloadJson: string;
};

export type BotSignalFlowConnection = {
  events: BotSignalRecord[];
  nextId: string;
};

export async function queryBotSignalFlow(input: {
  botId: number;
  signalType?: string;
  startTsMs?: number;
  startId?: number;
  limit?: number;
}): Promise<BotSignalFlowConnection | undefined> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_BOT_SIGNAL_FLOW,
      variables: {
        input: {
          botId: input.botId,
          signalType: input.signalType,
          startTsMs: input.startTsMs,
          startId: input.startId,
          limit: input.limit ?? 200,
        },
      },
    }),
  });
  return response.data?.Result;
}

export type BotHealthLevel = 'excellent' | 'good' | 'fair' | 'critical';

export function calculateBotHealth(botState: BotState): {
  score: number;
  level: BotHealthLevel;
} {
  const { executorStatus, jsRunnerStatus, signalAvgDurationMs, lastSignalTs } = botState;

  // 如果 executor 或 jsRunner 状态异常，健康度为 0
  if (executorStatus !== 'running' || jsRunnerStatus !== 'running') {
    return { score: 0, level: 'critical' };
  }

  let score = 100;
  const now = Date.now();

  // signal_avg_duration_ms 扣分
  if (signalAvgDurationMs !== undefined && signalAvgDurationMs !== null) {
    if (signalAvgDurationMs > 500) {
      score -= 60;
    } else if (signalAvgDurationMs >= 100) {
      score -= 30;
    }
  }

  // last_signal_ts 扣分
  if (lastSignalTs !== undefined && lastSignalTs !== null) {
    const minutesAgo = (now - lastSignalTs) / (1000 * 60);
    if (minutesAgo > 30) {
      score -= 80;
    } else if (minutesAgo > 5) {
      score -= 40;
    }
  }

  score = Math.max(0, score);

  let level: BotHealthLevel;
  if (score >= 90) {
    level = 'excellent';
  } else if (score >= 70) {
    level = 'good';
  } else if (score >= 50) {
    level = 'fair';
  } else {
    level = 'critical';
  }

  return { score, level };
}
