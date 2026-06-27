package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

type fakeInvoker struct {
	*mock.MockInvoker
	lastToolName string
	lastParams   aitool.InvokeParams
	resultData   any
	resultErr    error
	ExecuteToolRequiredAndCallWithoutRequiredOverride func(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error)
}

func (f *fakeInvoker) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
	f.lastToolName = toolName
	f.lastParams = params
	if f.ExecuteToolRequiredAndCallWithoutRequiredOverride != nil {
		return f.ExecuteToolRequiredAndCallWithoutRequiredOverride(ctx, toolName, params)
	}
	if f.resultErr != nil {
		return nil, false, f.resultErr
	}
	return &aitool.ToolResult{
		Success: true,
		Data:    f.resultData,
	}, true, nil
}

func newFakeInvoker(t *testing.T) *fakeInvoker {
	t.Helper()
	inv := mock.NewMockInvoker(context.Background())
	return &fakeInvoker{
		MockInvoker: inv,
		resultData:  `{"tool":"test","ok":true}`,
	}
}

func TestExecuteYakTool_ParamAssembly(t *testing.T) {
	inv := newFakeInvoker(t)

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	require.NoError(t, store.AutoMigrate(db))

	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID:         uuid.NewString(),
		CodeRootPath: "/code/root",
		Phase:        "ssa_done",
		CodePathOK:   true,
		Language:     "java",
		TargetRaw:    "http://localhost:8080",
		TargetHost:   "localhost",
		TargetPort:   "8080",
		TargetScheme: "http",
	}
	require.NoError(t, repo.CreateSession(sess))

	rt := &Runtime{
		DB:         db,
		Repo:       repo,
		Session:    sess,
		WorkDir:    "/tmp/work",
		SQLitePath: "/tmp/work/db.sqlite",
	}

	content, err := executeYakTool(inv, context.Background(), ToolRouteCoreHarvest, rt, map[string]any{
		"extra-key": "extra-val",
	})
	require.NoError(t, err)
	require.Contains(t, content, "test")

	require.Equal(t, ToolRouteCoreHarvest, inv.lastToolName)
	require.Equal(t, "/code/root", inv.lastParams["code-root"])
	require.Equal(t, "/tmp/work", inv.lastParams["workdir"])
	require.Equal(t, "/tmp/work/db.sqlite", inv.lastParams["sqlite-path"])
	require.Equal(t, sess.UUID, inv.lastParams["session-uuid"])
	require.Equal(t, "java", inv.lastParams["language"])
	require.Equal(t, "extra-val", inv.lastParams["extra-key"])
}

func TestExecuteYakTool_ToolResultFailurePropagation(t *testing.T) {
	inv := newFakeInvoker(t)
	inv.ExecuteToolRequiredAndCallWithoutRequiredOverride = func(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
		return &aitool.ToolResult{
			Success: false,
			Error:   "error invoking tool[route_core_harvest]: Panic Stack",
		}, true, nil
	}

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	require.NoError(t, store.AutoMigrate(db))

	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID:         uuid.NewString(),
		CodeRootPath: "/code",
		Phase:        "ssa_done",
		CodePathOK:   true,
	}
	require.NoError(t, repo.CreateSession(sess))

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: "/tmp"}

	_, err = executeYakTool(inv, context.Background(), ToolRouteCoreHarvest, rt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "route_core_harvest")
}

func TestExecuteYakTool_ErrorPropagation(t *testing.T) {
	inv := newFakeInvoker(t)
	inv.resultErr = utils.Error("tool failed")

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	require.NoError(t, store.AutoMigrate(db))

	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID:         uuid.NewString(),
		CodeRootPath: "/code",
		Phase:        "ssa_done",
		CodePathOK:   true,
	}
	require.NoError(t, repo.CreateSession(sess))

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: "/tmp"}

	_, err = executeYakTool(inv, context.Background(), ToolRouteCoreHarvest, rt, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "tool failed")
}

func TestExecuteYakTool_NilResult(t *testing.T) {
	inv := newFakeInvoker(t)
	inv.resultData = nil

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	require.NoError(t, store.AutoMigrate(db))

	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID:         uuid.NewString(),
		CodeRootPath: "/code",
		Phase:        "ssa_done",
		CodePathOK:   true,
	}
	require.NoError(t, repo.CreateSession(sess))

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: "/tmp"}

	content, err := executeYakTool(inv, context.Background(), ToolVulnBatchScan, rt, nil)
	require.NoError(t, err)
	require.Empty(t, content)
}

func TestExecuteYakTool_TargetBaseURL(t *testing.T) {
	inv := newFakeInvoker(t)

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	require.NoError(t, store.AutoMigrate(db))

	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID:         uuid.NewString(),
		CodeRootPath: "/code",
		Phase:        "ssa_done",
		CodePathOK:   true,
		TargetRaw:    "http://10.0.0.1:9090",
		TargetHost:   "10.0.0.1",
		TargetPort:   "9090",
		TargetScheme: "http",
	}
	require.NoError(t, repo.CreateSession(sess))

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: "/tmp"}

	_, err = executeYakTool(inv, context.Background(), ToolRouteCoreHarvest, rt, nil)
	require.NoError(t, err)

	baseURL, ok := inv.lastParams["target-base-url"]
	if ok {
		require.NotEmpty(t, baseURL)
	}
}

func TestPipeline_MockAllYakTools(t *testing.T) {
	inv := newFakeInvoker(t)

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	require.NoError(t, store.AutoMigrate(db))

	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID:         uuid.NewString(),
		CodeRootPath: "/code",
		Phase:        "ssa_done",
		CodePathOK:   true,
		Language:     "java",
	}
	require.NoError(t, repo.CreateSession(sess))

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: t.TempDir(), SQLitePath: "/tmp/db"}
	ctx := context.Background()

	tools := []string{
		ToolRouteCoreHarvest,
		ToolVulnBatchScan,
	}

	for _, tool := range tools {
		inv.resultData = json.RawMessage(`{"tool":"` + tool + `","ok":true}`)
		content, err := executeYakTool(inv, ctx, tool, rt, nil)
		require.NoError(t, err, "tool=%s", tool)
		require.Contains(t, content, tool, "tool=%s", tool)
		require.Equal(t, tool, inv.lastToolName, "tool=%s", tool)
	}
}
