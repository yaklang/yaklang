package ssaapi

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Loading an overlay after a restart must not let a visible definition lead
// back to callers in deleted, overridden, or different compile-unit files.
func TestResilienceStructOverlayBoundsCallerAndUseTraversal(t *testing.T) {
	oldDB := ssadb.GetDB()
	dbPath, db, err := consts.GetTempTestDatabase()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(ssadb.SSAProjectTables...).Error)
	ssadb.SetDB(db)
	t.Cleanup(func() { ssadb.SetDB(oldDB); _ = db.Close(); _ = consts.DeleteDatabaseFile(dbPath) })
	for _, olderDiff := range []bool{false, true} {
		name := "base-definition"
		if olderDiff {
			name = "older-diff-definition"
		}
		t.Run(name, func(t *testing.T) {
			fs := filesys.NewVirtualFs()
			fs.AddFile("Root.java", "class Root {}")
			addCallers := func() {
				fs.AddFile("a/Keep.java", `package a; public class Keep { public static void sink(String value) {} }`)
				fs.AddFile("a/Live.java", `package a; class Live { void run() { Keep.sink("live"); } }`)
				fs.AddFile("a/Deleted.java", `package a; class Deleted { void run() { Keep.sink("deleted"); } }`)
				fs.AddFile("a/Overridden.java", `package a; class Overridden { void run() { Keep.sink("overridden"); } }`)
				fs.AddFile("b/Foreign.java", `package b; import a.Keep; class Foreign { void run() { Keep.sink("foreign"); } }`)
			}
			if !olderDiff {
				addCallers()
			}
			baseName := "struct-overlay-" + uuid.NewString()
			names := []string{baseName}
			t.Cleanup(func() {
				for _, name := range names {
					ProgramCache.Remove(name)
				}
			})
			_, err := ParseProjectWithFS(fs, WithLanguage(ssaconfig.JAVA), WithProgramName(baseName))
			require.NoError(t, err)
			if olderDiff {
				addCallers()
				diffName := baseName + "-diff"
				_, err := ParseProjectWithIncrementalCompile(fs, baseName, diffName, ssaconfig.JAVA)
				require.NoError(t, err)
				names = append(names, diffName)
			}
			current := filesys.NewVirtualFs()
			current.AddFile("Root.java", "class Root {}")
			current.AddFile("a/Keep.java", `package a; public class Keep { public static void sink(String value) {} }`)
			current.AddFile("a/Live.java", `package a; class Live { void run() { Keep.sink("live"); } }`)
			current.AddFile("a/Overridden.java", `package a; class Overridden { void run() {} }`)
			current.AddFile("b/Foreign.java", `package b; import a.Keep; class Foreign { void run() { Keep.sink("foreign"); } }`)
			topName := baseName + "-current"
			_, err = ParseProjectWithIncrementalCompile(current, names[len(names)-1], topName, ssaconfig.JAVA)
			require.NoError(t, err)
			names = append(names, topName)
			for _, name := range names {
				ProgramCache.Remove(name)
				ssadb.GetIrCodeCache(name).Purge()
				ssadb.GetIrTypeCache(name).Purge()
			}
			top, err := FromDatabase(topName)
			require.NoError(t, err)
			require.NotNil(t, top.GetOverlay())
			sources := []*Program{top.GetOverlay().Base}
			for _, layer := range top.GetOverlay().Diff {
				sources = append(sources, layer.Program)
			}
			for _, source := range sources {
				require.Nil(t, source.structBound)
				require.False(t, source.structScanActive)
			}
			require.NoError(t, top.ScanProgramStruct(WithStructRuleRaw(`
desc(mode: "struct", language: "java")
*?{opcode: function} as $definition
$definition() as $callers
$definition -> *?{opcode: call} as $uses
alert $callers
alert $uses
`), ssaconfig.WithNoSaveRisk(true)))
			for _, source := range sources {
				require.Nil(t, source.structBound, "the source query boundary must be restored")
				require.False(t, source.structScanActive)
			}
			require.Nil(t, top.structBound, "the result identity must not retain the last source query boundary")
			require.False(t, top.structScanActive)
			for _, variable := range []string{"callers", "uses"} {
				var paths []string
				for _, result := range top.StructScanResults() {
					require.Equal(t, topName, result.GetProgramName())
					for _, value := range result.GetValues(variable) {
						require.NotNil(t, value.GetRange())
						require.NotNil(t, value.GetRange().GetEditor())
						paths = append(paths, value.GetRange().GetEditor().GetUrl())
					}
				}
				require.NotEmpty(t, paths, "%s must retain the live same-unit caller", variable)
				for _, path := range paths {
					assert.True(t, strings.HasSuffix(path, "/a/Live.java"), "%s crossed visible unit boundary: %s", variable, path)
				}
			}
		})
	}
}

func TestResilienceStructScanMissingOverlayBaseFailsClosed(t *testing.T) {
	oldDB := ssadb.GetDB()
	dbPath, db, err := consts.GetTempTestDatabase()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(ssadb.SSAProjectTables...).Error)
	ssadb.SetDB(db)
	t.Cleanup(func() { ssadb.SetDB(oldDB); _ = db.Close(); _ = consts.DeleteDatabaseFile(dbPath) })
	baseName := "struct-missing-base-" + uuid.NewString()
	diffName := baseName + "-diff"
	t.Cleanup(func() { ProgramCache.Remove(baseName); ProgramCache.Remove(diffName) })
	fs := filesys.NewVirtualFs()
	fs.AddFile("a/Keep.java", `package a; class Keep { void run() { println("base"); } }`)
	_, err = ParseProjectWithFS(fs, WithLanguage(ssaconfig.JAVA), WithProgramName(baseName))
	require.NoError(t, err)
	fs.AddFile("a/Added.java", `package a; class Added { void run() { println("diff"); } }`)
	_, err = ParseProjectWithIncrementalCompile(fs, baseName, diffName, ssaconfig.JAVA)
	require.NoError(t, err)
	require.NoError(t, ssadb.DeleteProgramChecked(db, baseName))
	for _, name := range []string{baseName, diffName} {
		ProgramCache.Remove(name)
		ssadb.GetIrCodeCache(name).Purge()
		ssadb.GetIrTypeCache(name).Purge()
	}
	loaded, err := FromDatabase(diffName)
	require.NoError(t, err, "the generic loader can still expose current diff metadata")
	require.True(t, loaded.IsIncrementalCompile())
	require.False(t, loaded.IsBaseProgram())
	require.Nil(t, loaded.GetOverlay())
	err = loaded.ScanProgramStruct(WithStructRuleRaw(`
desc(mode: "struct", language: "java")
println() as $call
alert $call
`), ssaconfig.WithNoSaveRisk(true))
	require.ErrorContains(t, err, "incremental program overlay unavailable")
	require.Empty(t, loaded.StructScanResults(), "a partial diff must not be reported as a successful complete review")
}

func TestResilienceStructScanMissingPersistedSourceFailsClosed(t *testing.T) {
	oldDB := ssadb.GetDB()
	dbPath, db, err := consts.GetTempTestDatabase()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(ssadb.SSAProjectTables...).Error)
	ssadb.SetDB(db)
	t.Cleanup(func() { ssadb.SetDB(oldDB); _ = db.Close(); _ = consts.DeleteDatabaseFile(dbPath) })
	name := "struct-missing-source-" + uuid.NewString()
	t.Cleanup(func() { ProgramCache.Remove(name) })
	fs := filesys.NewVirtualFs()
	fs.AddFile("a/Keep.java", `package a; class Keep { void run() { println("live"); } }`)
	_, err = ParseProjectWithFS(fs, WithLanguage(ssaconfig.JAVA), WithProgramName(name))
	require.NoError(t, err)
	ir, err := ssadb.GetProgram(name, ssadb.Application)
	require.NoError(t, err)
	ir.FileList["a/Missing.java"] = "missing-editor"
	require.NoError(t, ssadb.UpdateProgramWithError(ir))
	ProgramCache.Remove(name)
	ssadb.GetIrCodeCache(name).Purge()
	ssadb.GetIrTypeCache(name).Purge()
	loaded, err := FromDatabase(name)
	require.NoError(t, err)
	err = loaded.ScanProgramStruct(WithStructRuleRaw(`
desc(mode: "struct", language: "java")
println() as $call
alert $call
`), ssaconfig.WithNoSaveRisk(true))
	require.ErrorContains(t, err, "restore struct compile units")
	require.Empty(t, loaded.StructScanResults(), "missing unit evidence must not fall back to a whole-program scan")
}
