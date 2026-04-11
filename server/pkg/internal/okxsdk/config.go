package okxsdk

type Config struct {
	BaseUrl    string
	ApiKey     string
	ApiSecret  string
	Proxy      *string
	Passphrase string
	TimeOffset int64
	IsDebug    bool
	// IsTestNet 表示 OKX demo trading（会自动添加 x-simulated-trading: 1 头）
	IsTestNet bool
}
