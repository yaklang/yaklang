package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func Test_RealYak_Function(t *testing.T) {
	t.Run("object", func(t *testing.T) {
		ssatest.CheckNoError(t, `
lock := sync.NewLock()

x.Foreach([1,2], func(e){
	lock.Lock()
	println(e)
	lock.Unlock()
})
		`)
	})

	t.Run("function parameter but in loop", func(t *testing.T) {
		ssatest.CheckNoError(t, `
lock := sync.NewLock()
f = (i)=>{
    lock.Lock()
    println(i)
    lock.Unlock()
}
f(1)
for true {
    f(1)
}
`)
	})

}
func Test_RealYak_ObjectType(t *testing.T) {
	t.Run("map[string]any", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		fuzz.HTTPRequest("reqBytes")~
		exprDetails = fuzz.FuzzCalcExpr()
		result = exprDetails.result
		`)
	})
}
