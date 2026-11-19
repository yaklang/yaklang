package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestKotlinImports(t *testing.T) {
	t.Run("import declaration", func(t *testing.T) {
		checkSymbols(t, `
import kotlin.math.abs
fun main() {
    abs(-1)
}
`, "main", "abs")
	})

	t.Run("package and import", func(t *testing.T) {
		checkSymbols(t, `
package com.example
import kotlin.collections.List
fun main() {
    println(1)
}
`, "main")
	})

	t.Run("multi file import", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("util.kt", `
package util
fun helper(a: Int): Int {
    return a + 1
}
`)
		vf.AddFile("main.kt", `
package app
import util.helper
fun main() {
    helper(1)
}
`)
		test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
			require.NotEmpty(t, progs)
			prog := progs[0]
			require.NotEmpty(t, prog.Ref("main"))
			require.NotEmpty(t, prog.Ref("helper"))
			return nil
		}, ssaapi.WithLanguage(ssaconfig.KOTLIN), ssaapi.WithMemory())
	})
}
