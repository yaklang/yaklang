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

func TestLoadAllCodeReadingStages_SkipsMissingStage0(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))

	stage1 := CodeReadingStageOutput{
		Stage: 1,
		ReadFilesCompleted: []string{"Foo.java"},
		APIFragments: []APIFragment{
			{Method: "GET", PathPattern: "/admin/index", HandlerFile: "Foo.java"},
		},
	}
	stage3 := CodeReadingStageOutput{
		Stage: 3,
		APIFragments: []APIFragment{
			{Method: "POST", PathPattern: "/admin/login", HandlerFile: "Login.java"},
		},
	}
	b1, _ := json.MarshalIndent(stage1, "", "  ")
	b3, _ := json.MarshalIndent(stage3, "", "  ")
	require.NoError(t, os.WriteFile(store.CodeReadingStagePath(workDir, 1), b1, 0o644))
	require.NoError(t, os.WriteFile(store.CodeReadingStagePath(workDir, 3), b3, 0o644))

	stages, err := loadAllCodeReadingStages(workDir)
	require.NoError(t, err)
	require.Len(t, stages, 1, "stage_3 should not load after gap at stage_2")
	require.Equal(t, 1, stages[0].Stage)
	require.Len(t, stages[0].APIFragments, 1)
}

func TestBuildCodeReadingPlanFromStageFiles_MergesStages(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))

	stage1 := CodeReadingStageOutput{
		Stage: 1,
		APIFragments: []APIFragment{
			{Method: "GET", PathPattern: "/admin/index", HandlerFile: "Index.java"},
			{Method: "POST", PathPattern: "/admin/login", HandlerFile: "Login.java"},
		},
		RoutingFacts: []RoutingFact{{Kind: "class_mapping", MountPrefix: "/admin", Ref: "AdminConfig.java"}},
	}
	b1, _ := json.MarshalIndent(stage1, "", "  ")
	require.NoError(t, os.WriteFile(store.CodeReadingStagePath(workDir, 1), b1, 0o644))

	rt := &Runtime{WorkDir: workDir}
	plan, err := BuildCodeReadingPlanFromStageFiles(rt)
	require.NoError(t, err)
	require.Len(t, plan.DiscoveredAPIs, 2)
}

func TestEnsureCodeReadingPlanFile_RebuildsEmptyPlanFromStages(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))

	emptyPlan := `{"discovered_apis":null,"hint_diff":"staged code reading (evidence-first)"}`
	require.NoError(t, os.WriteFile(store.CodeReadingPlanPath(workDir), []byte(emptyPlan), 0o644))

	stage1 := CodeReadingStageOutput{
		Stage: 1,
		APIFragments: []APIFragment{
			{Method: "POST", PathPattern: "/admin/login", HandlerFile: "Login.java"},
		},
	}
	b1, _ := json.MarshalIndent(stage1, "", "  ")
	require.NoError(t, os.WriteFile(store.CodeReadingStagePath(workDir, 1), b1, 0o644))

	rt := &Runtime{
		WorkDir: workDir,
		Session: &store.DiscoverySession{CodePathOK: true},
	}
	require.NoError(t, EnsureCodeReadingPlanFile(rt))

	plan, err := LoadCodeReadingPlan(workDir)
	require.NoError(t, err)
	require.Len(t, plan.DiscoveredAPIs, 1)
	require.Equal(t, "/admin/login", plan.DiscoveredAPIs[0].PathPattern)
}

func TestBuildCodeReadingPlanFromDB_StaticHintOnlyMany(t *testing.T) {
	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString(), CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))

	for i := 0; i < 25; i++ {
		require.NoError(t, repo.CreateHttpEndpoint(&store.HttpEndpoint{
			SessionID: sess.ID, Method: "GET",
			PathPattern: "/api/route" + string(rune('a'+i)),
			Source: SourceStaticHint, Status: "candidate",
		}))
	}

	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}
	plan, err := BuildCodeReadingPlanFromDB(rt)
	require.NoError(t, err)
	require.Len(t, plan.DiscoveredAPIs, 25)
}

func TestSanitizeReconWorklist_RejectsGlobPaths(t *testing.T) {
	rt := &Runtime{}
	items := []WorklistSeedItem{
		{RelPath: "**/*Controller.java", Category: worklistCategoryAPIHandler},
		{RelPath: "**/WebConfig.java", Category: worklistCategoryRoutingConfig},
	}
	out := sanitizeReconWorklist(rt, items)
	require.Empty(t, out)
}

func TestSanitizeReconWorklist_KeepsConcretePaths(t *testing.T) {
	rt := &Runtime{}
	items := []WorklistSeedItem{
		{RelPath: "publiccms-core/src/main/java/com/publiccms/controller/admin/LoginAdminController.java", Category: worklistCategoryAuthEntry},
		{RelPath: "**/*Controller.java", Category: worklistCategoryAPIHandler},
	}
	out := sanitizeReconWorklist(rt, items)
	require.Len(t, out, 1)
	require.NotContains(t, out[0].RelPath, "*")
}

func TestTask2007_StageRecoveryIntegration(t *testing.T) {
	taskDir := "/home/murkfox/yakit-projects/aispace/2007_publiccms_ssa_scan_20260606_ac3dd"
	if _, err := os.Stat(taskDir); err != nil {
		t.Skip("task 2007 workdir not available")
	}

	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))

	for _, name := range []string{"code_reading_stage_1.json", "code_reading_stage_2.json", "code_reading_stage_3.json"} {
		src := filepath.Join(taskDir, store.SubDirName(), name)
		dst := filepath.Join(workDir, store.SubDirName(), name)
		b, err := os.ReadFile(src)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(dst, b, 0o644))
	}

	stages, err := loadAllCodeReadingStages(workDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(stages), 2)
	totalAPIs := 0
	for _, st := range stages {
		totalAPIs += len(st.APIFragments)
	}
	require.Greater(t, totalAPIs, 0, "2007 stage files should contain api_fragments")

	rt := &Runtime{WorkDir: workDir, Session: &store.DiscoverySession{CodePathOK: true}}
	plan, err := BuildCodeReadingPlanFromStageFiles(rt)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(plan.DiscoveredAPIs), 4)
}
