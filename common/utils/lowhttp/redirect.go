package lowhttp

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"regexp"
	"strings"
)

var (
	httpRedirect       = regexp.MustCompile(`(?i)Location: ([^\r^\n]+)`)
	redirectStatusCode = regexp.MustCompile(`HTTP/\d(\.\d)? 3\d\d`)
	htmlRedirect       = regexp.MustCompile(`(?i)<meta\s+http-equiv="?refresh"?.*?URL=['"]?([^\s^"^']+)['"]?`)
	javaScriptRedirect = regexp.MustCompile(`(?i)window\.location(.href)?\s*?=\s*?["']?([^\s^'^"]+)["']?`)
)

func GetRedirectFromHTTPResponse(rawResponse []byte, jsRedirect bool) (result string) {
	defer func() {
		if len(result) == 0 {
			return
		}
		testURL := result
		if !strings.HasPrefix(result, "http://") && !strings.HasPrefix(result, "https://") {
			if !strings.HasPrefix(result, "/") {
				testURL = "/" + result
			}
			testURL = fmt.Sprintf("http://127.0.0.1%s", result)
		}
		u, err := url.Parse(testURL)
		if err != nil || u == nil {
			result = ""
			return
		}
		if len(u.Host) == 0 {
			result = ""
		}
	}()

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

	matched := htmlRedirect.FindSubmatch(rawResponse)
	if len(matched) > 1 {
		return string(matched[1])
	}

	if jsRedirect {
		matched = javaScriptRedirect.FindSubmatch(rawResponse)
		if len(matched) > 2 {
			return string(matched[2])
		}
	}

	return ""
}
