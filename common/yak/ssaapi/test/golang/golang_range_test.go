package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"gotest.tools/v3/assert"
)

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
		target := prog.SyntaxFlow("println( * #-> as $target )").GetValues("target")
		a := target[0].GetSSAValue()
		b := target[1].GetSSAValue()
		if ca, ok := ssa.ToConst(a); ok {
			ra := ca.GetRange()
			assert.Equal(t, 4, ra.GetStart().GetLine())
			assert.Equal(t, 10, ra.GetStart().GetColumn())
			assert.Equal(t, 4, ra.GetEnd().GetLine())
			assert.Equal(t, 14, ra.GetEnd().GetColumn())
		}
		if cb, ok := ssa.ToConst(b); ok {
			rb := cb.GetRange()
			assert.Equal(t, 8, rb.GetStart().GetLine())
			assert.Equal(t, 10, rb.GetStart().GetColumn())
			assert.Equal(t, 8, rb.GetEnd().GetLine())
			assert.Equal(t, 15, rb.GetEnd().GetColumn())
		}

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

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
	ssatest.CheckWithName("funtion-range", t, code, func(prog *ssaapi.Program) error {
		target := prog.SyntaxFlow("test as $target1").GetValues("target1")
		target2 := prog.SyntaxFlow("test2 as $target2").GetValues("target2")
		a := target[0].GetSSAValue()
		b := target2[0].GetSSAValue()
		if ca, ok := ssa.ToFunction(a); ok {
			ra := ca.GetRange()
			assert.Equal(t, 3, ra.GetStart().GetLine())
			assert.Equal(t, 1, ra.GetStart().GetColumn())
			assert.Equal(t, 5, ra.GetEnd().GetLine())
			assert.Equal(t, 2, ra.GetEnd().GetColumn())
		}
		if cb, ok := ssa.ToFunction(b); ok {
			rb := cb.GetRange()
			assert.Equal(t, 7, rb.GetStart().GetLine())
			assert.Equal(t, 1, rb.GetStart().GetColumn())
			assert.Equal(t, 9, rb.GetEnd().GetLine())
			assert.Equal(t, 2, rb.GetEnd().GetColumn())
		}

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

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
	ssatest.CheckWithName("import-range", t, code, func(prog *ssaapi.Program) error {
		ent := prog.SyntaxFlow("ent?{<fullTypeName>?{have: 'entgo.io/ent'}} as $target;").GetValues("target")
		fmt := prog.SyntaxFlow("fmt?{<fullTypeName>?{have: 'fmt'}} as $target;").GetValues("target")
		a := ent[0].GetSSAValue()
		b := fmt[0].GetSSAValue()
		if ca, ok := ssa.ToExternLib(a); ok {
			ra := ca.GetRange()
			assert.Equal(t, 9, ra.GetStart().GetLine())
			assert.Equal(t, 2, ra.GetStart().GetColumn())
			assert.Equal(t, 9, ra.GetEnd().GetLine())
			assert.Equal(t, 16, ra.GetEnd().GetColumn())
		}
		if cb, ok := ssa.ToExternLib(b); ok {
			rb := cb.GetRange()
			assert.Equal(t, 5, rb.GetStart().GetLine())
			assert.Equal(t, 2, rb.GetStart().GetColumn())
			assert.Equal(t, 5, rb.GetEnd().GetLine())
			assert.Equal(t, 7, rb.GetEnd().GetColumn())
		}

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))
}
