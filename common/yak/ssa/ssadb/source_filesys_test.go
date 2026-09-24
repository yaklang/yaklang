package ssadb_test

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSourceFilesys(t *testing.T) {
	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.apackage")
	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.bpackage.sub")

	codeA := `
		package com.example.apackage; 
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				B b = new B();
				target1(b.get());
				b.show(1);
			}
		}
		`
	codeB := `
		package com.example.bpackage.sub; 
		class B {
			public  int get() {
				return 	 1;
			}
			public void show(int a) {
				target2(a);
			}
		}
		`
	wantDir := []string{
		"example", "example/src", "example/src/main", "example/src/main/java",
		"example/src/main/java/com", "example/src/main/java/com/example",
		"example/src/main/java/com/example/apackage",
		"example/src/main/java/com/example/bpackage",
		"example/src/main/java/com/example/bpackage/sub",
	}
	wantFile := []string{
		"example/src/main/java/com/example/apackage/a.java",
		"example/src/main/java/com/example/bpackage/sub/b.java",
	}

	assertTree := func(t *testing.T, programID string, dbfs fi.FileSystem) {
		t.Helper()
		var dirs, files []string
		require.NoError(t, filesys.Recursive(
			fmt.Sprintf("/%s", programID),
			filesys.WithFileSystem(dbfs),
			filesys.WithDirStat(func(s string, info fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				if path != "" {
					dirs = append(dirs, path)
				}
				return nil
			}),
			filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				files = append(files, path)
				return nil
			}),
		))
		gotDir := append([]string(nil), wantDir...)
		gotFile := append([]string(nil), wantFile...)
		slices.Sort(gotDir)
		slices.Sort(dirs)
		slices.Sort(gotFile)
		slices.Sort(files)
		assert.Equal(t, gotDir, dirs)
		assert.Equal(t, gotFile, files)
	}

	t.Run("virtual_fs", func(t *testing.T) {
		programID := uuid.NewString()
		opts := []ssaconfig.Option{
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID),
		}
		_, err := ssaconfig.New(ssaconfig.ModeProjectCompile, opts...)
		require.NoError(t, err)

		vf := filesys.NewVirtualFs()
		vf.AddFile("example/src/main/java/com/example/apackage/a.java", codeA)
		vf.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", codeB)
		_, err = ssaapi.ParseProjectWithFS(vf, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), programID) })

		dbfs := ssadb.NewIrSourceFs()
		assertTree(t, programID, dbfs)

		entries, err := dbfs.ReadDir("/")
		require.NoError(t, err)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		assert.Contains(t, names, programID)

		root := "/" + programID
		first, err := dbfs.ReadDir(root)
		require.NoError(t, err)
		require.NotEmpty(t, first)
		for i := 0; i < 20; i++ {
			again, err := dbfs.ReadDir(root)
			require.NoError(t, err)
			require.Equal(t, len(first), len(again))
		}
		data, err := dbfs.ReadFile(root + "/example/src/main/java/com/example/apackage/a.java")
		require.NoError(t, err)
		require.Contains(t, string(data), "class A")

		require.NoError(t, dbfs.Delete("/"+programID))
		fresh := ssadb.NewIrSourceFs()
		entries, err = fresh.ReadDir("/")
		require.NoError(t, err)
		for _, e := range entries {
			require.NotEqual(t, programID, e.Name())
		}
	})

	t.Run("local_path", func(t *testing.T) {
		dir := t.TempDir()
		paths := []string{
			"example/src/main/java/com/example/apackage",
			"example/src/main/java/com/example/bpackage/sub",
		}
		for _, p := range paths {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, p), 0o755))
		}
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "example/src/main/java/com/example/apackage/a.java"),
			[]byte(codeA), 0o644,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "example/src/main/java/com/example/bpackage/sub/b.java"),
			[]byte(codeB), 0o644,
		))

		programID := uuid.NewString()
		opts := []ssaconfig.Option{
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID),
		}
		_, err := ssaconfig.New(ssaconfig.ModeProjectCompile, opts...)
		require.NoError(t, err)
		_, err = ssaapi.ParseProjectFromPath(dir, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), programID) })

		dbfs := ssadb.NewIrSourceFs()
		assertTree(t, programID, dbfs)
		entries, err := dbfs.ReadDir("/")
		require.NoError(t, err)
		found := false
		for _, e := range entries {
			if e.Name() == programID {
				found = true
				break
			}
		}
		require.True(t, found)
	})
}

func TestSourceFilesystem_YakURL(t *testing.T) {
	codea := `
	package com.example.apackage; 
	import com.example.bpackage.sub.B;
	class A {
		public static void main(String[] args) {
			B b = new B();
			target1(b.get());
			b.show(1);
		}
	}
	`
	codeb := `
	package com.example.bpackage.sub; 
	class B {
		public  int get() {
			return 	 1;
		}
		public void show(int a) {
			target2(a);
		}
	}
	`
	vf := filesys.NewVirtualFs()
	vf.AddFile("example/src/main/java/com/example/apackage/a.java", codea)
	vf.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", codeb)

	compile := func(programID string) {
		t.Helper()
		opts := []ssaconfig.Option{
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID),
		}
		_, err := ssaconfig.New(ssaconfig.ModeProjectCompile, opts...)
		require.NoError(t, err)
		_, err = ssaapi.ParseProjectWithFS(vf, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), programID) })
	}

	programID1 := uuid.NewString()
	programID2 := uuid.NewString()
	compile(programID1)
	compile(programID2)

	local, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	list := func(path string) []string {
		return getDir(local, t, path)
	}
	readFile := func(path string) string {
		t.Helper()
		stream, err := local.ReadFile(context.Background(), &ypb.ReadFileRequest{
			FilePath:   path,
			FileSystem: "ssadb",
		})
		require.NoError(t, err)
		buf := make([]byte, 0, 1024)
		for {
			res, err := stream.Recv()
			if err != nil {
				require.ErrorIs(t, err, io.EOF, "unexpected error: %v", err)
				break
			}
			buf = append(buf, res.Data...)
		}
		return string(buf)
	}

	t.Run("list_root_and_extra", func(t *testing.T) {
		res, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssadb",
				Path:   "/",
				Query:  []*ypb.KVPair{{Key: "op", Value: "list"}},
			},
		})
		require.NoError(t, err)
		want := map[string]bool{
			"/" + programID1: false,
			"/" + programID2: false,
		}
		for _, resource := range res.Resources {
			if _, ok := want[resource.Path]; !ok {
				continue
			}
			langOK := false
			for _, info := range resource.Extra {
				if info.Key == "Language" && info.Value == string(ssaconfig.JAVA) {
					langOK = true
				}
			}
			require.True(t, langOK, "missing Language extra for %s", resource.Path)
			want[resource.Path] = true
		}
		for path, ok := range want {
			require.True(t, ok, "missing program %s", path)
		}
	})

	root1 := "/" + programID1
	t.Run("list_and_read", func(t *testing.T) {
		require.Contains(t, list("/"), root1)
		entries := list(root1)
		require.Contains(t, entries, root1+"/example")
		entries = list(root1 + "/example")
		require.Contains(t, entries, root1+"/example/src")

		require.Equal(t, codea, readFile(root1+"/example/src/main/java/com/example/apackage/a.java"))
		require.Equal(t, codeb, readFile(root1+"/example/src/main/java/com/example/bpackage/sub/b.java"))
	})

	t.Run("delete", func(t *testing.T) {
		deletePath := "/" + programID1
		_, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
			Method: "DELETE",
			Url:    &ypb.YakURL{Schema: "ssadb", Path: deletePath},
		})
		require.NoError(t, err)
		for _, path := range list("/") {
			require.NotEqual(t, deletePath, path)
		}
	})
}

func getDir(local ypb.YakClient, t *testing.T, path string) []string {
	t.Helper()
	res, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema: "ssadb",
			Path:   path,
			Query:  []*ypb.KVPair{{Key: "op", Value: "list"}},
		},
	})
	require.NoError(t, err)
	files := make([]string, 0, len(res.Resources))
	for _, info := range res.Resources {
		files = append(files, info.Path)
	}
	return files
}

func TestProgram_NewProgram(t *testing.T) {
	local, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	seed := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		progName := uuid.NewString()
		opts := []ssaconfig.Option{ssaapi.WithProgramName(progName)}
		_, err := ssaconfig.New(ssaconfig.ModeSSACompile, opts...)
		require.NoError(t, err)
		_, err = ssaapi.Parse(`println("a")`, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), progName) })
		seed = append(seed, progName)
	}
	_ = seed

	before := getDir(local, t, "/")
	progName := uuid.NewString()
	opts := []ssaconfig.Option{ssaapi.WithProgramName(progName)}
	_, err = ssaconfig.New(ssaconfig.ModeSSACompile, opts...)
	require.NoError(t, err)
	_, err = ssaapi.Parse(`println("a")`, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), progName) })

	after := getDir(local, t, "/")
	assert.Equal(t, len(before)+1, len(after))
	assert.Equal(t, "/"+progName, after[0])
}

func TestIrSourceFS_File_URL(t *testing.T) {
	content := `package org.example
		public class A {
			public void test() {
				println("hello");
			}
		}
	`

	compile := func(files map[string]string) (programID string) {
		t.Helper()
		programID = "prog_" + uuid.NewString()
		opts := []ssaconfig.Option{
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID),
		}
		_, err := ssaconfig.New(ssaconfig.ModeProjectCompile, opts...)
		require.NoError(t, err)
		vf := filesys.NewVirtualFs()
		for path, code := range files {
			vf.AddFile(path, code)
		}
		_, err = ssaapi.ParseProjectWithFS(vf, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), programID) })
		return programID
	}

	t.Run("same_content_different_projects", func(t *testing.T) {
		getSource := func() *ssadb.IrSource {
			fileName := "file_name_" + uuid.NewString() + ".java"
			folder := "path_" + uuid.NewString()
			programID := compile(map[string]string{
				fmt.Sprintf("/%s/%s", folder, fileName): content,
			})
			irSource, err := ssadb.GetIrSourceByPathAndName(fmt.Sprintf("/%s/%s", programID, folder), fileName)
			require.NoError(t, err)
			return irSource
		}
		source1 := getSource()
		source2 := getSource()
		require.NotEqual(t, source1.SourceCodeHash, source2.SourceCodeHash)
	})

	t.Run("same_content_same_project", func(t *testing.T) {
		fileName1 := "file_name_" + uuid.NewString() + ".java"
		fileName2 := "file_name_" + uuid.NewString() + ".java"
		folder := "path_" + uuid.NewString()
		programID := compile(map[string]string{
			fmt.Sprintf("/%s/%s", folder, fileName1): content,
			fmt.Sprintf("/%s/%s", folder, fileName2): content,
		})
		sources, err := ssadb.GetIrSourceByPath(fmt.Sprintf("/%s/%s", programID, folder))
		require.NoError(t, err)
		require.Equal(t, 2, len(sources))
		require.NotEqual(t, sources[0].SourceCodeHash, sources[1].SourceCodeHash)
	})
}
