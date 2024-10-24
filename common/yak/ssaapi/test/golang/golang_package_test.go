package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Package(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test1.go", `
	package A

	func add(a,b int) int {
	    return a + b + 3
	}
	`)
	vf.AddFile("src/main/go/A/test2.go", `
	package A

	func println(){}

	func test() {
	    a := add(1,2)
	    println(a)
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"1", "2", "3"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func Test_Package_lazybuild(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test1.go", `
	package A

	func println(){}

	func test() {
	    a := add(1,2)
	    println(a)
	}
	`)
	vf.AddFile("src/main/go/A/test2.go", `
	package A

	func add(a,b int) int {
	    return a + b + 3
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"3", "1", "2"},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func Test_Package_mutifile_init(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test1.go", `
	package A

	func init() int { // 特殊函数init可能会导致当前block提前finish
		return 0
	}
	`)
	vf.AddFile("src/main/go/A/test2.go", `
	package A

	var str = []string{
		"hello world",
	}

	func main() {
		for true {
			if true {

			}else{
				println(str[0])
			}
		}
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"\"hello world\""},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}

func Test_Package_mutifile_meminit(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test1.go", `
	package A

	type T struct {
	    
	}

	func (a *T) init() int { // 特殊函数init可能会导致当前block提前finish
		return 0
	}
	`)
	vf.AddFile("src/main/go/A/test2.go", `
	package A

	var str = []string{
		"hello world",
	}

	func main() {
		for true {
			if true {

			}else{
				println(str[0])
			}
		}
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"\"hello world\""},
	}, true, ssaapi.WithLanguage(ssaapi.GO),
	)
}
