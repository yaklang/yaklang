package test

import (
	"slices"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func check(code string, want []string, t *testing.T) *ssaapi.Program {
	test := assert.New(t)

	prog, err := ssaapi.Parse(code)

	test.Nil(err)

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

	return prog
}

type TestCase struct {
	code        string
	errs        []string
	ExternValue map[string]any
	ExternLib   map[string]map[string]any
}

func CheckTestCase(t *testing.T, tc TestCase) {
	opt := make([]ssaapi.Option, 0)
	for k, v := range tc.ExternLib {
		opt = append(opt, ssaapi.WithExternLib(k, v))
	}
	opt = append(opt, ssaapi.WithExternValue(tc.ExternValue))
	prog, err := ssaapi.Parse(tc.code, opt...)
	if err != nil {
		t.Fatal("failed to parse")
	}
	// prog.Show()
	// fmt.Println(prog.GetErrors().String())
	errs := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string { return e.Message })
	slices.Sort(errs)
	slices.Sort(tc.errs)
	if len(errs) != len(tc.errs) {
		t.Fatalf("error len not match %d vs %d : %s", len(errs), len(tc.errs), errs)
	}
	for i := 0; i < len(errs); i++ {
		for errs[i] != tc.errs[i] {
			t.Fatalf("error not match %s vs %s", errs[i], tc.errs[i])
		}
	}
}
