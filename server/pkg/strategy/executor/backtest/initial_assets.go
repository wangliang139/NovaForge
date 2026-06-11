package backtest

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func walletTypeMatches(exchange ctypes.Exchange, symType ctypes.MarketType, wt ctypes.WalletType) bool {
	return ctypes.GetWalletType(exchange, symType) == wt
}

// pickCarrierSymbol 为初始资产入账挑选一个代表交易对（决定 ledger 的 marketType）。
func pickCarrierSymbol(
	exchange ctypes.Exchange,
	walletType ctypes.WalletType,
	asset string,
	symbols []*stypes.BacktestSymbol,
) (ctypes.Symbol, bool) {
	asset = ctypes.ParseAssetCode(asset)
	for _, symCfg := range symbols {
		if symCfg == nil || symCfg.Exchange != exchange {
			continue
		}
		sym := symCfg.Symbol
		if !walletTypeMatches(exchange, sym.Type, walletType) {
			continue
		}
		if asset == ctypes.ParseAssetCode(sym.Base) || asset == ctypes.ParseAssetCode(sym.Quote) {
			return sym, true
		}
	}
	for _, symCfg := range symbols {
		if symCfg == nil || symCfg.Exchange != exchange {
			continue
		}
		sym := symCfg.Symbol
		if walletTypeMatches(exchange, sym.Type, walletType) {
			return sym, true
		}
	}
	return ctypes.Symbol{}, false
}

// ValidateInitialAssetsForSymbols 校验初始资产与交易对配置是否匹配（与 Bot 创建页语义一致）。
func ValidateInitialAssetsForSymbols(
	exchange ctypes.Exchange,
	symbols []*stypes.BacktestSymbol,
	assets []ctypes.AssetInput,
) error {
	if exchange == "" {
		return fmt.Errorf("backtest exchange is required")
	}
	if len(symbols) == 0 {
		return fmt.Errorf("backtest symbols is required")
	}
	if len(assets) == 0 {
		return fmt.Errorf("backtest initialAssets is required")
	}

	assetsByWallet := make(map[ctypes.WalletType]map[string]struct{})
	for _, item := range assets {
		code := ctypes.ParseAssetCode(item.Asset)
		if code == "" {
			return fmt.Errorf("initial asset code is required")
		}
		if !item.WalletType.Valid() {
			return fmt.Errorf("invalid wallet type for asset %s", code)
		}
		total, err := decimal.NewFromString(strings.TrimSpace(item.Total))
		if err != nil || total.IsNegative() {
			return fmt.Errorf("invalid total for asset %s", code)
		}
		frozen := decimal.Zero
		if strings.TrimSpace(item.Frozen) != "" {
			frozen, err = decimal.NewFromString(strings.TrimSpace(item.Frozen))
			if err != nil || frozen.IsNegative() {
				return fmt.Errorf("invalid frozen for asset %s", code)
			}
		}
		if frozen.GreaterThan(total) {
			return fmt.Errorf("frozen must be <= total for asset %s", code)
		}
		if assetsByWallet[item.WalletType] == nil {
			assetsByWallet[item.WalletType] = make(map[string]struct{})
		}
		key := code
		if _, ok := assetsByWallet[item.WalletType][key]; ok {
			return fmt.Errorf("duplicated initial asset %s in wallet %s", code, item.WalletType)
		}
		assetsByWallet[item.WalletType][key] = struct{}{}
	}

	for _, symCfg := range symbols {
		if symCfg == nil {
			continue
		}
		if symCfg.Exchange != exchange {
			return fmt.Errorf("symbol exchange %s does not match backtest exchange %s", symCfg.Exchange, exchange)
		}
		sym := symCfg.Symbol
		wt := ctypes.GetWalletType(exchange, sym.Type)
		assetSet := assetsByWallet[wt]
		if assetSet == nil {
			return fmt.Errorf("missing initial assets for wallet %s required by %s", wt, sym.String())
		}
		switch sym.Type {
		case ctypes.MarketTypeFuture:
			if _, ok := assetSet[ctypes.ParseAssetCode(sym.Quote)]; !ok {
				return fmt.Errorf("missing quote asset %s for future symbol %s", sym.Quote, sym.String())
			}
		case ctypes.MarketTypeSpot:
			base := ctypes.ParseAssetCode(sym.Base)
			quote := ctypes.ParseAssetCode(sym.Quote)
			if _, hasBase := assetSet[base]; !hasBase {
				if _, hasQuote := assetSet[quote]; !hasQuote {
					return fmt.Errorf("spot symbol %s requires base %s or quote %s in wallet %s", sym.String(), base, quote, wt)
				}
			}
		default:
			return fmt.Errorf("unsupported market type for symbol %s", sym.String())
		}
	}

	for _, item := range assets {
		if _, ok := pickCarrierSymbol(exchange, item.WalletType, item.Asset, symbols); !ok {
			return fmt.Errorf("initial asset %s (%s) does not match any configured symbol", item.Asset, item.WalletType)
		}
	}
	return nil
}

func initialAssetAmounts(assets []ctypes.AssetInput) (map[initialAssetKey]decimal.Decimal, error) {
	out := make(map[initialAssetKey]decimal.Decimal, len(assets))
	for _, item := range assets {
		code := ctypes.ParseAssetCode(item.Asset)
		total, err := decimal.NewFromString(strings.TrimSpace(item.Total))
		if err != nil {
			return nil, fmt.Errorf("invalid total for asset %s: %w", code, err)
		}
		if total.IsNegative() {
			return nil, fmt.Errorf("total must be >= 0 for asset %s", code)
		}
		k := initialAssetKey{
			WalletType: item.WalletType,
			Asset:      code,
		}
		out[k] = total
	}
	return out, nil
}

type initialAssetKey struct {
	WalletType ctypes.WalletType
	Asset      string
}
