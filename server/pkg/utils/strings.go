package utils

type _strings struct{}

var Strings = _strings{}

func (s _strings) TruncateUTF8(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return text
}
