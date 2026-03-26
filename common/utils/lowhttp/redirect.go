package lowhttp

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

var (
	httpRedirect       = regexp.MustCompile(`(?i)Location: ([^\r^\n]+)`)
	redirectStatusCode = regexp.MustCompile(`HTTP/\d(\.\d)? 3\d\d`)
	htmlRedirect       = regexp.MustCompile(`(?i)<meta\s+http-equiv="?refresh"?.*?URL=['"]?([^\s^"^']+)['"]?`)
	javaScriptRedirect = regexp.MustCompile(`(?i)window\.location(.href)?\s*?=\s*?["']?([^\s^'^"]+)["']?`)
)

// normalizeLocationHeader applies Chrome-compatible parsing rules to a raw
// Location header value.
//
// When Chrome resolves a redirect Location, it follows the WHATWG URL Standard
// (https://url.spec.whatwg.org/) and the Chromium URL parser
// (DoParseAfterSpecialScheme in url/third_party/mozilla/url_parse.cc).
// For special schemes (http, https), CountConsecutiveSlashesOrBackslashes
// consumes every leading '/' or '\' before extracting the authority, meaning:
//
//   - "///baidu.com"   → authority = "baidu.com"
//   - "//\baidu.com"   → authority = "baidu.com"
//   - "\\/baidu.com"   → authority = "baidu.com"
//
// When a Location value (which is a relative reference, not a full URL) starts
// with two or more characters that are all '/' or '\', the WHATWG parser enters
// the authority state, so the entire leading slash/backslash run is consumed
// and the remainder becomes the host. We normalise such values to the canonical
// protocol-relative form "//host…" so that the rest of our pipeline (UrlJoin)
// can handle them correctly by inheriting the scheme from the original request.
//
// A single leading '/' or '\' is left untouched because:
//   - '/'  is a standard absolute-path reference (host comes from request).
//   - '\'  for a relative-ref stays in path state per WHATWG, so it is also
//     treated as a path character by browsers.
func normalizeLocationHeader(location string) string {
	// Count the length of the leading run of '/' and '\' characters.
	i := 0
	for i < len(location) && (location[i] == '/' || location[i] == '\\') {
		i++
	}
	// Two or more leading slash/backslash characters → authority state.
	// Collapse to canonical "//" and append whatever follows the run.
	if i >= 2 {
		return "//" + location[i:]
	}
	// Zero or one leading character → relative path; leave untouched.
	return location
}

func GetRedirectFromHTTPResponse(rawResponse []byte, jsRedirect bool) (result string) {
	defer func() {
		if len(result) == 0 {
			return
		}
		result = normalizeLocationHeader(result)
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
