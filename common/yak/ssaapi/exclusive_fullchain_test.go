package ssaapi

import (
	"github.com/stretchr/testify/assert"
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

	prog.Ref("h").ForEach(func(value *Value) {
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
