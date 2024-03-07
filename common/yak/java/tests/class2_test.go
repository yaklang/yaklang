package tests

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func TestOOP_1(t *testing.T) {
	test.MockSSA(t, `
package foo.bar;

class A {
	public key int;

	public void foo() {
	}
}
`)
}
