package ssatest

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func checkProcess(vf filesys_interface.FileSystem, t *testing.T, opt ...ssaapi.Option) {
	type message struct {
		msg     string
		process float64
	}

	matchRightProcess := false
	msgs := make([]message, 0)
	programID := uuid.NewString()
	opt = append(opt,
		ssaapi.WithProgramName(programID),
		ssaapi.WithProcess(func(msg string, process float64) {
			if 0 < process && process < 1 {
				matchRightProcess = true
			}
			msgs = append(msgs, message{msg, process})
		}),
	)
	prog, err := ssaapi.ParseProject(vf, opt...)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programID)
	assert.NoError(t, err)
	assert.NotNil(t, prog)
	assert.True(t, matchRightProcess)
	log.Infof("message: %v", msgs)
	assert.Greater(t, len(msgs), 0)
	end := msgs[len(msgs)-1]
	assert.Equal(t, end.process, float64(1))

}

func TestParseProject_JAVA(t *testing.T) {
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
		import com.example.cpackage.C;
		class B {
			public  int get() {
				return 	 1;
			}
			public void show(int a) {
				var c = new C();
				target2(a);
			}
		}
	`)

	vf.AddFile("example/src/main/java/com/example/cpackage/c.java", `
	package com.example.cpackage;
	class C {
		public static void CFunc(String[] args) {
			System.out.println("Hello, World");
		}
	}
	`)

	checkProcess(vf, t, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestParseProject_PHP(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("example/src/main/php/a.php", `
		<?php
		require_once("b.php");
		class A {
			public function main() {
				$b = new B();
				// for test 1: A->B
				target1($b->get());
				// for test 2: B->A
				$b->show(1);
			}
		}`)
	vf.AddFile("example/src/main/php/b.php", `
		<?php
		require_once("c.php");
		class B {
			public function get() {
				return 1;
			}
		}`)
	vf.AddFile("example/src/main/php/c.php", `
		<?php
		class C {
			public function CFunc() {
				echo "Hello, World";	
			}
		}`)

	checkProcess(vf, t, ssaapi.WithLanguage(ssaapi.PHP))
}

func TestParseProject_PHP_withEmptyFile(t *testing.T) {
	t.Run("empty file ", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("example/src/main/php/a.php", `
		<?php
		require_once("b.php");
		`)
		vf.AddFile("example/src/main/php/b.php", `
		<?php
		`)
		vf.AddFile("example/src/main/php/c.php", ``)

		checkProcess(vf, t, ssaapi.WithLanguage(ssaapi.PHP))
	})

	t.Run("empty file with include", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("example/src/main/php/a.php", `
		<?php
		require_once("b.php");
		`)
		vf.AddFile("example/src/main/php/b.php", `
		<?php
		require_once("c.php");
		`)
		vf.AddFile("example/src/main/php/c.php", ``)

		checkProcess(vf, t, ssaapi.WithLanguage(ssaapi.PHP))
	})
}

func TestStopProcessByCtx(t *testing.T) {
	toCreate := []string{
		"example/src/main/java/com/example/testcontextFSB/a.java",
		"example/src/main/java/com/example/testcontextFSB/b.java",
		"example/src/main/java/com/example/testcontextFSA/a.java",
		"example/src/main/java/com/example/testcontextFSA/b.java",
		"example/src/main/java/com/example/testcontextFSA/c.java",
		"example/src/main/java/com/example/testcontextFSA/d.java",
		"example/src/main/java/com/example/testcontextFSA/e.java",
		"example/src/main/java/com/example/testcontextFSC/a.java",
		"example/src/main/java/com/example/testcontextFSC/b.java",
		"example/src/main/java/com/example/testcontextFSC/c.java",
	}

	t.Run("test local fileSystem stop process by context", func(t *testing.T) {
		ssadb.DeleteProgram(ssadb.GetDB(), "testcontextFSA")
		ssadb.DeleteProgram(ssadb.GetDB(), "testcontextFSB")
		ssadb.DeleteProgram(ssadb.GetDB(), "testcontextFSC")
		dir := t.TempDir()
		log.Infof("dir: %v", dir)

		for _, path := range toCreate {
			path = fmt.Sprintf("%s/%s", dir, path)
			pathDir := filepath.Dir(path)
			err := os.MkdirAll(pathDir, os.ModePerm)
			require.NoError(t, err)
			fd, err := os.Create(path)
			require.NoError(t, err)
			dirPath := filepath.Dir(path)
			lastDirName := filepath.Base(dirPath)
			pkg := fmt.Sprintf("package %s;", lastDirName)
			fd.WriteString(pkg)
			fd.Close()
		}
		var maxProcess float64
		programID := uuid.NewString()
		ctx, cancel := context.WithCancel(context.Background())
		ctxFS := filesys.NewFileSystemWithContext(ctx, filesys.NewRelLocalFs(dir))
		_, err := ssaapi.ParseProject(ctxFS,
			ssaapi.WithLanguage(ssaapi.JAVA),
			ssaapi.WithProgramName(programID),
			ssaapi.WithSaveToProfile(),
			ssaapi.WithProcess(func(msg string, process float64) {
				if process >= 0.5 {
					cancel()
				}
				if process > maxProcess {
					maxProcess = process
				}
				log.Infof("message %v, process: %f", msg, process)
			}),
		)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
			ssadb.DeleteSSAProgram(programID)
		}()
		assert.NoErrorf(t, err, "parse project error: %v", err)
		require.LessOrEqual(t, maxProcess, 0.5)
		file := make([]string, 0)
		dbfs := ssadb.NewIrSourceFs()
		filesys.Recursive(
			fmt.Sprintf("/%s", programID),
			filesys.WithFileSystem(dbfs),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				file = append(file, path)
				return nil
			}),
		)
		require.LessOrEqual(t, len(file), 5)
	})

	t.Run("test virtual fileSystem stop process by context", func(t *testing.T) {
		ssadb.DeleteProgram(ssadb.GetDB(), "testcontextFSA")
		ssadb.DeleteProgram(ssadb.GetDB(), "testcontextFSB")
		ssadb.DeleteProgram(ssadb.GetDB(), "testcontextFSC")

		vf := filesys.NewVirtualFs()
		for _, path := range toCreate {
			dirPath := filepath.Dir(path)
			lastDirName := filepath.Base(dirPath)
			vf.AddFile(path, fmt.Sprintf("package %s;", lastDirName))
		}

		var maxProcess float64
		programID := uuid.NewString()
		ctx, cancel := context.WithCancel(context.Background())
		ctxFS := filesys.NewFileSystemWithContext(ctx, vf)
		_, err := ssaapi.ParseProject(ctxFS,
			ssaapi.WithLanguage(ssaapi.JAVA),
			ssaapi.WithProgramName(programID),
			ssaapi.WithSaveToProfile(),
			ssaapi.WithProcess(func(msg string, process float64) {
				if process >= 0.5 {
					cancel()
				}
				if process > maxProcess {
					maxProcess = process
				}
				log.Infof("message %v, process: %f", msg, process)
			}),
		)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
			ssadb.DeleteSSAProgram(programID)
		}()
		assert.NoErrorf(t, err, "parse project error: %v", err)
		require.LessOrEqual(t, maxProcess, 0.5)
		file := make([]string, 0)
		dbfs := ssadb.NewIrSourceFs()
		filesys.Recursive(
			fmt.Sprintf("/%s", programID),
			filesys.WithFileSystem(dbfs),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				_, path, _ := strings.Cut(s, programID+"/")
				file = append(file, path)
				return nil
			}),
		)
		require.LessOrEqual(t, len(file), 5)
	})

}
