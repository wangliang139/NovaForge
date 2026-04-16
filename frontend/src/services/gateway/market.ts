import { Exchange } from '@/global.types';
import type { Document } from '@/services/gateway/document';
import type { Order, Position, PositionSide, WalletType } from '@/services/gateway/account';
import { request } from '@umijs/max';

export type Ticker = {
  exchange: Exchange;
  symbol: string;
  lastPrice: string;
  open24H: string;
  high24H: string;
  low24H: string;
  avg24H: string;
  volume24H: string;
  quoteVolume24H: string;
  ts: number;
};

export type MarkPrice = {
  exchange: Exchange;
  symbol: string;
  markPrice: string;
  ts: number;
};

export type FundingRate = {
  exchange: Exchange;
  symbol: string;
  fundingRate: string;
  interestRate: string;
  nextFundingTime: number;
  ts: number;
};

export type OpenInterest = {
  exchange: Exchange;
  symbol: string;
  openInterest: string; // base asset 数量
  ts: number;
};

export type Trade = {
  tradeId: string;
  exchange: Exchange;
  symbol: string;
  price: string;
  size: string;
  isBuy: boolean;
  ts: number;
};

export type DepthLevel = { price: string; size: string; ts?: number; seqId?: number };
export type Depth = {
  bids: DepthLevel[];
  asks: DepthLevel[];
  ts: number;
  seqId: number;
  prevSeqId: number;
};

export type MarketRules = {
  tickSize?: string;
  lotSize?: string;
};

export type MarketInfo = {
  exchange: string;
  symbol: string;
  status: string;
  baseAssetPrecision?: number;
  quoteAssetPrecision?: number;
  pricePrecision?: number;
  rules?: MarketRules;
};

export type Bracket = {
  bracket: number;
  maxLeverage: number;
  minNotional: string;
  maxNotional: string;
  mmr: string;
  cum: string;
};

export type LeverageBracket = {
  symbol: string;
  brackets: Bracket[];
};

export type IndexPrice = {
  exchange: string;
  symbol: string;
  indexPrice: string;
  ts: number;
};

export type IndexComponentItem = {
  exchange: string;
  symbol: string;
  price: string;
  weight: string;
};

export type IndexComponent = {
  exchange: string;
  symbol: string;
  price?: string;
  ts: number;
  components: IndexComponentItem[];
};

export type Kline = {
  interval: string;
  open: string;
  high: string;
  low: string;
  close: string;
  volume: string;
  quoteVolume: string;
  trades: number;
  openTs: number;
  closeTs: number;
};

export type AccountStreamAsset = {
  walletType: string;
  code: string;
  balance: string;
  locked: string;
  updatedTs: number;
};

export type AccountBalanceSnapshot = {
  scope: string[];
  assets: AccountStreamAsset[];
};

export type AccountBalanceUpdate = {
  eventId: string;
  type: 'snapshot' | 'increment';
  reason: string;
  assets: AccountStreamAsset[];
};

export type AccountPositionSnapshot = {
  positions: Position[];
};

export type AccountPositionsUpdate = {
  eventId: string;
  type: 'snapshot' | 'increment';
  reason: string;
  positions: Position[];
};

export type StreamSymbolLeverage = {
  exchange: string;
  symbol: string;
  side: string;
  leverage: number;
  updatedTs: number;
};

export type StreamFill = {
  exchange: string;
  symbol: string;
  orderId: string;
  clientOrderId: string;
  tradeId: string;
  side: string;
  isBuy: boolean;
  qty: string;
  price: string;
  fee: string;
  feeAsset: string;
  realizedPnl: string;
  isMaker: boolean;
  ts: number;
};

export type StreamEvent = {
  type: 'ticker' | 'trade' | 'depth' | 'kline' | 'mark_price' | 'social' | 'account';
  eventTs: number;
  ticker?: Ticker;
  trade?: Trade;
  depth?: Depth;
  kline?: Kline;
  markPrice?: MarkPrice;
  social?: Document;
  balanceSnapshot?: AccountBalanceSnapshot;
  balanceUpdate?: AccountBalanceUpdate;
  positionSnapshot?: AccountPositionSnapshot;
  positionsUpdate?: AccountPositionsUpdate;
  order?: Order;
  fill?: StreamFill;
  symbolLeverage?: StreamSymbolLeverage;
};

const GET_ORDER_BOOK = `
  query GetOrderBook($input: QueryOrderBookInput!) {
    Result: OrderBook(input: $input) {
      bids {
        price
        size
      }
      asks {
        price
        size
      }
      ts
      seqId
      prevSeqId
    }
  }
`;

export async function getOrderBook(
  exchange: string,
  symbol: string,
  depth?: number,
): Promise<Depth | null> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GET_ORDER_BOOK,
      variables: {
        input: {
          exchange,
          symbol,
          depth,
        },
      },
    }),
  });
  const result = response.data?.Result;
  if (!result) return null;
  return {
    bids: result.bids,
    asks: result.asks,
    ts: result.ts,
    seqId: result.seqId,
    prevSeqId: result.prevSeqId,
  };
}

const ASSET_SNAPSHOT_HISTORY = `
  query AssetSnapshotHistory($input: QueryAssetSnapshotHistoryInput!) {
    Result: AssetSnapshotHistory(input: $input) {
      tsMs
      total
    }
  }
`;

const POSITION_SNAPSHOT_HISTORY = `
  query PositionSnapshotHistory($input: QueryPositionSnapshotHistoryInput!) {
    Result: PositionSnapshotHistory(input: $input) {
      tsMs
      qty
      entryPrice
    }
  }
`;

export type AssetSnapshotHistoryPoint = {
  tsMs: number;
  total: string;
};

export type PositionSnapshotHistoryPoint = {
  tsMs: number;
  qty: string;
  entryPrice: string;
};

export async function queryAssetSnapshotHistory(input: {
  accountId: string;
  walletType: WalletType;
  asset: string;
  startTsMs: number;
  endTsMs: number;
}): Promise<AssetSnapshotHistoryPoint[]> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: ASSET_SNAPSHOT_HISTORY,
      variables: {
        input: {
          accountId: input.accountId,
          walletType: input.walletType,
          asset: input.asset,
          startTsMs: input.startTsMs,
          endTsMs: input.endTsMs,
        },
      },
    }),
  });
  return response.data?.Result ?? [];
}

export async function queryPositionSnapshotHistory(input: {
  accountId: string;
  symbol: string;
  side: PositionSide;
  startTsMs: number;
  endTsMs: number;
}): Promise<PositionSnapshotHistoryPoint[]> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: POSITION_SNAPSHOT_HISTORY,
      variables: {
        input: {
          accountId: input.accountId,
          symbol: input.symbol,
          side: input.side,
          startTsMs: input.startTsMs,
          endTsMs: input.endTsMs,
        },
      },
    }),
  });
  return response.data?.Result ?? [];
}
