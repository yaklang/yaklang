package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func Test_Yaklang_range(t *testing.T) {
	check := func(t *testing.T, code, want string, offset int64) {
		ssatest.CheckTestCase(t, ssatest.TestCase{
			Code: code,
			Want: []string{want},
			Check: func(prog *ssaapi.Program, want []string) {
				value := prog.GetFrontValueByOffset(offset)
				require.NotNil(t, value)
				require.Equal(t, want[0], value.String())
			},
		})
	}
	t.Run("normal", func(t *testing.T) {
		// ssatest.CheckTestCase(t, ssatest.TestCase{
		// 	Code: `
		check(t, `
			a = 1
			`,
			"1",
			9)
	})

	t.Run("normal variable", func(t *testing.T) {
		check(t, `
			a = 1
			`,
			"1",
			6)
	})

}
