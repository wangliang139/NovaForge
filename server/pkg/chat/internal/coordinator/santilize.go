package coordinator

import "regexp"

var (
	ansiCSIRegex    = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	ansiSingleRegex = regexp.MustCompile(`\x1b[@-_]`)
	controlRegex    = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

func SanitizeStreamText(text string) string {
	clean := ansiCSIRegex.ReplaceAllString(text, "")
	clean = ansiSingleRegex.ReplaceAllString(clean, "")
	clean = controlRegex.ReplaceAllString(clean, "")
	return clean
}
