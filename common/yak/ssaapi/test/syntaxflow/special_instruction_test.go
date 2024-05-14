package syntaxflow

import (
	"testing"

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
