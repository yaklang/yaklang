package ssadb_test

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yakurl"
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
	)
	// defer func() {
	// 	ssadb.DeleteProgram(ssadb.GetDB(), programID)
	// }()
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
	_, err := ssaapi.ParseProject(vf,
		ssaapi.WithLanguage(ssaapi.JAVA),
		ssaapi.WithProgramName(programID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
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
	_, err = ssaapi.ParseProject(vf, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithProgramName(programID1))
	assert.NoErrorf(t, err, "parse project error: %v", err)

	programID2 := uuid.NewString()
	_, err = ssaapi.ParseProject(vf, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithProgramName(programID2))
	assert.NoErrorf(t, err, "parse project error: %v", err)

	consts.SetSSADataBasePath(dbPath)
	programID3 := uuid.NewString()
	_, err = ssaapi.ParseProject(vf, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithProgramName(programID3))
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
				if len(paths) == 2 && paths[1] != "" {
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

	t.Run("program list and extra info  ", func(t *testing.T) {
		res, err := yakurl.LoadGetResource("ssadb:///?op=list")
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

}
