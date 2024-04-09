package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestNegativeCallStack_Basic(t *testing.T) {
	prog, err := ssaapi.Parse(`
a = () => {
	b = dddd;
	return b
}

c = a()
f = call(c)
e = f + 1
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	bRef := prog.Ref("b")
	result := bRef.GetBottomUses()[0]
	if !result.IsBinOp() {
		t.Fatal("expect binop, got " + result.String())
	}
}

func TestNegativeCallStack_Basic2(t *testing.T) {
	prog, err := ssaapi.Parse(`
a = () => {
	b = dddd;
	return b
}

c = a()
f = call(c)
e = f + 1

h = a()("abc")
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	bRef := prog.Ref("b")
	result := bRef.GetBottomUses()
	checkAdd := false
	checkCall := false
	result.ForEach(func(va *ssaapi.Value) {
		if va.IsBinOp() {
			checkAdd = true
		}
		if va.IsCall() {
			checkCall = true
		}
	})
	if !checkAdd {
		t.Fatal("expect add, got " + result.String())
	}
	if !checkCall {
		t.Fatal("expect call, got " + result.String())
	}
}
