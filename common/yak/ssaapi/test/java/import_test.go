package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestImport(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("a.java", `
package com.sun.lang.javaCC;
class A{
	public function get(int a){
		println(a);
	}
}
`)
	fs.AddFile("b.java", `
package com.sun.lang.javaCC1;
import com.sun.lang.javaCC.A;
class B{
	public A a;
	public function test(){
		a.get(1);
	}
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, fs, `println(* #-> * as $param)`, map[string][]string{
		"param": {"1"},
	}, true, ssaapi.WithLanguage(ssaconfig.JAVA))
}
