package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

func TestMitmPluginOption(t *testing.T) {
	t.Run("test MITM_PLUGIN in mitm ", func(t *testing.T) {
		check(t, `
		println(MITM_PLUGIN)
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
				ssa4analyze.ValueUndefined("MITM_PLUGIN"),
			},
			"yak",
		)
	})
}
