package auth

// TradeMutationAllowlist 拥有「交易」权限的 API Key 仅可调用下列 Mutation 根字段（其余 Mutation 一律拒绝）。
// 调整业务时在此维护列表；不在列表中的变更不会自动获得 API Key 访问能力。
var TradeMutationAllowlist = map[string]struct{}{
	// 账户与交易
	"CreateAccount":           {},
	"UpdateAccount":           {},
	"OnlineAccount":           {},
	"OfflineAccount":          {},
	"DeleteAccount":           {},
	"RefreshAccountSnapshots": {},
	"PlaceOrder":              {},
	"CancelOrder":             {},
	"SetLeverage":             {},
	"UpdateAccountRiskConfig": {},
	// 策略 / Bot / 回测
	"CreateStrategy":   {},
	"UpdateStrategy":   {},
	"GenerateStrategy": {},
	"DeleteStrategy":   {},
	"ActiveStrategy":   {},
	"InactiveStrategy": {},
	"CreateDatasource": {},
	"DeleteDatasource": {},
	"RunBacktest":      {},
	"CreateBot":        {},
	"UpdateBot":        {},
	"StartBot":         {},
	"StopBot":          {},
	"UpgradeBot":       {},
	"DeleteBot":        {},
	// 文档（写入类）
	"ArchiveDocument": {},
	"CreateChannel":   {},
	"UpdateChannel":   {},
	"TestExtract":     {},
	// LLM 配置与测试
	"CreateLlmScene":  {},
	"UpdateLlmScene":  {},
	"DeleteLlmScene":  {},
	"CreateLlmPrompt": {},
	"UpdateLlmPrompt": {},
	"DeleteLlmPrompt": {},
	"SceneTest":       {},
}

// JWTOnlyMutations 仅允许 JWT，拒绝 API Key（密钥管理、自我繁衍等）。
var JWTOnlyMutations = map[string]struct{}{
	"CreateUserApiKey":        {},
	"DeleteUserApiKey":        {},
	"ChangeUserPassword":      {},
	"UpdateLlmProviderConfig": {},
	"SetSettings":             {},
	"SendTelegramCode":        {},
	"GetTelegramSession":      {},
	"RequestGatewayRestart":   {},
}

// JWTOnlyQueries 仅允许 JWT（敏感或副作用查询）。
var JWTOnlyQueries = map[string]struct{}{
	"UserApiKeys":             {},
	"UserApiKeyNameAvailable": {},
	"LlmProviderConfig":       {},
	"PushConfig":              {},
	"GetSettings":             {},
}

// TradeMutationAllowed 是否在交易白名单中。
func TradeMutationAllowed(name string) bool {
	_, ok := TradeMutationAllowlist[name]
	return ok
}
