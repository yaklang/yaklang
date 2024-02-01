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

func checkPrintlnValue(code string, want []string, t *testing.T) *ssaapi.Program {
	var p *ssaapi.Program
	CheckTestCase(t, TestCase{
		code: code,
		want: want,
		Check: func(t *testing.T, prog *ssaapi.Program) {
			p = prog
			test := assert.New(t)
			prog.Show()

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
		},
	})

	return p
}

type TestCase struct {
	code        string
	want        []string
	ExternValue map[string]any
	ExternLib   map[string]map[string]any
	Check       func(*testing.T, *ssaapi.Program)
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
	fmt.Println(prog.GetErrors().String())

	tc.Check(t, prog)
}

func CheckError(t *testing.T, tc TestCase) {
	check := func(t *testing.T, prog *ssaapi.Program) {
		errs := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string { return e.Message })
		slices.Sort(errs)
		slices.Sort(tc.want)
		if len(errs) != len(tc.want) {
			t.Fatalf("error len not match %d vs %d : %s", len(errs), len(tc.want), errs)
		}
		for i := 0; i < len(errs); i++ {
			for errs[i] != tc.want[i] {
				t.Fatalf("error not match %s vs %s", errs[i], tc.want[i])
			}
		}
	}
	tc.Check = check
	CheckTestCase(t, tc)
}
