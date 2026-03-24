package freemarker

import (
	"regexp"
	"strings"
)

var freemarkerBareEqPattern = regexp.MustCompile(`([^!<>=\s])=([^=])`)

func normalizeDirectiveHeaderExpr(src string) string {
	lines := strings.Split(src, "\n")
	for idx, line := range lines {
		lines[idx] = normalizeDirectiveHeaderLine(line)
	}
	return strings.Join(lines, "\n")
}

func normalizeDirectiveHeaderLine(line string) string {
	for _, prefix := range []string{"<#if ", "<#elseif "} {
		start := strings.Index(line, prefix)
		if start < 0 {
			continue
		}
		end := strings.LastIndex(line, ">")
		if end <= start+len(prefix) {
			return line
		}
		expr := line[start+len(prefix) : end]
		expr = strings.ReplaceAll(expr, ">=", " gte ")
		expr = strings.ReplaceAll(expr, "<=", " lte ")
		expr = strings.ReplaceAll(expr, " > ", " gt ")
		expr = strings.ReplaceAll(expr, " < ", " lt ")
		expr = freemarkerBareEqPattern.ReplaceAllString(expr, `${1}==${2}`)
		return line[:start+len(prefix)] + expr + line[end:]
	}
	return line
}
