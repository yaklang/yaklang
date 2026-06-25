package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// TestTask2007_CatalogRecovery simulates post-fix pipeline recovery on task 2007 artifacts.
func TestTask2007_CatalogRecovery(t *testing.T) {
	taskDir := "/home/murkfox/yakit-projects/aispace/2007_publiccms_ssa_scan_20260606_ac3dd"
	if _, err := os.Stat(taskDir); err != nil {
		t.Skip("task 2007 workdir not available")
	}

	workDir := t.TempDir()
	discoveryDir := filepath.Join(workDir, store.SubDirName())
	require.NoError(t, os.MkdirAll(discoveryDir, 0o755))

	for _, name := range []string{
		"code_reading_stage_1.json", "code_reading_stage_2.json", "code_reading_stage_3.json",
		"code_reading_plan.json", "project_profile.json", "routing_profile.json",
	} {
		copyFile(t, filepath.Join(taskDir, store.SubDirName(), name), filepath.Join(discoveryDir, name))
	}

	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), CodePathOK: true,
		TargetScheme: "http", TargetHost: "192.168.1.4", TargetPort: "8080",
	}
	require.NoError(t, repo.CreateSession(sess))

	// Simulate static_hint endpoints like task 2007
	require.NoError(t, repo.CreateHttpEndpoint(&store.HttpEndpoint{
		SessionID: sess.ID, Method: "GET", PathPattern: "/dict/save",
		Source: SourceStaticHint, Status: "candidate",
	}))

	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}

	require.NoError(t, EnsureCodeReadingPlanFile(rt))
	plan, err := LoadCodeReadingPlan(workDir)
	require.NoError(t, err)
	require.NotEmpty(t, plan.DiscoveredAPIs)

	n, err := SyncAICodeReadingRoutesToEndpoints(rt)
	require.NoError(t, err)
	require.Greater(t, n, 0)

	catalog, err := AssembleApiCatalogFromDB(rt)
	require.NoError(t, err)
	require.NotEmpty(t, catalog.Entries)
	require.FileExists(t, store.ApiCatalogPath(workDir))

	stages, err := loadAllCodeReadingStages(workDir)
	require.NoError(t, err)
	require.NotEmpty(t, stages)
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		return
	}
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, b, 0o644))
}

func TestLoadCodeReadingPlan_AllowsNullDiscoveredAPIs(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))
	require.NoError(t, os.WriteFile(store.CodeReadingPlanPath(workDir), []byte(`{"discovered_apis":null}`), 0o644))
	plan, err := LoadCodeReadingPlan(workDir)
	require.NoError(t, err)
	require.Empty(t, plan.DiscoveredAPIs)
}

func TestMergeStagesToCodeReadingPlan_Task2007Sample(t *testing.T) {
	b, err := os.ReadFile("/home/murkfox/yakit-projects/aispace/2007_publiccms_ssa_scan_20260606_ac3dd/ssa_discovery/code_reading_stage_1.json")
	if err != nil {
		t.Skip("task 2007 stage_1 not available")
	}
	var st CodeReadingStageOutput
	require.NoError(t, json.Unmarshal(b, &st))
	plan := mergeStagesToCodeReadingPlan([]CodeReadingStageOutput{st}, nil)
	require.Contains(t, plan.DiscoveredAPIs[0].PathPattern, "/admin")
}
