package market

import "time"

type Config struct {
	AccountRawMsgTopic string
	TradeBufferSize    int
	KlineBufferSize    int
	ShutdownTimeout    time.Duration
	MaxOrderBookSize   int
}

func (c *Config) applyDefaults() {
	if c.AccountRawMsgTopic == "" {
		c.AccountRawMsgTopic = "account.raw.msg"
	}
	if c.TradeBufferSize <= 0 {
		c.TradeBufferSize = 1000
	}
	if c.KlineBufferSize <= 0 {
		c.KlineBufferSize = 500
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 5 * time.Second
	}
	if c.MaxOrderBookSize <= 0 {
		c.MaxOrderBookSize = 200
	}
}
