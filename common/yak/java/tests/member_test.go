package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestMemberThis(t *testing.T) {
	t.Run("test simple", func(t *testing.T) {
		code := `
package foo.bar;

class A {
	public  int key;

	public void foo() {
		print(this.key.String());
	}
}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`key.String() as $target`,
			map[string][]string{
				"target": {"Undefined-this.key.String()"},
			},
			ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
}
