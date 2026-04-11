package domain

import (
	"fmt"
	"regexp"
	"strings"
)

var codeBlockPattern = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\\n(.*?)```")

func ParseAnswerParts(content string) []DialogPart {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return []DialogPart{}
	}

	matches := codeBlockPattern.FindAllStringSubmatchIndex(trimmed, -1)
	if len(matches) == 0 {
		return []DialogPart{{
			Type: EventText,
			Text: trimmed,
		}}
	}

	parts := make([]DialogPart, 0, len(matches)*2+1)
	last := 0
	codeIdx := 0
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}
		start, end := match[0], match[1]
		langStart, langEnd := match[2], match[3]
		bodyStart, bodyEnd := match[4], match[5]

		if start > last {
			text := strings.TrimSpace(trimmed[last:start])
			if text != "" {
				parts = append(parts, DialogPart{Type: EventText, Text: text})
			}
		}

		codeIdx++
		language := strings.TrimSpace(trimmed[langStart:langEnd])
		body := strings.TrimSpace(trimmed[bodyStart:bodyEnd])
		if body != "" {
			parts = append(parts, DialogPart{
				Type:     EventCode,
				BlockID:  fmt.Sprintf("code_%d", codeIdx),
				Language: language,
				Text:     body,
			})
		}

		last = end
	}

	if last < len(trimmed) {
		text := strings.TrimSpace(trimmed[last:])
		if text != "" {
			parts = append(parts, DialogPart{Type: EventText, Text: text})
		}
	}

	if len(parts) == 0 {
		return []DialogPart{{Type: EventText, Text: trimmed}}
	}
	return parts
}

func NormalizeQuestionParts(content string) []DialogPart {
	return []DialogPart{{
		Type: EventText,
		Text: strings.TrimSpace(content),
	}}
}
