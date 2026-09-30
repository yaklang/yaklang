package ssa

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestFlushAuxSaversReleasesSourceEditors(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, filesys.NewVirtualFs())
	defer prog.Cache.sources.Close()
	editor := prog.CreateEditor([]byte("class CompletedUnit {}"), "/src/CompletedUnit.java")
	prog.SetEditor(editor.GetUrl(), editor)

	prog.Cache.FlushAuxSavers()
	require.Eventually(t, func() bool {
		return prog.Cache.sources.PayloadCount() == 0 && prog.Cache.sources.EditorCount() == 0
	}, 5*time.Second, 10*time.Millisecond, "completed batch sources must save and release before final close")

	require.True(t, prog.ShouldVisit(editor.GetUrl()), "releasing text must preserve the visited-file marker")
	got, ok := prog.GetEditor(editor.GetUrl())
	require.True(t, ok, "a later unit must still resolve the source URL")
	require.Equal(t, editor.GetSourceCode(), got.GetSourceCode())
	got, ok = prog.GetEditorByHash(editor.GetIrSourceHash())
	require.True(t, ok, "instruction reload must still resolve its source hash")
	require.Equal(t, editor.GetSourceCode(), got.GetSourceCode())
	require.Zero(t, prog.Cache.sources.EditorCount(), "readback must not rebuild an unbounded source-store cache")
}

func TestSourceStoreReleaseKeepsEditorLookups(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, filesys.NewVirtualFs())
	editor := prog.CreateEditor([]byte("def completed(): return 42"), "/src/completed.py")
	prog.SetEditor(editor.GetUrl(), editor)

	require.NoError(t, prog.Cache.sources.Flush())
	require.Equal(t, 1, prog.Cache.sources.ReleasePersistedEditors())
	require.True(t, prog.ShouldVisit(editor.GetUrl()))
	got, ok := prog.GetEditor(editor.GetUrl())
	require.True(t, ok, "visited URLs need a DB-backed lookup after their editor is released")
	require.NotNil(t, got)
	require.Equal(t, editor.GetSourceCode(), got.GetSourceCode())
}

func TestSourceFlushAsyncDoesNotBlockAndCloseJoins(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, filesys.NewVirtualFs())
	source := prog.Cache.sources
	prog.CreateEditor([]byte("class AsyncUnit {}"), "/src/AsyncUnit.java")

	locked := make(chan struct{})
	release := make(chan struct{})
	lockDone := make(chan error, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	go func() {
		lockDone <- withCacheWriteLock(ssadb.GetDB().Dialect().GetName(), func() error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	returned := make(chan struct{})
	go func() {
		prog.Cache.FlushAuxSavers()
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("source I/O must not block batch cleanup")
	}
	require.Equal(t, 1, source.PayloadCount(), "blocked DB write must retain the source payload")
	require.Equal(t, 1, source.EditorCount())

	closed := make(chan error, 1)
	go func() { closed <- source.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before the outstanding write completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	require.NoError(t, <-lockDone)
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not join the source saver")
	}
	require.Equal(t, 1, source.PersistedCount())
	require.Zero(t, source.PayloadCount())
	require.Zero(t, source.EditorCount())
}

func TestSourceFlushAsyncCoalescesRequests(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, filesys.NewVirtualFs())
	source := prog.Cache.sources
	defer source.Close()
	prog.CreateEditor([]byte("class FirstUnit {}"), "/src/FirstUnit.java")

	locked := make(chan struct{})
	release := make(chan struct{})
	lockDone := make(chan error, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	go func() {
		lockDone <- withCacheWriteLock(ssadb.GetDB().Dialect().GetName(), func() error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	source.FlushAsync()
	source.asyncMu.Lock()
	firstDone := source.asyncDone
	source.asyncMu.Unlock()
	prog.CreateEditor([]byte("class SecondUnit {}"), "/src/SecondUnit.java")
	source.FlushAsync()
	source.FlushAsync()
	source.asyncMu.Lock()
	sameSaver := source.asyncDone == firstDone
	source.asyncMu.Unlock()
	require.True(t, sameSaver, "overlapping requests must share the existing completion barrier")
	unblock()
	require.NoError(t, <-lockDone)
	require.Eventually(t, func() bool {
		return source.PersistedCount() == 2 && source.EditorCount() == 0
	}, 5*time.Second, 10*time.Millisecond)
	var count int
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrSource{}).Where("program_name = ?", programName).Count(&count).Error)
	require.Equal(t, 2, count, "overlapping flushes must neither lose nor duplicate a source row")
}

func TestSourceReleaseKeepsNewerEditorAtSameURL(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, filesys.NewVirtualFs())
	defer prog.Cache.sources.Close()
	old := prog.CreateEditor([]byte("class Old {}"), "/src/Changing.java")
	prog.SetEditor(old.GetUrl(), old)
	require.NoError(t, prog.Cache.sources.Flush())
	newer := prog.CreateEditor([]byte("class New {}"), "/src/Changing.java")
	prog.SetEditor(newer.GetUrl(), newer)

	require.Equal(t, 1, prog.Cache.sources.ReleasePersistedEditors())
	got, ok := prog.GetEditor(newer.GetUrl())
	require.True(t, ok)
	require.Same(t, newer, got, "completion of an older write must not discard a newer editor at the same URL")
}

func TestSourceCloseFailureRetainsEditorsForRetry(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, filesys.NewVirtualFs())
	source := prog.Cache.sources
	prog.CreateEditor([]byte("class RetryUnit {}"), "/src/RetryUnit.java")
	db := source.db
	source.db = db.Where("missing_source_column = ?", 1)
	require.Error(t, source.Close())
	require.Equal(t, 1, source.PayloadCount())
	require.Equal(t, 1, source.EditorCount(), "failed persistence must not discard the only source copy")
	source.db = db
	require.NoError(t, source.Close())
	require.Zero(t, source.PayloadCount())
	require.Zero(t, source.EditorCount())
}

func TestSourceReleaseKeepsAliasLookupWithoutRetainingEditor(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, filesys.NewVirtualFs())
	defer prog.Cache.sources.Close()
	editor := prog.CreateEditor([]byte("class Aliased {}"), "/src/Aliased.java")
	const alias = "alias://Aliased.java"
	prog.SetEditor(alias, editor)
	require.NoError(t, prog.Cache.sources.Flush())
	require.Equal(t, 1, prog.Cache.sources.ReleasePersistedEditors())
	require.Empty(t, prog.Cache.sources.editorsByURL, "all URL aliases must release their strong editor references")
	require.Nil(t, prog.Cache.sources.visitedURLs[alias])
	require.True(t, prog.ShouldVisit(alias))
	got, ok := prog.GetEditor(alias)
	require.True(t, ok)
	require.Equal(t, editor.GetSourceCode(), got.GetSourceCode())
}

func TestSourceStoreKeepsVisitedEditorWithoutHash(t *testing.T) {
	prog := newProgramWithTTL("memory-editor", 0, ProgramCacheMemory, filesys.NewVirtualFs())
	editor := memedit.NewMemEditor("")
	require.Empty(t, editor.GetIrSourceHash(), "an empty editor without file/program metadata has no hash")
	prog.SetEditor("in-memory.yak", editor)
	require.True(t, prog.ShouldVisit("in-memory.yak"))
	got, ok := prog.GetEditor("in-memory.yak")
	require.True(t, ok)
	require.Same(t, editor, got, "memory-mode URL lookups must not require a persistable hash")
}
