package ssadb_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSourceFilesysLocal(t *testing.T) {
	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.apackage")
	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.bpackage.sub")

	dir := fmt.Sprintf("%s/ssa_source_test", os.TempDir())
	os.Mkdir(dir, os.ModePerm)
	defer os.RemoveAll(dir)
	log.Infof("dir: %v", dir)
	// create file in dir
	os.Mkdir(fmt.Sprintf("%s/example", dir), os.ModePerm)
	os.Mkdir(fmt.Sprintf("%s/example/src", dir), os.ModePerm)
	os.Mkdir(fmt.Sprintf("%s/example/src/main", dir), os.ModePerm)
	os.Mkdir(fmt.Sprintf("%s/example/src/main/java", dir), os.ModePerm)
	os.Mkdir(fmt.Sprintf("%s/example/src/main/java/com", dir), os.ModePerm)
	os.Mkdir(fmt.Sprintf("%s/example/src/main/java/com/example", dir), os.ModePerm)
	os.Mkdir(fmt.Sprintf("%s/example/src/main/java/com/example/apackage", dir), os.ModePerm)
	os.Mkdir(fmt.Sprintf("%s/example/src/main/java/com/example/bpackage", dir), os.ModePerm)
	os.Mkdir(fmt.Sprintf("%s/example/src/main/java/com/example/bpackage/sub", dir), os.ModePerm)
	fd, err := os.OpenFile(
		fmt.Sprintf("%s/example/src/main/java/com/example/apackage/a.java", dir),
		os.O_CREATE|os.O_RDWR, os.ModePerm,
	)
	assert.NoError(t, err)
	fd.Write([]byte(`
		package com.example.apackage; 
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				B b = new B();
				// for test 1: A->B
				target1(b.get());
				// for test 2: B->A
				b.show(1);
			}
		}
	`))

	fd, err = os.OpenFile(
		fmt.Sprintf("%s/example/src/main/java/com/example/bpackage/sub/b.java", dir),
		os.O_CREATE|os.O_RDWR, os.ModePerm,
	)
	assert.NoError(t, err)
	fd.Write([]byte(`
		package com.example.bpackage.sub; 
		class B {
			public  int get() {
				return 	 1;
			}
			public void show(int a) {
				target2(a);
			}
		}
		`))
	programID := uuid.NewString()
	_, err = ssaapi.ParseProjectFromPath(dir,
		ssaapi.WithLanguage(ssaapi.JAVA),
		ssaapi.WithProgramName(programID),
		ssaapi.WithSaveToProfile(),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
		ssadb.DeleteSSAProgram(programID)
	}()
	assert.NoErrorf(t, err, "parse project error: %v", err)
	dirs := make([]string, 0)
	file := make([]string, 0)
	dbfs := ssadb.NewIrSourceFs()
	t.Run("test source file system", func(t *testing.T) {
		filesys.Recursive(
			fmt.Sprintf("/%s", programID),
			filesys.WithFileSystem(dbfs),
			filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				if path != "" {
					dirs = append(dirs, path)
				}
				return nil
			}),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				file = append(file, path)
				return nil
			}),
		)

		assert.Equal(t, []string{
			"example", "example/src", "example/src/main", "example/src/main/java",
			"example/src/main/java/com", "example/src/main/java/com/example",
			"example/src/main/java/com/example/apackage",
			"example/src/main/java/com/example/bpackage",
			"example/src/main/java/com/example/bpackage/sub",
		}, dirs)
		assert.Equal(t, []string{
			"example/src/main/java/com/example/apackage/a.java",
			"example/src/main/java/com/example/bpackage/sub/b.java",
		}, file)
	})
	t.Run("test source file system root path", func(t *testing.T) {
		info, err := dbfs.Stat("/")
		_ = info
		require.NoErrorf(t, err, "stat error: %v", err)

		dirs = make([]string, 0)
		infos, err := dbfs.ReadDir("/")
		require.NoErrorf(t, err, "read dir error: %v", err)
		for _, info := range infos {
			dirs = append(dirs, info.Name())
		}
		assert.Contains(t, dirs, programID)
	})

	t.Run("test new source file system root path", func(t *testing.T) {
		dbfs := ssadb.NewIrSourceFs()
		info, err := dbfs.Stat("/")
		_ = info
		require.NoErrorf(t, err, "stat error: %v", err)

		infos, err := dbfs.ReadDir("/")
		require.NoErrorf(t, err, "read dir error: %v", err)
		for _, info := range infos {
			log.Infof("info: %v", info.Name())
		}
	})

	t.Run("remove all program and query root path", func(t *testing.T) {
		dbfs := ssadb.NewIrSourceFs()
		info, err := dbfs.Stat("/")
		_ = info
		require.NoErrorf(t, err, "stat error: %v", err)

		infos, err := dbfs.ReadDir("/")
		require.NoErrorf(t, err, "read dir error: %v", err)
		for _, info := range infos {
			log.Infof("info: %v", info.Name())
			if info.Name() == programID {
				err := dbfs.Delete("/" + info.Name())
				require.NoErrorf(t, err, "delete error: %v", err)
			}
		}
		newFS := ssadb.NewIrSourceFs()
		infos, err = newFS.ReadDir("/")
		require.NoErrorf(t, err, "read dir error: %v", err)
		for _, info := range infos {
			if info.Name() == programID {
				t.Fatalf("program %v not deleted", programID)
			}
		}
	})
}

func TestSourceFilesys(t *testing.T) {

	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.apackage")
	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.bpackage.sub")

	vf := filesys.NewVirtualFs()
	vf.AddFile("example/src/main/java/com/example/apackage/a.java", `
		package com.example.apackage; 
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				B b = new B();
				// for test 1: A->B
				target1(b.get());
				// for test 2: B->A
				b.show(1);
			}
		}
		`)

	vf.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", `
		package com.example.bpackage.sub; 
		class B {
			public  int get() {
				return 	 1;
			}
			public void show(int a) {
				target2(a);
			}
		}
		`)

	programID := uuid.NewString()
	_, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaapi.JAVA),
		ssaapi.WithProgramName(programID),
		ssaapi.WithSaveToProfile(),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
		ssadb.DeleteSSAProgram(programID)
	}()
	assert.NoErrorf(t, err, "parse project error: %v", err)
	dir := make([]string, 0)
	file := make([]string, 0)
	dbfs := ssadb.NewIrSourceFs()

	t.Run("test source file system", func(t *testing.T) {
		filesys.Recursive(
			fmt.Sprintf("/%s", programID),
			filesys.WithFileSystem(dbfs),
			filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				if path != "" {
					dir = append(dir, path)
				}
				return nil
			}),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				file = append(file, path)
				return nil
			}),
		)

		assert.Equal(t, []string{
			"example", "example/src", "example/src/main", "example/src/main/java",
			"example/src/main/java/com", "example/src/main/java/com/example",
			"example/src/main/java/com/example/apackage",
			"example/src/main/java/com/example/bpackage",
			"example/src/main/java/com/example/bpackage/sub",
		}, dir)
		assert.Equal(t, []string{
			"example/src/main/java/com/example/apackage/a.java",
			"example/src/main/java/com/example/bpackage/sub/b.java",
		}, file)
	})

	t.Run("test source file system root path", func(t *testing.T) {
		dir = make([]string, 0)
		filesys.Recursive(
			"/",
			filesys.WithFileSystem(dbfs),
			filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
				log.Infof("dir: %v", s)
				paths := strings.Split(s, string(dbfs.GetSeparators()))
				if len(paths) == 2 && paths[1] != "" {
					dir = append(dir, paths[1])
					return filesys.SkipDir
				}
				return nil
			}),
		)
		assert.Contains(t, dir, programID)
	})
}

func TestProgram_ListAndDelete(t *testing.T) {

	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.apackage")
	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.bpackage.sub")

	vf := filesys.NewVirtualFs()
	vf.AddFile("example/src/main/java/com/example/apackage/a.java", `
	package com.example.apackage; 
	import com.example.bpackage.sub.B;
	class A {
		public static void main(String[] args) {
			B b = new B();
			// for test 1: A->B
			target1(b.get());
			// for test 2: B->A
			b.show(1);
		}
	}
	`)

	vf.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", `
	package com.example.bpackage.sub; 
	class B {
		public  int get() {
			return 	 1;
		}
		public void show(int a) {
			target2(a);
		}
	}
	`)

	/*
		default-ssa.db:
			programID1
			programID2
		tmp/ssa.db:
			programID3
	*/
	dir := fmt.Sprintf("%s/ssa_source_test", os.TempDir())
	os.Mkdir(dir, os.ModePerm)
	defer os.RemoveAll(dir)
	log.Infof("dir: %v", dir)
	dbPath := fmt.Sprintf("%s/%s", dir, "ssa.db")

	var err error
	programID1 := uuid.NewString()
	_, err = ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithProgramName(programID1), ssaapi.WithSaveToProfile())
	defer func() {
		ssadb.CheckAndSwitchDB(programID1)
		ssadb.DeleteProgram(ssadb.GetDB(), programID1)
		ssadb.DeleteSSAProgram(programID1)
	}()
	assert.NoErrorf(t, err, "parse project error: %v", err)

	programID2 := uuid.NewString()
	_, err = ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithProgramName(programID2), ssaapi.WithSaveToProfile())
	defer func() {
		ssadb.CheckAndSwitchDB(programID2)
		ssadb.DeleteProgram(ssadb.GetDB(), programID2)
		ssadb.DeleteSSAProgram(programID2)
	}()
	assert.NoErrorf(t, err, "parse project error: %v", err)

	consts.SetSSADataBasePath(dbPath)
	programID3 := uuid.NewString()
	_, err = ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithProgramName(programID3), ssaapi.WithSaveToProfile())
	defer func() {
		ssadb.CheckAndSwitchDB(programID3)
		ssadb.DeleteProgram(ssadb.GetDB(), programID3)
		ssadb.DeleteSSAProgram(programID3)
	}()
	assert.NoErrorf(t, err, "parse project error: %v", err)

	t.Run("test source file system root path", func(t *testing.T) {
		dir := make([]string, 0)
		ssafs := ssadb.NewIrSourceFs()
		filesys.Recursive(
			"/",
			filesys.WithFileSystem(ssafs),
			filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
				log.Infof("dir: %v", s)
				paths := strings.Split(s, string(ssafs.GetSeparators()))
				if len(paths) <= 3 && paths[1] != "" {
					dir = append(dir, paths[1])
					return filesys.SkipDir
				}
				return nil
			}),
		)
		assert.Contains(t, dir, programID1)
		assert.Contains(t, dir, programID2)
		assert.Contains(t, dir, programID3)
	})

	local, err := yakgrpc.NewLocalClient()
	assert.NoError(t, err)

	t.Run("program list and extra info  ", func(t *testing.T) {
		res, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssadb",
				Path:   "/",
				Query: []*ypb.KVPair{
					{
						Key:   "op",
						Value: "list",
					},
				},
			},
		})
		assert.NoErrorf(t, err, "load resource error: %v", err)
		// log.Infof("res: %v", res)
		match := map[string]bool{
			fmt.Sprintf("/%s", programID1): false,
			fmt.Sprintf("/%s", programID2): false,
			fmt.Sprintf("/%s", programID3): false,
		}
		for _, res := range res.Resources {
			if _, ok := match[res.Path]; ok {
				log.Infof("res: %v", res.Path)
				matchExtra := false
				for _, info := range res.Extra {
					if info.Key == "Language" {
						if info.Value == string(ssaapi.JAVA) {
							matchExtra = true
						}
					}
					log.Infof("extra: %v", info)
				}
				if !matchExtra {
					t.Fatalf("not found Language")
				}
				match[res.Path] = true
			}
		}

		for k, v := range match {
			assert.Truef(t, v, "not found: %v", k)
		}
	})

	t.Run("delete", func(t *testing.T) {
		deletePath := fmt.Sprintf("/%s", programID1)
		_, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
			Method: "DELETE",
			Url: &ypb.YakURL{
				Schema: "ssadb",
				Path:   deletePath,
			},
		})
		assert.NoErrorf(t, err, "delete error %v", err)

		res, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssadb",
				Path:   "/",
				Query: []*ypb.KVPair{
					{
						Key:   "op",
						Value: "list",
					},
				},
			},
		})
		assert.NoErrorf(t, err, "load resource error: %v", err)
		// log.Infof("res: %v", res)
		for _, info := range res.Resources {
			if info.Path == deletePath {
				t.Fatal("path deleted, but contain in all program ")
			}
		}
	})

}

func TestProgram_NewProgram(t *testing.T) {
	local, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)
	get := func() []string {
		res, err := local.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssadb",
				Path:   "/",
				Query: []*ypb.KVPair{
					{
						Key:   "op",
						Value: "list",
					},
				},
			},
		})
		require.NoError(t, err)
		progs := make([]string, 0, len(res.Resources))
		for _, info := range res.Resources {
			progs = append(progs, info.Path)
		}
		return progs
	}
	{
		progName := uuid.NewString()
		_, err := ssaapi.Parse(`println("a")`, ssaapi.WithProgramName(progName), ssaapi.WithSaveToProfile())
		require.NoError(t, err)
		defer ssadb.DeleteSSAProgram(progName)
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}
	{
		progName := uuid.NewString()
		_, err := ssaapi.Parse(`println("a")`, ssaapi.WithProgramName(progName), ssaapi.WithSaveToProfile())
		require.NoError(t, err)
		defer ssadb.DeleteSSAProgram(progName)
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}
	{
		progName := uuid.NewString()
		_, err := ssaapi.Parse(`println("a")`, ssaapi.WithProgramName(progName), ssaapi.WithSaveToProfile())
		require.NoError(t, err)
		defer ssadb.DeleteSSAProgram(progName)
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}
	{
		progName := uuid.NewString()
		_, err := ssaapi.Parse(`println("a")`, ssaapi.WithProgramName(progName), ssaapi.WithSaveToProfile())
		require.NoError(t, err)
		defer ssadb.DeleteSSAProgram(progName)
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}

	t.Run("test", func(t *testing.T) {
		progs := get()
		log.Infof("progs: %v", progs)

		progName := uuid.NewString()
		_, err := ssaapi.Parse(`println("a")`, ssaapi.WithProgramName(progName), ssaapi.WithSaveToProfile())
		require.NoError(t, err)
		log.Infof("progName: %v", progName)
		defer ssadb.DeleteSSAProgram(progName)
		defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

		newProgs := get()
		log.Info("new prog: ", newProgs)
		assert.Equal(t, len(progs)+1, len(newProgs))
		assert.Equal(t, fmt.Sprintf("/%s", progName), newProgs[0])
	})
}

func TestIrSourceFS_File_URL(t *testing.T) {
	content := `package org.example
		public class A {
			public void test() {
				println("hello");
			}
		}
	`

	t.Run("test compile the same content in different project", func(t *testing.T) {
		compileAndGetSource := func() *ssadb.IrSource {
			vf := filesys.NewVirtualFs()
			fileName := "file_name_" + uuid.NewString()
			programID := "prog_" + uuid.NewString()
			path := "path_" + uuid.NewString()
			vf.AddFile(fmt.Sprintf("/%s/%s", path, fileName), content)

			_, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithProgramName(programID), ssaapi.WithSaveToProfile())
			require.NoError(t, err)
			t.Cleanup(func() {
				ssadb.DeleteProgram(ssadb.GetDB(), programID)
				ssadb.DeleteSSAProgram(programID)
			})
			fullPath := fmt.Sprintf("/%s/%s", programID, path)
			irSource, err := ssadb.GetIrSourceByPathAndName(fullPath, fileName)
			require.NoError(t, err)
			return irSource
		}

		// 相同内容，不同文件的source code hash不应该一样
		source1 := compileAndGetSource()
		require.NotNil(t, source1)
		source2 := compileAndGetSource()
		require.NotNil(t, source2)
		require.NotEqual(t, source1.SourceCodeHash, source2.SourceCodeHash)
	})

	t.Run("test compile the same content in the same project", func(t *testing.T) {
		compileAndGetSource := func() []*ssadb.IrSource {
			vf := filesys.NewVirtualFs()
			fileName1 := "file_name_" + uuid.NewString()
			fileName2 := "file_name_" + uuid.NewString()

			programID := "prog_" + uuid.NewString()
			path := "path_" + uuid.NewString()

			vf.AddFile(fmt.Sprintf("/%s/%s", path, fileName1), content)
			vf.AddFile(fmt.Sprintf("/%s/%s", path, fileName2), content)

			_, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithProgramName(programID), ssaapi.WithSaveToProfile())
			require.NoError(t, err)
			t.Cleanup(func() {
				ssadb.DeleteProgram(ssadb.GetDB(), programID)
				ssadb.DeleteSSAProgram(programID)
			})
			fullPath := fmt.Sprintf("/%s/%s", programID, path)
			irSources, err := ssadb.GetIrSourceByPath(fullPath)
			require.NoError(t, err)
			return irSources
		}

		source := compileAndGetSource()
		require.Equal(t, 2, len(source))
		require.NotEqual(t, source[0].SourceCodeHash, source[1].SourceCodeHash)
	})
}
