package loop_ssa_api_discovery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestBackfillStaticVulnVerifications(t *testing.T) {
	repo, cleanup := openTestRepoForPhase1(t)
	defer cleanup()
	sess := &store.DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	finding := &store.DiscoverySyntaxFlowFinding{
		SessionID: sess.ID, RuleName: "sqli", Severity: "high", Title: "test",
	}
	require.NoError(t, repo.CreateDiscoverySyntaxFlowFinding(finding))

	require.NoError(t, repo.ReplaceVulnChecklistItems(sess.ID, []store.VulnChecklistItem{{
		SessionID: sess.ID, FindingID: finding.ID, Priority: highChecklistPriority,
		RuleName: "sqli", Severity: "high", Title: "test",
	}}))

	rt := &Runtime{Repo: repo, Session: sess}
	n, err := backfillStaticVulnVerifications(rt)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	verifications, err := repo.ListVulnVerifications(sess.ID)
	require.NoError(t, err)
	require.Len(t, verifications, 1)
	require.Equal(t, "safe", verifications[0].Status)
	require.Equal(t, finding.ID, verifications[0].SyntaxFlowFindingID)

	n2, err := backfillStaticVulnVerifications(rt)
	require.NoError(t, err)
	require.Equal(t, 0, n2)
}
