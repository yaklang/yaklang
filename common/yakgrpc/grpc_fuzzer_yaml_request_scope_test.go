package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_Matcher tests new request scope in matchers
func TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_Matcher(t *testing.T) {
	tests := []struct {
		name        string
		yamlTpl     string
		reqPath     string
		reqBody     string
		expectMatch bool
	}{
		{
			name: "match request_header scope",
			yamlTpl: `id: test-request-header
http:
  - method: POST
    path:
      - '{{RootURL}}/api/test'
    headers:
      Authorization: Bearer secret-token
      X-Custom-Header: test-value
    body: '{"key": "value"}'
    matchers:
      - type: word
        scope: request_header
        words:
          - "Authorization: Bearer"
        condition: and`,
			reqPath:     "/api/test",
			reqBody:     `{"key": "value"}`,
			expectMatch: true,
		},
		{
			name: "match request_body scope",
			yamlTpl: `id: test-request-body
http:
  - method: POST
    path:
      - '{{RootURL}}/api/login'
    body: 'username=admin&password=secret123'
    matchers:
      - type: word
        scope: request_body
        words:
          - "password=secret123"
        condition: and`,
			reqPath:     "/api/login",
			reqBody:     "username=admin&password=secret123",
			expectMatch: true,
		},
		{
			name: "match request_url scope",
			yamlTpl: `id: test-request-url
http:
  - method: GET
    path:
      - '{{RootURL}}/api/users?category=admin&limit=10'
    matchers:
      - type: word
        scope: request_url
        words:
          - "category=admin"
        condition: and`,
			reqPath:     "/api/users?category=admin&limit=10",
			expectMatch: true,
		},
		{
			name: "match request_raw scope with regexp",
			yamlTpl: `id: test-request-raw
http:
  - method: POST
    path:
      - '{{RootURL}}/api/data'
    body: '{"id": 123, "name": "test"}'
    matchers:
      - type: regex
        scope: request_raw
        regex:
          - 'POST /api/\w+'
        condition: and`,
			reqPath:     "/api/data",
			reqBody:     `{"id": 123, "name": "test"}`,
			expectMatch: true,
		},
		{
			name: "negative match request_body",
			yamlTpl: `id: test-negative
http:
  - method: POST
    path:
      - '{{RootURL}}/api/test'
    body: 'safe_data=value'
    matchers:
      - type: word
        scope: request_body
        words:
          - "malicious_code"
        condition: and`,
			reqPath:     "/api/test",
			reqBody:     "safe_data=value",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := false
			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(200)
				writer.Write([]byte("OK"))
			})
			addr := utils.HostPort(host, port)

			yakTemplate, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(tt.yamlTpl)
			require.NoError(t, err)

			_, err = yakTemplate.ExecWithUrl("http://"+addr, httptpl.NewConfig(
				httptpl.WithResultCallback(func(y *httptpl.YakTemplate, reqBulk *httptpl.YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
					matched = result
				}),
			))
			require.NoError(t, err)
			assert.Equal(t, tt.expectMatch, matched, "Match result mismatch")
		})
	}
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_Extractor tests new request scope in extractors
func TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_Extractor(t *testing.T) {
	tests := []struct {
		name          string
		yamlTpl       string
		extractorName string
		expectedValue string
	}{
		{
			name: "extract from request_header",
			yamlTpl: `id: test-extract-header
http:
  - method: GET
    path:
      - '{{RootURL}}/api/test'
    headers:
      Authorization: Bearer abc123xyz
      X-Session-ID: session-456
    extractors:
      - name: auth_token
        type: regex
        scope: request_header
        regex:
          - 'Bearer ([a-zA-Z0-9]+)'
        group: 1`,
			extractorName: "auth_token",
			expectedValue: "abc123xyz",
		},
		{
			name: "extract from request_body",
			yamlTpl: `id: test-extract-body
http:
  - method: POST
    path:
      - '{{RootURL}}/api/login'
    body: 'username=testuser&password=pass123&remember=true'
    extractors:
      - name: username
        type: regex
        scope: request_body
        regex:
          - 'username=(\w+)'
        group: 1`,
			extractorName: "username",
			expectedValue: "testuser",
		},
		{
			name: "extract from request_url",
			yamlTpl: `id: test-extract-url
http:
  - method: GET
    path:
      - '{{RootURL}}/api/products?category=electronics&page=2'
    extractors:
      - name: category
        type: regex
        scope: request_url
        regex:
          - 'category=(\w+)'
        group: 1`,
			extractorName: "category",
			expectedValue: "electronics",
		},
		{
			name: "extract from request_raw",
			yamlTpl: `id: test-extract-raw
http:
  - method: POST
    path:
      - '{{RootURL}}/api/endpoint'
    body: 'data=test'
    extractors:
      - name: endpoint
        type: regex
        scope: request_raw
        regex:
          - 'POST (/api/\w+)'
        group: 1`,
			extractorName: "endpoint",
			expectedValue: "/api/endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var extractedValue string
			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(200)
				writer.Write([]byte("OK"))
			})
			addr := utils.HostPort(host, port)

			yakTemplate, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(tt.yamlTpl)
			require.NoError(t, err)

			_, err = yakTemplate.ExecWithUrl("http://"+addr, httptpl.NewConfig(
				httptpl.WithResultCallback(func(y *httptpl.YakTemplate, reqBulk *httptpl.YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
					if val, ok := extractor[tt.extractorName]; ok {
						extractedValue = utils.InterfaceToString(val)
					}
				}),
			))
			require.NoError(t, err)
			assert.Equal(t, tt.expectedValue, extractedValue, "Extracted value mismatch")
		})
	}
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_IsHttps tests is_https variable in DSL
func TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_IsHttps(t *testing.T) {
	tests := []struct {
		name        string
		yamlTpl     string
		useHTTPS    bool
		expectMatch bool
	}{
		{
			name: "check is_https with HTTP",
			yamlTpl: `id: test-is-https-http
http:
  - method: GET
    path:
      - '{{RootURL}}/test'
    matchers:
      - type: dsl
        dsl:
          - 'is_https == false'`,
			useHTTPS:    false,
			expectMatch: true,
		},
		{
			name: "check is_https with HTTPS",
			yamlTpl: `id: test-is-https-https
http:
  - method: GET
    path:
      - '{{RootURL}}/test'
    matchers:
      - type: dsl
        dsl:
          - 'is_https == true'`,
			useHTTPS:    true,
			expectMatch: true,
		},
		{
			name: "check request_url contains https",
			yamlTpl: `id: test-request-url-https
http:
  - method: GET
    path:
      - '{{RootURL}}/api/secure'
    matchers:
      - type: dsl
        dsl:
          - 'contains(request_url, "https://")'`,
			useHTTPS:    true,
			expectMatch: true,
		},
		{
			name: "check request_url contains http (not https)",
			yamlTpl: `id: test-request-url-http
http:
  - method: GET
    path:
      - '{{RootURL}}/api/test'
    matchers:
      - type: dsl
        dsl:
          - 'contains(request_url, "http://") && !contains(request_url, "https://")'`,
			useHTTPS:    false,
			expectMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := false
			var handler http.HandlerFunc = func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(200)
				writer.Write([]byte("OK"))
			}

			var addr string
			var host string
			var port int
			if tt.useHTTPS {
				// Skip HTTPS tests for now as setup is complex
				t.Skip("HTTPS test skipped - requires TLS setup")
			} else {
				host, port = utils.DebugMockHTTPHandlerFunc(handler)
				addr = utils.HostPort(host, port)
			}

			yakTemplate, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(tt.yamlTpl)
			require.NoError(t, err)

			protocol := "http"
			if tt.useHTTPS {
				protocol = "https"
			}

			_, err = yakTemplate.ExecWithUrl(protocol+"://"+addr, httptpl.NewConfig(
				httptpl.WithResultCallback(func(y *httptpl.YakTemplate, reqBulk *httptpl.YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
					matched = result
				}),
			))
			require.NoError(t, err)
			assert.Equal(t, tt.expectMatch, matched, "Match result mismatch for is_https")
		})
	}
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_YamlImportExport tests yaml import/export with request scope
func TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_YamlImportExport(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name    string
		yamlTpl string
	}{
		{
			name: "import/export with request_header scope",
			yamlTpl: `id: test-import-export
http:
  - method: POST
    path:
      - '{{RootURL}}/api/test'
    headers:
      Authorization: Bearer token123
    body: '{"test": "data"}'
    matchers:
      - type: word
        scope: request_header
        words:
          - "Authorization"
    extractors:
      - name: auth
        type: regex
        scope: request_header
        regex:
          - 'Bearer (\w+)'
        group: 1`,
		},
		{
			name: "import/export with request_body scope",
			yamlTpl: `id: test-body-scope
http:
  - method: POST
    path:
      - '{{RootURL}}/login'
    body: 'user=admin&pass=secret'
    matchers:
      - type: word
        scope: request_body
        words:
          - "user=admin"
    extractors:
      - name: username
        type: regex
        scope: request_body
        regex:
          - 'user=(\w+)'
        group: 1`,
		},
		{
			name: "import/export with request_url scope",
			yamlTpl: `id: test-url-scope
http:
  - method: GET
    path:
      - '{{RootURL}}/api/data?id=123&type=test'
    matchers:
      - type: word
        scope: request_url
        words:
          - "id=123"
    extractors:
      - name: data_id
        type: regex
        scope: request_url
        regex:
          - 'id=(\d+)'
        group: 1`,
		},
		{
			name: "import/export with request_raw scope",
			yamlTpl: `id: test-raw-scope
http:
  - method: POST
    path:
      - '{{RootURL}}/api/endpoint'
    body: 'payload=test'
    matchers:
      - type: regex
        scope: request_raw
        regex:
          - 'POST /api/\w+'
    extractors:
      - name: method
        type: regex
        scope: request_raw
        regex:
          - '(POST|GET|PUT|DELETE)'
        group: 1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Import YAML
			importRsp, err := client.ImportHTTPFuzzerTaskFromYaml(ctx, &ypb.ImportHTTPFuzzerTaskFromYamlRequest{
				YamlContent: tt.yamlTpl,
			})
			require.NoError(t, err)
			require.NotNil(t, importRsp)
			require.Greater(t, len(importRsp.Requests.Requests), 0)

			// Check that scope is preserved
			req := importRsp.Requests.Requests[0]
			if req.Matchers != nil && len(req.Matchers) > 0 {
				for _, matcher := range req.Matchers {
					if strings.HasPrefix(matcher.Scope, "request_") {
						assert.Contains(t, []string{"request_header", "request_body", "request_url", "request_raw"}, matcher.Scope)
					}
				}
			}
			if len(req.Extractors) > 0 {
				for _, extractor := range req.Extractors {
					if strings.HasPrefix(extractor.Scope, "request_") {
						assert.Contains(t, []string{"request_header", "request_body", "request_url", "request_raw"}, extractor.Scope)
					}
				}
			}

			// Export back to YAML
			exportRsp, err := client.ExportHTTPFuzzerTaskToYaml(ctx, &ypb.ExportHTTPFuzzerTaskToYamlRequest{
				Requests:     importRsp.Requests,
				TemplateType: "raw",
			})
			require.NoError(t, err)
			require.NotEmpty(t, exportRsp.YamlContent)

			// Verify exported YAML contains request scope
			yamlContent := exportRsp.YamlContent
			if strings.Contains(tt.yamlTpl, "request_header") {
				assert.Contains(t, yamlContent, "request_header", "Exported YAML should contain request_header scope")
			}
			if strings.Contains(tt.yamlTpl, "request_body") {
				assert.Contains(t, yamlContent, "request_body", "Exported YAML should contain request_body scope")
			}
			if strings.Contains(tt.yamlTpl, "request_url") {
				assert.Contains(t, yamlContent, "request_url", "Exported YAML should contain request_url scope")
			}
			if strings.Contains(tt.yamlTpl, "request_raw") {
				assert.Contains(t, yamlContent, "request_raw", "Exported YAML should contain request_raw scope")
			}
		})
	}
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_Combined tests combined request and response scope
func TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_Combined(t *testing.T) {
	yamlTpl := `id: test-combined-scope
http:
  - method: POST
    path:
      - '{{RootURL}}/api/auth'
    headers:
      X-API-Key: secret-key-123
    body: '{"username": "admin", "password": "pass123"}'
    matchers-condition: and
    matchers:
      - type: word
        scope: request_header
        words:
          - "X-API-Key"
      - type: word
        scope: request_body
        words:
          - "username"
      - type: word
        scope: body
        words:
          - "success"
    extractors:
      - name: api_key
        type: regex
        scope: request_header
        regex:
          - 'X-API-Key: ([\w-]+)'
        group: 1
      - name: response_token
        type: regex
        scope: body
        regex:
          - 'token":"(\w+)'
        group: 1`

	matched := false
	var extractedAPIKey, extractedToken string

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Check request has expected headers and body
		body, _ := io.ReadAll(request.Body)
		if strings.Contains(string(body), "username") && request.Header.Get("X-API-Key") != "" {
			writer.WriteHeader(200)
			writer.Write([]byte(`{"status": "success", "token":"abc123xyz"}`))
		} else {
			writer.WriteHeader(401)
			writer.Write([]byte(`{"status": "failed"}`))
		}
	})
	addr := utils.HostPort(host, port)

	yakTemplate, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(yamlTpl)
	require.NoError(t, err)

	_, err = yakTemplate.ExecWithUrl("http://"+addr, httptpl.NewConfig(
		httptpl.WithResultCallback(func(y *httptpl.YakTemplate, reqBulk *httptpl.YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
			matched = result
			if val, ok := extractor["api_key"]; ok {
				extractedAPIKey = utils.InterfaceToString(val)
			}
			if val, ok := extractor["response_token"]; ok {
				extractedToken = utils.InterfaceToString(val)
			}
		}),
	))
	require.NoError(t, err)

	assert.True(t, matched, "Combined request and response matchers should match")
	assert.Equal(t, "secret-key-123", extractedAPIKey, "Should extract API key from request header")
	assert.Equal(t, "abc123xyz", extractedToken, "Should extract token from response body")
}

// TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_MultiRequest tests request scope with multiple requests
func TestGRPCMUSTPASS_HTTPFuzzer_RequestScope_MultiRequest(t *testing.T) {
	yamlTpl := `id: test-multi-request
http:
  - raw:
      - |
        GET /api/step1 HTTP/1.1
        Host: {{Hostname}}
        X-Step: 1
      - |
        POST /api/step2 HTTP/1.1
        Host: {{Hostname}}
        X-Step: 2
        Content-Type: application/json

        {"data": "test"}
    disable-cookie: true
    matchers:
      - id: 1
        type: word
        scope: request_header
        words:
          - "X-Step: 1"
      - id: 2
        type: word
        scope: request_body
        words:
          - "data"`

	requestCount := 0
	matched := false

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		writer.WriteHeader(200)
		writer.Write([]byte(fmt.Sprintf("OK-%d", requestCount)))
	})
	addr := utils.HostPort(host, port)

	yakTemplate, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(yamlTpl)
	require.NoError(t, err)

	_, err = yakTemplate.ExecWithUrl("http://"+addr, httptpl.NewConfig(
		httptpl.WithResultCallback(func(y *httptpl.YakTemplate, reqBulk *httptpl.YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
			matched = result
		}),
	))
	require.NoError(t, err)

	assert.Equal(t, 2, requestCount, "Should send 2 requests")
	assert.True(t, matched, "Should match request scope in multi-request scenario")
}
