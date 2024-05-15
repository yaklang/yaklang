package ssatest

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
)

type checkFunction func(*ssaapi.Program) error

func Check(t *testing.T, code string, handler checkFunction, opt ...ssaapi.Option) {
	// only in memory
	{
		prog, err := ssaapi.Parse(code, opt...)
		assert.Nil(t, err)
		// prog.Show()

		log.Infof("only in memory ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	programID := uuid.NewString()
	// parse with database
	{
		opt = append(opt, ssaapi.WithDatabaseProgramName(programID))
		prog, err := ssaapi.Parse(code, opt...)
		defer ssadb.DeleteProgram(consts.GetGormProjectDatabase(), programID)
		assert.Nil(t, err)
		// prog.Show()

		log.Infof("with database ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	// just use database
	{
		prog, err := ssaapi.FromDatabase(programID)
		assert.Nil(t, err)

		log.Infof("only use database ")
		err = handler(prog)
		assert.Nil(t, err)
	}
}

func CheckSyntaxFlowContain(t *testing.T, code string, sf string, wants map[string][]string, opt ...ssaapi.Option) {
	CheckSyntaxFlowEx(t, code, sf, true, wants, opt...)
}
func CheckSyntaxFlow(t *testing.T, code string, sf string, wants map[string][]string, opt ...ssaapi.Option) {
	CheckSyntaxFlowEx(t, code, sf, false, wants, opt...)
}
func CheckSyntaxFlowEx(t *testing.T, code string, sf string, contain bool, wants map[string][]string, opt ...ssaapi.Option) {
	Check(t, code, func(prog *ssaapi.Program) error {
		results, err := prog.SyntaxFlowWithError(sf)
		assert.Nil(t, err)
		for key, value := range results {
			log.Infof("\nkey: %s", key)
			value.Show()
		}

		for k, want := range wants {
			gotVs, ok := results[k]
			assert.Truef(t, ok, "key[%s] not found", k)
			got := lo.Map(gotVs, func(v *ssaapi.Value, _ int) string { return v.String() })
			sort.Strings(got)
			sort.Strings(want)
			if contain {
				if !utils.ContainsAll(got, want...) {
					t.Fatalf("\nkey[%s] \ngot[%v] \nwant[%v]", k, strings.Join(got, ","), strings.Join(want, ","))
				}
			} else {
				assert.Equal(t, len(want), len(gotVs))
				assert.Equal(t, want, got)
			}
		}
		return nil
	}, opt...)
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
