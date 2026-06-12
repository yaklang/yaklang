package ssa

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newDeferredBuildTestProgram(t *testing.T, programName string) *Program {
	t.Helper()

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	return NewProgram(cfg, ProgramCacheMemory, Application, filesys.NewVirtualFs(), "/tmp/project", 0)
}

func TestRunDeferredFileBuildsOnce(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "deferred-build-once")
	editor := prog.CreateEditor([]byte("a = 1"), "/tmp/project/main.yak")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	count := 0
	prog.RegisterFileBuild("main.yak", editor, builder, func(fileBuilder *FunctionBuilder) {
		count++
		fileBuilder.EmitConstInst(1)
	})

	prog.RunDeferredBuilds()
	prog.RunDeferredBuilds()
	require.Equal(t, 1, count)
	require.Equal(t, 1, prog.DeferredBuildCount())
	require.Nil(t, prog.deferredBuilds)
}

func TestRunDeferredBuildsDrainsTasksRegisteredDuringBuild(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "deferred-build-drains-register-during-build")

	var ran []string
	prog.RegisterDeferredBuild(DeferredBuildKindFile, "first", func() {
		ran = append(ran, "first")
		prog.RegisterDeferredBuild(DeferredBuildKindFile, "second", func() {
			ran = append(ran, "second")
		})
	})

	prog.RunDeferredBuilds()
	require.Equal(t, []string{"first", "second"}, ran)
	require.Equal(t, 2, prog.DeferredBuildCount())
	require.Nil(t, prog.deferredBuilds)

	prog.RunDeferredBuilds()
	require.Equal(t, []string{"first", "second"}, ran)
}

func TestRunDeferredBuildsForUnitsOnlyDrainsMatchingUnit(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "deferred-build-unit-drain")

	var ran []string
	prog.BeginCompileUnit("unit-a")
	prog.RegisterDeferredBuild(DeferredBuildKindFile, "a", func() {
		ran = append(ran, "a")
	})
	prog.EndCompileUnit()

	prog.BeginCompileUnit("unit-b")
	prog.RegisterDeferredBuild(DeferredBuildKindFile, "b", func() {
		ran = append(ran, "b")
	})
	prog.EndCompileUnit()

	require.True(t, prog.RunDeferredBuildsForUnits([]string{"unit-a"}, nil))
	require.Equal(t, []string{"a"}, ran)

	prog.RunDeferredBuilds()
	require.Equal(t, []string{"a", "b"}, ran)
}

func TestRunDeferredBuildsForUnitsRestoresUnitDuringTask(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "deferred-build-unit-context")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	var ran []string
	prog.BeginCompileUnit("unit-a")
	prog.RegisterDeferredBuild(DeferredBuildKindFile, "a", func() {
		require.Equal(t, "unit-a", prog.CurrentCompileUnit())
		builder.Function.AddLazyBuilder(func() {
			ran = append(ran, "nested")
		})
	})
	prog.EndCompileUnit()

	require.True(t, prog.RunDeferredBuildsForUnits([]string{"unit-a"}, nil))
	require.Equal(t, "", prog.CurrentCompileUnit())
	require.Empty(t, ran)

	prog.LazyBuildForUnits([]string{"unit-a"})
	require.Equal(t, []string{"nested"}, ran)
}

func TestFinishAllowsLazyLibraryExpansion(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "finish-expansion")
	editor := prog.CreateEditor([]byte("package main"), "/tmp/project/main.go")
	prog.PushEditor(editor)
	defer prog.PopEditor(false)

	lib := prog.NewLibrary("main", []string{"main"})
	require.NotNil(t, lib)

	builder := lib.GetAndCreateFunctionBuilder("main", "worker")
	require.NotNil(t, builder)
	errCh := make(chan error, 1)
	builder.Function.AddLazyBuilder(func() {
		child, err := lib.GetOrCreateLibrary("net")
		if err != nil {
			errCh <- err
			return
		}
		if child == nil {
			errCh <- errors.New("lazy library creation returned nil")
		}
	})

	done := make(chan struct{})
	go func() {
		prog.Finish()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("program finish deadlocked while adding lazy libraries")
	}
	select {
	case err := <-errCh:
		require.NoError(t, err)
	default:
	}
}

func TestLazyBuildForUnitsOnlyRunsMatchingLazyTasks(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "lazy-build-unit-drain")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	var ran []string
	prog.BeginCompileUnit("unit-a")
	builder.Function.AddLazyBuilder(func() {
		ran = append(ran, "a")
	})
	prog.EndCompileUnit()

	prog.BeginCompileUnit("unit-b")
	builder.Function.AddLazyBuilder(func() {
		ran = append(ran, "b")
	})
	prog.EndCompileUnit()

	prog.LazyBuildForUnits([]string{"unit-a"})
	require.Equal(t, []string{"a"}, ran)

	prog.LazyBuild()
	require.Equal(t, []string{"a", "b"}, ran)
}

func TestLazyBuildForUnitsKeepsBuilderOpenForLaterUnits(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "lazy-build-unit-keeps-builder-open")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	var ran []string
	prog.BeginCompileUnit("unit-a")
	builder.Function.AddLazyBuilder(func() {
		ran = append(ran, "a")
	})
	prog.EndCompileUnit()

	prog.LazyBuildForUnits([]string{"unit-a"})
	require.Equal(t, []string{"a"}, ran)

	prog.BeginCompileUnit("unit-b")
	builder.Function.AddLazyBuilder(func() {
		ran = append(ran, "b")
	})
	prog.EndCompileUnit()

	prog.LazyBuildForUnits([]string{"unit-b"})
	require.Equal(t, []string{"a", "b"}, ran)
}

func TestLazyBuildForUnitsClearsOnlyIndexedUnitBuilders(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "lazy-build-unit-index-clears-unit")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	prog.BeginCompileUnit("unit-a")
	builder.Function.AddLazyBuilder(func() {})
	prog.EndCompileUnit()

	prog.BeginCompileUnit("unit-b")
	builder.Function.AddLazyBuilder(func() {})
	prog.EndCompileUnit()

	require.Contains(t, prog.lazyBuildersByUnit, "unit-a")
	require.Contains(t, prog.lazyBuildersByUnit, "unit-b")

	prog.LazyBuildForUnits([]string{"unit-a"})

	require.NotContains(t, prog.lazyBuildersByUnit, "unit-a")
	require.Contains(t, prog.lazyBuildersByUnit, "unit-b")
}

func TestLazyBuildForUnitsDrainsIndexedBuildersRegisteredDuringBuild(t *testing.T) {
	prog := newDeferredBuildTestProgram(t, "lazy-build-unit-index-drains-new-builders")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	var ran []string
	prog.BeginCompileUnit("unit-a")
	builder.Function.AddLazyBuilder(func() {
		require.Equal(t, "unit-a", prog.CurrentCompileUnit())
		ran = append(ran, "first")
		builder.Function.AddLazyBuilder(func() {
			require.Equal(t, "unit-a", prog.CurrentCompileUnit())
			ran = append(ran, "second")
		})
	})
	prog.EndCompileUnit()

	prog.LazyBuildForUnits([]string{"unit-a"})

	require.Equal(t, "", prog.CurrentCompileUnit())
	require.Equal(t, []string{"first", "second"}, ran)
	require.NotContains(t, prog.lazyBuildersByUnit, "unit-a")
}

func TestLazyBuildForUnitsSkipsProgramCycles(t *testing.T) {
	progA := newDeferredBuildTestProgram(t, "lazy-build-unit-cycle-a")
	progB := newDeferredBuildTestProgram(t, "lazy-build-unit-cycle-b")
	builderA := progA.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	builderB := progB.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	var ran []string
	progA.BeginCompileUnit("unit-a")
	builderA.Function.AddLazyBuilder(func() {
		ran = append(ran, "a")
	})
	progA.EndCompileUnit()

	progB.BeginCompileUnit("unit-a")
	builderB.Function.AddLazyBuilder(func() {
		ran = append(ran, "b")
	})
	progB.EndCompileUnit()

	progA.AddUpStream(progB)
	progB.AddUpStream(progA)

	done := make(chan struct{})
	go func() {
		progA.LazyBuildForUnits([]string{"unit-a"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("lazy build for units did not finish across cyclic program graph")
	}
	require.ElementsMatch(t, []string{"a", "b"}, ran)

	progA.LazyBuildForUnits([]string{"unit-a"})
	require.ElementsMatch(t, []string{"a", "b"}, ran)
}
