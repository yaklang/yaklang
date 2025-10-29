package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPackageLoader(t *testing.T) {
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
	t.Run("check import class from outer", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `
			target1(* #-> as $a)
			`, map[string][]string{
			"a": {"1"},
		}, false,
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramPath("example"),
		)
	})

	t.Run("check import class from inner", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `
			target2(* #-> as $a)
			`, map[string][]string{
			"a": {"1"},
		}, false,
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramPath("example"),
		)
	})

}
