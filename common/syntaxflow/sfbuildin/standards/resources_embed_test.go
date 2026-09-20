package standards

import (
	"bytes"
	"os"
	"testing"
)

func TestCompressedResourceMatchesSource(t *testing.T) {
	want, err := os.ReadFile("mappings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := mappingsFS.ReadFile("mappings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("embedded resource differs from source; regenerate release assets")
	}
}
