package loop_infosec_recon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	boundedSaaSReconMaxPages        = 12
	boundedSaaSReconMaxScripts      = 8
	boundedSaaSReconMaxRequests     = 20
	boundedSaaSReconMaxDepth        = 2
	boundedSaaSReconMaxBodyBytes    = 512 * 1024
	boundedSaaSReconRequestTimeout  = 5 * time.Second
	boundedSaaSReconCollectedJSDir  = "saas-collected-js"
	boundedSaaSReconStaticFileLimit = 8
)

var boundedSaaSQuotedEndpointPattern = regexp.MustCompile(
	"(?i)[\\\"'`](https?://[^\\s\\\"'`<>]+|/[A-Za-z0-9][^\\s\\\"'`<>]{0,511})[\\\"'`]",
)

type boundedSaaSReconFinding struct {
	URL        string
	Method     string
	Source     string
	Evidence   string
	Confidence float64
}

type boundedSaaSReconCrawlStats struct {
	Pages         int
	Scripts       int
	Requests      int
	Candidates    int
	VerifiedJSDir string
}

type boundedSaaSReconQueueItem struct {
	URL   string
	Depth int
}

type boundedSaaSReconDocumentLink struct {
	URL       string
	Method    string
	IsScript  bool
	Crawlable bool
}

func newBoundedSaaSReconHTTPClient() *http.Client {
	return &http.Client{
		Timeout: boundedSaaSReconRequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func crawlBoundedSaaSRecon(
	ctx context.Context,
	client *http.Client,
	seedURL string,
	workDir string,
) (boundedSaaSReconCrawlStats, []boundedSaaSReconFinding, error) {
	stats := boundedSaaSReconCrawlStats{
		VerifiedJSDir: filepath.Join(workDir, boundedSaaSReconCollectedJSDir),
	}
	if client == nil {
		client = newBoundedSaaSReconHTTPClient()
	}
	seedURL, err := normalizeAuthorizedSaaSReconTarget(seedURL)
	if err != nil {
		return stats, nil, err
	}
	if err := os.MkdirAll(stats.VerifiedJSDir, 0o755); err != nil {
		return stats, nil, err
	}

	queue := []boundedSaaSReconQueueItem{{URL: seedURL}}
	queuedPages := map[string]bool{seedURL: true}
	queuedScripts := make(map[string]bool)
	var scriptURLs []string
	var findings []boundedSaaSReconFinding
	inlineScriptIndex := 0

	for len(queue) > 0 && stats.Pages < boundedSaaSReconMaxPages && stats.Requests < boundedSaaSReconMaxRequests {
		item := queue[0]
		queue = queue[1:]
		body, contentType, statusCode, fetchErr := fetchBoundedSaaSReconURL(ctx, client, seedURL, item.URL)
		stats.Requests++
		if fetchErr != nil {
			if stats.Pages == 0 {
				return stats, findings, fetchErr
			}
			continue
		}
		if statusCode < http.StatusOK || statusCode >= http.StatusBadRequest {
			continue
		}
		stats.Pages++
		if !strings.Contains(strings.ToLower(contentType), "html") && !looksLikeHTMLDocument(body) {
			continue
		}

		links, inlineScripts := extractBoundedSaaSReconDocumentLinks(body)
		for _, inlineScript := range inlineScripts {
			if strings.TrimSpace(inlineScript) == "" || inlineScriptIndex >= boundedSaaSReconMaxScripts {
				continue
			}
			inlineScriptIndex++
			name := fmt.Sprintf("inline_%03d.js", inlineScriptIndex)
			if err := os.WriteFile(filepath.Join(stats.VerifiedJSDir, name), []byte(inlineScript), 0o600); err == nil {
				stats.Scripts++
			}
		}
		for _, link := range links {
			resolved, ok := resolveBoundedSaaSReconURL(seedURL, item.URL, link.URL)
			if !ok {
				continue
			}
			if link.IsScript {
				if !queuedScripts[resolved] && len(scriptURLs) < boundedSaaSReconMaxScripts {
					queuedScripts[resolved] = true
					scriptURLs = append(scriptURLs, resolved)
				}
				continue
			}
			if !isBoundedSaaSReconStaticResource(resolved) {
				findings = append(findings, boundedSaaSReconFinding{
					URL:        resolved,
					Method:     link.Method,
					Source:     "saas_crawl",
					Evidence:   "same-origin link or form discovered from " + item.URL,
					Confidence: 0.72,
				})
			}
			if link.Crawlable && item.Depth < boundedSaaSReconMaxDepth && !queuedPages[resolved] && isBoundedSaaSReconCrawlablePage(resolved) {
				queuedPages[resolved] = true
				queue = append(queue, boundedSaaSReconQueueItem{URL: resolved, Depth: item.Depth + 1})
			}
		}
	}

	for _, scriptURL := range scriptURLs {
		if stats.Requests >= boundedSaaSReconMaxRequests || stats.Scripts >= boundedSaaSReconMaxScripts {
			break
		}
		body, _, statusCode, fetchErr := fetchBoundedSaaSReconURL(ctx, client, seedURL, scriptURL)
		stats.Requests++
		if fetchErr != nil || statusCode < http.StatusOK || statusCode >= http.StatusBadRequest {
			continue
		}
		name := fmt.Sprintf("external_%03d.js", stats.Scripts+1)
		if err := os.WriteFile(filepath.Join(stats.VerifiedJSDir, name), body, 0o600); err != nil {
			continue
		}
		stats.Scripts++
	}

	findings = dedupeBoundedSaaSReconFindings(findings)
	stats.Candidates = len(findings)
	return stats, findings, nil
}

func extractBoundedSaaSReconStaticFindings(
	seedURL string,
	verifiedJSDir string,
) ([]boundedSaaSReconFinding, int, error) {
	entries, err := os.ReadDir(verifiedJSDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var findings []boundedSaaSReconFinding
	filesRead := 0
	for _, entry := range entries {
		if entry.IsDir() || filesRead >= boundedSaaSReconStaticFileLimit {
			continue
		}
		path := filepath.Join(verifiedJSDir, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(file, boundedSaaSReconMaxBodyBytes))
		_ = file.Close()
		if readErr != nil {
			continue
		}
		filesRead++
		for _, match := range boundedSaaSQuotedEndpointPattern.FindAllSubmatch(body, -1) {
			if len(match) < 2 || !looksLikeBoundedSaaSReconEndpoint(string(match[1])) {
				continue
			}
			resolved, ok := resolveBoundedSaaSReconURL(seedURL, seedURL, string(match[1]))
			if !ok || isBoundedSaaSReconStaticResource(resolved) {
				continue
			}
			findings = append(findings, boundedSaaSReconFinding{
				URL:        resolved,
				Method:     http.MethodGet,
				Source:     "saas_js_static",
				Evidence:   "same-origin endpoint literal extracted from " + entry.Name(),
				Confidence: 0.82,
			})
		}
	}
	return dedupeBoundedSaaSReconFindings(findings), filesRead, nil
}

func mergeBoundedSaaSReconFindings(
	pool *APIPool,
	seedURL string,
	scopeHosts string,
	findings []boundedSaaSReconFinding,
) (int, []string) {
	rows := make([]struct {
		URL        string
		Method     string
		Source     string
		Evidence   string
		Confidence float64
	}, 0, len(findings))
	for _, finding := range findings {
		rows = append(rows, struct {
			URL        string
			Method     string
			Source     string
			Evidence   string
			Confidence float64
		}{
			URL:        finding.URL,
			Method:     finding.Method,
			Source:     finding.Source,
			Evidence:   finding.Evidence,
			Confidence: finding.Confidence,
		})
	}
	return MergeFindings(pool, seedURL, rows, scopeHosts)
}

func filterBoundedSaaSReconPool(pool *APIPool, seedURL string) {
	if pool == nil {
		return
	}
	filtered := pool.Entries[:0]
	for _, entry := range pool.Entries {
		if sameBoundedSaaSReconOrigin(seedURL, entry.NormalizedURL) {
			filtered = append(filtered, entry)
		}
	}
	pool.Entries = filtered
}

func fetchBoundedSaaSReconURL(
	ctx context.Context,
	client *http.Client,
	seedURL string,
	targetURL string,
) ([]byte, string, int, error) {
	if !sameBoundedSaaSReconOrigin(seedURL, targetURL) {
		return nil, "", 0, fmt.Errorf("SaaS recon URL is outside the authorized origin")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.Header.Set("Accept", "text/html,application/javascript,text/javascript;q=0.9,*/*;q=0.1")
	req.Header.Set("User-Agent", "IRify-SaaS-Recon/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, boundedSaaSReconMaxBodyBytes))
	if err != nil {
		return nil, "", resp.StatusCode, err
	}
	return body, resp.Header.Get("Content-Type"), resp.StatusCode, nil
}

func extractBoundedSaaSReconDocumentLinks(body []byte) ([]boundedSaaSReconDocumentLink, []string) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, nil
	}
	var links []boundedSaaSReconDocumentLink
	var inlineScripts []string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "a":
				if value := htmlNodeAttribute(node, "href"); value != "" {
					links = append(links, boundedSaaSReconDocumentLink{URL: value, Method: http.MethodGet, Crawlable: true})
				}
			case "iframe":
				if value := htmlNodeAttribute(node, "src"); value != "" {
					links = append(links, boundedSaaSReconDocumentLink{URL: value, Method: http.MethodGet, Crawlable: true})
				}
			case "form":
				if value := htmlNodeAttribute(node, "action"); value != "" {
					method := strings.ToUpper(htmlNodeAttribute(node, "method"))
					if method == "" {
						method = http.MethodGet
					}
					links = append(links, boundedSaaSReconDocumentLink{URL: value, Method: method})
				}
			case "script":
				if value := htmlNodeAttribute(node, "src"); value != "" {
					links = append(links, boundedSaaSReconDocumentLink{URL: value, Method: http.MethodGet, IsScript: true})
				} else if node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
					inlineScripts = append(inlineScripts, node.FirstChild.Data)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return links, inlineScripts
}

func htmlNodeAttribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func resolveBoundedSaaSReconURL(seedURL, baseURL, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(strings.ToLower(raw), "javascript:") || strings.HasPrefix(strings.ToLower(raw), "mailto:") {
		return "", false
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", false
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(ref)
	resolved.Fragment = ""
	resolved.User = nil
	value := resolved.String()
	if len(value) > maxAuthorizedSaaSReconTargetBytes || !sameBoundedSaaSReconOrigin(seedURL, value) {
		return "", false
	}
	return value, true
}

func sameBoundedSaaSReconOrigin(seedURL, candidateURL string) bool {
	seed, seedErr := url.Parse(strings.TrimSpace(seedURL))
	candidate, candidateErr := url.Parse(strings.TrimSpace(candidateURL))
	if seedErr != nil || candidateErr != nil || seed == nil || candidate == nil {
		return false
	}
	if !strings.EqualFold(seed.Scheme, candidate.Scheme) {
		return false
	}
	return strings.EqualFold(seed.Hostname(), candidate.Hostname()) && effectiveURLPort(seed) == effectiveURLPort(candidate)
}

func effectiveURLPort(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	if port := parsed.Port(); port != "" {
		return port
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return "443"
	}
	if strings.EqualFold(parsed.Scheme, "http") {
		return "80"
	}
	return ""
}

func looksLikeHTMLDocument(body []byte) bool {
	prefix := strings.ToLower(strings.TrimSpace(string(body)))
	return strings.HasPrefix(prefix, "<!doctype html") || strings.HasPrefix(prefix, "<html")
}

func looksLikeBoundedSaaSReconEndpoint(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.Path)
	for _, prefix := range []string{"/api", "/rest", "/graphql", "/oauth", "/auth", "/login", "/admin", "/v1", "/v2", "/v3"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func isBoundedSaaSReconCrawlablePage(raw string) bool {
	return !isBoundedSaaSReconStaticResource(raw)
}

func isBoundedSaaSReconStaticResource(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return true
	}
	ext := strings.ToLower(filepath.Ext(parsed.Path))
	switch ext {
	case ".js", ".mjs", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".woff", ".woff2", ".ttf", ".eot", ".map", ".mp4", ".pdf", ".zip", ".gz":
		return true
	default:
		return false
	}
}

func dedupeBoundedSaaSReconFindings(findings []boundedSaaSReconFinding) []boundedSaaSReconFinding {
	seen := make(map[string]bool)
	result := make([]boundedSaaSReconFinding, 0, len(findings))
	for _, finding := range findings {
		method := strings.ToUpper(strings.TrimSpace(finding.Method))
		if method == "" {
			method = http.MethodGet
		}
		key := method + "\x00" + finding.URL
		if finding.URL == "" || seen[key] {
			continue
		}
		seen[key] = true
		finding.Method = method
		result = append(result, finding)
	}
	return result
}
