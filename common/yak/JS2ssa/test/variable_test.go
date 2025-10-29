package test

import (
	"testing"

	"golang.org/x/exp/slices"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func Test_JS_Variable(t *testing.T) {

	check := func(code string, want []string, t *testing.T) {
		test := assert.New(t)

		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.JS))
		test.Nil(err)
		prog.Show()

		got := lo.Map(
			prog.Ref("target").Show(),
			func(v *ssaapi.Value, _ int) string { return v.String() },
		)

		slices.Sort(got)
		slices.Sort(want)

		test.Equal(want, got)
	}

	t.Run("var variable", func(t *testing.T) {
		check(`
		var a = 1 
		target = a
		`, []string{
			"1",
		}, t)
	})
}
