package loop_ssa_api_discovery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestParseRoutePathsFromDataFlowHint(t *testing.T) {
	paths := parseRoutePathsFromDataFlowHint(`UserController.login @GetMapping("/login")`)
	require.Contains(t, paths, "/login")
}

func TestAssociateFindingByDataFlowHintPath(t *testing.T) {
	f := store.DiscoverySyntaxFlowFinding{
		ID:           1,
		MatchedFile:  "src/UserService.java",
		DataFlowHint: `sink in login @PostMapping("/api/login")`,
		Severity:     "high",
		RuleName:     "sqli",
		Title:        "SQLi",
	}
	eps := []store.HttpEndpoint{
		{ID: 10, Method: "POST", PathPattern: "/api/login", HandlerClass: "UserController"},
	}
	item, matched := associateFindingToEndpoints(f, nil, eps, map[string]*store.HttpEndpoint{})
	require.True(t, matched)
	require.Equal(t, uint(10), item.EndpointID)
	require.Equal(t, "high", item.AssocConfidence)
}

func TestReplaceVulnChecklistItems(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		if s := db.DB(); s != nil {
			_ = s.Close()
		}
	}()
	require.NoError(t, store.AutoMigrate(db))
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	rows := []store.VulnChecklistItem{{
		FindingID: 1, RuleName: "r1", Severity: "high", AssocConfidence: "high", Priority: 3,
	}}
	require.NoError(t, repo.ReplaceVulnChecklistItems(sess.ID, rows))
	got, err := repo.ListVulnChecklistItems(sess.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "r1", got[0].RuleName)

	rows[0].RuleName = "r2"
	require.NoError(t, repo.ReplaceVulnChecklistItems(sess.ID, rows))
	got, err = repo.ListVulnChecklistItems(sess.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "r2", got[0].RuleName)
}

func TestUpsertPhaseArtifact(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		if s := db.DB(); s != nil {
			_ = s.Close()
		}
	}()
	require.NoError(t, store.AutoMigrate(db))
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString()}
	require.NoError(t, repo.CreateSession(sess))

	require.NoError(t, repo.UpsertPhaseArtifact(sess.ID, store.ArtifactStaticRouteHints, `{"count":1}`))
	row, err := repo.GetPhaseArtifact(sess.ID, store.ArtifactStaticRouteHints)
	require.NoError(t, err)
	require.Equal(t, 1, row.Version)

	require.NoError(t, repo.UpsertPhaseArtifact(sess.ID, store.ArtifactStaticRouteHints, `{"count":2}`))
	row, err = repo.GetPhaseArtifact(sess.ID, store.ArtifactStaticRouteHints)
	require.NoError(t, err)
	require.Equal(t, 2, row.Version)
	require.Contains(t, row.PayloadJSON, `"count":2`)
}

func TestDiscoveryReadEntities_includeNewTables(t *testing.T) {
	ents := store.AllSessionReadEntities()
	require.Contains(t, ents, store.SessionEntityVulnChecklistItems)
	require.Contains(t, ents, store.SessionEntityPhaseArtifacts)
	require.Contains(t, ents, store.SessionEntityCoverageWorkItems)
}
