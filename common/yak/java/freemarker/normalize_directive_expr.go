package freemarker

import (
	"regexp"
	"strconv"
	"strings"
)

var freemarkerSingleQuotedString = regexp.MustCompile(`'([^'\\]*(?:\\.[^'\\]*)*)'`)
var freemarkerCompactComparePattern = regexp.MustCompile(`([A-Za-z0-9_"\)\]])(gte|lte|gt|lt)([A-Za-z0-9_"\(\[])`)
var freemarkerCompactEqPattern = regexp.MustCompile(`([^!<>=])=([^=])`)

func normalizeDirectiveExpr(expr string) string {
	expr = freemarkerSingleQuotedString.ReplaceAllStringFunc(expr, func(raw string) string {
		content := raw[1 : len(raw)-1]
		return strconv.Quote(content)
	})
	expr = strings.ReplaceAll(expr, " gte ", ">=")
	expr = strings.ReplaceAll(expr, " lte ", "<=")
	expr = strings.ReplaceAll(expr, " gt ", ">")
	expr = strings.ReplaceAll(expr, " lt ", "<")
	expr = freemarkerCompactComparePattern.ReplaceAllStringFunc(expr, func(raw string) string {
		m := freemarkerCompactComparePattern.FindStringSubmatch(raw)
		if len(m) != 4 {
			return raw
		}
		op := m[2]
		switch op {
		case "gte":
			op = ">="
		case "lte":
			op = "<="
		case "gt":
			op = ">"
		case "lt":
			op = "<"
		}
		return m[1] + op + m[3]
	})
	return freemarkerCompactEqPattern.ReplaceAllString(expr, `$1==$2`)
}
