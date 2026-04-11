// @ts-ignore
/* eslint-disable */
import { request } from '@umijs/max';

export const QUERY_MARKETS = `
  query QueryMarkets($input: GetMarketsInput!) {
    Markets(input: $input) {
      exchange
      symbol
      status
    }
  }
`;

export const QUERY_MARKET = `
  query QueryMarket($input: GetMarketInput!) {
    Market(input: $input) {
      exchange
      symbol
      status
      baseAssetPrecision
      quoteAssetPrecision
      pricePrecision
      rules {
        maxOrderNum
        minPrice
        maxPrice
        tickSize
        minQuantity
        maxQuantity
        lotSize
        minNotional
        maxNotional
      }
      supportOrderTypes {
        orderType
        rules {
          maxOrderNum
          minPrice
          maxPrice
          tickSize
          minQuantity
          maxQuantity
          lotSize
          minNotional
          maxNotional
        }
      }
    }
  }
`;

export const QUERY_KLINE = `
  query QueryKline($input: QueryKlineInput!) {
    Result: Kline(input: $input) {
      interval
      open
      high
      low
      close
      volume
      quoteVolume
      trades
      openTs
      closeTs
    }
  }
`;

export const QUERY_FUNDING_RATE = `
  query QueryFundingRate($input: QueryFundingRateInput!) {
    Result: FundingRate(input: $input) {
      exchange
      symbol
      fundingRate
      interestRate
      nextFundingTime
      ts
    }
  }
`;

export const QUERY_FUNDING_RATES = `
  query QueryFundingRates($input: QueryFundingRatesInput!) {
    Result: FundingRates(input: $input) {
      exchange
      symbol
      fundingRate
      interestRate
      nextFundingTime
      ts
    }
  }
`;

export const QUERY_OPEN_INTEREST = `
  query QueryOpenInterest($input: QueryOpenInterestInput!) {
    Result: OpenInterest(input: $input) {
      exchange
      symbol
      openInterest
      ts
    }
  }
`;

export const QUERY_LEVERAGE_BRACKET = `
  query QueryLeverageBracket($input: QueryLeverageBracketInput!) {
    Result: LeverageBracket(input: $input) {
      symbol
      brackets {
        bracket
        maxLeverage
        minNotional
        maxNotional
        mmr
        cum
      }
    }
  }
`;

export const QUERY_INDEX_PRICE = `
  query QueryIndexPrice($input: QueryIndexPriceInput!) {
    Result: IndexPrice(input: $input) {
      exchange
      symbol
      indexPrice
      ts
    }
  }
`;

export const QUERY_INDEX_COMPONENT = `
  query QueryIndexComponent($input: QueryIndexComponentInput!) {
    Result: IndexComponent(input: $input) {
      exchange
      symbol
      price
      ts
      components {
        exchange
        symbol
        price
        weight
      }
    }
  }
`;

export const QUERY_LIST_ALERTS = `
  query QueryListAlerts($exchange: Exchange!, $symbol: String!) {
    Result: ListAlerts(exchange: $exchange, symbol: $symbol) {
      id
      exchange
      symbol
      type
      frequency
      price
      window
      percent
      remark
      cooldownSeconds
      status
      lastTriggeredAt
      triggerCount
      createdAt
      updatedAt
    }
  }
`;

export const MUTATION_ADD_ALERT = `
  mutation MutationAddAlert($input: AlertItemInput!) {
    Result: AddAlert(input: $input) {
      id
      exchange
      symbol
      type
      frequency
      price
      window
      percent
      remark
      cooldownSeconds
      status
      lastTriggeredAt
      triggerCount
      createdAt
      updatedAt
    }
  }
`;

export const MUTATION_REMOVE_ALERT = `
  mutation MutationRemoveAlert($id: ID!) {
    Result: RemoveAlert(id: $id)
  }
`;

/** 获取 Markets 列表 */
export async function queryMarkets(input: {
  exchange: string;
  marketTypes?: string[];
  accountId?: number;
}) {
  let response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_MARKETS,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Markets || [];
}

/** 获取单个 Market（exchange + symbol 必填） */
export async function queryMarket(input: { exchange: string; symbol: string; accountId?: number }) {
  let response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_MARKET,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Market || null;
}

/** 查询历史 K 线（startTime/endTime/limit 必填，单位：毫秒） */
export async function queryKline(input: {
  symbol: string;
  exchange: string;
  interval: string;
  startTime?: number;
  endTime?: number;
  limit?: number;
}) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_KLINE,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result || [];
}

/** 查询资金费率（合约） */
export async function queryFundingRate(input: { exchange: string; symbol: string; accountId?: number }) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_FUNDING_RATE,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result || null;
}

/** 查询资金费率历史（合约） */
export async function queryFundingRates(input: {
  exchange: string;
  symbol: string;
  startTime?: number;
  endTime?: number;
  limit?: number;
  accountId?: number;
}) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_FUNDING_RATES,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result || [];
}

/** 查询合约未平仓量（base） */
export async function queryOpenInterest(input: { exchange: string; symbol: string; accountId?: number }) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_OPEN_INTEREST,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result || null;
}

/** 查询杠杆档位（合约） */
export async function queryLeverageBracket(input: {
  exchange: string;
  symbol: string;
  markPrice: string;
  accountId?: string;
}) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_LEVERAGE_BRACKET,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result || null;
}

/** 查询指数价格（合约） */
export async function queryIndexPrice(input: { exchange: string; symbol: string; accountId?: number }) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_INDEX_PRICE,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result || null;
}

/** 查询指数构成（合约） */
export async function queryIndexComponent(input: { exchange: string; symbol: string; accountId?: number }) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_INDEX_COMPONENT,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result || null;
}

export type AlertItemDTO = {
  id: string;
  exchange: string;
  symbol: string;
  type: string;
  frequency: 'repeat' | 'once';
  price?: string;
  window?: '5m' | '1h' | '4h' | '24h';
  percent?: string;
  remark?: string;
  cooldownSeconds: number;
  status: 'active' | 'error';
  lastTriggeredAt?: number;
  triggerCount: number;
  createdAt: number;
  updatedAt: number;
};

export async function listAlerts(exchange: string, symbol: string): Promise<AlertItemDTO[]> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_LIST_ALERTS,
      variables: { exchange, symbol },
    }),
  });
  return response.data?.Result || [];
}

export type AddAlertInputDTO = {
  exchange: string;
  symbol: string;
  type: string;
  frequency: 'repeat' | 'once';
  price?: string;
  window?: '5m' | '1h' | '4h' | '24h';
  percent?: string;
  remark?: string;
  cooldownSeconds?: number;
};

export async function addAlert(input: AddAlertInputDTO): Promise<AlertItemDTO | null> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: MUTATION_ADD_ALERT,
      variables: { input },
    }),
  });
  return response.data?.Result || null;
}

export async function removeAlert(id: string): Promise<boolean> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: MUTATION_REMOVE_ALERT,
      variables: { id },
    }),
  });
  return Boolean(response.data?.Result);
}
