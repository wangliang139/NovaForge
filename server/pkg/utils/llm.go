package utils

import (
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
)

type _llm struct{}

var LLM = _llm{}

func (l _llm) Json(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func (l _llm) GenerateSchema(v any) any {
	// Structured Outputs uses a subset of JSON schema
	// These flags are necessary to comply with the subset
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	return reflector.ReflectFromType(reflect.TypeOf(v))
}
