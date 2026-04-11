import binanceLogo from '@/assets/exchange/binance.jpg';
import binanceTestLogo from '@/assets/exchange/binance_test.png';
import okxLogo from '@/assets/exchange/okx.jpg';
import okxTestLogo from '@/assets/exchange/okx_test.png';
import { Exchange, MarketType, ParsedSymbol, Symbol, SymbolStatus } from '@/global.types';
import { WalletType } from '@/services/gateway/account';

export function getExchangeLogo(exchange: string) {
  switch (exchange) {
    case Exchange.Binance:
      return binanceLogo;
    case Exchange.BinanceTest:
      return binanceTestLogo;
    case Exchange.OKX:
      return okxLogo;
    case Exchange.OKXTest:
      return okxTestLogo;
  }
}

export function getExchangeTitle(exchange: string) {
  switch (exchange) {
    case Exchange.Binance:
      return 'Binance';
    case Exchange.BinanceTest:
      return 'Binance Test';
    case Exchange.OKX:
      return 'OKX';
    case Exchange.OKXTest:
      return 'OKX Test';
  }
}

export function getWalletTypes(exchange: Exchange, marketType: MarketType): WalletType[] {
  if (exchange === Exchange.Binance || exchange === Exchange.BinanceTest) {
    if (marketType === MarketType.Spot) {
      return [WalletType.Spot];
    }
    if (marketType === MarketType.Future) {
      return [WalletType.Future, WalletType.Margin];
    }
  }
  if (exchange === Exchange.OKX || exchange === Exchange.OKXTest) {
    return [WalletType.Trade];
  }
  return [];
}

export function getSymbolStatusColor(status: SymbolStatus) {
  switch (status) {
    case SymbolStatus.Testing:
      return 'warning';
    case SymbolStatus.Trading:
    case SymbolStatus.PreTrading:
    case SymbolStatus.PostTrading:
    case SymbolStatus.AuctionMatch:
      return 'success';
    case SymbolStatus.EndOfDay:
    case SymbolStatus.Break:
      return 'error';
    case SymbolStatus.Halt:
      return 'default';
  }
}

export function getSortedSymbolExchanges(symbol?: Symbol): Exchange[] | undefined {
  const exchanges = symbol?.exchanges.map((exSymbol) => exSymbol.exchange);
  return exchanges?.sort((a, b) => {
    if (a === Exchange.Binance) {
      return -1;
    }
    return 1;
  });
}

/**
 * 解析 symbol 字符串，从 "BTC/USDT:SPOT" 或 "BTC/USDT:FUTURE" 格式中提取 base、quote 和 type
 * @param symbol - symbol 字符串，格式为 "BASE/QUOTE:TYPE" 或 "BASE/QUOTE"
 * @returns ParsedSymbol 对象，包含 base、quote 和 type
 */
export function parseSymbol(symbol: string): ParsedSymbol {
  if (!symbol || typeof symbol !== 'string') {
    return { base: '', quote: '', type: MarketType.Spot };
  }

  const trimmed = symbol.trim().toUpperCase();
  if (!trimmed) {
    return { base: '', quote: '', type: MarketType.Spot };
  }

  // 检查是否包含类型后缀（如 :SPOT 或 :FUTURE）
  const colonIndex = trimmed.lastIndexOf(':');
  let symbolPart = trimmed;
  let type: MarketType = MarketType.Spot;

  if (colonIndex > 0) {
    const typePart = trimmed.substring(colonIndex + 1).toUpperCase();
    symbolPart = trimmed.substring(0, colonIndex).toUpperCase();

    if (typePart === 'SPOT') {
      type = MarketType.Spot;
    } else if (typePart === 'FUTURE' || typePart === 'SWAP') {
      type = MarketType.Future;
    }
  }

  // 解析 base 和 quote
  const parts = symbolPart.split('/');
  if (parts.length !== 2) {
    return { base: '', quote: '', type: MarketType.Spot };
  }

  return {
    base: parts[0].trim(),
    quote: parts[1].trim(),
    type,
  };
}
