package aibalance

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aibalanceclient"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func bindClientUserIDTestProfile(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	oldDB, oldPath := consts.GetGormProfileDatabase(), consts.GetCurrentProfileDatabasePath()
	root := t.TempDir()
	t.Setenv("YAKIT_HOME", root)
	path := filepath.Join(root, "profile.db")
	db, err := gorm.Open("sqlite3", path)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	id := uuid.NewString()
	require.NoError(t, yakit.SetKey(db, aibalanceclient.TRACE_ID_DB_KEY, id))
	consts.BindProfileDatabase(db, path)
	t.Cleanup(func() {
		consts.BindProfileDatabase(oldDB, oldPath)
		_ = db.Close()
	})
	return db, id
}

func TestClientUserIDGatewayHeaderOverrides(t *testing.T) {
	_, id := bindClientUserIDTestProfile(t)
	g := &GatewayClient{config: &aispec.AIConfig{
		Model: "fixture", APIKey: "fixture-key",
		Headers: []*ypb.KVPair{
			{Key: "x-user-id", Value: "custom-a"},
			{Key: "X-USER-ID", Value: "custom-b"},
		},
	}}
	opts, err := g.BuildHTTPOptions()
	require.NoError(t, err)
	raw := poc.BuildRequest("GET / HTTP/1.1\r\nHost: example.com\r\nX-User-Id: old-a\r\nx-user-id: old-b\r\n\r\n", opts...)
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(raw)))
	require.NoError(t, err)
	require.Equal(t, []string{id}, req.Header.Values(aibalanceclient.ClientUserIDHeader))
	require.Equal(t, "Bearer fixture-key", req.Header.Get("Authorization"))
}

func TestClientUserIDGatewayStorageFailure(t *testing.T) {
	db, _ := bindClientUserIDTestProfile(t)
	require.NoError(t, db.Close())
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, nil, 0600))
	t.Setenv("YAKIT_HOME", blocker)
	g := &GatewayClient{config: &aispec.AIConfig{Model: "fixture",
		Headers: []*ypb.KVPair{{Key: "X-User-Id", Value: "untrusted-override"}},
	}}
	opts, err := g.BuildHTTPOptions()
	require.NoError(t, err, "storage failure must not disable AI requests")
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(poc.BuildRequest("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", opts...))))
	require.NoError(t, err)
	require.Empty(t, req.Header.Values(aibalanceclient.ClientUserIDHeader))
}

func TestClientUserIDGatewayRequests(t *testing.T) {
	_, id := bindClientUserIDTestProfile(t)
	previous := aibalanceclient.GetCachedSecret()
	aibalanceclient.SetCachedSecret("test-secret")
	t.Cleanup(func() { aibalanceclient.SetCachedSecret(previous) })
	type received struct{ path, id string }
	requests := make(chan received, 10)
	chatAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- received{r.URL.Path, r.Header.Get(aibalanceclient.ClientUserIDHeader)}
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"fixture","object":"model"}]}`)
		case "/v1/memfit-totp-uuid":
			fmt.Fprint(w, `{"uuid":"MEMFIT-AItest-secretMEMFIT-AI"}`)
		default:
			chatAttempts++
			if chatAttempts == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":{"type":"memfit_totp_auth_failed","message":"Memfit TOTP authentication failed"}}`)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	g := &GatewayClient{}
	g.LoadOption(aispec.WithAPIKey("fixture"), aispec.WithModel("memfit-test"))
	g.targetUrl = server.URL + "/v1/chat/completions"
	result, err := g.Chat("first")
	require.NoError(t, err)
	require.Contains(t, result, "ok")
	g.config.StreamHandler = func(r io.Reader) { _, _ = io.Copy(io.Discard, r) }
	_, err = g.Chat("stream")
	require.NoError(t, err)
	models, err := g.GetModelList()
	require.NoError(t, err)
	require.Len(t, models, 1)
	close(requests)
	var paths []string
	for req := range requests {
		require.Equal(t, id, req.id)
		paths = append(paths, req.path)
	}
	require.Equal(t, []string{"/v1/chat/completions", "/v1/memfit-totp-uuid",
		"/v1/chat/completions", "/v1/chat/completions", "/v1/models"}, paths)
}
