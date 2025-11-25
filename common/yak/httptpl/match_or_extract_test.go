package httptpl

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestMatchOrExtractHTTPFlow_Basic(t *testing.T) {
	req := "GET /login HTTP/1.1\r\nHost: example.com\r\n\r\n"
	rsp := "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\nhello token=abc123"

	yamlString := `
matchers:
  - type: word
    words:
      - "token"
    scope: body
extractors:
  - name: token
    type: regex
    regex:
      - "token=([a-z0-9]+)"
    group: 1
    scope: body
`

	result, err := MatchOrExtractHTTPFlow(req, rsp, yamlString)
	require.NoError(t, err)
	require.True(t, result.IsMatched)
	require.Equal(t, "abc123", result.Extracted["token"])
}

func TestMatchOrExtractHTTPFlow_WithVarsAndHTTPS(t *testing.T) {
	req := "POST /submit HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhello"
	rsp := "HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\n\r\n{\"ok\":false}"

	yamlString := `
matchers:
  - type: dsl
    dsl:
      - custom_flag == "ALLOW"
extractors:
  - name: request_url
    type: dsl
    dsl:
      - request_url
    scope: raw
`

	result, err := MatchOrExtractHTTPFlow(req, rsp, yamlString,
		MatchOrExtractHTTPS(true),
		MatchOrExtractVars(map[string]any{"custom_flag": "ALLOW"}),
	)
	require.NoError(t, err)
	require.True(t, result.IsMatched)
	require.Equal(t, "https://example.com/submit", result.Extracted["request_url"])
}

func TestMatchOrExtractHTTPFlow_TemplateYaml(t *testing.T) {
	req := "GET /api HTTP/1.1\r\nHost: example.com\r\n\r\n"
	rsp := "HTTP/1.1 200 OK\r\n\r\nhello template"
	yamlString := `
id: sample
info:
  name: demo
http:
  - method: GET
    path:
      - /api
    matchers:
      - type: word
        words:
          - "template"
    extractors:
      - name: status
        type: dsl
        dsl:
          - status_code
`

	result, err := MatchOrExtractHTTPFlow(req, rsp, yamlString)
	require.NoError(t, err)
	require.True(t, result.IsMatched)
	require.Equal(t, "200", result.Extracted["status"])
}

func TestMatchOrExtractHTTPFlow_RequestResponseStructs(t *testing.T) {
	req, err := http.NewRequest("GET", "https://example.com/login", strings.NewReader(""))
	require.NoError(t, err)
	rsp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.1",
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader("token=struct")),
		Request:    req,
	}

	yamlString := `
matchers:
  - type: regex
    regex:
      - "token=([a-z]+)"
extractors:
  - name: req_url
    type: dsl
    dsl:
      - request_url
`
	result, err := MatchOrExtractHTTPFlow(req, rsp, yamlString)
	require.NoError(t, err)
	require.True(t, result.IsMatched)
	require.Equal(t, "https://example.com/login", result.Extracted["req_url"])
}

func TestMatchOrExtractHTTPFlow_LowhttpResponseFallback(t *testing.T) {
	lowResp := &lowhttp.LowhttpResponse{
		RawPacket:  []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nbody token=low"),
		RawRequest: []byte("GET /fallback HTTP/1.1\r\nHost: example.com\r\n\r\n"),
		Https:      true,
	}

	yamlString := `
matchers:
  - type: word
    words:
      - token=low
extractors:
  - name: url
    type: dsl
    dsl:
      - request_url
`

	result, err := MatchOrExtractHTTPFlow(nil, lowResp, yamlString)
	require.NoError(t, err)
	require.True(t, result.IsMatched)
	require.Equal(t, "https://example.com/fallback", result.Extracted["url"])
}
