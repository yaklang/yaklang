package utils

import (
	"bytes"
	"io"
	"testing"
)

func TestTrimLeftReader_Read(t *testing.T) {
	reader := NewTrimLeftReader(bytes.NewBufferString("  abc  "))
	var raw, _ = io.ReadAll(reader)
	if string(raw) != "abc  " {
		t.Errorf("expected 'abc  ', got '%s'", raw)
	}
}
