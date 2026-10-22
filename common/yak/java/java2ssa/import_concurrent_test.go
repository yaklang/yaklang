package java2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// A package nobody is compiling, including a dependency in a later batch, is
// still loaded on demand. A package occupied by another prehandle worker is
// not: that compile would enter the owner's library builder. Serial prehandling
// has no owner and keeps the directory fallback.
func TestConcurrentJavaSkeletonDefersDirectoryImports(t *testing.T) {
	for _, tc := range []struct {
		name       string
		concurrent bool
		occupied   bool
		builds     int
	}{
		{name: "serial", builds: 1},
		{name: "concurrent-free", concurrent: true, builds: 1},
		{name: "concurrent-owned", concurrent: true, occupied: true, builds: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			const source = `package a; import b.B; class A { B value; }`
			vf.AddFile("a/A.java", source)
			vf.AddFile("b/B.java", `package b; class B {}`)
			cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
			require.NoError(t, err)
			app := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, vf, ".", 2)
			app.ProcessInfof = func(string, ...any) {}
			app.SetConcurrentUnitBuild(tc.concurrent)
			app.SetPreHandler(true)
			fb := app.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
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
