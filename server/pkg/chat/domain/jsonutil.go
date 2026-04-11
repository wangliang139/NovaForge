package domain

import (
	"encoding/json"
)

func DecodeParts(raw []byte) []DialogPart {
	if len(raw) == 0 {
		return []DialogPart{}
	}
	var parts []DialogPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return []DialogPart{}
	}
	return parts
}

func DecodeContextMeta(raw []byte) *ContextMeta {
	if len(raw) == 0 {
		return nil
	}
	var meta ContextMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil
	}
	return &meta
}
