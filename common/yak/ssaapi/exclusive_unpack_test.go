package ssaapi

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
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
	prog, err := Parse(`
	a={"b": f, "c": 2}; 
	e=a.b+a.b+a.b+a.b+a.b+a.b;
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	values := lo.UniqBy(
		prog.Ref("e").GetTopDefs(),
		func(v *Value) int64 { return v.GetId() },
	)
	assert.Equal(t, 1, len(values))
}

func TestUnpack_BasicFunctionUnpack(t *testing.T) {
	prog, err := Parse(`c = () => {return 1, 2};a,b = c()`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show().Ref("a").GetTopDefs().ForEach(func(value *Value) {
		value.Show()
	})
}
