package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestRustSyntaxFlow(t *testing.T) {
	t.Run("basic dataflow reaches literal", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, rustWithPrintln(`
fn main() {
    let a = 7;
    println(a);
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"7"},
		}, ssaapi.WithLanguage(ssaconfig.RUST))
	})

	t.Run("cross function dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, rustWithPrintln(`
fn id(a: i32) -> i32 {
    a
}
fn main() {
    println(id(9));
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"9"},
		}, ssaapi.WithLanguage(ssaconfig.RUST))
	})

	t.Run("closure free value dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, rustWithPrintln(`
fn main() {
    let n = 10;
    let f = |x: i32| x + n;
    println(f(1));
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"10"},
		}, ssaapi.WithLanguage(ssaconfig.RUST))
	})

	t.Run("impl method receiver dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, rustWithPrintln(`
struct Point {
    x: i32,
}
impl Point {
    fn get(&self) -> i32 {
        self.x
    }
}
fn main() {
    let p = Point { x: 7 };
    println(p.get());
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"7"},
		}, ssaapi.WithLanguage(ssaconfig.RUST))
	})

	t.Run("struct member dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, rustWithPrintln(`
struct Point {
    x: i32,
}
fn main() {
    let p = Point { x: 42 };
    println(p.x);
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"42"},
		}, ssaapi.WithLanguage(ssaconfig.RUST))
	})
}
