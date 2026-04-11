package types

import "context"

// Stream 服务请求/响应 DTO（避免依赖 protobuf 生成类型）。

type EnsureSubscriptionSelector struct {
	Stream    StreamType `json:"stream,omitempty"`
	Interval  *Interval  `json:"interval,omitempty"`
	AccountId *string    `json:"accountId,omitempty"`
	Symbol    *string    `json:"symbol,omitempty"`
}

type EnsureSubscriptionRequest struct {
	Exchange Exchange                    `json:"exchange,omitempty"`
	Selector *EnsureSubscriptionSelector `json:"selector,omitempty"`
}

type EnsureSubscriptionResponse struct {
	Subscription *Subscription `json:"subscription,omitempty"`
}

type ReleaseSubscriptionRequest struct {
	ID string `json:"id,omitempty"`
}

type ReleaseSubscriptionResponse struct {
	Success bool `json:"success,omitempty"`
}

type ListActiveSubscriptionsRequest struct {
	Exchange  *Exchange `json:"exchange,omitempty"`
	Symbol    *string   `json:"symbol,omitempty"`
	AccountId *string   `json:"accountId,omitempty"`
}

type ListActiveSubscriptionsResponse struct {
	Subscriptions []Subscription `json:"subscriptions,omitempty"`
}

type GetStreamStatsRequest struct {
	WindowHours int32 `json:"windowHours,omitempty"`
}

// StreamConnectorStats 单路 Connector 流统计（与 metrics.StreamStats 对应，交易所为强类型）。
type StreamConnectorStats struct {
	Exchange       Exchange `json:"exchange,omitempty"`
	Stream         string   `json:"stream,omitempty"`
	EventCount     int64    `json:"eventCount,omitempty"`
	AvgLatencyMs   float64  `json:"avgLatencyMs,omitempty"`
	MaxLatencyMs   float64  `json:"maxLatencyMs,omitempty"`
	ReconnectCount int64    `json:"reconnectCount,omitempty"`
}

type GetStreamStatsResponse struct {
	Stats []StreamConnectorStats `json:"stats,omitempty"`
}

type GetConnectorInfoRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountId string   `json:"accountId,omitempty"`
}

type GetConnectorInfoResponse struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	IsPrivate bool     `json:"isPrivate,omitempty"`
}

type SubscribeStreamRequest struct {
	StreamType StreamType `json:"streamType,omitempty"`
	Exchange   *Exchange  `json:"exchange,omitempty"`
	Symbol     string     `json:"symbol,omitempty"`
	Interval   *Interval  `json:"interval,omitempty"`
	AccountId  *string    `json:"accountId,omitempty"`
}

type SubscribeStreamResponse struct {
	Envelope *Envelope `json:"envelope,omitempty"`
}

// SubscribeStreamServer 流式订阅发送端（可由 gRPC / WebSocket 适配层实现）。
type SubscribeStreamServer interface {
	Send(*SubscribeStreamResponse) error
	Context() context.Context
}
