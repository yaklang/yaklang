package ssaapi

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func check[T ssa.Instruction](t *testing.T, gots ssaapi.Values, want []string, Cover func(ssa.Instruction) (T, bool)) {
	gotString := make([]string, 0, len(gots))
	for _, got := range gots {
		t, ok := Cover(got.GetSSAInst())
		if ok {
			gotString = append(gotString, t.GetRange().String())
		}
	}
	slices.Sort(want)
	slices.Sort(gotString)
	require.Equal(t, want, gotString)
}

func TestRange_normol(t *testing.T) {
	code := `package main
		
	func test() bool{
		return true
	}

	func test2() bool{
		return false
	}

	func main(){
		a := test()
		b := test2()
		println(a)
		println(b)
	}
`
	ssatest.CheckWithName("range", t, code, func(prog *ssaapi.Program) error {
		res := prog.SyntaxFlow("println( * #-> as $target )")
		res.Show()
		targets := res.GetValues("target")
		want := []string{
			"4:10 - 4:14: true",
			"8:10 - 8:15: false",
		}
		check(t, targets, want, ssa.ToConstInst)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

}

func TestRange_function(t *testing.T) {
	code := `package main
		
func test() bool{
	return true
}

func test2() bool{
	return false
}

func main(){
	a := test()
	b := test2()
	println(a)
	println(b)
}
`

	t.Run("check target1 ", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			target := prog.SyntaxFlow("test as $target1").GetValues("target1")
			check(t, target, []string{
				"3:1 - 5:2: func test() bool{\n\treturn true\n}",
			}, ssa.ToFunction)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("check target2 ", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			target2 := prog.SyntaxFlow("test2 as $target2").GetValues("target2")
			check(t, target2, []string{
				"7:1 - 9:2: func test2() bool{\n\treturn false\n}",
			}, ssa.ToFunction)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})
}
func TestRange_import(t *testing.T) {
	code := `package test

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"entgo.io/ent"
	_ "github.com/go-sql-driver/mysql"
)

func login(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	// 连接到数据库
	client, err := ent.Open("mysql", "user:password@tcp(localhost:3306)/dbname")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// 不安全的查询
	input := fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", username)
	ctx := context.Background()

	users, err := client.User.Query().Where(user.Name(input)).All(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// 打印结果
	for _, user := range users {
		fmt.Printf("User: %s, Age: %d\n", user.Name, user.Age)
	}
}

`

	t.Run("test ent", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			ent := prog.SyntaxFlow("ent?{<fullTypeName>?{have: 'entgo.io/ent'}} as $target;").GetValues("target")
			check(t, ent, []string{"9:2 - 9:16: \"entgo.io/ent\""}, ssa.ToExternLib)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})
	t.Run("test ent2", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			fmt := prog.SyntaxFlow("fmt?{<fullTypeName>?{have: 'fmt'}} as $target;").GetValues("target")
			check(t, fmt, []string{"5:2 - 5:7: \"fmt\""}, ssa.ToExternLib)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})
}
