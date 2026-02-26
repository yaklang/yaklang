package searchtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/net/html"
)

type TextCompressorFunc func(ctx context.Context, text any, destination string, maxBytes int64) (string, error)

const (
	defaultMaxResults     = 5
	maxContentPerPage     = 4096
	defaultFetchTimeout   = 10 * time.Second
	compressTargetBytes   = 10 * 1024
	maxRawBodyForParse    = 512 * 1024
)

// CreateEnhancedWebSearchTool creates the enhanced web_search tool.
// When compressor is provided (non-nil), results are compressed via AI for cleaner output.
// When compressor is nil, raw search results with page content are returned.
func CreateEnhancedWebSearchTool(compressor TextCompressorFunc) (*aitool.Tool, error) {
	return aitool.New(
		"web_search",
		aitool.WithDescription("Search the internet for information. Fetches page content and optionally compresses results via AI for refined output."),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("search query keywords"),
		),
		aitool.WithIntegerParam("max_results",
			aitool.WithParam_Description("maximum number of search results to process (default: 5)"),
		),
		aitool.WithBoolParam("fetch_content",
			aitool.WithParam_Description("whether to fetch and extract text from result URLs (default: true)"),
		),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			query := params.GetString("query")
			if query == "" {
				return nil, utils.Error("query is required")
			}
			maxResults := params.GetInteger("max_results")
			if maxResults <= 0 {
				maxResults = defaultMaxResults
			}
			fetchContent := true
			if _, ok := params["fetch_content"]; ok {
				fetchContent = params.GetBool("fetch_content")
			}

			searchOptions := []ostype.SearchOption{
				ostype.WithPageSize(maxResults),
			}

			results, err := omnisearch.Search(query, searchOptions...)
			if err != nil {
				return nil, utils.Errorf("search failed: %v", err)
			}
			if len(results) == 0 {
				return "no results found for: " + query, nil
			}

			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("# Web Search: %s\n\nResults: %d\n\n", query, len(results)))

			for i, r := range results {
				idx := i + 1
				builder.WriteString(fmt.Sprintf("## %d. %s\n", idx, r.Title))
				builder.WriteString(fmt.Sprintf("URL: %s\n", r.URL))
				if r.Source != "" {
					builder.WriteString(fmt.Sprintf("Source: %s\n", r.Source))
				}
				if r.Content != "" {
					builder.WriteString(fmt.Sprintf("Snippet: %s\n", r.Content))
				}

				if fetchContent && r.URL != "" {
					pageText := FetchPageContent(r.URL, defaultFetchTimeout)
					if pageText != "" {
						if len(pageText) > maxContentPerPage {
							pageText = pageText[:maxContentPerPage] + "\n...(truncated)"
						}
						builder.WriteString(fmt.Sprintf("\nPage Content:\n%s\n", pageText))
					}
				}
				builder.WriteString("\n---\n\n")
			}

			rawContent := builder.String()

			if compressor != nil && len(rawContent) > 1024 {
				compressed, compErr := compressor(ctx, rawContent, query, compressTargetBytes)
				if compErr != nil {
					log.Warnf("compress web search results failed: %v", compErr)
				} else if compressed != "" {
					snippets := buildSnippetSummary(results)
					return fmt.Sprintf("# Search Results for: %s\n\n## Quick Snippets\n%s\n\n## Refined Content\n%s",
						query, snippets, compressed), nil
				}
			}

			return rawContent, nil
		}),
	)
}

// CreateOmniSearchTools creates the basic web_search tool (without AI compression).
func CreateOmniSearchTools() ([]*aitool.Tool, error) {
	tool, err := CreateEnhancedWebSearchTool(nil)
	if err != nil {
		return nil, err
	}
	return []*aitool.Tool{tool}, nil
}

func buildSnippetSummary(results []*ostype.OmniSearchResult) string {
	var sb strings.Builder
	for i, r := range results {
		if r.Title != "" || r.Content != "" {
			sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   URL: %s\n", i+1, r.Title, r.Content, r.URL))
		}
	}
	return sb.String()
}

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
