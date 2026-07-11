package hooks

import "regexp"

var templatePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)

func Render(template string, ctx Context) string {
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := templatePattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return ""
		}
		return fieldValue(parts[1], ctx)
	})
}
