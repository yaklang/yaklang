package yakgrpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/browsertools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const browserTransformMaxBodyBytes = 8 * 1024 * 1024
const browserTransformFailureRequestPrefix = "YAKIT_BROWSER_TRANSFORM_FAILED\n"

type browserTransformCaller interface {
	CallDevice(context.Context, string, string, interface{}) (json.RawMessage, error)
}

type browserTransformProfileDescriptor struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Origin  string `json:"origin"`
	Request struct {
		Enabled bool `json:"enabled"`
	} `json:"request"`
	Response struct {
		Enabled bool `json:"enabled"`
	} `json:"response"`
	Match struct {
		Methods    []string `json:"methods"`
		URLPattern string   `json:"urlPattern"`
	} `json:"match"`
}

type browserTransformHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type browserTransformPacket struct {
	Method     string                   `json:"method,omitempty"`
	URL        string                   `json:"url"`
	StatusCode int                      `json:"statusCode,omitempty"`
	Headers    []browserTransformHeader `json:"headers"`
	BodyBase64 string                   `json:"bodyBase64"`
}

type browserTransformCall struct {
	ProfileID    string                 `json:"profileId,omitempty"`
	ValidationID string                 `json:"validationId,omitempty"`
	Direction    string                 `json:"direction"`
	Packet       browserTransformPacket `json:"packet"`
}

type browserTransformResult struct {
	Explanation   json.RawMessage          `json:"explanation,omitempty"`
	ProofLevel    string                   `json:"proofLevel,omitempty"`
	ProfileID     string                   `json:"profileId"`
	Direction     string                   `json:"direction"`
	URL           string                   `json:"url"`
	BodyBase64    string                   `json:"bodyBase64"`
	SetHeaders    []browserTransformHeader `json:"setHeaders"`
	RemoveHeaders []string                 `json:"removeHeaders"`
	DurationMs    float64                  `json:"durationMs"`
}

func browserTransformEffectivePort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	if strings.EqualFold(value.Scheme, "http") {
		return "80"
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return ""
}

func applyBrowserTransformURL(packet []byte, original *url.URL, rawResult string) ([]byte, error) {
	transformed, err := url.Parse(strings.TrimSpace(rawResult))
	if err != nil || transformed == nil || !transformed.IsAbs() {
		return nil, errors.New("browser transform returned an invalid URL")
	}
	if transformed.User != nil || transformed.Fragment != "" {
		return nil, errors.New("browser transform URL may not contain userinfo or a fragment")
	}
	if !strings.EqualFold(transformed.Scheme, original.Scheme) ||
		!strings.EqualFold(transformed.Hostname(), original.Hostname()) ||
		browserTransformEffectivePort(transformed) != browserTransformEffectivePort(original) ||
		transformed.EscapedPath() != original.EscapedPath() {
		return nil, errors.New("browser transform may only change URL query parameters")
	}
	return lowhttp.SetHTTPPacketUrl(packet, transformed.String()), nil
}

type browserTransformTrace struct {
	PlainRequest  []byte
	WireRequest   []byte
	WireResponse  []byte
	PlainResponse []byte
}

type browserTransformRuntime struct {
	caller          browserTransformCaller
	deviceID        string
	profileID       string
	validationID    string
	profileName     string
	origin          string
	methods         []string
	urlPattern      string
	requestEnabled  bool
	responseEnabled bool
	timeout         time.Duration

	mu        sync.Mutex
	pending   map[[32]byte][]*browserTransformTrace
	completed map[[32]byte][]*browserTransformTrace
	evidence  map[string]interface{}
}

func prepareBrowserValidationTransform(
	caller browserTransformCaller,
	deviceID string,
	validationID string,
	requestEnabled bool,
	responseEnabled bool,
	timeout time.Duration,
) (*browserTransformRuntime, error) {
	deviceID = strings.TrimSpace(deviceID)
	validationID = strings.TrimSpace(validationID)
	if caller == nil {
		return nil, errors.New("browser transform gateway is unavailable: extension bridge is not running")
	}
	if deviceID == "" || validationID == "" {
		return nil, errors.New("browser transform validation requires a paired browser and validation_id")
	}
	if !requestEnabled && !responseEnabled {
		return nil, errors.New("at least one browser transform direction must be enabled")
	}
	if timeout < 2*time.Second {
		timeout = 2 * time.Second
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	return &browserTransformRuntime{
		caller:          caller,
		deviceID:        deviceID,
		profileID:       "transient-" + validationID,
		validationID:    validationID,
		profileName:     "Validated temporary transform",
		requestEnabled:  requestEnabled,
		responseEnabled: responseEnabled,
		timeout:         timeout,
		pending:         make(map[[32]byte][]*browserTransformTrace),
		completed:       make(map[[32]byte][]*browserTransformTrace),
	}, nil
}

func prepareBrowserTransform(
	ctx context.Context,
	caller browserTransformCaller,
	deviceID string,
	profileID string,
	timeout time.Duration,
) (*browserTransformRuntime, error) {
	deviceID = strings.TrimSpace(deviceID)
	profileID = strings.TrimSpace(profileID)
	if deviceID == "" && profileID == "" {
		return nil, nil
	}
	if caller == nil {
		return nil, errors.New("browser transform gateway is unavailable: extension bridge is not running")
	}
	if deviceID == "" || profileID == "" {
		return nil, errors.New("browser transform gateway requires both a paired browser and a transform profile")
	}
	if timeout < 2*time.Second {
		timeout = 2 * time.Second
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	raw, err := caller.CallDevice(callCtx, deviceID, "browser.transform.profile.list", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("browser transform preflight failed: %w", err)
	}
	var profiles []browserTransformProfileDescriptor
	if err := json.Unmarshal(raw, &profiles); err != nil {
		return nil, fmt.Errorf("decode browser transform profiles: %w", err)
	}
	var selected *browserTransformProfileDescriptor
	for index := range profiles {
		if profiles[index].ID == profileID {
			selected = &profiles[index]
			break
		}
	}
	if selected == nil {
		return nil, errors.New("browser transform profile is not available in the current shared browser document")
	}
	if !selected.Enabled {
		return nil, fmt.Errorf("browser transform profile %q is disabled", selected.Name)
	}
	if !selected.Request.Enabled && !selected.Response.Enabled {
		return nil, fmt.Errorf("browser transform profile %q has no enabled direction", selected.Name)
	}
	return &browserTransformRuntime{
		caller:          caller,
		deviceID:        deviceID,
		profileID:       profileID,
		profileName:     selected.Name,
		origin:          selected.Origin,
		methods:         append([]string(nil), selected.Match.Methods...),
		urlPattern:      selected.Match.URLPattern,
		requestEnabled:  selected.Request.Enabled,
		responseEnabled: selected.Response.Enabled,
		timeout:         timeout,
		pending:         make(map[[32]byte][]*browserTransformTrace),
		completed:       make(map[[32]byte][]*browserTransformTrace),
	}, nil
}

func cloneTransformPacket(packet []byte) []byte {
	return append([]byte(nil), packet...)
}

func (r *browserTransformRuntime) rememberRequest(plain, wire []byte) {
	trace := &browserTransformTrace{PlainRequest: cloneTransformPacket(plain), WireRequest: cloneTransformPacket(wire)}
	key := sha256.Sum256(wire)
	r.mu.Lock()
	r.pending[key] = append(r.pending[key], trace)
	r.mu.Unlock()
}

func (r *browserTransformRuntime) rememberResponse(wireRequest, wireResponse, plainResponse []byte) {
	key := sha256.Sum256(wireRequest)
	r.mu.Lock()
	var trace *browserTransformTrace
	if queue := r.pending[key]; len(queue) > 0 {
		trace = queue[0]
		if len(queue) == 1 {
			delete(r.pending, key)
		} else {
			r.pending[key] = queue[1:]
		}
	}
	if trace == nil {
		trace = &browserTransformTrace{WireRequest: cloneTransformPacket(wireRequest)}
	}
	trace.WireResponse = cloneTransformPacket(wireResponse)
	trace.PlainResponse = cloneTransformPacket(plainResponse)
	r.completed[key] = append(r.completed[key], trace)
	r.mu.Unlock()
}

func popTransformTrace(values map[[32]byte][]*browserTransformTrace, key [32]byte) *browserTransformTrace {
	queue := values[key]
	if len(queue) == 0 {
		return nil
	}
	trace := queue[0]
	if len(queue) == 1 {
		delete(values, key)
	} else {
		values[key] = queue[1:]
	}
	return trace
}

func (r *browserTransformRuntime) takeTrace(wireRequest []byte) *browserTransformTrace {
	if r == nil {
		return nil
	}
	key := sha256.Sum256(wireRequest)
	r.mu.Lock()
	defer r.mu.Unlock()
	if trace := popTransformTrace(r.completed, key); trace != nil {
		return trace
	}
	return popTransformTrace(r.pending, key)
}

func browserTransformHeaders(packet []byte) []browserTransformHeader {
	full := lowhttp.GetHTTPPacketHeadersFull(packet)
	keys := make([]string, 0, len(full))
	for key := range full {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) })
	result := make([]browserTransformHeader, 0, len(keys))
	for _, key := range keys {
		for _, value := range full[key] {
			result = append(result, browserTransformHeader{Name: key, Value: value})
		}
	}
	return result
}

func browserTransformPacketFromRequest(packet []byte, isHTTPS bool) (browserTransformPacket, error) {
	if len(packet) == 0 {
		return browserTransformPacket{}, errors.New("HTTP request is empty")
	}
	if len(lowhttp.GetHTTPPacketBody(packet)) > browserTransformMaxBodyBytes {
		return browserTransformPacket{}, errors.New("HTTP request body exceeds 8 MiB")
	}
	requestURL, err := lowhttp.ExtractURLFromHTTPRequestRaw(packet, isHTTPS)
	if err != nil {
		return browserTransformPacket{}, fmt.Errorf("parse HTTP request: %w", err)
	}
	if requestURL == nil {
		return browserTransformPacket{}, errors.New("parse HTTP request: URL is empty")
	}
	method, _, _ := lowhttp.GetHTTPPacketFirstLine(packet)
	return browserTransformPacket{
		Method:     method,
		URL:        requestURL.String(),
		Headers:    browserTransformHeaders(packet),
		BodyBase64: base64.StdEncoding.EncodeToString(lowhttp.GetHTTPPacketBody(packet)),
	}, nil
}

func normalizeBrowserTransformResponse(packet []byte) []byte {
	if strings.TrimSpace(lowhttp.GetHTTPPacketHeader(packet, "Content-Encoding")) == "" {
		return packet
	}
	if fixed, _, err := lowhttp.FixHTTPResponse(packet); err == nil && len(fixed) > 0 {
		return fixed
	}
	return packet
}

func (r *browserTransformRuntime) transformPacket(
	ctx context.Context,
	direction string,
	packet []byte,
	requestPacket []byte,
	isHTTPS bool,
) ([]byte, error) {
	if r == nil {
		return packet, nil
	}
	if len(packet) == 0 {
		return nil, errors.New("browser transform packet is empty")
	}
	working := packet
	if direction == "response" {
		working = normalizeBrowserTransformResponse(packet)
	}
	body := lowhttp.GetHTTPPacketBody(working)
	if len(body) > browserTransformMaxBodyBytes {
		return nil, errors.New("browser transform packet body exceeds 8 MiB")
	}
	method, _, _ := lowhttp.GetHTTPPacketFirstLine(requestPacket)
	requestURL, err := lowhttp.ExtractURLFromHTTPRequestRaw(requestPacket, isHTTPS)
	if err != nil || requestURL == nil {
		return nil, fmt.Errorf("resolve browser transform request URL: %w", err)
	}
	statusCode := 0
	if direction == "response" {
		_, rawStatus, _ := lowhttp.GetHTTPPacketFirstLine(working)
		statusCode, _ = strconv.Atoi(rawStatus)
	}
	input := browserTransformCall{
		ProfileID:    r.profileID,
		ValidationID: r.validationID,
		Direction:    direction,
		Packet: browserTransformPacket{
			Method: method, URL: requestURL.String(), StatusCode: statusCode,
			Headers:    browserTransformHeaders(working),
			BodyBase64: base64.StdEncoding.EncodeToString(body),
		},
	}
	capability := "browser.transform.execute"
	if r.validationID != "" {
		capability = "browser.transform.validation.execute"
		input.ProfileID = ""
	}
	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	raw, err := r.caller.CallDevice(callCtx, r.deviceID, capability, input)
	if err != nil {
		return nil, err
	}
	var result browserTransformResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode browser transform result: %w", err)
	}
	if result.ProfileID != r.profileID || result.Direction != direction {
		return nil, errors.New("browser transform result identity does not match the request")
	}
	transformedBody, err := base64.StdEncoding.DecodeString(result.BodyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode browser transform body: %w", err)
	}
	if len(transformedBody) > browserTransformMaxBodyBytes {
		return nil, errors.New("browser transform result body exceeds 8 MiB")
	}
	output := lowhttp.ReplaceHTTPPacketBody(working, transformedBody, false)
	for _, name := range result.RemoveHeaders {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n:") {
			return nil, errors.New("browser transform returned an invalid header removal")
		}
		output = lowhttp.DeleteHTTPPacketHeader(output, name)
	}
	for _, header := range result.SetHeaders {
		if strings.TrimSpace(header.Name) == "" || strings.ContainsAny(header.Name, "\r\n:") || strings.ContainsAny(header.Value, "\r\n") {
			return nil, errors.New("browser transform returned an invalid header")
		}
		output = lowhttp.ReplaceHTTPPacketHeader(output, header.Name, header.Value)
	}
	if direction == "request" {
		output, err = applyBrowserTransformURL(output, requestURL, result.URL)
		if err != nil {
			return nil, err
		}
	}
	if r.evidence != nil {
		r.mu.Lock()
		r.evidence[direction] = map[string]interface{}{"explanation": result.Explanation, "proofLevel": result.ProofLevel}
		r.mu.Unlock()
	}
	return output, nil
}

const browserAgentPacketResultLimit = 256 * 1024

func browserAgentPacketResult(packet []byte) map[string]interface{} {
	truncated := len(packet) > browserAgentPacketResultLimit
	if truncated {
		packet = packet[:browserAgentPacketResultLimit]
	}
	return map[string]interface{}{
		"raw":       utils.EscapeInvalidUTF8Byte(packet),
		"truncated": truncated,
	}
}

func executeBrowserAgentHTTPRequest(
	ctx context.Context,
	bridge browsertools.Bridge,
	params aitool.InvokeParams,
	runtimeConfig *aitool.ToolRuntimeConfig,
) (interface{}, error) {
	deviceID, browserRef, err := browsertools.ResolveBrowserDevice(
		bridge,
		strings.TrimSpace(params.GetString("device_id")),
		params.GetString("browser_ref"),
	)
	if err != nil {
		return nil, err
	}
	profileID := strings.TrimSpace(params.GetString("profile_id"))
	validationID := strings.TrimSpace(params.GetString("validation_id"))
	if (profileID == "") == (validationID == "") {
		return nil, errors.New("provide exactly one of profile_id or validation_id")
	}
	plainRequest := []byte(params.GetString("request"))
	isHTTPS := params.GetBool("is_https")
	requestPacket, err := browserTransformPacketFromRequest(plainRequest, isHTTPS)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(params.GetInt("timeout_seconds", 30)) * time.Second
	if timeout < 2*time.Second {
		timeout = 2 * time.Second
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}

	var runtime *browserTransformRuntime
	if profileID != "" {
		runtime, err = prepareBrowserTransform(ctx, bridge, deviceID, profileID, timeout)
	} else {
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		raw, metadataErr := bridge.CallDevice(callCtx, deviceID, "browser.transform.validation.get", map[string]interface{}{"validationId": validationID})
		if metadataErr != nil {
			return nil, fmt.Errorf("read browser validation directions: %w", metadataErr)
		}
		var metadata struct {
			ID         string `json:"id"`
			Directions struct {
				Request  bool `json:"request"`
				Response bool `json:"response"`
			} `json:"directions"`
		}
		if err := json.Unmarshal(raw, &metadata); err != nil || metadata.ID != validationID {
			return nil, errors.New("browser validation metadata does not match requested draft")
		}
		requestEnabled := metadata.Directions.Request
		responseEnabled := metadata.Directions.Response
		if params.Has("transform_request") {
			requestEnabled = params.GetBool("transform_request")
		}
		if params.Has("transform_response") {
			responseEnabled = params.GetBool("transform_response")
		}
		if requestEnabled && !metadata.Directions.Request || responseEnabled && !metadata.Directions.Response {
			return nil, errors.New("requested transform direction is not present in the validated gateway")
		}
		runtime, err = prepareBrowserValidationTransform(
			bridge,
			deviceID,
			validationID,
			requestEnabled,
			responseEnabled,
			timeout,
		)
	}
	if err != nil {
		return nil, err
	}
	runtime.evidence = make(map[string]interface{})
	wireRequest := runtime.beforeHook(ctx)(isHTTPS, nil, plainRequest)
	if reason := browserTransformRequestFailureReason(wireRequest); reason != "" {
		return nil, fmt.Errorf("browser request transform failed before network send: %s", reason)
	}
	runtimeID := ""
	if runtimeConfig != nil {
		runtimeID = strings.TrimSpace(runtimeConfig.RuntimeID)
	}
	response, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithPacketBytes(wireRequest),
		lowhttp.WithHttps(isHTTPS),
		lowhttp.WithContext(ctx),
		lowhttp.WithTimeout(timeout),
		lowhttp.WithMaxContentLength(browserTransformMaxBodyBytes),
		lowhttp.WithSaveHTTPFlow(false),
		lowhttp.WithConnPool(false),
		lowhttp.WithRetryTimes(0),
		lowhttp.WithNoReadMultiResponse(true),
	)
	if err != nil {
		return nil, fmt.Errorf("send transformed HTTP request: %w", err)
	}
	if response == nil || (len(response.BareResponse) == 0 && len(response.RawPacket) == 0) {
		return nil, errors.New("transformed HTTP request returned an empty response")
	}
	wireResponse := response.BareResponse
	if len(wireResponse) == 0 {
		wireResponse = response.RawPacket
	}
	plainResponse := runtime.afterHook(ctx)(isHTTPS, nil, wireRequest, nil, wireResponse)
	responseTransformFailure := browserTransformResponseFailureReason(runtime, plainResponse)
	displayResponse := plainResponse
	if responseTransformFailure != "" {
		// The request already reached the network. Keep the real response as
		// evidence even when the browser-side decryptor fails.
		displayResponse = wireResponse
	}
	duration := time.Duration(0)
	if response.TraceInfo != nil {
		duration = response.TraceInfo.TotalTime
	}
	savedFlow, err := yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		isHTTPS,
		plainRequest,
		displayResponse,
		"ai-browser-http",
		requestPacket.URL,
		response.RemoteAddr,
		yakit.CreateHTTPFlowWithRuntimeID(runtimeID),
		yakit.CreateHTTPFlowWithDuration(duration),
		yakit.CreateHTTPFlowWithTags(yakit.HTTPFlowTagBrowserPlaintext),
		yakit.CreateHTTPFlowWithBarePacketsRaw(wireRequest, wireResponse),
	)
	if err != nil {
		return nil, fmt.Errorf("build browser HTTP flow: %w", err)
	}
	if err := yakit.InsertHTTPFlowEx(savedFlow, true); err != nil {
		return nil, fmt.Errorf("save browser HTTP flow: %w", err)
	}
	gateway := map[string]interface{}{
		"version": 1, "browserRef": browserRef,
		"requestEnabled": runtime.requestEnabled, "responseEnabled": runtime.responseEnabled,
		"responseFailed": responseTransformFailure != "",
		"directions":     runtime.evidence,
	}
	metadata, err := json.Marshal(gateway)
	if err != nil {
		return nil, fmt.Errorf("HTTP flow %d already sent and saved; encode gateway evidence: %w", savedFlow.ID, err)
	}
	if err := yakit.SetProjectKeyWithGroup(consts.GetGormProjectDatabase(), fmt.Sprintf("%d_browser_gateway", savedFlow.ID), string(metadata), "browser_gateway"); err != nil {
		return nil, fmt.Errorf("HTTP flow %d already sent and saved; save gateway evidence: %w", savedFlow.ID, err)
	}
	if responseTransformFailure != "" {
		return nil, fmt.Errorf("browser response transform failed: %s", responseTransformFailure)
	}
	result := map[string]interface{}{
		"gateway":                  gateway,
		"browserRef":               browserRef,
		"url":                      requestPacket.URL,
		"statusCode":               lowhttp.GetStatusCodeFromResponse(wireResponse),
		"profileId":                profileID,
		"validationId":             validationID,
		"requestTransformEnabled":  runtime.requestEnabled,
		"responseTransformEnabled": runtime.responseEnabled,
		"requestTransformed":       !bytes.Equal(plainRequest, wireRequest),
		"responseTransformed":      !bytes.Equal(wireResponse, plainResponse),
		"plaintextRequest":         browserAgentPacketResult(plainRequest),
		"wireRequest":              browserAgentPacketResult(wireRequest),
		"wireResponse":             browserAgentPacketResult(wireResponse),
		"plaintextResponse":        browserAgentPacketResult(plainResponse),
	}
	result["httpFlow"] = map[string]interface{}{
		"id":          savedFlow.ID,
		"hiddenIndex": savedFlow.HiddenIndex,
		"runtimeId":   savedFlow.RuntimeId,
		"source":      savedFlow.SourceType,
		"view":        "plaintext",
		"alternate":   "wire",
	}
	return result, nil
}

func buildBrowserHTTPTestTool(bridge browsertools.Bridge) (*aitool.Tool, error) {
	return aitool.New(
		"browser.http.test",
		aitool.WithDescription("Send one raw HTTP request through an A/B/C browser's page transform. The engine encrypts the plaintext request before network I/O, optionally decrypts the response, records the HTTP flow, and returns both plaintext and wire evidence."),
		aitool.WithVerboseName("Browser Plaintext HTTP Test"),
		aitool.WithVerboseNameZh("浏览器明文 HTTP 测试"),
		aitool.WithUsage("Use after browser.transform.prepare returns validationDraft.id, or with an extension-saved Profile. Pass the plaintext raw HTTP request including authentication headers. With several browsers, run once per browser_ref for an A/B authorization comparison. A temporary validation draft is not persisted. Never send the request separately with another HTTP tool. HTTP 200 and requestTransformed do not prove business success; inspect the response body. Do not claim response decryption unless responseTransformEnabled is true. Repeat a test only after an explicit input or mapping change and explain that change."),
		aitool.WithKeywords([]string{"browser HTTP", "plaintext gateway", "request encryption", "response decryption", "A/B authorization", "明文网关", "加密发包", "响应解密", "越权测试"}),
		aitool.WithStringParam("browser_ref", aitool.WithParam_Description("Online browser reference such as A or B; optional when exactly one browser is online"), aitool.WithParam_MaxLength(512)),
		aitool.WithStringParam("profile_id", aitool.WithParam_Description("Saved transform Profile ID; mutually exclusive with validation_id"), aitool.WithParam_MaxLength(512)),
		aitool.WithStringParam("validation_id", aitool.WithParam_Description("Short-lived ID returned by browser.profile.validate; mutually exclusive with profile_id"), aitool.WithParam_MaxLength(512)),
		aitool.WithStringParam("request", aitool.WithParam_Description("Complete plaintext HTTP/1.x request, including request line, Host, authentication headers, blank line, and body"), aitool.WithParam_Required(true)),
		aitool.WithBoolParam("is_https", aitool.WithParam_Description("Whether the upstream request uses TLS; set this explicitly from the captured request URL"), aitool.WithParam_Required(true)),
		aitool.WithBoolParam("transform_request", aitool.WithParam_Description("Optional override for a temporary draft; omitted uses its validated request direction")),
		aitool.WithBoolParam("transform_response", aitool.WithParam_Description("Optional override for a temporary draft; omitted uses its validated response direction, including automatic decryption for bidirectional gateways")),
		aitool.WithIntegerParam("timeout_seconds", aitool.WithParam_Description("Overall request and page-transform timeout"), aitool.WithParam_Default(30), aitool.WithParam_Min(2), aitool.WithParam_Max(60)),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, _ io.Writer, _ io.Writer) (interface{}, error) {
			return executeBrowserAgentHTTPRequest(ctx, bridge, params, runtimeConfig)
		}),
	)
}

func buildBrowserTransformPrepareTool(bridge browsertools.Bridge) (*aitool.Tool, error) {
	return aitool.New(
		"browser.transform.prepare",
		aitool.WithDescription("Prepare a temporary plaintext HTTP transform from one browser.crypto.inspect capture. The extension automatically re-triggers the inspected operation when business capture is required, captures missing directions, and validates one bidirectional gateway without plugin UI."),
		aitool.WithVerboseName("Prepare Browser Plaintext Transform"),
		aitool.WithVerboseNameZh("准备浏览器明文转换"),
		aitool.WithUsage("Call with gatewayPreparation.candidateId from browser.crypto.inspect, whether ready or capture-required, and a complete plaintext HTTP request. Missing business directions are captured automatically without plugin UI. If the trigger changed, pass fresh captureId/nodeId from browser.context. On success pass validationDraft.id to browser.http.test; its default directions follow the validated gateway. For advanced diagnosis or recovery, discover lower-level capabilities from browser.capability.catalog. Do not reopen the website or resend a request to recover from an unexplained error."),
		aitool.WithKeywords([]string{"browser transform", "plaintext gateway", "page encryption", "明文网关", "加密发包", "页面加密"}),
		aitool.WithStringParam("browser_ref", aitool.WithParam_Description("Online browser reference such as A or B; optional when exactly one browser is online"), aitool.WithParam_MaxLength(512)),
		aitool.WithStringParam("candidate_id", aitool.WithParam_Description("gatewayPreparation.candidateId returned by browser.crypto.inspect"), aitool.WithParam_MaxLength(160), aitool.WithParam_Required(true)),
		aitool.WithStringParam("request", aitool.WithParam_Description("Complete plaintext HTTP/1.x request to validate against the captured operation"), aitool.WithParam_Required(true)),
		aitool.WithBoolParam("is_https", aitool.WithParam_Description("Whether the captured upstream request uses TLS"), aitool.WithParam_Required(true)),
		aitool.WithStringArrayParam("input_paths", aitool.WithParam_Description("Optional explicit packet value paths when automatic input mapping is ambiguous")),
		aitool.WithStringParam("captureId", aitool.WithParam_Description("Optional fresh browser.context capture ID if the original trigger changed; supply together with nodeId")),
		aitool.WithStringParam("nodeId", aitool.WithParam_Description("Optional visible operation trigger from captureId; otherwise the extension reuses the inspected operation")),
		aitool.WithStringParam("name", aitool.WithParam_Description("Optional local draft name"), aitool.WithParam_MaxLength(120)),
		aitool.WithIntegerParam("tabId", aitool.WithParam_Description("Target tab ID returned by browser.crypto.inspect"), aitool.WithParam_Min(1)),
		aitool.WithIntegerParam("frameId", aitool.WithParam_Description("Target frame ID returned by browser.crypto.inspect"), aitool.WithParam_Min(0)),
		aitool.WithStringParam("documentId", aitool.WithParam_Description("Target document ID returned by browser.crypto.inspect"), aitool.WithParam_MaxLength(160)),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, _ *aitool.ToolRuntimeConfig, _ io.Writer, _ io.Writer) (interface{}, error) {
			deviceID, browserRef, err := browsertools.ResolveBrowserDevice(
				bridge,
				strings.TrimSpace(params.GetString("device_id")),
				params.GetString("browser_ref"),
			)
			if err != nil {
				return nil, err
			}
			packet, err := browserTransformPacketFromRequest([]byte(params.GetString("request")), params.GetBool("is_https"))
			if err != nil {
				return nil, err
			}
			callParams := map[string]interface{}{
				"candidateId": params.GetString("candidate_id"),
				"packet":      packet,
			}
			for _, key := range []string{"tabId", "frameId", "documentId", "name"} {
				if params.Has(key) {
					callParams[key] = params[key]
				}
			}
			if params.Has("input_paths") {
				callParams["inputPaths"] = params.GetStringSlice("input_paths")
			}
			if params.Has("captureId") || params.Has("nodeId") {
				if params.GetString("captureId") == "" || params.GetString("nodeId") == "" {
					return nil, errors.New("captureId and nodeId must be supplied together")
				}
				callParams["trigger"] = map[string]string{"captureId": params.GetString("captureId"), "nodeId": params.GetString("nodeId")}
			}
			catalog, connected := bridge.CapabilityCatalog(deviceID)
			if !connected {
				return nil, fmt.Errorf("browser %s is offline or has no signed capability catalog", browserRef)
			}
			if err := catalog.ValidateCapabilityParams("browser.transform.prepare", callParams); err != nil {
				return nil, err
			}
			callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			raw, err := bridge.CallDevice(callCtx, deviceID, "browser.transform.prepare", callParams)
			if err != nil {
				return nil, err
			}
			var result map[string]interface{}
			if err := json.Unmarshal(raw, &result); err != nil {
				return nil, fmt.Errorf("decode browser transform preparation: %w", err)
			}
			var explanation interface{}
			if execution, ok := result["execution"].(map[string]interface{}); ok {
				explanation = execution["explanation"]
			}
			return map[string]interface{}{
				"explanation":     explanation,
				"browserRef":      browserRef,
				"valid":           result["valid"],
				"proofLevel":      result["proofLevel"],
				"comparison":      result["comparison"],
				"validationDraft": result["validationDraft"],
				"next":            result["next"],
			}, nil
		}),
	)
}

func browserTransformFailureRequest(err error) []byte {
	message := "browser transform failed"
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	message = strings.NewReplacer("\r", " ", "\n", " ").Replace(message)
	if len(message) > 1024 {
		message = message[:1024]
	}
	// Deliberately not an HTTP request. The pool reports the exact error and
	// never opens a network connection, so plaintext cannot escape on failure.
	return []byte(browserTransformFailureRequestPrefix + message)
}

func browserTransformRequestFailureReason(packet []byte) string {
	if !bytes.HasPrefix(packet, []byte(browserTransformFailureRequestPrefix)) {
		return ""
	}
	return strings.TrimSpace(string(bytes.TrimPrefix(packet, []byte(browserTransformFailureRequestPrefix))))
}

func browserTransformFailureResponse(err error) []byte {
	message := "browser response transform failed"
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	message = strings.NewReplacer("\r", " ", "\n", " ").Replace(message)
	if len(message) > 4096 {
		message = message[:4096]
	}
	body := []byte(message)
	return []byte(fmt.Sprintf(
		"HTTP/1.1 598 Browser Transform Failed\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: %d\r\nX-Yakit-Browser-Transform: failed\r\n\r\n%s",
		len(body), body,
	))
}

func browserTransformResponseFailureReason(runtime *browserTransformRuntime, packet []byte) string {
	if runtime == nil || !runtime.responseEnabled || !strings.EqualFold(
		strings.TrimSpace(lowhttp.GetHTTPPacketHeader(packet, "X-Yakit-Browser-Transform")),
		"failed",
	) {
		return ""
	}
	return strings.TrimSpace(string(lowhttp.GetHTTPPacketBody(packet)))
}

func (r *browserTransformRuntime) beforeHook(ctx context.Context) func(bool, []byte, []byte) []byte {
	return func(isHTTPS bool, _ []byte, plainRequest []byte) []byte {
		wireRequest := plainRequest
		if r.requestEnabled {
			var err error
			wireRequest, err = r.transformPacket(ctx, "request", plainRequest, plainRequest, isHTTPS)
			if err != nil {
				wireRequest = browserTransformFailureRequest(err)
			}
		}
		r.rememberRequest(plainRequest, wireRequest)
		return wireRequest
	}
}

func (r *browserTransformRuntime) afterHook(ctx context.Context) func(bool, []byte, []byte, []byte, []byte) []byte {
	return func(isHTTPS bool, _ []byte, wireRequest []byte, _ []byte, wireResponse []byte) []byte {
		plainResponse := wireResponse
		if r.responseEnabled {
			var err error
			plainResponse, err = r.transformPacket(ctx, "response", wireResponse, wireRequest, isHTTPS)
			if err != nil {
				plainResponse = browserTransformFailureResponse(err)
			}
		}
		r.rememberResponse(wireRequest, wireResponse, plainResponse)
		return plainResponse
	}
}

func applyBeforeHook(
	hook func(bool, []byte, []byte) []byte,
	https bool,
	origin []byte,
	request []byte,
) []byte {
	if hook == nil {
		return request
	}
	if output := hook(https, origin, request); len(output) > 0 {
		return output
	}
	return request
}

func composeBrowserTransformBefore(
	user func(bool, []byte, []byte) []byte,
	browser func(bool, []byte, []byte) []byte,
) func(bool, []byte, []byte) []byte {
	if user == nil {
		return browser
	}
	if browser == nil {
		return user
	}
	return func(https bool, origin []byte, request []byte) []byte {
		plainRequest := applyBeforeHook(user, https, origin, request)
		return applyBeforeHook(browser, https, origin, plainRequest)
	}
}

func composeBrowserTransformAfter(
	browser func(bool, []byte, []byte, []byte, []byte) []byte,
	user func(bool, []byte, []byte, []byte, []byte) []byte,
) func(bool, []byte, []byte, []byte, []byte) []byte {
	if user == nil {
		return browser
	}
	if browser == nil {
		return user
	}
	return func(https bool, originRequest, request, originResponse, response []byte) []byte {
		plainResponse := browser(https, originRequest, request, originResponse, response)
		if len(plainResponse) == 0 {
			plainResponse = response
		}
		if output := user(https, originRequest, request, originResponse, plainResponse); len(output) > 0 {
			return output
		}
		return plainResponse
	}
}

func transformedResponsePackets(
	runtime *browserTransformRuntime,
	wireRequest []byte,
	plainResponse []byte,
) (plainRequest, savedWireRequest, savedWireResponse []byte) {
	if runtime == nil {
		return wireRequest, nil, nil
	}
	trace := runtime.takeTrace(wireRequest)
	if trace == nil {
		return wireRequest, cloneTransformPacket(wireRequest), nil
	}
	plainRequest = trace.PlainRequest
	if len(plainRequest) == 0 {
		plainRequest = wireRequest
	}
	savedWireRequest = trace.WireRequest
	if len(savedWireRequest) == 0 {
		savedWireRequest = wireRequest
	}
	savedWireResponse = trace.WireResponse
	if len(savedWireResponse) == 0 && !bytes.Equal(trace.PlainResponse, plainResponse) {
		savedWireResponse = cloneTransformPacket(plainResponse)
	}
	return plainRequest, savedWireRequest, savedWireResponse
}
