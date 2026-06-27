package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestYakRouteHarvest_JavaSpring(t *testing.T) {
	inv := newFakeInvoker(t)
	workdir := t.TempDir()
	db, sessUUID := setupTestSQLiteSession(t, workdir)
	defer func() { _ = db.DB().Close() }()

	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		require.Equal(t, ToolRouteCoreHarvest, toolName)

		sessionID := getSessionID(t, db, sessUUID)
		db.Exec("INSERT INTO http_endpoints (session_id, method, path_pattern, source, created_at, updated_at) VALUES (?, 'GET', '/api/items', 'yak_java_mapping', datetime('now'), datetime('now'))", sessionID)
		db.Exec("INSERT INTO http_endpoints (session_id, method, path_pattern, source, created_at, updated_at) VALUES (?, 'POST', '/api/items', 'yak_java_mapping', datetime('now'), datetime('now'))", sessionID)
		meta := `{"tool":"api_route_harvest","endpoints":2,"sqlite_written":true,"sqlite_inserted":2}`
		db.Exec("UPDATE discovery_sessions SET endpoint_harvest_meta_json = ? WHERE uuid = ?", meta, sessUUID)

		return &aitool.ToolResult{Success: true, Data: meta}, true, nil
	}

	repo := store.NewRepository(db)
	sess, err := repo.GetSessionByUUID(sessUUID)
	require.NoError(t, err)

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: workdir, SQLitePath: store.DBPath(workdir)}
	content, err := executeYakTool(inv, context.Background(), ToolRouteCoreHarvest, rt, nil)
	require.NoError(t, err)
	require.Contains(t, content, "endpoints")

	epCount := countHttpEndpoints(t, db, sessUUID)
	require.Equal(t, 2, epCount)

	meta := getSessionMeta(t, db, sessUUID, "endpoint_harvest_meta_json")
	require.NotEmpty(t, meta)
}

func TestYakRouteHarvest_MultiLang(t *testing.T) {
	inv := newFakeInvoker(t)
	workdir := t.TempDir()
	db, sessUUID := setupTestSQLiteSession(t, workdir)
	defer func() { _ = db.DB().Close() }()

	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		sessionID := getSessionID(t, db, sessUUID)
		db.Exec("INSERT INTO http_endpoints (session_id, method, path_pattern, source, created_at, updated_at) VALUES (?, 'GET', '/api/products', 'yak_go_verb', datetime('now'), datetime('now'))", sessionID)
		db.Exec("INSERT INTO http_endpoints (session_id, method, path_pattern, source, created_at, updated_at) VALUES (?, 'POST', '/api/orders', 'yak_express_like', datetime('now'), datetime('now'))", sessionID)
		db.Exec("INSERT INTO http_endpoints (session_id, method, path_pattern, source, created_at, updated_at) VALUES (?, 'GET', '/api/items', 'yak_java_mapping', datetime('now'), datetime('now'))", sessionID)
		return &aitool.ToolResult{Success: true, Data: `{"endpoints":3}`}, true, nil
	}

	repo := store.NewRepository(db)
	sess, err := repo.GetSessionByUUID(sessUUID)
	require.NoError(t, err)

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: workdir, SQLitePath: store.DBPath(workdir)}
	_, err = executeYakTool(inv, context.Background(), ToolRouteCoreHarvest, rt, nil)
	require.NoError(t, err)

	epCount := countHttpEndpoints(t, db, sessUUID)
	require.Equal(t, 3, epCount)
}

func TestYakRouteHarvest_JSMethodFix(t *testing.T) {
	inv := newFakeInvoker(t)
	workdir := t.TempDir()
	db, sessUUID := setupTestSQLiteSession(t, workdir)
	defer func() { _ = db.DB().Close() }()

	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		sessionID := getSessionID(t, db, sessUUID)
		db.Exec("INSERT INTO http_endpoints (session_id, method, path_pattern, source, created_at, updated_at) VALUES (?, 'POST', '/api/orders', 'yak_express_like', datetime('now'), datetime('now'))", sessionID)
		db.Exec("INSERT INTO http_endpoints (session_id, method, path_pattern, source, created_at, updated_at) VALUES (?, 'DELETE', '/api/orders/:id', 'yak_express_like', datetime('now'), datetime('now'))", sessionID)
		return &aitool.ToolResult{Success: true, Data: `{"endpoints":2}`}, true, nil
	}

	repo := store.NewRepository(db)
	sess, err := repo.GetSessionByUUID(sessUUID)
	require.NoError(t, err)

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: workdir, SQLitePath: store.DBPath(workdir)}
	_, err = executeYakTool(inv, context.Background(), ToolRouteCoreHarvest, rt, nil)
	require.NoError(t, err)

	var methods []string
	rows, _ := db.Raw("SELECT method FROM http_endpoints WHERE session_id = (SELECT id FROM discovery_sessions WHERE uuid = ?)", sessUUID).Rows()
	defer rows.Close()
	for rows.Next() {
		var m string
		rows.Scan(&m)
		methods = append(methods, m)
	}
	require.Contains(t, methods, "POST")
	require.Contains(t, methods, "DELETE")
	require.NotContains(t, methods, "GET", "JS method bug: all methods should NOT be GET")
}

func TestYakRouteHarvest_WithPreanalysis(t *testing.T) {
	inv := newFakeInvoker(t)
	workdir := t.TempDir()
	db, sessUUID := setupTestSQLiteSession(t, workdir)
	defer func() { _ = db.DB().Close() }()

	db.Exec("UPDATE discovery_sessions SET api_preanalysis_meta_json = ? WHERE uuid = ?",
		`{"tool":"api_preanalysis_collector","route_candidates_count":5}`, sessUUID)

	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		sessionID := getSessionID(t, db, sessUUID)
		db.Exec("INSERT INTO http_endpoints (session_id, method, path_pattern, source, created_at, updated_at) VALUES (?, 'GET', '/api/users', 'yak_java_mapping', datetime('now'), datetime('now'))", sessionID)
		return &aitool.ToolResult{Success: true, Data: `{"endpoints":1}`}, true, nil
	}

	repo := store.NewRepository(db)
	sess, err := repo.GetSessionByUUID(sessUUID)
	require.NoError(t, err)

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: workdir, SQLitePath: store.DBPath(workdir)}
	_, err = executeYakTool(inv, context.Background(), ToolRouteCoreHarvest, rt, nil)
	require.NoError(t, err)
	require.Equal(t, 1, countHttpEndpoints(t, db, sessUUID))
}

func TestYakRouteHarvest_Upsert(t *testing.T) {
	inv := newFakeInvoker(t)
	workdir := t.TempDir()
	db, sessUUID := setupTestSQLiteSession(t, workdir)
	defer func() { _ = db.DB().Close() }()

	sessionID := getSessionID(t, db, sessUUID)
	db.Exec("INSERT INTO http_endpoints (session_id, method, path_pattern, source, created_at, updated_at) VALUES (?, 'GET', '/api/users', 'ai', datetime('now'), datetime('now'))", sessionID)
	require.Equal(t, 1, countHttpEndpoints(t, db, sessUUID))

	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		// Simulate yak harvest: do not overwrite AI-primary sources.
		return &aitool.ToolResult{Success: true, Data: `{"endpoints":1,"sqlite_updated":0}`}, true, nil
	}

	repo := store.NewRepository(db)
	sess, err := repo.GetSessionByUUID(sessUUID)
	require.NoError(t, err)

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: workdir, SQLitePath: store.DBPath(workdir)}
	_, err = executeYakTool(inv, context.Background(), ToolRouteCoreHarvest, rt, nil)
	require.NoError(t, err)

	require.Equal(t, 1, countHttpEndpoints(t, db, sessUUID))

	var source string
	row := db.Raw("SELECT source FROM http_endpoints WHERE session_id = ? AND method = 'GET' AND path_pattern = '/api/users'", sessionID).Row()
	require.NoError(t, row.Scan(&source))
	require.Equal(t, "ai", source, "AI-primary source must not be overwritten by yak harvest")
}

func getSessionID(t *testing.T, db *gorm.DB, sessUUID string) uint {
	t.Helper()
	var id uint
	row := db.Raw("SELECT id FROM discovery_sessions WHERE uuid = ?", sessUUID).Row()
	require.NoError(t, row.Scan(&id))
	return id
}

// verify JSON parse of meta
func assertValidJSON(t *testing.T, s string) {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m))
}
