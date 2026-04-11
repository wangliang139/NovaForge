const Decimal = require("decimal.js");

// ========== 策略配置 ==========
// EMA 周期参数（可通过 params 配置）
var emaFastPeriod = 20; // 快线周期
var emaSlowPeriod = 50; // 慢线周期

// 交易参数（可通过 params 配置）
var orderType = "market"; // 订单类型：market 或 limit
var quoteQtyPercent = 0.1; // 每次交易使用的资金比例（0.1 = 10%）
var minQuoteQty = 10; // 最小交易金额（USDT）

// EMA 平滑系数
var alphaFast = null;
var alphaSlow = null;

// ========== 初始化函数 ==========
function onInit() {
  console.log("策略初始化：EMA 金叉/死叉交易策略");

  var paramsStr = JSON.stringify(params);
  console.debug("params", paramsStr);

  // 读取参数配置
  if (params) {
    if (params.emaFastPeriod) {
      emaFastPeriod = parseInt(params.emaFastPeriod);
    }
    if (params.emaSlowPeriod) {
      emaSlowPeriod = parseInt(params.emaSlowPeriod);
    }
    if (params.orderType) {
      orderType = params.orderType;
    }
    if (params.quoteQtyPercent) {
      quoteQtyPercent = parseFloat(params.quoteQtyPercent);
    }
    if (params.minQuoteQty) {
      minQuoteQty = parseFloat(params.minQuoteQty);
    }
  }

  // 计算 EMA 平滑系数
  alphaFast = new Decimal(2).div(new Decimal(emaFastPeriod).plus(1));
  alphaSlow = new Decimal(2).div(new Decimal(emaSlowPeriod).plus(1));

  // console.log("策略参数：", JSON.stringify({
  //   emaFastPeriod: emaFastPeriod,
  //   emaSlowPeriod: emaSlowPeriod,
  //   orderType: orderType,
  //   quoteQtyPercent: quoteQtyPercent,
  //   minQuoteQty: minQuoteQty
  // }));
}

// ========== 信号处理函数 ==========
function onSignal(signal) {
  try {
    // 只处理 K 线信号
    if (signal.type !== "kline") {
      if (signal.type !== "timer") {
        const signalStr = JSON.stringify(signal);
        console.log(`${signal.type} signal:`, signalStr);
      }
      return;
    }

    // 提取交易所和交易对
    if (!signal.exchange || !signal.symbol) {
      console.warn("警告：信号缺少 exchange 或 symbol，跳过处理");
      return;
    }

    var _exchange = signal.exchange;
    var _symbol = signal.symbol;

    var symbol = WithSymbol(_exchange, _symbol);
    if (!symbol) {
      console.warn("警告：无法获取 symbol 对象，跳过处理");
      return;
    }

    // 提取收盘价（signal.close 在 JSON 里通常是字符串）
    var close = new Decimal(signal.close);
    if (!close || close.lte(0)) {
      console.warn("警告：无效的收盘价，跳过处理");
      return;
    }

    // 读取并更新 EMA（按 symbol 独立存储）
    var emaFast = symbol.Get("emaFast");
    var emaSlow = symbol.Get("emaSlow");
    var lastCrossState = symbol.Get("lastCrossState"); // true/false/null/undefined

    if (!emaFast) {
      emaFast = close;
    } else {
      emaFast = emaFast.plus(close.minus(emaFast).mul(alphaFast));
    }

    if (!emaSlow) {
      emaSlow = close;
    } else {
      emaSlow = emaSlow.plus(close.minus(emaSlow).mul(alphaSlow));
    }

    symbol.Set("emaFast", emaFast);
    symbol.Set("emaSlow", emaSlow);
    symbol.Set("currentPrice", close);

    // 判断当前交叉状态
    var currentCrossState = emaFast.gt(emaSlow);

    // 记录时间戳用于日志（signal.ts 通常是 RFC3339 string）
    var timestamp = signal.ts ? new Date(signal.ts).toISOString() : new Date().toISOString();
    // console.log("[" + timestamp + "] EMA快线(" + emaFastPeriod + "): " + emaFast.toFixed(8) +
    //             ", EMA慢线(" + emaSlowPeriod + "): " + emaSlow.toFixed(8) +
    //             ", 收盘价: " + close.toFixed(8));

    // 检测金叉/死叉
    if (lastCrossState === null || typeof lastCrossState === "undefined") {
      // 首次初始化，记录状态但不交易
      symbol.Set("lastCrossState", currentCrossState);
      console.log(
        "[" +
          timestamp +
          "] EMA 初始化完成(" +
          _exchange +
          " " +
          _symbol +
          ")，当前状态: " +
          (currentCrossState ? "金叉" : "死叉")
      );
      return;
    }

    // 检测交叉信号
    if (lastCrossState !== currentCrossState) {
      // 发生交叉
      if (currentCrossState) {
        // 金叉：快线上穿慢线，买入信号
        console.log("[" + timestamp + "] 检测到金叉信号(" + _exchange + " " + _symbol + ")！快线上穿慢线，执行买入");
        handleBuy(symbol, close);
      } else {
        // 死叉：快线下穿慢线，卖出信号
        console.log("[" + timestamp + "] 检测到死叉信号(" + _exchange + " " + _symbol + ")！快线下穿慢线，执行卖出");
        handleSell(symbol, close);
      }
      symbol.Set("lastCrossState", currentCrossState);
    }
  } catch (error) {
    console.error("信号处理异常: " + error);
    console.error(error.stack);
  }
}

// ========== 买入处理函数 ==========
function handleBuy(symbol, currentPrice) {
  try {
    // 检查当前持仓
    var positions = symbol.GetPositions();

    // 如果已有持仓，不重复买入
    if (positions && positions.length > 0) {
      var hasPosition = false;
      for (var i = 0; i < positions.length; i++) {
        var pos = positions[i];
        if (pos.amount && new Decimal(pos.amount).gt(0)) {
          hasPosition = true;
          console.log("已有持仓，跳过买入。持仓数量: " + pos.amount);
          break;
        }
      }
      if (hasPosition) {
        return;
      }
    }

    // 获取余额
    var balance = symbol.GetAsset(symbol.quote);

    console.log('balance', balance);

    if (!balance) {
      console.error("错误：无法获取账户余额");
      return;
    }

    var availableBalance = new Decimal(balance.net);
    if (availableBalance.lte(0)) {
      console.error("错误：账户余额不足");
      return;
    }

    // 计算交易金额
    var quoteQty = availableBalance.mul(quoteQtyPercent);
    if (quoteQty.lt(minQuoteQty)) {
      console.error("错误：交易金额 " + quoteQty.toFixed(2) + " 小于最小金额 " + minQuoteQty);
      return;
    }

    // 构建下单参数（新 API：sym.Buy/Sell 接收 opts；旧 API：order.buy/sell 需要附带 exchange/symbol）
    var opts = {
      type: orderType,
    };

    if (orderType === "market") {
      // 市价单：使用金额
      opts.quoteQty = quoteQty.toFixed(8);
    } else {
      // 限价单：需要价格和数量
      if (currentPrice.lte(0)) {
        console.error("错误：当前价格无效，无法下单");
        return;
      }
      opts.price = currentPrice.toFixed(8);
      var quantity = quoteQty.div(currentPrice);
      opts.quantity = quantity.toFixed(8);
    }

    console.log(
      "执行买入订单:",
      JSON.stringify({ exchange: symbol.exchange, symbol: symbol.symbol, opts: opts })
    );

    // 下单
    var result = symbol.Buy(opts);
    if (result && result.error) {
      console.error("买入订单失败: " + result.error);
    } else {
      console.log("买入订单已提交，订单ID: " + (result ? result.orderId : "N/A"));
    }
  } catch (error) {
    console.error("买入处理异常: " + error);
    console.error(error.stack);
  }
}

// ========== 卖出处理函数 ==========
function handleSell(symbol, currentPrice) {
  try {
    // 检查当前持仓
    var positions = symbol.GetPositions();

    let positionsStr = JSON.stringify(positions);
    console.log("positions", positionsStr);

    if (!positions || positions.length === 0) {
      console.log("无持仓，跳过卖出");
      return;
    }

    // 查找持仓
    var position = null;
    for (var i = 0; i < positions.length; i++) {
      var pos = positions[i];
      if (pos.amount && new Decimal(pos.amount).gt(0)) {
        position = pos;
        break;
      }
    }

    if (!position) {
      console.log("无持仓，跳过卖出");
      return;
    }

    var positionQty = new Decimal(position.amount);
    if (positionQty.lte(0)) {
      console.log("持仓数量为 0，跳过卖出");
      return;
    }

    // 构建订单参数
    var opts = {
      type: orderType,
    };

    if (orderType === "market") {
      // 市价单：卖出全部持仓
      opts.quantity = positionQty.toFixed(8);
    } else {
      // 限价单：需要价格
      if (currentPrice.lte(0)) {
        console.error("错误：当前价格无效，无法下单");
        return;
      }
      opts.price = currentPrice.toFixed(8);
      opts.quantity = positionQty.toFixed(8);
    }

    console.log(
      "执行卖出订单:",
      JSON.stringify({ exchange: symbol.exchange, symbol: symbol.symbol, opts: opts })
    );

    // 下单
    var result = symbol.Sell(opts);
    if (result && result.error) {
      console.error("卖出订单失败: " + result.error);
    } else {
      console.log("卖出订单已提交，订单ID: " + (result ? result.orderId : "N/A"));
    }
  } catch (error) {
    console.error("卖出处理异常: " + error);
  }
}
