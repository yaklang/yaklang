package yarn

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseIntegrityInvalidUTF8DoesNotPanic(t *testing.T) {
	raw := []byte("000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000@00\nintegrity 000000000000000000000000000\x8f:")
	libs, deps, err := NewParser().Parse(nil, bytes.NewReader(raw))
	if err != nil && strings.Contains(err.Error(), "panic") {
		t.Fatalf("parser panicked: %v", err)
	}
	_ = libs
	_ = deps
}
