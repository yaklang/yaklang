package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestImport_struct(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	type A1 struct {
		str string
		arr []int
		mp map[string]int
	}

	`)
	vf.AddFile("src/main/go/B/test.go", `
	package B

	import "github.com/yaklang/yaklang/A"

	func test() {
		a := &A.A1{
			str: "hello world",
			arr: []int{1, 2, 3, 4},
			mp: map[string]int{
				"hello": 1,
				"world": 2,
			},
		}

	    println(a.str)
		println(a.arr[0])
		println(a.mp["world"])
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"\"hello world\"", "1", "2"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestImport_function(t *testing.T) {
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

	func test() {
	    println(alias.add(1,2))
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"1", "2"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestImport_method(t *testing.T) {
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

func TestImport_aliastyp(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	type Myint int
	`)

	vf.AddFile("src/main/go/B/test.go", `
	package B

	import (
		"github.com/yaklang/yaklang/A"
	)

	func test() {
		var a A.Myint = 1
		println(a)
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"1"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestImport_globals(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	var Mymap map[string]int = map[string]int{
		"hello":  1,
		"world":  2,
		"golang": 3,
	}

	var Mystring string = "hello world"

	var Myarray []int = []int{1, 2, 3, 4, 5}

	`)

	vf.AddFile("src/main/go/B/test.go", `
	package B

	import (
		"github.com/yaklang/yaklang/A"
	)

	func test() {
		println(A.Mymap["hello"])
		println(A.Mystring)
		println(A.Myarray[2])
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"1", "\"hello world\"", "3"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestImport_syntaxflow(t *testing.T) {
	t.Run("temp", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `package main

		import (
			"fmt"
		)

		func main() {
			fmt.Println("Hello, World!")
		}

	`,
			`fmt.Println(* #-> as $a)`,
			map[string][]string{
				"a": {"\"Hello, World!\""},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func TestImport_syntaxflow_muti(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	func function(a int) int {
	    return a
	}
	`)
	vf.AddFile("src/main/go/B/test.go", `
	package B

	import alias "github.com/yaklang/yaklang/A"

	func test() {
	   	alias.function(1)
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		alias.function(* #-> as $a)
		`, map[string][]string{
		"a": {"1"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestImport_unorder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	import "github.com/yaklang/yaklang/B"

	func test() {
		a := &B.B1{
			str: "hello world",
			arr: []int{1, 2, 3, 4},
			mp: map[string]int{
				"hello": 1,
				"world": 2,
			},
		}

	    println(a.str)
		println(a.arr[0])
		println(a.mp["world"])
	}
	`)
	vf.AddFile("src/main/go/B/test.go", `
	package B

	type B1 struct {
		str string
		arr []int
		mp map[string]int
	}

	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"\"hello world\"", "1", "2"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestImport_fulltypename(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	func add(a,b int) int {
	    return a + b
	}
	`)
	vf.AddFile("src/main/go/B/test.go", `
	package B

	import "github.com/yaklang/yaklang/A"

	func test() {
	    println(A.add(1,2))
	}
	`)

	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		have := prog.SyntaxFlowChain(`A?{<fullTypeName>?{have: 'github.com/yaklang/yaklang/A'}} as $have;`).Show()
		nothave := prog.SyntaxFlowChain(`A?{<fullTypeName>?{have: 'github1.com/yaklang/yaklang/A'}} as $nothave;`).Show()
		assert.GreaterOrEqual(t, have.Len(), 1)
		assert.GreaterOrEqual(t, nothave.Len(), 0)
		return nil
	}, ssaapi.WithLanguage(consts.GO))
}

func TestImport_fulltypename_lib(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	import "github.com/stretchr/testify/assert"

	func main() int {
	    assert.GreaterOrEqual(t, have.Len(), 1)
	}
	`)

	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		have := prog.SyntaxFlowChain(`assert?{<fullTypeName>?{have: 'github.com/stretchr/testify/assert'}} as $have;`).Show()
		nothave := prog.SyntaxFlowChain(`assert?{<fullTypeName>?{have: 'github1.com/stretchr/testify/assert'}} as $nothave;`).Show()
		assert.GreaterOrEqual(t, have.Len(), 1)
		assert.GreaterOrEqual(t, nothave.Len(), 0)
		return nil
	}, ssaapi.WithLanguage(consts.GO))
}
