package ssaapi

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

// Force each possible serial order of two cyclic libraries. A one-slot SCC
// scheduler still launches both owners, so semaphore acquisition order must not
// decide whether implementation instances are indexed under their interface.
func TestJavaHierarchyBeforeSCCBodies(t *testing.T) {
	for _, order := range [][]string{{"api", "impl"}, {"impl", "api"}} {
		t.Run(strings.Join(order, "-first-"), func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			files := []struct{ path, unit, code string }{
				{"api/IA.java", "java:api", `package api; public interface IA { void set(int value); }`},
				{"api/Consumer.java", "java:api", `package api;
import impl.First;
import impl.Second;
class Consumer {
    IA face;
    First first;
    Second second;
    void run() { face.set(0); first.set(1); second.set(2); }
}`},
				{"impl/First.java", "java:impl", `package impl; import api.IA; public class First implements IA { public void set(int value) {} }`},
				{"impl/Second.java", "java:impl", `package impl; import api.IA; public class Second implements IA { public void set(int value) {} }`},
			}
			for _, file := range files {
				vf.AddFile(file.path, file.code)
			}
			cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.JAVA), WithMemory(), ssaconfig.WithCompileConcurrency(1))
			require.NoError(t, err)
			prog, builder, err := cfg.init(vf, len(files))
			require.NoError(t, err)
			prog.SetPreHandler(true)
			prog.SetConcurrentUnitBuild(true)
			cfg.LanguageBuilder.InitHandler(builder)
			keys := []string{"java:api", "java:impl"}
			for _, key := range keys {
				prog.EnsureLibraryForCompileUnit(key)
			}

			// Both skeleton owners are active until every declaration exists.
			// This models the real prehandler barrier without on-demand imports
			// compiling a library a second time in this focused fixture.
			occupied := make(chan struct{})
			release := make(chan struct{})
			released := make(chan struct{})
			go func() {
				defer close(released)
				unlockAPI := prog.OccupyDirectoryPackage("api")
				unlockImpl := prog.OccupyDirectoryPackage("impl")
				defer unlockAPI()
				defer unlockImpl()
				close(occupied)
				<-release
			}()
			<-occupied
			stopped := false
			stopSkeletons := func() {
				if !stopped {
					close(release)
					<-released
					stopped = true
				}
			}
			t.Cleanup(stopSkeletons)

			env := &prehandleBatchEnv{
				c: cfg, prog: prog, builder: builder, filesystem: vf,
				fileUnits:         make(map[string]string),
				preHandlerFileSet: make(map[string]struct{}), handlerFileSet: make(map[string]struct{}),
				preHandlerBuildsFiles: true, preHandlerProcess: func() {}, concurrentUnits: true, concurrency: 1,
			}
			for _, file := range files {
				env.fileUnits[file.path] = file.unit
				env.preHandlerFileSet[file.path] = struct{}{}
				env.handlerFileSet[file.path] = struct{}{}
				ast, err := cfg.LanguageBuilder.ParseAST(file.code, cfg.LanguageBuilder.GetAntlrCache())
				require.NoError(t, err)
				env.prehandleFile(&ssareducer.FileContent{Path: file.path, Content: []byte(file.code), AST: ast})
			}
			require.NoError(t, env.astErr)
			stopSkeletons()
			for _, prefix := range []string{"java-static-import:", "java-type-hierarchy:"} {
				require.True(t, prog.RunDeferredBuildsForUnitsMatching(keys, nil, nil, func(id string) bool {
					return strings.Contains(id, prefix)
				}))
			}
			prog.SetPreHandler(false)
			for _, name := range order {
				library, _ := prog.GetLibrary(name)
				require.NotNil(t, library)
				library.LazyBuildCurrentProgram()
			}
			result, err := NewProgram(prog, cfg).SyntaxFlowWithError(`IA.set(* #-> ?{opcode: const} as $value) as $call`)
			require.NoError(t, err)
			var got []string
			for _, value := range result.GetValues("value") {
				got = append(got, value.String())
			}
			sort.Strings(got)
			require.Equal(t, []string{"0", "1", "2"}, got)
			require.Len(t, result.GetValues("call"), 3)
		})
	}
}
