package memory

import (
	"unicode/utf8"

	"github.com/wangliang139/NovaForge/server/pkg/chat/domain"
)

func EstimateInputTokens(messages []domain.ChatMessage) int {
	totalRunes := 0
	for _, item := range messages {
		totalRunes += utf8.RuneCountInString(item.Content)
	}
	if totalRunes == 0 {
		return 0
	}
	return totalRunes / 4
}
