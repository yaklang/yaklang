package ssaapi

import (
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
		Check(t, code,
			CheckTopDef_Contain("c", []string{"Function-b(", "2"}, true),
		)
	})

	t.Run("phi side-effect", func(t *testing.T) {
		Check(t, `
a = 1
b = () => {
	a = 2
}
if e {b()}
c = a;
		`, CheckTopDef_Contain("c", []string{"Function-b(", "2", "1"}, true))
	})
}
