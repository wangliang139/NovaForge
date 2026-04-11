// @ts-ignore
/* eslint-disable */
import { Exchange } from '@/global.types';
import { request } from '@umijs/max';


const QUERY_KLINE = `
  query QueryKline($input: QueryKlineInput!) {
    Kline(input: $input) {
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

export type QueryTradeInfoProps = {
  symbol: string;
  exchange: Exchange;
  depth: number;
  tick: string;
  interval: string;
  startTime?: number;
  endTime?: number;
  limit?: number;
};


export type QueryKlineProps = {
  symbol: string;
  exchange: Exchange;
  interval: string;
  startTime?: number;
  endTime?: number;
  limit?: number;
};

export async function queryKline(params: QueryKlineProps) {
  const response = await request('/query', {
    timeout: 1000,
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_KLINE,
      variables: {
        input: {
          symbol: params.symbol,
          exchange: params.exchange,
          interval: params.interval,
          startTime: params.startTime,
          endTime: params.endTime,
          limit: params.limit,
        },
      },
    }),
  });
  return response.data;
}
