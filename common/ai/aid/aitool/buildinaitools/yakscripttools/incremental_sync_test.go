package yakscripttools

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type incrementalAIToolFS struct{ fi.FileSystem }

func (*incrementalAIToolFS) GetHash() (string, error) { return "test", nil }

func newIncrementalAIToolDB(t *testing.T, scripts map[string]string) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AIYakTool{}, &schema.GeneralStorage{}).Error)
	previous := yakScriptFS
	t.Cleanup(func() { yakScriptFS = previous })
	setIncrementalAIToolFS(scripts)
	return db
}

func setIncrementalAIToolFS(scripts map[string]string) {
	vfs := filesys.NewVirtualFs()
	for name, code := range scripts {
		vfs.AddFile("yakscriptforai/test/"+name+".yak", code)
	}
	yakScriptFS = &incrementalAIToolFS{FileSystem: vfs}
}

func countBuiltinAIToolParses(t *testing.T) *atomic.Int32 {
	t.Helper()
	var count atomic.Int32
	previous := parseBuiltinAITool
	t.Cleanup(func() { parseBuiltinAITool = previous })
	parseBuiltinAITool = func(name, code string) *schema.AIYakTool {
		count.Add(1)
		return previous(name, code)
	}
	return &count
}

func TestBuiltinAIToolIncrementalMigration(t *testing.T) {
	scripts := map[string]string{
		"first":  `__DESC__ = "first"; cli.String("target"); cli.check(); yakit.AIOutput("ok")`,
		"second": `__DESC__ = "second"; println("ok")`,
	}
	db := newIncrementalAIToolDB(t, scripts)
	// Legacy hash and unset output option must be migrated in place.
	legacy := &schema.AIYakTool{Name: "first", Content: scripts["first"], Path: filepath.Join("test", "first"), IsBuiltin: true, IsFavorite: true}
	require.NoError(t, db.Create(legacy).Error)
	require.NoError(t, db.Model(legacy).UpdateColumn("hash", "legacy-hash").Error)
	count := countBuiltinAIToolParses(t)
	require.NoError(t, syncBuiltinAITools(db, "archive:1"))
	require.EqualValues(t, 2, count.Swap(0))
	first, err := yakit.GetAIYakTool(db, "first")
	require.NoError(t, err)
	require.Equal(t, legacy.ID, first.ID)
	require.Equal(t, legacy.CreatedAt.UnixNano(), first.CreatedAt.UnixNano())
	require.True(t, first.IsFavorite)
	require.Equal(t, 2, first.EnableAIOutputLog)
	second, err := yakit.GetAIYakTool(db, "second")
	require.NoError(t, err)
	require.Equal(t, 1, second.EnableAIOutputLog)
	require.Equal(t, first.CalcHash(), first.Hash)

	start := time.Now()
	require.NoError(t, syncBuiltinAITools(db, "archive:1"))
	t.Logf("hot sync: %s", time.Since(start))
	require.Zero(t, count.Swap(0), "package gate must not parse")
	require.NoError(t, syncBuiltinAITools(db, "repacked:1"))
	require.Zero(t, count.Swap(0), "a repacked archive must not parse unchanged items")

	// Equal byte length is deliberately insufficient to skip this change.
	scripts["first"] = `__DESC__ = "FIRST"; cli.String("target"); cli.check(); yakit.AIOutput("ok")`
	setIncrementalAIToolFS(scripts)
	start = time.Now()
	require.NoError(t, syncBuiltinAITools(db, "changed:1"))
	t.Logf("one changed item: %s", time.Since(start))
	require.EqualValues(t, 1, count.Swap(0))
	updated, err := yakit.GetAIYakTool(db, "first")
	require.NoError(t, err)
	require.Equal(t, "FIRST", updated.Description)
	require.True(t, updated.IsFavorite)
	untouched, err := yakit.GetAIYakTool(db, "second")
	require.NoError(t, err)
	require.Equal(t, second.UpdatedAt.UnixNano(), untouched.UpdatedAt.UnixNano())

	delete(scripts, "second")
	setIncrementalAIToolFS(scripts)
	require.NoError(t, syncBuiltinAITools(db, "removed:1"))
	require.Zero(t, count.Swap(0))
	_, err = yakit.GetAIYakTool(db, "second")
	require.True(t, gorm.IsRecordNotFoundError(err))

	// A stored hash from an earlier extractor must reparse every remaining item.
	require.NoError(t, db.Model(updated).UpdateColumn("hash", utils.CalcSha1(updated.Name, updated.Content, updated.Path, "old-extractor")).Error)
	require.NoError(t, syncBuiltinAITools(db, "removed:2"))
	require.EqualValues(t, 1, count.Swap(0))

	// Editing clears builtin ownership, but retains the author.
	updated.IsBuiltin, updated.Author, updated.Content = false, schema.AIResourceAuthorBuiltin, `println("mine")`
	_, err = yakit.UpdateAIYakToolByID(db, updated)
	require.NoError(t, err)
	require.NoError(t, syncBuiltinAITools(db, "updated-again:2"))
	require.Zero(t, count.Swap(0))
	custom, err := yakit.GetAIYakTool(db, "first")
	require.NoError(t, err)
	require.False(t, custom.IsBuiltin)
	require.Equal(t, updated.Content, custom.Content)
}

func TestBuiltinAIToolMarkerRollback(t *testing.T) {
	db := newIncrementalAIToolDB(t, map[string]string{"first": `println("one")`, "second": `println("two")`})
	require.NoError(t, yakit.SetKey(db, builtinAIToolEmbedKey, "legacy"))
	retired := &schema.AIYakTool{Name: "retired", IsBuiltin: true}
	require.NoError(t, db.Create(retired).Error)
	// The marker is written last: its failure must roll back every preceding write.
	require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TRIGGER fail_sync_marker BEFORE UPDATE ON general_storages WHEN NEW.key = '%s' BEGIN SELECT RAISE(FAIL, 'marker failure'); END`, `"`+builtinAIToolEmbedKey+`"`)).Error)
	require.ErrorContains(t, syncBuiltinAITools(db, "new:1"), "marker failure")
	require.Equal(t, "legacy", yakit.GetKey(db, builtinAIToolEmbedKey))
	var count int
	require.NoError(t, db.Model(&schema.AIYakTool{}).Count(&count).Error)
	require.Equal(t, 1, count)
	require.NoError(t, db.Exec(`DROP TRIGGER fail_sync_marker`).Error)
	require.NoError(t, syncBuiltinAITools(db, "new:1"))
	require.Equal(t, "new:1", yakit.GetKey(db, builtinAIToolEmbedKey))
}

func TestBuiltinAIToolSyncUsesTwoWorkers(t *testing.T) {
	db := newIncrementalAIToolDB(t, map[string]string{"one": `println(1)`, "two": `println(2)`, "three": `println(3)`})
	started, release := make(chan struct{}, 3), make(chan struct{})
	previous := parseBuiltinAITool
	parseBuiltinAITool = func(name, code string) *schema.AIYakTool {
		started <- struct{}{}
		<-release
		return previous(name, code)
	}
	result := make(chan error, 1)
	go func() { result <- syncBuiltinAITools(db, "workers:1") }()
	defer func() {
		close(release)
		err := <-result
		parseBuiltinAITool = previous
		require.NoError(t, err)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("two workers must start concurrently")
		}
	}
	select {
	case <-started:
		t.Fatal("more than two parse workers started")
	default:
	}
}

func TestBuiltinAIToolUnprefixedArchivePath(t *testing.T) {
	newIncrementalAIToolDB(t, nil)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("nested/demo.yak", "println(1)")
	yakScriptFS = &incrementalAIToolFS{FileSystem: vfs}
	tools, err := loadAllYakScriptFromEmbedFS()
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, filepath.Join("nested", "demo"), tools[0].Path)
}
