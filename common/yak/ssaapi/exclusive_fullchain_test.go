package ssaapi

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/dot"
	"strings"
	"testing"
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
	prog, err := Parse(`a=b+c;
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
