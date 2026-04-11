package types

import (
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/internal/zai"
	"github.com/wangliang139/llt-trade/server/pkg/repos/llm_prompt"
	"github.com/wangliang139/llt-trade/server/pkg/repos/llm_scene"
)

type LlmMessage struct {
	Role    string  `json:"role"`
	Content string  `json:"content"`
	Name    *string `json:"name,omitempty"`
}

type LlmMessages []*LlmMessage

// PromptConfig

func (p *LlmMessages) Valid() bool {
	if p == nil {
		return false
	}
	return len(*p) > 0
}

// LlmConfig
type LlmConfig struct {
	Temperature         *float64 `json:"temperature"`
	TopP                *float64 `json:"top_p"`
	MaxTokens           *int64   `json:"max_tokens"`
	MaxCompletionTokens *int64   `json:"max_completion_tokens"`
}

func (c *LlmConfig) IsEmpty() bool {
	if c == nil {
		return true
	}
	return c.Temperature == nil && c.TopP == nil && c.MaxTokens == nil && c.MaxCompletionTokens == nil
}

func (c *LlmConfig) Clone() *LlmConfig {
	if c == nil {
		return nil
	}
	newConfig := new(LlmConfig)
	if c.Temperature != nil {
		newConfig.Temperature = lo.ToPtr(*c.Temperature)
	}
	if c.TopP != nil {
		newConfig.TopP = lo.ToPtr(*c.TopP)
	}
	if c.MaxTokens != nil {
		newConfig.MaxTokens = lo.ToPtr(*c.MaxTokens)
	}
	if c.MaxCompletionTokens != nil {
		newConfig.MaxCompletionTokens = lo.ToPtr(*c.MaxCompletionTokens)
	}
	return newConfig
}

func (c *LlmConfig) Merge(other *LlmConfig) *LlmConfig {
	newConfig := c.Clone()
	if other == nil {
		return newConfig
	}
	if newConfig == nil {
		newConfig = new(LlmConfig)
	}
	if other.Temperature != nil {
		newConfig.Temperature = other.Temperature
	}
	if other.TopP != nil {
		newConfig.TopP = other.TopP
	}
	if other.MaxTokens != nil {
		newConfig.MaxTokens = other.MaxTokens
	}
	if other.MaxCompletionTokens != nil {
		newConfig.MaxCompletionTokens = other.MaxCompletionTokens
	}
	return newConfig
}

type LlmCompletion struct {
	ID        int64          `json:"id"`
	SessionID int64          `json:"session_id"`
	SceneID   int64          `json:"scene_id"`
	PromptID  int64          `json:"prompt_id"`
	Variant   string         `json:"variant"`
	Platform  string         `json:"platform"`
	Provider  string         `json:"provider"`
	Model     string         `json:"model"`
	Variables map[string]any `json:"variables"`
	Messages  []byte         `json:"messages"`
	Question  string         `json:"question"`
	Answer    string         `json:"answer"`
	Error     string         `json:"error"`
	Duration  int32          `json:"duration"`
	Tokens    []byte         `json:"tokens"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type LlmScene struct {
	ID             int64             `json:"id"`
	Key            string            `json:"key"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Config         *LlmConfig        `json:"config"`
	Messages       LlmMessages       `json:"messages"`
	Timeout        int               `json:"timeout"`
	ResponseFormat LlmResponseFormat `json:"response_format"`
	Enabled        bool              `json:"enabled"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`

	Prompts []*LlmPrompt `json:"prompts"`
}

type LlmPrompt struct {
	ID        int64            `json:"id"`
	SceneID   int64            `json:"scene_id"`
	SceneKey  string           `json:"scene_key"`
	Platform  zai.PlatformType `json:"platform"`
	Name      string           `json:"name"`
	Model     string           `json:"model"`
	Providers []string         `json:"providers"`
	Config    *LlmConfig       `json:"config"`
	Messages  LlmMessages      `json:"messages"`
	Timeout   int              `json:"timeout"`
	Weight    int              `json:"weight"`
	Variants  []string         `json:"variants"`
	Enabled   bool             `json:"enabled"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type LlmResponseFormat struct {
	Type       string                                          `json:"type"`
	JSONSchema *openai.ResponseFormatJSONSchemaJSONSchemaParam `json:"json_schema,omitempty"`
}

func SceneFromDB(scene *llm_scene.LlmScene) (*LlmScene, error) {
	var config *LlmConfig
	if len(scene.Config) > 0 {
		if err := sonic.Unmarshal(scene.Config, &config); err != nil {
			return nil, err
		}
	}
	var messages LlmMessages
	if len(scene.Messages) > 0 {
		if err := sonic.Unmarshal(scene.Messages, &messages); err != nil {
			return nil, err
		}
	}
	var responseFormat LlmResponseFormat
	if len(scene.ResponseFormat) > 0 {
		if err := sonic.Unmarshal(scene.ResponseFormat, &responseFormat); err != nil {
			return nil, err
		}
	}
	return &LlmScene{
		ID:             scene.ID,
		Key:            scene.Key,
		Name:           scene.Name,
		Description:    scene.Description,
		Config:         config,
		Messages:       messages,
		Timeout:        int(scene.Timeout),
		ResponseFormat: responseFormat,
		Enabled:        scene.Enabled,
		CreatedAt:      scene.CreatedAt,
		UpdatedAt:      scene.UpdatedAt,
	}, nil
}

func PromptFromDB(model *llm_prompt.LlmPrompt) (*LlmPrompt, error) {
	platform := zai.PlatformType(model.Platform)
	if !platform.Valid() {
		return nil, fmt.Errorf("invalid platform: %s", model.Platform)
	}
	var config *LlmConfig
	if len(model.Config) > 0 {
		if err := sonic.Unmarshal(model.Config, &config); err != nil {
			return nil, err
		}
	}
	var messages LlmMessages
	if len(model.Messages) > 0 {
		if err := sonic.Unmarshal(model.Messages, &messages); err != nil {
			return nil, err
		}
	}
	return &LlmPrompt{
		ID:        model.ID,
		SceneID:   model.SceneID,
		SceneKey:  model.SceneKey,
		Platform:  platform,
		Name:      model.Name,
		Model:     model.Model,
		Providers: model.Providers,
		Config:    config,
		Messages:  messages,
		Timeout:   int(model.Timeout),
		Weight:    int(model.Weight),
		Variants:  model.Variants,
		Enabled:   model.Enabled,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}, nil
}

// UpsertSceneInput is the business object for insert/update an LLM scene
type UpsertSceneInput struct {
	ID             *int64
	Key            string
	Name           string
	Description    string
	Config         LlmConfig
	Messages       LlmMessages
	Timeout        int32
	ResponseFormat LlmResponseFormat
	Enabled        bool
}

// QueryScenesInput is the business object for querying LLM scenes
type QueryScenesInput struct {
	Offset  int32
	Limit   int32
	Enabled *bool
}

// GetSceneInput is the business object for getting an LLM scene
type GetSceneInput struct {
	Key string
}

// CreatePromptInput is the business object for creating an LLM prompt
type CreatePromptInput struct {
	SceneID   int64
	Platform  string
	Name      string
	Model     string
	Providers []string
	Config    LlmConfig
	Messages  LlmMessages
	Timeout   int32
	Weight    int32
	Variants  []string
	Enabled   bool
}

// UpdatePromptInput is the business object for updating an LLM prompt
type UpdatePromptInput struct {
	ID       int64
	Weight   *int32
	Enabled  *bool
	Name     *string
	Timeout  *int32
	Variants *[]string
}

// QueryPromptsInput is the business object for querying LLM prompts
type QueryPromptsInput struct {
	Offset  int32
	Limit   int32
	SceneID *int64
	Enabled *bool
}

// GetPromptInput is the business object for getting an LLM prompt
type GetPromptInput struct {
	ID int64
}

// --- LLM service API request/response DTOs (in-process, non-proto)

type CreateSceneRequest struct {
	Key            string
	Name           string
	Description    string
	Config         LlmConfig
	Messages       LlmMessages
	Timeout        int32
	ResponseFormat LlmResponseFormat
	Enabled        bool
}

type CreateSceneResponse struct {
	Scene *LlmScene
}

type UpdateSceneRequest struct {
	ID             int64
	Key            string
	Name           string
	Description    string
	Config         LlmConfig
	Messages       LlmMessages
	Timeout        int32
	ResponseFormat LlmResponseFormat
	Enabled        bool
}

type UpdateSceneResponse struct {
	Scene *LlmScene
}

type DeleteSceneRequest struct {
	ID int64
}

type DeleteSceneResponse struct {
	Success bool
}

type QueryScenesRequest struct {
	Offset  int32
	Limit   int32
	Enabled *bool
}

type QueryScenesResponse struct {
	Scenes []*LlmScene
	Count  int64
}

type GetSceneRequest struct {
	ID          int64
	WithPrompts bool
}

type GetSceneResponse struct {
	Scene   *LlmScene
	Prompts []*LlmPrompt
}

type CreatePromptRequest struct {
	SceneID   int64
	Platform  string
	Name      string
	Model     string
	Providers []string
	Config    LlmConfig
	Messages  LlmMessages
	Timeout   int32
	Weight    int32
	Variants  []string
	Enabled   bool
}

type CreatePromptResponse struct {
	Prompt *LlmPrompt
}

type UpdatePromptRequest struct {
	ID       int64
	Enabled  *bool
	Weight   *int32
	Variants *[]string
	Name     *string
	Timeout  *int32
}

type UpdatePromptResponse struct {
	Prompt *LlmPrompt
}

type DeletePromptRequest struct {
	ID int64
}

type DeletePromptResponse struct {
	Success bool
}

type QueryPromptsRequest struct {
	Offset  int32
	Limit   int32
	SceneID *int64
	Enabled *bool
}

type QueryPromptsResponse struct {
	Prompts []*LlmPrompt
	Count   int64
}

type GetPromptRequest struct {
	ID int64
}

type GetPromptResponse struct {
	Prompt *LlmPrompt
}

type CompletionTestByKind int8

const (
	CompletionTestByPromptID CompletionTestByKind = 1
	CompletionTestByVariant  CompletionTestByKind = 2
	CompletionTestByPrompt   CompletionTestByKind = 3
)

// CompletionTestBy selects how the prompt is resolved (mirrors proto oneof test_by).
type CompletionTestBy struct {
	Kind     CompletionTestByKind
	PromptID int64 // for PromptID kind; 0 means weighted route among prompts
	Variant  string
	Prompt   *LlmPrompt
}

type CompletionRequest struct {
	SceneID   int64
	SceneKey  string
	TestBy    *CompletionTestBy
	Variables []byte
	Force     bool
}

type LlmTokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

type LlmCompletionMetadata struct {
	SceneKey string
	PromptID int64
	SceneID  int64
	Provider string
	Model    string
}

type CompletionResponse struct {
	Success      bool
	Duration     int64
	Result       string
	CompletionID *int64
	Usage        *LlmTokenUsage
	Metadata     *LlmCompletionMetadata
	Error        string
}

type GetCompletionStatsRequest struct {
	StartTs int64
	EndTs   int64
}

type LlmCompletionSceneStats struct {
	SceneKey      string
	SceneID       int64
	TotalCount    int64
	SuccessCount  int64
	FailCount     int64
	SuccessRate   float64
	AvgDurationMs float64
}

type GetCompletionStatsResponse struct {
	TotalCount   int64
	SuccessCount int64
	FailCount    int64
	SuccessRate  float64
	SceneStats   []*LlmCompletionSceneStats
}
