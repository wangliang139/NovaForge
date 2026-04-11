export type EditIndicator<T, M extends string = 'new' | 'edit' | 'readonly'> = {
  mode?: M;
  open: boolean;
  value?: T | null;
  index?: number;
};

export enum Exchange {
  Binance = 'binance',
  OKX = 'okx',
  BinanceTest = 'binance_test',
  OKXTest = 'okx_test',
  // Bitfinex = 'bitfinex',
  // Bittrex = 'bittrex',
  // Coinbase = 'coinbase',
  // Huobi = 'huobi',
  // Kraken = 'kraken',
}

export enum SymbolStatus {
  Unspecified = 'unspecified',
  Testing = 'testing',
  PreTrading = 'pre_trading',
  Trading = 'trading',
  PostTrading = 'post_trading',
  EndOfDay = 'end_of_day',
  Halt = 'halt',
  AuctionMatch = 'auction_match',
  Break = 'break',
}

export type Symbol = {
  id: string;
  name: string;
  base: string;
  quote: string;
  exchanges: ExSymbol[];
  createdAt: number;
  updatedAt: number;
};

export type ExSymbol = {
  exchange: Exchange;
  status: SymbolStatus;
  base: string;
  quote: string;
  minTick: string;
  lotSize: string;
  minSize: string;
  minNotional: string;
};

export enum MarketType {
  Unspecified = 'unspecified',
  Spot = 'spot',
  Future = 'future',
}

// Symbol 解析结果
export type ParsedSymbol = {
  base: string;
  quote: string;
  type: MarketType;
};
