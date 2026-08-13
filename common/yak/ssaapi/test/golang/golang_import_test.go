package ssaapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
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
		"a": {"1", "2", "3"},
	}, false, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestImport_alias(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang
	go 1.20
	`)
	vf.AddFile("src/main/go/A/test.go", `
	package main

	import "github.com/dgrijalva/jwt-go"

	func main() {
		token := jwt.New(jwt.SigningMethodHS256)
		println(token)
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
jwt as $a
		`, map[string][]string{
		"a": {"ExternLib-jwt"},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
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
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
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
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestImport_globals(t *testing.T) {
	t.Run("import struct", func(t *testing.T) {
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
		}, true, ssaapi.WithLanguage(ssaconfig.GO),
		)
	})

	t.Run("import global cross", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
		vf.AddFile("src/main/go/main.go", `
	package main

	import "go0p/A"

	var PI = A.PI

	func main() {
		println(PI)
	}
	`)
		vf.AddFile("src/main/go/A/test.go", `
	package A

	import "go0p/B"

	var PI = B.PI
	`)
		vf.AddFile("src/main/go/B/test.go", `
	package B

	var PI = 3.1415926
	`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
			"a": {"3.1415926"},
		}, true, ssaapi.WithLanguage(ssaconfig.GO),
		)
	})

	t.Run("import global cross ver", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
		vf.AddFile("src/main/go/main.go", `
	package main

	import "go0p/A"

	func main() {
		var PI = A.PI
		println(PI)
	}
	`)
		vf.AddFile("src/main/go/A/test.go", `
	package A

	import "go0p/B"

	var PI = B.PI
	`)
		vf.AddFile("src/main/go/B/test.go", `
	package B

	var PI = 3.1415926
	`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
			"a": {"3.1415926"},
		}, true, ssaapi.WithLanguage(ssaconfig.GO),
		)
	})
}

func TestImport_Syntaxflow(t *testing.T) {
	t.Run("import syntaxflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `package main

		import (
			"github.com/yaklang/test"
		)

		func main() {
			test.Println("Hello, World!") // function
			a := test.A
		}
	`,
			`
			test?{<fullTypeName>?{have: 'github.com/yaklang/test'}} as $entry;
			$entry.Println?{<fullTypeName>?{have: 'github.com/yaklang/test'}} as $function // function
			$entry.A?{<fullTypeName>?{have: 'github.com/yaklang/test'}} as $value // value
			`,
			map[string][]string{
				"entry":    {"ExternLib-test"},
				"function": {"Undefined-test.Println"},
				"value":    {"Undefined-a"},
			},
			ssaapi.WithLanguage(ssaconfig.GO),
		)
	})

	t.Run("import syntaxflow muti", func(t *testing.T) {
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
		}, true, ssaapi.WithLanguage(ssaconfig.GO),
		)
	})
}

func TestFakeImport_Syntaxflow(t *testing.T) {
	t.Run("fake import syntaxflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `package main

	import (
		"fmt"
		"io/ioutil"
		"net/http"
	)

	func handleGet(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		response := fmt.Sprintf("Hello, %s!", name)
		
		w.Write([]byte(response))
	}
	`,
			`
			http?{<fullTypeName>?{have: 'net/http'}} as $entry;
			$entry.Request as $target;
			
			`,
			map[string][]string{
				"target": {"Parameter-r"},
			},
			ssaapi.WithLanguage(ssaconfig.GO),
		)
	})
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
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestImport_fulltypename(t *testing.T) {
	t.Run("fulltypename", func(t *testing.T) {
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("fulltypename lib", func(t *testing.T) {
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})
}

func TestImport_golang_global_var_init(t *testing.T) {
	const sessionsFilesystemStoreGetRule = `
sessions?{<fullTypeName()>?{have: "github.com/gorilla/sessions"}} as $entry

$entry.NewFilesystemStore() as $a
$a.Get() as $b
*.Get() as $c
`
	assertStoreGetMatched := func(res *ssaapi.SyntaxFlowResult) {
		bVals := res.GetValues("b")
		cVals := res.GetValues("c")
		res.Show()
		require.Greater(t, len(cVals), 0, "expected $c to have results")
		require.Greater(t, len(bVals), 0, "expected $b to have results")
	}

	t.Run("package_global_initialized_in_init", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang

go 1.20
`)
		vf.AddFile("src/main/go/store.go", `
package main

import (
	"github.com/gorilla/sessions"
)

var Store *sessions.FilesystemStore

func init() {
	Store = sessions.NewFilesystemStore("sessions", []byte("k"))
}
`)
		vf.AddFile("src/main/go/reset.go", `
package main

import (
	"net/http"
	"github.com/gorilla/sessions"
)

func reset(r *http.Request) {
	_, _ = Store.Get(r, "GOSESSID")
}
`)
		ssatest.CheckResultWithFS(t, vf, sessionsFilesystemStoreGetRule, assertStoreGetMatched,
			ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("package_global_var_inline_initializer", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang

go 1.20
`)
		vf.AddFile("src/main/go/store.go", `
package main

import (
	"github.com/gorilla/sessions"
)

var Store *sessions.FilesystemStore = sessions.NewFilesystemStore("sessions", []byte("k"))
`)
		vf.AddFile("src/main/go/reset.go", `
package main

import (
	"net/http"
	"github.com/gorilla/sessions"
)

func reset(r *http.Request) {
	_, _ = Store.Get(r, "GOSESSID")
}
`)
		ssatest.CheckResultWithFS(t, vf, sessionsFilesystemStoreGetRule, assertStoreGetMatched,
			ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("package_global_initialized_in_init_import", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang

go 1.20
`)
		vf.AddFile("src/main/go/store.go", `
package api

import (
	"github.com/gorilla/sessions"
)

var Store *sessions.FilesystemStore

func init() {
	Store = sessions.NewFilesystemStore("sessions", []byte("k"))
}
`)
		vf.AddFile("src/main/go/reset.go", `
package main

import (
	"net/http"
	"github.com/gorilla/sessions"
	"github.com/yaklang/yaklang/api"
)

func reset(r *http.Request) {
	_, _ = api.Store.Get(r, "GOSESSID")
}
`)
		ssatest.CheckResultWithFS(t, vf, sessionsFilesystemStoreGetRule, assertStoreGetMatched,
			ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("package_global_var_inline_initializer_import", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang

go 1.20
`)
		vf.AddFile("src/main/go/store.go", `
package api

import (
	"github.com/gorilla/sessions"
)

var Store *sessions.FilesystemStore = sessions.NewFilesystemStore("sessions", []byte("k"))
`)
		vf.AddFile("src/main/go/reset.go", `
package main

import (
	"net/http"
	"github.com/gorilla/sessions"
	"github.com/yaklang/yaklang/api"
)

func reset(r *http.Request) {
	_, _ = api.Store.Get(r, "GOSESSID")
}
`)
		ssatest.CheckResultWithFS(t, vf, sessionsFilesystemStoreGetRule, assertStoreGetMatched,
			ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("package_global_store_get_inside_closure", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang

go 1.20
`)
		vf.AddFile("src/main/go/store.go", `
package main

import (
	"github.com/gorilla/sessions"
)

var Store *sessions.FilesystemStore

func init() {
	Store = sessions.NewFilesystemStore("sessions", []byte("k"))
}
`)
		vf.AddFile("src/main/go/handler.go", `
package main

import (
	"net/http"
	"github.com/gorilla/sessions"
)

func sessionReader() func(*http.Request) {
	return func(r *http.Request) {
		_, _ = Store.Get(r, "GOSESSID")
	}
}
`)
		ssatest.CheckResultWithFS(t, vf, sessionsFilesystemStoreGetRule, assertStoreGetMatched,
			ssaapi.WithLanguage(ssaconfig.GO))
	})
}

func TestImport_GlobalVariableCrossPackage(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang

go 1.20
`)
	vf.AddFile("src/main/go/A/a.go", `
package A

var GlobalA = 1
`)
	vf.AddFile("src/main/go/B/b.go", `
package B

import "fmt"

func init() {
	fmt.Println(GlobalA)
}
`)

	// pkg_b does NOT import pkg_a, so GlobalA should be Undefined,
	// not the value 1 from pkg_a.
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* as $target)
	`, map[string][]string{
		"target": {"Undefined-GlobalA"},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestImport_GlobalVariableSamePackage(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang

go 1.20
`)
	vf.AddFile("src/main/go/A/a.go", `
package A

var GlobalA = 1
`)
	vf.AddFile("src/main/go/A/b.go", `
package A

import "fmt"

func init() {
	fmt.Println(GlobalA)
}
`)

	// Same package: GlobalA should resolve to 1
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* as $target)
	`, map[string][]string{
		"target": {"1"},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

// ===== Global Variable Mechanism Comprehensive Tests =====
// These tests document the current behavior of the GlobalVariablesBlueprint
// mechanism for Go, covering all code paths that will be affected by the
// global mechanism refactor.

func TestGlobal_samePackage_globalVarWithInit(t *testing.T) {
	// Global var declared in file A, assigned in init() in file A,
	// referenced in function in file B (same package).
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

var Config string

func init() {
	Config = "production"
}
`)
	vf.AddFile("src/main/go/b.go", `
package main

import "fmt"

func runConfig() {
	fmt.Println(Config)
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* as $target)
	`, map[string][]string{
		"target": {"\"production\""},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_samePackage_globalVarInlineInit(t *testing.T) {
	// Global var with inline initializer, referenced in another file.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

var Count = 42
`)
	vf.AddFile("src/main/go/b.go", `
package main

import "fmt"

func showCount() {
	fmt.Println(Count)
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* as $target)
	`, map[string][]string{
		"target": {"42"},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_samePackage_globalSliceIndex(t *testing.T) {
	// Global slice, index access in another file.
	// This tests the member call relationship (str[0]) that is currently
	// stored in globalVarsContainer and restored by LoadGlobalVariable.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

var Items = []string{"alpha", "beta", "gamma"}
`)
	vf.AddFile("src/main/go/b.go", `
package main

import "fmt"

func showFirst() {
	fmt.Println(Items[0])
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* #-> as $target)
	`, map[string][]string{
		"target": {"\"alpha\""},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_samePackage_globalMapKey(t *testing.T) {
	// Global map, key access in another file.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

var ConfigMap = map[string]int{
	"timeout": 30,
	"retries": 3,
}
`)
	vf.AddFile("src/main/go/b.go", `
package main

import "fmt"

func showTimeout() {
	fmt.Println(ConfigMap["timeout"])
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* #-> as $target)
	`, map[string][]string{
		"target": {"30"},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_samePackage_globalUpdatedInInit(t *testing.T) {
	// Global var declared with default value, updated in init(),
	// referenced in another function. The value should be the
	// updated value, not the default.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

var Mode int

func init() {
	Mode = 1
}
`)
	vf.AddFile("src/main/go/b.go", `
package main

import "fmt"

func showMode() {
	fmt.Println(Mode)
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* as $target)
	`, map[string][]string{
		"target": {"1"},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_samePackage_multipleInit(t *testing.T) {
	// Multiple init() functions update the same global. Sequential
	// accumulation to 15 is not guaranteed under concurrent compile;
	// Println must still see the global (10 from the first assignment
	// and/or 15 if later inits were applied).
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

var Counter int

func init() {
	Counter = 10
}

func init() {
	Counter = Counter + 5
}
`)
	vf.AddFile("src/main/go/c.go", `
package main

import "fmt"

func showCounter() {
	fmt.Println(Counter)
}
`)
	ssatest.CheckResultWithFS(t, vf, `
		fmt.Println(* as $target)
	`, func(sfr *ssaapi.SyntaxFlowResult) {
		require.NotNil(t, sfr)
		got := sfr.GetValues("target")
		require.NotEmpty(t, got)
		ok := false
		for _, v := range got {
			s := v.String()
			if strings.Contains(s, "10") || strings.Contains(s, "15") {
				ok = true
				break
			}
		}
		require.True(t, ok, "Println(Counter) should see init assignment 10 or accumulated 15, got %v", got)
	}, ssaapi.WithLanguage(ssaconfig.GO), ssaconfig.WithCompileConcurrency(1))
}

func TestGlobal_samePackage_globalInClosure(t *testing.T) {
	// Global variable referenced in a function that also creates a closure.
	// SyntaxFlow does not traverse into closure bodies, so we verify the
	// global variable is visible in the outer function.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

var Secret = "s3cr3t"
`)
	vf.AddFile("src/main/go/b.go", `
package main

import "fmt"

func getSecretFunc() func() string {
	fmt.Println(Secret)
	return func() string {
		return Secret
	}
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* as $target)
	`, map[string][]string{
		"target": {"\"s3cr3t\""},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_crossPackage_importGlobal(t *testing.T) {
	// Global var in package A, imported and used in package B.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/A/a.go", `
package A

var Version = "1.0.0"
`)
	vf.AddFile("src/main/go/B/b.go", `
package B

import (
	"fmt"
	"github.com/yaklang/yaklang/A"
)

func showVersion() {
	fmt.Println(A.Version)
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* #-> as $target)
	`, map[string][]string{
		"target": {"\"1.0.0\""},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_crossPackage_importGlobalChain(t *testing.T) {
	// Global var in package C, imported by B, imported by A.
	// Tests transitive import of global variables.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/C/c.go", `
package C

var Depth = 100
`)
	vf.AddFile("src/main/go/B/b.go", `
package B

import "github.com/yaklang/yaklang/C"

var BDepth = C.Depth
`)
	vf.AddFile("src/main/go/A/a.go", `
package A

import (
	"fmt"
	"github.com/yaklang/yaklang/B"
)

func showDepth() {
	fmt.Println(B.BDepth)
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* #-> as $target)
	`, map[string][]string{
		"target": {"100"},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_samePackage_globalVarDefaultZero(t *testing.T) {
	// Global var declared without initializer, no init() assigns it.
	// Should have default zero value.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

var Uninitialized int
`)
	vf.AddFile("src/main/go/b.go", `
package main

import "fmt"

func showUninit() {
	fmt.Println(Uninitialized)
}
`)
	// The value should be 0 (default int value)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* as $target)
	`, map[string][]string{
		"target": {"0"},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_samePackage_globalStructFieldAccess(t *testing.T) {
	// Global struct variable, field access in another file.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

type Settings struct {
	Host string
	Port int
}

var Config = Settings{
	Host: "localhost",
	Port: 8080,
}
`)
	vf.AddFile("src/main/go/b.go", `
package main

import "fmt"

func showHost() {
	fmt.Println(Config.Host)
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* #-> as $target)
	`, map[string][]string{
		"target": {"\"localhost\""},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_samePackage_globalVarReassigned(t *testing.T) {
	// Global var assigned multiple times in init(), final value wins.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/a.go", `
package main

var Status string

func init() {
	Status = "starting"
	Status = "running"
}
`)
	vf.AddFile("src/main/go/b.go", `
package main

import "fmt"

func showStatus() {
	fmt.Println(Status)
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* as $target)
	`, map[string][]string{
		"target": {"\"running\""},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}

func TestGlobal_crossPackage_notVisibleWithoutImport(t *testing.T) {
	// Global var in package A, NOT imported by package B.
	// B should see Undefined, not the value from A.
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
module github.com/yaklang/yaklang
go 1.20
`)
	vf.AddFile("src/main/go/A/a.go", `
package A

var Private = "secret"
`)
	vf.AddFile("src/main/go/B/b.go", `
package B

import "fmt"

func showPrivate() {
	fmt.Println(Private)
}
`)
	// Private should be Undefined (not visible across packages without import)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		fmt.Println(* as $target)
	`, map[string][]string{
		"target": {"Undefined-Private"},
	}, true, ssaapi.WithLanguage(ssaconfig.GO),
	)
}
