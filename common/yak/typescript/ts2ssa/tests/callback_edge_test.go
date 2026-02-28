package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCallbackSideEffectEdgeCases(t *testing.T) {
	t.Run("single callback update flow", func(t *testing.T) {
		code := `
var handlers = {}
var on = (name, cb) => {
	handlers[name] = cb
}
var emit = (name, value) => {
	var h = handlers[name]
	if (h) {
		h(value)
	}
}
var state = 1
on("u", (n) => {state = n})
emit("u", 2)
var a = state
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"1", "2"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("branch callback keeps both candidate updates", func(t *testing.T) {
		code := `
var handlers = {}
var on = (name, cb) => {
	handlers[name] = cb
}
var emit = (name, value) => {
	var h = handlers[name]
	if (h) {
		h(value)
	}
}
var state = 1
on("u", (n) => {
	if (n > 0) {
		state = 2
	} else {
		state = 3
	}
})
emit("u", 1)
var a = state
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"1", "2", "3"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("callback alias chain invoke", func(t *testing.T) {
		code := `
var state = 1
var register = (cb) => cb
var update = (v) => { state = v }
var alias1 = register(update)
var alias2 = alias1
alias2(6)
var a = state
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"6"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("callback multi-branch multi-call chain", func(t *testing.T) {
		code := `
var handlers = {}
var on = (name, cb) => {
	handlers[name] = cb
}
var emit = (name, value) => {
	var h = handlers[name]
	if (h) {
		h(value)
	}
}
var state = 1
on("u", (n) => {
	if (n > 0) {
		state = 7
	} else {
		state = 8
	}
})
emit("u", 1)
emit("u", 0)
var a = state
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"1", "7", "8"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})
}
