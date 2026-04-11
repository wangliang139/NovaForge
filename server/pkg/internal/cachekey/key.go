package cachekey

import (
	"fmt"

	"github.com/wangliang139/mow/env"
)

const (
	SymbolsKey                = "%s:exchange:symbols"
	DocSemanticDedupLockKey   = "doc:semantic:dedup:lock"
	MarketAccountStreamSubsKey = "%s:market:account_stream_subscriptions"
	MarketStreamSubsKey       = "%s:market:stream_subscriptions"
)

func BuildSymbolsKey() string {
	return fmt.Sprintf(SymbolsKey, env.ServiceName())
}

// MarketAccountStreamSubscriptionsKey 用于持久化 account stream 订阅列表（已废弃，请用 MarketStreamSubscriptionsKey）。
func MarketAccountStreamSubscriptionsKey() string {
	return fmt.Sprintf(MarketAccountStreamSubsKey, env.ServiceName())
}

// MarketStreamSubscriptionsKey 用于持久化所有流类型的订阅列表，重启后据此自动续订。
func MarketStreamSubscriptionsKey() string {
	return fmt.Sprintf(MarketStreamSubsKey, env.ServiceName())
}

func DocEmbeddingLockKey(docId int64) string {
	return fmt.Sprintf("doc:embedding:lock:%d", docId)
}

func DocEmbeddingKey(docId int64) string {
	return fmt.Sprintf("doc:embedding:%d", docId)
}