package ssatest

import (
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
