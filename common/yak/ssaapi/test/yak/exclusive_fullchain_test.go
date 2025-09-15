package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
	code := `a=b+c;d=e(a);`

	ssatest.CheckSyntaxFlowGraphEdge(t, code, `d #-> as $target`, map[string][]ssatest.PathInTest{
		"target": {
			{"e(a)", "b+c", ""},
			{"e(a)", "e", ""},
			{"b+c", "b", ""},
			{"b+c", "c", ""},
		},
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func TestChain_Basic2(t *testing.T) {
	code := `a=b+c;d=e+a;`
	ssatest.CheckSyntaxFlowGraphEdge(t, code, `d #-> as $target`, map[string][]ssatest.PathInTest{
		"target": {
			{"e+a", "b+c", ""},
			{"e+a", "e", ""},
			{"b+c", "b", ""},
			{"b+c", "c", ""},
		},
	}, ssaapi.WithLanguage(ssaapi.Yak))
}

func TestChain_Phi_If(t *testing.T) {
	code := `
d = 1
a=b+c;
if(a){
	d=e
}else{
	d=f
};

g=d+a;`

	ssatest.CheckSyntaxFlowGraph(t, code, `g #-> as $target`, map[string]func(g *ssatest.GraphInTest){
		"target": func(graph *ssatest.GraphInTest) {
			graph.Show()
			require.Contains(t, graph.GenerateDOTString(), "e")
			require.Contains(t, graph.GenerateDOTString(), "f")
			require.Contains(t, graph.GenerateDOTString(), "if")
			graph.Check(t, "d+a", "b+c")
			graph.Check(t, "b+c", "b")
			graph.Check(t, "b+c", "c")
		},
	}, ssaapi.WithLanguage(ssaapi.Yak))
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
	// checkDotPhi := false
	prog.Show()
	prog.Ref("g").FullUseDefChain(func(value *ssaapi.Value) {
		value.ForEachDependOn(func(value *ssaapi.Value) {
			if value.IsPhi() {
				checkPhi = true
			}
		})
		// if strings.Contains(value.DotGraph(), `phi`) {
		// checkDotPhi = true
		// }
		value.ShowDot()
	})
	if !checkPhi {
		t.Fatal("checkPhi failed")
	}
	// if !checkDotPhi {
	// 	t.Fatal("checkDotPhi failed")
	// }
}

func TestFunctionDotTrace(t *testing.T) {
	text := `a = 1
b = (c, d, e) => {
	a = c + d
	return d, c
}
f = b(2,3,4)`
	ssatest.CheckSyntaxFlowGraph(t, text, `f #-> as $target`, map[string]func(g *ssatest.GraphInTest){
		"target": func(g *ssatest.GraphInTest) {
			dot := g.String()
			require.Contains(t, dot, `label="2"`)
			require.Contains(t, dot, `label="3"`)
			require.NotContains(t, dot, `label="4"`)
		},
	}, ssaapi.WithLanguage(ssaapi.Yak))
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
