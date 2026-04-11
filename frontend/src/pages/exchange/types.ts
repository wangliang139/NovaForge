import { Kline } from '@/services/gateway/market';

export enum SyncerStatus {
  Unspecified = 'unspecified',
  Syncing = 'syncing',
  Normal = 'normal',
  Failed = 'failed',
}

export type SymbolKline = {
  symbol: string;
  depth: number;
  status: SyncerStatus;
  credibility: number;
  lastEventTime: number;
  klines?: Kline[];
};

export function calcKlineAvgPrice(kline?: Kline): number {
  if (!kline) {
    return 0;
  }
  return Number(kline.volume) > 0
    ? Number(kline.quoteVolume) / Number(kline.volume)
    : Number(kline.open);
}
