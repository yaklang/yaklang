package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestUnpack_Basic(t *testing.T) {
	prog, err := ssaapi.Parse(`a,b = c;e=a+b;`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	assert.Equal(t, 1, len(prog.Ref("a").GetTopDefs()))
}

func TestUnpack_Basic2(t *testing.T) {
	prog, err := ssaapi.Parse(`a,b = c();e=a+b;`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	assert.Equal(t, 1, len(prog.Ref("a").GetTopDefs()))
}

func TestUnpack_Basic3(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t, `
	a={"b": f, "c": 2}; 
	e=a.b+a.b+a.b+a.b+a.b+a.b;
	`, `e #-> as $res`, map[string][]string{
		"res": {"Undefined-a.b"},
	})
}

func TestUnpack_BasicFunctionUnpack(t *testing.T) {
	prog, err := ssaapi.Parse(`c = () => {return 1, 2};a,b = c()`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show().Ref("a").GetTopDefs().ForEach(func(value *ssaapi.Value) {
		value.Show()
	})
}
