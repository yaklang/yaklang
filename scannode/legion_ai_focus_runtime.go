package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/net/html"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	serverFocusCapabilityHTTPRequest       = "http.request"
	serverFocusCapabilityExtractReferences = "web.extract_references"
	serverFocusCapabilitySubmitAsset       = "result.asset"
	serverFocusCapabilitySubmitRisk        = "result.risk"

	maxServerFocusRequests       = 32
	maxServerFocusResponseBytes  = 512 * 1024
	maxServerFocusDocumentBytes  = 512 * 1024
	maxServerFocusReferences     = 64
	maxServerFocusHeaderValueLen = 256
	serverFocusRequestTimeout    = 5 * time.Second
)

var serverFocusRequestHeaderAllowlist = map[string]struct{}{
	"accept":           {},
	"accept-language":  {},
	"user-agent":       {},
	"x-requested-with": {},
}

var serverFocusAPIReferencePattern = regexp.MustCompile(
	"(?i)[\"'`](?:https?://[^\\s\"'`<>]+|/(?:api|rest|graphql|oauth|auth|login|admin|v[0-9]+)(?:/[^\\s\"'`<>]*)?)[\"'`]",
)

// legionServerFocusRuntime is a generic capability boundary. It deliberately
// knows nothing about a particular Focus name or stage order; Legion-delivered
// Yak code owns that orchestration.
type legionServerFocusRuntime struct {
	ctx        context.Context
	authorized *url.URL
	client     *http.Client
	sink       aiFocusAssetResultSink

	mu           sync.Mutex
	requestCount int
}

func newLegionServerFocusRuntime(
	ctx context.Context,
	authorizedTarget string,
	sink aiFocusResultSink,
) (aicommon.LegionResultRuntime, error) {
	if sink == nil || strings.TrimSpace(authorizedTarget) == "" {
		return nil, nil
	}
	assetSink, ok := sink.(aiFocusAssetResultSink)
	if !ok {
		return nil, fmt.Errorf("server focus runtime requires an asset-capable result sink")
	}
	authorized, err := normalizeServerFocusURL(authorizedTarget)
	if err != nil {
		return nil, fmt.Errorf("server focus runtime target: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &legionServerFocusRuntime{
		ctx:        ctx,
		authorized: authorized,
		client: &http.Client{
			Timeout: serverFocusRequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		sink: assetSink,
	}, nil
}

func (r *legionServerFocusRuntime) AuthorizedTarget() string {
	if r == nil || r.authorized == nil {
		return ""
	}
	return r.authorized.String()
}

func (r *legionServerFocusRuntime) Execute(
	capability string,
	params map[string]any,
) (map[string]any, error) {
	if r == nil {
		return nil, fmt.Errorf("server focus runtime is unavailable")
	}
	if params == nil {
		params = map[string]any{}
	}
	switch strings.TrimSpace(capability) {
	case serverFocusCapabilityHTTPRequest:
		return r.executeHTTPRequest(params)
	case serverFocusCapabilityExtractReferences:
		return r.extractReferences(params)
	case serverFocusCapabilitySubmitAsset:
		return r.submitAsset(params)
	case serverFocusCapabilitySubmitRisk:
		return r.submitRisk(params)
	default:
		return nil, fmt.Errorf("unsupported server focus capability %q", capability)
	}
}

func (r *legionServerFocusRuntime) executeHTTPRequest(params map[string]any) (map[string]any, error) {
	target, err := r.resolveAuthorizedURL(focusRuntimeString(params, "url"))
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(focusRuntimeString(params, "method"))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return nil, fmt.Errorf("server focus HTTP method %q is not allowed", method)
	}
	if err := r.takeRequestBudget(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(r.ctx, method, target.String(), nil)
	if err != nil {
		return nil, err
	}
	if err := applyServerFocusHeaders(req, params["headers"]); err != nil {
		return nil, err
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/html,application/json,application/javascript,text/javascript;q=0.9,*/*;q=0.1")
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "IRify-Focus-Runtime/1.0")
	}

	response, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxServerFocusResponseBytes+1))
	if err != nil {
		return nil, err
	}
	truncated := len(body) > maxServerFocusResponseBytes
	if truncated {
		body = body[:maxServerFocusResponseBytes]
	}
	bodyText := strings.ToValidUTF8(string(body), "�")
	bodyHash := sha256.Sum256(body)
	requestEvidence := renderServerFocusRequest(req)
	responseEvidence := renderServerFocusResponse(response, bodyText)
	return map[string]any{
		"url":               target.String(),
		"method":            method,
		"status_code":       response.StatusCode,
		"content_type":      response.Header.Get("Content-Type"),
		"content_length":    len(body),
		"body":              bodyText,
		"body_sha256":       hex.EncodeToString(bodyHash[:]),
		"body_truncated":    truncated,
		"request_evidence":  requestEvidence,
		"response_evidence": responseEvidence,
	}, nil
}

func (r *legionServerFocusRuntime) extractReferences(params map[string]any) (map[string]any, error) {
	document := focusRuntimeRawString(params, "document")
	if len(document) > maxServerFocusDocumentBytes {
		return nil, fmt.Errorf("server focus document exceeds %d bytes", maxServerFocusDocumentBytes)
	}
	base, err := r.resolveAuthorizedURL(focusRuntimeString(params, "base_url"))
	if err != nil {
		return nil, err
	}

	pageSet := make(map[string]struct{})
	scriptSet := make(map[string]struct{})
	add := func(raw string, isScript bool) {
		if len(pageSet)+len(scriptSet) >= maxServerFocusReferences {
			return
		}
		resolved, resolveErr := r.resolveAgainstBase(base, raw)
		if resolveErr != nil {
			return
		}
		value := resolved.String()
		if isScript || strings.HasSuffix(strings.ToLower(resolved.Path), ".js") || strings.HasSuffix(strings.ToLower(resolved.Path), ".mjs") {
			scriptSet[value] = struct{}{}
			return
		}
		if !serverFocusStaticResource(resolved.Path) {
			pageSet[value] = struct{}{}
		}
	}

	doc, parseErr := html.Parse(strings.NewReader(document))
	if parseErr == nil {
		var walk func(*html.Node)
		walk = func(node *html.Node) {
			if node.Type == html.ElementNode {
				tag := strings.ToLower(node.Data)
				for _, attr := range node.Attr {
					key := strings.ToLower(attr.Key)
					if (tag == "a" || tag == "link") && key == "href" {
						add(attr.Val, false)
					}
					if (tag == "script" || tag == "iframe") && key == "src" {
						add(attr.Val, tag == "script")
					}
					if tag == "form" && key == "action" {
						add(attr.Val, false)
					}
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		walk(doc)
	}
	for _, candidate := range serverFocusAPIReferencePattern.FindAllString(document, -1) {
		add(strings.Trim(candidate, "\"'`"), false)
	}

	pages := sortedStringSet(pageSet)
	scripts := sortedStringSet(scriptSet)
	return map[string]any{
		"base_url": base.String(),
		"pages":    pages,
		"scripts":  scripts,
		"count":    len(pages) + len(scripts),
	}, nil
}

func (r *legionServerFocusRuntime) submitAsset(params map[string]any) (map[string]any, error) {
	target, err := r.resolveAuthorizedURL(focusRuntimeString(params, "target"))
	if err != nil {
		return nil, err
	}
	payload, err := marshalServerFocusPayload(params["payload"])
	if err != nil {
		return nil, err
	}
	asset := aiFocusAssetResult{
		Kind:        focusRuntimeString(params, "kind"),
		Title:       focusRuntimeString(params, "title"),
		Target:      target.String(),
		IdentityKey: focusRuntimeString(params, "identity_key"),
		Payload:     payload,
	}
	receipt, err := r.sink.SubmitAsset(r.ctx, asset)
	if err != nil {
		return nil, err
	}
	return focusResultReceiptMap(receipt), nil
}

func (r *legionServerFocusRuntime) submitRisk(params map[string]any) (map[string]any, error) {
	if !utils.InterfaceToBoolean(params["verified"]) {
		return nil, fmt.Errorf("server focus risk requires verified evidence")
	}
	target, err := r.resolveAuthorizedURL(focusRuntimeString(params, "target"))
	if err != nil {
		return nil, err
	}
	requestEvidence := focusRuntimeRawString(params, "request_evidence")
	responseEvidence := focusRuntimeRawString(params, "response_evidence")
	description := focusRuntimeRawString(params, "description")
	details := focusRuntimeRawString(params, "details")
	if strings.TrimSpace(requestEvidence) == "" &&
		strings.TrimSpace(responseEvidence) == "" &&
		strings.TrimSpace(details) == "" &&
		strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("server focus risk requires structured evidence")
	}
	risk := &schema.Risk{
		Title:          focusRuntimeString(params, "title"),
		RiskType:       focusRuntimeString(params, "risk_type"),
		Severity:       focusRuntimeString(params, "severity"),
		Url:            target.String(),
		Parameter:      focusRuntimeString(params, "parameter"),
		Payload:        focusRuntimeRawString(params, "payload"),
		Description:    description,
		Solution:       focusRuntimeRawString(params, "solution"),
		Details:        details,
		QuotedRequest:  requestEvidence,
		QuotedResponse: responseEvidence,
	}
	receipt, err := r.sink.SubmitRisk(r.ctx, risk)
	if err != nil {
		return nil, err
	}
	return focusResultReceiptMap(receipt), nil
}

func (r *legionServerFocusRuntime) takeRequestBudget() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.requestCount >= maxServerFocusRequests {
		return fmt.Errorf("server focus request budget exhausted")
	}
	r.requestCount++
	return nil
}

func (r *legionServerFocusRuntime) resolveAuthorizedURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		raw = r.AuthorizedTarget()
	}
	return r.resolveAgainstBase(r.authorized, raw)
}

func (r *legionServerFocusRuntime) resolveAgainstBase(base *url.URL, raw string) (*url.URL, error) {
	if base == nil {
		return nil, fmt.Errorf("server focus authorized target is unavailable")
	}
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	resolved, err := normalizeServerFocusURL(base.ResolveReference(ref).String())
	if err != nil {
		return nil, err
	}
	if !sameServerFocusOrigin(r.authorized, resolved) {
		return nil, fmt.Errorf("server focus URL is outside the authorized origin")
	}
	return resolved, nil
}

func normalizeServerFocusURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 4096 {
		return nil, fmt.Errorf("invalid HTTP(S) URL")
	}
	for _, char := range raw {
		if unicode.IsControl(char) {
			return nil, fmt.Errorf("invalid HTTP(S) URL")
		}
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed == nil || parsed.Host == "" || parsed.Opaque != "" {
		return nil, fmt.Errorf("invalid HTTP(S) URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("URL must use HTTP(S)")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("URL cannot contain credentials or fragments")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return nil, fmt.Errorf("URL host is required")
	}
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(hostname, ":") {
		if port == "" {
			parsed.Host = "[" + hostname + "]"
		} else {
			parsed.Host = net.JoinHostPort(hostname, port)
		}
	} else if port == "" {
		parsed.Host = hostname
	} else {
		parsed.Host = net.JoinHostPort(hostname, port)
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed, nil
}

func sameServerFocusOrigin(left, right *url.URL) bool {
	if left == nil || right == nil || !strings.EqualFold(left.Scheme, right.Scheme) || !strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return serverFocusEffectivePort(left) == serverFocusEffectivePort(right)
}

func serverFocusEffectivePort(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	if port := parsed.Port(); port != "" {
		return port
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return "443"
	}
	return "80"
}

func normalizeServerFocusResultTarget(authorizedTarget, candidate string) (string, error) {
	authorized, err := normalizeServerFocusURL(authorizedTarget)
	if err != nil {
		return "", err
	}
	candidate = strings.TrimSpace(candidate)
	ref, err := parseServerFocusResultReference(authorized, candidate)
	if err != nil {
		return "", err
	}
	resolved, err := normalizeServerFocusURL(authorized.ResolveReference(ref).String())
	if err != nil {
		return "", err
	}
	if !sameServerFocusOrigin(authorized, resolved) {
		return "", fmt.Errorf("focus result target is outside the authorized origin")
	}
	return resolved.String(), nil
}

func parseServerFocusResultReference(authorized *url.URL, candidate string) (*url.URL, error) {
	if strings.HasPrefix(candidate, "/") {
		return url.Parse(candidate)
	}
	authorityEnd := strings.IndexAny(candidate, "/?#")
	if authorityEnd < 0 {
		authorityEnd = len(candidate)
	}
	authority := candidate[:authorityEnd]
	suffix := candidate[authorityEnd:]
	if strings.Count(authority, ":") > 1 && !strings.HasPrefix(authority, "[") {
		if net.ParseIP(authority) == nil {
			return url.Parse(candidate)
		}
		authority = "[" + authority + "]"
	}
	bare, normalizeErr := normalizeServerFocusURL(authorized.Scheme + "://" + authority + suffix)
	if normalizeErr == nil && isServerFocusNetworkHost(bare.Hostname(), authorized.Hostname()) {
		return bare, nil
	}
	return url.Parse(candidate)
}

func isServerFocusNetworkHost(hostname, authorizedHostname string) bool {
	return net.ParseIP(hostname) != nil ||
		strings.Contains(hostname, ".") ||
		strings.EqualFold(hostname, authorizedHostname)
}

func applyServerFocusHeaders(request *http.Request, raw any) error {
	headers := utils.InterfaceToMapInterface(raw)
	if len(headers) > len(serverFocusRequestHeaderAllowlist) {
		return fmt.Errorf("too many server focus request headers")
	}
	for name, value := range headers {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if _, ok := serverFocusRequestHeaderAllowlist[strings.ToLower(canonical)]; !ok {
			return fmt.Errorf("server focus request header %q is not allowed", name)
		}
		text := strings.TrimSpace(utils.InterfaceToString(value))
		if text == "" || len(text) > maxServerFocusHeaderValueLen || strings.ContainsAny(text, "\r\n\x00") {
			return fmt.Errorf("server focus request header %q has an invalid value", name)
		}
		request.Header.Set(canonical, text)
	}
	return nil
}

func renderServerFocusRequest(request *http.Request) string {
	var lines []string
	for name, values := range request.Header {
		for _, value := range values {
			lines = append(lines, name+": "+value)
		}
	}
	sort.Strings(lines)
	return fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\n%s\r\n\r\n", request.Method, request.URL.RequestURI(), request.Host, strings.Join(lines, "\r\n"))
}

func renderServerFocusResponse(response *http.Response, body string) string {
	var lines []string
	for _, name := range []string{"Content-Type", "Content-Length", "Location", "Server"} {
		if value := response.Header.Get(name); value != "" {
			lines = append(lines, name+": "+value)
		}
	}
	return fmt.Sprintf("HTTP/1.1 %s\r\n%s\r\n\r\n%s", response.Status, strings.Join(lines, "\r\n"), body)
}

func focusRuntimeString(params map[string]any, key string) string {
	return strings.TrimSpace(utils.InterfaceToString(params[key]))
}

func focusRuntimeRawString(params map[string]any, key string) string {
	value := params[key]
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return utils.InterfaceToString(value)
	}
	return string(raw)
}

func marshalServerFocusPayload(value any) ([]byte, error) {
	if text, ok := value.(string); ok && json.Valid([]byte(text)) {
		return []byte(text), nil
	}
	return json.Marshal(value)
}

func focusResultReceiptMap(receipt aiFocusResultReceipt) map[string]any {
	return map[string]any{
		"result_id":  receipt.ResultID,
		"dedupe_key": receipt.DedupeKey,
		"backend_id": receipt.BackendID,
	}
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func serverFocusStaticResource(path string) bool {
	lower := strings.ToLower(path)
	for _, suffix := range []string{".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".woff", ".woff2", ".ttf", ".map", ".pdf", ".zip", ".gz"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}
