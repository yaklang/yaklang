package yakit

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// newRepairTestDB creates an in-memory-style SQLite profile database
// with the projects table migrated, for testing repair logic.
func newRepairTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Project{}).Error)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestRepairProjectDatabasePath_NoRepairNeeded verifies that when the
// database file exists, RepairProjectDatabasePath returns the original
// path and true without any modification.
func TestRepairProjectDatabasePath_NoRepairNeeded(t *testing.T) {
	db := newRepairTestDB(t)

	// create a real db file in a temp dir
	dbFile := filepath.Join(t.TempDir(), "yakit-project-test-123.sqlite3.db")
	_, err := consts.CreateProjectDatabase(dbFile)
	require.NoError(t, err)

	proj := &schema.Project{
		Model:        gorm.Model{ID: 1},
		ProjectName:  "test-project",
		DatabasePath: dbFile,
		Type:         TypeProject,
	}
	require.NoError(t, db.Create(proj).Error)

	path, ok := RepairProjectDatabasePath(db, proj)
	require.True(t, ok, "file exists, should return true")
	require.Equal(t, dbFile, path, "path should be unchanged")
}

// TestRepairProjectDatabasePath_FileMovedAndFound verifies that when the
// database file has been moved to the expected directory, RepairProjectDatabasePath
// finds it by filename, updates the record, and returns the new path.
func TestRepairProjectDatabasePath_FileMovedAndFound(t *testing.T) {
	db := newRepairTestDB(t)

	// create the db file in the expected projects directory
	expectedDir := consts.GetDefaultYakitProjectsDir()
	dbFile := filepath.Join(expectedDir, "yakit-project-repair-test-999.sqlite3.db")
	_, err := consts.CreateProjectDatabase(dbFile)
	require.NoError(t, err)
	t.Cleanup(func() {
		consts.DeleteDatabaseFile(dbFile)
	})

	// simulate a stale path (file was "moved" from old location)
	stalePath := filepath.Join(t.TempDir(), "old-location", "yakit-project-repair-test-999.sqlite3.db")

	proj := &schema.Project{
		Model:        gorm.Model{ID: 1},
		ProjectName:  "repair-test",
		DatabasePath: stalePath,
		Type:         TypeProject,
	}
	require.NoError(t, db.Create(proj).Error)

	path, ok := RepairProjectDatabasePath(db, proj)
	require.True(t, ok, "file should be found in expected dir")
	require.Equal(t, dbFile, path, "path should be updated to the new location")

	// verify the DB record was updated
	var updated schema.Project
	require.NoError(t, db.First(&updated, proj.ID).Error)
	require.Equal(t, dbFile, updated.DatabasePath, "database record should reflect new path")
}

// TestRepairProjectDatabasePath_FileNotFound verifies that when the
// database file cannot be found anywhere, RepairProjectDatabasePath
// returns false and the original stale path.
func TestRepairProjectDatabasePath_FileNotFound(t *testing.T) {
	db := newRepairTestDB(t)

	stalePath := filepath.Join(t.TempDir(), "nonexistent", "yakit-project-missing-456.sqlite3.db")

	proj := &schema.Project{
		Model:        gorm.Model{ID: 1},
		ProjectName:  "missing-project",
		DatabasePath: stalePath,
		Type:         TypeProject,
	}
	require.NoError(t, db.Create(proj).Error)

	path, ok := RepairProjectDatabasePath(db, proj)
	require.False(t, ok, "file not found, should return false")
	require.Equal(t, stalePath, path, "should return original stale path")

	// verify the DB record was NOT updated
	var updated schema.Project
	require.NoError(t, db.First(&updated, proj.ID).Error)
	require.Equal(t, stalePath, updated.DatabasePath, "database record should be unchanged")
}

// TestRepairProjectDatabasePath_EmptyPath verifies nil/empty handling.
func TestRepairProjectDatabasePath_EmptyPath(t *testing.T) {
	db := newRepairTestDB(t)

	path, ok := RepairProjectDatabasePath(db, nil)
	require.False(t, ok)
	require.Empty(t, path)

	proj := &schema.Project{
		DatabasePath: "",
	}
	path, ok = RepairProjectDatabasePath(db, proj)
	require.False(t, ok)
	require.Empty(t, path)
}

// TestRepairProjectDatabasePath_SSA_DSN verifies that SSA projects with
// MySQL/Postgres DSN strings are skipped (not treated as file paths).
func TestRepairProjectDatabasePath_SSA_DSN(t *testing.T) {
	db := newRepairTestDB(t)

	proj := &schema.Project{
		Model:        gorm.Model{ID: 1},
		ProjectName:  "ssa-mysql",
		DatabasePath: "mysql://root:password@tcp(localhost:3306)/yak",
		Type:         TypeSSAProject,
	}
	require.NoError(t, db.Create(proj).Error)

	path, ok := RepairProjectDatabasePath(db, proj)
	require.False(t, ok, "DSN should not be treated as a file path")
	require.Equal(t, "mysql://root:password@tcp(localhost:3306)/yak", path)
}

// TestRepairProjectDatabasePath_SSA_SQLiteMoved verifies that SSA projects
// with SQLite file paths are repaired when the file is found in the SSA dir.
func TestRepairProjectDatabasePath_SSA_SQLiteMoved(t *testing.T) {
	db := newRepairTestDB(t)

	// create the db file in the expected SSA projects directory
	expectedDir := consts.GetDefaultSSAProjectDir()
	dbFile := filepath.Join(expectedDir, "ssa-project-repair-test-888.sqlite3.db")
	_, err := consts.CreateSSAProjectDatabase(consts.SSA_PROJECT_DB_DIALECT, dbFile)
	require.NoError(t, err)
	t.Cleanup(func() {
		consts.DeleteDatabaseFile(dbFile)
	})

	stalePath := filepath.Join(t.TempDir(), "old-ssa", "ssa-project-repair-test-888.sqlite3.db")

	proj := &schema.Project{
		Model:        gorm.Model{ID: 1},
		ProjectName:  "ssa-repair-test",
		DatabasePath: stalePath,
		Type:         TypeSSAProject,
	}
	require.NoError(t, db.Create(proj).Error)

	path, ok := RepairProjectDatabasePath(db, proj)
	require.True(t, ok, "SSA SQLite file should be found in SSA dir")
	require.Equal(t, dbFile, path, "path should be updated to the SSA dir location")
}

// TestRepairProjectDatabasePaths_ScanAll verifies the batch repair function
// repairs multiple projects with stale paths.
func TestRepairProjectDatabasePaths_ScanAll(t *testing.T) {
	db := newRepairTestDB(t)

	expectedDir := consts.GetDefaultYakitProjectsDir()

	// project 1: file exists in expected dir, stale path
	dbFile1 := filepath.Join(expectedDir, "yakit-project-scan-1-111.sqlite3.db")
	_, err := consts.CreateProjectDatabase(dbFile1)
	require.NoError(t, err)
	t.Cleanup(func() { consts.DeleteDatabaseFile(dbFile1) })

	proj1 := &schema.Project{
		ProjectName:  "scan-test-1",
		DatabasePath: filepath.Join(t.TempDir(), "old", "yakit-project-scan-1-111.sqlite3.db"),
		Type:         TypeProject,
	}
	require.NoError(t, db.Create(proj1).Error)

	// project 2: file genuinely missing
	proj2 := &schema.Project{
		ProjectName:  "scan-test-2",
		DatabasePath: filepath.Join(t.TempDir(), "missing", "yakit-project-scan-2-222.sqlite3.db"),
		Type:         TypeProject,
	}
	require.NoError(t, db.Create(proj2).Error)

	// project 3: file exists, no repair needed
	dbFile3 := filepath.Join(t.TempDir(), "yakit-project-scan-3-333.sqlite3.db")
	_, err = consts.CreateProjectDatabase(dbFile3)
	require.NoError(t, err)
	proj3 := &schema.Project{
		ProjectName:  "scan-test-3",
		DatabasePath: dbFile3,
		Type:         TypeProject,
	}
	require.NoError(t, db.Create(proj3).Error)

	// run batch repair
	repairProjectDatabasePaths(db)

	// verify proj1 was repaired
	var updated1 schema.Project
	require.NoError(t, db.First(&updated1, proj1.ID).Error)
	require.Equal(t, dbFile1, updated1.DatabasePath, "proj1 should be repaired")

	// verify proj2 was NOT repaired (file missing)
	var updated2 schema.Project
	require.NoError(t, db.First(&updated2, proj2.ID).Error)
	require.Equal(t, proj2.DatabasePath, updated2.DatabasePath, "proj2 should remain stale")

	// verify proj3 was NOT changed
	var updated3 schema.Project
	require.NoError(t, db.First(&updated3, proj3.ID).Error)
	require.Equal(t, dbFile3, updated3.DatabasePath, "proj3 should be unchanged")
}

// TestRepairProjectDatabasePath_FileExists helper to ensure utils.FileExists
// works as expected in this context.
func TestRepairProjectDatabasePath_FileExistsCheck(t *testing.T) {
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "exists.db")
	require.NoError(t, utils.SaveFile([]byte("test"), existingFile))
	require.True(t, utils.FileExists(existingFile))
	require.False(t, utils.FileExists(filepath.Join(tmpDir, "not-exists.db")))
}
