package runner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/runner/api"
	"rogchap.com/v8go"
)

type fakeAICompleter struct {
	req    api.AICompletionRequest
	called bool
	result *api.AICompletionResult
	err    error
}

func (f *fakeAICompleter) Complete(ctx context.Context, req api.AICompletionRequest) (*api.AICompletionResult, error) {
	f.called = true
	f.req = req
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &api.AICompletionResult{
		Result:   map[string]any{"action": "hold", "confidence": 0.9},
		Text:     `{"action":"hold","confidence":0.9}`,
		JSON:     map[string]any{"action": "hold", "confidence": 0.9},
		Model:    "fake-model",
		Duration: 1,
	}, nil
}

func TestRuntimeInjectAIComplete(t *testing.T) {
	iso := v8go.NewIsolate()
	defer iso.Dispose()
	ctx := v8go.NewContext(iso)
	defer ctx.Close()

	fake := &fakeAICompleter{}
	runtime := NewRuntime(nil, nil).WithAIAPI(api.NewAIAPI(api.AIAPIConfig{
		Completer:      fake,
		DefaultTimeout: 2 * time.Second,
		MaxTimeout:     5 * time.Second,
	}))

	require.NoError(t, runtime.Inject(ctx))
	_, err := ctx.RunScript(`
		const decision = ai.complete({
			prompt: "return json",
			json: true,
			timeoutMs: 1000,
		});
		if (decision.result.action !== "hold") {
			throw new Error("unexpected action");
		}
	`, "test_ai_complete.js")
	require.NoError(t, err)
	require.True(t, fake.called)
	require.Equal(t, "return json", fake.req.Prompt)
	require.Equal(t, time.Second, fake.req.Timeout)
	require.True(t, fake.req.JSON)
}

func TestRuntimeAICompleteRejectsTimeoutAboveLimit(t *testing.T) {
	iso := v8go.NewIsolate()
	defer iso.Dispose()
	ctx := v8go.NewContext(iso)
	defer ctx.Close()

	fake := &fakeAICompleter{}
	runtime := NewRuntime(nil, nil).WithAIAPI(api.NewAIAPI(api.AIAPIConfig{
		Completer:  fake,
		MaxTimeout: time.Second,
	}))

	require.NoError(t, runtime.Inject(ctx))
	_, err := ctx.RunScript(`
		ai.complete({
			prompt: "too slow",
			timeoutMs: 2000,
		});
	`, "test_ai_timeout.js")
	require.Error(t, err)
	require.False(t, fake.called)
}
