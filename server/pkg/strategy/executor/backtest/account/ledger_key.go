package account

import (
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// ledgerKey 余额池主键，与生产侧 (asset, walletType) 及 Portfolio AssetKey 对齐。
type ledgerKey struct {
	WalletType ctypes.WalletType
	Asset      string
}

func newLedgerKey(walletType ctypes.WalletType, asset string) ledgerKey {
	return ledgerKey{
		WalletType: walletType,
		Asset:      ctypes.ParseAssetCode(asset),
	}
}

func ledgerKeyForSymbol(exchange ctypes.Exchange, symbol ctypes.Symbol, asset string) ledgerKey {
	return newLedgerKey(ctypes.GetWalletType(exchange, symbol.Type), asset)
}

func resolveWalletType(exchange ctypes.Exchange, symbol *ctypes.Symbol, signalWT ctypes.WalletType) ctypes.WalletType {
	if signalWT.Valid() {
		return signalWT
	}
	if symbol != nil {
		return ctypes.GetWalletType(exchange, symbol.Type)
	}
	return ""
}
