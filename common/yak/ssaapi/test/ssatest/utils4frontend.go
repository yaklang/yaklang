package ssatest

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/consts"

	"golang.org/x/exp/slices"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

// ===================== test case ====================
type TestCase struct {
	Code        string
	Want        []string
	ExternValue map[string]any
	ExternLib   map[string]map[string]any
	Check       func(*ssaapi.Program, []string)
	Option      []ssaapi.Option
}

var (
	languageOption ssaapi.Option = nil
	language       consts.Language
)

func SetLanguage(lang consts.Language, build ...ssa.CreateBuilder) {
	if len(build) > 0 {
		ssaapi.LanguageBuilders[lang] = build[0]
	}
	languageOption = ssaapi.WithLanguage(lang)
	language = lang
}

func CheckTestCase(t *testing.T, tc TestCase) {
	opt := make([]ssaapi.Option, 0)
	for k, v := range tc.ExternLib {
		opt = append(opt, ssaapi.WithExternLib(k, v))
	}

	if languageOption != nil {
		opt = append(opt, languageOption)
	}
	opt = append(opt, ssaapi.WithExternValue(tc.ExternValue))
	opt = append(opt, static_analyzer.GetPluginSSAOpt(string(language))...)
	opt = append(opt, tc.Option...)
	prog, err := ssaapi.Parse(tc.Code, opt...)
	require.Nil(t, err, "parse error")

	prog.Show()
	fmt.Println(prog.GetErrors().String())

	if tc.Want == nil {
		tc.Want = make([]string, 0)
	}
	tc.Check(prog, tc.Want)
}

func MockSSA(t *testing.T, src string) {
	tc := TestCase{
		Code: src,
		Check: func(prog *ssaapi.Program, w []string) {
			err := lo.Filter(prog.GetErrors(), func(err *ssa.SSAError, index int) bool {
				if err.Kind != ssa.Error {
					return false
				}
				// if strings.HasPrefix(err.Message, "Value undefined") {
				// 	return false
				// }
				return true
			})
			require.Len(t, err, 0, "error not match")
		},
	}
	CheckTestCase(t, tc)
}
func NonStrictMockSSA(t *testing.T, code string) {
	tc := TestCase{
		Code: code,
		Check: func(program *ssaapi.Program, strings []string) {
			program.Show()
		},
	}
	CheckTestCase(t, tc)
}

// ===================== struct =====================
type ExampleInterface interface {
	ExampleMethod()
}

type ExampleStruct struct {
	ExampleFieldFunction func()
}

func (a ExampleStruct) ExampleMethod() {}

func GetExampleStruct() ExampleStruct {
	return ExampleStruct{}
}

func GetExampleInterface() ExampleInterface {
	return ExampleStruct{}
}

// --------------------- for test ---------------------
func CheckPrintlnValue(code string, want []string, t *testing.T) {
	CheckPrintf(t, TestCase{
		Code: code,
		Want: want,
	})
}

func CheckPrintlnValueContain(code string, want []string, t *testing.T) {
	CheckPrintf(t, TestCase{
		Code: code,
		Want: want,
	}, true)
}

func CheckPrintf(t *testing.T, tc TestCase, contains ...bool) {
	contain := false
	if len(contains) > 0 {
		contain = contains[0]
	}
	tc.Check = func(prog *ssaapi.Program, want []string) {
		println := prog.Ref("println").ShowWithSource()
		// test.Equal(1, len(println), "println should only 1")
		got := lo.Map(
			println.GetUsers().ShowWithSource().Flat(func(v *ssaapi.Value) ssaapi.Values {
				return ssaapi.Values{v.GetOperand(1)}
			}),
			func(v *ssaapi.Value, _ int) string {
				return v.String()
			},
		)
		// sort.Strings(got)
		log.Info("got :", got)
		// sort.Strings(want)
		log.Info("want :", want)

		equalSlices := func(a, b []string) {
			// Sort both slices
			sort.Strings(a)
			sort.Strings(b)

			if contain {
				for _, containSubStr := range want {
					match := false
					// should contain at least one
					for _, g := range got {
						if strings.Contains(g, containSubStr) {
							match = true
						}
					}
					if !match {
						t.Errorf("want[%s] not found in got[%v]", want, got)
					}
				}
			} else {
				// Compare the sorted slices
				require.Equal(t, a, b)
			}
		}

		equalSlices(want, got)
	}
	CheckTestCase(t, tc)
}

func CheckParse(t *testing.T, code string, opt ...ssaapi.Option) {
	tc := TestCase{
		Code:   code,
		Check:  func(prog *ssaapi.Program, _ []string) {},
		Option: opt,
	}
	CheckTestCase(t, tc)
}

func CheckNoError(t *testing.T, code string, opt ...ssaapi.Option) {
	tc := TestCase{
		Code: code,
		Check: func(prog *ssaapi.Program, _ []string) {
			require.Len(t, prog.GetErrors(), 0, "error not match")
		},
		Option: opt,
	}
	CheckTestCase(t, tc)
}

func CheckError(t *testing.T, tc TestCase) {
	check := func(prog *ssaapi.Program, want []string) {
		errs := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string { return e.Message })
		slices.Sort(errs)
		slices.Sort(want)
		require.Len(t, errs, len(want), "error len not match")
		require.Equal(t, want, errs, "error not match")
	}
	tc.Check = check
	CheckTestCase(t, tc)
}

func CheckTypeKind(t *testing.T, code string, kind ssa.TypeKind, opt ...ssaapi.Option) {
	opt = append(opt, static_analyzer.GetPluginSSAOpt(string(language))...)
	Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		match := false
		prog.Ref("target").Show().ForEach(func(v *ssaapi.Value) {
			log.Info("type and kind: ", v.GetType(), v.GetTypeKind())
			if kind == v.GetTypeKind() {
				match = true
			}
			// require.Equal(t, kind, v.GetTypeKind())
		})
		require.True(t, match, "type kind not match, want %v", kind)
		return nil
	}, opt...)
}

func CheckType(t *testing.T, code string, typ ssa.Type, opt ...ssaapi.Option) {
	tc := TestCase{
		Code: code,
		Check: func(prog *ssaapi.Program, _ []string) {
			vs := prog.Ref("target")
			require.Len(t, vs, 1)

			v := vs[0]
			require.NotNil(t, v)

			log.Info("type and kind: ", v.GetType(), v.GetTypeKind())
			require.Truef(t, ssa.TypeEqual(ssaapi.GetBareType(v.GetType()), typ), "want %s, got %s", typ, v.GetType())
		},
		Option: opt,
	}
	CheckTestCase(t, tc)
}

func CheckTypeEx(t *testing.T, code string, typCallback func(*ssaapi.Program) *ssaapi.Type, opt ...ssaapi.Option) {
	tc := TestCase{
		Code: code,
		Check: func(prog *ssaapi.Program, _ []string) {
			vs := prog.Ref("target")
			require.Len(t, vs, 1)

			v := vs[0]
			require.NotNil(t, v)
			typ := ssaapi.GetBareType(typCallback(prog))

			log.Info("type and kind: ", v.GetType(), v.GetTypeKind())
			require.Truef(t, ssa.TypeEqual(ssaapi.GetBareType(v.GetType()), typ), "want %s, got %s", typ, v.GetType())
		},
		Option: opt,
	}
	CheckTestCase(t, tc)
}

func CheckMask(t *testing.T, tc TestCase) {
	tc.Check = func(p *ssaapi.Program, want []string) {
		targets := p.Ref("target").ShowWithSource()
		require.Len(t, targets, 1)

		target := targets[0]

		// v := ssaapi.GetBareNode(target)
		// require.NotNil(t, v)

		// test.Equal("1", v.String())
		inst := target.GetSSAInst()
		require.NotNil(t, inst)

		maskV, ok := inst.(ssa.Maskable)
		require.True(t, ok)

		maskValues := maskV.GetMask()
		log.Infof("mask values: %s", maskValues)

		require.Equal(t, want, lo.Map(maskValues, func(v ssa.Value, _ int) string { return ssa.LineDisASM(v) }))
	}
	CheckTestCase(t, tc)
}

func CheckFreeValue(t *testing.T, tc TestCase) {
	tc.Check = func(p *ssaapi.Program, want []string) {
		targets := p.Ref("target").ShowWithSource()
		require.Len(t, targets, 1, "target len not match")

		target := targets[0]

		typ := ssaapi.GetBareType(target.GetType())
		require.Equal(t, ssa.FunctionTypeKind, typ.GetTypeKind())

		funTyp, ok := ssa.ToFunctionType(typ)
		require.True(t, ok)

		freeValues := lo.Map(funTyp.FreeValue, func(v *ssa.Parameter, _ int) string { return v.GetName() })
		slices.Sort(freeValues)
		require.Equal(t, want, freeValues)
	}
	CheckTestCase(t, tc)
}

func CheckParameter(t *testing.T, tc TestCase) {
	tc.Check = func(p *ssaapi.Program, want []string) {
		targets := p.Ref("target").ShowWithSource()
		require.Len(t, targets, 1, "target len not match")

		target := targets[0]

		typ := ssaapi.GetBareType(target.GetType())
		require.Equal(t, ssa.FunctionTypeKind, typ.GetTypeKind())

		funTyp, ok := ssa.ToFunctionType(typ)
		require.True(t, ok)

		parameters := lo.Map(funTyp.ParameterValue, func(v *ssa.Parameter, _ int) string { return v.GetVerboseName() })
		require.Equal(t, want, parameters)
	}
	CheckTestCase(t, tc)
}

func CheckParameterMember(t *testing.T, tc TestCase) {
	tc.Check = func(p *ssaapi.Program, want []string) {
		targets := p.Ref("target").ShowWithSource()
		require.Len(t, targets, 1, "target len not match")

		target := targets[0]

		typ := ssaapi.GetBareType(target.GetType())
		require.Equal(t, ssa.FunctionTypeKind, typ.GetTypeKind())

		funTyp, ok := ssa.ToFunctionType(typ)
		require.True(t, ok)

		parameters := lo.Map(funTyp.ParameterMember, func(v *ssa.ParameterMember, _ int) string { return v.String() })
		require.Equal(t, want, parameters)
	}
	CheckTestCase(t, tc)
}

func CheckFunctionReturnType(t *testing.T, code string, kind ssa.TypeKind) {
	tc := TestCase{
		Code: code,
		Check: func(prog *ssaapi.Program, _ []string) {
			vs := prog.Ref("target")
			require.Equal(t, 1, len(vs))

			v := vs[0]
			require.NotNil(t, v)

			typ := v.GetType()

			rawTyp := ssaapi.GetBareType(typ)
			funTyp, ok := ssa.ToFunctionType(rawTyp)

			require.True(t, ok)

			retType := funTyp.ReturnType
			require.NotNil(t, retType)

			log.Info("return type : ", retType)
			require.Equal(t, kind, retType.GetTypeKind())
		},
	}
	CheckTestCase(t, tc)
}
