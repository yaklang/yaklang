//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
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
	ProfileID string                 `json:"profileId"`
	Direction string                 `json:"direction"`
	Packet    browserTransformPacket `json:"packet"`
}

type browserTransformResult struct {
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
		ProfileID: r.profileID,
		Direction: direction,
		Packet: browserTransformPacket{
			Method: method, URL: requestURL.String(), StatusCode: statusCode,
			Headers:    browserTransformHeaders(working),
			BodyBase64: base64.StdEncoding.EncodeToString(body),
		},
	}
	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	raw, err := r.caller.CallDevice(callCtx, r.deviceID, "browser.transform.execute", input)
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
	return output, nil
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
