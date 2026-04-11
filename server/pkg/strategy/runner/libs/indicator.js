'use strict';

Object.defineProperty(exports, '__esModule', { value: true });

var ta = require('ta');

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

function extractOHLC(source) {
    return {
        high: extractField(source, 'high'),
        low: extractField(source, 'low'),
        close: extractField(source, 'close')
    };
}

function extractOHLCV(source) {
    var ohlc = extractOHLC(source);
    ohlc.volume = extractField(source, 'volume');
    return ohlc;
}

function indicator() {}

indicator.sma = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period) {
        return [];
    }
    return ta.sma({ period: input.period, values: values });
};

indicator.ema = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period) {
        return [];
    }
    return ta.ema({ period: input.period, values: values });
};

indicator.wma = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period) {
        return [];
    }
    return ta.wma({ period: input.period, values: values });
};

indicator.wema = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period) {
        return [];
    }
    return ta.wema({ period: input.period, values: values });
};

indicator.rsi = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period + 1) {
        return [];
    }
    return ta.rsi({ period: input.period, values: values });
};

indicator.roc = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period + 1) {
        return [];
    }
    return ta.roc({ period: input.period, values: values });
};

indicator.trix = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period * 2) {
        return [];
    }
    return ta.trix({ period: input.period, values: values });
};

indicator.macd = function(input) {
    var values = extractField(input.source, 'close');
    var fastPeriod = input.fastPeriod || 12;
    var slowPeriod = input.slowPeriod || 26;
    var signalPeriod = input.signalPeriod || 9;
    if (values.length < slowPeriod) {
        return { MACD: [], Signal: [], Histogram: [] };
    }
    return ta.macd({
        values: values,
        fastPeriod: fastPeriod,
        slowPeriod: slowPeriod,
        signalPeriod: signalPeriod,
        SimpleMAOscillator: input.SimpleMAOscillator || false,
        SimpleMASignal: input.SimpleMASignal || false
    });
};

indicator.kst = function(input) {
    var values = extractField(input.source, 'close');
    var rocMa1Period = input.rocMa1Period || 10;
    var rocMa2Period = input.rocMa2Period || 15;
    var rocMa3Period = input.rocMa3Period || 20;
    var rocMa4Period = input.rocMa4Period || 30;
    var signalPeriod = input.signalPeriod || 9;
    if (values.length < rocMa4Period + signalPeriod) {
        return [];
    }
    return ta.kst({
        values: values,
        ROCPer1: rocMa1Period,
        ROCPer2: rocMa2Period,
        ROCPer3: rocMa3Period,
        ROCPer4: rocMa4Period,
        SMAROCPer1: rocMa1Period,
        SMAROCPer2: rocMa2Period,
        SMAROCPer3: rocMa3Period,
        SMAROCPer4: rocMa4Period,
        signalPeriod: signalPeriod
    });
};

indicator.stochasticrsi = function(input) {
    var values = extractField(input.source, 'close');
    var period = input.period || 14;
    var rsiPeriod = input.rsiPeriod || 14;
    var stochasticPeriod = input.stochasticPeriod || 14;
    var signalPeriod = input.signalPeriod || 3;
    if (values.length < rsiPeriod + stochasticPeriod) {
        return { k: [], d: [] };
    }
    return ta.stochasticrsi({
        values: values,
        period: period,
        rsiPeriod: rsiPeriod,
        stochasticPeriod: stochasticPeriod,
        signalPeriod: signalPeriod
    });
};

indicator.atr = function(input) {
    var ohlc = extractOHLC(input.source);
    if (ohlc.high.length < input.period + 1) {
        return [];
    }
    return ta.atr({
        period: input.period,
        high: ohlc.high,
        low: ohlc.low,
        close: ohlc.close
    });
};

indicator.adx = function(input) {
    var ohlc = extractOHLC(input.source);
    if (ohlc.high.length < input.period + 1) {
        return [];
    }
    return ta.adx({
        period: input.period,
        high: ohlc.high,
        low: ohlc.low,
        close: ohlc.close
    });
};

indicator.truerange = function(input) {
    var ohlc = extractOHLC(input.source);
    if (ohlc.high.length < 2) {
        return [];
    }
    return ta.truerange({
        high: ohlc.high,
        low: ohlc.low,
        close: ohlc.close
    });
};

indicator.stochastic = function(input) {
    var ohlc = extractOHLC(input.source);
    var period = input.period || 14;
    var signalPeriod = input.signalPeriod || 3;
    if (ohlc.high.length < period) {
        return { k: [], d: [] };
    }
    return ta.stochastic({
        high: ohlc.high,
        low: ohlc.low,
        close: ohlc.close,
        period: period,
        signalPeriod: signalPeriod
    });
};

indicator.williamsr = function(input) {
    var ohlc = extractOHLC(input.source);
    if (ohlc.high.length < input.period + 1) {
        return [];
    }
    return ta.williamsr({
        period: input.period,
        high: ohlc.high,
        low: ohlc.low,
        close: ohlc.close
    });
};

indicator.cci = function(input) {
    var ohlc = extractOHLC(input.source);
    if (ohlc.high.length < input.period) {
        return [];
    }
    return ta.cci({
        period: input.period,
        high: ohlc.high,
        low: ohlc.low,
        close: ohlc.close
    });
};

indicator.mfi = function(input) {
    var ohlcv = extractOHLCV(input.source);
    if (ohlcv.high.length < input.period + 1) {
        return [];
    }
    return ta.mfi({
        period: input.period,
        high: ohlcv.high,
        low: ohlcv.low,
        close: ohlcv.close,
        volume: ohlcv.volume
    });
};

indicator.bb = function(input) {
    var values = extractField(input.source, 'close');
    var period = input.period || 20;
    var stdDev = input.stdDev || 2;
    if (values.length < period) {
        return { upper: [], middle: [], lower: [] };
    }
    return ta.bollingerbands({
        period: period,
        values: values,
        stdDev: stdDev
    });
};

indicator.bollingerbands = indicator.bb;

indicator.vwap = function(input) {
    var ohlcv = extractOHLCV(input.source);
    if (ohlcv.high.length === 0) {
        return [];
    }
    return ta.vwap({
        high: ohlcv.high,
        low: ohlcv.low,
        close: ohlcv.close,
        open: ohlcv.high,
        volume: ohlcv.volume
    });
};

indicator.obv = function(input) {
    var ohlc = extractOHLC(input.source);
    var volume = extractField(input.source, 'volume');
    if (ohlc.close.length === 0) {
        return [];
    }
    return ta.obv({
        close: ohlc.close,
        volume: volume
    });
};

indicator.adl = function(input) {
    var ohlc = extractOHLC(input.source);
    var volume = extractField(input.source, 'volume');
    if (ohlc.close.length === 0) {
        return [];
    }
    return ta.adl({
        high: ohlc.high,
        low: ohlc.low,
        close: ohlc.close,
        volume: volume
    });
};

indicator.forceindex = function(input) {
    var ohlc = extractOHLC(input.source);
    var volume = extractField(input.source, 'volume');
    if (ohlc.close.length < (input.period || 13) + 1) {
        return [];
    }
    return ta.forceindex({
        close: ohlc.close,
        volume: volume,
        period: input.period || 13
    });
};

indicator.awesomeoscillator = function(input) {
    var ohlc = extractOHLC(input.source);
    if (ohlc.high.length < 34) {
        return [];
    }
    return ta.awesomeoscillator({
        high: ohlc.high,
        low: ohlc.low
    });
};

indicator.psar = function(input) {
    var ohlc = extractOHLC(input.source);
    if (ohlc.high.length < 2) {
        return [];
    }
    var accelerationFactor = input.accelerationFactor || 0.02;
    var maxAccelerationFactor = input.maxAccelerationFactor || 0.2;
    return ta.psar({
        high: ohlc.high,
        low: ohlc.low,
        accelerationFactor: accelerationFactor,
        maxAccelerationFactor: maxAccelerationFactor
    });
};

indicator.highest = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period) {
        return [];
    }
    return ta.highest({ values: values, period: input.period });
};

indicator.lowest = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period) {
        return [];
    }
    return ta.lowest({ values: values, period: input.period });
};

indicator.sum = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period) {
        return [];
    }
    return ta.sum({ values: values, period: input.period });
};

indicator.sd = function(input) {
    var values = extractField(input.source, 'close');
    if (values.length < input.period) {
        return [];
    }
    return ta.sd({ values: values, period: input.period });
};

indicator.crossUp = function(input) {
    var lineA = input.lineA || [];
    var lineB = input.lineB || [];
    return ta.crossUp({ lineA: lineA, lineB: lineB });
};

indicator.crossDown = function(input) {
    var lineA = input.lineA || [];
    var lineB = input.lineB || [];
    return ta.crossDown({ lineA: lineA, lineB: lineB });
};

indicator.crossover = function(input) {
    var lineA = input.lineA || [];
    var lineB = input.lineB || [];
    return ta.crossover({ lineA: lineA, lineB: lineB });
};

indicator.keltnerchannels = function(input) {
    var ohlc = extractOHLC(input.source);
    var period = input.period || 20;
    var multiplier = input.multiplier || 2;
    if (ohlc.high.length < period) {
        return { upper: [], middle: [], lower: [] };
    }
    return ta.keltnerchannels({
        period: period,
        multiplier: multiplier,
        high: ohlc.high,
        low: ohlc.low,
        close: ohlc.close
    });
};

indicator.donchian = function(input) {
    var values = extractField(input.source, 'close');
    var period = input.period || 20;
    if (values.length < period) {
        return { upper: [], middle: [], lower: [] };
    }
    return ta.donchian({ period: period, values: values });
};

indicator.getAvailableIndicators = function() {
    return Object.keys(indicator).filter(function(key) {
        return typeof indicator[key] === 'function' && key !== 'getAvailableIndicators';
    });
};

exports.default = indicator;
exports.indicator = indicator;
