package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestRustMultiFileProject(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.rs", `
fn main() {
    let value = helper(1);
    println(value);
}
`)
	vf.AddFile("helper.rs", `
fn helper(a: i32) -> i32 {
    a + 1
}
`)
	test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		require.NotEmpty(t, progs)
		prog := progs[0]
		require.NotEmpty(t, prog.Ref("main"))
		require.NotEmpty(t, prog.Ref("helper"))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.RUST), ssaapi.WithMemory())
}

func TestRustProjectLocal(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("app.rs", `
fn id(a: i32) -> i32 {
    a
}
fn main() {
    id(1);
}
`)
	test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		require.NotEmpty(t, progs)
		require.NotEmpty(t, progs[0].Ref("id"))
		require.NotEmpty(t, progs[0].Ref("main"))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.RUST), ssaapi.WithMemory())
}
