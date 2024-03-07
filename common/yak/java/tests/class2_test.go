package tests

import (
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"testing"
)

func TestOOP_1(t *testing.T) {
	java2ssa.ParserSSA(`
package foo.bar;

class A {
	public key int a;

	public void foo() {
	}
}
`).Show()
}
