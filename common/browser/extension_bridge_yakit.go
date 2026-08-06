package browser

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const maxExtensionFuzzerRequestBytes = 2 << 20

type extensionWebFuzzerOpenParams struct {
	RawRequestBase64 string `json:"rawRequestBase64"`
	IsHTTPS          bool   `json:"isHttps"`
	TabName          string `json:"tabName"`
}

type extensionCapturedRequestParams struct {
	RawRequestBase64 string                        `json:"rawRequestBase64"`
	IsHTTPS          bool                          `json:"isHttps"`
	Observations     []extensionRequestObservation `json:"observations,omitempty"`
}

type extensionRequestObservation struct {
	Kind             string `json:"kind"`
	Operation        string `json:"operation"`
	Algorithm        string `json:"algorithm,omitempty"`
	Direction        string `json:"direction,omitempty"`
	ScriptURL        string `json:"scriptUrl,omitempty"`
	ByteLength       int64  `json:"byteLength,omitempty"`
	ResultByteLength int64  `json:"resultByteLength,omitempty"`
	Timestamp        int64  `json:"timestamp"`
}

func (s *ExtensionBridgeServer) handleExtensionClientRequest(ctx context.Context, deviceID, method string, params json.RawMessage) (interface{}, *ExtensionBridgeError) {
	select {
	case <-ctx.Done():
		return nil, &ExtensionBridgeError{Code: "cancelled", Message: "extension request cancelled"}
	default:
	}
	switch method {
	case "yakit.web_fuzzer.open":
		return openCapturedRequestInWebFuzzer(params)
	case "yakit.poc.generate":
		return generateCapturedRequestYakPoC(params)
	case "yakit.browser_request.prepare_analysis":
		return prepareCapturedRequestAnalysis(params)
	case "yakit.browser_authorization.task":
		return s.handleExtensionAuthorizationClientTask(ctx, deviceID, params)
	case "yakit.browser_authorization.open":
		return s.openExtensionAuthorizationWorkspaceInYakit(ctx, deviceID, params)
	default:
		return nil, &ExtensionBridgeError{Code: "method_not_found", Message: "unsupported engine method: " + method}
	}
}

func decodeCapturedRequest(params json.RawMessage) ([]byte, extensionCapturedRequestParams, *ExtensionBridgeError) {
	decoder := json.NewDecoder(bytes.NewReader(params))
	decoder.DisallowUnknownFields()
	var input extensionCapturedRequestParams
	if err := decoder.Decode(&input); err != nil {
		return nil, input, &ExtensionBridgeError{Code: "invalid_params", Message: "invalid captured request: " + err.Error()}
	}
	packet, err := base64.StdEncoding.DecodeString(input.RawRequestBase64)
	if err != nil {
		return nil, input, &ExtensionBridgeError{Code: "invalid_params", Message: "rawRequestBase64 is invalid"}
	}
	if len(packet) == 0 || len(packet) > maxExtensionFuzzerRequestBytes {
		return nil, input, &ExtensionBridgeError{Code: "payload_too_large", Message: "captured request must be between 1 byte and 2 MiB"}
	}
	firstLineEnd := bytes.Index(packet, []byte("\r\n"))
	if firstLineEnd <= 0 || len(strings.Fields(string(packet[:firstLineEnd]))) != 3 {
		return nil, input, &ExtensionBridgeError{Code: "invalid_http_request", Message: "captured request has an invalid HTTP request line"}
	}
	return packet, input, nil
}

func generateCapturedRequestYakPoC(params json.RawMessage) (interface{}, *ExtensionBridgeError) {
	packet, input, bridgeErr := decodeCapturedRequest(params)
	if bridgeErr != nil {
		return nil, bridgeErr
	}
	host := "captured-request"
	if request, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(packet))); err == nil {
		host = request.Host
	}
	fileName := strings.Trim(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, host), "-")
	if fileName == "" {
		fileName = "captured-request"
	}
	if len(fileName) > 80 {
		fileName = fileName[:80]
	}
	code := fmt.Sprintf(`packet = codec.DecodeBase64(%q)

rsp, req, err = poc.HTTP(packet,
    poc.https(%t),
    poc.timeout(10),
    poc.redirectTimes(3),
)
die(err)

dump(req)
dump(rsp)
`, base64.StdEncoding.EncodeToString(packet), input.IsHTTPS)
	return map[string]interface{}{
		"language": "yak",
		"fileName": filepath.Base(fileName) + ".yak",
		"code":     code,
	}, nil
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func collectJSONKeys(value interface{}, prefix string, output map[string]struct{}, depth int) {
	if depth > 4 || len(output) >= 200 {
		return
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			output[path] = struct{}{}
			collectJSONKeys(child, path, output, depth+1)
		}
	case []interface{}:
		for _, child := range typed {
			collectJSONKeys(child, prefix+"[]", output, depth+1)
		}
	}
}

func signalCategory(name string) string {
	normalized := strings.ToLower(name)
	switch {
	case normalized == "authorization" || strings.Contains(normalized, "access-token") || strings.Contains(normalized, "api-key"):
		return "authorization"
	case strings.Contains(normalized, "csrf") || strings.Contains(normalized, "xsrf"):
		return "csrf"
	case strings.Contains(normalized, "signature") || strings.Contains(normalized, "sign") || strings.Contains(normalized, "hmac"):
		return "signature"
	case strings.Contains(normalized, "nonce"):
		return "nonce"
	case strings.Contains(normalized, "timestamp") || normalized == "date" || strings.HasSuffix(normalized, "-time"):
		return "timestamp"
	case normalized == "cookie":
		return "cookie"
	default:
		return ""
	}
}

func prepareCapturedRequestAnalysis(params json.RawMessage) (interface{}, *ExtensionBridgeError) {
	packet, input, bridgeErr := decodeCapturedRequest(params)
	if bridgeErr != nil {
		return nil, bridgeErr
	}
	request, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		return nil, &ExtensionBridgeError{Code: "invalid_http_request", Message: err.Error()}
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxExtensionFuzzerRequestBytes+1))
	if err != nil {
		return nil, &ExtensionBridgeError{Code: "invalid_http_request", Message: "cannot read request body: " + err.Error()}
	}
	if separator := bytes.Index(packet, []byte("\r\n\r\n")); len(body) == 0 && separator >= 0 && separator+4 < len(packet) {
		body = packet[separator+4:]
	}
	queryKeys := make(map[string]struct{})
	for key := range request.URL.Query() {
		queryKeys[key] = struct{}{}
	}
	headerNames := make([]string, 0, len(request.Header))
	signals := make([]map[string]string, 0)
	for name := range request.Header {
		headerNames = append(headerNames, name)
		if category := signalCategory(name); category != "" {
			signals = append(signals, map[string]string{"location": "header", "name": name, "category": category})
		}
	}
	sort.Strings(headerNames)
	cookieNames := make([]string, 0)
	for _, cookie := range request.Cookies() {
		cookieNames = append(cookieNames, cookie.Name)
	}
	sort.Strings(cookieNames)
	bodyKeys := make(map[string]struct{})
	contentType := request.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "json") && len(body) > 0 {
		var decoded interface{}
		if json.Unmarshal(body, &decoded) == nil {
			collectJSONKeys(decoded, "", bodyKeys, 0)
		}
	} else if strings.Contains(strings.ToLower(contentType), "application/x-www-form-urlencoded") {
		if values, parseErr := url.ParseQuery(string(body)); parseErr == nil {
			for key := range values {
				bodyKeys[key] = struct{}{}
			}
		}
	}
	for _, key := range append(sortedKeys(queryKeys), sortedKeys(bodyKeys)...) {
		if category := signalCategory(key); category != "" {
			signals = append(signals, map[string]string{"location": "parameter", "name": key, "category": category})
		}
	}
	observations := input.Observations
	if len(observations) > 100 {
		observations = observations[len(observations)-100:]
	}
	return map[string]interface{}{
		"request": map[string]interface{}{
			"method": request.Method, "scheme": map[bool]string{true: "https", false: "http"}[input.IsHTTPS],
			"host": request.Host, "path": request.URL.Path, "contentType": contentType,
			"queryKeys": sortedKeys(queryKeys), "headerNames": headerNames, "cookieNames": cookieNames,
			"bodyKeys": sortedKeys(bodyKeys), "bodyBytes": len(body),
		},
		"signals":      signals,
		"observations": observations,
		"valuePolicy":  "Request values, cookie values, authorization values, and body values are omitted.",
		"recommendedChecks": []string{
			"Identify the exact authentication boundary and whether credentials are replay-bound.",
			"Correlate signature, nonce, and timestamp fields with observed WebCrypto or CryptoJS calls.",
			"Test object-level authorization with user-approved alternate identifiers.",
			"Verify replay, expiry, canonicalization, and cross-origin assumptions without guessing secret values.",
		},
	}, nil
}

func openCapturedRequestInWebFuzzer(params json.RawMessage) (interface{}, *ExtensionBridgeError) {
	config, tabName, bridgeErr := buildCapturedWebFuzzerConfig(params)
	if bridgeErr != nil {
		return nil, bridgeErr
	}
	if _, err := yakit.CreateOrUpdateWebFuzzerConfig(consts.GetGormProjectDatabase(), &schema.WebFuzzerConfig{
		PageId: config.PageId,
		Type:   config.Type,
		Config: config.Config,
	}); err != nil {
		return nil, &ExtensionBridgeError{Code: "fuzzer_save_failed", Message: err.Error()}
	}
	yakit.BroadcastWebFuzzerTab(true, config)
	return map[string]interface{}{"pageId": config.PageId, "tabName": tabName}, nil
}

func buildCapturedWebFuzzerConfig(params json.RawMessage) (*ypb.FuzzerConfig, string, *ExtensionBridgeError) {
	decoder := json.NewDecoder(bytes.NewReader(params))
	decoder.DisallowUnknownFields()
	var input extensionWebFuzzerOpenParams
	if err := decoder.Decode(&input); err != nil {
		return nil, "", &ExtensionBridgeError{Code: "invalid_params", Message: "invalid Web Fuzzer request: " + err.Error()}
	}
	packet, err := base64.StdEncoding.DecodeString(input.RawRequestBase64)
	if err != nil {
		return nil, "", &ExtensionBridgeError{Code: "invalid_params", Message: "rawRequestBase64 is invalid"}
	}
	if len(packet) == 0 || len(packet) > maxExtensionFuzzerRequestBytes {
		return nil, "", &ExtensionBridgeError{Code: "payload_too_large", Message: "captured request must be between 1 byte and 2 MiB"}
	}
	firstLineEnd := bytes.Index(packet, []byte("\r\n"))
	if firstLineEnd <= 0 || len(strings.Fields(string(packet[:firstLineEnd]))) != 3 {
		return nil, "", &ExtensionBridgeError{Code: "invalid_http_request", Message: "captured request has an invalid HTTP request line"}
	}
	tabName := strings.TrimSpace(input.TabName)
	if tabName == "" {
		tabName = "Browser Request"
	}
	if len(tabName) > 120 {
		tabName = tabName[:120]
	}
	config, err := yakit.BuildWebFuzzerConfig(&ypb.FuzzerRequest{RequestRaw: packet, IsHTTPS: input.IsHTTPS}, func(options *yakit.WebFuzzerPageBuildOptions) {
		options.TabName = tabName
	})
	if err != nil {
		return nil, "", &ExtensionBridgeError{Code: "fuzzer_config_failed", Message: err.Error()}
	}
	return config, tabName, nil
}
