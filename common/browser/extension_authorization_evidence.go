package browser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/net/html"
)

const maxAuthorizationEvidencePacketBytes = 512 << 10

var (
	authorizationVolatileValuePattern = regexp.MustCompile(
		`(?i)^(?:[0-9]{10,16}|[0-9a-f]{8}-[0-9a-f-]{27,}|[0-9a-f]{24,}|[A-Za-z0-9_-]{32,})$`,
	)
	authorizationHTMLSpacePattern = regexp.MustCompile(`\s+`)
)

type ExtensionAuthorizationEvidenceComparison struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	LeftCaseID  string `json:"leftCaseId"`
	RightCaseID string `json:"rightCaseId"`
	Purpose     string `json:"purpose"`
}

type ExtensionAuthorizationEvidenceCase struct {
	ID                string                                 `json:"id"`
	Label             string                                 `json:"label"`
	AuthContextSide   string                                 `json:"authContextSide"`
	ResourceValueSide string                                 `json:"resourceValueSide"`
	State             string                                 `json:"state"`
	Status            int                                    `json:"status,omitempty"`
	Outcome           string                                 `json:"outcome,omitempty"`
	Timing            ExtensionAuthorizationRequestTiming    `json:"timing"`
	RequestAvailable  bool                                   `json:"requestAvailable"`
	ResponseAvailable bool                                   `json:"responseAvailable"`
	Response          *ExtensionAuthorizationResponseSummary `json:"response,omitempty"`
}

type ExtensionAuthorizationEvidenceBundle struct {
	Version         int                                        `json:"version"`
	WorkspaceID     string                                     `json:"workspaceId"`
	ExecutionID     string                                     `json:"executionId"`
	Mode            string                                     `json:"mode"`
	Verdict         string                                     `json:"verdict"`
	Confidence      string                                     `json:"confidence"`
	Cases           []ExtensionAuthorizationEvidenceCase       `json:"cases"`
	Comparisons     []ExtensionAuthorizationEvidenceComparison `json:"comparisons"`
	Semantic        []ExtensionAuthorizationCanaryEvidence     `json:"semantic"`
	Representations []string                                   `json:"representations"`
	ExpiresAt       int64                                      `json:"expiresAt"`
}

type ExtensionAuthorizationEvidenceInspectInput struct {
	WorkspaceID string `json:"workspaceId"`
	ExecutionID string `json:"executionId"`
}

type ExtensionAuthorizationEvidencePacketInput struct {
	WorkspaceID string `json:"workspaceId"`
	ExecutionID string `json:"executionId"`
	CaseID      string `json:"caseId"`
	Side        string `json:"side"`
	View        string `json:"view,omitempty"`
}

type ExtensionAuthorizationEvidencePacket struct {
	Version       int    `json:"version"`
	WorkspaceID   string `json:"workspaceId"`
	ExecutionID   string `json:"executionId"`
	CaseID        string `json:"caseId"`
	Side          string `json:"side"`
	View          string `json:"view"`
	PacketBase64  string `json:"packetBase64"`
	CapturedBytes int    `json:"capturedBytes"`
	Truncated     bool   `json:"truncated"`
}

type ExtensionAuthorizationEvidenceDiffInput struct {
	WorkspaceID string `json:"workspaceId"`
	ExecutionID string `json:"executionId"`
	LeftCaseID  string `json:"leftCaseId"`
	RightCaseID string `json:"rightCaseId"`
	Scope       string `json:"scope,omitempty"`
	View        string `json:"view,omitempty"`
}

type ExtensionAuthorizationEvidenceDiffEntry struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Left      string `json:"left,omitempty"`
	Right     string `json:"right,omitempty"`
	Volatile  bool   `json:"volatile"`
	Sensitive bool   `json:"sensitive"`
	Semantic  bool   `json:"semantic"`
}

type ExtensionAuthorizationEvidenceDiff struct {
	Version        int                                       `json:"version"`
	WorkspaceID    string                                    `json:"workspaceId"`
	ExecutionID    string                                    `json:"executionId"`
	LeftCaseID     string                                    `json:"leftCaseId"`
	RightCaseID    string                                    `json:"rightCaseId"`
	Scope          string                                    `json:"scope"`
	View           string                                    `json:"view"`
	Representation string                                    `json:"representation"`
	Equal          bool                                      `json:"equal"`
	Entries        []ExtensionAuthorizationEvidenceDiffEntry `json:"entries"`
	Omitted        int                                       `json:"omitted"`
}

type ExtensionAuthorizationEvidenceValidationInput struct {
	WorkspaceID string   `json:"workspaceId"`
	ExecutionID string   `json:"executionId"`
	Direction   string   `json:"direction"`
	Paths       []string `json:"paths"`
}

type ExtensionAuthorizationEvidenceValidation struct {
	Version        int                                    `json:"version"`
	WorkspaceID    string                                 `json:"workspaceId"`
	ExecutionID    string                                 `json:"executionId"`
	Direction      string                                 `json:"direction"`
	Verified       bool                                   `json:"verified"`
	Evidence       []ExtensionAuthorizationCanaryEvidence `json:"evidence"`
	RejectedPaths  []string                               `json:"rejectedPaths"`
	Verdict        string                                 `json:"verdict"`
	Confidence     string                                 `json:"confidence"`
	VerdictChanged bool                                   `json:"verdictChanged"`
	Reason         string                                 `json:"reason"`
}

func milliseconds(duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	return float64(duration.Microseconds()) / 1000
}

func extensionAuthorizationRequestTiming(
	response *lowhttp.LowhttpResponse,
	elapsed time.Duration,
) ExtensionAuthorizationRequestTiming {
	timing := ExtensionAuthorizationRequestTiming{TotalMS: milliseconds(elapsed)}
	if response == nil || response.TraceInfo == nil {
		return timing
	}
	trace := response.TraceInfo
	timing.DNSMS = milliseconds(trace.DNSTime)
	timing.ConnectMS = milliseconds(trace.ConnTime)
	timing.TLSMS = milliseconds(trace.TLSHandshakeTime)
	timing.TTFBMS = milliseconds(trace.ServerTime)
	if trace.TotalTime > 0 {
		timing.TotalMS = milliseconds(trace.TotalTime)
	}
	timing.TransferMS = timing.TotalMS - timing.TTFBMS
	if timing.TransferMS < 0 {
		timing.TransferMS = 0
	}
	return timing
}

func boundedAuthorizationEvidencePacket(packet []byte) ([]byte, bool) {
	if len(packet) <= maxAuthorizationEvidencePacketBytes {
		return append([]byte(nil), packet...), false
	}
	return append([]byte(nil), packet[:maxAuthorizationEvidencePacketBytes]...), true
}

func authorizationExecutionHasEvidenceBundle(
	execution *ExtensionAuthorizationExecution,
) bool {
	if execution == nil {
		return false
	}
	for index := range execution.Cases {
		result := execution.Cases[index].Result
		if result != nil &&
			(len(result.requestPacket) > 0 || len(result.responsePacket) > 0) {
			return true
		}
	}
	return false
}

func authorizationEvidenceComparisons(
	mode string,
) []ExtensionAuthorizationEvidenceComparison {
	if mode == "vertical" {
		return []ExtensionAuthorizationEvidenceComparison{
			{
				ID: "low-vs-privileged", Label: "低权限正常响应 ↔ 高权限正常响应",
				LeftCaseID: "low-control", RightCaseID: "privileged-baseline",
				Purpose: "control",
			},
			{
				ID: "probe-vs-privileged", Label: "低权限探测 ↔ 高权限正常响应",
				LeftCaseID: "low-privileged-probe", RightCaseID: "privileged-baseline",
				Purpose: "authorization",
			},
			{
				ID: "post-state", Label: "操作前状态 ↔ 操作后状态",
				LeftCaseID: "post-state-before", RightCaseID: "post-state-after",
				Purpose: "state-change",
			},
		}
	}
	return []ExtensionAuthorizationEvidenceComparison{
		{
			ID: "controls", Label: "身份 A 自有资源 ↔ 身份 B 自有资源",
			LeftCaseID: "a-own", RightCaseID: "b-own", Purpose: "control",
		},
		{
			ID: "a-to-b", Label: "A 访问 B ↔ B 自有资源",
			LeftCaseID: "a-to-b", RightCaseID: "b-own", Purpose: "authorization",
		},
		{
			ID: "b-to-a", Label: "B 访问 A ↔ A 自有资源",
			LeftCaseID: "b-to-a", RightCaseID: "a-own", Purpose: "authorization",
		},
	}
}

func (m *ExtensionBridgeManager) authorizationEvidenceExecution(
	ctx context.Context,
	workspaceID, executionID string,
) (ExtensionAuthorizationWorkspace, *ExtensionAuthorizationExecution, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	executionID = strings.TrimSpace(executionID)
	if workspaceID == "" || executionID == "" {
		return ExtensionAuthorizationWorkspace{}, nil, errors.New(
			"authorization evidence workspaceId and executionId are required",
		)
	}
	workspace, err := m.GetExtensionAuthorizationWorkspace(ctx, workspaceID, false)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, nil, err
	}
	if workspace.Execution == nil || workspace.Execution.ID != executionID {
		return ExtensionAuthorizationWorkspace{}, nil, errors.New(
			"authorization evidence execution was not found in the workspace",
		)
	}
	if !authorizationExecutionHasEvidenceBundle(workspace.Execution) {
		return ExtensionAuthorizationWorkspace{}, nil, errors.New(
			"authorization execution no longer has an evidence bundle",
		)
	}
	return workspace, workspace.Execution, nil
}

func (m *ExtensionBridgeManager) InspectExtensionAuthorizationEvidence(
	ctx context.Context,
	input ExtensionAuthorizationEvidenceInspectInput,
) (ExtensionAuthorizationEvidenceBundle, error) {
	workspace, execution, err := m.authorizationEvidenceExecution(
		ctx,
		input.WorkspaceID,
		input.ExecutionID,
	)
	if err != nil {
		return ExtensionAuthorizationEvidenceBundle{}, err
	}
	cases := make([]ExtensionAuthorizationEvidenceCase, 0, len(execution.Cases))
	present := make(map[string]struct{}, len(execution.Cases))
	for index := range execution.Cases {
		item := execution.Cases[index]
		view := ExtensionAuthorizationEvidenceCase{
			ID: item.ID, Label: item.Label,
			AuthContextSide:   item.AuthContextSide,
			ResourceValueSide: item.ResourceValueSide,
			State:             item.State,
		}
		present[item.ID] = struct{}{}
		if item.Result != nil {
			response := item.Result.Response
			view.Status = item.Result.Status
			view.Outcome = item.Result.Outcome
			view.Timing = item.Result.Timing
			view.RequestAvailable = len(item.Result.requestPacket) > 0
			view.ResponseAvailable = len(item.Result.responsePacket) > 0
			view.Response = &response
		}
		cases = append(cases, view)
	}
	comparisons := authorizationEvidenceComparisons(workspace.Mode)
	filtered := comparisons[:0]
	for _, comparison := range comparisons {
		if _, leftOK := present[comparison.LeftCaseID]; !leftOK {
			continue
		}
		if _, rightOK := present[comparison.RightCaseID]; !rightOK {
			continue
		}
		filtered = append(filtered, comparison)
	}
	semantic := append([]ExtensionAuthorizationCanaryEvidence{}, execution.Evidence...)
	return ExtensionAuthorizationEvidenceBundle{
		Version: 1, WorkspaceID: workspace.ID, ExecutionID: execution.ID,
		Mode: workspace.Mode, Verdict: execution.Verdict,
		Confidence: execution.Confidence, Cases: cases,
		Comparisons: filtered, Semantic: semantic,
		Representations: []string{
			"raw-packet",
			"bounded-decoded-body",
			"binary-summary",
			"structured-diff",
			"semantic-evidence",
		},
		ExpiresAt: workspace.ExpiresAt,
	}, nil
}

func authorizationEvidenceCase(
	execution *ExtensionAuthorizationExecution,
	caseID string,
) (*ExtensionAuthorizationCaseExecution, error) {
	caseID = strings.TrimSpace(caseID)
	if caseID == "" {
		return nil, errors.New("authorization evidence caseId is required")
	}
	item := authorizationExecutionCaseByID(execution, caseID)
	if item == nil || item.Result == nil {
		return nil, errors.New("authorization evidence case was not found or did not complete")
	}
	return item, nil
}

func authorizationSensitiveHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie",
		"x-csrf-token", "x-xsrf-token", "x-api-key", "api-key":
		return true
	default:
		return authorizationSensitivePath(name)
	}
}

func authorizationSensitivePath(path string) bool {
	normalized := strings.ToLower(path)
	for _, term := range []string{
		"password", "passwd", "secret", "token", "authorization", "cookie",
		"session", "csrf", "xsrf", "privatekey", "private-key", "credential",
		"signature", "api_key", "apikey", "access_key", "accesskey",
	} {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}

func redactAuthorizationRequestTarget(line string) string {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 || !strings.Contains(parts[1], "?") {
		return line
	}
	target, err := url.Parse(parts[1])
	if err != nil {
		return line
	}
	values := target.Query()
	changed := false
	for key := range values {
		if authorizationSensitivePath(key) {
			values.Set(key, "<redacted>")
			changed = true
		}
	}
	if !changed {
		return line
	}
	target.RawQuery = values.Encode()
	return parts[0] + " " + target.String() + " " + parts[2]
}

func redactAuthorizationPacket(packet []byte) []byte {
	header, body := lowhttp.SplitHTTPPacketFast(packet)
	lines := strings.Split(strings.ReplaceAll(string(header), "\r\n", "\n"), "\n")
	if len(lines) > 0 {
		lines[0] = redactAuthorizationRequestTarget(lines[0])
	}
	redactingContinuation := false
	for index := 1; index < len(lines); index++ {
		if strings.HasPrefix(lines[index], " ") ||
			strings.HasPrefix(lines[index], "\t") {
			if redactingContinuation {
				lines[index] = "\t<redacted>"
			}
			continue
		}
		redactingContinuation = false
		name, _, ok := strings.Cut(lines[index], ":")
		if ok && authorizationSensitiveHeader(name) {
			lines[index] = name + ": <redacted>"
			redactingContinuation = true
		}
	}
	redactedBody := redactAuthorizationStructuredBody(
		body,
		lowhttp.GetHTTPPacketContentType(packet),
	)
	separator := "\r\n\r\n"
	if len(body) == 0 {
		separator = "\r\n"
	}
	return append([]byte(strings.Join(lines, "\r\n")+separator), redactedBody...)
}

func redactAuthorizationStructuredBody(body []byte, contentType string) []byte {
	if len(body) == 0 {
		return nil
	}
	contentType = strings.ToLower(contentType)
	if strings.Contains(contentType, "json") {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var value interface{}
		if decoder.Decode(&value) == nil {
			var redact func(interface{}, string)
			redact = func(current interface{}, path string) {
				switch typed := current.(type) {
				case map[string]interface{}:
					for key, child := range typed {
						childPath := path + "." + key
						if authorizationSensitivePath(childPath) {
							typed[key] = "<redacted>"
							continue
						}
						redact(child, childPath)
					}
				case []interface{}:
					for index := range typed {
						redact(typed[index], fmt.Sprintf("%s[%d]", path, index))
					}
				}
			}
			redact(value, "body")
			if encoded, err := json.MarshalIndent(value, "", "  "); err == nil {
				return encoded
			}
		}
	}
	if strings.Contains(contentType, "x-www-form-urlencoded") {
		if values, err := url.ParseQuery(string(body)); err == nil {
			for key := range values {
				if authorizationSensitivePath(key) {
					values.Set(key, "<redacted>")
				}
			}
			return []byte(values.Encode())
		}
	}
	return append([]byte(nil), body...)
}

func (m *ExtensionBridgeManager) ReadExtensionAuthorizationEvidencePacket(
	ctx context.Context,
	input ExtensionAuthorizationEvidencePacketInput,
) (ExtensionAuthorizationEvidencePacket, error) {
	workspace, execution, err := m.authorizationEvidenceExecution(
		ctx,
		input.WorkspaceID,
		input.ExecutionID,
	)
	if err != nil {
		return ExtensionAuthorizationEvidencePacket{}, err
	}
	item, err := authorizationEvidenceCase(execution, input.CaseID)
	if err != nil {
		return ExtensionAuthorizationEvidencePacket{}, err
	}
	side := strings.ToLower(strings.TrimSpace(input.Side))
	view := strings.ToLower(strings.TrimSpace(input.View))
	if view == "" {
		view = "redacted"
	}
	if view != "redacted" && view != "raw" {
		return ExtensionAuthorizationEvidencePacket{}, errors.New(
			"authorization evidence packet view must be redacted or raw",
		)
	}
	var packet []byte
	var truncated bool
	switch side {
	case "request":
		packet = item.Result.requestPacket
		truncated = item.Result.requestTruncated
	case "response":
		packet = item.Result.responsePacket
		truncated = item.Result.responseTruncated
	default:
		return ExtensionAuthorizationEvidencePacket{}, errors.New(
			"authorization evidence packet side must be request or response",
		)
	}
	if len(packet) == 0 {
		return ExtensionAuthorizationEvidencePacket{}, errors.New(
			"authorization evidence packet is not available",
		)
	}
	capturedBytes := len(packet)
	if view == "redacted" {
		packet = redactAuthorizationPacket(packet)
	}
	return ExtensionAuthorizationEvidencePacket{
		Version: 1, WorkspaceID: workspace.ID, ExecutionID: execution.ID,
		CaseID: item.ID, Side: side, View: view,
		PacketBase64:  base64.StdEncoding.EncodeToString(packet),
		CapturedBytes: capturedBytes, Truncated: truncated,
	}, nil
}

func flattenAuthorizationResponseSemanticLeaves(
	body []byte,
	contentType string,
) map[string][]byte {
	contentType = strings.ToLower(contentType)
	switch {
	case strings.Contains(contentType, "json"):
		return flattenAuthorizationResponseJSON(body)
	case strings.Contains(contentType, "html") || looksLikeAuthorizationHTML(body):
		return flattenAuthorizationResponseHTML(body)
	case strings.Contains(contentType, "x-www-form-urlencoded"):
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil
		}
		output := make(map[string][]byte, len(values))
		for key, items := range values {
			for index, value := range items {
				output[fmt.Sprintf("body.form.%s[%d]", key, index)] = []byte(value)
			}
		}
		return output
	default:
		return nil
	}
}

func looksLikeAuthorizationHTML(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	return bytes.HasPrefix(bytes.ToLower(trimmed), []byte("<!doctype html")) ||
		bytes.HasPrefix(bytes.ToLower(trimmed), []byte("<html"))
}

func authorizationHTMLText(value string) string {
	value = authorizationHTMLSpacePattern.ReplaceAllString(value, " ")
	value = strings.TrimSpace(value)
	if len(value) > 4<<10 {
		value = value[:4<<10]
	}
	return value
}

func authorizationHTMLElementIdentity(node *html.Node, index int) string {
	tag := strings.ToLower(node.Data)
	for _, preferred := range []string{"id", "name", "data-testid", "aria-label"} {
		for _, attr := range node.Attr {
			if strings.EqualFold(attr.Key, preferred) {
				value := strings.Map(func(r rune) rune {
					if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
						return r
					}
					return '-'
				}, attr.Val)
				value = strings.Trim(value, "-")
				if value != "" {
					if len(value) > 64 {
						value = value[:64]
					}
					return tag + "[" + preferred + "=" + value + "]"
				}
			}
		}
	}
	return fmt.Sprintf("%s[%d]", tag, index)
}

func authorizationHTMLSemanticName(text string) string {
	normalized := strings.ToLower(text)
	terms := []struct {
		name    string
		aliases []string
	}{
		{"user_id", []string{"user id", "userid", "用户id", "用户编号"}},
		{"username", []string{"username", "user name", "用户名", "账号", "账户"}},
		{"email", []string{"email", "e-mail", "邮箱"}},
		{"owner", []string{"owner", "所有者", "归属人"}},
		{"tenant", []string{"tenant", "租户"}},
		{"organization", []string{"organization", "organisation", "组织", "机构"}},
		{"role", []string{"role", "角色", "权限"}},
		{"profile", []string{"profile", "用户资料", "个人资料"}},
		{"remark", []string{"remark", "note", "备注"}},
		{"order", []string{"order", "订单"}},
		{"resource", []string{"resource", "资源"}},
		{"project", []string{"project", "项目"}},
		{"workspace", []string{"workspace", "工作区"}},
	}
	for _, term := range terms {
		for _, alias := range term.aliases {
			if strings.Contains(normalized, alias) {
				return term.name
			}
		}
	}
	return ""
}

func flattenAuthorizationResponseHTML(body []byte) map[string][]byte {
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	output := make(map[string][]byte)
	semanticCounts := make(map[string]int)
	visited := 0
	var walk func(*html.Node, string, int)
	walk = func(node *html.Node, path string, depth int) {
		if node == nil || visited >= 1000 || len(output) >= 300 || depth > 16 {
			return
		}
		visited++
		if node.Type == html.ElementNode {
			tag := strings.ToLower(node.Data)
			switch tag {
			case "script", "style", "noscript", "template", "svg":
				return
			}
			for _, attr := range node.Attr {
				key := strings.ToLower(attr.Key)
				switch key {
				case "value", "href", "title", "aria-label", "data-user-id",
					"data-account-id", "data-owner-id", "data-resource-id":
					value := authorizationHTMLText(attr.Val)
					if value != "" && len(value) <= 8<<10 {
						output[path+".@"+key] = []byte(value)
					}
				}
			}
		}
		if node.Type == html.TextNode {
			value := authorizationHTMLText(node.Data)
			if value != "" && utf8.ValidString(value) {
				output[path+".text"] = []byte(value)
			}
			return
		}
		childIndex := 0
		var combined strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.TextNode {
				combined.WriteString(" ")
				combined.WriteString(child.Data)
			}
			childPath := path
			if child.Type == html.ElementNode {
				childPath += "." + authorizationHTMLElementIdentity(child, childIndex)
				childIndex++
			}
			walk(child, childPath, depth+1)
		}
		directText := authorizationHTMLText(combined.String())
		if semantic := authorizationHTMLSemanticName(directText); semantic != "" &&
			directText != "" && len(directText) <= 8<<10 {
			index := semanticCounts[semantic]
			semanticCounts[semantic] = index + 1
			output[fmt.Sprintf("body.dom.semantic.%s[%d]", semantic, index)] = []byte(directText)
		}
	}
	walk(root, "body.dom", 0)
	return output
}

func authorizationDiffValue(value []byte, sensitive bool, raw bool) string {
	if value == nil {
		return ""
	}
	if sensitive && !raw {
		return "<redacted>"
	}
	output := string(value)
	if len(output) > 4<<10 {
		output = output[:4<<10] + "…"
	}
	return output
}

func authorizationVolatileDiff(path string, value []byte) bool {
	normalized := strings.ToLower(path)
	if strings.Contains(normalized, ".headers.date[") ||
		strings.Contains(normalized, ".headers.expires[") ||
		strings.Contains(normalized, ".headers.last-modified[") {
		return true
	}
	compact := strings.NewReplacer(
		"-", "",
		"_", "",
		"[", "",
		"]", "",
	).Replace(normalized)
	for _, term := range []string{
		"timestamp", "createdat", "updatedat", "requestid", "traceid",
		"correlationid", "nonce", "duration", "elapsed", "servertime",
	} {
		if strings.Contains(compact, term) {
			return true
		}
	}
	// Stable identity/resource paths commonly contain UUIDs, ObjectIDs, or other
	// opaque identifiers. Their shape alone must not make them timing noise.
	if authorizationResponseCanaryPath(path) {
		return false
	}
	return authorizationVolatileValuePattern.Match(bytes.TrimSpace(value))
}

func flattenAuthorizationEvidenceRawPacket(
	packet []byte,
	prefix string,
	bodyOverride ...[]byte,
) map[string][]byte {
	header, body := lowhttp.SplitHTTPPacketFast(packet)
	if len(bodyOverride) > 0 {
		body = bodyOverride[0]
	}
	lines := strings.Split(strings.ReplaceAll(string(header), "\r\n", "\n"), "\n")
	output := make(map[string][]byte, len(lines)+2)
	if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		output[prefix+".start-line"] = []byte(strings.TrimSpace(lines[0]))
	}
	counts := make(map[string]int)
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		name = strings.Map(func(character rune) rune {
			if unicode.IsLetter(character) ||
				unicode.IsDigit(character) ||
				character == '-' ||
				character == '_' {
				return character
			}
			return '-'
		}, name)
		if name == "" {
			continue
		}
		index := counts[name]
		counts[name] = index + 1
		output[fmt.Sprintf("%s.headers.%s[%d]", prefix, name, index)] =
			[]byte(strings.TrimSpace(value))
	}
	if len(body) > 0 {
		contentType := lowhttp.GetHTTPPacketContentType(packet)
		if authorizationTextResponseBody(body, contentType) {
			output[prefix+".body.raw"] = append([]byte(nil), body...)
		} else {
			digest := sha256.Sum256(body)
			output[prefix+".body.binary.bytes"] = []byte(strconv.Itoa(len(body)))
			output[prefix+".body.binary.sha256"] = []byte(fmt.Sprintf("sha256:%x", digest))
		}
	}
	return output
}

func authorizationEvidenceDiffLeaves(
	result *ExtensionAuthorizationRequestExecution,
	scope string,
	redacted bool,
) (map[string][]byte, string) {
	if scope == "request" {
		if result == nil || len(result.requestPacket) == 0 {
			return nil, "raw"
		}
		contentType := lowhttp.GetHTTPPacketContentType(result.requestPacket)
		_, body := lowhttp.SplitHTTPPacketFast(result.requestPacket)
		if leaves := flattenAuthorizationResponseSemanticLeaves(body, contentType); len(leaves) > 0 {
			return leaves, "structured"
		}
		packet := result.requestPacket
		if redacted {
			packet = redactAuthorizationPacket(packet)
		}
		return flattenAuthorizationEvidenceRawPacket(packet, "request"), "raw"
	}
	if result == nil || len(result.responsePacket) == 0 {
		return nil, "raw"
	}
	body := result.responseBody
	if len(body) == 0 && result.Response.AnalysisState != "encoded-unavailable" {
		_, body = lowhttp.SplitHTTPPacketFast(result.responsePacket)
	}
	if leaves := flattenAuthorizationResponseSemanticLeaves(body, result.Response.ContentType); len(leaves) > 0 {
		return leaves, "structured"
	}
	packet := result.responsePacket
	if redacted {
		packet = redactAuthorizationPacket(packet)
	}
	leaves := flattenAuthorizationEvidenceRawPacket(packet, "response", body)
	if result.Response.AnalysisRepresentation == "binary" {
		delete(leaves, "response.body.binary.sha256")
		leaves["response.body.binary.fingerprint"] = []byte(result.Response.ValueFingerprint)
	}
	if result.Response.AnalysisState == "encoded-unavailable" {
		leaves["response.body.encoded.state"] = []byte(result.Response.AnalysisState)
		leaves["response.body.encoded.bytes"] = []byte(strconv.Itoa(result.Response.CapturedBytes))
		leaves["response.body.encoded.fingerprint"] = []byte(result.Response.ValueFingerprint)
		if result.Response.ContentEncoding != "" {
			leaves["response.body.encoded.content-encoding"] = []byte(result.Response.ContentEncoding)
		}
	}
	return leaves, "raw"
}

func (m *ExtensionBridgeManager) DiffExtensionAuthorizationEvidence(
	ctx context.Context,
	input ExtensionAuthorizationEvidenceDiffInput,
) (ExtensionAuthorizationEvidenceDiff, error) {
	workspace, execution, err := m.authorizationEvidenceExecution(
		ctx,
		input.WorkspaceID,
		input.ExecutionID,
	)
	if err != nil {
		return ExtensionAuthorizationEvidenceDiff{}, err
	}
	left, err := authorizationEvidenceCase(execution, input.LeftCaseID)
	if err != nil {
		return ExtensionAuthorizationEvidenceDiff{}, err
	}
	right, err := authorizationEvidenceCase(execution, input.RightCaseID)
	if err != nil {
		return ExtensionAuthorizationEvidenceDiff{}, err
	}
	scope := strings.ToLower(strings.TrimSpace(input.Scope))
	if scope == "" {
		scope = "response"
	}
	if scope != "request" && scope != "response" {
		return ExtensionAuthorizationEvidenceDiff{}, errors.New(
			"authorization evidence diff scope must be request or response",
		)
	}
	view := strings.ToLower(strings.TrimSpace(input.View))
	if view == "" {
		view = "redacted"
	}
	if view != "redacted" && view != "raw" {
		return ExtensionAuthorizationEvidenceDiff{}, errors.New(
			"authorization evidence diff view must be redacted or raw",
		)
	}
	leftLeaves, leftRepresentation := authorizationEvidenceDiffLeaves(
		left.Result,
		scope,
		view == "redacted",
	)
	rightLeaves, rightRepresentation := authorizationEvidenceDiffLeaves(
		right.Result,
		scope,
		view == "redacted",
	)
	representation := "structured"
	if leftRepresentation == "raw" || rightRepresentation == "raw" {
		representation = "raw"
	}
	paths := make([]string, 0, len(leftLeaves)+len(rightLeaves))
	seen := make(map[string]struct{}, len(leftLeaves)+len(rightLeaves))
	for path := range leftLeaves {
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	for path := range rightLeaves {
		if _, exists := seen[path]; !exists {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	entries := make([]ExtensionAuthorizationEvidenceDiffEntry, 0, len(paths))
	for _, path := range paths {
		leftValue, leftOK := leftLeaves[path]
		rightValue, rightOK := rightLeaves[path]
		kind := "changed"
		switch {
		case !leftOK:
			kind = "added"
		case !rightOK:
			kind = "removed"
		case bytes.Equal(leftValue, rightValue):
			continue
		}
		sensitive := authorizationSensitivePath(path)
		volatile := authorizationVolatileDiff(path, leftValue) ||
			authorizationVolatileDiff(path, rightValue)
		entries = append(entries, ExtensionAuthorizationEvidenceDiffEntry{
			Path: path, Kind: kind,
			Left:     authorizationDiffValue(leftValue, sensitive, view == "raw"),
			Right:    authorizationDiffValue(rightValue, sensitive, view == "raw"),
			Volatile: volatile, Sensitive: sensitive,
			Semantic: authorizationResponseCanaryPath(path) && !volatile && !sensitive,
		})
	}
	diffRank := func(entry ExtensionAuthorizationEvidenceDiffEntry) int {
		switch {
		case entry.Semantic:
			return 0
		case !entry.Volatile && !entry.Sensitive:
			return 1
		case entry.Sensitive:
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(entries, func(left, right int) bool {
		leftRank := diffRank(entries[left])
		rightRank := diffRank(entries[right])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return entries[left].Path < entries[right].Path
	})
	omitted := 0
	if len(entries) > 300 {
		omitted = len(entries) - 300
		entries = entries[:300]
	}
	return ExtensionAuthorizationEvidenceDiff{
		Version: 1, WorkspaceID: workspace.ID, ExecutionID: execution.ID,
		LeftCaseID: left.ID, RightCaseID: right.ID,
		Scope: scope, View: view, Representation: representation,
		Equal:   len(entries) == 0 && omitted == 0,
		Entries: entries, Omitted: omitted,
	}, nil
}

func validAuthorizationEvidenceValidationPath(path string) bool {
	path = strings.TrimSpace(path)
	if len(path) < 5 || len(path) > 512 || !strings.HasPrefix(path, "body.") {
		return false
	}
	for _, character := range path {
		if unicode.IsLetter(character) ||
			unicode.IsDigit(character) ||
			strings.ContainsRune("._-[]=@:", character) {
			continue
		}
		return false
	}
	return true
}

func authorizationEvidenceResultByID(
	execution *ExtensionAuthorizationExecution,
	caseID string,
) *ExtensionAuthorizationRequestExecution {
	item := authorizationExecutionCaseByID(execution, caseID)
	if item == nil {
		return nil
	}
	return item.Result
}

func appendUniqueAuthorizationEvidence(
	current []ExtensionAuthorizationCanaryEvidence,
	additions []ExtensionAuthorizationCanaryEvidence,
) []ExtensionAuthorizationCanaryEvidence {
	seen := make(map[string]struct{}, len(current)+len(additions))
	for _, evidence := range current {
		seen[evidence.Direction+"\x00"+evidence.Path+"\x00"+evidence.Source] = struct{}{}
	}
	for _, evidence := range additions {
		key := evidence.Direction + "\x00" + evidence.Path + "\x00" + evidence.Source
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		current = append(current, evidence)
	}
	return current
}

func (m *ExtensionBridgeManager) ValidateExtensionAuthorizationEvidence(
	ctx context.Context,
	input ExtensionAuthorizationEvidenceValidationInput,
) (ExtensionAuthorizationEvidenceValidation, error) {
	workspace, execution, err := m.authorizationEvidenceExecution(
		ctx,
		input.WorkspaceID,
		input.ExecutionID,
	)
	if err != nil {
		return ExtensionAuthorizationEvidenceValidation{}, err
	}
	input.Direction = strings.ToLower(strings.TrimSpace(input.Direction))
	if len(input.Paths) == 0 || len(input.Paths) > 16 {
		return ExtensionAuthorizationEvidenceValidation{}, errors.New(
			"authorization evidence validation requires between 1 and 16 paths",
		)
	}
	paths := make([]string, 0, len(input.Paths))
	rejected := make([]string, 0)
	seen := make(map[string]struct{}, len(input.Paths))
	for _, rawPath := range input.Paths {
		path := strings.TrimSpace(rawPath)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		if !validAuthorizationEvidenceValidationPath(path) {
			rejected = append(rejected, path)
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return ExtensionAuthorizationEvidenceValidation{}, errors.New(
			"authorization evidence validation did not contain a valid response path",
		)
	}

	evidence := make([]ExtensionAuthorizationCanaryEvidence, 0)
	allowVerdictUpgrade := false
	switch input.Direction {
	case "a-to-b":
		evidence = deriveAuthorizationCanaryEvidence(
			input.Direction,
			authorizationEvidenceResultByID(execution, "a-own"),
			authorizationEvidenceResultByID(execution, "b-own"),
			authorizationEvidenceResultByID(execution, "a-to-b"),
			workspace.comparisonKey,
			paths,
		)
		allowVerdictUpgrade = true
	case "b-to-a":
		evidence = deriveAuthorizationCanaryEvidence(
			input.Direction,
			authorizationEvidenceResultByID(execution, "b-own"),
			authorizationEvidenceResultByID(execution, "a-own"),
			authorizationEvidenceResultByID(execution, "b-to-a"),
			workspace.comparisonKey,
			paths,
		)
		allowVerdictUpgrade = true
	case "low-to-privileged":
		evidence = deriveVerticalAuthorizationCanaryEvidence(
			authorizationEvidenceResultByID(execution, "low-control"),
			authorizationEvidenceResultByID(execution, "privileged-baseline"),
			authorizationEvidenceResultByID(execution, "low-privileged-probe"),
			workspace.comparisonKey,
			paths,
		)
	case "post-state":
		evidence = deriveVerticalAuthorizationPostStateEvidence(
			authorizationEvidenceResultByID(execution, "post-state-before"),
			authorizationEvidenceResultByID(execution, "post-state-after"),
			workspace.comparisonKey,
			paths,
		)
		allowVerdictUpgrade = true
	default:
		return ExtensionAuthorizationEvidenceValidation{}, errors.New(
			"authorization evidence validation direction is not supported",
		)
	}
	requestedPaths := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		requestedPaths[path] = struct{}{}
	}
	filteredEvidence := evidence[:0]
	for _, item := range evidence {
		if _, requested := requestedPaths[item.Path]; requested {
			filteredEvidence = append(filteredEvidence, item)
		}
	}
	evidence = filteredEvidence
	if evidence == nil {
		evidence = make([]ExtensionAuthorizationCanaryEvidence, 0)
	}
	verifiedPaths := make(map[string]struct{}, len(evidence))
	for _, item := range evidence {
		verifiedPaths[item.Path] = struct{}{}
	}
	for _, path := range paths {
		if _, verified := verifiedPaths[path]; !verified {
			rejected = append(rejected, path)
		}
	}
	changed := false
	if len(evidence) > 0 {
		execution.Evidence = appendUniqueAuthorizationEvidence(execution.Evidence, evidence)
		if allowVerdictUpgrade &&
			(execution.Verdict == "likely" || execution.Verdict == "inconclusive") {
			execution.Verdict = "confirmed"
			execution.Confidence = "high"
			execution.Reasons = append(
				execution.Reasons,
				fmt.Sprintf(
					"用户或 Agent 提出的 %d 个业务路径已由确定性差分验证",
					len(evidence),
				),
			)
			changed = true
		}
		workspace.Execution = execution
		if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
			return ExtensionAuthorizationEvidenceValidation{}, err
		}
	}
	reason := "所选路径没有同时满足“双方正常值不同且交叉值精确等于目标身份”的条件"
	if len(evidence) > 0 {
		reason = fmt.Sprintf(
			"%d 个路径通过确定性业务归属验证；原始值未写入验证结果",
			len(evidence),
		)
		if !allowVerdictUpgrade {
			reason += "；纵向操作响应本身不会替代独立后置状态证据"
		}
	}
	return ExtensionAuthorizationEvidenceValidation{
		Version: 1, WorkspaceID: workspace.ID, ExecutionID: execution.ID,
		Direction: input.Direction, Verified: len(evidence) > 0,
		Evidence: evidence, RejectedPaths: rejected,
		Verdict: execution.Verdict, Confidence: execution.Confidence,
		VerdictChanged: changed, Reason: reason,
	}, nil
}
