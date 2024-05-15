package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestParameterMember(t *testing.T) {
	code := `
	f = (a)  => {
		target = a.c
		print(target)
	}
	`
	ssatest.CheckSyntaxFlowContain(t, code,
		`print(*) #-> * as $target`,
		map[string][]string{
			"target": {
				"Parameter-a",
			},
		},
	)
}

func Test_ExternFunction(t *testing.T) {
	code := `
	a = 1
	print(a)
	`
	ssatest.CheckSyntaxFlow(t, code,
		`print(* as $target)`,
		map[string][]string{
			"target": {"1"},
		},
		ssaapi.WithExternValue(map[string]any{
			"print": func(a any) {},
		}),
	)
}
