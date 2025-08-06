package ssaapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestFullChain(t *testing.T) {
	prog, err := ssaapi.Parse(`a = b+c
d = a + e
f = d + g
h = f + i
j = h + k
l = j + m
n = l + o
p = n + q
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()

	prog.Ref("h").FullUseDefChain(func(value *ssaapi.Value) {
		value.ShowDot()
	})
	prog.Ref("p").ForEach(func(value *ssaapi.Value) {
		ssaapi.FullUseDefChain(value)
	})
}

func TestChain_Basic(t *testing.T) {
	prog, err := ssaapi.Parse(`a=b+c;d=e(a);`)
	if err != nil {
		t.Fatal(err)
	}
	ds := prog.Ref("d")
	require.Lenf(t, ds, 1, "d should be 1, but got %d", len(ds))
	d := prog.Ref("d")[0]
	d.GetTopDefs()

	test := assert.New(t)
	test.Equal(2, d.GetDependOnCount())
	d.ForEachDependOn(func(n *ssaapi.Value) {
		if n.GetName() == "e" {
			test.Equal(0, n.GetDependOnCount())
		} else {
			test.Equal(2, n.GetDependOnCount())
		}
	})
}

func TestChain_Basic2(t *testing.T) {
	prog, err := ssaapi.Parse(`a=b+c;d=e+a;`)
	if err != nil {
		t.Fatal(err)
	}
	d := prog.Ref("d")[0]
	d.GetTopDefs()

	test := assert.New(t)
	test.Equal(2, d.GetDependOnCount())
	d.ForEachDependOn(func(n *ssaapi.Value) {
		if n.GetName() == "e" {
			test.Equal(0, n.GetDependOnCount())
		} else {
			test.Equal(2, n.GetDependOnCount())
		}
	})
}

func TestChain_Phi_If(t *testing.T) {
	prog, err := ssaapi.Parse(`
d = 1
a=b+c;
if(a){
	d=e
}else{
	d=f
};

g=d+a;`)
	if err != nil {
		t.Fatal(err)
	}

	checkPhi := false
	// checkDotPhi := false
	prog.Show()
	prog.Ref("g").FullUseDefChain(func(value *ssaapi.Value) {
		value.ForEachDependOn(func(value *ssaapi.Value) {
			if value.IsPhi() {
				checkPhi = true
			}
		})
		if strings.Contains(value.DotGraph(), `phi`) {
			// checkDotPhi = true
		}
		dot.ShowDotGraphToAsciiArt(value.DotGraph())
	})
	if !checkPhi {
		t.Fatal("checkPhi failed")
	}
	// if !checkDotPhi {
	// 	t.Fatal("checkDotPhi failed")
	// }

	/*
		          +------------------+
		          |        e         |
		          +------------------+
		            ^
		            |
		            |
		+---+     +------------------+     +-----------------+
		| f | <-- |     [phi]: d     |     |        b        |
		+---+     +------------------+     +-----------------+
		            ^                        ^
		            |                        |
		            |                        |
		          +------------------+     +-----------------+     +---+
		          | t9: g=add(d, t2) | --> | t2: a=add(b, c) | --> | c |
		          +------------------+     +-----------------+     +---+
	*/
}

func TestChain_Phi_ForSelfSpin(t *testing.T) {
	prog, err := ssaapi.Parse(`a=b+c;
for i=0;i<10;i++ {
	a = a + i
}
g=d+a;`)
	if err != nil {
		t.Fatal(err)
	}

	checkPhi := false
	checkDotPhi := false
	prog.Show()
	prog.Ref("g").FullUseDefChain(func(value *ssaapi.Value) {
		value.ForEachDependOn(func(value *ssaapi.Value) {
			if value.IsPhi() {
				checkPhi = true
			}
		})
		if strings.Contains(value.DotGraph(), `phi`) {
			checkDotPhi = true
		}
		value.ShowDot()
		dot.ShowDotGraphToAsciiArt(value.DotGraph())
	})
	if !checkPhi {
		t.Fatal("checkPhi failed")
	}
	if !checkDotPhi {
		t.Fatal("checkDotPhi failed")
	}
}

func TestFunctionDotTrace(t *testing.T) {
	text := `a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)`
	prog, err := ssaapi.Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	check2 := false
	check3 := false
	prog.Ref("f").FullUseDefChain(func(value *ssaapi.Value) {
		value.ShowDot()
		dot.ShowDotGraphToAsciiArt(value.DotGraph())
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			ret := value.GetConstValue()
			if ret == 2 && value.EffectOn.Count() == 1 {
				check2 = true
			}
			if ret == 3 && value.EffectOn.Count() == 1 {
				check3 = true
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

func TestPathTrace(t *testing.T) {
	text := `a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)`
	prog, err := ssaapi.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	var checkC, checkD bool
	var checkA bool
	prog.Ref("f").FullUseDefChain(func(value *ssaapi.Value) {
		value.ShowDot()
		var results = prog.Ref("c")
		results = append(results, prog.Ref("d")...)
		results = append(results, value)
		ret := ssaapi.FindStrictCommonDepends(results)
		if len(ret) != 2 {
			t.Fatal("the literal 2 trace failed")
		}
		ret.ForEach(func(value *ssaapi.Value) {
			if value.GetName() == "c" {
				checkC = true
			}
			if value.GetName() == "d" {
				checkD = true
			}
		})
	})
	_ = checkA
	if !checkC {
		t.Fatal("the literal 2 trace failed")
	}

	if !checkD {
		t.Fatal("the literal 2 trace failed")
	}
}

func TestPathTrace_Negative(t *testing.T) {
	text := `a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)`
	prog, err := ssaapi.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	var checkC, checkD bool = true, true
	var checkA bool
	prog.Ref("f").FullUseDefChain(func(value *ssaapi.Value) {
		value.ShowDot()
		var results = prog.Ref("c")
		results = append(results, prog.Ref("d")...)
		ret := ssaapi.FindStrictCommonDepends(results)
		if len(ret) != 0 {
			t.Fatal("b & c is not common depends")
		}
		ret.ForEach(func(value *ssaapi.Value) {
			if value.GetName() == "c" {
				checkC = false
			}
			if value.GetName() == "d" {
				checkD = false
			}
		})
	})
	_ = checkA
	if !checkC {
		t.Fatal("the literal 2 trace failed")
	}

	if !checkD {
		t.Fatal("the literal 2 trace failed")
	}
}

func TestPathTrace_Flexible(t *testing.T) {
	text := `a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)`
	prog, err := ssaapi.Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	results := ssaapi.FindFlexibleCommonDepends(append(prog.Ref("a"), prog.Ref("f")...))
	if len(results) <= 0 {
		t.Fatal("f & a is common depends (flexible)")
	}
}

func TestPathTrace_Flexible2(t *testing.T) {
	text := `a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)`
	prog, err := ssaapi.Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	results := ssaapi.FindFlexibleCommonDepends(append(prog.Ref("c"), prog.Ref("d")...)).Show()
	if len(results) > 0 {
		t.Fatal("common depends (flexible) check failed")
	}
}

func TestPathTrace_Flexible_Positive(t *testing.T) {
	text := `a = 1
b = (c, d, e) => {
	c = c + d
	return d, c
}
f = b(2,3,4)`
	prog, err := ssaapi.Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	results := ssaapi.FindFlexibleCommonDepends(append(prog.Ref("c"), prog.Ref("d")...)).Show()
	if len(results) <= 0 {
		t.Fatal("common depends (flexible) check failed")
	}
}

func TestPathTrace_Strict(t *testing.T) {
	text := `a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)`
	prog, err := ssaapi.Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	results := ssaapi.FindStrictCommonDepends(append(prog.Ref("c"), prog.Ref("d")...))
	if len(results) > 0 {
		t.Fatal("common depends (flexible)")
	}
}

func TestPathTrace_FlexibleDepends(t *testing.T) {
	t.Skip()
	text := `a = 1
b = a + c
d = b + e
f = d + h
z = f + g
y = x + z
`
	prog, err := ssaapi.Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	vals := prog.Ref("d").FlexibleDepends().ShowDot()
	var d string
	vals.ForEach(func(value *ssaapi.Value) {
		d = value.DotGraph()
	})
	if !utils.MatchAllOfSubString(d, `label="c"`, `label="e"`, `label="h"`, `label="g"`, `label="x"`) {
		t.Fatal("not flexible depends")
	}
}
