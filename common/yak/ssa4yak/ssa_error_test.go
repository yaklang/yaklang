package ssa4yak

import (
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestError(t *testing.T) {
	testcase := []struct {
		name string
		code string
		err  []string
	}{
		{
			name: "loop-if empty basicblock",
			code: `
			for i {
				if j {
					return a  
				}else {
					return b 
				}
				// unreachable
			}
			`,
			err: []string{
				"this value undefine:i",
				"this value undefine:j",
				"this value undefine:a",
				"this value undefine:b",
			},
		},

		{
			name: "only declear variable",
			code: `
			var a1 
			if 1 {
				a1 = 1
			}
			b = a1

			// var a2 -> undefine
			if 1 {
				a2 = 1
			}
			c = a2
			`,
			err: []string{
				"this value undefine:a2",
			},
		},

		{
			name: "undefined lexical",
			code: `
			a == undefined
			`,
			err: []string{
				"this value undefine:a",
			},
		},

		{
			name: "free-value in extern-instance ahead",
			code: `
			param = ""
			delayFuzz =() =>{
				param.a().b()
			}
			`,
			err: []string{},
		},
	}

	for _, tc := range testcase {
		t.Logf("run test : %s", tc.name)
		prog := ParseSSA(tc.code)
		prog.Show()
		fmt.Println(prog.GetErrors().String())
		err := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string { return e.Message })
		if len(err) != len(tc.err) {
			t.Fatalf("error len not match %d vs %d", len(err), len(tc.err))
		}
		for i := 0; i < len(err); i++ {
			for err[i] != tc.err[i] {
				t.Fatalf("error not match %s vs %s", err[i], tc.err[i])
			}
		}
	}
}
