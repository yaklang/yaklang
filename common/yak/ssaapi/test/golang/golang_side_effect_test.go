package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"gotest.tools/v3/assert"
)

func Test_Closu_DoubleSideEffect(t *testing.T) {
	code := `package main

func main() {
	var a = 0
	f1 := func() {
		a = 1
	}
	f2 := func() {
	    f1()
	}
	f2()
}
`
	ssatest.CheckWithName("side-effect: f1->f2", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun2.SideEffects))

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

}

func Test_Closu_DoubleSideEffect_lower(t *testing.T) {
	code := `package main

func main() {
	var a = 0
	f2 := func() {
		f1 := func() {
			a = 1
		}
	    f1()
	}
	f2()
}
`
	ssatest.CheckWithName("side-effect: f1->f2", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun2.SideEffects))

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

}

func Test_Closu_SideEffect_syntaxflow(t *testing.T) {
	code := `package example

import (
	"flag"
	"log"

	"database/sql"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)


func main() {
	db := function1()

	router := gin.Default()
	router.GET("/inject", func(ctx *gin.Context) {
		db = function2()
		db.Query("11111111111") // db为side-effect，syntaxflow中应该能识别并查找到
	})
	router.Run(Addr)
}
`

	t.Run("side-effect bind syntaxflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			function1() as $output;
			$output.Query as $query;
	`, map[string][]string{
			"query": {""},
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
			"c": {"1", "3"},
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
		c := t.a // 0 会被side-effect影响
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
		{
			t2 := t
			t := T{2}
			t2.setA(3) 
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
}
