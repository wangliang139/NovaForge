package utils

import (
	"strings"
	"text/template"
)

var _templates = make(map[string]*template.Template)

type _template struct{}

var Template = _template{}

func (t _template) Render(tpl string, variables map[string]any) (string, error) {
	key, err := Hash.Md5(tpl)
	if err != nil {
		return "", err
	}
	tmpl, ok := _templates[key]
	if !ok {
		var err error
		tmpl, err = template.New(key).Parse(tpl)
		if err != nil {
			return "", err
		}
		_templates[key] = tmpl
	}
	var result strings.Builder
	err = tmpl.Execute(&result, variables)
	if err != nil {
		return "", err
	}
	return result.String(), nil
}
