package tests

import (
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func init() {
	test.SetLanguage("java", java2ssa.Build)
}
func TestOOP_1(t *testing.T) {
	test.MockSSA(t, `
package foo.bar;

class A {
	public  int key;

	public void foo() {
	}
}
`)
}
