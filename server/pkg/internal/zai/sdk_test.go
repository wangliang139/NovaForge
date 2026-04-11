package zai

import (
	"context"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/rs/zerolog/log"
)

func Test_NewEngine(t *testing.T) {
	engine := NewEngine()

	log.Info().Msg("start test")
	response, err := engine.Caller().WithPlatform(PlatformTypeOpenRouter).CreateChatCompletion(context.Background(), openai.ChatCompletionNewParams{
		Model: "openrouter/auto",
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfUser: &openai.ChatCompletionUserMessageParam{Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String("hello")}}},
		},
		Temperature: openai.Float(0.5),
		// ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
		// 	OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
		// 		JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
		// 			Name:   "ai_summary_result",
		// 			Schema: AiSummaryResultSchema,
		// 			Strict: openai.Bool(true),
		// 		},
		// 	},
		// },
	})
	if err != nil {
		t.Fatal(err)
	} else {
		t.Log(response)
	}
}
