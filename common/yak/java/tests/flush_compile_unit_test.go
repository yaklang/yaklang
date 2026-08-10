package tests

import (
	"testing"

		"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestFlushCompileUnitCrossUnitSyntaxFlow verifies that enabling per-batch
// FlushCompileUnit does not break cross-unit SyntaxFlow resolution.
// This is the "TestImportClass over-resolves" bug that originally disabled
// per-batch flush: flushing stores mid-compile caused `#->` to over-resolve
// imported symbols.
//
// The fix only flushes INSTRUCTIONS (not types/indexes/sources), so
// cross-unit resolution should work correctly.
func TestFlushCompileUnitCrossUnitSyntaxFlow(t *testing.T) {
	t.Run("cross-file reference after per-batch flush", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("com/example/demo1/A.java", `
package com.example.demo1;
class A {
    public static int a = 1;
    public static int test() {
        return 1;
    }
}
`)
		fs.AddFile("com/example/demo2/test.java", `
package com.example.demo2;
import com.example.demo1.A;
class test {
    public static void main(String[] args) {
        println(A.test());
    }
}
`)
		// This SyntaxFlow query traverses cross-unit: test.java calls A.test()
		// which returns 1. The `#->` operator must resolve this correctly
		// even when per-batch flush is enabled.
		ssatest.CheckSyntaxFlowWithFS(t, fs, `println(* #-> * as $param)`,
			map[string][]string{"param": {"1"}},
			false,
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("cross-file member access after per-batch flush", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("a.java", `
package com.example.demo1;
import com.example.demo.B;
class A {
    public B b;
    public void main() {
        println(this.b.a);
    }
}
`)
		fs.AddFile("b.java", `
package com.example.demo;
class B {
    public static int a = 1;
}
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `println(* as $sink)`,
			map[string][]string{
				"sink": {"ParameterMember-parameterMember"},
			},
			true,
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})
}


