package registry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestBuildStreamSelectorsAccountSignalDoesNotRequireSymbol(t *testing.T) {
	exchange := ctypes.ExchangeBinance
	bot := &stypes.Bot{
		Exchange:  exchange,
		AccountID: "paper-account",
		Config: stypes.BotConfig{
			Signals: []stypes.SignalBinding{
				{
					SignalID: "balance",
					Exchange: &exchange,
				},
			},
		},
	}
	strategy := &stypes.Strategy{
		Signals: []stypes.SignalDefinition{
			{
				ID:   "balance",
				Type: ctypes.SignalTypeBalance,
			},
		},
	}

	specs, err := (&ExecutorRegistry{}).buildStreamSelectors(context.Background(), bot, strategy)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	require.Equal(t, exchange, specs[0].exchange)
	require.Equal(t, ctypes.StreamTypeAccount, specs[0].selector.Stream)
	require.Equal(t, "paper-account", *specs[0].selector.Account)
	require.Nil(t, specs[0].selector.Symbol)
}

func TestBuildStreamSelectorsMarketSignalStillRequiresSymbol(t *testing.T) {
	exchange := ctypes.ExchangeBinance
	bot := &stypes.Bot{
		Exchange: exchange,
		Config: stypes.BotConfig{
			Signals: []stypes.SignalBinding{
				{
					SignalID: "ticker",
					Exchange: &exchange,
				},
			},
		},
	}
	strategy := &stypes.Strategy{
		Signals: []stypes.SignalDefinition{
			{
				ID:   "ticker",
				Type: ctypes.SignalTypeTicker,
			},
		},
	}

	specs, err := (&ExecutorRegistry{}).buildStreamSelectors(context.Background(), bot, strategy)
	require.NoError(t, err)
	require.Empty(t, specs)
}
