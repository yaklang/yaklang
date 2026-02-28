package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestTSClosureSideEffectMatrix(t *testing.T) {
	t.Run("basic closure invoke updates captured variable", func(t *testing.T) {
		code := `
var state = 1
var update = () => { state = 2 }
update()
var a = state
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"2"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("boundary closure not invoked keeps original candidate", func(t *testing.T) {
		code := `
var state = 1
var update = () => { state = 2 }
var a = state
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"1", "2"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("complex nested closure branch and multi-call", func(t *testing.T) {
		code := `
var state = 1
var factory = () => {
	return (x) => {
		if (x > 0) {
			state = 3
		} else {
			state = 4
		}
	}
}
var update = factory()
update(1)
update(0)
var a = state
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"3", "4"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("boundary closure alias chain invoke", func(t *testing.T) {
		code := `
var state = 1
var f = () => { state = 6 }
var g = f
var h = g
h()
var a = state
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"6"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("complex rest-parameter callback with branch and multi-call", func(t *testing.T) {
		code := `
var state = 1
var run = (fn, ...values) => {
	fn(values[0])
	fn(values[1])
}
var update = (v) => {
	if (v > 0) {
		state = 7
	} else {
		state = 8
	}
}
run(update, 1, 0)
var a = state
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"0", "0", "1", "7", "8"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})
}
