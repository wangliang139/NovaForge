package strategy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/executor/backtest"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

func Test_backtest(t *testing.T) {
	strategyId := "123"
	strategyVer := "1.0.0"
	strategy := &stypes.Strategy{
		ID:      strategyId,
		Version: strategyVer,
		Code: `
function onInit() {
  console.log("A");
  console.log("B");
  console.log("C");
}

function onSignal(signal) {}
`,
	}

	btCtx := stypes.BacktestContext{
		ID:          strategyId,
		StrategyID:  strategyId,
		StrategyVer: strategyVer,
	}

	params := map[string]any{}
	startTime := time.Now()
	endTime := time.Now()

	symbols := []*stypes.BacktestSymbol{
		{
			Exchange: "binance",
			Symbol: ctypes.Symbol{
				Base:  "BTC",
				Quote: "USDT",
				Type:  ctypes.MarketTypeSpot,
			},
			BaseAssetQty:  "0",
			QuoteAssetQty: "10000",
		},
	}

	config := stypes.BacktestConfig{
		StartTime: startTime,
		EndTime:   endTime,
		Symbols:   symbols,
		Sources:   nil,
		Params:    params,
	}

	ctx := context.Background()
	// 仅保留最后 2 条日志，验证控制台日志缓存裁剪逻辑
	executor, err := backtest.NewBacktestExecutor(strategy, btCtx, config, backtest.WithConsoleLogMaxCache(2))
	if err != nil {
		t.Fatal(err)
	}
	done, err := executor.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx := context.WithoutCancel(ctx)
		_ = executor.Stop(ctx)
	}()

	select {
	case <-done:
		result, err := executor.GetResult()
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || result.Data == nil {
			t.Fatalf("expected result.data, got %#v", result)
		}
		if got := len(result.Data.ConsoleLogs); got != 2 {
			t.Fatalf("expected 2 console logs (max cache), got %d: %#v", got, result.Data.ConsoleLogs)
		}
		if result.Data.ConsoleLogs[0].Message != "B" || result.Data.ConsoleLogs[1].Message != "C" {
			t.Fatalf("unexpected console logs: %#v", result.Data.ConsoleLogs)
		}
		if result.InitialBalance != "10000" {
			t.Fatalf("expected initial balance 10000, got %s", result.InitialBalance)
		}
		if result.FinalBalance != "10000" {
			t.Fatalf("expected final balance 10000, got %s", result.FinalBalance)
		}
		break
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

// Test_backtest_symbol_account_isolation 测试策略级交易对资金隔离功能
// 验证：
// 1. 两个交易对同 quote（USDT），初始净值不会被 double
// 2. 每个交易对的资金额度独立，不能互相挪用
func Test_backtest_symbol_account_isolation(t *testing.T) {
	strategyId := "test-symbol-isolation"
	strategyVer := "1.0.0"
	strategy := &stypes.Strategy{
		ID:      strategyId,
		Version: strategyVer,
		Code: `
function onInit() {
  console.log("Strategy initialized with symbol account isolation");
}

function onSignal(signal) {}
`,
	}

	btCtx := stypes.BacktestContext{
		ID:          strategyId,
		StrategyID:  strategyId,
		StrategyVer: strategyVer,
	}

	params := map[string]any{}
	startTime := time.Now()
	endTime := time.Now()

	// 两个交易对，同是 USDT 计价，各自初始 1000 USDT
	symbols := []*stypes.BacktestSymbol{
		{
			Exchange: "binance",
			Symbol: ctypes.Symbol{
				Base:  "BTC",
				Quote: "USDT",
				Type:  ctypes.MarketTypeSpot,
			},
			BaseAssetQty:  "0",
			QuoteAssetQty: "1000",
		},
		{
			Exchange: "binance",
			Symbol: ctypes.Symbol{
				Base:  "ETH",
				Quote: "USDT",
				Type:  ctypes.MarketTypeSpot,
			},
			BaseAssetQty:  "0",
			QuoteAssetQty: "1000",
		},
	}

	config := stypes.BacktestConfig{
		StartTime:    startTime,
		EndTime:      endTime,
		Symbols:      symbols,
		Sources:      nil,
		Params:       params,
		BaseCurrency: "USDT",
		BaseExchange: ctypes.ExchangeBinance,
	}

	ctx := context.Background()
	executor, err := backtest.NewBacktestExecutor(strategy, btCtx, config)
	if err != nil {
		t.Fatal(err)
	}
	done, err := executor.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx := context.WithoutCancel(ctx)
		_ = executor.Stop(ctx)
	}()

	select {
	case <-done:
		// 验证 symbolAccountMgr 的余额（每个交易对独立）
		symbolAccountMgr := executor.GetSymbolAccountManager()
		btcUsdt := ctypes.NewExSymbol("binance", ctypes.Symbol{Base: "BTC", Quote: "USDT", Type: ctypes.MarketTypeSpot})
		ethUsdt := ctypes.NewExSymbol("binance", ctypes.Symbol{Base: "ETH", Quote: "USDT", Type: ctypes.MarketTypeSpot})

		// 验证每个交易对的 USDT 余额分别是 1000
		btcFree, btcFrozen := symbolAccountMgr.GetBalance(btcUsdt, "USDT")
		btcTotal := btcFree.Add(btcFrozen)
		expectedPerSymbol := decimal.NewFromInt(1000)
		if !btcTotal.Equal(expectedPerSymbol) {
			t.Fatalf("expected BTC/USDT symbol account balance %s, got %s", expectedPerSymbol.String(), btcTotal.String())
		}

		ethFree, ethFrozen := symbolAccountMgr.GetBalance(ethUsdt, "USDT")
		ethTotal := ethFree.Add(ethFrozen)
		if !ethTotal.Equal(expectedPerSymbol) {
			t.Fatalf("expected ETH/USDT symbol account balance %s, got %s", expectedPerSymbol.String(), ethTotal.String())
		}

		// 验证总余额是 2000（不是 4000）
		totalBalance := btcTotal.Add(ethTotal)
		expectedTotal := decimal.NewFromInt(2000)
		if !totalBalance.Equal(expectedTotal) {
			t.Fatalf("expected total balance %s, got %s", expectedTotal.String(), totalBalance.String())
		}

		t.Logf("✓ Symbol account isolation verified:")
		t.Logf("  - BTC/USDT has %s USDT", btcTotal.String())
		t.Logf("  - ETH/USDT has %s USDT", ethTotal.String())
		t.Logf("  - Total: %s USDT (not doubled)", totalBalance.String())

	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

// Test_backtest_symbol_account_assert_available 测试 symbolAccount 的 AssertAvailable 方法
// 验证：当可用余额不足时会返回错误
func Test_backtest_symbol_account_assert_available(t *testing.T) {
	strategyId := "test-assert"
	strategyVer := "1.0.0"
	strategy := &stypes.Strategy{
		ID:      strategyId,
		Version: strategyVer,
		Code: `
function onInit() {
  console.log("Test assert available");
}
function onSignal(signal) {}
`,
	}

	btCtx := stypes.BacktestContext{
		ID:          strategyId,
		StrategyID:  strategyId,
		StrategyVer: strategyVer,
	}

	params := map[string]any{}
	startTime := time.Now()
	endTime := time.Now()

	// 一个交易对，初始 1000 USDT
	symbols := []*stypes.BacktestSymbol{
		{
			Exchange: "binance",
			Symbol: ctypes.Symbol{
				Base:  "BTC",
				Quote: "USDT",
				Type:  ctypes.MarketTypeSpot,
			},
			BaseAssetQty:  "0",
			QuoteAssetQty: "1000",
		},
	}

	config := stypes.BacktestConfig{
		StartTime:    startTime,
		EndTime:      endTime,
		Symbols:      symbols,
		Sources:      nil,
		Params:       params,
		BaseCurrency: "USDT",
		BaseExchange: ctypes.ExchangeBinance,
	}

	ctx := context.Background()
	executor, err := backtest.NewBacktestExecutor(strategy, btCtx, config)
	if err != nil {
		t.Fatal(err)
	}
	done, err := executor.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx := context.WithoutCancel(ctx)
		_ = executor.Stop(ctx)
	}()

	select {
	case <-done:
		symbolAccountMgr := executor.GetSymbolAccountManager()
		btcUsdt := ctypes.NewExSymbol("binance", ctypes.Symbol{Base: "BTC", Quote: "USDT", Type: ctypes.MarketTypeSpot})

		// 验证可以通过的额度检查（900 < 1000）
		err := symbolAccountMgr.AssertAvailable(btcUsdt, "USDT", decimal.NewFromInt(900))
		if err != nil {
			t.Fatalf("expected no error for 900 USDT, got: %v", err)
		}

		// 验证会被拒绝的额度检查（1500 > 1000）
		err = symbolAccountMgr.AssertAvailable(btcUsdt, "USDT", decimal.NewFromInt(1500))
		if err == nil {
			t.Fatal("expected error for 1500 USDT, got nil")
		}
		expectedErrMsg := "insufficient symbol account balance"
		if !strings.Contains(err.Error(), expectedErrMsg) {
			t.Fatalf("expected error containing %q, got: %v", expectedErrMsg, err)
		}

		t.Logf("✓ Symbol account AssertAvailable works correctly")
		t.Logf("  - 900 USDT check passed")
		t.Logf("  - 1500 USDT check failed as expected: %v", err)

	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
