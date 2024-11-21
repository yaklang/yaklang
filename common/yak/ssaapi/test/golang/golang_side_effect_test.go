package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"gotest.tools/v3/assert"
)

func Test_SideEffect_Inherit(t *testing.T) {
	code := `package main

	func main() {
		a := 0
		f1 := func() {
			a = 1
		}
		f2 := func() {
			f1()
		}
		f2()
	}
`
	ssatest.CheckWithNameOnlyInMemory("side-effect inherit: f1->f2", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype1, ok := ssa.ToFunctionType(fun1.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype2, ok := ssa.ToFunctionType(fun2.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype2.SideEffects))
		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

	code = `package main

	func main() {
		a := 0
		f2 := func() {
			f1 := func() {
				a = 1
			}
			f1()
		}
		f2()
	}
`
	ssatest.CheckWithNameOnlyInMemory("side-effect inherit lower: f1->f2", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype1, ok := ssa.ToFunctionType(fun1.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype2, ok := ssa.ToFunctionType(fun2.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype2.SideEffects))
		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

	code = `package main

	func main() {
		a := 0
		f2 := func() {
			a := 0
			f1 := func() {
				a = 1
			}
			f1()
		}
		f2()
	}
`
	ssatest.CheckWithNameOnlyInMemory("side-effect inherit stop", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype1, ok := ssa.ToFunctionType(fun1.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype2, ok := ssa.ToFunctionType(fun2.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 0, len(funtype2.SideEffects))
		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

	code = `package main

	func main() {
		a := 0
		b := 0
		f2 := func() {
			a := 0
			f1 := func() {
				a = 1
				b = 1
			}
			f1()
		}
		f2()
	}
`
	ssatest.CheckWithNameOnlyInMemory("side-effect inherit or inherit stop", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype1, ok := ssa.ToFunctionType(fun1.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 2, len(funtype1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		funtype2, ok := ssa.ToFunctionType(fun2.Type)
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(funtype2.SideEffects))
		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))
}

func Test_SideEffect_Bind(t *testing.T) {
	code := `package main

	func main() {
		a := 1
		f1 := func() {
			a = 10
		}
		{
			a := 2
			f2 := func() {
				a = 20
			}

			f2()
			b := a // 20
		}
		c := a // 1
	}`

	t.Run("side-effect bind", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
	`, map[string][]string{
			"b": {"20"},
			"c": {"1", "10"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	code = `package main

	func main() {
		a := 1
		f1 := func() {
			a = 10
		}
		{
			a := 2
			f2 := func() {
				a = 20
			}
			f3 := func() {
			    f1()
			}

			f1()
			b := a // 2
		}
		c := a // 10
	}`

	t.Run("side-effect lower bind", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
	`, map[string][]string{
			"b": {"2", "20"},
			"c": {"10"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	code = `package main

	func main() {
		a := 1
		f1 := func() {
			a = 10
		}
		{
			a := 2
			f2 := func() {
				a = 20
			}
			f3 := func() {
			    f1()
			}

			f3()
			b := a // 2
		}
		c := a // 10
	}`

	t.Run("side-effect nesting bind", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c  #->as $c
	`, map[string][]string{
			"b": {"2", "20"},
			"c": {"10"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}

func Test_Captured_SideEffect(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		code := `package main

	import "fmt"

	func test() {
		a := 1
		f := func() {
			a = 2
		}
		f()

		c := a // 2 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			c #-> as $c
		`, map[string][]string{
			"c": {"2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("normal nesting", func(t *testing.T) {
		code := `package main

	import "fmt"

	func test() {
		a := 1
		f := func() {
			a = 3
		}
		{
			a := 2
			f()
			b := a // 2 不会被side-effect影响
		}
		c := a // 3 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
		`, map[string][]string{
			"b": {"2"},
			"c": {"3"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("object", func(t *testing.T) {
		code := `package main

	import "fmt"

	type T struct {
	    a int
	}

	func test() {
		t := T{1}
		f := func() {
			t.a = 2
		}
		f()
		c := t.a // 2 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			c #-> as $c
		`, map[string][]string{
			"c": {"2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("object nesting", func(t *testing.T) {
		code := `package main

	import "fmt"

	type T struct {
	    a int
	}

	func test() {
		t := T{1}
		f := func() {
			t.a = 3
		}
		{
			t := T{2}
			f()
			b := t.a // 2 不会被side-effect影响
		}

		c := t.a // 3 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
		`, map[string][]string{
			"b": {"2"},
			"c": {"3"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("method", func(t *testing.T) {
		code := `package main

	import "fmt"

	func (t *T)setA(a int) {
	    t.a = a
	}

	type T struct {
	    a int
	}

	func test() {
		t := T{1}
		t.setA(2) // 2 会被side-effect影响

		c := t.a
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			c #-> as $c
		`, map[string][]string{
			"c": {"2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("method nesting", func(t *testing.T) {
		code := `package main

	import "fmt"

	func (t *T)setA(a int) {
	    t.a = a
	}

	type T struct {
	    a int
	}

	func test() {
		t := T{1}
		f := func() {
			t.setA(3)
		}
		{
			t := T{2}
			f()
			b := t.a // 2 不会被side-effect影响
		}


		c := t.a // 3 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
		`, map[string][]string{
			"b": {"2"},
			"c": {"3"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("closu", func(t *testing.T) {
		code := `package main

	import "fmt"


	func test() {
		t := 1

		f := func(){
		    t = 2
		}

		f2 := func(){
			t := 3
		    f()
			b := t // 3 不会被side-effect影响
		}

		f2()
		c := t // 2 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
		`, map[string][]string{
			"b": {"3"},
			"c": {"2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("closu not find", func(t *testing.T) {
		code := `package main

	import "fmt"


	func test() {
		t := 1

		f := func(){
		    t = 2
		}

		f2 := func(){
		    f()
			b := t // 2 会被side-effect影响
		}

		f2()
		c := t // 2 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
		`, map[string][]string{
			"b": {"2"},
			"c": {"2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
	t.Run("closu find but not modify", func(t *testing.T) {
		code := `package main

	import "fmt"


	func test() {
		t := 1

		f := func(){
		    t = 2
		}

		{
			t := 3
			f2 := func(){
				f()
				b := t // 3 不会被side-effect影响
			}

			f2()
			c := t // 3 会被side-effect影响
		}
		d := t // 2 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
			d #-> as $d
		`, map[string][]string{
			"b": {"3"},
			"c": {"3"},
			"d": {"2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}

func Test_SideEffect_Syntaxflow(t *testing.T) {
	code := `package example

import (
	"flag"
	"log"

	"database/sql"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	var db *sql.DB

	opendb := func() {
		db, _ = sql.Open("mysql","root:root@tcp(127.0.0.1:3306)/test")
	}

	router := gin.Default()
	router.GET("/inject", func(ctx *gin.Context) {
		opendb()
		db.Query("11111111111") // db为side-effect，syntaxflow中应该能识别并查找到
	})
	router.Run(Addr)
}
`

	t.Run("value can find side-effect member", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			sql?{<fullTypeName>?{have: 'database/sql'}} as $entry;
			$entry.Open <getCall> as $db;
			$db <getMembers> as $output;
			$output.Query as $query;
	`, map[string][]string{
			"query": {""},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}
