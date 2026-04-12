package converter

import (
	"github.com/rs/zerolog/log"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func AccountRepo2Types(a *accountrepo.Account) *types.Account {
	if a == nil {
		return nil
	}
	accountType := types.AccountTypeReal
	if a.AccountType.Valid() {
		accountType = types.AccountType(a.AccountType)
	}
	acc := &types.Account{
		ID:               a.ID,
		Name:             a.Name,
		Exchange:         ctypes.Exchange(a.Exchange),
		ApiKey:           a.ApiKey,
		ApiSecret:        a.ApiSecret,
		Passphrase:       a.Passphrase,
		Tags:             a.Tags,
		Status:           types.AccountStatus(a.Status),
		Algorithm:        types.AuthAlgorithm(a.Algorithm),
		AccountType:      accountType,
		ParentAccountID:  a.ParentAccountID,
		MultiBotMode:     a.MultiBotMode,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
	}

	// 解析 config.risk 为风控配置
	if len(a.Config) > 0 {
		if riskCfg, err := types.ParseRiskConfigFromJSON(a.Config); err == nil {
			acc.Config = riskCfg
		} else {
			log.Err(err).Msg("failed to parse risk config")
		}
	}

	return acc
}
