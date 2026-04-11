package converter

import (
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/samber/lo"
	"github.com/wangliang139/NovaForge/server/pkg/action/model"
	"github.com/wangliang139/NovaForge/server/pkg/internal/zai"
	llmtypes "github.com/wangliang139/NovaForge/server/pkg/types"
	utypes "github.com/wangliang139/NovaForge/server/pkg/utils/types"
	"github.com/wangliang139/mow/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func LlmConfigGql2Types(in *model.LlmConfigInput) llmtypes.LlmConfig {
	if in == nil {
		return llmtypes.LlmConfig{}
	}
	var out llmtypes.LlmConfig
	if in.Temperature != nil {
		out.Temperature = in.Temperature
	}
	if in.TopP != nil {
		out.TopP = in.TopP
	}
	if in.MaxTokens != nil {
		v := int64(*in.MaxTokens)
		out.MaxTokens = &v
	}
	if in.MaxCompletionTokens != nil {
		v := int64(*in.MaxCompletionTokens)
		out.MaxCompletionTokens = &v
	}
	return out
}

func LlmConfigTypes2Gql(in *llmtypes.LlmConfig) *model.LlmConfig {
	if in == nil || in.IsEmpty() {
		return &model.LlmConfig{}
	}
	return &model.LlmConfig{
		Temperature:         in.Temperature,
		TopP:                in.TopP,
		MaxTokens:           utypes.PInt64ToPInt(in.MaxTokens),
		MaxCompletionTokens: utypes.PInt64ToPInt(in.MaxCompletionTokens),
	}
}

func LlmMessagesGql2Types(in []*model.LlmMessageInput) llmtypes.LlmMessages {
	if len(in) == 0 {
		return nil
	}
	out := make(llmtypes.LlmMessages, len(in))
	for i := range in {
		out[i] = &llmtypes.LlmMessage{
			Role:    in[i].Role,
			Content: in[i].Content,
			Name:    in[i].Name,
		}
	}
	return out
}

func LlmMessagesTypes2Gql(msgs llmtypes.LlmMessages) []*model.LlmMessage {
	if len(msgs) == 0 {
		return []*model.LlmMessage{}
	}
	out := make([]*model.LlmMessage, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		out = append(out, &model.LlmMessage{
			Role:    m.Role,
			Content: m.Content,
			Name:    m.Name,
		})
	}
	return out
}

func LlmResponseFormatGql2Types(in *model.LlmResponseFormatInput) llmtypes.LlmResponseFormat {
	if in == nil {
		return llmtypes.LlmResponseFormat{}
	}
	out := llmtypes.LlmResponseFormat{Type: in.Type}
	if in.JSONSchema != nil {
		var schema map[string]any
		if in.JSONSchema.Schema != "" {
			_ = sonic.UnmarshalString(in.JSONSchema.Schema, &schema)
		}
		out.JSONSchema = &openai.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   in.JSONSchema.Name,
			Strict: openai.Bool(in.JSONSchema.Strict),
			Schema: schema,
		}
	}
	return out
}

func LlmResponseFormatTypes2Gql(in llmtypes.LlmResponseFormat) *model.LlmResponseFormat {
	if in.Type == "" && in.JSONSchema == nil {
		return nil
	}
	var js *model.LlmResponseFormatJSONSchema
	if in.JSONSchema != nil {
		schemaStr, _ := sonic.MarshalString(in.JSONSchema.Schema)
		strict := in.JSONSchema.Strict.Or(false)
		js = &model.LlmResponseFormatJSONSchema{
			Name:   in.JSONSchema.Name,
			Strict: strict,
			Schema: schemaStr,
		}
	}
	return &model.LlmResponseFormat{
		Type:       in.Type,
		JSONSchema: js,
	}
}

func LlmSceneTypes2Gql(in *llmtypes.LlmScene) *model.LlmScene {
	if in == nil {
		return nil
	}
	return &model.LlmScene{
		ID:             strconv.FormatInt(in.ID, 10),
		Key:            in.Key,
		Name:           in.Name,
		Description:    in.Description,
		Config:         LlmConfigTypes2Gql(in.Config),
		Messages:       LlmMessagesTypes2Gql(in.Messages),
		Timeout:        in.Timeout,
		ResponseFormat: LlmResponseFormatTypes2Gql(in.ResponseFormat),
		Enabled:        in.Enabled,
		CreatedAt:      int(in.CreatedAt.Unix()),
		UpdatedAt:      int(in.UpdatedAt.Unix()),
	}
}

func LlmPromptTypes2Gql(in *llmtypes.LlmPrompt) *model.LlmPrompt {
	if in == nil {
		return nil
	}
	variants := in.Variants
	if variants == nil {
		variants = []string{}
	}
	return &model.LlmPrompt{
		ID:        strconv.FormatInt(in.ID, 10),
		SceneID:   strconv.FormatInt(in.SceneID, 10),
		SceneKey:  in.SceneKey,
		Platform:  string(in.Platform),
		Name:      in.Name,
		Model:     in.Model,
		Providers: in.Providers,
		Config:    LlmConfigTypes2Gql(in.Config),
		Messages:  LlmMessagesTypes2Gql(in.Messages),
		Timeout:   in.Timeout,
		Weight:    in.Weight,
		Enabled:   in.Enabled,
		Variants:  variants,
		CreatedAt: int(in.CreatedAt.Unix()),
		UpdatedAt: int(in.UpdatedAt.Unix()),
	}
}

func ConvertToCreateSceneRequest(in *model.CreateLlmSceneInput) *llmtypes.CreateSceneRequest {
	if in == nil {
		return nil
	}
	return &llmtypes.CreateSceneRequest{
		Key:            in.Key,
		Name:           in.Name,
		Description:    in.Description,
		Config:         LlmConfigGql2Types(in.Config),
		Messages:       LlmMessagesGql2Types(in.Messages),
		Timeout:        int32(in.Timeout),
		ResponseFormat: LlmResponseFormatGql2Types(in.ResponseFormat),
		Enabled:        in.Enabled,
	}
}

func ConvertToUpdateSceneRequest(in *model.UpdateLlmSceneInput) (*llmtypes.UpdateSceneRequest, error) {
	if in == nil {
		return nil, errors.New(errors.InvalidArgument, "input is nil")
	}
	id, err := strconv.ParseInt(in.ID, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "invalid id")
	}
	return &llmtypes.UpdateSceneRequest{
		ID:             id,
		Key:            in.Key,
		Name:           in.Name,
		Description:    in.Description,
		Config:         LlmConfigGql2Types(in.Config),
		Messages:       LlmMessagesGql2Types(in.Messages),
		Timeout:        int32(in.Timeout),
		ResponseFormat: LlmResponseFormatGql2Types(in.ResponseFormat),
		Enabled:        in.Enabled,
	}, nil
}

func ConvertToDeleteSceneRequest(in *model.DeleteLlmSceneInput) (*llmtypes.DeleteSceneRequest, error) {
	if in == nil {
		return nil, errors.New(errors.InvalidArgument, "input is nil")
	}
	id, err := strconv.ParseInt(in.ID, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "invalid id")
	}
	return &llmtypes.DeleteSceneRequest{
		ID: id,
	}, nil
}

func ConvertToCreatePromptRequest(in *model.CreateLlmPromptInput) (*llmtypes.CreatePromptRequest, error) {
	if in == nil {
		return nil, errors.New(errors.InvalidArgument, "input is nil")
	}
	sceneID, err := strconv.ParseInt(in.SceneID, 10, 64)
	if err != nil || sceneID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "invalid sceneId")
	}
	return &llmtypes.CreatePromptRequest{
		SceneID:   sceneID,
		Platform:  in.Platform,
		Name:      in.Name,
		Model:     in.Model,
		Providers: in.Providers,
		Config:    LlmConfigGql2Types(in.Config),
		Messages:  LlmMessagesGql2Types(in.Messages),
		Timeout:   int32(in.Timeout),
		Weight:    int32(in.Weight),
		Enabled:   in.Enabled,
		Variants:  in.Variants,
	}, nil
}

func ConvertToUpdatePromptRequest(in *model.UpdateLlmPromptInput) (*llmtypes.UpdatePromptRequest, error) {
	if in == nil {
		return nil, errors.New(errors.InvalidArgument, "input is nil")
	}
	id, err := strconv.ParseInt(in.ID, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "invalid id")
	}

	var variants *[]string
	if in.Variants != nil {
		v := in.Variants.Values
		variants = &v
	}

	return &llmtypes.UpdatePromptRequest{
		ID:       id,
		Enabled:  in.Enabled,
		Weight:   utypes.PIntToPInt32(in.Weight),
		Variants: variants,
		Name:     in.Name,
		Timeout:  utypes.PIntToPInt32(in.Timeout),
	}, nil
}

func sceneTestPromptFromGql(in *model.SceneTestPromptInput) (*llmtypes.LlmPrompt, error) {
	if in == nil {
		return nil, nil
	}
	platform := zai.PlatformType(in.Platform)
	if !platform.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid platform")
	}
	cfg := LlmConfigGql2Types(in.Config)
	var cfgPtr *llmtypes.LlmConfig
	if !cfg.IsEmpty() {
		cfgPtr = &cfg
	}
	return &llmtypes.LlmPrompt{
		Platform:  platform,
		Name:      in.Name,
		Model:     in.Model,
		Providers: in.Providers,
		Config:    cfgPtr,
		Messages:  LlmMessagesGql2Types(in.Messages),
		Timeout:   in.Timeout,
	}, nil
}

func ConvertToSceneTestRequest(in *model.SceneTestInput) (*llmtypes.CompletionRequest, error) {
	if in == nil {
		return nil, nil
	}
	id, err := strconv.ParseInt(in.SceneID, 10, 64)
	if err != nil || id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "scene_id is invalid")
	}
	request := &llmtypes.CompletionRequest{
		SceneID:   id,
		Variables: []byte(in.Variables),
	}
	if in.ByVariant != nil {
		request.TestBy = &llmtypes.CompletionTestBy{
			Kind:    llmtypes.CompletionTestByVariant,
			Variant: *in.ByVariant,
		}
	}
	if in.ByPromptID != nil {
		promptID, err := strconv.ParseInt(*in.ByPromptID, 10, 64)
		if err != nil || promptID <= 0 {
			return nil, status.Error(codes.InvalidArgument, "prompt_id is invalid")
		}
		request.TestBy = &llmtypes.CompletionTestBy{
			Kind:     llmtypes.CompletionTestByPromptID,
			PromptID: promptID,
		}
	}
	if in.ByPrompt != nil {
		p, err := sceneTestPromptFromGql(in.ByPrompt)
		if err != nil {
			return nil, err
		}
		request.TestBy = &llmtypes.CompletionTestBy{
			Kind:   llmtypes.CompletionTestByPrompt,
			Prompt: p,
		}
	}
	return request, nil
}

func ConvertToSceneTestResult(in *llmtypes.CompletionResponse) *model.SceneTestResult {
	if in == nil {
		return nil
	}
	return &model.SceneTestResult{
		Success:  in.Success,
		Metadata: CompletionMetadataTypes2Gql(in.Metadata),
		Result:   &in.Result,
		Error:    &in.Error,
		CompletionID: utypes.PInt64ToPString(in.CompletionID),
		Duration: lo.ToPtr(int(in.Duration)),
		Usage:    CompletionUsageTypes2Gql(in.Usage),
	}
}

func CompletionMetadataTypes2Gql(in *llmtypes.LlmCompletionMetadata) *model.CompletionMetadata {
	if in == nil {
		return nil
	}
	out := &model.CompletionMetadata{
		SceneKey: in.SceneKey,
		SceneID:  strconv.FormatInt(in.SceneID, 10),
		PromptID: strconv.FormatInt(in.PromptID, 10),
	}
	if in.Model != "" {
		out.Model = &in.Model
	}
	if in.Provider != "" {
		out.Provider = &in.Provider
	}
	return out
}

func CompletionUsageTypes2Gql(in *llmtypes.LlmTokenUsage) *model.CompletionUsage {
	if in == nil {
		return nil
	}
	return &model.CompletionUsage{
		PromptTokens:     int(in.PromptTokens),
		CompletionTokens: int(in.CompletionTokens),
		TotalTokens:      int(in.TotalTokens),
	}
}

func ConvertToDeletePromptRequest(in *model.DeleteLlmPromptInput) (*llmtypes.DeletePromptRequest, error) {
	if in == nil {
		return nil, errors.New(errors.InvalidArgument, "input is nil")
	}
	id, err := strconv.ParseInt(in.ID, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "invalid id")
	}
	return &llmtypes.DeletePromptRequest{
		ID: id,
	}, nil
}
