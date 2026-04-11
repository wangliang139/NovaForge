package llm

import (
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/wangliang139/NovaForge/server/pkg/types"
)

type SceneKey struct {
	Key     string
	Variant string
}

func (k *SceneKey) String() string {
	if len(k.Variant) == 0 {
		return k.Key
	}
	return fmt.Sprintf("%s:%s", k.Key, k.Variant)
}

type Metadata struct {
	SceneKey  string
	Scene     types.LlmScene
	Prompt    types.LlmPrompt
	Variables map[string]any

	Provider *string
	Model    *string
}

type CompletionRequest struct {
	SceneKey string

	ByPromptID *int64
	ByPrompt   *types.LlmPrompt

	Force     bool
	Variables map[string]any
}

type CompletionResponse struct {
	Metadata *Metadata

	Raw          *openai.ChatCompletion
	Usage        *openai.CompletionUsage
	Result       string
	Duration     time.Duration
	CompletionID *int64
}
