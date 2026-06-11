package ssareducer

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

//go:embed testlib/***
var lib embed.FS

func TestReducerCompiling_NORMAL(t *testing.T) {
	count := 0
	var existed []string
	err := filesys.Recursive(
		"testlib",
		filesys.WithEmbedFS(lib),
		filesys.WithFileStat(func(pathname string, fi fs.FileInfo) error {
			count++
			if strings.HasSuffix(pathname, ".yak") {
				existed = append(existed, pathname)
			}
			return nil
		}),
	)
	if err != nil {
		panic(err)
	}
	if count != 5 {
		t.Error("count should be 5")
	}

	count = 0
	err = ReducerCompile(
		"testlib",
		WithEmbedFS(lib),
		WithCompileMethod(func(s string, r string) ([]string, error) {
			if !strings.HasSuffix(s, ".yak") {
				return []string{s}, nil
			}
			count++

			var visited []string
			visited = append(visited, s)

			checked := 0
			for _, v := range existed {
				if v == s {
					continue
				}
				checked++
				visited = append(visited, v)
				if checked == 2 {
					break
				}
			}
			log.Infof("start to Compile %s", s)
			spew.Dump(visited)
			return visited, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("count should be 2. got " + fmt.Sprint(count))
	}
}

func TestReducerCompiling2_CompileFailed(t *testing.T) {
	count := 0
	err := filesys.Recursive("testlib",
		filesys.WithEmbedFS(lib),
		filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
			count++
			return nil
		}))
	if err != nil {
		panic(err)
	}
	if count != 5 {
		t.Error("count should be 5")
	}

	count = 0
	err = ReducerCompile("testlib",
		WithEmbedFS(lib),
		WithCompileMethod(func(s string, r string) ([]string, error) {
			count++
			log.Infof("start to Compile %s", s)
			return []string{"testlib/aa/a3.yak", "testlib/dd/a1.yak", "testlib/dd/a2.yak"}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatal("count should be 2")
	}
}

func TestReducerCompiling2_NOLIMIT(t *testing.T) {
	count := 0
	err := filesys.Recursive("testlib",
		filesys.WithEmbedFS(lib),
		filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
			count++
			return nil
		}),
	)
	if err != nil {
		panic(err)
	}
	if count != 5 {
		t.Error("count should be 5")
	}

	count = 0
	err = ReducerCompile("testlib",
		WithEmbedFS(lib),
		WithCompileMethod(func(s string, r string) ([]string, error) {
			count++
			log.Infof("start to Compile %s", s)
			return []string{}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatal("count should be 5")
	}
}

func TestReducerCompiling2_VirtualFile(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("a/b/c.txt", "c")
	vfs.AddFile("a/b.txt", "b")
	vfs.AddFile("a/c/b.txt", "b")

	var count = 0

	count = 0
	err := ReducerCompile(
		"a",
		WithFileSystem(vfs),
		WithCompileMethod(func(s string, r string) ([]string, error) {
			log.Infof("start to Compile %s", s)
			count++
			return []string{}, nil
		}),
	)
	require.NoError(t, err, "compile failed")
	require.Equal(t, 3, count, "count should be 3")

	count = 0
	err = ReducerCompile(
		"a",
		WithFileSystem(vfs),
		WithCompileMethod(func(s string, r string) ([]string, error) {
			log.Infof("start to Compile %s", s)
			count++
			return []string{"a/b.txt"}, nil
		}),
	)
	require.NoError(t, err, "compile failed")
	require.Equal(t, 2, count, "count should be 2")
}

func TestPipeInitBufSize_capsHugeProjects(t *testing.T) {
	if g := pipeInitBufSize(100_000, 8); g != 16 {
		t.Fatalf("100k paths conc=8: want 16, got %d", g)
	}
	if g := pipeInitBufSize(100_000, 64); g != 128 {
		t.Fatalf("100k paths conc=64: want 128, got %d", g)
	}
	if g := pipeInitBufSize(100_000, 10_000); g != maxSourceQueueSize {
		t.Fatalf("100k paths conc=10000: want hard cap %d, got %d", maxSourceQueueSize, g)
	}
}

func TestPipeInitBufSize_smallProjectUnchanged(t *testing.T) {
	if g := pipeInitBufSize(5, 8); g != 5 {
		t.Fatalf("5 paths: want 5, got %d", g)
	}
	if g := pipeInitBufSize(1, 0); g != 1 {
		t.Fatalf("1 path conc=0: want 1, got %d", g)
	}
}

func TestPipeInitBufSize_envOverride(t *testing.T) {
	t.Setenv("YAK_SSA_AST_IN_FLIGHT_FILES", "3")
	require.Equal(t, 3, pipeInitBufSize(100, 8))

	t.Setenv("YAK_SSA_AST_IN_FLIGHT_FILES", "0")
	require.Equal(t, 1, pipeInitBufSize(100, 8))
}

func TestEffectiveASTSequence_downgradesLargeOrderedMode(t *testing.T) {
	t.Setenv("YAK_SSA_ORDERED_AST_MAX_FILES", "2")
	require.Equal(t, Order, effectiveASTSequence(Order, 2))
	require.Equal(t, OutOfOrder, effectiveASTSequence(Order, 3))
	require.Equal(t, ReverseOrder, effectiveASTSequence(ReverseOrder, 2))
	require.Equal(t, OutOfOrder, effectiveASTSequence(ReverseOrder, 3))
}

func TestFilesHandler_OutOfOrderBackpressuresParsedAST(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	var paths []string
	for i := 0; i < 20; i++ {
		path := fmt.Sprintf("src/%02d.go", i)
		paths = append(paths, path)
		vfs.AddFile(path, "package main")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var parsed int64
	out := FilesHandler(
		ctx,
		vfs,
		paths,
		func(path string, content []byte, store *utils.SafeMap[any]) (ssa.FrontAST, error) {
			atomic.AddInt64(&parsed, 1)
			return path, nil
		},
		nil,
		OutOfOrder,
		2,
	)

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&parsed) == 2
	}, time.Second, time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int64(2), atomic.LoadInt64(&parsed), "parsed AST should not queue beyond active workers without a consumer")

	cancel()
	for range out {
	}
}

func TestFilesHandler_OutOfOrderWaitsForReleaseBeforeParsingNextAST(t *testing.T) {
	t.Setenv("YAK_SSA_AST_BUILD_WINDOW_FILES", "1")

	vfs := filesys.NewVirtualFs()
	var paths []string
	for i := 0; i < 4; i++ {
		path := fmt.Sprintf("src/%02d.go", i)
		paths = append(paths, path)
		vfs.AddFile(path, "package main")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var parsed int64
	out := FilesHandler(
		ctx,
		vfs,
		paths,
		func(path string, content []byte, store *utils.SafeMap[any]) (ssa.FrontAST, error) {
			atomic.AddInt64(&parsed, 1)
			return path, nil
		},
		nil,
		OutOfOrder,
		4,
	)

	first := <-out
	require.NotNil(t, first)
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&parsed) == 1
	}, time.Second, time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int64(1), atomic.LoadInt64(&parsed), "parser should wait for the consumer to release the AST slot")

	first.Release()
	second := <-out
	require.NotNil(t, second)
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&parsed) == 2
	}, time.Second, time.Millisecond)

	second.Release()
	for fc := range out {
		fc.Release()
	}
	require.Equal(t, int64(4), atomic.LoadInt64(&parsed))
}
