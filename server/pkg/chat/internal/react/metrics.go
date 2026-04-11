package react

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "NovaForge/server/pkg/chat/react"

var (
	metricsOnce      sync.Once
	counterStep      metric.Int64Counter
	counterToolCall  metric.Int64Counter
	counterToolFail  metric.Int64Counter
	histModelSeconds metric.Float64Histogram
	histToolSeconds  metric.Float64Histogram
)

func initMetrics() {
	metricsOnce.Do(func() {
		m := otel.Meter(meterName)
		var err error
		counterStep, err = m.Int64Counter("chat.react.step",
			metric.WithDescription("Chat ReAct completed model round"))
		if err != nil {
			counterStep = nil
		}
		counterToolCall, err = m.Int64Counter("chat.react.tool.call",
			metric.WithDescription("Chat ReAct tool invocation"))
		if err != nil {
			counterToolCall = nil
		}
		counterToolFail, err = m.Int64Counter("chat.react.tool.failure",
			metric.WithDescription("Chat ReAct tool failure by error code"))
		if err != nil {
			counterToolFail = nil
		}
		histModelSeconds, err = m.Float64Histogram("chat.react.model.roundtrip.seconds",
			metric.WithDescription("Latency of CreateChatCompletion per step"),
			metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120))
		if err != nil {
			histModelSeconds = nil
		}
		histToolSeconds, err = m.Float64Histogram("chat.react.tool.exec.seconds",
			metric.WithDescription("Latency of runtime tool execution"),
			metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 8))
		if err != nil {
			histToolSeconds = nil
		}
	})
}

func recordStep(ctx context.Context, outcome string) {
	initMetrics()
	if counterStep != nil {
		counterStep.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
	}
}

func recordToolCall(ctx context.Context, result string) {
	initMetrics()
	if counterToolCall != nil {
		counterToolCall.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
	}
}

func recordToolFailure(ctx context.Context, code string) {
	initMetrics()
	if counterToolFail != nil {
		counterToolFail.Add(ctx, 1, metric.WithAttributes(attribute.String("code", code)))
	}
}

func recordModelLatency(ctx context.Context, seconds float64) {
	initMetrics()
	if histModelSeconds != nil {
		histModelSeconds.Record(ctx, seconds)
	}
}

func recordToolLatency(ctx context.Context, seconds float64) {
	initMetrics()
	if histToolSeconds != nil {
		histToolSeconds.Record(ctx, seconds)
	}
}
