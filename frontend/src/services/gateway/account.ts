import { Exchange } from '@/global.types';
import { request } from '@umijs/max';
import dayjs from 'dayjs';

export enum AccountStatus {
  Unspecified = 'UNSPECIFIED',
  Online = 'online',
  Offline = 'offline',
}

export enum AuthAlgorithm {
  None = 'none',
  Hmac = 'hmac',
  Ed25519 = 'ed25519',
  Rsa = 'rsa',
}

export enum AccountType {
  Unspecified = 'unspecified',
  Real = 'real',
  Virtual = 'virtual',
  VirtualSub = 'virtual_sub',
}

export type AccountStats = {
  notional: string;
  unRealizedProfit: string;
  notional24HChange: string;
};

export type AccountConfig = {
  maxOrderSize?: string | null;
  maxPositionPerSymbol?: AmountLimit | null;
  maxDailyLoss?: AmountLimit | null;
  maxLeverage?: string | null;
  maxOrdersPerMinute?: number | null;
  minMaintenanceMarginRatio?: string | null;
  maxTotalNetExposure?: AmountLimit | null;
  maxTotalGrossExposure?: AmountLimit | null;
  riskIndexThreshold?: string | null;
  riskIndexAction?: string | null;
  cooldownSeconds?: number | null;
};

export type Account = {
  id: string;
  name: string;
  exchange: Exchange;
  apiKey: string;
  apiSecret: string;
  passphrase: string;
  tags?: string[];
  status: string;
  algorithm: AuthAlgorithm;
  accountType: AccountType;
  parentAccountId?: string | null;
  multiBotMode?: boolean;
  config?: AccountConfig | null;
  createdAt: number;
  updatedAt: number;
  stats?: AccountStats;
  riskIndex?: string | null;
};

export type AccountUnallocatedAsset = {
  asset: string;
  walletType: WalletType;
  parentTotal: string;
  subsAllocated: string;
  unallocated: string;
};

export type MultiBotSubAccount = {
  accountId: string;
  name: string;
  createdAt: number;
};

export type MultiBotAssetSubAllocation = {
  accountId: string;
  amount: string;
};

export type MultiBotAssetAllocation = {
  asset: string;
  walletType: WalletType;
  parentTotal: string;
  subAllocations: MultiBotAssetSubAllocation[];
  unallocated: string;
};

export type MultiBotPositionSubAllocation = {
  accountId: string;
  amount: string;
};

export type MultiBotPositionAllocation = {
  symbol: string;
  side: PositionSide;
  parentTotal: string;
  subAllocations: MultiBotPositionSubAllocation[];
  unallocated: string;
};

export type AccountMultiBotDetails = {
  subAccounts: MultiBotSubAccount[];
  assetAllocations: MultiBotAssetAllocation[];
  positionAllocations: MultiBotPositionAllocation[];
};

export type AmountLimit = {
  amount?: string | null;
  ratio?: string | null;
};

export type Asset = {
  code: string;
  balance: string;
  locked: string;
  notional: string;
  unRealizedProfit?: string;
  walletType: WalletType;
  updatedTs: number;
};

export type Balance = {
  notional: string;
  unRealizedProfit?: string;
  notional24HChange?: string;
  assets: Asset[];
};

export type AccountEquity = {
  id: number;
  accountId: string;
  ts: number;
  notional: string;
  unRealizedProfit: string;
  createdAt: number;
};

export type QueryAccountsParams = API.PageParams & {
  id?: string;
  name?: string;
  exchange?: Exchange;
  /** 筛选账户类型；不传或 unspecified 表示不按类型过滤 */
  accountType?: AccountType | string;
  createdAtRange?: string[];
  tags?: string[];
  status?: string;
};

export enum WalletType {
  Unspecified = 'unspecified',
  Fund = 'fund',
  Trade = 'trade',
  Spot = 'spot',
  Future = 'future',
  Margin = 'margin',
}

export enum MarketSource {
  Unspecified = 'unspecified',
  Db = 'db',
  Remote = 'remote',
}

export enum OrderType {
  Market = 'market',
  Limit = 'limit',
}

export enum OrderSource {
  User = 'user',
  Strategy = 'strategy',
  Liquidation = 'liquidation',
  Adl = 'adl',
}

export enum OrderStatus {
  New = 'new',
  Pending = 'pending',
  Working = 'working',
  PartialDone = 'partial_done',
  Done = 'done',
  Canceled = 'canceled',
  Rejected = 'rejected',
  Expired = 'expired',
}

export enum PositionSide {
  Long = 'long',
  Short = 'short',
}

export type Position = {
  symbol: string;
  side: PositionSide;
  isolated: boolean;
  amount: string;
  entryPrice: string;
  markPrice: string;
  liquidationPrice: string;
  notional: string;
  leverage: number;
  initialMargin: string;
  maintMargin: string;
  unRealizedProfit: string;
  updatedTs: number;
};

export type OrdersConnection = {
  list: Order[];
  totalCount: number;
};

export type LedgersConnection = {
  list: Ledger[];
  totalCount: number;
};

export type Ledger = {
  id: number;
  accountId: string;
  exchange: string;
  asset: string;
  walletType: WalletType;
  total: string;
  frozen: string;
  totalDelta: string;
  frozenDelta: string;
  type: string;
  detail?: string;
  isEffective: boolean;
  ts: number;
  createdAt: number;
};

export type AccountInfo = {
  exchange: string;
  uid: string;
  isSpotEnabled: boolean;
  isFutureEnabled: boolean;
};

export type OrderCondition = {
  triggerType: string;
  orderPrice: string;
  callbackDistance: string;
  callbackRate: string;
  activationPrice: string;
  priceWorkingType: string;
  priceMode: string;
  isTrailing: boolean;
  activated: boolean;
  activatedTs: number;
};

export type OrderAllocation = {
  accountId: string;
  ratio: string;
};

export type Order = {
  accountId: string;
  botId: number;
  exchange: string;
  symbol: string;
  clientOrderId: string;
  orderId: string;
  drivedOrderId: string;
  side: string;
  isBuy: boolean;
  orderType: string;
  algoType: string;
  source: string;
  price: string;
  originalQty: string;
  executedQty: string;
  originalQuoteQty: string;
  executedQuoteQty: string;
  avgPrice: string;
  priceWorkingType: string;
  priceMode: string;
  status: string;
  timeInForce: string;
  reduceOnly: boolean;
  closePosition: boolean;
  postOnly: boolean;
  priceProtect: boolean;
  conditions: OrderCondition[];
  isWorking: boolean;
  workingTs: number;
  rejectReason: string;
  createdTs: number;
  updatedTs: number;
  finishedTs: number;
  locked?: string;
  lockedAsset?: string;
  fee?: string;
  feeAsset?: string;
  realizedPnl?: string;
  pnlAsset?: string; // 现货订单 realizedPnl 对应的资产；买入=quote，卖出=base
  allocations?: OrderAllocation[];
};

const QUERY_ACCOUNTS = `
  query QueryAccounts($input: QueryAccountsInput!) {
    Result: Accounts(input: $input) {
      totalCount
      list {
        id
        name
        exchange
        apiKey
        apiSecret
        passphrase
        tags
        status
        algorithm
        accountType
        parentAccountId
        multiBotMode
        createdAt
        updatedAt
        config {
          maxOrderSize
          maxPositionPerSymbol {
            amount
            ratio
          }
          maxDailyLoss {
            amount
            ratio
          }
          maxLeverage
          maxOrdersPerMinute
          minMaintenanceMarginRatio
          maxTotalNetExposure {
            amount
            ratio
          }
          maxTotalGrossExposure {
            amount
            ratio
          }
          riskIndexThreshold
          riskIndexAction
          cooldownSeconds
        }
        stats {
          notional
          unRealizedProfit
          notional24HChange
        }
      }
    }
  }
`;

const QUERY_ACCOUNT = `
  query QueryAccounts($input: QueryAccountsInput!) {
    Result: Accounts(input: $input) {
      totalCount
      list {
        id
        name
        exchange
        apiKey
        apiSecret
        passphrase
        tags
        status
        algorithm
        accountType
        parentAccountId
        multiBotMode
        createdAt
        updatedAt
        config {
          maxOrderSize
          maxPositionPerSymbol {
            amount
            ratio
          }
          maxDailyLoss {
            amount
            ratio
          }
          maxLeverage
          maxOrdersPerMinute
          minMaintenanceMarginRatio
          maxTotalNetExposure {
            amount
            ratio
          }
          maxTotalGrossExposure {
            amount
            ratio
          }
          riskIndexThreshold
          riskIndexAction
          cooldownSeconds
        }
        riskIndex
        stats {
          notional
          unRealizedProfit
          notional24HChange
        }
      }
    }
  }
`;

const CREATE_ACCOUNT = `
  mutation CreateAccount($input: MutationAccountInput!) {
    Result: CreateAccount(input: $input) {
      id
      name
      exchange
      apiKey
      apiSecret
      passphrase
      tags
      status
      algorithm
      accountType
      parentAccountId
      multiBotMode
      createdAt
      updatedAt
    }
  }
`;

const QUERY_ACCOUNT_UNALLOCATED = `
  query AccountUnallocatedAssets($accountId: ID!) {
    Result: AccountUnallocatedAssets(accountId: $accountId) {
      asset
      walletType
      parentTotal
      subsAllocated
      unallocated
    }
  }
`;

const QUERY_ACCOUNT_MULTI_BOT_DETAILS = `
  query AccountMultiBotDetails($accountId: ID!) {
    Result: AccountMultiBotDetails(accountId: $accountId) {
      subAccounts {
        accountId
        name
        createdAt
      }
      assetAllocations {
        asset
        walletType
        parentTotal
        subAllocations {
          accountId
          amount
        }
        unallocated
      }
      positionAllocations {
        symbol
        side
        parentTotal
        subAllocations {
          accountId
          amount
        }
        unallocated
      }
    }
  }
`;

const UPDATE_ACCOUNT = `
  mutation UpdateAccount($input: MutationAccountInput!) {
    Result: UpdateAccount(input: $input) {
      id
      name
      exchange
      apiKey
      apiSecret
      passphrase
      tags
      status
      algorithm
      accountType
      parentAccountId
      multiBotMode
      createdAt
      updatedAt
    }
  }
`;

const ONLINE_ACCOUNT = `
  mutation OnlineAccount($id: ID!) {
    Result: OnlineAccount(id: $id) {
      id
      name
      exchange
      apiKey
      apiSecret
      passphrase
      tags
      status
      algorithm
      accountType
      createdAt
      updatedAt
    }
  }
`;

const OFFLINE_ACCOUNT = `
  mutation OfflineAccount($id: ID!) {
    Result: OfflineAccount(id: $id) {
      id
      name
      exchange
      apiKey
      apiSecret
      passphrase
      tags
      status
      algorithm
      accountType
      createdAt
      updatedAt
    }
  }
`;

const DELETE_ACCOUNT = `
  mutation DeleteAccount($id: ID!) {
    Result: DeleteAccount(id: $id)
  }
`;

export async function queryAccounts(params: QueryAccountsParams) {
  const pageSize = params.pageSize || 10;
  const input: any = {
    limit: pageSize,
    offset: ((params?.current || 1) - 1) * pageSize,
    createdAtStart: params.createdAtRange ? dayjs(params.createdAtRange[0]).unix() : undefined,
    createdAtEnd: params.createdAtRange ? dayjs(params.createdAtRange[1]).unix() : undefined,
    id: params.id,
    name: params.name,
    tags: params.tags,
    status: params.status,
    exchange: params.exchange,
    accountType: (() => {
      const raw = params.accountType ?? (params as { type?: AccountType | string }).type;
      if (raw === undefined || raw === null || raw === '') {
        return undefined;
      }
      const s = String(raw);
      if (s === AccountType.Unspecified) {
        return undefined;
      }
      return s;
    })(),
  };
  Object.keys(input).forEach((key) => {
    if (input[key] === undefined || input[key] === null || input[key] === '') {
      delete input[key];
    }
  });
  let response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_ACCOUNTS,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result;
}

export async function queryAccount(id: string) {
  const input: any = {
    offset: 0,
    limit: 1,
    id: id,
  };
  let response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_ACCOUNT,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result;
}

export async function queryAccountUnallocatedAssets(
  accountId: string,
): Promise<AccountUnallocatedAsset[]> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_ACCOUNT_UNALLOCATED,
      variables: { accountId },
    }),
  });
  return (response.data?.Result || []) as AccountUnallocatedAsset[];
}

export async function queryAccountMultiBotDetails(accountId: string): Promise<AccountMultiBotDetails> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_ACCOUNT_MULTI_BOT_DETAILS,
      variables: { accountId },
    }),
  });
  return (response.data?.Result || {
    subAccounts: [],
    assetAllocations: [],
    positionAllocations: [],
  }) as AccountMultiBotDetails;
}

export async function createAccount(params: Account) {
  if (!params.passphrase) {
    params.passphrase = '';
  }
  // MutationAccountInput 不含查询结果中的只读字段，避免 GraphQL unknown field
  delete (params as any).parentAccountId;
  delete (params as any).config;
  delete (params as any).riskIndex;
  delete (params as any).stats;
  delete (params as any).createdAt;
  delete (params as any).updatedAt;
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: CREATE_ACCOUNT,
      variables: {
        input: params,
      },
    }),
  });
}

export async function updateAccount(params: any) {
  delete params.createdAt;
  delete params.updatedAt;
  delete params.stats;
  delete params.parentAccountId;
  delete params.config;
  delete params.riskIndex;
  if (!params.passphrase) {
    params.passphrase = '';
  }
  delete params.accountType;
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: UPDATE_ACCOUNT,
      variables: {
        input: params,
      },
    }),
  });
}

export async function onlineAccount(id: string) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: ONLINE_ACCOUNT,
      variables: {
        id,
      },
    }),
  });
}

export async function offlineAccount(id: string) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: OFFLINE_ACCOUNT,
      variables: {
        id,
      },
    }),
  });
}

export async function deleteAccount(id: string) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: DELETE_ACCOUNT,
      variables: {
        id,
      },
    }),
  });
}

const GET_BALANCE = `
  query GetBalance($input: QueryBalanceInput!) {
    Result: Balance(input: $input) {
      notional
      assets {
        code
        balance
        locked
        notional
        walletType
        updatedTs
      }
    }
  }
`;

const GET_POSITIONS = `
  query GetPositions($input: QueryPositionsInput!) {
    Result: Positions(input: $input) {
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

const QUERY_ORDERS = `
  query QueryOrders($input: QueryOrdersInput!) {
    Result: Orders(input: $input) {
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
        price
        originalQty
        executedQty
        originalQuoteQty
        executedQuoteQty
        avgPrice
        priceWorkingType
        priceMode
        status
        timeInForce
        reduceOnly
        closePosition
        postOnly
        priceProtect
        conditions {
          triggerType
          orderPrice
          callbackDistance
          callbackRate
          activationPrice
          priceWorkingType
          priceMode
          isTrailing
          activated
          activatedTs
        }
        isWorking
        workingTs
        rejectReason
        createdTs
        updatedTs
        finishedTs
        locked
        lockedAsset
        fee
        feeAsset
        realizedPnl
        pnlAsset
        allocations {
          accountId
          ratio
        }
      }
      totalCount
    }
  }
`;

const QUERY_LEDGERS = `
  query QueryLedgers($input: QueryLedgersInput!) {
    Result: Ledgers(input: $input) {
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
      totalCount
    }
  }
`;

const QUERY_ACCOUNT_INFO = `
  query QueryAccountInfo($input: QueryAccountInfoInput!) {
    Result: AccountInfo(input: $input) {
      exchange
      uid
      isSpotEnabled
      isFutureEnabled
    }
  }
`;

const QUERY_EQUITYS = `
  query QueryEquitys($input: QueryEquitysInput!) {
    Result: Equitys(input: $input) {
      id
      accountId
      ts
      notional
      unRealizedProfit
      createdAt
    }
  }
`;

const REFRESH_ACCOUNT_SNAPSHOTS = `
  mutation RefreshAccountSnapshots($accountId: ID!) {
    Result: RefreshAccountSnapshots(accountId: $accountId)
  }
`;

const GET_LEVERAGE = `
  query GetLeverage($accountId: ID!, $symbol: String!) {
    Result: Leverage(accountId: $accountId, symbol: $symbol)
  }
`;

const SET_LEVERAGE = `
  mutation SetLeverage($accountId: ID!, $symbol: String!, $leverage: Int!) {
    Result: SetLeverage(accountId: $accountId, symbol: $symbol, leverage: $leverage)
  }
`;

const UPDATE_ACCOUNT_RISK_CONFIG = `
  mutation UpdateAccountRiskConfig($input: UpdateAccountRiskConfigInput!) {
    Result: UpdateAccountRiskConfig(input: $input) {
      id
      config {
        maxOrderSize
        maxPositionPerSymbol {
          amount
          ratio
        }
        maxDailyLoss {
          amount
          ratio
        }
        maxLeverage
        maxOrdersPerMinute
        minMaintenanceMarginRatio
        maxTotalNetExposure {
          amount
          ratio
        }
        maxTotalGrossExposure {
          amount
          ratio
        }
        riskIndexThreshold
        riskIndexAction
        cooldownSeconds
      }
    }
  }
`;

const PLACE_ORDER = `
  mutation PlaceOrder($input: PlaceOrderInput!) {
    Result: PlaceOrder(input: $input) {
      orderId
      clientOrderId
    }
  }
`;

const CANCEL_ORDER = `
  mutation CancelOrder($input: CancelOrderInput!) {
    Result: CancelOrder(input: $input)
  }
`;

const QUERY_ACCOUNT_METRICS = `
  query QueryAccountMetrics($input: QueryAccountMetricsInput!) {
    Result: AccountMetrics(input: $input) {
      accountId
      dimension
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

const QUERY_ACCOUNT_EVENT_FLOW = `
  query QueryAccountEventFlow($input: QueryAccountEventFlowInput!) {
    Result: AccountEventFlow(input: $input) {
      events {
        id
        accountId
        exchange
        stream
        topic
        eventKind
        tsMs
        receiveAtMs
        publishAtMs
        ingestAtMs
        payloadJson
      }
      nextId
    }
  }
`;

const QUERY_RISK_EVENTS = `
  query QueryRiskEvents($input: QueryRiskEventsInput!) {
    Result: RiskEvents(input: $input) {
      id
      accountId
      exchange
      rule
      riskIndex
      payloadJson
      createdAt
    }
  }
`;

const ESTIMATE_ORDER = `
  query EstimateOrder($input: EstimateOrderInput!) {
    Result: EstimateOrder(input: $input) {
      liquidationPrice
      fee
      feeAsset
      expectedPnl
    }
  }
`;

export enum EventFlowStream {
  All = 'all',
  AccountRaw = 'accountRaw',
  Account = 'account',
}

export type EventRecord = {
  id: string;
  accountId: string;
  exchange: string;
  stream: string;
  topic: string;
  eventKind: string;
  tsMs: number;
  receiveAtMs: number;
  publishAtMs: number;
  ingestAtMs: number;
  payloadJson: string;
};

export type EventRecordsConnection = {
  events: EventRecord[];
  nextId: string;
};

export type RiskEvent = {
  id: number;
  accountId: string;
  exchange: string;
  rule: string;
  riskIndex?: string | null;
  payloadJson?: string | null;
  createdAt: number;
};

export type SymbolMetrics = {
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

export type AccountMetricsResponse = {
  accountId: string;
  dimension: string;
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
  symbols: SymbolMetrics[];
};

export async function queryAccountMetrics(
  accountId: string,
  dimension: 'account' | 'symbol' = 'account',
  options?: { symbol?: string; startTs?: number; endTs?: number },
) {
  const input: Record<string, unknown> = {
    accountId,
    dimension,
  };
  if (options?.symbol) input.symbol = options.symbol;
  if (options?.startTs != null) input.startTs = options.startTs;
  if (options?.endTs != null) input.endTs = options.endTs;
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_ACCOUNT_METRICS,
      variables: { input },
    }),
  });
  return response.data?.Result as AccountMetricsResponse | null;
}

export async function getBalance(accountId: string, source?: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GET_BALANCE,
      variables: {
        input: {
          accountId,
          withNotional: true,
          source: source || 'db',
        },
      },
    }),
  });
  return response.data?.Result as Balance;
}

export async function queryEquitys(accountId: string, range: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_EQUITYS,
      variables: {
        input: {
          accountId,
          range,
        },
      },
    }),
  });
  return response.data?.Result as AccountEquity[];
}

export async function getPositions(accountId: string, source?: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GET_POSITIONS,
      variables: {
        input: {
          accountId,
          source: source || 'db',
        },
      },
    }),
  });
  return response.data?.Result as Position[];
}

export async function getOrders(input: Record<string, any>) {
  const requestInput = {
    page: 1,
    size: 50,
    ...input,
  };
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_ORDERS,
      variables: {
        input: requestInput,
      },
    }),
  });
  return response.data?.Result as OrdersConnection;
}

export async function getLedgers(
  accountId: string,
  startTs: number,
  endTs: number,
  size: number = 20,
  page: number = 1,
  source?: string,
) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_LEDGERS,
      variables: {
        input: {
          accountId,
          startTs,
          endTs,
          size,
          page,
          source: source || 'db',
        },
      },
    }),
  });
  return response.data?.Result as LedgersConnection;
}

export async function queryAccountInfo(accountId: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_ACCOUNT_INFO,
      variables: {
        input: {
          accountId,
        },
      },
    }),
  });
  return response.data?.Result as AccountInfo;
}

export async function refreshAccountSnapshots(accountId: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: REFRESH_ACCOUNT_SNAPSHOTS,
      variables: {
        accountId,
      },
    }),
  });
  return response.data?.Result as boolean;
}

export async function getLeverage(accountId: string, symbol: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GET_LEVERAGE,
      variables: {
        accountId,
        symbol,
      },
    }),
  });
  const value = response.data?.Result;
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

export async function setLeverage(accountId: string, symbol: string, leverage: number) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: SET_LEVERAGE,
      variables: {
        accountId,
        symbol,
        leverage,
      },
    }),
  });
  if (response.errors) {
    throw new Error(response.errors[0].message);
  }
  return response.data?.Result;
}

export async function updateAccountRiskConfig(input: {
  accountId: string;
  maxOrderSize?: string;
  maxPositionPerSymbol?: AmountLimit;
  maxDailyLoss?: AmountLimit;
  maxLeverage?: string;
  maxOrdersPerMinute?: number;
  minMaintenanceMarginRatio?: string;
  maxTotalNetExposure?: AmountLimit;
  maxTotalGrossExposure?: AmountLimit;
  riskIndexThreshold?: string;
  riskIndexAction?: string;
}) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: UPDATE_ACCOUNT_RISK_CONFIG,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result as Account | null;
}

// estimateLiquidationPrice 已被 EstimateOrder 替代，保留函数以兼容老代码（当前已无调用方，可后续删除）

export type EstimateOrderResult = {
  liquidationPrice?: string | null;
  fee?: string | null;
  feeAsset?: string | null;
  expectedPnl?: string | null;
};

export async function estimateOrder(params: {
  accountId: string;
  symbol: string;
  side: PositionSide;
  isBuy: boolean;
  orderType: OrderType;
  price: string;
  notional: string;
  leverage?: number;
}): Promise<EstimateOrderResult | null> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: ESTIMATE_ORDER,
      variables: {
        input: {
          accountId: params.accountId,
          symbol: params.symbol,
          side: params.side,
          isBuy: params.isBuy,
          orderType: params.orderType,
          price: params.price,
          notional: params.notional,
          leverage: params.leverage,
        },
      },
    }),
  });
  return (response.data?.Result ?? null) as EstimateOrderResult | null;
}

export type PlaceOrderResult = {
  orderId: string;
  clientOrderId: string;
};

export async function placeOrder(input: {
  accountId: string;
  symbol: string;
  side: PositionSide;
  isBuy: boolean;
  orderType: OrderType;
  price?: string;
  quantity: string;
  timeInForce?: string;
  reduceOnly?: boolean;
  closePosition?: boolean;
}) {
  return request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: PLACE_ORDER,
      variables: {
        input,
      },
    }),
  })
    .then((response) => {
      if (response.errors) {
        throw new Error(response.errors[0].message);
      }
      return response.data?.Result as PlaceOrderResult;
    })
    .catch((error) => {
      throw new Error(error.message);
    });
}

export async function cancelOrder(
  accountId: string,
  symbol: string,
  clientOrderId: string,
  orderId?: string,
) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: CANCEL_ORDER,
      variables: {
        input: {
          accountId,
          symbol,
          clientOrderId,
          orderId,
        },
      },
    }),
  });
  return response.data?.Result as boolean;
}

export async function queryAccountEventFlow(
  accountId: string,
  stream: EventFlowStream,
  startTsMs?: number,
  startId?: string,
  limit?: number,
) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_ACCOUNT_EVENT_FLOW,
      variables: {
        input: {
          accountId,
          stream,
          startTsMs,
          startId,
          limit: limit || 100,
        },
      },
    }),
  });
  return response.data?.Result as EventRecordsConnection;
}

export async function queryRiskEvents(
  accountId: string,
  options?: { limit?: number; offset?: number },
) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_RISK_EVENTS,
      variables: {
        input: {
          accountId,
          limit: options?.limit,
          offset: options?.offset,
        },
      },
    }),
  });
  return (response.data?.Result ?? []) as RiskEvent[];
}
