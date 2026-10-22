package sfpattern

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// countingFS records ReadFile calls so a test can prove the scan does not open
// files except through the FileSystem interface, and that the in-flight cap holds.
type countingFS struct {
	fi.FileSystem
	mu      sync.Mutex
	reads   int
	current int
	max     int
	entered chan struct{}
	release chan struct{}
}

func (c *countingFS) ReadFile(name string) ([]byte, error) {
	c.mu.Lock()
	c.reads++
	c.current++
	if c.current > c.max {
		c.max = c.current
	}
	entered, release := c.entered, c.release
	c.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if release != nil {
		<-release
	}
	defer func() {
		c.mu.Lock()
		c.current--
		c.mu.Unlock()
	}()
	return c.FileSystem.ReadFile(name)
}

func (c *countingFS) snapshot() (reads, max int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads, c.max
}

func TestStreamingSource_InFlightCapAndReadFileCount(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	for i := 0; i < 6; i++ {
		vfs.AddFile(filepath.ToSlash(filepath.Join("dir", "f"+string(rune('a'+i))+".txt")), "alpha\n")
	}
	entered := make(chan struct{}, 6)
	release := make(chan struct{})
	counted := &countingFS{FileSystem: vfs, entered: entered, release: release}
	store, err := OpenStreamingSource(counted, nil, 2)
	require.NoError(t, err)
	require.Len(t, store.Paths(), 6)
	require.Zero(t, store.ReadCount(), "listing a source must not read file bodies")

	frames := sourceFrames(t, `${*.txt}.pattern_regex(/alpha/) as $hit
alert $hit`)
	plan, err := NewSourceScanPlan(frames)
	require.NoError(t, err)
	defer plan.Close()
	previous := goruntime.GOMAXPROCS(4)
	defer goruntime.GOMAXPROCS(previous)
	runtime := plan.StartStore(context.Background(), store)

	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for the in-flight reads")
		}
	}
	select {
	case <-entered:
		t.Fatal("a third file was read while the in-flight cap was full")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	require.NoError(t, runtime.Wait(context.Background()))
	runtime.Close()

	reads, max := counted.snapshot()
	require.Equal(t, 6, reads, "every eligible file is read once, through FileSystem.ReadFile")
	require.LessOrEqual(t, max, 2)
	require.Equal(t, int64(6), store.ReadCount())
	require.LessOrEqual(t, store.MaxInFlight(), 2)
}

func TestStreamingSource_VirtualZipAndDirMatch(t *testing.T) {
	const (
		note = "alpha secret\n"
		skip = "alpha\n"
	)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("a/note.txt", note)
	vfs.AddFile("b/skip.go", skip)

	var raw bytes.Buffer
	zw := zip.NewWriter(&raw)
	for name, body := range map[string]string{"a/note.txt": note, "b/skip.go": skip} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	zfs, err := filesys.NewZipFSFromString(raw.String())
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "note.txt"), []byte(note), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b", "skip.go"), []byte(skip), 0o644))

	virtual := alertsFor(t, vfs)
	zipped := alertsFor(t, zfs)
	local := alertsFor(t, filesys.NewRelLocalFs(dir))
	require.Equal(t, virtual, zipped)
	require.Equal(t, virtual, local)
	require.NotEmpty(t, virtual)
	require.Contains(t, virtual[0], "a/note.txt")
	for _, row := range virtual {
		require.NotContains(t, row, "skip.go")
	}
}

func alertsFor(t *testing.T, fsys fi.FileSystem) []string {
	t.Helper()
	opt := DefaultLoadOptions()
	store, err := OpenStreamingSource(fsys, &opt, 2)
	require.NoError(t, err)
	frames := sourceFrames(t, `${*.txt}.pattern_regex(/alpha/) as $hit
alert $hit`)
	plan, err := NewSourceScanPlan(frames)
	require.NoError(t, err)
	defer plan.Close()
	runtime := plan.StartStore(context.Background(), store)
	defer runtime.Close()
	root := NewRootFromStore(store)
	root.SetFileFilterMatcher(runtime.Matcher(context.Background()))
	result, err := frames[0].Feed(sfvm.ValuesOf(root))
	require.NoError(t, err)
	return sourceAlertSnapshot(result)
}
