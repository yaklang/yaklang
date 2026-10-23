package ssaapi

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestProgramQueryStructRetainsOnlyVisibleOverlayFiles(t *testing.T) {
	old := ssadb.GetDB()
	dbPath, db, err := consts.GetTempTestDatabase()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(ssadb.SSAProjectTables...).Error)
	// Use the production patches too: IR upserts require the unique indexes.
	schema.ApplyPatches(db, schema.KEY_SCHEMA_SSA_DATABASE)
	require.NoError(t, db.Error)
	ssadb.SetDB(db)
	t.Cleanup(func() { ssadb.SetDB(old); _ = db.Close(); _ = consts.DeleteDatabaseFile(dbPath) })
	fs := filesys.NewVirtualFs()
	fs.AddFile("app.py", "def run(x):\n    return eval(x)\n")
	fs.AddFile("other.py", "def safe():\n    return 1\n")
	prefix := "struct-query-" + uuid.NewString()
	names := []string{}
	t.Cleanup(func() {
		for _, name := range names {
			ProgramCache.Remove(name)
		}
	})
	for version, want := range []int{1, 1, 1, 0, 1, 0} {
		switch version {
		case 1:
			require.NoError(t, fs.RemoveFileOrDir("other.py"))
			fs.AddFile("other.py", "def safe():\n    return 2\n")
		case 3:
			require.NoError(t, fs.RemoveFileOrDir("app.py"))
			fs.AddFile("app.py", "def run(x):\n    return x\n")
		case 4:
			require.NoError(t, fs.RemoveFileOrDir("app.py"))
			fs.AddFile("app.py", "def run(x):\n    return eval(x)\n")
		case 5:
			fs = filesys.NewVirtualFs()
			fs.AddFile("other.py", "def safe():\n    return 2\n")
		}
		name := prefix + string(rune('a'+version))
		opts := []ssaconfig.Option{WithLanguage(ssaconfig.PYTHON), WithProgramName(name)}
		if len(names) > 0 {
			opts = append(opts, WithBaseProgramName(names[len(names)-1]))
		}
		_, err := ParseProjectWithFS(fs, opts...)
		if version == 2 {
			// The compiler rejects a truly unchanged diff; it must not fabricate a review.
			require.ErrorContains(t, err, "not found can compiled file")
			continue
		}
		require.NoError(t, err, "compile version %d", version)
		names = append(names, name)
		for _, n := range names {
			ProgramCache.Remove(n)
			ssadb.GetIrCodeCache(n).Purge()
			ssadb.GetIrTypeCache(n).Purge()
		}
		top, err := FromDatabase(name)
		require.NoError(t, err)
		result, err := top.SyntaxFlowWithError("desc(mode: \"struct\", language: \"python\")\neval(* as $arg) as $call\nalert $call", QueryWithMemory())
		require.NoError(t, err, "version %d", version)
		count := 0
		if result != nil {
			count = result.RiskCount()
			require.Equal(t, name, result.GetProgramName())
		}
		require.Equal(t, want, count, "version %d", version)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = top.SyntaxFlowWithError("desc(mode: \"struct\", language: \"python\")\neval(* as $arg) as $call\nalert $call", QueryWithMemory(), QueryWithContext(ctx))
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, top.structBound)
		require.False(t, top.structScanActive)
	}
}
