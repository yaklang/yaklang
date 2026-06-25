package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestNormalizePipelineMaxStage_LegacySixMapsToFive(t *testing.T) {
	require.Equal(t, 5, NormalizePipelineMaxStage(6))
	require.Equal(t, 5, NormalizePipelineMaxStage(99))
	require.Equal(t, 5, NormalizePipelineMaxStage(0))
}

func TestEnforcePhase1Contract_TargetUnreachableWaiver(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	workDir := t.TempDir()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: false}
	require.NoError(t, repo.CreateSession(sess))
	writePhase1ArtifactFiles(t, workDir)
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}
	require.NoError(t, EnforcePhaseContract(rt, nil, 1))
	s2, err := repo.GetSessionByUUID(sess.UUID)
	require.NoError(t, err)
	require.True(t, hasPipelineWaiver(s2, waiverTargetUnreachable))
}

func TestEnforcePhase1Contract_MissingPrepBundleFails(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	workDir := t.TempDir()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: false}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}
	err := EnforcePhaseContract(rt, nil, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "phase1_prep_bundle")
}

func writePhase1ArtifactFiles(t *testing.T, workDir string) {
	t.Helper()
	dir := filepath.Join(workDir, store.SubDirName())
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, writeJSONFile(store.Phase1PrepBundlePath(workDir), []byte(`{"ok":true}`)))
	cands, _ := json.Marshal(map[string]any{"count": 0, "candidates": []any{}})
	require.NoError(t, writeJSONFile(store.RouteCandidatesPath(workDir), cands))
}

func TestEnforcePhase2Contract_FeatureVerifyVerifiedWithoutProbeURL(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()

	workDir := t.TempDir()
	dir := filepath.Join(workDir, store.SubDirName())
	require.NoError(t, os.MkdirAll(dir, 0o755))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))

	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/api/content",
		Verified: true, Source: "feature_verify", VerdictReason: "feature_verify: verified",
	}))
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "POST", PathPattern: "/admin/sysUser/save",
		Verified: false, Source: "feature_verify", RejectReason: "not_probed",
	}))

	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}
	pl := NewPipelineState(workDir, "", sess.UUID)
	require.NoError(t, finalizePhase1DiscoveryArtifacts(nil, rt, pl))
	require.NoError(t, EnforcePhaseContract(rt, pl, 2))
}

func writePhase4StepReports(t *testing.T, pl *PipelineState, dir string) {
	t.Helper()
	pl.SetStep0ReportPath(filepath.Join(dir, "step0_vuln_checklist.md"))
	pl.SetStep1AuthReportPath(filepath.Join(dir, "step1_auth_result.md"))
	pl.SetStep2VerifyReportPath(filepath.Join(dir, "step2_static_verify.md"))
	pl.SetStep3GreyboxReportPath(filepath.Join(dir, "step3_greybox_scan.md"))
	reportBody := strings.Repeat("x", minStepReportBytes+8)
	for _, p := range []string{pl.GetStep0ReportPath(), pl.GetStep1AuthReportPath(), pl.GetStep2VerifyReportPath(), pl.GetStep3GreyboxReportPath()} {
		require.NoError(t, os.WriteFile(p, []byte(reportBody), 0o644))
	}
}

func TestEnforcePhase4Contract_MissingAuthCredentialsRecordsWaiver(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()

	workDir := t.TempDir()
	dir := filepath.Join(workDir, store.SubDirName())
	require.NoError(t, os.MkdirAll(dir, 0o755))

	sess := &store.DiscoverySession{
		UUID:            uuid.NewString(),
		TargetReachable: true,
	}
	require.NoError(t, repo.CreateSession(sess))

	require.NoError(t, os.WriteFile(store.AuthSurfacePath(workDir), []byte(`{"login_paths":["/api/auth/login"]}`), 0o644))
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/api/x",
		Verified: true, Source: "test", FullSampleURL: "http://127.0.0.1:8090/api/x",
	}))

	pl := NewPipelineState(workDir, "", sess.UUID)
	writePhase4StepReports(t, pl, dir)
	pl.SetGreyboxExecuted(true)

	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess, Phase4ModeRaw: Phase4ModeBatchScan}
	require.NoError(t, EnforcePhaseContract(rt, pl, 4))

	s2, err := repo.GetSessionByUUID(sess.UUID)
	require.NoError(t, err)
	require.True(t, hasPipelineWaiver(s2, waiverAuthCredentialsMissing))
}

func TestEnforcePhase4Contract_NoProbeTargetsWaiverNotError(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()

	workDir := t.TempDir()
	dir := filepath.Join(workDir, store.SubDirName())
	require.NoError(t, os.MkdirAll(dir, 0o755))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))

	pl := NewPipelineState(workDir, "", sess.UUID)
	writePhase4StepReports(t, pl, dir)
	pl.SetGreyboxExecuted(true)

	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}
	require.NoError(t, enforcePhase4DynamicContract(rt, pl))

	s2, err := repo.GetSessionByUUID(sess.UUID)
	require.NoError(t, err)
	require.True(t, hasPipelineWaiver(s2, waiverDeepMiningNoTargets))
}

func TestEnforcePhase4Contract_DeepMiningRequiresFinalize(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()

	workDir := t.TempDir()
	dir := filepath.Join(workDir, store.SubDirName())
	require.NoError(t, os.MkdirAll(dir, 0o755))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	api := &store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/api/x",
		Verified: true, Source: "test", FullSampleURL: "http://127.0.0.1:8090/api/x",
	}
	require.NoError(t, repo.UpsertVerifiedHttpApi(api))

	pl := NewPipelineState(workDir, "", sess.UUID)
	writePhase4StepReports(t, pl, dir)
	pl.SetGreyboxExecuted(true)

	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess, Phase4ModeRaw: Phase4ModeDeepMining}
	err := enforcePhase4DynamicContract(rt, pl)
	require.Error(t, err)
	require.Contains(t, err.Error(), "deep_mining 未完成")

	pl.MarkDeepMiningDone(api.ID)
	require.NoError(t, enforcePhase4DynamicContract(rt, pl))
}
