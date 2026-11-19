package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestRustUseAndModules(t *testing.T) {
	t.Run("use declaration", func(t *testing.T) {
		checkSymbols(t, `
use std::collections::HashMap;
fn main() {
    let map = HashMap::new();
}
`, "main")
	})

	t.Run("inline module function", func(t *testing.T) {
		checkSymbols(t, `
mod util {
    pub fn add(a: i32, b: i32) -> i32 {
        a + b
    }
}
fn main() {
    add(1, 2);
}
`, "main", "add")
	})

	t.Run("module file declaration", func(t *testing.T) {
		checkSymbols(t, `
mod util;
fn main() {
    util::add(1, 2);
}
`, "main")
	})

	t.Run("multi file module", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("util.rs", `
pub fn add(a: i32, b: i32) -> i32 {
    a + b
}
`)
		vf.AddFile("main.rs", `
mod util;
fn main() {
    helper();
}
`)
		test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
			require.NotEmpty(t, progs)
			prog := progs[0]
			require.NotEmpty(t, prog.Ref("main"))
			require.NotEmpty(t, prog.Ref("add"))
			return nil
		}, ssaapi.WithLanguage(ssaconfig.RUST), ssaapi.WithMemory())
	})
}
