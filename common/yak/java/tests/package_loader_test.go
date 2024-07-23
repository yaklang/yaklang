package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPackageLoader(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("example/src/main/java/com/example/apackage/a.java", `
		package com.example.apackage; 
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				B b = new B();
				System.out.println(b.get());
			}
		}
		`)

		vf.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", `
		package com.example.bpackage.sub; 
		class B {
			public  int get() {
				return 	 1;
			}
		}
		`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
			System.out.println(* #-> as $a)
			`, map[string][]string{
			"a": {"1"},
		}, false,
			ssaapi.WithLanguage(ssaapi.JAVA),
			ssaapi.WithProgramPath("example"),
		)
	})

}
