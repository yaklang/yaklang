package loop_ssa_api_discovery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func setupGatewayRT(t *testing.T) (*Runtime, func()) {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString(), CodePathOK: true, Language: "java"}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: t.TempDir()}
	return rt, func() { _ = db.DB().Close() }
}

func TestGateway_NormalInsert(t *testing.T) {
	rt, cleanup := setupGatewayRT(t)
	defer cleanup()

	ep := &store.HttpEndpoint{Method: "GET", PathPattern: "/api/users", Source: "ai"}
	res, err := EndpointInsertionGateway(rt, ep)
	require.NoError(t, err)
	require.Equal(t, "created", res.Reason)
	require.Equal(t, store.EndpointStatusPendingValidation, res.Status)
	require.NotZero(t, res.EndpointID)
}

func TestGateway_InvalidMethod(t *testing.T) {
	rt, cleanup := setupGatewayRT(t)
	defer cleanup()

	ep := &store.HttpEndpoint{Method: "TRACE", PathPattern: "/api/test", Source: "ai"}
	res, err := EndpointInsertionGateway(rt, ep)
	require.NoError(t, err)
	require.Equal(t, "rejected_at_gate", res.Status)
	require.Contains(t, res.Reason, "invalid HTTP method")
}

func TestGateway_EmptyPath(t *testing.T) {
	rt, cleanup := setupGatewayRT(t)
	defer cleanup()

	ep := &store.HttpEndpoint{Method: "GET", PathPattern: "", Source: "ai"}
	res, err := EndpointInsertionGateway(rt, ep)
	require.NoError(t, err)
	require.Equal(t, "rejected_at_gate", res.Status)
}

func TestGateway_UnsampleableWildcard(t *testing.T) {
	rt, cleanup := setupGatewayRT(t)
	defer cleanup()

	for _, path := range []string{"/**", "/*", "/.*", "regex:.*"} {
		ep := &store.HttpEndpoint{Method: "GET", PathPattern: path, Source: "ai"}
		res, err := EndpointInsertionGateway(rt, ep)
		require.NoError(t, err)
		require.Equal(t, "rejected_at_gate", res.Status, "expected rejection for %s", path)
	}
}

func TestGateway_PathTooLong(t *testing.T) {
	rt, cleanup := setupGatewayRT(t)
	defer cleanup()

	longPath := "/" + string(make([]byte, 300))
	ep := &store.HttpEndpoint{Method: "GET", PathPattern: longPath, Source: "ai"}
	res, err := EndpointInsertionGateway(rt, ep)
	require.NoError(t, err)
	require.Equal(t, "rejected_at_gate", res.Status)
	require.Contains(t, res.Reason, "too long")
}

func TestGateway_DedupeMerge(t *testing.T) {
	rt, cleanup := setupGatewayRT(t)
	defer cleanup()

	ep1 := &store.HttpEndpoint{Method: "POST", PathPattern: "/api/login", Source: "static"}
	res1, err := EndpointInsertionGateway(rt, ep1)
	require.NoError(t, err)
	require.Equal(t, "created", res1.Reason)

	ep2 := &store.HttpEndpoint{Method: "POST", PathPattern: "/api/login", HandlerClass: "AuthController", Source: "ai"}
	res2, err := EndpointInsertionGateway(rt, ep2)
	require.NoError(t, err)
	require.True(t, res2.Merged)
	require.Equal(t, res1.EndpointID, res2.EndpointID)
}

func TestGateway_BatchInsert(t *testing.T) {
	rt, cleanup := setupGatewayRT(t)
	defer cleanup()

	candidates := []store.HttpEndpoint{
		{Method: "GET", PathPattern: "/api/users", Source: "static"},
		{Method: "POST", PathPattern: "/api/users", Source: "static"},
		{Method: "INVALID", PathPattern: "/bad", Source: "static"},
		{Method: "GET", PathPattern: "/api/users", Source: "ai"},
	}
	ins, merged, rejected := EndpointInsertionGatewayBatch(rt, candidates)
	require.Equal(t, 2, ins)
	require.Equal(t, 1, merged)
	require.Equal(t, 1, rejected)
}

func TestNormalize_MethodUppercase(t *testing.T) {
	ep := &store.HttpEndpoint{Method: "get", PathPattern: "/api/test"}
	reason := NormalizeAndValidateEndpoint(ep)
	require.Empty(t, reason)
	require.Equal(t, "GET", ep.Method)
}

func TestNormalize_PathLeadingSlash(t *testing.T) {
	ep := &store.HttpEndpoint{Method: "GET", PathPattern: "api/test"}
	reason := NormalizeAndValidateEndpoint(ep)
	require.Empty(t, reason)
	require.Equal(t, "/api/test", ep.PathPattern)
}

func TestNormalize_PathDoubleSlash(t *testing.T) {
	ep := &store.HttpEndpoint{Method: "GET", PathPattern: "/api//users///list"}
	reason := NormalizeAndValidateEndpoint(ep)
	require.Empty(t, reason)
	require.Equal(t, "/api/users/list", ep.PathPattern)
}
