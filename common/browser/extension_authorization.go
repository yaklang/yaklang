package browser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const maxExtensionAuthorizationTaskBytes = 256 << 10

type extensionAuthorizationClientTaskParams struct {
	Schema  string          `json:"schema"`
	Payload json.RawMessage `json:"payload"`
}

type extensionAuthorizationTargetInput struct {
	DeviceID string `json:"deviceId,omitempty"`
	TabID    int    `json:"tabId"`
}

type extensionAuthorizationPairInput struct {
	Left  extensionAuthorizationTargetInput `json:"left"`
	Right extensionAuthorizationTargetInput `json:"right"`
}

type extensionAuthorizationInstance struct {
	DeviceID string                              `json:"deviceId"`
	Badge    string                              `json:"badge"`
	Current  bool                                `json:"current"`
	Tabs     []extensionAuthorizationInstanceTab `json:"tabs"`
	Error    string                              `json:"error,omitempty"`
}

type extensionAuthorizationInstanceTab struct {
	ID           int     `json:"id"`
	WindowID     int     `json:"windowId"`
	Active       bool    `json:"active,omitempty"`
	Title        string  `json:"title"`
	URL          string  `json:"url"`
	Incognito    bool    `json:"incognito"`
	LastAccessed float64 `json:"lastAccessed,omitempty"`
}

type extensionAuthorizationRequest struct {
	ID                     string  `json:"id"`
	Method                 string  `json:"method"`
	URL                    string  `json:"url"`
	ResourceType           string  `json:"resourceType"`
	StartedAt              float64 `json:"startedAt"`
	StatusCode             int     `json:"statusCode,omitempty"`
	RequestHeadersCaptured bool    `json:"requestHeadersCaptured"`
}

type extensionAuthorizationRequestExport struct {
	ID               string   `json:"id"`
	URL              string   `json:"url"`
	IsHTTPS          bool     `json:"isHttps"`
	RawRequestBase64 string   `json:"rawRequestBase64"`
	Limitations      []string `json:"limitations"`
}

type ExtensionAuthorizationSelector struct {
	ID       string `json:"id"`
	Location string `json:"location"`
	Path     string `json:"path"`
	Label    string `json:"label"`
}

type ExtensionAuthorizationPairInspection struct {
	Method        string                           `json:"method"`
	Route         string                           `json:"route"`
	SideEffect    bool                             `json:"sideEffect"`
	Selectors     []ExtensionAuthorizationSelector `json:"selectors"`
	Limitations   []string                         `json:"limitations"`
	BlockedReason string                           `json:"blockedReason,omitempty"`
}

type ExtensionAuthorizationCaseResult struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	Status        int     `json:"status"`
	StatusText    string  `json:"statusText"`
	Outcome       string  `json:"outcome"`
	DurationMS    float64 `json:"durationMs"`
	ContentType   string  `json:"contentType,omitempty"`
	BodyBytes     int     `json:"bodyBytes"`
	MatchesTarget bool    `json:"matchesTarget,omitempty"`
}

type ExtensionAuthorizationResult struct {
	Verdict     string                             `json:"verdict"`
	Summary     string                             `json:"summary"`
	Selector    ExtensionAuthorizationSelector     `json:"selector"`
	Cases       []ExtensionAuthorizationCaseResult `json:"cases"`
	Limitations []string                           `json:"limitations"`
}

type extensionAuthorizationCandidateValue struct {
	selector ExtensionAuthorizationSelector
	left     interface{}
	right    interface{}
}

type extensionAuthorizationExecuted struct {
	result      ExtensionAuthorizationCaseResult
	fingerprint string
}

func decodeExtensionAuthorizationJSON(raw json.RawMessage, output interface{}) error {
	if len(raw) == 0 {
		return errors.New("JSON payload is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON payload contains trailing data")
	}
	return nil
}

func extensionAuthorizationInstances(connections []ExtensionBridgeConnection, currentDeviceID string) []extensionAuthorizationInstance {
	instances := make([]extensionAuthorizationInstance, 0, len(connections))
	for _, connection := range connections {
		if connection.ManagedInstance == nil || connection.ManagedInstance.Manager != "ytray" {
			continue
		}
		instances = append(instances, extensionAuthorizationInstance{
			DeviceID: connection.DeviceID,
			Badge:    connection.ManagedInstance.Badge,
			Current:  connection.DeviceID == currentDeviceID,
			Tabs:     []extensionAuthorizationInstanceTab{},
		})
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].Badge < instances[j].Badge })
	return instances
}

func (s *ExtensionBridgeServer) listExtensionAuthorizationInstances(ctx context.Context, deviceID string) (interface{}, *ExtensionBridgeError) {
	if s.manager == nil {
		return nil, &ExtensionBridgeError{Code: "unavailable", Message: "browser extension bridge manager is not available"}
	}
	instances := extensionAuthorizationInstances(s.Connections(), deviceID)
	for index := range instances {
		raw, err := s.manager.CallDevice(ctx, instances[index].DeviceID, "browser.tabs", map[string]interface{}{})
		if err != nil {
			instances[index].Error = err.Error()
			continue
		}
		var tabs []extensionAuthorizationInstanceTab
		if err := json.Unmarshal(raw, &tabs); err != nil {
			instances[index].Error = "browser returned an invalid tab list"
			continue
		}
		for _, tab := range tabs {
			if len(instances[index].Tabs) >= 256 {
				break
			}
			if tab.ID > 0 && (strings.HasPrefix(tab.URL, "http://") || strings.HasPrefix(tab.URL, "https://")) {
				instances[index].Tabs = append(instances[index].Tabs, tab)
			}
		}
	}
	return map[string]interface{}{"instances": instances}, nil
}

func extensionAuthorizationOrigin(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func (s *ExtensionBridgeServer) extensionAuthorizationPair(
	ctx context.Context,
	callerDeviceID string,
	input extensionAuthorizationPairInput,
) (extensionAuthorizationPairInput, error) {
	input.Left.DeviceID = strings.TrimSpace(input.Left.DeviceID)
	input.Right.DeviceID = strings.TrimSpace(input.Right.DeviceID)
	if input.Left.DeviceID != "" && input.Left.DeviceID != callerDeviceID {
		return input, errors.New("browser A must be the calling extension")
	}
	input.Left.DeviceID = callerDeviceID
	if input.Right.DeviceID == "" || input.Left.DeviceID == input.Right.DeviceID {
		return input, errors.New("A/B must be two different YTray browser instances")
	}
	if input.Left.TabID <= 0 || input.Right.TabID <= 0 {
		return input, errors.New("A/B must each select an HTTP(S) tab")
	}

	connections := map[string]ExtensionBridgeConnection{}
	for _, connection := range s.Connections() {
		connections[connection.DeviceID] = connection
	}
	for _, target := range []extensionAuthorizationTargetInput{input.Left, input.Right} {
		connection, ok := connections[target.DeviceID]
		if !ok || connection.ManagedInstance == nil || connection.ManagedInstance.Manager != "ytray" {
			return input, errors.New("selected YTray browser instance is not connected")
		}
	}

	readTab := func(target extensionAuthorizationTargetInput) (extensionAuthorizationInstanceTab, error) {
		raw, err := s.manager.CallDevice(ctx, target.DeviceID, "browser.tabs", map[string]interface{}{})
		if err != nil {
			return extensionAuthorizationInstanceTab{}, err
		}
		var tabs []extensionAuthorizationInstanceTab
		if err := json.Unmarshal(raw, &tabs); err != nil {
			return extensionAuthorizationInstanceTab{}, errors.New("browser returned an invalid tab list")
		}
		for _, tab := range tabs {
			if tab.ID == target.TabID {
				return tab, nil
			}
		}
		return extensionAuthorizationInstanceTab{}, errors.New("selected browser tab is no longer available")
	}
	leftTab, err := readTab(input.Left)
	if err != nil {
		return input, fmt.Errorf("read browser A: %w", err)
	}
	rightTab, err := readTab(input.Right)
	if err != nil {
		return input, fmt.Errorf("read browser B: %w", err)
	}
	leftOrigin, rightOrigin := extensionAuthorizationOrigin(leftTab.URL), extensionAuthorizationOrigin(rightTab.URL)
	if leftOrigin == "" || leftOrigin != rightOrigin {
		return input, errors.New("A/B tabs must currently belong to the same HTTP(S) origin")
	}
	return input, nil
}

func (s *ExtensionBridgeServer) extensionAuthorizationCallTarget(
	ctx context.Context,
	target extensionAuthorizationTargetInput,
	method string,
	params map[string]interface{},
) (interface{}, error) {
	params["tabId"] = target.TabID
	params["frameId"] = 0
	raw, err := s.manager.CallDevice(ctx, target.DeviceID, method, params)
	if err != nil {
		return nil, err
	}
	var output interface{}
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, errors.New("browser returned invalid JSON")
	}
	return output, nil
}

func (s *ExtensionBridgeServer) extensionAuthorizationRequests(
	ctx context.Context,
	target extensionAuthorizationTargetInput,
	limit int,
) ([]extensionAuthorizationRequest, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		return nil, errors.New("request limit must be between 1 and 200")
	}
	raw, err := s.manager.CallDevice(ctx, target.DeviceID, "browser.network.list", map[string]interface{}{
		"tabId": target.TabID, "frameId": 0, "limit": limit,
	})
	if err != nil {
		return nil, err
	}
	var records []extensionAuthorizationRequest
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, errors.New("browser returned an invalid request list")
	}
	output := make([]extensionAuthorizationRequest, 0, len(records))
	for _, record := range records {
		method := strings.ToUpper(record.Method)
		if !record.RequestHeadersCaptured || record.ID == "" || extensionAuthorizationOrigin(record.URL) == "" {
			continue
		}
		switch method {
		case "GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE":
		default:
			continue
		}
		switch strings.ToLower(record.ResourceType) {
		case "image", "font", "stylesheet", "script", "media", "websocket":
			continue
		}
		record.Method = method
		output = append(output, record)
	}
	sort.Slice(output, func(i, j int) bool { return output[i].StartedAt > output[j].StartedAt })
	return output, nil
}

func (s *ExtensionBridgeServer) extensionAuthorizationExport(
	ctx context.Context,
	target extensionAuthorizationTargetInput,
	requestID string,
) (extensionAuthorizationRequestExport, []byte, error) {
	raw, err := s.manager.CallDevice(ctx, target.DeviceID, "browser.network.export", map[string]interface{}{
		"tabId": target.TabID, "frameId": 0, "id": strings.TrimSpace(requestID),
	})
	if err != nil {
		return extensionAuthorizationRequestExport{}, nil, err
	}
	var exported extensionAuthorizationRequestExport
	if err := json.Unmarshal(raw, &exported); err != nil {
		return exported, nil, errors.New("browser returned an invalid request export")
	}
	packet, err := base64.StdEncoding.DecodeString(exported.RawRequestBase64)
	if err != nil || len(packet) == 0 || len(packet) > 2<<20 {
		return exported, nil, errors.New("browser returned an invalid or oversized HTTP request")
	}
	return exported, packet, nil
}

func extensionAuthorizationSelectorID(location, path string) string {
	return location + ":" + base64.RawURLEncoding.EncodeToString([]byte(path))
}

func extensionAuthorizationSensitiveName(name string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "-", ""), "_", ""))
	for _, marker := range []string{"token", "auth", "cookie", "session", "csrf", "nonce", "signature", "secret", "password", "timestamp", "trace", "random", "salt"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "sign"
}

func extensionAuthorizationAddCandidate(
	output *[]extensionAuthorizationCandidateValue,
	location, path, label string,
	left, right interface{},
) {
	if len(*output) >= 50 || fmt.Sprint(left) == fmt.Sprint(right) || extensionAuthorizationSensitiveName(path) {
		return
	}
	*output = append(*output, extensionAuthorizationCandidateValue{
		selector: ExtensionAuthorizationSelector{
			ID: extensionAuthorizationSelectorID(location, path), Location: location, Path: path, Label: label,
		},
		left: left, right: right,
	})
}

func extensionAuthorizationJSONScalars(value interface{}, path string, depth int, output map[string]interface{}) {
	if depth > 8 || len(output) >= 200 {
		return
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			extensionAuthorizationJSONScalars(typed[key], path+"/"+strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1"), depth+1, output)
		}
	case []interface{}:
		for index, item := range typed {
			extensionAuthorizationJSONScalars(item, path+"/"+strconv.Itoa(index), depth+1, output)
		}
	case string, json.Number, float64:
		output[path] = typed
	}
}

func extensionAuthorizationCandidateValues(
	leftExport, rightExport extensionAuthorizationRequestExport,
	leftPacket, rightPacket []byte,
) (ExtensionAuthorizationPairInspection, []extensionAuthorizationCandidateValue, error) {
	leftMethod, leftURI, _ := lowhttp.GetHTTPPacketFirstLine(leftPacket)
	rightMethod, rightURI, _ := lowhttp.GetHTTPPacketFirstLine(rightPacket)
	leftMethod, rightMethod = strings.ToUpper(leftMethod), strings.ToUpper(rightMethod)
	if leftMethod == "" || leftMethod != rightMethod {
		return ExtensionAuthorizationPairInspection{}, nil, errors.New("A/B requests must use the same HTTP method")
	}
	leftURL, err := url.Parse(leftExport.URL)
	if err != nil {
		return ExtensionAuthorizationPairInspection{}, nil, errors.New("browser A request URL is invalid")
	}
	rightURL, err := url.Parse(rightExport.URL)
	if err != nil || extensionAuthorizationOrigin(leftExport.URL) != extensionAuthorizationOrigin(rightExport.URL) {
		return ExtensionAuthorizationPairInspection{}, nil, errors.New("A/B requests must belong to the same origin")
	}
	leftSegments := strings.Split(strings.Trim(leftURL.EscapedPath(), "/"), "/")
	rightSegments := strings.Split(strings.Trim(rightURL.EscapedPath(), "/"), "/")
	if len(leftSegments) != len(rightSegments) {
		return ExtensionAuthorizationPairInspection{}, nil, errors.New("A/B requests are not the same route")
	}
	candidates := make([]extensionAuthorizationCandidateValue, 0, 16)
	routeSegments := append([]string(nil), leftSegments...)
	for index := range leftSegments {
		leftValue, _ := url.PathUnescape(leftSegments[index])
		rightValue, _ := url.PathUnescape(rightSegments[index])
		if leftValue != rightValue {
			routeSegments[index] = ":value"
			if index > 0 && extensionAuthorizationSensitiveName(leftSegments[index-1]) {
				continue
			}
			extensionAuthorizationAddCandidate(&candidates, "path", strconv.Itoa(index), fmt.Sprintf("路径第 %d 段", index+1), leftValue, rightValue)
		}
	}
	leftQuery, rightQuery := leftURL.Query(), rightURL.Query()
	queryKeys := make([]string, 0, len(leftQuery))
	for key := range leftQuery {
		if _, ok := rightQuery[key]; ok {
			queryKeys = append(queryKeys, key)
		}
	}
	sort.Strings(queryKeys)
	for _, key := range queryKeys {
		extensionAuthorizationAddCandidate(&candidates, "query", key, "查询参数 "+key, leftQuery.Get(key), rightQuery.Get(key))
	}

	leftContentType := strings.ToLower(lowhttp.GetHTTPPacketHeader(leftPacket, "Content-Type"))
	rightContentType := strings.ToLower(lowhttp.GetHTTPPacketHeader(rightPacket, "Content-Type"))
	_, leftBody := lowhttp.SplitHTTPPacketFast(leftPacket)
	_, rightBody := lowhttp.SplitHTTPPacketFast(rightPacket)
	if strings.Contains(leftContentType, "application/json") && strings.Contains(rightContentType, "application/json") {
		decode := func(body []byte) map[string]interface{} {
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.UseNumber()
			var value interface{}
			if decoder.Decode(&value) != nil {
				return nil
			}
			output := map[string]interface{}{}
			extensionAuthorizationJSONScalars(value, "", 0, output)
			return output
		}
		leftValues, rightValues := decode(leftBody), decode(rightBody)
		keys := make([]string, 0, len(leftValues))
		for key := range leftValues {
			if _, ok := rightValues[key]; ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			extensionAuthorizationAddCandidate(&candidates, "json", key, "JSON "+key, leftValues[key], rightValues[key])
		}
	} else if strings.Contains(leftContentType, "application/x-www-form-urlencoded") && strings.Contains(rightContentType, "application/x-www-form-urlencoded") {
		leftForm, leftErr := url.ParseQuery(string(leftBody))
		rightForm, rightErr := url.ParseQuery(string(rightBody))
		if leftErr == nil && rightErr == nil {
			keys := make([]string, 0, len(leftForm))
			for key := range leftForm {
				if _, ok := rightForm[key]; ok {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				extensionAuthorizationAddCandidate(&candidates, "form", key, "表单字段 "+key, leftForm.Get(key), rightForm.Get(key))
			}
		}
	}
	if len(candidates) == 0 {
		return ExtensionAuthorizationPairInspection{}, nil, errors.New("同类请求中没有发现可安全交换的非认证字段；签名、加密或二进制请求请交给 Web Fuzzer")
	}
	selectors := make([]ExtensionAuthorizationSelector, len(candidates))
	for index := range candidates {
		selectors[index] = candidates[index].selector
	}
	limitations := append(append([]string{}, leftExport.Limitations...), rightExport.Limitations...)
	blockedReason := ""
	for _, limitation := range limitations {
		if strings.Contains(limitation, "只保留") || strings.Contains(limitation, "可能不完整") {
			blockedReason = "捕获的请求不完整，请改用 Web Fuzzer"
			break
		}
	}
	if strings.Contains(strings.ToLower(string(leftPacket)), "x-signature:") || strings.Contains(strings.ToLower(string(rightPacket)), "x-signature:") {
		limitations = append(limitations, "请求包含签名头；简单交换不会重新计算签名")
		blockedReason = "请求需要动态签名，请改用 Web Fuzzer"
	}
	route := "/" + strings.Join(routeSegments, "/")
	if leftURI == "" || rightURI == "" {
		return ExtensionAuthorizationPairInspection{}, nil, errors.New("A/B request line is invalid")
	}
	return ExtensionAuthorizationPairInspection{
		Method: leftMethod, Route: route, SideEffect: !extensionAuthorizationReadOnly(leftMethod),
		Selectors: selectors, Limitations: limitations, BlockedReason: blockedReason,
	}, candidates, nil
}

func extensionAuthorizationReadOnly(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func extensionAuthorizationSetJSONPointer(value interface{}, pointer string, replacement interface{}) error {
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	if pointer == "" || len(parts) == 0 {
		return errors.New("JSON selector cannot replace the root")
	}
	current := value
	for index, raw := range parts {
		part := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		last := index == len(parts)-1
		switch typed := current.(type) {
		case map[string]interface{}:
			if last {
				if _, ok := typed[part]; !ok {
					return errors.New("JSON selector no longer exists")
				}
				typed[part] = replacement
				return nil
			}
			next, ok := typed[part]
			if !ok {
				return errors.New("JSON selector no longer exists")
			}
			current = next
		case []interface{}:
			position, err := strconv.Atoi(part)
			if err != nil || position < 0 || position >= len(typed) {
				return errors.New("JSON selector array index is invalid")
			}
			if last {
				typed[position] = replacement
				return nil
			}
			current = typed[position]
		default:
			return errors.New("JSON selector crosses a scalar value")
		}
	}
	return errors.New("JSON selector is invalid")
}

func extensionAuthorizationReplace(packet []byte, rawURL string, candidate extensionAuthorizationCandidateValue, replacement interface{}) ([]byte, error) {
	value := fmt.Sprint(replacement)
	switch candidate.selector.Location {
	case "query":
		return lowhttp.ReplaceHTTPPacketQueryParam(packet, candidate.selector.Path, value), nil
	case "form":
		return lowhttp.ReplaceHTTPPacketPostParam(packet, candidate.selector.Path, value), nil
	case "path":
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return nil, err
		}
		segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
		index, err := strconv.Atoi(candidate.selector.Path)
		if err != nil || index < 0 || index >= len(segments) {
			return nil, errors.New("path selector is invalid")
		}
		segments[index] = url.PathEscape(value)
		path := "/" + strings.Join(segments, "/")
		return lowhttp.ReplaceHTTPPacketPathWithoutEncoding(packet, path), nil
	case "json":
		_, body := lowhttp.SplitHTTPPacketFast(packet)
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var document interface{}
		if err := decoder.Decode(&document); err != nil {
			return nil, errors.New("request JSON body is invalid")
		}
		if err := extensionAuthorizationSetJSONPointer(document, candidate.selector.Path, replacement); err != nil {
			return nil, err
		}
		rewritten, err := json.Marshal(document)
		if err != nil {
			return nil, err
		}
		return lowhttp.ReplaceHTTPPacketBody(packet, rewritten, false), nil
	default:
		return nil, errors.New("unsupported authorization selector")
	}
}

func extensionAuthorizationOutcome(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "success"
	case status == 401 || status == 403 || status == 404:
		return "denied"
	case status >= 300 && status < 400:
		return "redirect"
	case status >= 400 && status < 500:
		return "client-error"
	case status >= 500:
		return "server-error"
	default:
		return "opaque"
	}
}

func extensionAuthorizationExecuteRequest(ctx context.Context, id, label string, packet []byte, https bool) (extensionAuthorizationExecuted, error) {
	started := time.Now()
	response, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithPacketBytes(packet),
		lowhttp.WithHttps(https),
		lowhttp.WithContext(ctx),
		lowhttp.WithTimeout(30*time.Second),
		lowhttp.WithMaxContentLength(256*1024),
		lowhttp.WithSaveHTTPFlow(false),
		lowhttp.WithConnPool(false),
		lowhttp.WithRetryTimes(0),
		lowhttp.WithNoFixContentLength(true),
		lowhttp.WithNoReadMultiResponse(true),
	)
	if err != nil {
		return extensionAuthorizationExecuted{}, err
	}
	if response == nil || len(response.RawPacket) == 0 {
		return extensionAuthorizationExecuted{}, errors.New("authorization request returned an empty response")
	}
	status := lowhttp.GetStatusCodeFromResponse(response.RawPacket)
	_, _, statusText := lowhttp.GetHTTPPacketFirstLine(response.RawPacket)
	_, body := lowhttp.SplitHTTPPacketFast(response.RawPacket)
	if len(body) > 256*1024 {
		body = body[:256*1024]
	}
	sum := sha256.Sum256(body)
	return extensionAuthorizationExecuted{
		result: ExtensionAuthorizationCaseResult{
			ID: id, Label: label, Status: status, StatusText: statusText,
			Outcome: extensionAuthorizationOutcome(status), DurationMS: float64(time.Since(started).Microseconds()) / 1000,
			ContentType: lowhttp.GetHTTPPacketContentType(response.RawPacket), BodyBytes: len(body),
		},
		fingerprint: hex.EncodeToString(sum[:]),
	}, nil
}

func extensionAuthorizationVerdict(cases []extensionAuthorizationExecuted) (string, string) {
	if len(cases) < 2 || cases[0].result.Outcome != "success" || cases[1].result.Outcome != "success" {
		return "invalid-controls", "A/B 正常请求未同时成功，未发送交叉请求"
	}
	if len(cases) != 4 {
		return "inconclusive", "交叉请求未完整执行"
	}
	if cases[2].result.MatchesTarget || cases[3].result.MatchesTarget {
		return "suspected", "交叉响应与目标账号的正常响应完全一致，请结合业务权限确认"
	}
	if cases[2].result.Outcome == "denied" && cases[3].result.Outcome == "denied" {
		return "protected", "两个方向的交叉访问均被明确拒绝"
	}
	if cases[2].result.Outcome == "success" || cases[3].result.Outcome == "success" {
		return "possible", "至少一个交叉请求成功，但响应与目标对照不完全一致"
	}
	return "inconclusive", "交叉响应未形成稳定结论"
}

func (s *ExtensionBridgeServer) handleExtensionAuthorizationClientTask(
	ctx context.Context,
	deviceID string,
	params json.RawMessage,
) (interface{}, *ExtensionBridgeError) {
	if s.manager == nil {
		return nil, &ExtensionBridgeError{Code: "bridge_unavailable", Message: "browser extension bridge manager is not available"}
	}
	if len(params) == 0 || len(params) > maxExtensionAuthorizationTaskBytes {
		return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "authorization task is missing or too large"}
	}
	var task extensionAuthorizationClientTaskParams
	if err := decodeExtensionAuthorizationJSON(params, &task); err != nil {
		return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid authorization task: " + err.Error()}
	}
	task.Schema = strings.ToLower(strings.TrimSpace(task.Schema))
	fail := func(err error) (interface{}, *ExtensionBridgeError) {
		return nil, &ExtensionBridgeError{Code: "authorization_task_failed", Message: err.Error()}
	}

	switch task.Schema {
	case "authorization.capture.start", "authorization.capture.status", "authorization.capture.stop":
		var input struct {
			extensionAuthorizationPairInput
			Side string `json:"side"`
		}
		if err := decodeExtensionAuthorizationJSON(task.Payload, &input); err != nil {
			return fail(err)
		}
		pair, err := s.extensionAuthorizationPair(ctx, deviceID, input.extensionAuthorizationPairInput)
		if err != nil {
			return fail(err)
		}
		target := pair.Left
		if strings.ToLower(input.Side) == "right" {
			target = pair.Right
		} else if strings.ToLower(input.Side) != "left" {
			return fail(errors.New("capture side must be left or right"))
		}
		method := "browser.network.status"
		callParams := map[string]interface{}{}
		if task.Schema == "authorization.capture.start" {
			method = "browser.network.start"
			callParams = map[string]interface{}{"captureHeaders": true, "captureBody": true, "maxEntries": 200, "maxBodyBytes": 64 * 1024}
		} else if task.Schema == "authorization.capture.stop" {
			method = "browser.network.stop"
		}
		result, err := s.extensionAuthorizationCallTarget(ctx, target, method, callParams)
		if err != nil {
			return fail(err)
		}
		return result, nil

	case "authorization.requests":
		var input struct {
			extensionAuthorizationPairInput
			Side  string `json:"side"`
			Limit int    `json:"limit,omitempty"`
		}
		if err := decodeExtensionAuthorizationJSON(task.Payload, &input); err != nil {
			return fail(err)
		}
		pair, err := s.extensionAuthorizationPair(ctx, deviceID, input.extensionAuthorizationPairInput)
		if err != nil {
			return fail(err)
		}
		target := pair.Left
		if strings.ToLower(input.Side) == "right" {
			target = pair.Right
		} else if strings.ToLower(input.Side) != "left" {
			return fail(errors.New("request side must be left or right"))
		}
		requests, err := s.extensionAuthorizationRequests(ctx, target, input.Limit)
		if err != nil {
			return fail(err)
		}
		return requests, nil

	case "authorization.pair.inspect", "authorization.execute":
		var input struct {
			extensionAuthorizationPairInput
			LeftRequestID     string `json:"leftRequestId"`
			RightRequestID    string `json:"rightRequestId"`
			SelectorID        string `json:"selectorId,omitempty"`
			ApproveSideEffect bool   `json:"approveSideEffect,omitempty"`
		}
		if err := decodeExtensionAuthorizationJSON(task.Payload, &input); err != nil {
			return fail(err)
		}
		pair, err := s.extensionAuthorizationPair(ctx, deviceID, input.extensionAuthorizationPairInput)
		if err != nil {
			return fail(err)
		}
		leftExport, leftPacket, err := s.extensionAuthorizationExport(ctx, pair.Left, input.LeftRequestID)
		if err != nil {
			return fail(fmt.Errorf("export browser A request: %w", err))
		}
		rightExport, rightPacket, err := s.extensionAuthorizationExport(ctx, pair.Right, input.RightRequestID)
		if err != nil {
			return fail(fmt.Errorf("export browser B request: %w", err))
		}
		inspection, candidates, err := extensionAuthorizationCandidateValues(leftExport, rightExport, leftPacket, rightPacket)
		if err != nil {
			return fail(err)
		}
		if task.Schema == "authorization.pair.inspect" {
			return inspection, nil
		}
		if inspection.SideEffect && !input.ApproveSideEffect {
			return fail(errors.New("non-read-only requests require explicit user confirmation"))
		}
		if inspection.BlockedReason != "" {
			return fail(errors.New(inspection.BlockedReason))
		}
		var selected *extensionAuthorizationCandidateValue
		for index := range candidates {
			if candidates[index].selector.ID == input.SelectorID {
				selected = &candidates[index]
				break
			}
		}
		if selected == nil {
			return fail(errors.New("selected resource field is missing or stale"))
		}
		leftCross, err := extensionAuthorizationReplace(leftPacket, leftExport.URL, *selected, selected.right)
		if err != nil {
			return fail(err)
		}
		rightCross, err := extensionAuthorizationReplace(rightPacket, rightExport.URL, *selected, selected.left)
		if err != nil {
			return fail(err)
		}
		cases := make([]extensionAuthorizationExecuted, 0, 4)
		for _, item := range []struct {
			id, label string
			packet    []byte
			https     bool
		}{
			{"a-own", "A 访问自己的资源", leftPacket, leftExport.IsHTTPS},
			{"b-own", "B 访问自己的资源", rightPacket, rightExport.IsHTTPS},
		} {
			executed, err := extensionAuthorizationExecuteRequest(ctx, item.id, item.label, item.packet, item.https)
			if err != nil {
				return fail(fmt.Errorf("%s: %w", item.label, err))
			}
			cases = append(cases, executed)
		}
		if cases[0].result.Outcome == "success" && cases[1].result.Outcome == "success" {
			for _, item := range []struct {
				id, label         string
				packet            []byte
				https             bool
				targetFingerprint string
				targetBodyBytes   int
			}{
				{"a-to-b", "A 使用自己的登录态访问 B 的资源", leftCross, leftExport.IsHTTPS, cases[1].fingerprint, cases[1].result.BodyBytes},
				{"b-to-a", "B 使用自己的登录态访问 A 的资源", rightCross, rightExport.IsHTTPS, cases[0].fingerprint, cases[0].result.BodyBytes},
			} {
				executed, err := extensionAuthorizationExecuteRequest(ctx, item.id, item.label, item.packet, item.https)
				if err != nil {
					return fail(fmt.Errorf("%s: %w", item.label, err))
				}
				executed.result.MatchesTarget = executed.result.BodyBytes > 0 && item.targetBodyBytes > 0 && executed.fingerprint == item.targetFingerprint
				cases = append(cases, executed)
			}
		}
		verdict, summary := extensionAuthorizationVerdict(cases)
		results := make([]ExtensionAuthorizationCaseResult, len(cases))
		for index := range cases {
			results[index] = cases[index].result
		}
		return ExtensionAuthorizationResult{
			Verdict: verdict, Summary: summary, Selector: selected.selector,
			Cases: results, Limitations: inspection.Limitations,
		}, nil

	default:
		return nil, &ExtensionBridgeError{Code: "invalid_params", Message: "unsupported authorization task schema: " + task.Schema}
	}
}
