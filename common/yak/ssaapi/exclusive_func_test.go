package ssaapi

import "testing"

func TestFunctionTrace(t *testing.T) {
	Parse(`
a = 1
b = (c, d) => {
	a = c + d
	return d, c
}
e, f = b(2,3)
g = e // 3
h = f // 2
i = a // 2 + 3
`)
}
