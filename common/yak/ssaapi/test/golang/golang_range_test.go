package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"gotest.tools/v3/assert"
)

func Test_Range(t *testing.T) {
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
		prog.Show()
		target := prog.SyntaxFlow("println( * #-> as $target )").GetValues("target")
		target.Show()
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
