package xlic

import "testing"

func TestXLic(t *testing.T) {
	EnsureInitialized()
	if Machine == nil {
		t.Fatal("Machine is nil")
	}
}
