package scannode

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/node"
)

func TestDownloadAndRewriteManagedSourcePayloadUsesTrustedNodeBaseAndSession(t *testing.T) {
	const archive = "PK\x03\x04source-payload"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/legion/v1/ssa-source-payloads/sourcepayload-123/download", request.URL.Path)
		assert.Equal(t, "session-123", request.URL.Query().Get("node_session_id"))
		assert.Equal(t, "Bearer secret-session-token", request.Header.Get("Authorization"))
		_, _ = io.WriteString(writer, archive)
	}))
	defer server.Close()

	params := map[string]any{
		"config": `{"Mode":127,"CodeSource":{"kind":"compression","url":"http://127.0.0.1:1/forged-token-sink","payload_id":"sourcepayload-123"}}`,
	}
	reference, err := managedCodeSource(params)
	require.NoError(t, err)
	require.NotNil(t, reference)
	cleanup, err := downloadAndRewriteManagedSourcePayload(
		context.Background(),
		server.Client(),
		server.URL+"/legion",
		node.SessionState{SessionID: "session-123", SessionToken: "secret-session-token"},
		reference,
	)
	require.NoError(t, err)
	require.NotNil(t, cleanup)

	var rewrittenConfig map[string]any
	require.NoError(t, json.Unmarshal([]byte(params["config"].(string)), &rewrittenConfig))
	rewrittenCodeSource := rewrittenConfig["CodeSource"].(map[string]any)
	localFile, ok := rewrittenCodeSource["local_file"].(string)
	require.True(t, ok)
	contents, err := os.ReadFile(localFile)
	require.NoError(t, err)
	assert.Equal(t, archive, string(contents))
	assert.NotContains(t, rewrittenCodeSource, "url", "the forged task URL must not survive the trusted rewrite")

	cleanup()
	_, err = os.Stat(localFile)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestManagedCodeSourceLeavesDirectArchiveURLUnchanged(t *testing.T) {
	codeSource := map[string]any{
		"kind": "compression",
		"url":  "https://archives.example.test/source.zip",
	}
	params := map[string]any{"CodeSource": codeSource}

	reference, err := managedCodeSource(params)
	require.NoError(t, err)
	assert.Nil(t, reference)
	assert.Equal(t, "https://archives.example.test/source.zip", codeSource["url"])
}

func TestDownloadManagedSourcePayloadRejectsInvalidInputBeforeHTTP(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		session   node.SessionState
		payloadID string
	}{
		{
			name:      "payload path injection",
			baseURL:   "https://platform.example.test",
			session:   node.SessionState{SessionID: "session-123", SessionToken: "token-123"},
			payloadID: "../../secret",
		},
		{
			name:      "missing session token",
			baseURL:   "https://platform.example.test",
			session:   node.SessionState{SessionID: "session-123"},
			payloadID: "sourcepayload-123",
		},
		{
			name:      "non HTTP platform URL",
			baseURL:   "file:///tmp/platform",
			session:   node.SessionState{SessionID: "session-123", SessionToken: "token-123"},
			payloadID: "sourcepayload-123",
		},
		{
			name:      "platform URL with query",
			baseURL:   "https://platform.example.test?redirect=other",
			session:   node.SessionState{SessionID: "session-123", SessionToken: "token-123"},
			payloadID: "sourcepayload-123",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := downloadManagedSourcePayload(
				context.Background(),
				nil,
				test.baseURL,
				test.session,
				test.payloadID,
			)
			require.Error(t, err)
		})
	}
}

func TestDownloadManagedSourcePayloadRejectsUnauthorizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := downloadManagedSourcePayload(
		context.Background(),
		server.Client(),
		server.URL,
		node.SessionState{SessionID: "session-123", SessionToken: "token-123"},
		"sourcepayload-123",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status=401")
}

func TestDownloadManagedSourcePayloadDoesNotForwardSessionThroughRedirect(t *testing.T) {
	var redirectTargetCalled atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectTargetCalled.Store(true)
	}))
	defer target.Close()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "Bearer token-123", request.Header.Get("Authorization"))
		http.Redirect(writer, request, target.URL+"/token-sink", http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	_, err := downloadManagedSourcePayload(
		context.Background(),
		server.Client(),
		server.URL,
		node.SessionState{SessionID: "session-123", SessionToken: "token-123"},
		"sourcepayload-123",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status=307")
	assert.False(t, redirectTargetCalled.Load(), "node session token must not follow source payload redirects")
}
