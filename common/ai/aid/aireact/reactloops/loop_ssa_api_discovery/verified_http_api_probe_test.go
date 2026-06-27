package loop_ssa_api_discovery

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestUpsertVerifiedHttpApiFromProbeResult(t *testing.T) {
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
		UUID:            uuid.NewString(),
		TargetReachable: true,
		TargetRaw:       "http://127.0.0.1:8080",
	}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{
		Repo: repo, Session: sess, WorkDir: t.TempDir(),
	}
	row, err := UpsertVerifiedHttpApiFromProbeResult(rt, &ProbeResult{
		Verified:      true,
		Confidence:    90,
		Method:        "GET",
		PathPattern:   "/api/login",
		FullSampleURL: "http://127.0.0.1:8080/api/login",
		VerdictReason: "ok",
	})
	require.NoError(t, err)
	require.NotZero(t, row.ID)
	require.True(t, row.Verified)
}
