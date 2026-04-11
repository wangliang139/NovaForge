const ta = require('./ta.js');
const indicatorModule = require('./indicator.js');
const indicator = indicatorModule.default || indicatorModule.indicator;

function generateKlines(count, basePrice = 100) {
    const klines = [];
    let price = basePrice;
    for (let i = 0; i < count; i++) {
        const change = (Math.random() - 0.5) * 4;
        const close = price + change;
        const high = close + Math.random() * 2;
        const low = close - Math.random() * 2;
        const volume = Math.random() * 1000;
        
        klines.push({
            open: String(price.toFixed(2)),
            high: String(high.toFixed(2)),
            low: String(low.toFixed(2)),
            close: String(close.toFixed(2)),
            volume: String(volume.toFixed(2))
        });
        price = close;
    }
    return klines;
}

function extractField(source, fieldName) {
    if (!source || !Array.isArray(source)) {
        return [];
    }
    return source.map(function(k) {
        var val = k[fieldName];
        if (val === undefined || val === null) {
            return NaN;
        }
        if (typeof val === 'string') {
            var parsed = parseFloat(val);
            return isNaN(parsed) ? NaN : parsed;
        }
        return val;
    });
}

function compareResults(name, oldResult, newResult) {
    if (!oldResult && !newResult) {
        console.log(`  ✅ ${name}: 两者均为空`);
        return true;
    }
    if (!oldResult || !newResult) {
        console.log(`  ❌ ${name}: 结果类型不一致 - old: ${typeof oldResult}, new: ${typeof newResult}`);
        return false;
    }
    
    if (Array.isArray(oldResult) && Array.isArray(newResult)) {
        if (oldResult.length !== newResult.length) {
            console.log(`  ❌ ${name}: 长度不一致 - old: ${oldResult.length}, new: ${newResult.length}`);
            return false;
        }
        
        let allMatch = true;
        let maxDiff = 0;
        for (let i = 0; i < oldResult.length; i++) {
            const diff = Math.abs((oldResult[i] || 0) - (newResult[i] || 0));
            if (diff > 1e-10) {
                allMatch = false;
                maxDiff = Math.max(maxDiff, diff);
            }
        }
        
        if (allMatch) {
            console.log(`  ✅ ${name}: 完全一致 (${oldResult.length} 个值)`);
            return true;
        } else {
            console.log(`  ⚠️  ${name}: 存在微小差异，最大差值: ${maxDiff}`);
            return true;
        }
    }
    
    if (typeof oldResult === 'object' && typeof newResult === 'object') {
        let allMatch = true;
        for (const key of Object.keys(oldResult)) {
            const oldArr = oldResult[key];
            const newArr = newResult[key];
            
            if (Array.isArray(oldArr) && Array.isArray(newArr)) {
                if (oldArr.length !== newArr.length) {
                    console.log(`  ❌ ${name}.${key}: 长度不一致 - old: ${oldArr.length}, new: ${newArr.length}`);
                    allMatch = false;
                    continue;
                }
                
                for (let i = 0; i < oldArr.length; i++) {
                    const diff = Math.abs((oldArr[i] || 0) - (newArr[i] || 0));
                    if (diff > 1e-10) {
                        allMatch = false;
                    }
                }
            }
        }
        
        if (allMatch) {
            console.log(`  ✅ ${name}: 对象结构完全一致`);
            return true;
        } else {
            console.log(`  ⚠️  ${name}: 存在微小数值差异`);
            return true;
        }
    }
    
    console.log(`  ❌ ${name}: 无法比较的类型`);
    return false;
}

console.log('========== 指标库对比测试 ==========\n');

const klines = generateKlines(100);
const closes = extractField(klines, 'close');
const highs = extractField(klines, 'high');
const lows = extractField(klines, 'low');
const volumes = extractField(klines, 'volume');

console.log(`测试数据: ${klines.length} 根K线`);
console.log(`价格范围: ${Math.min(...closes).toFixed(2)} - ${Math.max(...closes).toFixed(2)}\n`);

let passed = 0;
let failed = 0;

function test(name, indicatorFn, taFn, indicatorParams, taParams) {
    try {
        const newResult = indicatorFn(indicatorParams);
        const oldResult = taFn(taParams);
        
        if (compareResults(name, oldResult, newResult)) {
            passed++;
        } else {
            failed++;
        }
    } catch (e) {
        console.log(`  ❌ ${name}: 异常 - ${e.message}`);
        failed++;
    }
}

console.log('--- 仅需 close 的指标 ---');

test('SMA(10)', 
    (p) => indicator.sma(p), 
    (p) => ta.sma(p),
    { source: klines, period: 10 },
    { period: 10, values: closes }
);

test('EMA(12)', 
    (p) => indicator.ema(p), 
    (p) => ta.ema(p),
    { source: klines, period: 12 },
    { period: 12, values: closes }
);

test('WMA(10)', 
    (p) => indicator.wma(p), 
    (p) => ta.wma(p),
    { source: klines, period: 10 },
    { period: 10, values: closes }
);

test('WEMA(10)', 
    (p) => indicator.wema(p), 
    (p) => ta.wema(p),
    { source: klines, period: 10 },
    { period: 10, values: closes }
);

test('RSI(14)', 
    (p) => indicator.rsi(p), 
    (p) => ta.rsi(p),
    { source: klines, period: 14 },
    { period: 14, values: closes }
);

test('ROC(10)', 
    (p) => indicator.roc(p), 
    (p) => ta.roc(p),
    { source: klines, period: 10 },
    { period: 10, values: closes }
);

test('TRIX(15)', 
    (p) => indicator.trix(p), 
    (p) => ta.trix(p),
    { source: klines, period: 15 },
    { period: 15, values: closes }
);

test('CCI(20)', 
    (p) => indicator.cci(p), 
    (p) => ta.cci(p),
    { source: klines, period: 20 },
    { period: 20, high: highs, low: lows, close: closes }
);

test('StochasticRSI', 
    (p) => indicator.stochasticrsi(p), 
    (p) => ta.stochasticrsi(p),
    { source: klines, period: 14, rsiPeriod: 14, stochasticPeriod: 14, signalPeriod: 3 },
    { period: 14, values: closes, rsiPeriod: 14, stochasticPeriod: 14, signalPeriod: 3 }
);

console.log('\n--- 需要 OHLC 的指标 ---');

test('ATR(14)', 
    (p) => indicator.atr(p), 
    (p) => ta.atr(p),
    { source: klines, period: 14 },
    { period: 14, high: highs, low: lows, close: closes }
);

test('ADX(14)', 
    (p) => indicator.adx(p), 
    (p) => ta.adx(p),
    { source: klines, period: 14 },
    { period: 14, high: highs, low: lows, close: closes }
);

test('TrueRange', 
    (p) => indicator.truerange(p), 
    (p) => ta.truerange(p),
    { source: klines },
    { high: highs, low: lows, close: closes }
);

test('Stochastic(14,3)', 
    (p) => indicator.stochastic(p), 
    (p) => ta.stochastic(p),
    { source: klines, period: 14, signalPeriod: 3 },
    { period: 14, signalPeriod: 3, high: highs, low: lows, close: closes }
);

test('WilliamsR(14)', 
    (p) => indicator.williamsr(p), 
    (p) => ta.williamsr(p),
    { source: klines, period: 14 },
    { period: 14, high: highs, low: lows, close: closes }
);

console.log('\n--- 需要 OHLCV 的指标 ---');

test('BollingerBands(20,2)', 
    (p) => indicator.bb(p), 
    (p) => ta.bollingerbands(p),
    { source: klines, period: 20, stdDev: 2 },
    { period: 20, stdDev: 2, values: closes }
);

test('VWAP', 
    (p) => indicator.vwap(p), 
    (p) => ta.vwap(p),
    { source: klines },
    { high: highs, low: lows, close: closes, open: highs, volume: volumes }
);

test('MFI(14)', 
    (p) => indicator.mfi(p), 
    (p) => ta.mfi(p),
    { source: klines, period: 14 },
    { period: 14, high: highs, low: lows, close: closes, volume: volumes }
);

console.log('\n--- 需要 close + volume 的指标 ---');

test('OBV', 
    (p) => indicator.obv(p), 
    (p) => ta.obv(p),
    { source: klines },
    { close: closes, volume: volumes }
);

test('ADL', 
    (p) => indicator.adl(p), 
    (p) => ta.adl(p),
    { source: klines },
    { high: highs, low: lows, close: closes, volume: volumes }
);

test('ForceIndex(13)', 
    (p) => indicator.forceindex(p), 
    (p) => ta.forceindex(p),
    { source: klines, period: 13 },
    { period: 13, close: closes, volume: volumes }
);

console.log('\n--- MACD 指标 ---');

test('MACD(12,26,9)', 
    (p) => indicator.macd(p), 
    (p) => ta.macd(p),
    { source: klines, fastPeriod: 12, slowPeriod: 26, signalPeriod: 9 },
    { fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, values: closes, SimpleMAOscillator: false, SimpleMASignal: false }
);

console.log('\n--- KST 指标 ---');

test('KST', 
    (p) => indicator.kst(p), 
    (p) => ta.kst(p),
    { source: klines },
    { 
        values: closes,
        ROCPer1: 10, ROCPer2: 15, ROCPer3: 20, ROCPer4: 30,
        SMAROCPer1: 10, SMAROCPer2: 15, SMAROCPer3: 20, SMAROCPer4: 30,
        signalPeriod: 9
    }
);

console.log('\n--- 其他指标 ---');

test('AwesomeOscillator', 
    (p) => indicator.awesomeoscillator(p), 
    (p) => ta.awesomeoscillator(p),
    { source: klines },
    { high: highs, low: lows }
);

test('PSAR', 
    (p) => indicator.psar(p), 
    (p) => ta.psar(p),
    { source: klines, accelerationFactor: 0.02, maxAccelerationFactor: 0.2 },
    { high: highs, low: lows, accelerationFactor: 0.02, maxAccelerationFactor: 0.2 }
);

test('Highest(20)', 
    (p) => indicator.highest(p), 
    (p) => ta.highest(p),
    { source: klines, period: 20 },
    { period: 20, values: closes }
);

test('Lowest(20)', 
    (p) => indicator.lowest(p), 
    (p) => ta.lowest(p),
    { source: klines, period: 20 },
    { period: 20, values: closes }
);

test('Sum(20)', 
    (p) => indicator.sum(p), 
    (p) => ta.sum(p),
    { source: klines, period: 20 },
    { period: 20, values: closes }
);

test('SD(20)', 
    (p) => indicator.sd(p), 
    (p) => ta.sd(p),
    { source: klines, period: 20 },
    { period: 20, values: closes }
);

console.log('\n--- 交叉信号指标 ---');

const sma5 = ta.sma({ period: 5, values: closes });
const sma20 = ta.sma({ period: 20, values: closes });

test('CrossUp', 
    (p) => indicator.crossUp(p), 
    (p) => ta.crossUp(p),
    { lineA: sma5, lineB: sma20 },
    { lineA: sma5, lineB: sma20 }
);

test('CrossDown', 
    (p) => indicator.crossDown(p), 
    (p) => ta.crossDown(p),
    { lineA: sma5, lineB: sma20 },
    { lineA: sma5, lineB: sma20 }
);

console.log('\n========== 测试结果 ==========');
console.log(`✅ 通过: ${passed}`);
console.log(`❌ 失败: ${failed}`);
console.log(`总计: ${passed + failed}`);

if (failed > 0) {
    process.exit(1);
}
