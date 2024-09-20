package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_SideEffect(t *testing.T) {
	t.Run("normal side-effect", func(t *testing.T) {
		code := `
a = 1
b = () => {
	a = 2
}
b()
c = a;
`
		ssatest.Check(t, code,
			ssatest.CheckTopDef_Contain("c", []string{"2"}, true),
		)
	})

	t.Run("phi side-effect", func(t *testing.T) {
		ssatest.Check(t, `
a = 1
b = () => {
	a = 2
}
if e {b()}
c = a;
		`, ssatest.CheckTopDef_Contain("c", []string{"2", "1", "Undefined-e"}, true))
	})

	t.Run("if-else phi side-effect", func(t *testing.T) {
		ssatest.Check(t, `
		d = "kkk"
		ok = foo("ooo", d)
		a= 1 
		if ok{
			a= 1
		}else{
			a = 2
		}
		b = a

		`, ssatest.CheckTopDef_Contain("b", []string{
			"1",
			"2",
			"Undefined-foo",
			"ooo",
			"kkk",
		}, true))
	})

	t.Run("simple if else-if phi side-effect ", func(t *testing.T) {
		ssatest.Check(t, `
		a = 3
		if c{
			a= 1
		}else if d{
			a = 2
		}
		b = a

		`, ssatest.CheckTopDef_Contain("b", []string{
			"1",
			"2",
			"3",
			"Undefined-c",
			"Undefined-d",
		}, true))
	})

	t.Run("complex if else-if phi side-effect", func(t *testing.T) {
		ssatest.Check(t, `
		a = 1

		ok = false
		if e {
			ok = true
		}else{
			ok = false
		}

		if c{
			a= 11
		}else if ok{
			a = 111
		}
		b = a

		`, ssatest.CheckTopDef_Contain("b", []string{
			"1",
			"111",
			"11",
			"Undefined-c",
			"true",
			"false",
			"Undefined-e",
		}, true))
	})
}
