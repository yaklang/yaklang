package ssa

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newRootBuildTestProgram(t *testing.T, programName string) *Program {
	t.Helper()

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	return NewProgram(cfg, ProgramCacheMemory, Application, filesys.NewVirtualFs(), "/tmp/project", 0)
}

func TestRunRootTopLevelBuildsOnce(t *testing.T) {
	prog := newRootBuildTestProgram(t, "root-build-once")
	editor := prog.CreateEditor([]byte("a = 1"), "/tmp/project/main.yak")
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	count := 0
	task := prog.RegisterRootTopLevel("main.yak", editor, builder, func(root *FunctionBuilder) {
		count++
		root.EmitConstInst(1)
	})
	require.NotNil(t, task)

	prog.RunRootBuilds()
	prog.RunRootBuilds()
	require.Equal(t, 1, count)
}

func TestFinishAllowsLazyLibraryExpansion(t *testing.T) {
	prog := newRootBuildTestProgram(t, "finish-expansion")
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
