package language

import (
	"github.com/pemistahl/lingua-go"
	"golang.org/x/text/language"
)

func Detect(text string) language.Tag {
	detector := lingua.NewLanguageDetectorBuilder().
		FromAllLanguages().
		Build()

	confidences := detector.ComputeLanguageConfidenceValues(text)

	var (
		lang          lingua.Language
		maxConfidence float64
	)
	for _, confidence := range confidences {
		if confidence.Value() > maxConfidence {
			lang = confidence.Language()
			maxConfidence = confidence.Value()
		}
	}
	// Convert lingua-go language to golang.org/x/text/language
	return language.Make(lang.IsoCode639_1().String())
}
