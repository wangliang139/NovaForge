package tgbot

type ParseMode string

const (
	ParseModeMarkdown   ParseMode = "Markdown"
	ParseModeMarkdownV2 ParseMode = "MarkdownV2"
	ParseModeHTML       ParseMode = "HTML"
)

type CallbackData struct {
	ChatID int64  `json:"chat_id"`
	Action string `json:"action"`
	InnerId string `json:"inner_id"`
}
