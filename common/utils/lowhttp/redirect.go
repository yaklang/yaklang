package lowhttp

import (
	"regexp"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	httpRedirect       = regexp.MustCompile(`(?i)Location: ([^\r^\n]+)`)
	redirectStatusCode = regexp.MustCompile(`HTTP/\d(\.\d)? 3\d\d`)
	htmlRedirect       = regexp.MustCompile(`(?i)<meta\s+http-equiv="?refresh"?.*?URL=['"]?([^\s^"^']+)['"]?`)
	javaScriptRedirect = regexp.MustCompile(`(?i)window\.location(.href)?\s*?=\s*?["']?([^\s^'^"]+)["']?`)
)

func GetRedirectFromHTTPResponse(rawResponse []byte, jsRedirect bool) string {
	lines := utils.ParseStringToLines(string(rawResponse))
	if lines == nil {
		return ""
	}

	firstLine := lines[0]
	if redirectStatusCode.MatchString(firstLine) {
		result := httpRedirect.FindSubmatch(rawResponse)
		if result != nil && len(result) > 1 {
			path := result[1]
			return string(path)
		}
		return ""
	}

	res := htmlRedirect.FindSubmatch(rawResponse)
	if len(res) > 1 {
		return string(res[1])
	}

	if jsRedirect {
		res = javaScriptRedirect.FindSubmatch(rawResponse)
		if len(res) > 2 {
			return string(res[2])
		}
	}

	return ""
}
