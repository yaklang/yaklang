package thirdparty_bin

import (
	"bytes"
	"os"
	"testing"
)

func TestCompressedResourceMatchesSource(t *testing.T) {
	want, err := os.ReadFile("bin_cfg.yml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := configFS.ReadFile("bin_cfg.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("embedded resource differs from source; regenerate release assets")
	}
}
