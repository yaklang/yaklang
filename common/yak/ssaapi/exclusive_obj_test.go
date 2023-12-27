package ssaapi

import "testing"

func TestObjectTest(t *testing.T) {
	prog := Parse(`a = {}
a.b = 1;
a.b++;
c = a.b
`) // .Show()

	prog.Ref("c").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})
}
