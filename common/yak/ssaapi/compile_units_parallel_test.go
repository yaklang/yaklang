package ssaapi

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestCompileUnitBuildWavesDependencyDiamond(t *testing.T) {
	units := make(map[string]*CompileUnit)
	for _, key := range []string{"base", "left", "right", "top", "cycle1", "cycle2"} {
		units[key] = testCompileUnit(key, 1, 1)
	}
	plan := topoCompileUnits(units, []UnitRef{
		{From: "left", To: "base"}, {From: "right", To: "base"},
		{From: "top", To: "left"}, {From: "top", To: "right"},
		{From: "cycle1", To: "cycle2"}, {From: "cycle2", To: "cycle1"},
		{From: "cycle1", To: "top"},
	})
	waves := compileUnitBuildWaves(plan, compileUnitExecutionBatch{unitKeys: []string{"base", "left", "right", "top", "cycle1", "cycle2"}})
	require.Len(t, waves, 4)
	require.Equal(t, [][]string{{"base"}}, waves[0])
	require.ElementsMatch(t, [][]string{{"left"}, {"right"}}, waves[1])
	require.Equal(t, [][]string{{"top"}}, waves[2])
	require.Equal(t, [][]string{{"cycle1", "cycle2"}}, waves[3])
}

func TestCompileUnitParallelQualifiedDependency(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/A.java", `package a; public class A extends b.B { c.C value; }`)
	vf.AddFile("b/B.java", `package b; public class B {}`)
	vf.AddFile("c/C.java", `package c; public class C {}`)
	plan := buildPlanForLanguage(t, ssaconfig.JAVA, vf, []string{"a/A.java", "b/B.java", "c/C.java"})
	waves := compileUnitBuildWaves(plan, compileUnitExecutionBatch{unitKeys: []string{"java:a", "java:b", "java:c"}})
	require.Len(t, waves, 2)
	require.ElementsMatch(t, [][]string{{"java:b"}, {"java:c"}}, waves[0])
	require.Equal(t, [][]string{{"java:a"}}, waves[1])
}

func TestCompileUnitBuildWaveOverlapsLibraries(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var completed atomic.Int32
	for _, name := range []string{"a", "b"} {
		lib := prog.NewLibrary(name, []string{name})
		lib.NewFunction("work").AddLazyBuilder(func() {
			entered <- struct{}{}
			<-release
			completed.Add(1)
		})
	}
	finished := make(chan error, 1)
	go func() {
		_, err := buildCompileUnitWave(context.Background(), prog, [][]string{{"java:a"}, {"java:b"}}, 2)
		finished <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("independent library compilation did not overlap")
		}
	}
	require.EqualValues(t, 0, completed.Load())
	close(release)
	require.NoError(t, <-finished)
	require.EqualValues(t, 2, completed.Load())
}

func TestCompileUnitSCCGroupLibrariesOverlap(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var completed atomic.Int32
	for _, name := range []string{"a", "b"} {
		lib := prog.NewLibrary(name, []string{name})
		lib.NewFunction("work").AddLazyBuilder(func() {
			entered <- struct{}{}
			<-release
			completed.Add(1)
		})
	}
	finished := make(chan error, 1)
	go func() {
		_, err := buildCompileUnitWave(context.Background(), prog, [][]string{{"java:a", "java:b"}}, 2)
		finished <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("libraries inside one SCC group did not overlap")
		}
	}
	require.EqualValues(t, 0, completed.Load())
	close(release)
	require.NoError(t, <-finished)
	require.EqualValues(t, 2, completed.Load())
}

func TestCompileUnitParallelJavaMemory(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("base/Base.java", "package base;\n@Example public class Base { public static String shared = \"shared\"; public String field; public static String value(String s) { return s; } }")
	for i := 0; i < 12; i++ {
		vf.AddFile(fmt.Sprintf("p%d/C.java", i), fmt.Sprintf(`package p%d;
import base.Base;
public class C {
  String f(String input) { String value = Base.value(input); System.out.println(value); return value; }
  int n(int a) { int r = 0; for (int i = 0; i < a; i++) { r += i; } return r; }
  String fields(Base b) { Base.shared = "changed"; b.field = Base.shared; return b.field; }
  Base instance() { return new Base(); }
}`, i))
	}
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.JAVA), WithMemory(), ssaconfig.WithCompileConcurrency(4))
	require.NoError(t, err)
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.NotNil(t, program)
	result, err := program.SyntaxFlowWithError(`println(* as $value) as $call`)
	require.NoError(t, err)
	require.Equal(t, 12, result.GetValues("call").Len())
}

type overlappingUnitParser struct {
	ssa.Builder
	entered  atomic.Int32
	once     sync.Once
	ready    chan struct{}
	timedOut atomic.Bool
}

func (b *overlappingUnitParser) ParseAST(source string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	if b.entered.Add(1) == 2 {
		b.once.Do(func() { close(b.ready) })
	}
	select {
	case <-b.ready:
	case <-time.After(5 * time.Second):
		b.timedOut.Store(true)
		b.once.Do(func() { close(b.ready) })
	}
	return b.Builder.ParseAST(source, cache)
}

func TestCompileUnitParallelParserSharesBatchWindow(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/A.java", `package a; class A { int f() { return 1; } }`)
	vf.AddFile("b/B.java", `package b; class B { int f() { return 2; } }`)
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.JAVA), WithMemory(), ssaconfig.WithCompileConcurrency(2))
	require.NoError(t, err)
	parser := &overlappingUnitParser{Builder: cfg.LanguageBuilder, ready: make(chan struct{})}
	cfg.LanguageBuilder = parser
	_, err = cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.False(t, parser.timedOut.Load(), "a one-file unit must not prevent the next unit from parsing in the same bounded batch")
}

func TestCompileUnitBuildWaveCancellationJoinsWorkers(t *testing.T) {
	for _, second := range []string{"java:b", "dir:shared"} {
		t.Run(second, func(t *testing.T) {
			cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
			require.NoError(t, err)
			prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
			entered, release := make(chan struct{}), make(chan struct{})
			var unexpected atomic.Bool
			prog.NewLibrary("a", []string{"a"}).NewFunction("a").AddLazyBuilder(func() {
				close(entered)
				<-release
			})
			prog.NewLibrary("b", []string{"b"}).NewFunction("b").AddLazyBuilder(func() { unexpected.Store(true) })
			prog.NewFunction("app").AddLazyBuilder(func() { unexpected.Store(true) })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				_, err := buildCompileUnitWave(ctx, prog, [][]string{{"java:a"}, {second}}, 1)
				finished <- err
			}()
			<-entered
			cancel()
			select {
			case <-finished:
				t.Error("cancellation returned before the running builder released its AST")
			default:
			}
			close(release)
			require.ErrorIs(t, <-finished, context.Canceled)
			require.False(t, unexpected.Load(), "queued or exclusive work must not start after cancellation")
		})
	}
}

func TestCompileUnitParallelJavaDatabaseReload(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("base/Base.java", `package base; class Base { static String value() { return "parallel-source"; } }`)
	for _, item := range []struct{ pkg, class string }{{"left", "Left"}, {"right", "Right"}} {
		vf.AddFile(item.pkg+"/"+item.class+".java", fmt.Sprintf(`package %s; import base.Base;
class %s { static String value() { return Base.value(); } }`, item.pkg, item.class))
	}
	vf.AddFile("top/Top.java", `package top; import left.Left; import right.Right;
class Top { void run() { println(Left.value()); println(Right.value()); } }`)
	name := uuid.NewString()
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), name) })
	programs, err := ParseProjectWithFS(vf, WithLanguage(ssaconfig.JAVA), WithProgramName(name),
		ssaconfig.WithCompileConcurrency(4), ssaconfig.WithCompileIrCacheTTL(time.Millisecond),
		ssaconfig.WithCompileIrCacheMax(1))
	require.NoError(t, err)
	require.Len(t, programs, 1)
	ProgramCache.Remove(name)
	loaded, err := FromDatabase(name)
	require.NoError(t, err)
	require.NotSame(t, programs[0].Program, loaded.Program)
	result, err := loaded.SyntaxFlowWithError(`println(* #-> * as $source) as $call`)
	require.NoError(t, err)
	require.Len(t, result.GetValues("call"), 2)
	require.Contains(t, result.GetValues("source").String(), "parallel-source")
	for _, value := range result.GetValues("call") {
		require.NotNil(t, value.GetRange(), "persisted parallel IR must retain source locations")
	}
}

func TestPrehandleUnitWorkerPinsApplicationAndHashesPackages(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
	env := &prehandleBatchEnv{prog: prog}
	require.Equal(t, 0, env.unitWorker("resource:web", 8))
	require.Equal(t, 0, env.unitWorker("dir:src", 8))
	require.Equal(t, 0, env.unitWorker("", 8))
	a := env.unitWorker("java:a", 8)
	require.NotZero(t, a)
	require.Equal(t, a, env.unitWorker("java:a", 8))
	seen := map[int]bool{}
	for i := 0; i < 40; i++ {
		seen[env.unitWorker(fmt.Sprintf("java:p%d", i), 8)] = true
	}
	require.Greater(t, len(seen), 1)
}

func TestCompileUnitPrehandlerIndependentPackages(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for i := 0; i < 8; i++ {
		vf.AddFile(fmt.Sprintf("p%d/C.java", i), fmt.Sprintf("package p%d; public class C { public static String n() { return \"p%d\"; } }", i, i))
	}
	vf.AddFile("top/Top.java", `package top;
import p0.C;
public class Top { String run() { println(C.n()); return C.n(); } }`)
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.JAVA), WithMemory(), ssaconfig.WithCompileConcurrency(8))
	require.NoError(t, err)
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.NotNil(t, program)
	result, err := program.SyntaxFlowWithError(`println(* #-> * as $source) as $call`)
	require.NoError(t, err)
	require.Equal(t, 1, result.GetValues("call").Len())
	require.Contains(t, result.GetValues("source").String(), "p0")
}

func TestCompileUnitApplicationDoesNotDrainReadyLibraries(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
	library := prog.NewLibrary("child", []string{"child"})
	var calls atomic.Int32
	library.NewFunction("work").AddLazyBuilder(func() { calls.Add(1) })
	built := make(map[*ssa.Program]bool)
	_, err = buildCompileUnitWaveWithOwners(context.Background(), prog, [][]string{{"resource:one"}, {"dir:two"}}, 2, built)
	require.NoError(t, err)
	require.EqualValues(t, 0, calls.Load(), "application metadata must not consume a library's scheduled work")
	peak, err := buildCompileUnitWaveWithOwners(context.Background(), prog, [][]string{{"resource:one"}}, 2, built)
	require.NoError(t, err)
	require.Zero(t, peak, "the same owner must not be drained again in a later wave")
	_, err = buildCompileUnitWaveWithOwners(context.Background(), prog, [][]string{{"java:child"}}, 2, built)
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
}
