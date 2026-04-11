import { OrderBook, OrderPriceLevel } from '@/pages/exchange/types';
import Decimal from 'decimal.js';
import utils from '@/utils';

const genTickOptions = (value: string) => {
  let precision = utils.math.getDecimalPrecision(value);
  const minTick = Number(value);
  let ticks = [];
  for (let i = 0; i < 3; i++) {
    let tick = (minTick * Math.pow(10, i)).toFixed(precision >= i ? precision - i : 0);
    ticks.push({
      label: tick,
      value: tick,
    });
  }
  return ticks;
};

const mergeOrderBook = (
  orderBook: OrderBook | undefined,
  tick: string,
  depth: number,
): OrderBook | undefined => {
  if (!orderBook) return;

  let dataTick = new Decimal(orderBook.tick);
  let nowTick = new Decimal(tick);

  if (nowTick.greaterThan(dataTick)) {
    orderBook.tick = tick;
    orderBook.bids = mergeOrderPrice(orderBook.bids, nowTick);
    orderBook.asks = mergeOrderPrice(orderBook.asks, nowTick);
  }

  if (orderBook.bids) {
    orderBook.bids.sort((a, b) => {
      if (typeof a.price === 'string') {
        a.price = new Decimal(a.price);
      }
      if (typeof b.price === 'string') {
        b.price = new Decimal(b.price);
      }
      return (a.price as Decimal).cmp(b.price);
    });
  }
  if (orderBook.asks) {
    orderBook.asks.sort((a, b) => {
      if (typeof a.price === 'string') {
        a.price = new Decimal(a.price);
      }
      if (typeof b.price === 'string') {
        b.price = new Decimal(b.price);
      }
      return -(b.price as Decimal).cmp(a.price);
    });
  }

  if (orderBook?.bids && orderBook.bids.length > depth) {
    orderBook.bids = orderBook.bids.slice(-depth);
  }
  if (orderBook?.asks && orderBook.asks.length > depth) {
    orderBook.asks = orderBook.asks.slice(0, depth);
  }
  orderBook.depth = depth;
  return orderBook;
};

const mergeOrderPrice = (
  orderPrices: OrderPriceLevel[] | undefined,
  tick: Decimal,
): OrderPriceLevel[] | undefined => {
  if (!orderPrices || orderPrices.length === 0) {
    return;
  }
  const merged: { [key: string]: OrderPriceLevel } = {};

  for (const order of orderPrices) {
    if (typeof order.price === 'string') {
      order.price = new Decimal(order.price);
    }
    if (typeof order.quantity === 'string') {
      order.quantity = new Decimal(order.quantity);
    }

    const num = order.price.div(tick).floor();
    const roundedPrice = num.mul(tick);

    const roundedPriceStr = roundedPrice.toString();
    if (!merged[roundedPriceStr]) {
      merged[roundedPriceStr] = {
        price: roundedPrice,
        quantity: new Decimal(0),
        time: order.time,
        latestId: order.latestId,
      };
    }

    merged[roundedPriceStr].quantity = Decimal.add(
      merged[roundedPriceStr].quantity,
      order.quantity,
    );
    if (order.time > merged[roundedPriceStr].time) {
      merged[roundedPriceStr].time = order.time;
    }
    if (order.latestId > merged[roundedPriceStr].latestId) {
      merged[roundedPriceStr].latestId = order.latestId;
    }
  }
  return Object.values(merged);
};

export default {
  genTickOptions,
  mergeOrderBook,
};
