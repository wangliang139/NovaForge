package binancesdk

type Config struct {
	ApiKey     string
	ApiSecret  string
	Proxy      *string
	TimeOffset int64
	IsDebug    bool
}
