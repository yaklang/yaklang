package aibalanceclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func openClientIDTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", path+"?_busy_timeout=10000&_journal_mode=WAL")
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestClientUserIDPersistence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "profile.db")
	file := clientUserIDFile(path, filepath.Join(root, "temp"))
	db := openClientIDTestDB(t, path)
	id, inDB, err := loadClientUserID(db, file)
	require.NoError(t, err)
	require.True(t, inDB)
	parsed, err := uuid.Parse(id)
	require.NoError(t, err)
	require.Equal(t, uuid.Version(4), parsed.Version())
	require.Equal(t, id, yakit.GetKey(db, TRACE_ID_DB_KEY))
	backup, err := readClientUserIDFile(file)
	require.NoError(t, err)
	require.Equal(t, id, backup)

	require.NoError(t, db.Close())
	reopened := openClientIDTestDB(t, path)
	reused, inDB, err := loadClientUserID(reopened, file)
	require.NoError(t, err)
	require.True(t, inDB)
	require.Equal(t, id, reused)
}

func TestClientUserIDDatabaseFailureAndRecovery(t *testing.T) {
	file := filepath.Join(t.TempDir(), "temp", "trace-id")
	id, inDB, err := loadClientUserID(nil, file)
	require.Error(t, err)
	require.False(t, inDB)
	require.NotEmpty(t, id)
	_, err = uuid.Parse(id)
	require.NoError(t, err)
	again, _, err := loadClientUserID(nil, file)
	require.Error(t, err)
	require.Equal(t, id, again)

	db := openClientIDTestDB(t, filepath.Join(t.TempDir(), "profile.db"))
	recovered, inDB, err := loadClientUserID(db, file)
	require.NoError(t, err)
	require.True(t, inDB)
	require.Equal(t, id, recovered)
	require.Equal(t, id, yakit.GetKey(db, TRACE_ID_DB_KEY))
	require.NoError(t, db.Close())
	reused, _, err := loadClientUserID(db, file)
	require.Error(t, err)
	require.Equal(t, id, reused)
}

func TestClientUserIDWriteFailure(t *testing.T) {
	root := t.TempDir()
	db := openClientIDTestDB(t, filepath.Join(root, "profile.db"))
	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)
	file := filepath.Join(root, "temp", "trace-id")
	id, inDB, err := loadClientUserID(db, file)
	require.Error(t, err)
	require.False(t, inDB)
	require.NotEmpty(t, id)
	again, _, err := loadClientUserID(db, file)
	require.Error(t, err)
	require.Equal(t, id, again)
	require.NoError(t, db.Exec("PRAGMA query_only = OFF").Error)
	recovered, inDB, err := loadClientUserID(db, file)
	require.NoError(t, err)
	require.True(t, inDB)
	require.Equal(t, id, recovered)
}

func TestClientUserIDStorageFailures(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("blocked"), 0600))
	file := filepath.Join(blocker, "trace-id")
	for i := 0; i < 2; i++ {
		id, inDB, err := loadClientUserID(nil, file)
		require.Error(t, err)
		require.False(t, inDB)
		require.Empty(t, id, "must not return an unpersisted random identity")
	}
	db := openClientIDTestDB(t, filepath.Join(root, "profile.db"))
	id, inDB, err := loadClientUserID(db, file)
	require.Error(t, err, "backup failure remains visible")
	require.True(t, inDB)
	require.NotEmpty(t, id, "healthy DB still provides an identity")
	require.Equal(t, id, yakit.GetKey(db, TRACE_ID_DB_KEY))
}

func TestClientUserIDExistingProfileWins(t *testing.T) {
	root := t.TempDir()
	db := openClientIDTestDB(t, filepath.Join(root, "profile.db"))
	existing := uuid.NewString()
	require.NoError(t, yakit.SetKey(db, TRACE_ID_DB_KEY, existing))
	file := filepath.Join(root, "temp", "trace-id")
	require.NoError(t, writeClientUserIDFile(file, uuid.NewString(), false))
	id, inDB, err := loadClientUserID(db, file)
	require.NoError(t, err)
	require.True(t, inDB)
	require.Equal(t, existing, id)
	backup, err := readClientUserIDFile(file)
	require.NoError(t, err)
	require.Equal(t, existing, backup)
}

func TestClientUserIDCorruptFallback(t *testing.T) {
	file := filepath.Join(t.TempDir(), "trace-id")
	require.NoError(t, os.WriteFile(file, []byte("partial"), 0600))
	id, _, err := loadClientUserID(nil, file)
	require.Error(t, err)
	require.Empty(t, id)
	db := openClientIDTestDB(t, filepath.Join(t.TempDir(), "profile.db"))
	id, inDB, err := loadClientUserID(db, file)
	require.NoError(t, err)
	require.True(t, inDB)
	backup, err := readClientUserIDFile(file)
	require.NoError(t, err)
	require.Equal(t, id, backup)
}

func TestClientUserIDFallbackProfileIsolation(t *testing.T) {
	root := t.TempDir()
	fileA := clientUserIDFile(filepath.Join(root, "a.db"), root)
	fileB := clientUserIDFile(filepath.Join(root, "b.db"), root)
	idA, _, _ := loadClientUserID(nil, fileA)
	idB, _, _ := loadClientUserID(nil, fileB)
	require.NotEmpty(t, idA)
	require.NotEmpty(t, idB)
	require.NotEqual(t, idA, idB)
	reused, _, _ := loadClientUserID(nil, fileA)
	require.Equal(t, idA, reused)
}

func TestClientUserIDUpgradeCacheCleanup(t *testing.T) {
	root := t.TempDir()
	fileA := clientUserIDFile(filepath.Join(root, "a.db"), root)
	fileB := clientUserIDFile(filepath.Join(root, "b.db"), root)
	idA, _, _ := loadClientUserID(nil, fileA)
	idB, _, _ := loadClientUserID(nil, fileB)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "ordinary-cache"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ordinary-cache", "payload"), nil, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".aibalance-trace-id-partial"), nil, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, clientUserIDFilePrefix+"invalid"), nil, 0600))
	require.NoError(t, ClearTempDirPreservingClientUserIDs(root))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	reusedA, _, _ := loadClientUserID(nil, fileA)
	reusedB, _, _ := loadClientUserID(nil, fileB)
	require.Equal(t, idA, reusedA)
	require.Equal(t, idB, reusedB)
	require.NoError(t, ClearTempDirPreservingClientUserIDs(filepath.Join(root, "missing")))
}

func TestClientUserIDProfileCacheIsolation(t *testing.T) {
	root := t.TempDir()
	previousDB := consts.GetGormProfileDatabase()
	previousPath := consts.GetCurrentProfileDatabasePath()
	t.Setenv("YAKIT_HOME", root)
	t.Cleanup(func() {
		consts.BindProfileDatabase(previousDB, previousPath)
		clientUserIDCache.Lock()
		clientUserIDCache.id = ""
		clientUserIDCache.Unlock()
	})
	pathA, pathB := filepath.Join(root, "a.db"), filepath.Join(root, "b.db")
	dbA, dbB := openClientIDTestDB(t, pathA), openClientIDTestDB(t, pathB)
	consts.BindProfileDatabase(dbA, pathA)
	idA := GetOrCreateClientUserID()
	require.NotEmpty(t, idA)
	consts.BindProfileDatabase(dbB, pathB)
	idB := GetOrCreateClientUserID()
	require.NotEmpty(t, idB)
	require.NotEqual(t, idA, idB)
	consts.BindProfileDatabase(dbA, pathA)
	require.Equal(t, idA, GetTraceID())
	require.NotEqual(t, clientUserIDFile(pathA, root), clientUserIDFile(pathB, root))

	previousProject := consts.CaptureProjectDatabaseBinding()
	t.Cleanup(func() {
		consts.BindProjectDatabaseWithReader(previousProject.Database, previousProject.ReadDatabase, previousProject.Path)
	})
	// The selected project even contains a different trace ID. Only the active
	// Profile may determine identity, including on a cache miss.
	consts.BindProjectDatabaseWithReader(dbB, dbB, pathB)
	clientUserIDCache.Lock()
	clientUserIDCache.id = ""
	clientUserIDCache.Unlock()
	require.Equal(t, idA, GetOrCreateClientUserID())

	var wg sync.WaitGroup
	ids := make(chan string, 32)
	clientUserIDCache.Lock()
	clientUserIDCache.id = ""
	clientUserIDCache.Unlock()
	require.NoError(t, os.Remove(clientUserIDFile(pathA, consts.GetDefaultYakitBaseTempDir())))
	yakit.DelKey(dbA, TRACE_ID_DB_KEY)
	for i := 0; i < cap(ids); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids <- GetOrCreateClientUserID()
		}()
	}
	wg.Wait()
	close(ids)
	stored := yakit.GetKey(dbA, TRACE_ID_DB_KEY)
	require.NotEmpty(t, stored)
	for id := range ids {
		require.Equal(t, stored, id)
	}
}

func TestClientUserIDMultipleProcesses(t *testing.T) {
	for _, mode := range []string{"database", "database-without-file", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "profile.db")
			file := clientUserIDFile(path, filepath.Join(root, "temp"))
			if mode == "database-without-file" {
				blocker := filepath.Join(root, "not-a-directory")
				require.NoError(t, os.WriteFile(blocker, nil, 0600))
				file = filepath.Join(blocker, "trace-id")
			}
			db := openClientIDTestDB(t, path)
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			commands := make([]*exec.Cmd, 6)
			for i := range commands {
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestClientUserIDProcessHelper$")
				cmd.Env = append(os.Environ(), "YAK_CLIENT_ID_TEST_PROFILE="+path,
					"YAK_CLIENT_ID_TEST_FILE="+file, "YAK_CLIENT_ID_TEST_MODE="+mode,
					fmt.Sprintf("YAK_CLIENT_ID_TEST_RESULT=%s", filepath.Join(root, fmt.Sprint(i))))
				commands[i] = cmd
				require.NoError(t, cmd.Start())
			}
			var id string
			for i, cmd := range commands {
				require.NoError(t, cmd.Wait())
				data, err := os.ReadFile(filepath.Join(root, fmt.Sprint(i)))
				require.NoError(t, err)
				require.NotEmpty(t, data)
				if id == "" {
					id = string(data)
				}
				require.Equal(t, id, string(data))
			}
			if mode != "database-without-file" {
				backup, err := readClientUserIDFile(file)
				require.NoError(t, err)
				require.Equal(t, id, backup)
			}
			if mode != "fallback" {
				require.Equal(t, id, yakit.GetKey(db, TRACE_ID_DB_KEY))
				var count int
				require.NoError(t, db.Model(&schema.GeneralStorage{}).Count(&count).Error)
				require.Equal(t, 1, count)
			}
		})
	}
}

func TestClientUserIDProcessHelper(t *testing.T) {
	path := os.Getenv("YAK_CLIENT_ID_TEST_PROFILE")
	if path == "" {
		t.Skip("subprocess helper")
	}
	var db *gorm.DB
	if os.Getenv("YAK_CLIENT_ID_TEST_MODE") != "fallback" {
		var err error
		db, err = gorm.Open("sqlite3", path+"?_busy_timeout=10000")
		require.NoError(t, err)
		defer db.Close()
	}
	id, _, _ := loadClientUserID(db, os.Getenv("YAK_CLIENT_ID_TEST_FILE"))
	require.NotEmpty(t, id)
	require.NoError(t, os.WriteFile(os.Getenv("YAK_CLIENT_ID_TEST_RESULT"), []byte(id), 0600))
}
