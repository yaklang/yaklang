package test

import (
	"fmt"
	"testing"

	slices "golang.org/x/exp/slices"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

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
	CheckPrintf(t, TestCase{
		code: code,
		want: want,
	})
}

func CheckPrintf(t *testing.T, tc TestCase) {
	tc.Check = func(t *testing.T, prog *ssaapi.Program, want []string) {
		test := assert.New(t)

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

type TestCase struct {
	code        string
	want        []string
	ExternValue map[string]any
	ExternLib   map[string]map[string]any
	Check       func(*testing.T, *ssaapi.Program, []string)
}

func CheckTestCase(t *testing.T, tc TestCase) {
	opt := make([]ssaapi.Option, 0)
	for k, v := range tc.ExternLib {
		opt = append(opt, ssaapi.WithExternLib(k, v))
	}
	opt = append(opt, ssaapi.WithExternValue(tc.ExternValue))
	prog, err := ssaapi.Parse(tc.code, opt...)
	if err != nil {
		t.Fatal("failed to parse: ", err)
	}

	prog.Show()
	// prog.Program.ShowWithSource()
	fmt.Println(prog.GetErrors().String())

	tc.Check(t, prog, tc.want)
}

func CheckError(t *testing.T, tc TestCase) {
	check := func(t *testing.T, prog *ssaapi.Program, want []string) {
		errs := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string { return e.Message })
		slices.Sort(errs)
		slices.Sort(want)
		if len(errs) != len(want) {
			t.Fatalf("error len not match %d vs %d : %s", len(errs), len(tc.want), errs)
		}
		for i := 0; i < len(errs); i++ {
			for errs[i] != want[i] {
				t.Fatalf("error not match %s vs %s", errs[i], tc.want[i])
			}
		}
	}
	tc.Check = check
	CheckTestCase(t, tc)
}

func CheckType(t *testing.T, code, name string, kind ssa.TypeKind) {
	test := assert.New(t)
	prog, err := ssaapi.Parse(code)
	test.Nil(err)

	prog.Show()

	vs := prog.Ref(name)
	test.Equal(1, len(vs))

	v := vs[0]
	test.NotNil(v)

	log.Info("type and kind: ", v.GetType(), v.GetTypeKind())
	test.Equal(kind, v.GetTypeKind())
}
