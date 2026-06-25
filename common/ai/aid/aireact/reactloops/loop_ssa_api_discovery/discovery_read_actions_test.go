package loop_ssa_api_discovery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestSessionRowsPayload_truncates(t *testing.T) {
	rows := make([]store.VulnVerification, 5)
	for i := range rows {
		rows[i].SessionID = 1
		rows[i].SyntaxFlowFindingID = uint(i + 1)
	}
	out, err := sessionRowsPayload(rows, nil, 2)
	require.NoError(t, err)
	require.Equal(t, 5, out["total"])
	require.True(t, out["truncated"].(bool))
	got := out["rows"].([]store.VulnVerification)
	require.Len(t, got, 2)
}

func TestBuildDiscoveryStatusPayload_andReadSchema(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		if s := db.DB(); s != nil {
			_ = s.Close()
		}
	}()
	require.NoError(t, store.AutoMigrate(db))
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{
		UUID:         uuid.NewString(),
		CodeRootPath: "/code",
		Phase:        "core_analyzed",
		TargetRaw:    "http://127.0.0.1:8080",
	}
	require.NoError(t, repo.CreateSession(sess))
	require.NoError(t, repo.CreateVulnVerification(&store.VulnVerification{
		SessionID:           sess.ID,
		SyntaxFlowFindingID: 9,
		Status:              "confirmed",
		Confidence:          8,
		ExploitPayload:      "id=1",
	}))
	require.NoError(t, repo.UpsertVerifiedHttpApi(&store.VerifiedHttpApi{
		SessionID: sess.ID, Method: "GET", PathPattern: "/api/x",
		Verified: true, FullSampleURL: "http://127.0.0.1/api/x", VerdictReason: "ok",
	}))

	rt := &Runtime{DB: db, Repo: repo, Session: sess, WorkDir: t.TempDir(), SQLitePath: ":memory:"}
	payload, err := buildDiscoveryStatusPayload(rt, sess)
	require.NoError(t, err)
	counts := payload["counts"].(map[string]int)
	require.Equal(t, 1, counts["vuln_verifications"])
	require.Equal(t, 1, counts["verified_http_apis_verified"])
	vha := payload["verified_http_apis"].(map[string]int)
	require.Equal(t, 1, vha["verified"])

	cols := store.DocumentedSessionTableColumns["vuln_verifications"]
	require.Contains(t, cols, "syntax_flow_finding_id")
	require.NotContains(t, cols, "http_endpoint_id")
}
