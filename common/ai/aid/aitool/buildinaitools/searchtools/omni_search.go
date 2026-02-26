package searchtools

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/net/html"
)

const (
	maxContentPerPage  = 4096
	maxRawBodyForParse = 512 * 1024
)

// FetchPageContent fetches a URL and extracts its text content.
func FetchPageContent(pageURL string, timeout time.Duration) string {
	isHttps, reqBytes, err := lowhttp.ParseUrlToHttpRequestRaw("GET", pageURL)
	if err != nil {
		log.Debugf("parse url %s failed: %v", pageURL, err)
		return ""
	}

	rsp, err := lowhttp.HTTP(
		lowhttp.WithRequest(reqBytes),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithTimeout(timeout),
		lowhttp.WithRedirectTimes(3),
	)
	if err != nil {
		log.Debugf("fetch %s failed: %v", pageURL, err)
		return ""
	}

	statusCode := rsp.GetStatusCode()
	if statusCode < 200 || statusCode >= 400 {
		return ""
	}

	body := rsp.GetBody()
	if len(body) == 0 {
		return ""
	}

	contentType := strings.ToLower(string(lowhttp.GetHTTPPacketHeader(rsp.RawPacket, "Content-Type")))
	if strings.Contains(contentType, "html") {
		if len(body) > maxRawBodyForParse {
			body = body[:maxRawBodyForParse]
		}
		return extractTextFromHTML(body)
	}

	if len(body) > maxContentPerPage {
		return string(body[:maxContentPerPage])
	}
	return string(body)
}

func extractTextFromHTML(body []byte) string {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return ""
	}

	skipTags := map[string]bool{
		"script": true, "style": true, "noscript": true,
		"iframe": true, "svg": true, "head": true,
	}

	var textParts []string
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				textParts = append(textParts, text)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(doc)

	return strings.Join(textParts, " ")
}

// SerializeResults serializes search results to JSON (utility for callers that need raw JSON).
func SerializeResults(results []*ostype.OmniSearchResult) (string, error) {
	resultMap := map[string]interface{}{
		"total":   len(results),
		"results": results,
	}
	data, err := json.MarshalIndent(resultMap, "", "  ")
	if err != nil {
		return "", utils.Errorf("serialize result failed: %v", err)
	}
	return string(data), nil
}
