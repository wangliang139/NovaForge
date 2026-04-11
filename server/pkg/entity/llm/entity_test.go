package llm

import (
	"context"
	"os"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/kelseyhightower/envconfig"
	"github.com/openai/openai-go/v3"
	"github.com/rs/zerolog/log"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/NovaForge/server/pkg/internal/zai"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
)

func TestEntity_Completion(t *testing.T) {
	os.Setenv("POSTGRES_PASSWORD", "postgres")
	os.Setenv("POSTGRES_APPNAME", "novaforge")
	os.Setenv("POSTGRES_DBNAME", "llt_data_db")
	os.Setenv("POSTGRES_PASSWORD", "my-secret")

	var wpgxConfig wpgx.Config
	envconfig.MustProcess("postgres", &wpgxConfig)
	log.Info().Msgf("wpgx config: %+v", &wpgxConfig)

	pool, err := wpgx.NewPool(context.Background(), &wpgxConfig)
	if err != nil {
		t.Fatalf("failed to create wpgx pool: %v", err)
	}
	defer pool.Close()

	db := repos.New(pool, nil)

	zai := zai.NewEngine()
	entity := New(db, zai)

	req := &CompletionRequest{
		SceneKey: "ai_document_summary",
		Variables: map[string]any{
			"title":   "The Impact of AI on the Future of Work",
			"content": "The impact of AI on the future of work is a topic that has been debated for years. Some people believe that AI will replace human workers, while others believe that AI will create new jobs and opportunities. The truth is that AI will have a significant impact on the future of work, but it is not clear what the impact will be.",
		},
	}

	resp, err := entity.Completion(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(resp)
}

func Test_LlmResponseFormat(t *testing.T) {
	type AiSummaryResult struct {
		Title          string   `json:"title"`
		Summary        string   `json:"summary"`
		Tags           []string `json:"tags"`
		Coins          []string `json:"coins"`
		Influence      string   `json:"influence"`
		InfluenceScore int      `json:"influence_score"`
		Sentiment      int      `json:"sentiment"`
	}

	AiSummaryResultSchema := utils.LLM.GenerateSchema(AiSummaryResult{})

	format := types.LlmResponseFormat{
		Type: "json_schema",
		JSONSchema: &openai.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   "ai_summary_result",
			Schema: AiSummaryResultSchema,
			Strict: openai.Bool(true),
		},
	}
	schema, err := sonic.MarshalString(format)
	if err != nil {
		t.Fatal(err)
	}
	log.Info().Msg(schema)
}
