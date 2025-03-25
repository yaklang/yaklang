package ssaapi_test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestValue_NewValue(t *testing.T) {
	code := `
a = 1 
if c {
    print(a)
}
	`
	ssatest.CheckSyntaxFlow(t, code, `
c<getUsers> as $if 
`, map[string][]string{
		"if": {"if (Undefined-c)"},
	})
}
