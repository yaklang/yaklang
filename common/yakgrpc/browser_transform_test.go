//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

type fakeBrowserTransformCaller struct {
	mu         sync.Mutex
	calls      []string
	execute    func(browserTransformCall) (browserTransformResult, error)
	profileID  string
	requestOn  bool
	responseOn bool
	origin     string
	methods    []string
	urlPattern string
}

type browserTransformCallerFunc func(context.Context, string, string, interface{}) (json.RawMessage, error)

func (f browserTransformCallerFunc) CallDevice(
	ctx context.Context,
	deviceID string,
	method string,
	params interface{},
) (json.RawMessage, error) {
	return f(ctx, deviceID, method, params)
}

func (f *fakeBrowserTransformCaller) CallDevice(
	_ context.Context,
	_ string,
	method string,
	params interface{},
) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, method)
	f.mu.Unlock()
	if method == "browser.transform.profile.list" {
		return json.Marshal([]map[string]interface{}{
			{
				"id": f.profileID, "name": "Login gateway", "enabled": true,
				"origin":   f.origin,
				"request":  map[string]bool{"enabled": f.requestOn},
				"response": map[string]bool{"enabled": f.responseOn},
				"match": map[string]interface{}{
					"methods": f.methods, "urlPattern": f.urlPattern,
				},
			},
		})
	}
	if method != "browser.transform.execute" {
		return nil, errors.New("unexpected method")
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var input browserTransformCall
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	result, err := f.execute(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func encodedTransformBody(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

func TestBrowserValidationTransformRuntime(t *testing.T) {
	caller := browserTransformCallerFunc(func(
		_ context.Context,
		deviceID string,
		method string,
		params interface{},
	) (json.RawMessage, error) {
		require.Equal(t, "browser-a", deviceID)
		require.Equal(t, "browser.transform.validation.execute", method)
		raw, err := json.Marshal(params)
		require.NoError(t, err)
		var input browserTransformCall
		require.NoError(t, json.Unmarshal(raw, &input))
		require.Empty(t, input.ProfileID)
		require.Equal(t, "validation-1", input.ValidationID)
		body := `{"wire":true}`
		if input.Direction == "response" {
			body = `{"plain":true}`
		}
		return json.Marshal(browserTransformResult{
			ProfileID:  "transient-validation-1",
			Direction:  input.Direction,
			URL:        input.Packet.URL,
			BodyBase64: encodedTransformBody(body),
		})
	})
	runtime, err := prepareBrowserValidationTransform(caller, "browser-a", "validation-1", true, true, 5*time.Second)
	require.NoError(t, err)
	plainRequest := []byte("POST /api HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/json\r\n\r\n{\"plain\":true}")
	wireRequest := runtime.beforeHook(context.Background())(true, nil, plainRequest)
	require.JSONEq(t, `{"wire":true}`, string(lowhttp.GetHTTPPacketBody(wireRequest)))
	wireResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"cipher\":true}")
	plainResponse := runtime.afterHook(context.Background())(true, nil, wireRequest, nil, wireResponse)
	require.JSONEq(t, `{"plain":true}`, string(lowhttp.GetHTTPPacketBody(plainResponse)))
}

func TestBrowserHTTPTestToolContract(t *testing.T) {
	tool, err := buildBrowserHTTPTestTool(nil)
	require.NoError(t, err)
	require.False(t, tool.NoNeedUserReview)
	schema := tool.ToJSONSchemaString()
	require.Contains(t, schema, "browser_ref")
	require.Contains(t, schema, "validation_id")
	require.Contains(t, schema, "profile_id")
	require.Contains(t, schema, "is_https")
	require.NotContains(t, schema, "device_id")
}

func TestBrowserTransformPrepareToolContractAndPacket(t *testing.T) {
	tool, err := buildBrowserTransformPrepareTool(nil)
	require.NoError(t, err)
	require.Contains(t, tool.ToJSONSchemaString(), "candidate_id")
	require.NotContains(t, tool.ToJSONSchemaString(), "device_id")

	packet, err := browserTransformPacketFromRequest([]byte(
		"POST /api/login HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/json\r\n\r\n{\"username\":\"admin\"}",
	), true)
	require.NoError(t, err)
	require.Equal(t, "POST", packet.Method)
	require.Equal(t, "https://example.test/api/login", packet.URL)
	require.JSONEq(t, `{"username":"admin"}`, string(mustDecodeBase64(t, packet.BodyBase64)))
}

func TestBrowserTransformRuntimeRequestResponse(t *testing.T) {
	caller := &fakeBrowserTransformCaller{
		profileID: "profile-1", requestOn: true, responseOn: true,
		execute: func(input browserTransformCall) (browserTransformResult, error) {
			require.Equal(t, "profile-1", input.ProfileID)
			require.Equal(t, "POST", input.Packet.Method)
			if input.Direction == "request" {
				require.Equal(t, "https://example.test/api/login", input.Packet.URL)
				require.JSONEq(t, `{"password":"plain"}`, string(mustDecodeBase64(t, input.Packet.BodyBase64)))
				return browserTransformResult{
					ProfileID: input.ProfileID, Direction: input.Direction,
					Explanation: json.RawMessage(`{"version":1,"directions":[{"direction":"request","stages":[]}]}`),
					ProofLevel:  "structure",
					URL:         "https://example.test/api/login?channel=browser&signature=a%2Bb",
					BodyBase64:  encodedTransformBody(`{"password":"cipher"}`),
					SetHeaders:  []browserTransformHeader{{Name: "X-Sign", Value: "signed"}},
				}, nil
			}
			require.Equal(t, "https://example.test/api/login?channel=browser&signature=a%2Bb", input.Packet.URL)
			require.Equal(t, 200, input.Packet.StatusCode)
			return browserTransformResult{
				ProfileID: input.ProfileID, Direction: input.Direction,
				URL:        input.Packet.URL,
				BodyBase64: encodedTransformBody(`{"ok":true}`),
			}, nil
		},
	}
	runtime, err := prepareBrowserTransform(context.Background(), caller, "browser-1", "profile-1", 5*time.Second)
	require.NoError(t, err)

	plainRequest := []byte("POST /api/login HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/json\r\nContent-Length: 20\r\n\r\n{\"password\":\"plain\"}")
	runtime.evidence = make(map[string]interface{})
	wireRequest := runtime.beforeHook(context.Background())(true, nil, plainRequest)
	require.JSONEq(t, `{"password":"cipher"}`, string(lowhttp.GetHTTPPacketBody(wireRequest)))
	require.Equal(t, "signed", lowhttp.GetHTTPPacketHeader(wireRequest, "X-Sign"))
	wireURL, err := lowhttp.ExtractURLFromHTTPRequestRaw(wireRequest, true)
	require.NoError(t, err)
	require.Equal(t, "channel=browser&signature=a%2Bb", wireURL.RawQuery)

	wireResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 21\r\n\r\n{\"payload\":\"cipher\"}")
	plainResponse := runtime.afterHook(context.Background())(true, nil, wireRequest, nil, wireResponse)
	require.JSONEq(t, `{"ok":true}`, string(lowhttp.GetHTTPPacketBody(plainResponse)))

	displayRequest, savedWireRequest, savedWireResponse := transformedResponsePackets(runtime, wireRequest, plainResponse)
	require.Equal(t, plainRequest, displayRequest)
	require.Equal(t, wireRequest, savedWireRequest)
	require.Equal(t, wireResponse, savedWireResponse)
	evidence, err := json.Marshal(runtime.evidence["request"])
	require.NoError(t, err)
	require.JSONEq(t, `{"proofLevel":"structure","explanation":{"version":1,"directions":[{"direction":"request","stages":[]}]}}`, string(evidence))
}

func TestBrowserTransformAgentContractReachesWebFuzzer(t *testing.T) {
	const profileID = "profile-agent-contract-v1"
	caller := &fakeBrowserTransformCaller{
		profileID: profileID,
		requestOn: true,
		execute: func(input browserTransformCall) (browserTransformResult, error) {
			require.Equal(t, profileID, input.ProfileID)
			require.Equal(t, "request", input.Direction)
			require.Equal(t, "POST", input.Packet.Method)
			require.Equal(t, "https://example.test/encrypt/aesrsa.php", input.Packet.URL)
			require.JSONEq(
				t,
				`{"username":"admin","password":"123456"}`,
				string(mustDecodeBase64(t, input.Packet.BodyBase64)),
			)
			return browserTransformResult{
				ProfileID: input.ProfileID,
				Direction: input.Direction,
				URL:       input.Packet.URL,
				BodyBase64: encodedTransformBody(
					`{"encryptedData":"ciphertext","encryptedKey":"wrapped-key","encryptedIv":"wrapped-iv"}`,
				),
				SetHeaders: []browserTransformHeader{
					{Name: "Content-Type", Value: "application/json"},
				},
			}, nil
		},
	}
	runtime, err := prepareBrowserTransform(
		context.Background(),
		caller,
		"browser-agent-contract-v1",
		profileID,
		5*time.Second,
	)
	require.NoError(t, err)

	plainRequest := []byte(
		"POST /encrypt/aesrsa.php HTTP/1.1\r\n" +
			"Host: example.test\r\n" +
			"Content-Type: application/json\r\n" +
			"Content-Length: 40\r\n\r\n" +
			`{"username":"admin","password":"123456"}`,
	)
	wireRequest := runtime.beforeHook(context.Background())(true, nil, plainRequest)
	require.JSONEq(
		t,
		`{"encryptedData":"ciphertext","encryptedKey":"wrapped-key","encryptedIv":"wrapped-iv"}`,
		string(lowhttp.GetHTTPPacketBody(wireRequest)),
	)
	require.Equal(t, "application/json", lowhttp.GetHTTPPacketHeader(wireRequest, "Content-Type"))

	displayRequest, savedWireRequest, _ := transformedResponsePackets(runtime, wireRequest, nil)
	require.Equal(t, plainRequest, displayRequest)
	require.Equal(t, wireRequest, savedWireRequest)
	require.Equal(t, []string{
		"browser.transform.profile.list",
		"browser.transform.execute",
	}, caller.calls)
}

func TestBrowserTransformURLMutationIsQueryOnly(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "origin", url: "https://other.test/api/login?mode=wire"},
		{name: "path", url: "https://example.test/api/admin?mode=wire"},
		{name: "fragment", url: "https://example.test/api/login?mode=wire#secret"},
		{name: "userinfo", url: "https://operator@example.test/api/login?mode=wire"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &fakeBrowserTransformCaller{
				profileID: "profile-1", requestOn: true,
				execute: func(input browserTransformCall) (browserTransformResult, error) {
					return browserTransformResult{
						ProfileID:  input.ProfileID,
						Direction:  input.Direction,
						URL:        test.url,
						BodyBase64: input.Packet.BodyBase64,
					}, nil
				},
			}
			runtime, err := prepareBrowserTransform(context.Background(), caller, "browser-1", "profile-1", time.Second)
			require.NoError(t, err)
			request := []byte("POST /api/login?mode=plain HTTP/1.1\r\nHost: example.test\r\nContent-Length: 5\r\n\r\nplain")
			_, err = runtime.transformPacket(context.Background(), "request", request, request, true)
			require.Error(t, err)
		})
	}
}

func TestBrowserTransformRequestFailureIsClosed(t *testing.T) {
	caller := &fakeBrowserTransformCaller{
		profileID: "profile-1", requestOn: true,
		execute: func(browserTransformCall) (browserTransformResult, error) {
			return browserTransformResult{}, errors.New("page callable is stale")
		},
	}
	runtime, err := prepareBrowserTransform(context.Background(), caller, "browser-1", "profile-1", time.Second)
	require.NoError(t, err)
	plainRequest := []byte("POST / HTTP/1.1\r\nHost: example.test\r\nContent-Length: 5\r\n\r\nplain")
	output := runtime.beforeHook(context.Background())(true, nil, plainRequest)
	require.Contains(t, string(output), "YAKIT_BROWSER_TRANSFORM_FAILED")
	require.Equal(t, "page callable is stale", browserTransformRequestFailureReason(output))
	_, parseErr := lowhttp.ParseBytesToHttpRequest(output)
	require.Error(t, parseErr)
	displayRequest, _, _ := transformedResponsePackets(runtime, output, nil)
	require.Equal(t, plainRequest, displayRequest)
}

func TestBrowserTransformResponseFailureIsExplicit(t *testing.T) {
	runtime := &browserTransformRuntime{responseEnabled: true}
	packet := browserTransformFailureResponse(errors.New("page decrypt function is stale"))
	require.Equal(t, "page decrypt function is stale", browserTransformResponseFailureReason(runtime, packet))
	require.Empty(t, browserTransformResponseFailureReason(nil, packet))
	require.Empty(t, browserTransformResponseFailureReason(runtime, []byte("HTTP/1.1 200 OK\r\n\r\nok")))
}

func TestBrowserTransformHookOrder(t *testing.T) {
	var order []string
	userBefore := func(_ bool, _ []byte, request []byte) []byte {
		order = append(order, "user-before")
		return append(request, []byte("-user")...)
	}
	browserBefore := func(_ bool, _ []byte, request []byte) []byte {
		order = append(order, "browser-before")
		require.Equal(t, "plain-user", string(request))
		return []byte("wire")
	}
	require.Equal(t, "wire", string(composeBrowserTransformBefore(userBefore, browserBefore)(false, nil, []byte("plain"))))

	browserAfter := func(_ bool, _, _ []byte, _ []byte, response []byte) []byte {
		order = append(order, "browser-after")
		require.Equal(t, "wire-response", string(response))
		return []byte("plain-response")
	}
	userAfter := func(_ bool, _, _ []byte, _ []byte, response []byte) []byte {
		order = append(order, "user-after")
		require.Equal(t, "plain-response", string(response))
		return []byte("final")
	}
	require.Equal(t, "final", string(composeBrowserTransformAfter(browserAfter, userAfter)(false, nil, nil, nil, []byte("wire-response"))))
	require.Equal(t, []string{"user-before", "browser-before", "browser-after", "user-after"}, order)
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(value)
	require.NoError(t, err)
	return decoded
}
