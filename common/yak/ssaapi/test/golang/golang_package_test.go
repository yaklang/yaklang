package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPackage_normol(t *testing.T) {
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

func TestPackage_lazybuild(t *testing.T) {
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

func TestPackage_muti_file_init(t *testing.T) {
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

func Test_Package_muti_file_meminit(t *testing.T) {
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

func TestFileName_muti_package(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package A

	import "fmt"

	func test(){
		fmt.Println("A")
	}

	`)
	vf.AddFile("src/main/go/B/test.go", `
	package B

	import "fmt"

	func test(){
		fmt.Println("B")
	}
	`)

	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		var as, bs string
		for _, prog := range progs {
			sf := `
					fmt?{<fullTypeName>?{have: 'fmt'}} as $entry;
					$entry.Println( * as $target);
				`
			result := prog.SyntaxFlow(sf)
			result.Show()
			target := result.GetValues("target")
			a := target[0].GetSSAValue()
			b := target[1].GetSSAValue()
			if ca, ok := ssa.ToConst(a); ok {
				ea := ca.GetRange().GetEditor()
				as = ea.GetFilename()
			}
			if cb, ok := ssa.ToConst(b); ok {
				eb := cb.GetRange().GetEditor()
				bs = eb.GetFilename()
			}
			require.NotEqual(t, as, bs)
		}

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))
}

func TestFileName_muti_file(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test1.go", `
	package A

	import "fmt"

	func test1(){
		fmt.Println("A")
	}

	`)
	vf.AddFile("src/main/go/A/test2.go", `
	package A

	import "fmt"

	func test2(){
		// padding
		fmt.Println("B")
	}
	`)

	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		var as, bs string
		for _, prog := range progs {
			sf := `
					fmt?{<fullTypeName>?{have: 'fmt'}} as $entry;
					$entry.Println( * as $target);
				`
			result := prog.SyntaxFlow(sf)
			target := result.GetValues("target")
			a := target[0].GetSSAValue()
			b := target[1].GetSSAValue()
			if ca, ok := ssa.ToConst(a); ok {
				ea := ca.GetRange().GetEditor()
				as = ea.GetFilename()
			}
			if cb, ok := ssa.ToConst(b); ok {
				eb := cb.GetRange().GetEditor()
				bs = eb.GetFilename()
			}
			require.NotEqual(t, as, bs)
		}

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))
}
