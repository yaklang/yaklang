package test

import (
	"fmt"
	"testing"

	"golang.org/x/exp/slices"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

// ===================== test case ====================
type TestCase struct {
	code        string
	want        []string
	ExternValue map[string]any
	ExternLib   map[string]map[string]any
	Check       func(*assert.Assertions, *ssaapi.Program, []string)
}

func CheckTestCase(t *testing.T, tc TestCase) {
	test := assert.New(t)

	opt := make([]ssaapi.Option, 0)
	for k, v := range tc.ExternLib {
		opt = append(opt, ssaapi.WithExternLib(k, v))
	}
	opt = append(opt, ssaapi.WithExternValue(tc.ExternValue))
	opt = append(opt, static_analyzer.GetPluginSSAOpt("yak")...)
	prog, err := ssaapi.Parse(tc.code, opt...)
	test.Nil(err, "parse error")

	prog.Show()
	// prog.Program.ShowWithSource()
	fmt.Println(prog.GetErrors().String())

	if tc.want == nil {
		tc.want = make([]string, 0)
	}
	tc.Check(test, prog, tc.want)
}

// ===================== struct =====================
type ExampleInterface interface {
	ExampleMethod()
}

type ExampleStruct struct {
	ExampleFieldFunction func()
}

func (a ExampleStruct) ExampleMethod() {}

func getExampleStruct() ExampleStruct {
	return ExampleStruct{}
}

func getExampleInterface() ExampleInterface {
	return ExampleStruct{}
}

// --------------------- for test ---------------------
func checkPrintlnValue(code string, want []string, t *testing.T) {
	checkPrintf(t, TestCase{
		code: code,
		want: want,
	})
}

func checkPrintf(t *testing.T, tc TestCase) {
	tc.Check = func(test *assert.Assertions, prog *ssaapi.Program, want []string) {
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

		test.Equal(want, got)

	}
	CheckTestCase(t, tc)
}

func checkError(t *testing.T, tc TestCase) {
	check := func(test *assert.Assertions, prog *ssaapi.Program, want []string) {
		errs := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string { return e.Message })
		slices.Sort(errs)
		slices.Sort(want)
		test.Len(errs, len(want), "error len not match")
		test.Equal(want, errs, "error not match")
	}
	tc.Check = check
	CheckTestCase(t, tc)
}

func checkType(t *testing.T, code string, kind ssa.TypeKind) {
	tc := TestCase{
		code: code,
		Check: func(test *assert.Assertions, prog *ssaapi.Program, _ []string) {
			vs := prog.Ref("target")
			test.Equal(1, len(vs))

			v := vs[0]
			test.NotNil(v)

			log.Info("type and kind: ", v.GetType(), v.GetTypeKind())
			test.Equal(kind, v.GetTypeKind())
		},
	}
	CheckTestCase(t, tc)
}

func checkMask(t *testing.T, tc TestCase) {
	tc.Check = func(test *assert.Assertions, p *ssaapi.Program, want []string) {
		targets := p.Ref("target").ShowWithSource()
		test.Len(targets, 1)

		target := targets[0]

		v := ssaapi.GetBareNode(target)
		test.NotNil(v)

		// test.Equal("1", v.String())

		maskV, ok := v.(ssa.Maskable)
		test.True(ok)

		maskValues := maskV.GetMask()
		log.Infof("mask values: %s", maskValues)

		test.Equal(want, lo.Map(maskValues, func(v ssa.Value, _ int) string { return ssa.LineDisasm(v) }))
	}
	CheckTestCase(t, tc)
}

func checkFreeValue(t *testing.T, tc TestCase) {
	tc.Check = func(test *assert.Assertions, p *ssaapi.Program, want []string) {
		targets := p.Ref("target").ShowWithSource()
		test.Len(targets, 1)

		target := targets[0]

		typ := ssaapi.GetBareType(target.GetType())
		test.Equal(ssa.FunctionTypeKind, typ.GetTypeKind())

		funTyp, ok := ssa.ToFunctionType(typ)
		test.True(ok)

		freeValues := lo.Map(funTyp.FreeValue, func(v *ssa.FunctionFreeValue, _ int) string { return v.Name })
		slices.Sort(freeValues)
		test.Equal(want, freeValues)
	}
	CheckTestCase(t, tc)
}
