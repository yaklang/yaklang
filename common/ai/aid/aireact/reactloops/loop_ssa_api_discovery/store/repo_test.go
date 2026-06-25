package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func openMemRepo(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	return NewRepository(db), db
}

func TestRepository_SessionCRUD(t *testing.T) {
	repo, db := openMemRepo(t)
	defer func() {
		if sqlDB := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	s := &DiscoverySession{
		UUID:         uuid.NewString(),
		CodeRootPath: "/tmp/foo",
		Phase:        "initialized",
	}
	require.NoError(t, repo.CreateSession(s))
	require.NotZero(t, s.ID)

	loaded, err := repo.GetSessionByUUID(s.UUID)
	require.NoError(t, err)
	require.Equal(t, "/tmp/foo", loaded.CodeRootPath)

	loaded.Phase = "ssa_done"
	require.NoError(t, repo.UpdateSession(loaded))

	require.NoError(t, repo.UpdateSessionFields(s.UUID, map[string]interface{}{
		"ssa_compile_ok": true,
		"ssa_file_count": 42,
	}))
	loaded2, err := repo.GetSessionByUUID(s.UUID)
	require.NoError(t, err)
	require.True(t, loaded2.SSACompileOK)
	require.Equal(t, 42, loaded2.SSAFileCount)
}

func TestRepository_GetLatestSession(t *testing.T) {
	repo, db := openMemRepo(t)
	defer func() {
		if sqlDB := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	s1 := &DiscoverySession{UUID: uuid.NewString(), CodeRootPath: "/a", Phase: "ssa_done", CodePathOK: true}
	s2 := &DiscoverySession{UUID: uuid.NewString(), CodeRootPath: "/b", Phase: "core_analyzed", CodePathOK: true}
	require.NoError(t, repo.CreateSession(s1))
	require.NoError(t, repo.CreateSession(s2))

	_ = repo.UpdateSessionFields(s1.UUID, map[string]interface{}{"notes": "touch"})
	latest, err := repo.GetLatestSession()
	require.NoError(t, err)
	require.Equal(t, s1.UUID, latest.UUID)
}

func TestRepository_ChildTablesCRUD(t *testing.T) {
	repo, db := openMemRepo(t)
	defer func() {
		if sqlDB := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	s := &DiscoverySession{UUID: uuid.NewString(), Phase: "ssa_done"}
	require.NoError(t, repo.CreateSession(s))

	comp := &ArchitectureComponent{SessionID: s.ID, Name: "api", Kind: "service", Summary: "REST", Confidence: 80, Source: "ai"}
	require.NoError(t, repo.CreateComponent(comp))

	cf := &ConfigArtifact{SessionID: s.ID, RelPath: "app.yml", Format: "yaml", Summary: "cfg"}
	require.NoError(t, repo.CreateConfigArtifact(cf))

	dep := &DependencyRef{SessionID: s.ID, Name: "spring-web", Version: "5.x", Ecosystem: "maven"}
	require.NoError(t, repo.CreateDependency(dep))

	ep := &HttpEndpoint{SessionID: s.ID, Method: "GET", PathPattern: "/api/health", Source: "ai"}
	require.NoError(t, repo.CreateHttpEndpoint(ep))

	sm := &SecurityMechanism{SessionID: s.ID, Category: "authn", Description: "JWT"}
	require.NoError(t, repo.CreateSecurityMechanism(sm))

	bc := &BusinessCapability{SessionID: s.ID, Name: "Orders", Description: "handles orders", LayerHint: "domain"}
	require.NoError(t, repo.CreateBusinessCapability(bc))

	comps, err := repo.ListComponents(s.ID)
	require.NoError(t, err)
	require.Len(t, comps, 1)

	require.NoError(t, repo.DeleteComponent(s.ID, comps[0].ID))
	comps, err = repo.ListComponents(s.ID)
	require.NoError(t, err)
	require.Len(t, comps, 0)

	c1, c2, d, e, se, b, ver, sf, vv, err := repo.CountsBySession(s.ID)
	require.NoError(t, err)
	require.Equal(t, 0, c1)
	require.Equal(t, 1, c2)
	require.Equal(t, 1, d)
	require.Equal(t, 1, e)
	require.Equal(t, 1, se)
	require.Equal(t, 1, b)
	require.Equal(t, 0, ver)
	require.Equal(t, 0, sf)
	require.Equal(t, 0, vv)

	require.NoError(t, repo.AppendEvent(s.ID, "info", "probe", `{}`))
	evs, err := repo.ListEvents(s.ID, 10)
	require.NoError(t, err)
	require.Len(t, evs, 1)
}

func TestOpenSessionDB_File(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "w")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	db, err := OpenSessionDB(dir)
	require.NoError(t, err)
	defer func() {
		if sqlDB := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()
	require.FileExists(t, DBPath(dir))
}
