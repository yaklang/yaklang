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
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type phpPrehandlerProbe struct {
	ssa.Builder
	active atomic.Int32
	peak   atomic.Int32
}

func (*phpPrehandlerProbe) SupportsConcurrentUnitPreHandler() bool { return true }

func (p *phpPrehandlerProbe) PreHandlerProject(fs fi.FileSystem, ast ssa.FrontAST, builder *ssa.FunctionBuilder, editor *memedit.MemEditor) error {
	current := p.active.Add(1)
	defer p.active.Add(-1)
	for previous := p.peak.Load(); current > previous; previous = p.peak.Load() {
		if p.peak.CompareAndSwap(previous, current) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	return p.Builder.PreHandlerProject(fs, ast, builder, editor)
}

func TestCompileUnitPipelineDependencyDiamond(t *testing.T) {
	units := make(map[string]*CompileUnit)
	for _, key := range []string{"java:base", "java:left", "java:right", "java:top", "java:cycle1", "java:cycle2"} {
		units[key] = testCompileUnit(key, 1, 1)
	}
	plan := topoCompileUnits(units, []UnitRef{
		{From: "java:left", To: "java:base"}, {From: "java:right", To: "java:base"},
		{From: "java:top", To: "java:left"}, {From: "java:top", To: "java:right"},
		{From: "java:cycle1", To: "java:cycle2"}, {From: "java:cycle2", To: "java:cycle1"},
		{From: "java:cycle1", To: "java:top"},
	})
	prog := newPipelineTestProgram(t, "base", "left", "right", "top", "cycle1", "cycle2")
	nodes := compileUnitBuildNodes(prog, plan, compileUnitExecutionBatch{unitKeys: []string{"java:base", "java:left", "java:right", "java:top", "java:cycle1", "java:cycle2"}})
	require.Len(t, nodes, 5)
	remaining := make(map[string]int)
	for _, node := range nodes {
		for _, key := range node.keys {
			remaining[key] = node.remaining
		}
	}
	require.Equal(t, 0, remaining["java:base"])
	require.Equal(t, 1, remaining["java:left"])
	require.Equal(t, 1, remaining["java:right"])
	require.Equal(t, 2, remaining["java:top"])
	require.Equal(t, 1, remaining["java:cycle1"])
	require.Equal(t, 1, remaining["java:cycle2"])
}

func TestCompileUnitParallelQualifiedDependency(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/A.java", `package a; public class A extends b.B { c.C value; }`)
	vf.AddFile("b/B.java", `package b; public class B {}`)
	vf.AddFile("c/C.java", `package c; public class C {}`)
	plan := buildPlanForLanguage(t, ssaconfig.JAVA, vf, []string{"a/A.java", "b/B.java", "c/C.java"})
	prog := newPipelineTestProgram(t, "a", "b", "c")
	nodes := compileUnitBuildNodes(prog, plan, compileUnitExecutionBatch{unitKeys: []string{"java:a", "java:b", "java:c"}})
	require.Len(t, nodes, 3)
	remaining := make(map[string]int)
	for _, node := range nodes {
		remaining[node.keys[0]] = node.remaining
	}
	require.Equal(t, 2, remaining["java:a"])
	require.Zero(t, remaining["java:b"])
	require.Zero(t, remaining["java:c"])
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
		plan, batch := pipelineTestPlan([]string{"a", "b"})
		_, err := buildCompileUnitPipeline(context.Background(), prog, plan, batch, 2)
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
		plan := &UnitPlan{Order: [][]*CompileUnit{{{Key: "java:a"}, {Key: "java:b"}}}}
		batch := compileUnitExecutionBatch{unitKeys: []string{"java:a", "java:b"}}
		_, err := buildCompileUnitPipeline(context.Background(), prog, plan, batch, 2)
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
				plan := &UnitPlan{Order: [][]*CompileUnit{{{Key: "java:a"}}, {{Key: second}}}}
				batch := compileUnitExecutionBatch{unitKeys: []string{"java:a", second}}
				_, err := buildCompileUnitPipeline(ctx, prog, plan, batch, 1)
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
	// Force separate persistence boundaries, including lowercase methods
	// referenced from a later unit. A single batch cannot catch release bugs.
	t.Setenv(compileUnitBatchMinFilesEnv, "1")
	t.Setenv(compileUnitBatchMinBytesEnv, "0")
	t.Setenv(compileUnitBatchMaxFilesEnv, "1")
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
	require.Equal(t, 0, env.unitWorker("dir:.", 8))
	require.NotZero(t, env.unitWorker("dir:src", 8))
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

func TestCompileUnitParallelPythonMemory(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for i := 0; i < 8; i++ {
		vf.AddFile(fmt.Sprintf("p%d/c.py", i), fmt.Sprintf("def n():\n    return \"p%d\"\n", i))
	}
	vf.AddFile("top/t.py", "from p0.c import n\nprint(n())\n")
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.PYTHON), WithMemory(), ssaconfig.WithCompileConcurrency(8))
	require.NoError(t, err)
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.NotNil(t, program)
}

func TestCompileUnitGoMemory(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for i := 0; i < 8; i++ {
		vf.AddFile(fmt.Sprintf("p%d/c.go", i), fmt.Sprintf("package p%d\nfunc N() string { return \"p%d\" }\n", i, i))
	}
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.GO), WithMemory(), ssaconfig.WithCompileConcurrency(8))
	require.NoError(t, err)
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.NotNil(t, program)
	require.True(t, program.Program.ConcurrentUnitBuildEnabled())
	for i := 0; i < 8; i++ {
		key := fmt.Sprintf("dir:p%d", i)
		lib := program.Program.ProgramForCompileUnit(key)
		require.NotNil(t, lib)
		require.NotSame(t, program.Program.GetApplication(), lib, "Go unit %s should drain its package library", key)
	}
}

func TestCompileUnitGoErrorsAreNotReplicatedAcrossFiles(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for i := 0; i < 4; i++ {
		vf.AddFile(fmt.Sprintf("p/file%d.go", i), fmt.Sprintf("package p\nfunc F%d() { break }\n", i))
	}
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.GO), WithMemory())
	require.NoError(t, err)
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.NotNil(t, program)
	breakErrors := 0
	for _, compileErr := range program.GetErrors() {
		if compileErr.Message == "unexpected break stmt" {
			breakErrors++
		}
	}
	require.Equal(t, 4, breakErrors, "each file's error must reach the application exactly once")
}

func TestCompileUnitGoPackageLibrariesOverlap(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
	prog.Language = ssaconfig.GO
	prog.CompileUnits = []*ssa.CompileUnit{
		{Key: "dir:first", LibraryName: "first"},
		{Key: "dir:second", LibraryName: "second"},
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	for _, name := range []string{"first", "second"} {
		lib := prog.NewLibrary(name, []string{name})
		lib.NewFunction("work").AddLazyBuilder(func() {
			entered <- struct{}{}
			<-release
		})
	}
	finished := make(chan error, 1)
	go func() {
		plan := &UnitPlan{Order: [][]*CompileUnit{{{Key: "dir:first"}}, {{Key: "dir:second"}}}}
		batch := compileUnitExecutionBatch{unitKeys: []string{"dir:first", "dir:second"}}
		_, err := buildCompileUnitPipeline(context.Background(), prog, plan, batch, 2)
		finished <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("Go package library builds did not overlap")
		}
	}
	close(release)
	require.NoError(t, <-finished)
}

func TestCompileUnitPHPDirectoryMemory(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for i := 0; i < 8; i++ {
		vf.AddFile(fmt.Sprintf("p%d/C.php", i), fmt.Sprintf("<?php\nnamespace P%d;\nclass C { public static function n() { return \"p%d\"; } }\n", i, i))
	}
	vf.AddFile("top/T.php", "<?php\nnamespace Top;\nuse P0\\C;\nclass T { public function run() { echo C::n(); } }\n")
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.PHP), WithMemory(), ssaconfig.WithCompileConcurrency(8))
	require.NoError(t, err)
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.NotNil(t, program)
}

func TestCompileUnitPHPPrehandlersOverlapAcrossDirectories(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for i := 0; i < 12; i++ {
		vf.AddFile(fmt.Sprintf("p%d/C.php", i), fmt.Sprintf("<?php namespace P%d; class C { public function n() { return %d; } }", i, i))
	}
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.PHP), WithMemory(), ssaconfig.WithCompileConcurrency(8))
	require.NoError(t, err)
	probe := &phpPrehandlerProbe{Builder: php2ssa.CreateBuilder()}
	cfg.LanguageBuilder = probe
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.NotNil(t, program)
	require.Greater(t, probe.peak.Load(), int32(1), "PHP prehandlers never overlapped")
}

func TestCompileUnitParallelTypeScriptMemory(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for i := 0; i < 8; i++ {
		vf.AddFile(fmt.Sprintf("p%d/c.ts", i), fmt.Sprintf("export function n(): string { return \"p%d\"; }\n", i))
	}
	vf.AddFile("top/t.ts", "import { n } from \"../p0/c\";\nconsole.log(n());\n")
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.TS), WithMemory(), ssaconfig.WithCompileConcurrency(8))
	require.NoError(t, err)
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.NotNil(t, program)
	result, err := program.SyntaxFlowWithError(`console.log(* #-> * as $source) as $call`)
	require.NoError(t, err)
	require.NotEmpty(t, result.GetValues("call"))
	require.Contains(t, result.GetValues("source").String(), "p0")
}

func TestCompileUnitApplicationDoesNotDrainReadyLibraries(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
	library := prog.NewLibrary("child", []string{"child"})
	var calls atomic.Int32
	library.NewFunction("work").AddLazyBuilder(func() { calls.Add(1) })
	plan := &UnitPlan{Order: [][]*CompileUnit{{{Key: "resource:one"}}, {{Key: "dir:two"}}, {{Key: "java:child"}}}}
	batch := compileUnitExecutionBatch{unitKeys: []string{"resource:one", "dir:two", "java:child"}}
	nodes := compileUnitBuildNodes(prog, plan, batch)
	require.Len(t, nodes[0].libraries, 1)
	require.Empty(t, nodes[1].libraries, "the same application owner must be drained only once")
	_, err = buildCompileUnitPipeline(context.Background(), prog, plan, batch, 2)
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
}
