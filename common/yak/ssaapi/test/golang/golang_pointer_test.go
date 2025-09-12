package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPointer_normal(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("test.go", `package main

	func main(){
		b := 1
		p := &b
		*p = 2
	}
`)

		ssatest.CheckSyntaxFlowWithFS(t, fs, `
			b as $b
		`, map[string][]string{
			"b": {"1", "2"},
		}, true, ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func TestPointer_SideEffect(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("test.go", `package main

	func pointer(a *int){
		*a = 2
	}

	func main(){
		a := 1
		pointer(&a)
	}
`)

		ssatest.CheckSyntaxFlowWithFS(t, fs, `
			a as $a
		`, map[string][]string{
			"a": {"1", "2"},
		}, true, ssaapi.WithLanguage(ssaapi.GO),
		)
	})

}
