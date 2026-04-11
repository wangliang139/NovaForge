package llm

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/jackc/pgx/v5"
	"github.com/kelseyhightower/envconfig"
	"github.com/samber/lo"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/NovaForge/server/pkg/internal/zai"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/repos/llm_prompt"
	"github.com/wangliang139/NovaForge/server/pkg/repos/llm_scene"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/errors"
)

type Config struct {
	MaxTimeout int `split_words:"true" envconfig:"MAX_TIMEOUT" default:"300"`
}

type Entity struct {
	db  *repos.Entity
	cfg Config

	zai *zai.Engine
}

func New(db *repos.Entity, zai *zai.Engine) *Entity {
	cfg := Config{}
	envconfig.MustProcess("LLM", &cfg)
	return &Entity{
		cfg: cfg,
		db:  db,
		zai: zai,
	}
}

// CreateScene creates a new LLM scene
func (e *Entity) CreateScene(ctx context.Context, input *types.UpsertSceneInput) (*types.LlmScene, error) {
	config, err := sonic.Marshal(input.Config)
	if err != nil {
		return nil, err
	}

	messages, err := sonic.Marshal(input.Messages)
	if err != nil {
		return nil, err
	}

	responseFormat, err := sonic.Marshal(input.ResponseFormat)
	if err != nil {
		return nil, err
	}

	params := &llm_scene.CreateParams{
		Key:            input.Key,
		Name:           input.Name,
		Description:    input.Description,
		Config:         config,
		Messages:       messages,
		Timeout:        input.Timeout,
		ResponseFormat: responseFormat,
		Enabled:        false, // disabled by default
	}
	scenePo, err := e.db.LlmSceneRepo.Create(ctx, *params)
	if err != nil {
		return nil, err
	}

	scene, err := types.SceneFromDB(scenePo)
	if err != nil {
		return nil, err
	}
	return scene, nil
}

// UpdateScene updates an existing LLM scene
func (e *Entity) UpdateScene(ctx context.Context, input *types.UpsertSceneInput) (*types.LlmScene, error) {
	if input.ID == nil {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	config, err := sonic.Marshal(input.Config)
	if err != nil {
		return nil, err
	}

	messages, err := sonic.Marshal(input.Messages)
	if err != nil {
		return nil, err
	}

	responseFormat, err := sonic.Marshal(input.ResponseFormat)
	if err != nil {
		return nil, err
	}

	scenePo, err := e.db.LlmSceneRepo.Update(ctx, llm_scene.UpdateParams{
		ID:             *input.ID,
		Name:           &input.Name,
		Description:    &input.Description,
		Config:         config,
		Messages:       messages,
		Timeout:        &input.Timeout,
		ResponseFormat: responseFormat,
		Enabled:        &input.Enabled,
	})
	if err != nil {
		return nil, err
	}

	if scenePo == nil {
		return nil, errors.New(errors.NotFound, "scene not found")
	}

	scene, err := types.SceneFromDB(scenePo)
	if err != nil {
		return nil, err
	}
	return scene, nil
}

// DeleteScene deletes an LLM scene (logical delete)
func (e *Entity) DeleteScene(ctx context.Context, id int64) error {
	_, err := e.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		count, err := e.db.LlmSceneRepo.WithTx(tx).Delete(ctx, id)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, errors.New(errors.NotFound, "scene not found")
		}
		_, err = e.db.LlmPromptRepo.WithTx(tx).DeleteBySceneID(ctx, id)
		if err != nil {
			return nil, err
		}

		return nil, nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (e *Entity) CountScenes(ctx context.Context, input *types.QueryScenesInput) (int64, error) {
	count, err := e.db.LlmSceneRepo.Count(ctx, input.Enabled)
	if err != nil {
		return 0, err
	}
	return *count, nil
}

// QueryScenes queries LLM scenes
func (e *Entity) QueryScenes(ctx context.Context, input *types.QueryScenesInput) ([]*types.LlmScene, error) {
	list, err := e.db.LlmSceneRepo.List(ctx, llm_scene.ListParams{
		Enabled: input.Enabled,
		Offset:  input.Offset,
		Limit:   input.Limit,
	})
	if err != nil {
		return nil, err
	}

	scenes := make([]*types.LlmScene, len(list))
	for i := range list {
		scenes[i], err = types.SceneFromDB(&list[i])
		if err != nil {
			return nil, err
		}
	}
	return scenes, nil
}

// GetScene gets an LLM scene by key
func (e *Entity) GetScene(ctx context.Context, id int64) (*types.LlmScene, error) {
	scenePo, err := e.db.LlmSceneRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if scenePo == nil {
		return nil, errors.New(errors.NotFound, "scene not found")
	}
	scene, err := types.SceneFromDB(scenePo)
	if err != nil {
		return nil, err
	}
	return scene, nil
}

// GetSceneByKey gets an LLM scene by key
func (e *Entity) GetSceneByKey(ctx context.Context, key string) (*types.LlmScene, error) {
	scenePo, err := e.db.LlmSceneRepo.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if scenePo == nil {
		return nil, errors.New(errors.NotFound, "scene not found")
	}
	scene, err := types.SceneFromDB(scenePo)
	if err != nil {
		return nil, err
	}
	return scene, nil
}

// CreatePrompt creates a new LLM prompt
func (e *Entity) CreatePrompt(ctx context.Context, input *types.CreatePromptInput) (*types.LlmPrompt, error) {
	config, err := sonic.Marshal(input.Config)
	if err != nil {
		return nil, err
	}

	messages, err := sonic.Marshal(input.Messages)
	if err != nil {
		return nil, err
	}

	scene, err := e.db.LlmSceneRepo.GetByID(ctx, input.SceneID)
	if err != nil {
		return nil, err
	}
	if scene == nil {
		return nil, errors.New(errors.NotFound, "scene not found")
	}

	params := &llm_prompt.CreateParams{
		SceneID:   input.SceneID,
		SceneKey:  scene.Key,
		Platform:  input.Platform,
		Name:      input.Name,
		Model:     input.Model,
		Providers: lo.Ternary(input.Providers == nil, []string{}, input.Providers),
		Config:    config,
		Messages:  messages,
		Timeout:   input.Timeout,
		Weight:    input.Weight,
		Variants:  lo.Ternary(input.Variants == nil, []string{}, input.Variants),
		Enabled:   input.Enabled,
	}
	promptPo, err := e.db.LlmPromptRepo.Create(ctx, *params)
	if err != nil {
		return nil, err
	}
	prompt, err := types.PromptFromDB(promptPo)
	if err != nil {
		return nil, err
	}
	return prompt, nil
}

// UpdatePrompt updates an existing LLM prompt
func (e *Entity) UpdatePrompt(ctx context.Context, input *types.UpdatePromptInput) (*types.LlmPrompt, error) {
	if input.ID <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	var variants []string
	if input.Variants != nil {
		variants = *input.Variants
	}

	params := &llm_prompt.UpdateParams{
		ID:       input.ID,
		Weight:   input.Weight,
		Enabled:  input.Enabled,
		Name:     input.Name,
		Timeout:  input.Timeout,
		Variants: variants,
	}
	promptPo, err := e.db.LlmPromptRepo.Update(ctx, *params)
	if err != nil {
		return nil, err
	}
	prompt, err := types.PromptFromDB(promptPo)
	if err != nil {
		return nil, err
	}
	return prompt, nil
}

// DeletePrompt deletes an LLM prompt (logical delete)
func (e *Entity) DeletePrompt(ctx context.Context, id int64) error {
	count, err := e.db.LlmPromptRepo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New(errors.NotFound, "prompt not found")
	}
	return nil
}

// GetPrompt gets an LLM prompt by ID
func (e *Entity) GetPrompt(ctx context.Context, id int64) (*types.LlmPrompt, error) {
	promptPo, err := e.db.LlmPromptRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if promptPo == nil {
		return nil, errors.New(errors.NotFound, "prompt not found")
	}
	prompt, err := types.PromptFromDB(promptPo)
	if err != nil {
		return nil, err
	}
	return prompt, nil
}

// GetPromptsByScene gets prompts by scene key
func (e *Entity) GetPromptsByScene(ctx context.Context, sceneID int64, enabled *bool) ([]*types.LlmPrompt, error) {
	list, err := e.db.LlmPromptRepo.GetBySceneID(ctx, llm_prompt.GetBySceneIDParams{
		SceneID: sceneID,
		Enabled: enabled,
	})
	if err != nil {
		return nil, err
	}
	prompts := make([]*types.LlmPrompt, len(list))
	for i := range list {
		prompts[i], err = types.PromptFromDB(&list[i])
		if err != nil {
			return nil, err
		}
	}
	return prompts, nil
}

// CountPrompts counts the number of LLM prompts
func (e *Entity) CountPrompts(ctx context.Context, input *types.QueryPromptsInput) (int64, error) {
	count, err := e.db.LlmPromptRepo.Count(ctx, llm_prompt.CountParams{
		SceneID: input.SceneID,
		Enabled: input.Enabled,
	})
	if err != nil {
		return 0, err
	}
	return *count, nil
}

// QueryPrompts queries LLM prompts
func (e *Entity) QueryPrompts(ctx context.Context, input *types.QueryPromptsInput) ([]*types.LlmPrompt, error) {
	list, err := e.db.LlmPromptRepo.List(ctx, llm_prompt.ListParams{
		SceneID: input.SceneID,
		Enabled: input.Enabled,
		Offset:  input.Offset,
		Limit:   input.Limit,
	})
	if err != nil {
		return nil, err
	}
	prompts := make([]*types.LlmPrompt, len(list))
	for i := range list {
		prompts[i], err = types.PromptFromDB(&list[i])
		if err != nil {
			return nil, err
		}
	}
	return prompts, nil
}
