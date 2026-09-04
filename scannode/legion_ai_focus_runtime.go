package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
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
	serverFocusCapabilityHTTPRequest         = "http.request"
	serverFocusCapabilityExtractReferences   = "web.extract_references"
	serverFocusCapabilitySubmitAsset         = "result.asset"
	serverFocusCapabilitySubmitRisk          = "result.risk"
	serverFocusCapabilitySourceWorkspaceInfo = "source.workspace.info"
	serverFocusCapabilitySourceList          = "source.list"
	serverFocusCapabilitySourceRead          = "source.read"
	serverFocusCapabilitySourceSearch        = "source.search"
	serverFocusCapabilitySubmitFindingV1     = "result.finding.v1"
	serverFocusCapabilitySubmitReportV1      = "result.report.v1"
	serverFocusCapabilityTaskStage           = "task.stage"
	// Temporary aliases for already-published pre-platform Releases.
	serverFocusCapabilitySubmitCodeFinding = "result.code_finding"
	serverFocusCapabilitySubmitCodeAudit   = "result.code_audit_report"
	serverFocusCapabilityProgressPhase     = "progress.phase"

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
	workspace  *legionCodeWorkspaceRuntime
	emitEvent  func(string, []byte)
	// A source workspace belongs to one server-authorized Focus Run. The
	// capability surface is dormant between Turns and is activated only while
	// the matching immutable Focus Release executes.
	authorizedFocusReleaseID string
	activeFocusReleaseID     string
	activeExecutionContract  *legionFocusExecutionContract
	activeFocusContext       context.Context
	activeFocusCancel        context.CancelFunc
	activeFocusGeneration    uint64

	mu               sync.Mutex
	requestCount     int
	ruleMu           sync.Mutex
	ruleCallCount    int
	ruleDebugHistory []legionSyntaxFlowDebugResult
	// Zero uses the immutable server defaults; nonzero values are used only
	// by local deterministic cancellation/budget tests, never model input.
	ruleDebugTimeout time.Duration
	ruleWorkLimit    int64
}

func newLegionServerFocusRuntime(
	ctx context.Context,
	authorizedTarget string,
	sink aiFocusResultSink,
	workspaces ...*legionCodeWorkspaceRuntime,
) (aicommon.LegionResultRuntime, error) {
	var workspace *legionCodeWorkspaceRuntime
	if len(workspaces) > 0 {
		workspace = workspaces[0]
	}
	if sink == nil || strings.TrimSpace(authorizedTarget) == "" {
		if workspace != nil {
			return nil, fmt.Errorf("source workspace runtime requires a result sink and authorized sentinel target")
		}
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
		sink:      assetSink,
		workspace: workspace,
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
	requestedCapability := strings.TrimSpace(capability)
	capability = normalizeLegionFocusCapability(requestedCapability)
	if r.workspace != nil {
		r.mu.Lock()
		active := strings.TrimSpace(r.activeFocusReleaseID)
		authorized := strings.TrimSpace(r.authorizedFocusReleaseID)
		contract := cloneLegionFocusExecutionContract(r.activeExecutionContract)
		r.mu.Unlock()
		if active == "" || authorized == "" || active != authorized {
			return nil, fmt.Errorf("source workspace capabilities are available only during the authorized Focus Turn")
		}
		if contract == nil || !contract.allowsCapability(capability) {
			return nil, fmt.Errorf("server focus capability %q is not allowed by the immutable Focus execution contract", requestedCapability)
		}
	}
	switch capability {
	case serverFocusCapabilityHTTPRequest:
		if r.workspace != nil {
			return nil, fmt.Errorf("http.request is disabled for source workspace sessions")
		}
		return r.executeHTTPRequest(params)
	case serverFocusCapabilityExtractReferences:
		if r.workspace != nil {
			return nil, fmt.Errorf("web.extract_references is disabled for source workspace sessions")
		}
		return r.extractReferences(params)
	case serverFocusCapabilitySubmitAsset:
		if r.workspace != nil {
			return nil, fmt.Errorf("result.asset is disabled for source workspace sessions")
		}
		return r.submitAsset(params)
	case serverFocusCapabilitySubmitRisk:
		if r.workspace != nil {
			return nil, fmt.Errorf("result.risk is disabled for source workspace sessions; use result.finding.v1")
		}
		return r.submitRisk(params)
	case serverFocusCapabilitySourceWorkspaceInfo:
		if r.workspace == nil {
			return nil, fmt.Errorf("source workspace is unavailable")
		}
		return r.workspace.info(), nil
	case serverFocusCapabilitySourceList:
		if r.workspace == nil {
			return nil, fmt.Errorf("source workspace is unavailable")
		}
		return r.workspace.list(params)
	case serverFocusCapabilitySourceRead:
		if r.workspace == nil {
			return nil, fmt.Errorf("source workspace is unavailable")
		}
		return r.workspace.read(params)
	case serverFocusCapabilitySourceSearch:
		if r.workspace == nil {
			return nil, fmt.Errorf("source workspace is unavailable")
		}
		return r.workspace.search(params)
	case serverFocusCapabilityOriginalSampleRead:
		return r.readSyntaxFlowOriginalSample(params)
	case serverFocusCapabilitySubmitFindingV1:
		return r.submitFindingV1(capability, params)
	case serverFocusCapabilitySubmitReportV1:
		return r.submitReportV1(capability, params)
	case serverFocusCapabilityRuleCheck:
		return r.checkSyntaxFlowRule(params)
	case serverFocusCapabilityRuleDebug:
		return r.debugSyntaxFlowRule(params)
	case serverFocusCapabilityRuleCandidate:
		return r.submitSyntaxFlowRuleCandidate(params)
	case serverFocusCapabilityTaskStage:
		return r.publishTaskStage(params)
	default:
		return nil, fmt.Errorf("unsupported server focus capability %q", capability)
	}
}

func (r *legionServerFocusRuntime) activateFocusTurn(releaseID string, contracts ...*legionFocusExecutionContract) error {
	if r == nil || r.workspace == nil {
		return nil
	}
	releaseID = strings.TrimSpace(releaseID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if releaseID == "" || releaseID != strings.TrimSpace(r.authorizedFocusReleaseID) {
		return fmt.Errorf("focus release is not authorized for the bound source workspace")
	}
	if r.activeFocusReleaseID != "" {
		return fmt.Errorf("a source workspace Focus Turn is already active")
	}
	var contract *legionFocusExecutionContract
	if len(contracts) > 0 {
		contract = cloneLegionFocusExecutionContract(contracts[0])
	}
	if contract == nil {
		return fmt.Errorf("source workspace Focus Turn requires an immutable execution contract")
	}
	if binder, ok := r.sink.(aiFocusExecutionContractBinder); ok {
		if err := binder.bindFocusExecutionContract(contract); err != nil {
			return err
		}
	}
	r.activeFocusReleaseID = releaseID
	r.activeExecutionContract = contract
	r.activeFocusGeneration++
	parentContext := r.ctx
	if parentContext == nil {
		parentContext = context.Background()
	}
	r.activeFocusContext, r.activeFocusCancel = context.WithCancel(parentContext)
	r.ruleCallCount = 0
	r.ruleDebugHistory = nil
	return nil
}

func (r *legionServerFocusRuntime) deactivateFocusTurn(releaseID string) {
	if r == nil || r.workspace == nil {
		return
	}
	r.mu.Lock()
	cleanup := false
	if r.activeFocusReleaseID == strings.TrimSpace(releaseID) {
		if r.activeFocusCancel != nil {
			r.activeFocusCancel()
		}
		r.activeFocusReleaseID = ""
		r.activeExecutionContract = nil
		r.activeFocusContext = nil
		r.activeFocusCancel = nil
		r.ruleCallCount = 0
		r.ruleDebugHistory = nil
		cleanup = true
	}
	r.mu.Unlock()
	if cleanup {
		_ = r.workspace.Cleanup()
	}
}

func (r *legionServerFocusRuntime) publishTaskStage(params map[string]any) (map[string]any, error) {
	if r.workspace == nil {
		return nil, fmt.Errorf("task.stage requires a source workspace")
	}
	if r.emitEvent == nil {
		return nil, fmt.Errorf("task.stage event publisher is unavailable")
	}
	phase := focusRuntimeString(params, "phase")
	status := strings.ToLower(focusRuntimeString(params, "status"))
	r.mu.Lock()
	contract := cloneLegionFocusExecutionContract(r.activeExecutionContract)
	r.mu.Unlock()
	if contract == nil || !contract.allowsStage(phase) {
		return nil, fmt.Errorf("task.stage phase is not part of the immutable Focus execution contract")
	}
	switch status {
	case "started", "progress", "completed", "failed":
	default:
		return nil, fmt.Errorf("task.stage status %q is unsupported", status)
	}
	payload := map[string]any{
		"workspace_id": r.workspace.spec.WorkspaceID,
		"phase":        phase,
		"status":       status,
	}
	if message := focusRuntimeRawString(params, "message"); message != "" {
		if len(message) > 2048 {
			return nil, fmt.Errorf("task.stage message exceeds 2048 bytes")
		}
		payload["message"] = message
	}
	if progress, ok := params["progress"]; ok {
		value := utils.InterfaceToFloat64(progress)
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return nil, fmt.Errorf("task.stage progress must be between 0 and 1")
		}
		payload["progress"] = value
	}
	r.emitEvent("task.stage", mustJSON(payload))
	return payload, nil
}

func normalizeLegionFocusCapability(capability string) string {
	switch strings.TrimSpace(capability) {
	case serverFocusCapabilitySubmitCodeFinding:
		return serverFocusCapabilitySubmitFindingV1
	case serverFocusCapabilitySubmitCodeAudit:
		return serverFocusCapabilitySubmitReportV1
	case serverFocusCapabilityProgressPhase:
		return serverFocusCapabilityTaskStage
	default:
		return strings.TrimSpace(capability)
	}
}

func (r *legionServerFocusRuntime) submitFindingV1(capability string, params map[string]any) (map[string]any, error) {
	if r.workspace == nil {
		return nil, fmt.Errorf("result.finding.v1 requires a source workspace")
	}
	r.mu.Lock()
	contract := cloneLegionFocusExecutionContract(r.activeExecutionContract)
	r.mu.Unlock()
	resultContract, ok := contract.resultForCapability(capability)
	if !ok {
		return nil, fmt.Errorf("result.finding.v1 has no immutable result contract")
	}
	sink, ok := r.sink.(aiFocusCodeResultSink)
	if !ok {
		return nil, fmt.Errorf("server focus result sink does not accept finding.v1")
	}
	finding := aiFocusCodeFinding{
		WorkspaceID:        r.workspace.spec.WorkspaceID,
		File:               focusRuntimeString(params, "file"),
		StartLine:          utils.InterfaceToInt(params["start_line"]),
		EndLine:            utils.InterfaceToInt(params["end_line"]),
		StartColumn:        utils.InterfaceToInt(params["start_column"]),
		EndColumn:          utils.InterfaceToInt(params["end_column"]),
		CWE:                focusRuntimeString(params, "cwe"),
		VulnerabilityType:  focusRuntimeString(params, "vulnerability_type"),
		Category:           focusRuntimeString(params, "category"),
		Module:             focusRuntimeString(params, "module"),
		Severity:           focusRuntimeString(params, "severity"),
		Confidence:         utils.InterfaceToFloat64(params["confidence"]),
		VerificationStatus: focusRuntimeString(params, "verification_status"),
		Title:              focusRuntimeString(params, "title"),
		Description:        focusRuntimeRawString(params, "description"),
		Evidence:           focusRuntimeRawString(params, "evidence"),
		DataFlow:           focusRuntimeRawString(params, "data_flow"),
		ExploitScenario:    focusRuntimeRawString(params, "exploit_scenario"),
		Recommendation:     focusRuntimeRawString(params, "recommendation"),
		DedupeKey:          focusRuntimeString(params, "dedupe_key"),
	}
	resolved, _, err := r.workspace.resolve(finding.File)
	if err != nil {
		return nil, fmt.Errorf("result.finding.v1 file: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("result.finding.v1 file must be a regular source file")
	}
	containsEndLine, err := legionCodeTextContainsLine(resolved, finding.EndLine)
	if err != nil {
		return nil, fmt.Errorf("result.finding.v1 inspect file: %w", err)
	}
	if finding.StartLine <= 0 || finding.EndLine < finding.StartLine || !containsEndLine {
		return nil, fmt.Errorf("result.finding.v1 line range is outside the source file")
	}
	receipt, err := sink.SubmitCodeFinding(r.ctx, resultContract.Kind, finding)
	if err != nil {
		return nil, err
	}
	return focusResultReceiptMap(receipt), nil
}

func (r *legionServerFocusRuntime) submitReportV1(capability string, params map[string]any) (map[string]any, error) {
	if r.workspace == nil {
		return nil, fmt.Errorf("result.report.v1 requires a source workspace")
	}
	r.mu.Lock()
	contract := cloneLegionFocusExecutionContract(r.activeExecutionContract)
	r.mu.Unlock()
	resultContract, ok := contract.resultForCapability(capability)
	if !ok {
		return nil, fmt.Errorf("result.report.v1 has no immutable result contract")
	}
	sink, ok := r.sink.(aiFocusCodeResultSink)
	if !ok {
		return nil, fmt.Errorf("server focus result sink does not accept report.v1")
	}
	summaryValue := params["structured_summary"]
	if summaryValue == nil {
		summaryValue = params["structured_summary_json"]
	}
	summary, err := marshalServerFocusPayload(summaryValue)
	if err != nil {
		return nil, fmt.Errorf("result.report.v1 structured_summary: %w", err)
	}
	receipt, err := sink.SubmitCodeAuditReport(r.ctx, resultContract.Kind, aiFocusCodeAuditReport{
		WorkspaceID:       r.workspace.spec.WorkspaceID,
		Title:             focusRuntimeRawString(params, "title"),
		Markdown:          focusRuntimeRawString(params, "markdown"),
		StructuredSummary: summary,
	})
	if err != nil {
		return nil, err
	}
	return focusResultReceiptMap(receipt), nil
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
