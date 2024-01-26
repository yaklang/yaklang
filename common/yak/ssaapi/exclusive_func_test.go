package ssaapi

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/dot"
	"testing"
)

func TestFunctionTrace(t *testing.T) {
	prog, err := Parse(`c =((i,i1)=>i)(1,2);a = {};a.b=c;e=a.b;dump(e)`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Ref("e").GetTopDefs().Show()
}

func TestFunctionTrace_FormalParametersCheck(t *testing.T) {
	prog, err := Parse(`
a = 1
b = (c, d) => {
	a = c + d
	return d, c
}
f = b(2,3)
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()

	check2 := false
	check3 := false
	prog.Ref("f").Show().ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			if value.IsConstInst() {
				if value.GetConstValue() == 2 {
					check2 = true
				}
				if value.GetConstValue() == 3 {
					check3 = true
				}
			}
		})
	})

	if !check2 {
		t.Fatal("the literal 2 trace failed")
	}
	if !check3 {
		t.Fatal("the literal 3 trace failed")
	}
}

func TestFunctionTrace_FormalParametersCheck_2(t *testing.T) {
	prog, err := Parse(`
a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()

	check2 := false
	check3 := false
	noCheck4 := true
	prog.Ref("f").Show().ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			d := value.Dot()
			_ = d
			value.ShowDot()
			if value.IsConstInst() {
				if value.GetConstValue() == 2 {
					check2 = true
				}
				if value.GetConstValue() == 3 {
					check3 = true
				}
				if value.GetConstValue() == 4 {
					noCheck4 = false
				}
			}
		})
	})

	if !noCheck4 {
		t.Fatal("literal 4 should not be traced")
	}

	if !check2 {
		t.Fatal("the literal 2 trace failed")
	}
	if !check3 {
		t.Fatal("the literal 3 trace failed")
	}
}

func TestDepthLimit(t *testing.T) {
	prog, err := Parse(`var a;
b = a+1
c = b + e;
d = c + f;
g = d
`)
	if err != nil {
		t.Fatal(err)
	}

	depth2check := false
	depthAllcheck := false
	prog.Ref("g").ForEach(func(value *Value) {
		var count int
		value.GetTopDefs(WithDepthLimit(2)).ForEach(func(value *Value) {
			count++
		})
		if count == 2 {
			depth2check = true
		}

		count = 0
		value.GetTopDefs(WithMaxDepth(0)).ForEach(func(value *Value) {
			count++
		})
		if count == 4 {
			depthAllcheck = true
		}
	})

	if !depth2check {
		t.Fatal("depth2check failed")
	}

	if !depthAllcheck {
		t.Fatal("depthAllcheck failed")
	}
}

func TestDominatorTree(t *testing.T) {
	prog, err := Parse(`var a;
b = a+1
c = b + e;
d = c + f;
g = d
`)
	if err != nil {
		t.Fatal(err)
	}

	depth2check := false
	depthAllcheck := false
	prog.Ref("g").ForEach(func(value *Value) {
		var count int
		value.GetTopDefs(WithDepthLimit(2)).ForEach(func(value *Value) {
			count++
			value.Show()
		})
		if count == 2 {
			depth2check = true
		}

		count = 0
		value.GetTopDefs(WithMaxDepth(0)).ForEach(func(value *Value) {
			count++
		})
		if count == 4 {
			depthAllcheck = true
		}
	})

	if !depth2check {
		t.Fatal("depth2check failed")
	}

	if !depthAllcheck {
		t.Fatal("depthAllcheck failed")
	}
}

func TestBottomUse(t *testing.T) {
	prog, err := Parse(`var a;
b = a+1
c = b + e;
d = c + f;	
`)
	if err != nil {
		t.Fatal(err)
	}
	checkAdef := false
	prog.Ref("a").GetBottomUses().ForEach(func(value *Value) {
		if value.GetDepth() == 3 {
			checkAdef = true
		}
	}).FullUseDefChain(func(value *Value) {
		dot.ShowDotGraphToAsciiArt(value.Dot())
	})
	if !checkAdef {
		t.Fatal("checkAdef failed")
	}
}

func TestBottomUse_Func(t *testing.T) {
	prog, err := Parse(`var a;
b = (i, j) => i
c = b(a,2)
e = c + 3
`)
	if err != nil {
		t.Fatal(err)
	}
	checkAdef := false
	prog.Ref("a").GetBottomUses().ForEach(func(value *Value) {
		spew.Dump(value.GetDepth())
	}).FullUseDefChain(func(value *Value) {
		value.ShowDot()
		dot.ShowDotGraphToAsciiArt(value.Dot())
	})
	if !checkAdef {
		t.Fatal("checkAdef failed")
	}

}
