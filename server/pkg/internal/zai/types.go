package zai

type LlmApiType string

const (
	LlmApiTypeAll LlmApiType = "*"

	LlmApiTypeChat       LlmApiType = "chat"
	LlmApiTypeEmbedding  LlmApiType = "embedding"
	LlmApiTypeModeration LlmApiType = "moderation"
	LlmApiTypeModels     LlmApiType = "models"
)

type OrErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}
