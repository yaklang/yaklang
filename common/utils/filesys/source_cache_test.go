package filesys

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// countingSourceFS is a minimal read-only filesystem that records how many
// times ReadFile is called per path and can simulate slow decompilation.
type countingSourceFS struct {
	fi.FileSystem
	delay time.Duration

	mu    sync.Mutex
	reads map[string]int
}

func newCountingSourceFS(delay time.Duration) *countingSourceFS {
	return &countingSourceFS{FileSystem: NewVirtualFs(), delay: delay, reads: map[string]int{}}
}

func (f *countingSourceFS) ReadFile(name string) ([]byte, error) {
	f.mu.Lock()
	f.reads[name]++
	f.mu.Unlock()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.FileSystem.ReadFile(name)
}

func (f *countingSourceFS) readCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[name]
}

func (f *countingSourceFS) totalReads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, n := range f.reads {
		total += n
	}
	return total
}

func seedSourceFS(t *testing.T, paths ...string) *countingSourceFS {
	t.Helper()
	fsys := newCountingSourceFS(0)
	for _, p := range paths {
		fsys.FileSystem.(*VirtualFS).AddFile(p, "source-of:"+p)
	}
	return fsys
}

type releasableSourceFS struct {
	*countingSourceFS
	released int
}

func (f *releasableSourceFS) ReleasePreparationCaches() { f.released++ }

func TestReleasePreparedSourceCachesPreservesVirtualFS(t *testing.T) {
	base := &releasableSourceFS{countingSourceFS: seedSourceFS(t, "a/B.java")}
	materializer := NewSourceMaterializer()
	fsys := WithMaterializedSources(NewUnifiedFS(base), materializer)
	first, err := fsys.ReadFile("a/B.java")
	require.NoError(t, err)
	ReleasePreparedSourceCaches(fsys)
	require.Equal(t, 1, base.released)
	again, err := fsys.ReadFile("a/B.java")
	require.NoError(t, err)
	require.Equal(t, first, again)
	require.Equal(t, 1, base.readCount("a/B.java"), "released scratch must not force materialized sources to be produced twice")
}

// TestSourceMaterializerDecompilesOncePerPath is the core contract: a class is
// read once even when several compile stages (and goroutines) ask for it.
func TestSourceMaterializerDecompilesOncePerPath(t *testing.T) {
	base := seedSourceFS(t, "a/B.java")
	m := NewSourceMaterializer()

	const callers = 32
	var wg sync.WaitGroup
	results := make([][]byte, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = m.ReadFile(base, "a/B.java")
		}(i)
	}
	wg.Wait()

	for i := 0; i < callers; i++ {
		require.NoError(t, errs[i])
		require.Equal(t, "source-of:a/B.java", string(results[i]))
	}
	require.Equal(t, 1, base.readCount("a/B.java"), "concurrent readers must share one decompile")
}

// TestSourceMaterializerPrefetchRunsInParallel proves the prefetch pass uses
// the worker pool instead of decompiling serially.
func TestSourceMaterializerPrefetchRunsInParallel(t *testing.T) {
	const (
		paths   = 8
		delay   = 60 * time.Millisecond
		workers = 8
	)
	list := make([]string, 0, paths)
	for i := 0; i < paths; i++ {
		list = append(list, fmt.Sprintf("pkg/C%d.java", i))
	}
	base := seedSourceFS(t, list...)
	base.delay = delay
	m := NewSourceMaterializer()

	var progress int
	var progressMu sync.Mutex
	start := time.Now()
	require.NoError(t, m.Prefetch(context.Background(), base, list, workers, func(done, total int) {
		progressMu.Lock()
		progress = done
		progressMu.Unlock()
	}))
	elapsed := time.Since(start)

	require.Equal(t, paths, progress, "progress must be reported for every path")
	serial := time.Duration(paths) * delay
	require.Less(t, elapsed, serial/2,
		"prefetch must overlap work: elapsed=%v serial=%v", elapsed, serial)
	require.Equal(t, paths, base.totalReads())

	// Every path is now served from the in-memory tree without touching base.
	for _, p := range list {
		data, err := m.ReadFile(base, p)
		require.NoError(t, err)
		require.Equal(t, "source-of:"+p, string(data))
	}
	require.Equal(t, paths, base.totalReads(), "materialized reads must not decompile again")
}

// TestPrefetchThroughWrappedFSDoesNotDeadlock is the compile path: prefetch is
// handed the already-wrapped filesystem, whose ReadFile calls back into the
// materializer. That re-entry used to wait on its own in-flight load.
func TestPrefetchThroughWrappedFSDoesNotDeadlock(t *testing.T) {
	base := seedSourceFS(t, "a/B.java", "a/C.java")
	m := NewSourceMaterializer()
	wrapped := WithMaterializedSources(base, m)

	done := make(chan error, 1)
	go func() {
		done <- m.Prefetch(context.Background(), wrapped, []string{"a/B.java", "a/C.java"}, 2, nil)
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("prefetch through the wrapped filesystem deadlocked")
	}

	data, err := wrapped.ReadFile("a/B.java")
	require.NoError(t, err)
	require.Equal(t, "source-of:a/B.java", string(data))
	require.Equal(t, 1, base.readCount("a/B.java"))
	require.Equal(t, 1, base.readCount("a/C.java"))
}

// TestMaterializedSourceFSServesFromMemory checks the wrapper keeps the tree
// shape of the wrapped filesystem while replacing ReadFile content.
func TestMaterializedSourceFSServesFromMemory(t *testing.T) {
	base := seedSourceFS(t, "WEB-INF/classes/a/B.java", "WEB-INF/web.xml")
	m := NewSourceMaterializer()
	wrapped := WithMaterializedSources(base, m)

	require.Nil(t, MaterializerOf(base))
	require.NotNil(t, MaterializerOf(wrapped))

	// Stat/ReadDir/Exists keep delegating to the wrapped filesystem.
	info, err := wrapped.Stat("WEB-INF/classes/a/B.java")
	require.NoError(t, err)
	require.False(t, info.IsDir())
	entries, err := wrapped.ReadDir("WEB-INF")
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	require.Equal(t, '/', wrapped.GetSeparators())

	// First read decompiles, second read is memory-only.
	data, err := wrapped.ReadFile("WEB-INF/classes/a/B.java")
	require.NoError(t, err)
	require.Equal(t, "source-of:WEB-INF/classes/a/B.java", string(data))
	_, err = wrapped.ReadFile("WEB-INF/classes/a/B.java")
	require.NoError(t, err)
	require.Equal(t, 1, base.readCount("WEB-INF/classes/a/B.java"))

	// Non-materialized files still resolve through the wrapped filesystem.
	xml, err := wrapped.ReadFile("WEB-INF/web.xml")
	require.NoError(t, err)
	require.Equal(t, "source-of:WEB-INF/web.xml", string(xml))
}

// TestSourceMaterializerKeepsSourcesInVirtualFS documents that the materialized
// tree is a real filesystem holding the decompiled sources.
func TestSourceMaterializerKeepsSourcesInVirtualFS(t *testing.T) {
	base := seedSourceFS(t, "a/B.java")
	m := NewSourceMaterializer()
	_, err := m.ReadFile(base, "a/B.java")
	require.NoError(t, err)

	virtual := m.VirtualFS()
	require.NotNil(t, virtual)
	require.True(t, m.Has("a/B.java"))
	require.False(t, m.Has("a/missing.java"))

	data, err := virtual.ReadFile("a/B.java")
	require.NoError(t, err)
	require.Equal(t, "source-of:a/B.java", string(data))

	// A second read through the materializer is a memory hit, not a new decompile.
	_, err = m.ReadFile(base, "a/B.java")
	require.NoError(t, err)
	require.Equal(t, 1, base.readCount("a/B.java"))

	stats := m.Stats()
	require.EqualValues(t, 1, stats.Paths)
	require.EqualValues(t, len("source-of:a/B.java"), stats.Loaded)
	require.EqualValues(t, 1, stats.Misses)
	require.GreaterOrEqual(t, stats.Hits, int64(1))
}

func TestSourceMaterializerTreeWritesJavaWithoutRetainingBytes(t *testing.T) {
	dir := t.TempDir()
	base := seedSourceFS(t, "pkg/A.class", "web/index.jsp")
	m := NewSourceMaterializer(WithSourceMaterializerTree(dir))
	_, err := m.ReadFile(base, "pkg/A.class")
	require.NoError(t, err)
	m.Put("web/index.jsp", []byte("class Index {}"))
	again, err := m.ReadFile(base, "web/index.jsp")
	require.NoError(t, err)
	require.Equal(t, "source-of:web/index.jsp", string(again), "a later prepare must read the page, not the generated Java")

	classBytes, err := os.ReadFile(filepath.Join(dir, "pkg", "A.java"))
	require.NoError(t, err)
	require.Equal(t, "source-of:pkg/A.class", string(classBytes))
	jspBytes, err := os.ReadFile(filepath.Join(dir, "web", "index.jsp.java"))
	require.NoError(t, err)
	require.Equal(t, "class Index {}", string(jspBytes))

	require.False(t, m.Has("pkg/A.class"), "tree mode must not keep source bytes in memory")
	got, err := m.ReadFile(base, "pkg/A.class")
	require.NoError(t, err)
	require.Equal(t, "source-of:pkg/A.class", string(got))
	require.Equal(t, 1, base.readCount("pkg/A.class"))

	rel, err := materializeTreeRel("../../etc/passwd.java")
	require.NoError(t, err)
	require.NotContains(t, rel, "..")
	full, err := m.treeFile("../../etc/passwd.java")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(full, filepath.Clean(dir)+string(os.PathSeparator)) || full == filepath.Clean(dir))
}

// TestSourceMaterializerPrefetchHonorsContext makes sure a cancelled scan does
// not keep decompiling.
func TestSourceMaterializerPrefetchHonorsContext(t *testing.T) {
	list := []string{"a/1.java", "a/2.java", "a/3.java", "a/4.java"}
	base := seedSourceFS(t, list...)
	base.delay = 40 * time.Millisecond
	m := NewSourceMaterializer()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := m.Prefetch(ctx, base, list, 2, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, base.totalReads(), len(list))
}

// TestSourceMaterializerPutReplacesCachedContent is the JSP case: the raw page
// is read first, which caches it, and the generated Java is then Put under the
// same path. Later reads must return the replacement, not the raw page.
func TestSourceMaterializerPutReplacesCachedContent(t *testing.T) {
	base := seedSourceFS(t, "webapp/index.jsp")
	m := NewSourceMaterializer()

	first, err := m.ReadFile(base, "webapp/index.jsp")
	require.NoError(t, err)
	require.Equal(t, "source-of:webapp/index.jsp", string(first))

	generated := "public class Index { void _JavaTemplateService() {} }"
	m.Put("webapp/index.jsp", []byte(generated))

	got, err := m.ReadFile(base, "webapp/index.jsp")
	require.NoError(t, err)
	require.Equal(t, generated, string(got))

	// The replacement must also be what the overlay exposes, and the counters
	// must not double-count the replaced entry.
	overlay, err := m.VirtualFS().ReadFile("webapp/index.jsp")
	require.NoError(t, err)
	require.Equal(t, generated, string(overlay))
	stats := m.Stats()
	require.EqualValues(t, 1, stats.Paths)
	require.EqualValues(t, len(generated), stats.Loaded)
	// The replacement is served from memory, so the base is not read again.
	require.Equal(t, 1, base.readCount("webapp/index.jsp"))
}
