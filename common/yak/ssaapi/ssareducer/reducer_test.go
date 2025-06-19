package ssareducer

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils/filesys"
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
