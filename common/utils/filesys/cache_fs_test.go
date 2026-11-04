package filesys

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// countingSourceFS is a minimal read-only filesystem that records how many
// times ReadFile is called per path and can simulate a slow producer.
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

// TestFileCacheLoadsOncePerPath is the core contract: content is produced
// once even when several compile stages (and goroutines) ask for it.
func TestFileCacheLoadsOncePerPath(t *testing.T) {
	base := seedSourceFS(t, "a/B.java")
	s := NewFileCache()

	const callers = 32
	var wg sync.WaitGroup
	results := make([][]byte, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = s.ReadFile(base, "a/B.java")
		}(i)
	}
	wg.Wait()

	for i := 0; i < callers; i++ {
		require.NoError(t, errs[i])
		require.Equal(t, "source-of:a/B.java", string(results[i]))
	}
	require.Equal(t, 1, base.readCount("a/B.java"), "concurrent readers must share one production")
}

// TestFileCachePrefetchRunsInParallel proves the prefetch pass uses the worker
// pool instead of producing serially.
func TestFileCachePrefetchRunsInParallel(t *testing.T) {
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
	s := NewFileCache()

	var progress int
	var progressMu sync.Mutex
	start := time.Now()
	require.NoError(t, s.Prefetch(context.Background(), base, list, workers, func(done, total int) {
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

	for _, p := range list {
		data, err := s.ReadFile(base, p)
		require.NoError(t, err)
		require.Equal(t, "source-of:"+p, string(data))
	}
	require.Equal(t, paths, base.totalReads(), "cached reads must not produce again")
}

// TestPrefetchThroughWrappedFSDoesNotDeadlock is the compile path: prefetch is
// handed the already-wrapped filesystem, whose ReadFile calls back into the
// cache. That re-entry used to wait on its own in-flight load.
func TestPrefetchThroughWrappedFSDoesNotDeadlock(t *testing.T) {
	base := seedSourceFS(t, "a/B.java", "a/C.java")
	s := NewFileCache()
	wrapped := WrapWithFileCache(base, s)

	done := make(chan error, 1)
	go func() {
		done <- s.Prefetch(context.Background(), wrapped, []string{"a/B.java", "a/C.java"}, 2, nil)
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

// TestFileCacheServesContentAndKeepsTreeShape checks the wrapper keeps the tree
// shape of the wrapped filesystem while replacing ReadFile content.
func TestFileCacheServesContentAndKeepsTreeShape(t *testing.T) {
	base := seedSourceFS(t, "WEB-INF/classes/a/B.java", "WEB-INF/web.xml")
	s := NewFileCache()
	wrapped := WrapWithFileCache(base, s)

	require.Nil(t, FileCacheOf(base))
	require.Same(t, s, FileCacheOf(wrapped))

	info, err := wrapped.Stat("WEB-INF/classes/a/B.java")
	require.NoError(t, err)
	require.False(t, info.IsDir())
	entries, err := wrapped.ReadDir("WEB-INF")
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	require.Equal(t, '/', wrapped.GetSeparators())

	data, err := wrapped.ReadFile("WEB-INF/classes/a/B.java")
	require.NoError(t, err)
	require.Equal(t, "source-of:WEB-INF/classes/a/B.java", string(data))
	_, err = wrapped.ReadFile("WEB-INF/classes/a/B.java")
	require.NoError(t, err)
	require.Equal(t, 1, base.readCount("WEB-INF/classes/a/B.java"))

	xml, err := wrapped.ReadFile("WEB-INF/web.xml")
	require.NoError(t, err)
	require.Equal(t, "source-of:WEB-INF/web.xml", string(xml))
}

// TestFileCachePutReplacesCachedContent is the replacement case: the base
// content is read first, which caches it, and transformed content is then Put
// under the same name. Later reads must return the replacement.
func TestFileCachePutReplacesCachedContent(t *testing.T) {
	base := seedSourceFS(t, "webapp/page.txt")
	s := NewFileCache()

	first, err := s.ReadFile(base, "webapp/page.txt")
	require.NoError(t, err)
	require.Equal(t, "source-of:webapp/page.txt", string(first))

	generated := "transformed body"
	s.Put("webapp/page.txt", []byte(generated))

	got, err := s.ReadFile(base, "webapp/page.txt")
	require.NoError(t, err)
	require.Equal(t, generated, string(got))
	require.Equal(t, 1, base.readCount("webapp/page.txt"))

	stats := s.Stats()
	require.EqualValues(t, 1, stats.Paths)
	require.EqualValues(t, len(generated), stats.MemoryHold)
	require.EqualValues(t, 1, stats.Misses)
	require.GreaterOrEqual(t, stats.Hits, int64(1))
}

// TestFileCacheOverflowsMemoryLimit is the budget contract: once retained
// bytes would exceed the limit, the entry goes to the private overflow directory
// and remains readable; Close removes that directory.
func TestFileCacheOverflowsMemoryLimit(t *testing.T) {
	base := seedSourceFS(t, "a/A.java", "a/B.java", "a/C.java")
	s := NewFileCache(WithFileCacheMemoryLimit(16))
	defer s.Close()

	first, err := s.ReadFile(base, "a/A.java")
	require.NoError(t, err)
	require.Equal(t, "source-of:a/A.java", string(first))

	second, err := s.ReadFile(base, "a/B.java")
	require.NoError(t, err)
	require.Equal(t, "source-of:a/B.java", string(second))

	stats := s.Stats()
	require.EqualValues(t, 2, stats.Paths)
	require.Greater(t, stats.DiskHold, int64(0), "over-limit entry must be written to disk")
	require.GreaterOrEqual(t, stats.DiskWrites, int64(1))

	// Both entries remain readable: one from memory, one from disk.
	for _, p := range []string{"a/A.java", "a/B.java"} {
		got, err := s.ReadFile(base, p)
		require.NoError(t, err)
		require.Equal(t, "source-of:"+p, string(got))
	}

	// Close removes the private directory and drops every entry.
	require.NoError(t, s.Close())
	require.NoError(t, s.Close())
	after := s.Stats()
	require.Zero(t, after.Paths)
	require.Zero(t, after.DiskHold)
}

// TestFileCacheWalkEntries lists cached paths without exposing internals.
func TestFileCacheWalkEntries(t *testing.T) {
	base := seedSourceFS(t, "a/A.java", "a/B.java")
	s := NewFileCache()
	_, err := s.ReadFile(base, "a/A.java")
	require.NoError(t, err)
	s.Put("a/B.java", []byte("generated"))

	var got []string
	require.NoError(t, s.WalkEntries(func(name string, size int64) error {
		got = append(got, name)
		require.Greater(t, size, int64(0))
		return nil
	}))
	require.ElementsMatch(t, []string{"a/A.java", "a/B.java"}, got)
}

// TestFileCacheLoaderAndPublisher observes one load and one replacement.
func TestFileCacheLoaderAndPublisher(t *testing.T) {
	base := seedSourceFS(t, "a/A.class")
	var loads, publishes int
	var mu sync.Mutex
	s := NewFileCache(
		WithFileCacheLoader(func(base fi.FileSystem, name string) ([]byte, error) {
			mu.Lock()
			loads++
			mu.Unlock()
			return []byte("compiled:" + name), nil
		}),
		WithFileCachePublisher(func(name string, data []byte) {
			mu.Lock()
			publishes++
			mu.Unlock()
		}),
	)
	got, err := s.ReadFile(base, "a/A.class")
	require.NoError(t, err)
	require.Equal(t, "compiled:a/A.class", string(got))
	s.Put("a/A.class", []byte("java"))
	mu.Lock()
	require.Equal(t, 1, loads)
	require.Equal(t, 2, publishes)
	mu.Unlock()
}

// TestFileCachePrefetchHonorsContext makes sure a cancelled scan stops producing.
func TestFileCachePrefetchHonorsContext(t *testing.T) {
	list := []string{"a/1.java", "a/2.java", "a/3.java", "a/4.java"}
	base := seedSourceFS(t, list...)
	base.delay = 40 * time.Millisecond
	s := NewFileCache()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.Prefetch(ctx, base, list, 2, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, base.totalReads(), len(list))
}

// TestReleasePreparationCachesPreservesCachedContent keeps producer scratch
// release separate from cached sources.
func TestReleasePreparationCachesPreservesCachedContent(t *testing.T) {
	base := &releasableSourceFS{countingSourceFS: seedSourceFS(t, "a/B.java")}
	s := NewFileCache()
	fsys := WrapWithFileCache(NewUnifiedFS(base), s)
	first, err := fsys.ReadFile("a/B.java")
	require.NoError(t, err)
	ReleasePreparationCaches(fsys)
	require.Equal(t, 1, base.released)
	again, err := fsys.ReadFile("a/B.java")
	require.NoError(t, err)
	require.Equal(t, first, again)
	require.Equal(t, 1, base.readCount("a/B.java"), "released scratch must not force sources to be produced twice")
}
