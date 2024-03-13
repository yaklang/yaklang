package test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func GetFrontValueByOffset(prog *ssaapi.Program, offset int64) *ssaapi.Value {
	vs := prog.GetFrontValueByOffset(offset)
	if len(vs) == 0 {
		return nil
	}
	newVs := lo.Filter(vs, func(v *ssaapi.Value, index int) bool {
		return !v.IsUndefined()
	})
	if len(newVs) > 0 {
		return newVs[0]
	}
	return vs[0]
}

func Test_Yaklang_range(t *testing.T) {
	check := func(t *testing.T, code, want string, offset int64) {
		ssatest.CheckTestCase(t, ssatest.TestCase{
			Code: code,
			Want: []string{want},
			Check: func(prog *ssaapi.Program, want []string) {
				// spew.Dump(prog.Program.OffsetSegmentToValues)
				value := GetFrontValueByOffset(prog, offset)
				require.NotNil(t, value)
				require.Equal(t, want[0], value.GetVerboseName())
			},
		})
	}
	t.Run("value", func(t *testing.T) {
		check(t, `
			a = 1
			`,
			"1",
			9)
	})

	t.Run("variable", func(t *testing.T) {
		check(t, `
			a = 1
			`,
			"1",
			6)
	})

	t.Run("function", func(t *testing.T) {
		check(t, `
			a = 1
			println(a)
			`,
			"println",
			13)
	})

	t.Run("member call", func(t *testing.T) {
		check(t, `
			a = 1
			b = {"c": () => 1}
			b.c()
			`,
			"b.c",
			50)
	})

	t.Run("cli", func(t *testing.T) {
		check(t, `cli.`,
			"1",
			4)
	})
}
