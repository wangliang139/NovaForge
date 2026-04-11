package push

import (
	"fmt"
	"regexp"
	"strings"
)

var templateVarPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

func renderTemplate(tpl string, vars map[string]any) string {
	if len(vars) == 0 {
		return tpl
	}
	return templateVarPattern.ReplaceAllStringFunc(tpl, func(seg string) string {
		matches := templateVarPattern.FindStringSubmatch(seg)
		if len(matches) < 2 {
			return seg
		}
		k := strings.TrimSpace(matches[1])
		v, ok := vars[k]
		if !ok || v == nil {
			return ""
		}
		return fmt.Sprint(v)
	})
}
