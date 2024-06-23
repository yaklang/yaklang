package ssaapi

import (
	"regexp"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"

	"github.com/yaklang/yaklang/common/utils/dot"
)

func Test_Function_Parameter(t *testing.T) {
	t.Run("multiple parameter", func(t *testing.T) {
		code := `
	f = (i,i1)=>i
	c = f(1,2);
	a = {};
	a.b=c;
	e=a.b;
	dump(e)
	`
		ssatest.Check(t, code,
			ssatest.CheckTopDef_Equal("e", []string{"1"}),
		)
	})
}

func Test_Function_Return(t *testing.T) {
	t.Run("multiple return first", func(t *testing.T) {
		ssatest.Check(t, `
		c = () => {return 1,2}; 
		a,b=c();
		`,
			ssatest.CheckTopDef_Equal("a", []string{"1"}),
		)
	})

	t.Run("multiple return second", func(t *testing.T) {
		ssatest.Check(t, `
		c = () => {return 1,2}
		a,b=c();
		`,
			ssatest.CheckTopDef_Equal("b", []string{"2"}),
		)
	})

	t.Run("multiple return unpack", func(t *testing.T) {
		ssatest.Check(t, `
		c = () => {return 1,2}
		f=c();
		a,b=f;
		dump(b)
		`,
			ssatest.CheckTopDef_Equal("b", []string{"2"}),
		)
	})
}

func Test_Function_FreeValue(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.Check(t, `
a = 1
b = (c, d) => {
	a = c + d
	return d, c
}
f = b(2,3)
		`, ssatest.CheckTopDef_Equal("f", []string{"2", "3"}))
	})

}

func TestFunctionTrace_FormalParametersCheck_2(t *testing.T) {
	prog, err := ssaapi.Parse(`
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
	prog.Ref("f").Show().ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			d := value.DotGraph()
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
	prog, err := ssaapi.Parse(`var a;
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
	prog.Ref("g").ForEach(func(value *ssaapi.Value) {
		var count int
		value.GetTopDefs(ssaapi.WithDepthLimit(2)).Show().ForEach(func(value *ssaapi.Value) {
			count++
		})
		if count == 1 {
			depth2check = true
		}

		count = 0
		value.GetTopDefs(ssaapi.WithMaxDepth(0)).ForEach(func(value *ssaapi.Value) {
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
	prog, err := ssaapi.Parse(`var a;
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
	prog.Ref("g").ForEach(func(value *ssaapi.Value) {
		var count int
		value.GetTopDefs(ssaapi.WithDepthLimit(2)).ForEach(func(value *ssaapi.Value) {
			count++
			value.Show()
		})
		if count == 1 {
			depth2check = true
		}

		count = 0
		value.GetTopDefs(ssaapi.WithMaxDepth(0)).ForEach(func(value *ssaapi.Value) {
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
	prog, err := ssaapi.Parse(`var a;
b = a+1
c = b + e;
d = c + f;	
`)
	if err != nil {
		t.Fatal(err)
	}
	checkAdef := false
	prog.Ref("a").GetBottomUses().ForEach(func(value *ssaapi.Value) {
		if value.GetDepth() == 3 {
			checkAdef = true
		}
	}).FullUseDefChain(func(value *ssaapi.Value) {
		dot.ShowDotGraphToAsciiArt(value.DotGraph())
	})
	if !checkAdef {
		t.Fatal("checkAdef failed")
	}
}

func TestBottomUse_Func(t *testing.T) {
	prog, err := ssaapi.Parse(`var a;
b = (i, j) => i
c = b(a,2)
e = c + 3
`)
	if err != nil {
		t.Fatal(err)
	}
	var vals string
	prog.Ref("a").GetBottomUses().ForEach(func(value *ssaapi.Value) {
		value.ShowDot()
		vals = value.DotGraph()
	})
	var count = 0
	regexp.MustCompile(`n\d -> n\d `).ReplaceAllStringFunc(vals, func(s string) string {
		count++
		return s
	})
	if count < 5 {
		t.Fatal("count edge failed")
	}
}

func TestBottomUse_ReturnUnpack(t *testing.T) {
	prog, err := ssaapi.Parse(`a = (i, j, k) => {
	return i, j, k
}
c,d,e = a(f,2,3);
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	vals := prog.Ref("f").GetBottomUses()
	if len(vals) != 1 {
		t.Fatal("bottom use failed")
	}
	vals.Show()
	var cId int64 = -1
	prog.Ref("c").Show().ForEach(func(value *ssaapi.Value) {
		cId = value.GetId()
	})
	if ret := vals[0].GetId(); ret != cId {
		t.Fatalf("bottom use failed: expect: %v got: %v", cId, ret)
	}
}

func TestBottomUse_ReturnUnpack2(t *testing.T) {
	prog, err := ssaapi.Parse(`a = (i, j, k) => {
	return i, i+1, k
}
c,d,e = a(f,2,3);
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	vals := prog.Ref("f").GetBottomUses()
	if len(vals) != 2 {
		t.Fatal("bottom use failed")
	}
	vals.Show()
}
