package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestShouldSkipPhase4Step2_NoFindings(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{Repo: repo, Session: sess}
	require.True(t, shouldSkipPhase4Step2StaticVerify(rt))
}

func TestRunPhase4Step2StaticVerify_SkipRecordsWaiver(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	workDir := t.TempDir()
	dir := filepath.Join(workDir, store.SubDirName())
	require.NoError(t, os.MkdirAll(dir, 0o755))

	sess := &store.DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))
	pl := NewPipelineState(workDir, "", sess.UUID)
	writePhase4StepReports(t, pl, dir)
	pl.SetGreyboxExecuted(true)

	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}
	require.NoError(t, runPhase4Step2StaticVerify(nil, nil, nil, rt, pl))
	require.FileExists(t, pl.GetStep2VerifyReportPath())
	require.True(t, hasStaticVerifySkippedWaiver(rt))

	s2, err := repo.GetSessionByUUID(sess.UUID)
	require.NoError(t, err)
	require.True(t, hasPipelineWaiver(s2, waiverStaticVerifySkipped))
	require.NoError(t, enforcePhase4DynamicContract(rt, pl))
}
