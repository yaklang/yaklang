package ssaapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
)

func TestFullChain(t *testing.T) {
	prog, err := Parse(`a = b+c
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

	prog.Ref("h").FullUseDefChain(func(value *Value) {
		value.ShowDot()
	})
	prog.Ref("p").ForEach(func(value *Value) {
		FullUseDefChain(value)
	})
}

func TestChain_Basic(t *testing.T) {
	prog, err := Parse(`a=b+c;d=e(a);`)
	if err != nil {
		t.Fatal(err)
	}
	d := prog.Ref("d")[0]
	d.GetTopDefs()

	test := assert.New(t)
	test.Equal(2, len(d.DependOn))
	for _, n := range d.DependOn {
		if n.GetName() == "e" {
			test.Equal(0, len(n.DependOn))
		} else {
			test.Equal(2, len(n.DependOn))
		}
	}
}

func TestChain_Basic2(t *testing.T) {
	prog, err := Parse(`a=b+c;d=e+a;`)
	if err != nil {
		t.Fatal(err)
	}
	d := prog.Ref("d")[0]
	d.GetTopDefs()

	test := assert.New(t)
	test.Equal(2, len(d.DependOn))
	for _, n := range d.DependOn {
		if n.GetName() == "e" {
			test.Equal(0, len(n.DependOn))
		} else {
			test.Equal(2, len(n.DependOn))
		}
	}
}

func TestChain_Phi_If(t *testing.T) {
	prog, err := Parse(`
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
	checkDotPhi := false
	prog.Show()
	prog.Ref("g").FullUseDefChain(func(value *Value) {
		value.DependOn.ForEach(func(value *Value) {
			if value.IsPhi() {
				checkPhi = true
			}
		})
		if strings.Contains(value.Dot(), `phi`) {
			checkDotPhi = true
		}
		dot.ShowDotGraphToAsciiArt(value.Dot())
	})
	if !checkPhi {
		t.Fatal("checkPhi failed")
	}
	if !checkDotPhi {
		t.Fatal("checkDotPhi failed")
	}

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
	prog, err := Parse(`a=b+c;
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
	prog.Ref("g").FullUseDefChain(func(value *Value) {
		value.DependOn.ForEach(func(value *Value) {
			if value.IsPhi() {
				checkPhi = true
			}
		})
		if strings.Contains(value.Dot(), `phi`) {
			checkDotPhi = true
		}
		value.ShowDot()
		dot.ShowDotGraphToAsciiArt(value.Dot())
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
	prog, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	check2 := false
	check3 := false
	prog.Ref("f").FullUseDefChain(func(value *Value) {
		value.ShowDot()
		dot.ShowDotGraphToAsciiArt(value.Dot())
		value.GetTopDefs().ForEach(func(value *Value) {
			ret := value.GetConstValue()
			if ret == 2 && len(value.EffectOn) == 3 {
				check2 = true
			}
			if ret == 3 && len(value.EffectOn) == 3 {
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
	prog, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	var checkC, checkD bool
	var checkA bool
	prog.Ref("f").FullUseDefChain(func(value *Value) {
		value.ShowDot()
		var results = prog.Ref("c")
		results = append(results, prog.Ref("d")...)
		results = append(results, value)
		ret := FindStrictCommonDepends(results)
		if len(ret) != 2 {
			t.Fatal("the literal 2 trace failed")
		}
		ret.ForEach(func(value *Value) {
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
	prog, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	var checkC, checkD bool = true, true
	var checkA bool
	prog.Ref("f").FullUseDefChain(func(value *Value) {
		value.ShowDot()
		var results = prog.Ref("c")
		results = append(results, prog.Ref("d")...)
		ret := FindStrictCommonDepends(results)
		if len(ret) != 0 {
			t.Fatal("b & c is not common depends")
		}
		ret.ForEach(func(value *Value) {
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
	prog, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	results := FindFlexibleCommonDepends(append(prog.Ref("a"), prog.Ref("f")...))
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
	prog, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	results := FindFlexibleCommonDepends(append(prog.Ref("c"), prog.Ref("d")...))
	if len(results) <= 0 {
		t.Fatal("common depends (flexible)")
	}
}

func TestPathTrace_Strict(t *testing.T) {
	text := `a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)`
	prog, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	results := FindStrictCommonDepends(append(prog.Ref("c"), prog.Ref("d")...))
	if len(results) > 0 {
		t.Fatal("common depends (flexible)")
	}
}

func TestPathTrace_FlexibleDepends(t *testing.T) {
	text := `a = 1
b = a + c
d = b + e
f = d + h
z = f + g
y = x + z
`
	prog, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}

	vals := prog.Ref("d").FlexibleDepends().ShowDot()
	var d string
	vals.ForEach(func(value *Value) {
		d = value.Dot()
	})
	if !utils.MatchAllOfSubString(d, `label="c"`, `label="e"`, `label="h"`, `label="g"`, `label="x"`) {
		t.Fatal("not flexible depends")
	}
}
