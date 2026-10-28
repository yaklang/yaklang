package java2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Planned packages remain owned by the unit scheduler before their library or
// file is published. Other directories are loaded on demand unless occupied by
// another worker. Serial prehandling keeps the directory fallback.
func TestConcurrentJavaSkeletonDefersDirectoryImports(t *testing.T) {
	for _, tc := range []struct {
		name       string
		concurrent bool
		occupied   bool
		planned    bool
		static     bool
		declared   bool
		builds     int
	}{
		{name: "serial", builds: 1},
		{name: "concurrent-free", concurrent: true, builds: 1},
		{name: "concurrent-owned", concurrent: true, occupied: true, builds: 0},
		{name: "concurrent-planned-before-library", concurrent: true, planned: true, builds: 0},
		{name: "concurrent-planned-static-before-library", concurrent: true, planned: true, static: true, builds: 0},
		{name: "concurrent-planned-type-before-file", concurrent: true, planned: true, declared: true, builds: 0},
		{name: "concurrent-planned-static-type-before-file", concurrent: true, planned: true, static: true, declared: true, builds: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			source := `package a; import b.B; class A { B value; }`
			if tc.static {
				source = `package a; import static b.B.value; class A { int field = value; }`
			}
			vf.AddFile("a/A.java", source)
			vf.AddFile("b/B.java", `package b; class B {}`)
			cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
			require.NoError(t, err)
			app := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, vf, ".", 2)
			app.ProcessInfof = func(string, ...any) {}
			app.SetConcurrentUnitBuild(tc.concurrent)
			if tc.planned {
				app.CompileUnits = []*ssa.CompileUnit{{Key: "java:b", LibraryName: "b"}}
			}
			app.SetPreHandler(true)
			fb := app.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
			if tc.declared {
				lib := app.NewLibrary("b", []string{"b"})
				libBuilder := lib.GetAndCreateFunctionBuilder("b", string(ssa.MainFunctionName))
				lib.SetExportType("B", libBuilder.CreateBlueprint("B"))
				_, hasFile := app.GetLibrary("b")
				require.False(t, hasFile, "the declared type must precede file registration")
			}
			editor := app.CreateEditor([]byte(source), "a/A.java")
			fb.SetEditor(editor)
			frontend := CreateBuilder()
			frontend.InitHandler(fb)
			ast, err := frontend.ParseAST(source, frontend.GetAntlrCache())
			require.NoError(t, err)
			defer ssa.ReleaseASTRoot(ast)

			releaseOccupy := func() {}
			if tc.occupied {
				held := make(chan struct{})
				release := make(chan struct{})
				done := make(chan struct{})
				go func() {
					defer close(done)
					unlock := app.OccupyDirectoryPackage("b")
					close(held)
					<-release
					unlock()
				}()
				<-held
				releaseOccupy = func() {
					close(release)
					<-done
				}
			}
			defer releaseOccupy()

			importBuilds := 0
			app.Build = func(ssa.FrontAST, *memedit.MemEditor, *ssa.FunctionBuilder) error {
				importBuilds++
				return nil
			}
			require.NoError(t, frontend.BuildFromAST(ast, fb))
			require.Equal(t, tc.builds, importBuilds)
			require.Positive(t, app.DeferredBuildCount(), "link imports after the skeleton barrier")
		})
	}
}
