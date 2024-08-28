package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Import(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	type A1 struct {
	    a int
	}

	func (a *A1) get() int {
	    return a.a
	}
	`)
	vf.AddFile("src/main/go/B/test.go", `
	package B

	import "github.com/yaklang/yaklang/A"

	func println(){}

	func test() {
	    a := &A.A1{a: 1}
	    println(a.get())
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"1"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestImport_alias(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	func add(a,b int) int {
	    return a + b + 3
	}
	`)
	vf.AddFile("src/main/go/B/test.go", `
	package B

	import alias "github.com/yaklang/yaklang/A"

	func println(){}

	func test() {
	    println(alias.add(1,2))
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"1", "2", "3"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestImport_muti(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	type A1 struct {
	    a int
	}

	func (a *A1) get() int {
	    return a.a
	}
	`)
	vf.AddFile("src/main/go/B/test.go", `
	package B

	func add(a,b int) int {
	    return a + b
	}
	`)
	vf.AddFile("src/main/go/C/test.go", `
	package C

	import (
		"github.com/yaklang/yaklang/A"
		"github.com/yaklang/yaklang/B"
	)

	func println(){}

	func test() {
	    a := &A.A1{a: 1}
	    println(B.add(2,a.get()))
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"1", "2"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}
