package llmsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/entity"
	"github.com/wangliang139/llt-trade/server/pkg/entity/llm"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/logger"
)

type Service struct{}

func New() (*Service, error) {
	return &Service{}, nil
}

func (s *Service) CreateScene(ctx context.Context, request *types.CreateSceneRequest) (*types.CreateSceneResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("CreateScene params")

	key := strings.TrimSpace(request.Key)
	if len(key) == 0 {
		return nil, errors.New(errors.InvalidArgument, "key is required")
	}

	name := strings.TrimSpace(request.Name)
	if len(name) == 0 {
		return nil, errors.New(errors.InvalidArgument, "name is required")
	}

	input := &types.UpsertSceneInput{
		Key:            key,
		Name:           name,
		Description:    request.Description,
		Config:         request.Config,
		Messages:       request.Messages,
		Timeout:        request.Timeout,
		ResponseFormat: request.ResponseFormat,
		Enabled:        request.Enabled,
	}

	scene, err := entity.Llm.CreateScene(ctx, input)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to create scene")
		return nil, err
	}

	return &types.CreateSceneResponse{Scene: scene}, nil
}

func (s *Service) UpdateScene(ctx context.Context, request *types.UpdateSceneRequest) (*types.UpdateSceneResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("UpdateScene params")

	id := request.ID
	if id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	key := strings.TrimSpace(request.Key)
	if len(key) == 0 {
		return nil, errors.New(errors.InvalidArgument, "key is required")
	}

	input := &types.UpsertSceneInput{
		ID:             &id,
		Key:            key,
		Name:           request.Name,
		Description:    request.Description,
		Config:         request.Config,
		Messages:       request.Messages,
		Timeout:        request.Timeout,
		ResponseFormat: request.ResponseFormat,
		Enabled:        request.Enabled,
	}

	scene, err := entity.Llm.UpdateScene(ctx, input)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to update scene")
		return nil, err
	}

	return &types.UpdateSceneResponse{Scene: scene}, nil
}

func (s *Service) DeleteScene(ctx context.Context, request *types.DeleteSceneRequest) (*types.DeleteSceneResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("DeleteScene params")

	id := request.ID
	if id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	err := entity.Llm.DeleteScene(ctx, id)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to delete scene")
		return nil, err
	}

	return &types.DeleteSceneResponse{Success: true}, nil
}

func (s *Service) QueryScenes(ctx context.Context, request *types.QueryScenesRequest) (*types.QueryScenesResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("QueryScenes params")

	if request.Offset < 0 {
		return nil, errors.New(errors.InvalidArgument, "offset is invalid")
	}
	if request.Limit <= 0 {
		return nil, errors.New(errors.InvalidArgument, "limit is invalid")
	}

	input := &types.QueryScenesInput{
		Offset:  request.Offset,
		Limit:   request.Limit,
		Enabled: request.Enabled,
	}

	count, err := entity.Llm.CountScenes(ctx, input)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to count scenes")
		return nil, err
	}

	if int64(request.Offset) >= count {
		return &types.QueryScenesResponse{
			Count: count,
		}, nil
	}

	scenes, err := entity.Llm.QueryScenes(ctx, input)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to list scenes")
		return nil, err
	}

	return &types.QueryScenesResponse{
		Scenes: scenes,
		Count:  count,
	}, nil
}

func (s *Service) GetScene(ctx context.Context, request *types.GetSceneRequest) (*types.GetSceneResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("GetScene params")

	id := request.ID
	if id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	scene, err := entity.Llm.GetScene(ctx, id)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to get scene")
		return nil, err
	}

	resp := &types.GetSceneResponse{Scene: scene}
	if !request.WithPrompts {
		return resp, nil
	}

	prompts, err := entity.Llm.GetPromptsByScene(ctx, id, nil)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to get prompts")
		return nil, err
	}
	resp.Prompts = prompts

	return resp, nil
}

func (s *Service) CreatePrompt(ctx context.Context, request *types.CreatePromptRequest) (*types.CreatePromptResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("CreatePrompt params")

	sceneID := request.SceneID
	if sceneID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "scene_id is required")
	}

	platform := strings.TrimSpace(request.Platform)
	if len(platform) == 0 {
		return nil, errors.New(errors.InvalidArgument, "platform is required")
	}

	name := strings.TrimSpace(request.Name)
	if len(name) == 0 {
		return nil, errors.New(errors.InvalidArgument, "name is required")
	}

	modelName := strings.TrimSpace(request.Model)
	if len(modelName) == 0 {
		return nil, errors.New(errors.InvalidArgument, "model is required")
	}

	input := &types.CreatePromptInput{
		SceneID:   sceneID,
		Platform:  platform,
		Name:      name,
		Model:     modelName,
		Providers: request.Providers,
		Config:    request.Config,
		Messages:  request.Messages,
		Timeout:   request.Timeout,
		Weight:    request.Weight,
		Variants:  request.Variants,
		Enabled:   request.Enabled,
	}

	prompt, err := entity.Llm.CreatePrompt(ctx, input)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to create prompt")
		return nil, err
	}

	return &types.CreatePromptResponse{Prompt: prompt}, nil
}

func (s *Service) UpdatePrompt(ctx context.Context, request *types.UpdatePromptRequest) (*types.UpdatePromptResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("UpdatePrompt params")

	id := request.ID
	if id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	input := &types.UpdatePromptInput{
		ID:       id,
		Weight:   request.Weight,
		Enabled:  request.Enabled,
		Name:     request.Name,
		Timeout:  request.Timeout,
		Variants: request.Variants,
	}

	prompt, err := entity.Llm.UpdatePrompt(ctx, input)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to update prompt")
		return nil, err
	}

	return &types.UpdatePromptResponse{Prompt: prompt}, nil
}

func (s *Service) DeletePrompt(ctx context.Context, request *types.DeletePromptRequest) (*types.DeletePromptResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("DeletePrompt params")

	id := request.ID
	if id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	err := entity.Llm.DeletePrompt(ctx, id)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to delete prompt")
		return nil, err
	}

	return &types.DeletePromptResponse{Success: true}, nil
}

func (s *Service) QueryPrompts(ctx context.Context, request *types.QueryPromptsRequest) (*types.QueryPromptsResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("QueryPrompts params")

	if request.Offset < 0 {
		return nil, errors.New(errors.InvalidArgument, "offset is invalid")
	}
	if request.Limit <= 0 {
		return nil, errors.New(errors.InvalidArgument, "limit is invalid")
	}

	input := &types.QueryPromptsInput{
		Offset:  request.Offset,
		Limit:   request.Limit,
		SceneID: request.SceneID,
		Enabled: request.Enabled,
	}

	count, err := entity.Llm.CountPrompts(ctx, input)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to count prompts")
		return nil, err
	}

	if int64(request.Offset) >= count {
		return &types.QueryPromptsResponse{
			Count: count,
		}, nil
	}

	prompts, err := entity.Llm.QueryPrompts(ctx, input)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to list prompts")
		return nil, err
	}

	return &types.QueryPromptsResponse{
		Prompts: prompts,
		Count:   count,
	}, nil
}

func (s *Service) GetPrompt(ctx context.Context, request *types.GetPromptRequest) (*types.GetPromptResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("GetPrompt params")

	id := request.ID
	if id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	prompt, err := entity.Llm.GetPrompt(ctx, id)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to get prompt")
		return nil, err
	}

	return &types.GetPromptResponse{Prompt: prompt}, nil
}

func (s *Service) Completion(ctx context.Context, request *types.CompletionRequest) (*types.CompletionResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("Completion params")

	sceneID := request.SceneID
	sceneKey := strings.TrimSpace(request.SceneKey)
	if sceneID <= 0 && sceneKey == "" {
		return nil, errors.New(errors.InvalidArgument, "scene_id or scene_key is required")
	}

	if request.TestBy == nil {
		return nil, errors.New(errors.InvalidArgument, "test_by is required")
	}

	// Parse variables JSON
	var variables map[string]any
	if len(request.Variables) > 0 {
		if err := sonic.Unmarshal(request.Variables, &variables); err != nil {
			return nil, errors.New(errors.InvalidArgument, "invalid variables: "+err.Error())
		}
	}

	var scene *types.LlmScene
	var err error
	switch {
	case sceneKey != "":
		scene, err = entity.Llm.GetSceneByKey(ctx, sceneKey)
	case sceneID > 0:
		scene, err = entity.Llm.GetScene(ctx, sceneID)
	default:
		return nil, errors.New(errors.InvalidArgument, "scene_id or scene_key is required")
	}

	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to get scene")
		return nil, err
	}
	if scene == nil {
		return nil, errors.New(errors.InvalidArgument, "scene not found")
	}

	req := &llm.CompletionRequest{Variables: variables, Force: request.Force}
	req.SceneKey = scene.Key

	switch request.TestBy.Kind {
	case types.CompletionTestByPromptID:
		if request.TestBy.PromptID > 0 {
			req.ByPromptID = lo.ToPtr(request.TestBy.PromptID)
		}
	case types.CompletionTestByVariant:
		if len(strings.TrimSpace(request.TestBy.Variant)) > 0 {
			req.SceneKey = fmt.Sprintf("%s:%s", scene.Key, strings.TrimSpace(request.TestBy.Variant))
		}
	case types.CompletionTestByPrompt:
		if request.TestBy.Prompt != nil {
			req.ByPrompt = request.TestBy.Prompt
		}
	default:
		return nil, errors.New(errors.InvalidArgument, "invalid test_by kind")
	}

	startTime := time.Now()
	response, err := entity.Llm.Completion(ctx, req)
	duration := time.Since(startTime)

	resp := &types.CompletionResponse{
		Success:  true,
		Duration: duration.Milliseconds(),
	}

	if response != nil {
		resp.Result = response.Result
		resp.CompletionID = response.CompletionID
		if response.Usage != nil {
			resp.Usage = &types.LlmTokenUsage{
				PromptTokens:     response.Usage.PromptTokens,
				CompletionTokens: response.Usage.CompletionTokens,
				TotalTokens:      response.Usage.TotalTokens,
			}
		}

		if response.Metadata != nil {
			resp.Metadata = &types.LlmCompletionMetadata{
				SceneKey: response.Metadata.SceneKey,
				PromptID: response.Metadata.Prompt.ID,
				SceneID:  response.Metadata.Scene.ID,
				Provider: lo.FromPtrOr(response.Metadata.Provider, ""),
				Model:    lo.FromPtrOr(response.Metadata.Model, ""),
			}
		}
	}

	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
		logger.Ctx(ctx).Err(err).Msg("failed to completion")
	}

	return resp, nil
}

func (s *Service) GetCompletionStats(ctx context.Context, request *types.GetCompletionStatsRequest) (*types.GetCompletionStatsResponse, error) {
	if request.StartTs <= 0 || request.EndTs <= 0 || request.StartTs >= request.EndTs {
		return nil, errors.New(errors.InvalidArgument, "start_ts and end_ts must be valid (start_ts < end_ts)")
	}

	result, err := entity.Llm.GetCompletionStats(ctx, request.StartTs, request.EndTs)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to get completion stats")
		return nil, err
	}

	sceneStats := make([]*types.LlmCompletionSceneStats, 0, len(result.SceneStats))
	for _, ss := range result.SceneStats {
		sceneStats = append(sceneStats, &types.LlmCompletionSceneStats{
			SceneKey:      ss.SceneKey,
			SceneID:       ss.SceneID,
			TotalCount:    ss.TotalCount,
			SuccessCount:  ss.SuccessCount,
			FailCount:     ss.FailCount,
			SuccessRate:   ss.SuccessRate,
			AvgDurationMs: ss.AvgDurationMs,
		})
	}

	return &types.GetCompletionStatsResponse{
		TotalCount:   result.TotalCount,
		SuccessCount: result.SuccessCount,
		FailCount:    result.FailCount,
		SuccessRate:  result.SuccessRate,
		SceneStats:   sceneStats,
	}, nil
}
