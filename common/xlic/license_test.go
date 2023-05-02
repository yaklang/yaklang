package xlic

import "testing"

func TestXLic(t *testing.T) {
	if Machine == nil {
		t.Fatal("Machine is nil")
	}
}
