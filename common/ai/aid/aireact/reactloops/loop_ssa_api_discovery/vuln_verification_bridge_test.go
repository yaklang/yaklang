package loop_ssa_api_discovery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestBridgeDynamicFindingToVulnVerification(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString(), CodeRootPath: "/code", CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{DB: db, Repo: repo, Session: sess}

	dyn := &store.DynamicVulnFinding{
		SessionID:      sess.ID,
		HttpEndpointID: 1,
		VulnType:       "sqli",
		Severity:       "high",
		Confidence:     80,
		Payload:        "' OR 1=1--",
		RequestURL:     "http://host/api/search?q=x",
		Evidence:       "error in SQL syntax",
		Status:         "confirmed",
	}
	require.NoError(t, repo.CreateDynamicVulnFinding(dyn))

	require.NoError(t, BridgeDynamicFindingToVulnVerification(rt, dyn))

	rows, err := repo.ListVulnVerifications(sess.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, dyn.ID, rows[0].DynamicFindingID)
	require.Equal(t, "dynamic", rows[0].Source)
	require.Equal(t, "confirmed", rows[0].Status)
}

func TestBridgeAllConfirmedDynamicFindings(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{DB: db, Repo: repo, Session: sess}

	require.NoError(t, repo.CreateDynamicVulnFinding(&store.DynamicVulnFinding{
		SessionID: sess.ID, VulnType: "xss", Severity: "medium", Status: "confirmed", Confidence: 70,
	}))
	require.NoError(t, repo.CreateDynamicVulnFinding(&store.DynamicVulnFinding{
		SessionID: sess.ID, VulnType: "sqli", Severity: "high", Status: "false_positive", Confidence: 20,
	}))

	n, err := BridgeAllConfirmedDynamicFindings(rt)
	require.NoError(t, err)
	require.Equal(t, 1, n)
}
