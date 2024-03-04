package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"testing"
)

func TestNegativeCallStack_Basic(t *testing.T) {
	prog, err := Parse(`
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
	if _, ok := result.node.(*ssa.BinOp); !ok {
		t.Fatal("expect binop, got " + result.String())
	}
}

func TestNegativeCallStack_Basic2(t *testing.T) {
	prog, err := Parse(`
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
	result.ForEach(func(va *Value) {
		_, ok := va.node.(*ssa.BinOp)
		if ok {
			checkAdd = true
		}
		_, ok = va.node.(*ssa.Call)
		if ok {
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
