package loop_ssa_api_discovery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestApplyVerifiedHttpApiProbeBackfill_CorrectsPathAndEndpointStatus(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	dir := t.TempDir()

	plan := &CodeReadingPlan{
		DiscoveredAPIs: []DiscoveredAPI{{
			Method: "POST", PathPattern: "/login", HandlerClass: "LoginController",
		}},
	}
	require.NoError(t, PersistCodeReadingPlan(&Runtime{WorkDir: dir}, plan))

	sess := &store.DiscoverySession{UUID: uuid.NewString(), TargetReachable: true}
	require.NoError(t, repo.CreateSession(sess))
	_, err := EndpointInsertionGateway(&Runtime{WorkDir: dir, Repo: repo, Session: sess}, &store.HttpEndpoint{
		SessionID: sess.ID, Method: "POST", PathPattern: "/login",
		HandlerClass: "LoginController", Source: SourceAICodeRead,
		Status: store.EndpointStatusPendingValidation,
	})
	require.NoError(t, err)

	row := &store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "POST", PathPattern: "/login",
		FullSampleURL: "http://127.0.0.1:8080/admin/login",
		Verified: true, ProbeStatusCode: 200,
		ProbeAttemptsJSON: `[{"url":"http://127.0.0.1:8080/admin/login","status":200}]`,
	}
	require.NoError(t, repo.CreateVerifiedHttpApi(row))

	rt := &Runtime{WorkDir: dir, Repo: repo, Session: sess}
	require.NoError(t, ApplyVerifiedHttpApiProbeBackfill(rt, row))

	updated, err := repo.GetVerifiedHttpApi(sess.ID, row.ID)
	require.NoError(t, err)
	require.Equal(t, "/admin/login", updated.PathPattern)

	eps, err := repo.ListHttpEndpoints(sess.ID)
	require.NoError(t, err)
	require.Len(t, eps, 1)
	require.Equal(t, "/admin/login", eps[0].PathPattern)
	require.Equal(t, store.EndpointStatusAlive, eps[0].Status)
	require.Equal(t, 200, eps[0].ProbeStatusCode)

	reloaded, err := LoadCodeReadingPlan(dir)
	require.NoError(t, err)
	require.Len(t, reloaded.DiscoveredAPIs, 1)
	require.Equal(t, "/admin/login", reloaded.DiscoveredAPIs[0].PathPattern)
}
