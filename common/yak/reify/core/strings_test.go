package core

import (
	"testing"
)

func TestArtifactStringAt(t *testing.T) {
	artifact := testArtifact(t)
	raw := make([]byte, 100)
	copy(raw[0x10:], []byte("hello, reify\x00trailing"))
	artifact.Reader = newReader(t, raw)
	if text, ok := artifact.StringAt(0x1000, 64); !ok || text != "hello, reify" {
		t.Fatalf("StringAt = %q %v", text, ok)
	}
	if _, ok := artifact.StringAt(0x2000, 64); ok {
		t.Fatal("zero-fill section returned a string")
	}
}
