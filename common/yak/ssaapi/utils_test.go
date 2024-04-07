package ssaapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
)

type checkFunction func(*Program) error

func Check(t *testing.T, code string, handler checkFunction, languages ...Language) {
	opt := make([]Option, 0)
	if len(languages) > 0 {
		opt = append(opt, WithLanguage(languages[0]))
	}
	prog, err := Parse(code, opt...)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()

	if err := handler(prog); err != nil {
		t.Fatal("check failed: ", err)
	}
}

func CheckBottomUser_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			p, variable,
			func(v *Value) Values { return v.GetBottomUses() },
			checkLength, want,
			func(v1 *Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckTopDef_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			p, variable,
			func(v *Value) Values { return v.GetTopDefs() },
			checkLength, want,
			func(v1 *Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}
func CheckTopDef_Equal(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			p, variable,
			func(v *Value) Values { return v.GetTopDefs() },
			checkLength, want,
			func(v1 *Value, v2 string) bool {
				return v1.String() == v2
			},
		)
	}
}

func checkFunctionEx(
	prog *Program, // program
	variable string, // variable  for test
	get func(*Value) Values, // getTop / getBottom
	checkLength bool,
	want []string,
	compare func(*Value, string) bool,
) error {
	values := prog.Ref(variable)
	if len(values) != 1 {
		return fmt.Errorf("variable[%s] not len(1): %d", variable, len(values))
	}
	value := values[0]
	vs := get(value)
	vs = lo.UniqBy(vs, func(v *Value) int { return v.GetId() })
	if checkLength {
		if len(vs) != len(want) {
			return fmt.Errorf("variable[%s] not want len(%d): %d: %v", variable, len(want), len(vs), vs)
		}
	}
	mark := make([]bool, len(want))
	for _, value := range vs {
		log.Infof("value: %s", value.String())
		for j, w := range want {
			mark[j] = mark[j] || compare(value, w)
		}
	}
	for i, m := range mark {
		if !m {
			return fmt.Errorf("want[%d] %s not found", i, want[i])
		}
	}
	return nil

}
