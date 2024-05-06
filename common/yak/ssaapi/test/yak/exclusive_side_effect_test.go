package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
	"testing"
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
		`, ssatest.CheckTopDef_Contain("c", []string{"2", "1"}, true))
	})
}
