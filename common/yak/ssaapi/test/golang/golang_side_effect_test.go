package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_SideEffect_Inherit(t *testing.T) {

	checkSideeffect := func(t *testing.T, values ssaapi.Values, num int) error {
		have := false
		for _, value := range values {
			fun1, ok := ssa.ToFunction(value.GetSSAInst())
			if !ok {
				continue
			}
			have = true
			funtype1, ok := ssa.ToFunctionType(fun1.Type)
			if !ok {
				t.Fatal("BUG::value is function but type not function type ")
			}
			if num != len(funtype1.SideEffects) {
				return utils.Errorf("side effect num not match, want %d, got %d", num, len(funtype1.SideEffects))
			}
		}
		if !have {
			return utils.Errorf("no function found ")
		} else {
			return nil
		}
	}

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
		require.NoError(t, checkSideeffect(t, prog.SyntaxFlow("f1 as $func").GetValues("func"), 1))
		require.NoError(t, checkSideeffect(t, prog.SyntaxFlow("f2 as $b").GetValues("b"), 1))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

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
		require.NoError(t, checkSideeffect(t, a, 1))
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")
		require.NoError(t, checkSideeffect(t, b, 1))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

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
		require.NoError(t, checkSideeffect(t, a, 1))
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")
		require.NoError(t, checkSideeffect(t, b, 0))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

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
		require.NoError(t, checkSideeffect(t, a, 2))
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")
		require.NoError(t, checkSideeffect(t, b, 1))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))
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
			"c": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
			"b": {"2"},
			"c": {"10"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
			c #-> as $c
	`, map[string][]string{
			"b": {"2"},
			"c": {"10"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	code = `package main

	func main() {
		a := 1
		{
			a = 2
			a := 3
			f1 := func() {
				a = 4
			}
			f1()
			b := a // 4
			{
				a = 5
			}
			f1()
			c := a // 4
		}
	}
	`

	t.Run("side-effect muti nesting bind", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
	`, map[string][]string{
			"b": {"4"},
			"c": {"4"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	code = `package main

	func main(){
		a := 0
		f := func() {
			if true {
				a = 2
			}else{

			}
			b := a // phi(a)[2,FreeValue-a]
		}
		a = 1
		f()
		c := a // side-effect(phi(a)[2,FreeValue-a], a)
	}
	`

	t.Run("side-effect with empty path", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
	`, map[string][]string{
			"b": {"2", "1", "true"},
			"c": {"2", "1"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	code = `package main

	func main(){
		a := 0
		f := func() {
			if true {
				a = 2
			}else{

			}
			b := a // phi(a)[2,FreeValue-a]
		}
		a := 1
		f()
		c := a // phi(a)[2,FreeValue-a]
	}
	`

	t.Run("side-effect with empty path have local", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
	`, map[string][]string{
			"b": {"2", "0", "true"},
			"c": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	code = `package main

		func main(){
		    a := 0
			f := func() {
			    if true {
			        a = 2
			    }else{
	
				}
				b := a // phi(a)[2,FreeValue-a]
			}
			a = 1
			f()
			c := a // side-effect(phi(a)[2,FreeValue-a], a)
			a = 3
			f()
			d := a // side-effect(phi(a)[2,FreeValue-a], a)
		}
	`

	t.Run("side-effect with empty path extend", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
			d #-> as $d
	`, map[string][]string{
			"b": {"1", "2", "3", "true"},
			"c": {"2", "1"},
			"d": {"2", "3"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	code = `package main

	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		{
			a := 3	 
			f2 := func() {
				if true{
					f1()
				}else{
					a = 4
				}
				b := a // phi(a)[FreeValue-a,4]
			}
			f2()
			c := a // side-effect(phi(a)[FreeValue-a,4], a)
		}
		d := a // side-effect(phi(a)[side-effect(2, a),FreeValue-a], a)
	}`

	t.Run("side-effect cross block nesting bind with phi", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
			d #-> as $d
	`, map[string][]string{
			"b": {"3", "4", "true"},
			"c": {"3", "4"},
			"d": {"1", "2"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})
}

func Test_SideEffect_Capture(t *testing.T) {
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("object member", func(t *testing.T) {
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("object member nesting", func(t *testing.T) {
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("object", func(t *testing.T) {
		code := `package main

	import "fmt"

	type T struct {
		a int
		b int
	}

	func main(){
		o := &T{a: 1, b: 2}
		f1 := func() {
			o = &T{a: 3, b: 4}
		}
		o1 := o.a
		f1()
		o2 := o.a
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			o1 #-> as $o1
			o2 #-> as $o2
		`, map[string][]string{
			"o1": {"1"},
			"o2": {"3"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
			$entry.Open()<getMembers> as $db;
			$db.* as $output;
			$output(, * as $sink);
	`, map[string][]string{
			"sink": {`"11111111111"`},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})
}

func Test_SideEffect_Type(t *testing.T) {

	code := `
package main

import (
	"test"
)

func main(){
	a = 1
	f1 := func() {
		a = test.a
	}
	println(a) // 1
	f1()
	println(a) // side-effect(3, a)
}

`
	ssatest.CheckSyntaxFlowContain(t, code, `
a as $a
$a?{<fullTypeName>?{have: 'test'}} as $output;
		`,
		map[string][]string{
			"output": {"side-effect(Undefined-a, a)", "Undefined-a"},
		}, ssaapi.WithLanguage(ssaconfig.GO),
	)
}
