package ssatest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
)

type checkFunction func(*ssaapi.Program) error

func Check(t *testing.T, code string, handler checkFunction, languages ...ssaapi.Language) {
	opt := make([]ssaapi.Option, 0)
	if len(languages) > 0 {
		opt = append(opt, ssaapi.WithLanguage(languages[0]))
	}
	prog, err := ssaapi.Parse(code, opt...)
	if err != nil {
		t.Fatalf("prog parse error: %v", err)
	}
	prog.Show()

	if err := handler(prog); err != nil {
		t.Fatal("check failed: ", err)
	}
}

func CheckBottomUser_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				return p.Ref(variable)
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetBottomUses() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckBottomUserCall_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				lastIndex := strings.LastIndex(variable, ".")
				if lastIndex != -1 {
					member := variable[:lastIndex]
					key := variable[lastIndex+1:]
					return p.Ref(member).Ref(key)
				} else {
					return p.Ref(variable)
				}
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetBottomUses() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckTopDef_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				return p.Ref(variable)
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetTopDefs() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckTopDef_Equal(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				return p.Ref(variable)
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetTopDefs() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return v1.String() == v2
			},
		)
	}
}

func checkFunctionEx(
	variable func() ssaapi.Values, // variable  for test
	get func(*ssaapi.Value) ssaapi.Values, // getTop / getBottom
	checkLength bool,
	want []string,
	compare func(*ssaapi.Value, string) bool,
) error {
	values := variable()
	if len(values) != 1 {
		return fmt.Errorf("variable[%s] not len(1): %d", values, len(values))
	}
	value := values[0]
	vs := get(value)
	vs = lo.UniqBy(vs, func(v *ssaapi.Value) int64 { return v.GetId() })
	if checkLength {
		if len(vs) != len(want) {
			return fmt.Errorf("variable[%v] not want len(%d): %d: %v", values, len(want), len(vs), vs)
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
