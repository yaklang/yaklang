package loop_ssa_api_discovery

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestListProbeTargets_PreferVerifiedHttpApi(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetScheme: "http", TargetHost: "127.0.0.1", TargetPort: "8080"}
	require.NoError(t, repo.CreateSession(sess))
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "POST", PathPattern: "/login",
		Verified: true, FullSampleURL: "http://127.0.0.1:8080/login",
	}))
	rt := &Runtime{Repo: repo, Session: sess}
	targets, err := ListProbeTargets(rt)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, "verified_http_api", targets[0].Source)
	require.Equal(t, "http://127.0.0.1:8080/login", targets[0].FullSampleURL)
}

func TestListProbeTargets_NoLegacyFallback(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{
		UUID: uuid.NewString(), TargetScheme: "http", TargetHost: "127.0.0.1", TargetPort: "8080",
		TargetRaw: "http://127.0.0.1:8080",
	}
	require.NoError(t, repo.CreateSession(sess))
	ep := &store.HttpEndpoint{SessionID: sess.ID, Method: "GET", PathPattern: "/legacy", Status: store.EndpointStatusAlive}
	require.NoError(t, repo.CreateHttpEndpoint(ep))
	rt := &Runtime{Repo: repo, Session: sess}
	targets, err := ListProbeTargets(rt)
	require.NoError(t, err)
	require.Empty(t, targets)
}

func TestEnforcePhase1Contract_CoveredRoutesPass(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	workDir := t.TempDir()
	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true, CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, Repo: repo, Session: sess}

	cands := map[string]any{"candidates": []any{map[string]any{"method": "GET", "path": "/a"}}, "count": 1}
	b, err := json.MarshalIndent(cands, "", "  ")
	require.NoError(t, err)
	require.NoError(t, writeJSONFile(store.RouteCandidatesPath(workDir), b))
	require.NoError(t, repo.CreateHttpEndpoint(&store.HttpEndpoint{
		SessionID: sess.ID, Method: "GET", PathPattern: "/a", Source: "ai_code_read",
	}))
	require.NoError(t, writeJSONFile(store.Phase1PrepBundlePath(workDir), []byte(`{"ok":true}`)))
	sess.ApiPreanalysisMetaJSON = `{}`
	sess.ApiBaseCalibrationMetaJSON = `{"top_score":1}`
	require.NoError(t, repo.UpdateSession(sess))
	require.NoError(t, writeJSONFile(store.CodeReadingPlanPath(workDir), []byte(`{"discovered_apis":[{"method":"GET","path_pattern":"/a"}]}`)))
	require.NoError(t, writePhase1ContractStubArtifacts(workDir))
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/a",
		Verified: true, FullSampleURL: "http://t/a", VerdictReason: "ok",
		ProbeStatusCode: 200, ProbeAttemptsJSON: `[{"status":200}]`,
	}))

	require.NoError(t, EnforcePhaseContract(rt, nil, 1))
}
