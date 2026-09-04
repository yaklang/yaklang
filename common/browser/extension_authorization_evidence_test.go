package browser

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func authorizationEvidenceTestResult(
	body string,
	contentType string,
) *ExtensionAuthorizationRequestExecution {
	packet := []byte(
		"HTTP/1.1 200 OK\r\nContent-Type: " + contentType +
			"\r\nContent-Length: 0\r\n\r\n" + body,
	)
	return &ExtensionAuthorizationRequestExecution{
		Version: 1, Status: 200, Outcome: "success",
		Response: ExtensionAuthorizationResponseSummary{
			ContentType:   contentType,
			CapturedBytes: len(body),
		},
		responseBody:   []byte(body),
		responsePacket: packet,
		requestPacket: []byte(
			"GET /profile HTTP/1.1\r\nHost: example.test\r\n" +
				"Cookie: session=secret\r\n\r\n",
		),
	}
}

func TestAuthorizationHTMLDifferentialProducesConfirmedEvidence(t *testing.T) {
	page := func(username, remark string) string {
		return `<html><body>` +
			`<section id="profile">` +
			`<div id="username"><span>用户名</span><strong>` + username + `</strong></div>` +
			`<div id="remark"><span>备注</span><strong>` + remark + `</strong></div>` +
			`</section></body></html>`
	}
	execution := &ExtensionAuthorizationExecution{
		Cases: []ExtensionAuthorizationCaseExecution{
			{ID: "a-own", State: "completed", Result: authorizationEvidenceTestResult(page("admin", "alpha"), "text/html; charset=utf-8")},
			{ID: "b-own", State: "completed", Result: authorizationEvidenceTestResult(page("member", "beta"), "text/html; charset=utf-8")},
			{ID: "a-to-b", State: "completed", Result: authorizationEvidenceTestResult(page("member", "beta"), "text/html; charset=utf-8")},
			{ID: "b-to-a", State: "completed", Result: authorizationEvidenceTestResult(page("admin", "alpha"), "text/html; charset=utf-8")},
		},
	}
	comparisonKey := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	evaluateExtensionAuthorizationExecution(execution, comparisonKey)

	require.Equal(t, "confirmed", execution.Verdict)
	require.Equal(t, "high", execution.Confidence)
	require.NotEmpty(t, execution.Evidence)
	require.Contains(t, execution.Evidence[0].Source, "response")
	require.True(t, strings.Contains(execution.Evidence[0].Path, "username") ||
		strings.Contains(execution.Evidence[0].Path, "remark"))
}

func TestAuthorizationEvidencePacketRedactsCredentialsAndStructuredSecrets(t *testing.T) {
	request := []byte(
		"POST /profile?access_token=query-secret&view=summary HTTP/1.1\r\n" +
			"Host: example.test\r\n" +
			"Authorization: Bearer secret\r\n" +
			"\tcontinued-secret\r\n" +
			"Cookie: session=secret\r\n" +
			"X-Request-Signature: signed-secret\r\n" +
			"Content-Type: application/json\r\n\r\n" +
			`{"username":"alice","password":"secret","token":"private"}`,
	)
	redacted := string(redactAuthorizationPacket(request))

	require.Contains(t, redacted, "Authorization: <redacted>")
	require.Contains(t, redacted, "Cookie: <redacted>")
	require.Contains(t, redacted, "access_token=%3Credacted%3E")
	require.Contains(t, redacted, "view=summary")
	require.Contains(t, redacted, `"username": "alice"`)
	require.NotContains(t, redacted, "Bearer secret")
	require.NotContains(t, redacted, "query-secret")
	require.NotContains(t, redacted, "continued-secret")
	require.NotContains(t, redacted, "signed-secret")
	require.NotContains(t, redacted, `"password":"secret"`)
	require.NotContains(t, redacted, `"token":"private"`)
}

func TestAuthorizationEvidenceDiffSeparatesSemanticAndVolatileFields(t *testing.T) {
	left := authorizationEvidenceTestResult(
		`{"user":{"id":1,"name":"alice"},"timestamp":1711111111111}`,
		"application/json",
	)
	right := authorizationEvidenceTestResult(
		`{"user":{"id":2,"name":"bob"},"timestamp":1711111112222}`,
		"application/json",
	)
	manager := &ExtensionBridgeManager{
		authorization: map[string]ExtensionAuthorizationWorkspace{
			"workspace-1": {
				ID: "workspace-1", Mode: "horizontal",
				ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
				Execution: &ExtensionAuthorizationExecution{
					ID: "execution-1",
					Cases: []ExtensionAuthorizationCaseExecution{
						{ID: "a-own", State: "completed", Result: left},
						{ID: "b-own", State: "completed", Result: right},
					},
				},
			},
		},
	}
	diff, err := manager.DiffExtensionAuthorizationEvidence(
		context.Background(),
		ExtensionAuthorizationEvidenceDiffInput{
			WorkspaceID: "workspace-1", ExecutionID: "execution-1",
			LeftCaseID: "a-own", RightCaseID: "b-own",
		},
	)
	require.NoError(t, err)
	require.Equal(t, "structured", diff.Representation)
	require.Len(t, diff.Entries, 3)
	byPath := make(map[string]ExtensionAuthorizationEvidenceDiffEntry)
	for _, entry := range diff.Entries {
		byPath[entry.Path] = entry
	}
	require.True(t, byPath["body.user.id"].Semantic)
	require.True(t, byPath["body.user.name"].Semantic)
	require.True(t, byPath["body.timestamp"].Volatile)
	require.False(t, byPath["body.timestamp"].Semantic)
	require.Equal(t, "body.timestamp", diff.Entries[len(diff.Entries)-1].Path)
}

func TestAuthorizationEvidenceDiffRedactsRawPacketFallback(t *testing.T) {
	left := authorizationEvidenceTestResult("plain left", "text/plain")
	right := authorizationEvidenceTestResult("plain right", "text/plain")
	left.requestPacket = []byte(
		"GET /profile HTTP/1.1\r\nHost: example.test\r\n" +
			"Authorization: Bearer left-secret\r\nCookie: session=left-secret\r\n" +
			"X-Test-Role: left\r\n\r\n",
	)
	right.requestPacket = []byte(
		"GET /profile HTTP/1.1\r\nHost: example.test\r\n" +
			"Authorization: Bearer right-secret\r\nCookie: session=right-secret\r\n" +
			"X-Test-Role: right\r\n\r\n",
	)
	manager := &ExtensionBridgeManager{
		authorization: map[string]ExtensionAuthorizationWorkspace{
			"workspace-1": {
				ID: "workspace-1", Mode: "horizontal",
				ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
				Execution: &ExtensionAuthorizationExecution{
					ID: "execution-1",
					Cases: []ExtensionAuthorizationCaseExecution{
						{ID: "a-own", State: "completed", Result: left},
						{ID: "b-own", State: "completed", Result: right},
					},
				},
			},
		},
	}
	diff, err := manager.DiffExtensionAuthorizationEvidence(
		context.Background(),
		ExtensionAuthorizationEvidenceDiffInput{
			WorkspaceID: "workspace-1", ExecutionID: "execution-1",
			LeftCaseID: "a-own", RightCaseID: "b-own",
			Scope: "request", View: "redacted",
		},
	)
	require.NoError(t, err)
	require.Equal(t, "raw", diff.Representation)
	encoded, err := json.Marshal(diff)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "left-secret")
	require.NotContains(t, string(encoded), "right-secret")
	require.Len(t, diff.Entries, 1)
	require.Equal(t, "request.headers.x-test-role[0]", diff.Entries[0].Path)
	require.Equal(t, "left", diff.Entries[0].Left)
	require.Equal(t, "right", diff.Entries[0].Right)
}

func TestAuthorizationEvidenceDiffSummarizesBinaryBodies(t *testing.T) {
	left := authorizationEvidenceTestResult("", "application/octet-stream")
	right := authorizationEvidenceTestResult("", "application/octet-stream")
	leftBody := []byte{0, 1, 2, 3}
	rightBody := []byte{0, 1, 2, 4}
	left.responseBody = leftBody
	right.responseBody = rightBody
	left.responsePacket = append(
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\n\r\n"),
		leftBody...,
	)
	right.responsePacket = append(
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\n\r\n"),
		rightBody...,
	)
	left.Response.AnalysisRepresentation = "binary"
	right.Response.AnalysisRepresentation = "binary"
	left.Response.ValueFingerprint = "workspace-hmac-sha256:" + strings.Repeat("a", 64)
	right.Response.ValueFingerprint = "workspace-hmac-sha256:" + strings.Repeat("b", 64)
	manager := &ExtensionBridgeManager{
		authorization: map[string]ExtensionAuthorizationWorkspace{
			"workspace-1": {
				ID: "workspace-1", Mode: "horizontal",
				ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
				Execution: &ExtensionAuthorizationExecution{
					ID: "execution-1",
					Cases: []ExtensionAuthorizationCaseExecution{
						{ID: "a-own", State: "completed", Result: left},
						{ID: "b-own", State: "completed", Result: right},
					},
				},
			},
		},
	}
	diff, err := manager.DiffExtensionAuthorizationEvidence(
		context.Background(),
		ExtensionAuthorizationEvidenceDiffInput{
			WorkspaceID: "workspace-1", ExecutionID: "execution-1",
			LeftCaseID: "a-own", RightCaseID: "b-own",
		},
	)
	require.NoError(t, err)
	require.Equal(t, "raw", diff.Representation)
	require.Len(t, diff.Entries, 1)
	require.Equal(t, "response.body.binary.fingerprint", diff.Entries[0].Path)
	require.Contains(t, diff.Entries[0].Left, "workspace-hmac-sha256:")
	require.Contains(t, diff.Entries[0].Right, "workspace-hmac-sha256:")
}

func TestAuthorizationUUIDResourceIdentityIsNotClassifiedAsVolatile(t *testing.T) {
	require.False(t, authorizationVolatileDiff(
		"body.order.id",
		[]byte("550e8400-e29b-41d4-a716-446655440000"),
	))
	require.True(t, authorizationVolatileDiff(
		"body.traceId",
		[]byte("550e8400-e29b-41d4-a716-446655440000"),
	))
}

func TestAuthorizationRawPacketDiffMarksDynamicHeadersSeparately(t *testing.T) {
	left := authorizationEvidenceTestResult("same body", "text/plain")
	right := authorizationEvidenceTestResult("same body", "text/plain")
	left.responsePacket = []byte(
		"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n" +
			"Date: Wed, 30 Jul 2026 10:00:00 GMT\r\n" +
			"X-Request-ID: request-left\r\n\r\nsame body",
	)
	right.responsePacket = []byte(
		"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n" +
			"Date: Wed, 30 Jul 2026 10:00:01 GMT\r\n" +
			"X-Request-ID: request-right\r\n\r\nsame body",
	)
	manager := &ExtensionBridgeManager{
		authorization: map[string]ExtensionAuthorizationWorkspace{
			"workspace-1": {
				ID: "workspace-1", Mode: "horizontal",
				ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
				Execution: &ExtensionAuthorizationExecution{
					ID: "execution-1",
					Cases: []ExtensionAuthorizationCaseExecution{
						{ID: "a-own", State: "completed", Result: left},
						{ID: "b-own", State: "completed", Result: right},
					},
				},
			},
		},
	}
	diff, err := manager.DiffExtensionAuthorizationEvidence(
		context.Background(),
		ExtensionAuthorizationEvidenceDiffInput{
			WorkspaceID: "workspace-1", ExecutionID: "execution-1",
			LeftCaseID: "a-own", RightCaseID: "b-own",
			Scope: "response", View: "redacted",
		},
	)
	require.NoError(t, err)
	require.Equal(t, "raw", diff.Representation)
	require.Len(t, diff.Entries, 2)
	for _, entry := range diff.Entries {
		require.True(t, entry.Volatile, entry.Path)
		require.False(t, entry.Semantic, entry.Path)
	}
}

func TestAuthorizationEvidencePacketsStayOutOfWorkspaceJSON(t *testing.T) {
	result := authorizationEvidenceTestResult(
		`{"profile":{"id":2}}`,
		"application/json",
	)
	result.requestPacket = append(
		result.requestPacket,
		[]byte("X-Private-Marker: request-secret\r\n")...,
	)
	result.responsePacket = append(
		result.responsePacket,
		[]byte("response-secret")...,
	)
	workspace := ExtensionAuthorizationWorkspace{
		ID: "workspace-1",
		Execution: &ExtensionAuthorizationExecution{
			ID: "execution-1",
			Cases: []ExtensionAuthorizationCaseExecution{{
				ID: "a-own", State: "completed", Result: result,
			}},
			EvidenceAvailable: true,
		},
	}
	encoded, err := json.Marshal(workspace)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "request-secret")
	require.NotContains(t, string(encoded), "response-secret")
	require.NotContains(t, string(encoded), "requestPacket")
	require.NotContains(t, string(encoded), "responsePacket")
	require.Contains(t, string(encoded), `"evidenceAvailable":true`)
}

func TestAuthorizationEvidenceValidationUpgradesOnlyDeterministicallyVerifiedPath(t *testing.T) {
	result := func(userID int, remark string) *ExtensionAuthorizationRequestExecution {
		item := authorizationEvidenceTestResult(
			fmt.Sprintf(`{"profile":{"userId":%d,"remark":%q}}`, userID, remark),
			"application/json",
		)
		item.responseBody = nil
		return item
	}
	comparisonKey := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32))
	execution := &ExtensionAuthorizationExecution{
		ID: "execution-1", Verdict: "likely", Confidence: "high",
		Cases: []ExtensionAuthorizationCaseExecution{
			{ID: "a-own", State: "completed", Result: result(1, "alpha")},
			{ID: "b-own", State: "completed", Result: result(2, "beta")},
			{ID: "a-to-b", State: "completed", Result: result(2, "beta")},
			{ID: "b-to-a", State: "completed", Result: result(1, "alpha")},
		},
	}
	manager := &ExtensionBridgeManager{
		authorization: map[string]ExtensionAuthorizationWorkspace{
			"workspace-1": {
				ID: "workspace-1", Mode: "horizontal",
				ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
				Execution: execution, comparisonKey: comparisonKey,
			},
		},
	}
	validation, err := manager.ValidateExtensionAuthorizationEvidence(
		context.Background(),
		ExtensionAuthorizationEvidenceValidationInput{
			WorkspaceID: "workspace-1", ExecutionID: "execution-1",
			Direction: "a-to-b",
			Paths: []string{
				"body.profile.userId",
				"body.profile.missing",
			},
		},
	)
	require.NoError(t, err)
	require.True(t, validation.Verified)
	require.True(t, validation.VerdictChanged)
	require.Equal(t, "confirmed", validation.Verdict)
	require.Len(t, validation.Evidence, 1)
	require.Equal(t, "body.profile.userId", validation.Evidence[0].Path)
	require.Equal(t, []string{"body.profile.missing"}, validation.RejectedPaths)

	stored, err := manager.GetExtensionAuthorizationWorkspace(
		context.Background(),
		"workspace-1",
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "confirmed", stored.Execution.Verdict)
	require.NotEmpty(t, stored.Execution.Evidence)

	rejected, err := manager.ValidateExtensionAuthorizationEvidence(
		context.Background(),
		ExtensionAuthorizationEvidenceValidationInput{
			WorkspaceID: "workspace-1", ExecutionID: "execution-1",
			Direction: "b-to-a", Paths: []string{"body.profile.missing"},
		},
	)
	require.NoError(t, err)
	require.False(t, rejected.Verified)
	require.NotNil(t, rejected.Evidence)
	require.Empty(t, rejected.Evidence)
	encoded, err := json.Marshal(rejected)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"evidence":[]`)
	require.NotContains(t, string(encoded), `"evidence":null`)
}

func TestAuthorizationRequestStopsAfterFirstKeepAliveResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"user":{"id":1}}`))
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	packet := []byte(
		"GET / HTTP/1.1\r\nHost: " + parsed.Host +
			"\r\nConnection: keep-alive\r\n\r\n",
	)
	baseline := &ExtensionAuthorizationBaseline{
		ID: "baseline-1",
		Request: ExtensionAuthorizationBaselineRequest{
			Method: "GET", URL: server.URL,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	startedAt := time.Now()
	result, err := executeAuthorizationCompiledRequest(
		ctx,
		extensionAuthorizationCompiledRequest{IsHTTPS: false},
		packet,
		baseline,
		ExtensionAuthorizationPlanSelector{
			Source: "wire", Location: "query", Path: "query.id",
		},
		base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)),
	)
	require.NoError(t, err)
	require.Less(t, time.Since(startedAt), time.Second)
	require.Equal(t, 200, result.Status)
	require.True(t, result.DurationMS < 1000)
	require.NotEmpty(t, result.requestPacket)
	require.NotEmpty(t, result.responsePacket)
}

func TestAuthorizationRequestAnalyzesBoundedGzipJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Content-Encoding", "gzip")
		var payload bytes.Buffer
		compressed := gzip.NewWriter(&payload)
		_, _ = compressed.Write([]byte(`{"user":{"id":42,"name":"yak"}}`))
		if compressed.Close() != nil {
			http.Error(writer, "compression failed", http.StatusInternalServerError)
			return
		}
		_, _ = writer.Write(payload.Bytes())
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	packet := []byte(
		"GET / HTTP/1.1\r\nHost: " + parsed.Host +
			"\r\nConnection: close\r\n\r\n",
	)
	baseline := &ExtensionAuthorizationBaseline{
		ID: "baseline-gzip",
		Request: ExtensionAuthorizationBaselineRequest{
			Method: "GET", URL: server.URL,
		},
	}
	result, err := executeAuthorizationCompiledRequest(
		context.Background(),
		extensionAuthorizationCompiledRequest{IsHTTPS: false},
		packet,
		baseline,
		ExtensionAuthorizationPlanSelector{
			Source: "wire", Location: "query", Path: "query.id",
		},
		base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)),
	)
	require.NoError(t, err)
	require.True(t, result.Response.Decoded)
	require.Equal(t, "decoded", result.Response.AnalysisState)
	require.Equal(t, "json", result.Response.AnalysisRepresentation)
	require.Equal(t, "gzip", result.Response.ContentEncoding)
	require.Equal(t, `{"user":{"id":42,"name":"yak"}}`, string(result.responseBody))
	require.NotEmpty(t, result.Response.ShapeFingerprint)
	require.Contains(t, authorizationResponseLeaves(result), "body.user.id")
}

func TestAuthorizationResponseAnalysisFailsClosedOnExpansionLimit(t *testing.T) {
	var payload bytes.Buffer
	compressed := gzip.NewWriter(&payload)
	_, err := compressed.Write(bytes.Repeat(
		[]byte("a"),
		maxAuthorizationResponseAnalysisBytes+1,
	))
	require.NoError(t, err)
	require.NoError(t, compressed.Close())
	packet := append([]byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n",
		payload.Len(),
	)), payload.Bytes()...)

	analysis := analyzeAuthorizationResponsePacket(packet)

	require.Equal(t, "encoded-unavailable", analysis.state)
	require.Equal(t, "encoded", analysis.representation)
	require.Empty(t, analysis.body)
}
