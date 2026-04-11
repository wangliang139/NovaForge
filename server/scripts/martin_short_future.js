const Decimal = require("decimal.js");

// ======================
// 马丁策略（合约做空，默认 10x）
// - 使用 symbol.Sell() 开空（Side=SHORT, IsBuy=false）
// - 使用 symbol.Buy({side:"SHORT", isBuy:true}) 平空（需要后端支持 opts 覆盖 side/isBuy）
// ======================

// ========== 参数（可通过 params 覆盖） ==========
var defaultLeverage = 10; // 杠杆倍数（做空方向）
var orderType = "market";

// 资金与下单
var marginPercent = 0.02; // 初始开仓使用余额的保证金比例（例如 0.02=2%）
var minNotionalQuote = 10; // 最小名义价值（USDT）

// 马丁参数
var stepUpPercent = 0.8; // 价格每上涨 step% 加一次仓（做空被套加仓）
var multiplier = 2.0; // 每次加仓名义价值倍数
var maxAdds = 6; // 最大加仓次数

// 出场与风控
var takeProfitPercent = 0.4; // 止盈：价格 <= 均价*(1-tp%)
var stopLossPercent = 8.0; // 止损：价格 >= 均价*(1+sl%)（触发直接平空并进入冷却）
var cooldownMs = 5 * 60 * 1000; // 平仓后冷却时间

// ========== 初始化 ==========
function onInit() {
  console.log("策略初始化：马丁策略（合约做空）");

  if (params) {
    if (params.leverage) defaultLeverage = parseInt(params.leverage);
    if (params.orderType) orderType = params.orderType;

    if (params.marginPercent) marginPercent = parseFloat(params.marginPercent);
    if (params.minNotionalQuote) minNotionalQuote = parseFloat(params.minNotionalQuote);

    if (params.stepUpPercent) stepUpPercent = parseFloat(params.stepUpPercent);
    if (params.multiplier) multiplier = parseFloat(params.multiplier);
    if (params.maxAdds) maxAdds = parseInt(params.maxAdds);

    if (params.takeProfitPercent) takeProfitPercent = parseFloat(params.takeProfitPercent);
    if (params.stopLossPercent) stopLossPercent = parseFloat(params.stopLossPercent);
    if (params.cooldownMs) cooldownMs = parseInt(params.cooldownMs);
  }

  if (!defaultLeverage || defaultLeverage <= 0) defaultLeverage = 10;
  if (!maxAdds || maxAdds < 0) maxAdds = 0;

  console.log(`params: leverage=${defaultLeverage}, marginPercent=${marginPercent}, minNotionalQuote=${minNotionalQuote}, stepUpPercent=${stepUpPercent}, multiplier=${multiplier}, maxAdds=${maxAdds}, takeProfitPercent=${takeProfitPercent}, stopLossPercent=${stopLossPercent}, cooldownMs=${cooldownMs}`);

  for (var i = 0; i < symbols.length; i++) {
    var sym = symbols[i];
    console.log("symbol", JSON.stringify(sym));
    if (sym.type !== "FUTURE") continue;
    sym.SetLeverage("SHORT", defaultLeverage);
    sym.SetLeverage("LONG", defaultLeverage);
  }
}

// ========== 主逻辑 ==========
function onSignal(signal) {
  try {
    // 只处理 K 线信号
    if (signal.type !== "kline") {
      if (signal.type !== "timer" && signal.kind !== 'order_snapshot') {
        const signalStr = JSON.stringify(signal);
        console.log(`${signal.type} signal:`, signalStr);
      }
      return;
    }

    if (signal.type !== "kline" || signal.kind !== "bar_close") return;
    if (!signal.exchange || !signal.symbol) return;

    var sym = WithSymbol(signal.exchange, signal.symbol);
    if (!sym) return;

    // 只对合约标的生效
    if (sym.type !== "FUTURE") return;

    var leverage = sym.GetLeverage("SHORT");
    if (leverage !== defaultLeverage) {
      console.error("杠杆不匹配:", sym.exchange, sym.symbol, "side:SHORT, leverage:", defaultLeverage, leverage);
      return;
    }

    var close = new Decimal(signal.close);
    if (!close || close.lte(0)) return;

    var nowMs = signal.ts;

    // 冷却
    var coolUntil = sym.Get("m_coolUntil");
    if (coolUntil && nowMs < coolUntil) return;

    // 读取当前空仓
    var pos = getShortPosition(sym);
    var hasShort = pos && pos.qty.gt(0);

    // 如果没有空仓：开第一单
    if (!hasShort) {
      resetState(sym);
      console.log("pos", JSON.stringify(pos));
      var notional = calcBaseNotional(sym, leverage);
      if (notional.lt(minNotionalQuote)) {
        console.log("名义价值不足，跳过开仓:", notional.toFixed(2));
        return;
      }
      openShort(sym, notional, close, nowMs, "init");
      return;
    }

    // 已有空仓：计算均价、触发条件
    var avgEntry = new Decimal(pos.entryPrice);
    if (avgEntry.lte(0)) return;

    // 止盈
    var tpPx = avgEntry.mul(new Decimal(1).minus(new Decimal(takeProfitPercent).div(100)));
    if (close.lte(tpPx)) {
      console.log("pos", JSON.stringify(pos));
      console.log(
        "触发止盈，平空:",
        JSON.stringify({ avgEntry: avgEntry.toFixed(8), close: close.toFixed(8), tpPx: tpPx.toFixed(8) })
      );
      closeShort(sym, pos.qty, close, nowMs, "tp");
      return;
    }

    // 止损
    var slPx = avgEntry.mul(new Decimal(1).plus(new Decimal(stopLossPercent).div(100)));
    if (close.gte(slPx)) {
      console.log("pos", JSON.stringify(pos));
      console.log(
        "触发止损，平空并进入冷却:",
        JSON.stringify({ avgEntry: avgEntry.toFixed(8), close: close.toFixed(8), slPx: slPx.toFixed(8) })
      );
      closeShort(sym, pos.qty, close, nowMs, "sl");
      sym.Set("m_coolUntil", nowMs + cooldownMs);
      return;
    }

    // 加仓：价格上涨到阈值
    var addCount = sym.Get("m_addCount") || 0;
    if (addCount >= maxAdds) return;

    var lastAddPxStr = sym.Get("m_lastAddPx");
    var lastAddPx = lastAddPxStr ? new Decimal(lastAddPxStr) : avgEntry;
    if (lastAddPx.lte(0)) lastAddPx = avgEntry;

    var triggerPx = lastAddPx.mul(new Decimal(1).plus(new Decimal(stepUpPercent).div(100)));
    if (close.gte(triggerPx)) {
      console.log("pos", JSON.stringify(pos));
      var baseNotional = calcBaseNotional(sym, leverage);
      var addNotional = baseNotional.mul(new Decimal(Math.pow(multiplier, addCount + 1)));
      if (addNotional.lt(minNotionalQuote)) addNotional = new Decimal(minNotionalQuote);

      console.log(
        "触发马丁加仓（做空）:",
        JSON.stringify({
          addCount: addCount + 1,
          close: close.toFixed(8),
          triggerPx: triggerPx.toFixed(8),
          addNotional: addNotional.toFixed(2),
        })
      );

      openShort(sym, addNotional, close, nowMs, "add");
      sym.Set("m_addCount", addCount + 1);
      sym.Set("m_lastAddPx", close.toFixed(8));
      return;
    }
  } catch (e) {
    console.error("onSignal 异常:", e);
    if (e && e.stack) console.error(e.stack);
  }
}

function getShortPosition(sym) {
  var positions = sym.GetPositions("SHORT");
  if (!positions || positions.length === 0) return null;
  // 取第一条 SHORT（通常只有一条）
  for (var i = 0; i < positions.length; i++) {
    var p = positions[i];
    if (!p) continue;
    if (p.side !== "SHORT") continue;
    var qty = new Decimal(p.amount || "0");
    if (qty.gt(0)) {
      return { qty: qty, entryPrice: p.entryPrice || "0", markPrice: p.markPrice || "0" };
    }
  }
  return null;
}

function calcBaseNotional(sym, leverage) {
  // 保证金比例 -> 名义价值 = margin * leverage
  var bal = sym.GetAsset(sym.quote);
  if (!bal) return new Decimal(0);
  var free = new Decimal(bal.balance - bal.locked || "0");
  if (free.lte(0)) return new Decimal(0);
  var margin = free.mul(new Decimal(marginPercent));
  var notional = margin.mul(new Decimal(leverage));
  return notional;
}

function openShort(sym, notionalQuote, px, nowMs, reason) {
  var opts = { type: orderType };
  if (orderType === "market") {
    opts.quoteQty = new Decimal(notionalQuote).toFixed(8);
  } else {
    opts.price = px.toFixed(8);
    opts.quoteQty = new Decimal(notionalQuote).toFixed(8);
  }

  console.log("开空下单:", JSON.stringify({ reason: reason, exchange: sym.exchange, symbol: sym.symbol, opts: opts }));
  var res = sym.Sell(opts); // FUTURE: Side=SHORT, IsBuy=false => 开空
  if (res && res.error) {
    console.error("开空失败:", res.error);
  } else {
    sym.Set("m_lastActionTs", nowMs);
  }
}

function closeShort(sym, qty, px, nowMs, reason) {
  if (!qty || new Decimal(qty).lte(0)) return;

  // 关键：用 Buy 发“SHORT side 的买单”来平空
  var opts = { type: orderType, side: "SHORT", isBuy: true };
  if (orderType === "market") {
    opts.quantity = new Decimal(qty).toFixed(8);
  } else {
    opts.price = px.toFixed(8);
    opts.quantity = new Decimal(qty).toFixed(8);
  }

  console.log("平空下单:", JSON.stringify({ reason: reason, exchange: sym.exchange, symbol: sym.symbol, opts: opts }));
  var res = sym.Buy(opts);
  if (res && res.error) {
    console.error("平空失败:", res.error);
    return;
  }

  resetState(sym);
  sym.Set("m_lastActionTs", nowMs);
}

function resetState(sym) {
  sym.Set("m_addCount", 0);
  sym.Set("m_lastAddPx", null);
}
