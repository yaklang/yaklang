package ssaapi

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newPipelineTestProgram(t *testing.T, names ...string) *ssa.Program {
	t.Helper()
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	prog := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, filesys.NewVirtualFs(), ".", 0)
	for _, name := range names {
		prog.CompileUnits = append(prog.CompileUnits, &ssa.CompileUnit{Key: "java:" + name, LibraryName: name})
		prog.NewLibrary(name, []string{name})
	}
	return prog
}

func pipelineTestPlan(names []string, edges ...UnitRef) (*UnitPlan, compileUnitExecutionBatch) {
	plan := &UnitPlan{Edges: edges}
	batch := compileUnitExecutionBatch{}
	for _, name := range names {
		key := "java:" + name
		unit := &CompileUnit{Key: key}
		plan.Order = append(plan.Order, []*CompileUnit{unit})
		batch.unitKeys = append(batch.unitKeys, key)
	}
	return plan, batch
}

func TestCompileUnitPipelineReleasesDependentBeforeSlowSibling(t *testing.T) {
	prog := newPipelineTestProgram(t, "base", "slow", "child")
	slowEntered := make(chan struct{})
	releaseSlow := make(chan struct{})
	childEntered := make(chan struct{})
	prog.ProgramForCompileUnit("java:slow").NewFunction("slow").AddLazyBuilder(func() {
		close(slowEntered)
		<-releaseSlow
	})
	prog.ProgramForCompileUnit("java:child").NewFunction("child").AddLazyBuilder(func() { close(childEntered) })
	plan, batch := pipelineTestPlan([]string{"base", "slow", "child"}, UnitRef{From: "java:child", To: "java:base"})
	finished := make(chan error, 1)
	go func() {
		_, err := buildCompileUnitPipeline(context.Background(), prog, plan, batch, 2)
		finished <- err
	}()
	select {
	case <-slowEntered:
	case <-time.After(5 * time.Second):
		close(releaseSlow)
		t.Fatal("slow sibling did not start")
	}
	select {
	case <-childEntered:
	case <-time.After(5 * time.Second):
		close(releaseSlow)
		t.Fatal("ready dependent waited for unrelated slow sibling")
	}
	close(releaseSlow)
	require.NoError(t, <-finished)
}

func TestCompileUnitPipelineBoundsConcurrentLibraries(t *testing.T) {
	prog := newPipelineTestProgram(t, "a", "b", "c")
	entered := make(chan string, 3)
	release := make(chan struct{})
	for _, name := range []string{"a", "b", "c"} {
		name := name
		prog.ProgramForCompileUnit("java:" + name).NewFunction("work").AddLazyBuilder(func() {
			entered <- name
			<-release
		})
	}
	plan, batch := pipelineTestPlan([]string{"a", "b", "c"})
	finished := make(chan struct{})
	var peak int
	var err error
	go func() {
		peak, err = buildCompileUnitPipeline(context.Background(), prog, plan, batch, 2)
		close(finished)
	}()
	for range 2 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("two independent libraries did not overlap")
		}
	}
	select {
	case <-entered:
		close(release)
		t.Fatal("third library exceeded concurrency limit")
	default:
	}
	close(release)
	<-finished
	require.NoError(t, err)
	require.Equal(t, 2, peak)
}

func TestCompileUnitPipelineApplicationIsExclusive(t *testing.T) {
	prog := newPipelineTestProgram(t, "child")
	appEntered := make(chan struct{})
	releaseApp := make(chan struct{})
	childEntered := make(chan struct{})
	prog.NewFunction("app").AddLazyBuilder(func() {
		close(appEntered)
		<-releaseApp
	})
	prog.ProgramForCompileUnit("java:child").NewFunction("child").AddLazyBuilder(func() {
		close(childEntered)
	})
	plan := &UnitPlan{Order: [][]*CompileUnit{{{Key: "resource:one"}}, {{Key: "java:child"}}}}
	batch := compileUnitExecutionBatch{unitKeys: []string{"resource:one", "java:child"}}
	finished := make(chan error, 1)
	go func() {
		_, err := buildCompileUnitPipeline(context.Background(), prog, plan, batch, 2)
		finished <- err
	}()
	select {
	case <-appEntered:
	case <-time.After(5 * time.Second):
		close(releaseApp)
		t.Fatal("application build did not start")
	}
	select {
	case <-childEntered:
		close(releaseApp)
		t.Fatal("library build overlapped the application")
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseApp)
	require.NoError(t, <-finished)
	select {
	case <-childEntered:
	default:
		t.Fatal("library build was not resumed after application")
	}
}

func TestCompileUnitPipelineCancellationJoinsRunningBuild(t *testing.T) {
	prog := newPipelineTestProgram(t, "a", "b")
	entered := make(chan struct{})
	release := make(chan struct{})
	prog.ProgramForCompileUnit("java:a").NewFunction("work").AddLazyBuilder(func() {
		close(entered)
		<-release
	})
	plan, batch := pipelineTestPlan([]string{"a", "b"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := buildCompileUnitPipeline(ctx, prog, plan, batch, 1)
		finished <- err
	}()
	<-entered
	cancel()
	select {
	case <-finished:
		close(release)
		t.Fatal("pipeline returned before running build completed")
	default:
	}
	close(release)
	require.ErrorIs(t, <-finished, context.Canceled)
}
