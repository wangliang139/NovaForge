package types

import (
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func Test_Signal(t *testing.T) {
	signal := &TickerSignal{
		BaseSignal: BaseSignal{
			ID:       "123",
			Exchange: lo.ToPtr(ctypes.ExchangeBinance),
			Ts:       time.Now(),
		},
		LastPrice:     decimal.NewFromInt(10000),
		Open24:        decimal.NewFromInt(10000),
		High24:        decimal.NewFromInt(10000),
		Low24:         decimal.NewFromInt(10000),
		Avg24:         decimal.NewFromInt(10000),
		Volume24:      decimal.NewFromInt(10000),
		QuoteVolume24: decimal.NewFromInt(10000),
	}

	json, err := jsoniter.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(json))

	message := NewMessage(signal, false)
	json, err = jsoniter.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(json))
}
