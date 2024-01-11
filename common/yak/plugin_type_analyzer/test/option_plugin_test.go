package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

func TestMitmPluginOptionValue(t *testing.T) {
	t.Run("test MITM_PLUGIN in mitm ", func(t *testing.T) {
		check(t, `
		println(MITM_PLUGIN)
		hijackSaveHTTPFlow = func(flow /* *yakit.HTTPFlow */, modify /* func(modified *yakit.HTTPFlow) */, drop/* func() */) {
			a = 1
		}
		`,
			[]string{},
			"mitm",
		)
	})

	t.Run("test MITM_PLUGIN in yak ", func(t *testing.T) {
		check(t, `
		println(MITM_PLUGIN)
		`,
			[]string{
				ssa.ValueUndefined("MITM_PLUGIN"),
			},
			"yak",
		)
	})
}

func TestPluginOptionDefineFunc(t *testing.T) {
	t.Run("test define func in mitm ", func(t *testing.T) {
		check(t, `
		hijackSaveHTTPFlow = func(flow /* *yakit.HTTPFlow */, modify /* func(modified *yakit.HTTPFlow) */, drop/* func() */) {
			responseBytes, _ = codec.StrconvUnquote(flow.Response)
			a = flow.BeforeSave() //error
		}
		`,
			[]string{
				ssa4analyze.ErrorUnhandled(), // if this exist, it means the flow has correct type
			},
			"mitm",
		)
	})
}
