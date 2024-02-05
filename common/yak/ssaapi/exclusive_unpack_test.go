package ssaapi

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUnpack_Basic(t *testing.T) {
	prog, err := Parse(`a,b = c;e=a+b;`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	assert.Equal(t, 1, len(prog.Ref("a").GetTopDefs()))
}

func TestUnpack_Basic2(t *testing.T) {
	prog, err := Parse(`a,b = c();e=a+b;`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	assert.Equal(t, 1, len(prog.Ref("a").GetTopDefs()))
}

func TestUnpack_Basic3(t *testing.T) {
	prog, err := Parse(`a={"b": f, "c": 2}; e=a.b+a.b+a.b+a.b+a.b+a.b;`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	id := make(map[int]struct{})
	prog.Ref("e").GetTopDefs().ForEach(func(value *Value) {
		id[value.GetId()] = struct{}{}
	})
	assert.Equal(t, 1, len(id))
}
