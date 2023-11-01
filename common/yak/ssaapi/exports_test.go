package ssaapi

import "testing"

func TestA(t *testing.T) {
	prog := Parse(
		`
windows.location.href = "www"
	`,
		WithLanguage(JS),
	)

	win := prog.Ref("windows").Ref("location").Ref("href")
	win.Show()
	win.UseDefChain(func(udc *UseDefChain) {
		udc.Show()
		udc.SetSetDirectionUseBy().Walk(func(v *Instruction) {
		})
	})
}
