package ssareducer

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundedOrderedFilesSlowHeadAndRelease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan struct{})
	started := make(chan string, 4)
	out := boundedOrderedFiles(ctx, []string{"a", "b", "c", "d"}, 4, 2, nil,
		func(path string, _ *utils.SafeMap[any]) *FileContent {
			started <- path
			if path == "a" {
				<-first
			}
			return &FileContent{Path: path}
		})
	got := []string{<-started, <-started}
	require.ElementsMatch(t, []string{"a", "b"}, got)
	select {
	case <-out:
		t.Fatal("a slow head was overtaken")
	default:
	}
	close(first)
	head := <-out
	require.Equal(t, "a", head.Path)
	select {
	case path := <-started:
		t.Fatalf("window exceeded before release: %s", path)
	default:
	}
	head.Release()
	head.Release() // idempotent; must not consume a second slot
	var paths []string
	for file := range out {
		paths = append(paths, file.Path)
		file.Release()
	}
	require.Equal(t, []string{"b", "c", "d"}, paths)
}

func TestBoundedOrderedFilesCancellationDrainsPending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	gate := make(chan struct{})
	started := make(chan struct{}, 2)
	var parsed atomic.Int32
	out := boundedOrderedFiles(ctx, []string{"a", "b", "c", "d"}, 2, 2, nil,
		func(path string, _ *utils.SafeMap[any]) *FileContent {
			parsed.Add(1)
			started <- struct{}{}
			<-gate
			return &FileContent{Path: path}
		})
	<-started
	<-started
	cancel()
	close(gate)
	select {
	case _, ok := <-out:
		require.False(t, ok)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled ordered parser did not drain")
	}
	require.EqualValues(t, 2, parsed.Load())
}

func TestFilesHandlerBoundedOrderKeepsErrorsAndIgnoresFullBufferLimit(t *testing.T) {
	t.Setenv("YAK_SSA_ORDERED_AST_MAX_FILES", "1")
	fs := filesys.NewVirtualFs()
	fs.AddFile("a", "a")
	fs.AddFile("c", "c")
	out := FilesHandler(context.Background(), fs, []string{"a", "missing", "c"},
		func(_ string, _ []byte, _ *utils.SafeMap[any]) (ssa.FrontAST, error) { return nil, nil },
		nil, BoundedOrder, 2, 2)
	var paths []string
	for file := range out {
		paths = append(paths, file.Path)
		if file.Path == "missing" {
			require.Equal(t, FileStatusFsError, file.Status)
		}
		file.Release()
	}
	require.Equal(t, []string{"a", "missing", "c"}, paths)
}
