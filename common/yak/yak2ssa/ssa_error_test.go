package yak2ssa

import (
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type TestCase struct {
	code           string
	errs           []string
	ExternInstance map[string]any
}

func CheckTestCase(t *testing.T, tc TestCase) {
	opts := make([]Option, 0)
	if tc.ExternInstance != nil {
		opts = append(opts, WithSymbolTable(tc.ExternInstance))
	}
	prog := ParseSSA(tc.code, opts...)
	// prog.Show()
	// fmt.Println(prog.GetErrors().String())
	errs := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string { return e.Message })
	if len(errs) != len(tc.errs) {
		t.Fatalf("error len not match %d vs %d", len(errs), len(tc.errs))
	}
	for i := 0; i < len(errs); i++ {
		for errs[i] != tc.errs[i] {
			t.Fatalf("error not match %s vs %s", errs[i], tc.errs[i])
		}
	}
}

func TestCfgEmptyBasic(t *testing.T) {
	CheckTestCase(t, TestCase{
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
		errs: []string{
			"this value undefine:i",
			"this value undefine:j",
			"this value undefine:a",
			"this value undefine:b",
		},
	})

}

func TestOnlyDeclareVariable(t *testing.T) {
	CheckTestCase(t, TestCase{
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
		errs: []string{
			"this value undefine:a2",
		},
	})
}

func TestUndefinedLexical(t *testing.T) {
	CheckTestCase(t, TestCase{
		code: `
			a == undefined
			`,
		errs: []string{
			"this value undefine:a",
		},
	})
}

func TestFreeValueAheadExternInstance(t *testing.T) {
	CheckTestCase(t, TestCase{
		code: `
			param() // extern value 
			param = "" // value
			delayFuzz =() =>{
				param.a().b() // freeValue 
			}
			`,
		errs: []string{},
		ExternInstance: map[string]any{
			"param": func() {},
		},
	})

}
