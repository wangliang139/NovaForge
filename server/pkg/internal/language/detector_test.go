package language

import (
	"testing"

	"golang.org/x/text/language"
)

func Test_detector(t *testing.T) {
	t.Run("Detect Chinese", func(t *testing.T) {
		text := "這段話已經是中文繁體，不需要進一步翻譯"
		expect := language.Chinese
		lang := Detect(text)

		matcher := language.NewMatcher([]language.Tag{language.Chinese, language.SimplifiedChinese, language.TraditionalChinese})
		// if there is no match the lang will be default to American English
		tag, _, _ := matcher.Match(lang)

		if tag.String() != expect.String() {
			t.Errorf("Expected language to be %s, got %s", lang.String(), expect.String())
		}
	})
	t.Run("Detect English", func(t *testing.T) {
		text := "This passage is already in Traditional Chinese, no further translation is needed."
		expect := language.English
		lang := Detect(text)

		matcher := language.NewMatcher([]language.Tag{expect})
		// if there is no match the lang will be default to American English
		tag, _, _ := matcher.Match(lang)

		if tag.String() != expect.String() {
			t.Errorf("Expected language to be %s, got %s", lang.String(), expect.String())
		}
	})
}
