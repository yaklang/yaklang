package test

import (
	"testing"

)

func Test_ObjectFactor_FunctionSideEffect(t *testing.T) {
	t.Run("f modify parameter", func(t *testing.T) {
		checkPrintlnValue(`
		f = (arg) => {
			arg["b"] = 1 
		} 
		a = {}
		f(a)
		println(a.b)
		`, []string{
			"side-effect(1, #0.b)",
		}, t)
	})

}

