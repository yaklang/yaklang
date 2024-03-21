package ssatest

import (
	"fmt"
	"testing"

	"golang.org/x/exp/slices"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
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
}

var languageOption ssaapi.Option = nil
var language ssaapi.Language

func SetLanguage(language ssaapi.Language, build ssaapi.Build) {
	ssaapi.LanguageBuilders[language] = build
	languageOption = ssaapi.WithLanguage(language)
	language = language
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
	prog, err := ssaapi.Parse(tc.Code, opt...)
	require.Nil(t, err, "parse error")

	prog.Show()
	// prog.Program.ShowWithSource()
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

func CheckPrintf(t *testing.T, tc TestCase) {
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

		require.Equal(t, want, got)
	}
	CheckTestCase(t, tc)
}

func CheckNoError(t *testing.T, code string) {
	tc := TestCase{
		Code: code,
		Check: func(prog *ssaapi.Program, _ []string) {
			require.Len(t, prog.GetErrors(), 0, "error not match")
		},
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

func CheckType(t *testing.T, code string, kind ssa.TypeKind) {
	tc := TestCase{
		Code: code,
		Check: func(prog *ssaapi.Program, _ []string) {
			vs := prog.Ref("target")
			require.Len(t, vs, 1)

			v := vs[0]
			require.NotNil(t, v)

			log.Info("type and kind: ", v.GetType(), v.GetTypeKind())
			require.Equal(t, kind, v.GetTypeKind())
		},
	}
	CheckTestCase(t, tc)
}

func CheckMask(t *testing.T, tc TestCase) {
	tc.Check = func(p *ssaapi.Program, want []string) {
		targets := p.Ref("target").ShowWithSource()
		require.Len(t, targets, 1)

		target := targets[0]

		v := ssaapi.GetBareNode(target)
		require.NotNil(t, v)

		// test.Equal("1", v.String())

		maskV, ok := v.(ssa.Maskable)
		require.True(t, ok)

		maskValues := maskV.GetMask()
		log.Infof("mask values: %s", maskValues)

		require.Equal(t, want, lo.Map(maskValues, func(v ssa.Value, _ int) string { return ssa.LineDisasm(v) }))
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

		parameters := lo.Map(funTyp.ParameterValue, func(v *ssa.Parameter, _ int) string { return v.GetName() })
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
