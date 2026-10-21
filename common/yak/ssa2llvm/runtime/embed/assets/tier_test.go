package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLargestTierDiskArchiveOverridesEmbedded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(TierDirEnv, dir)
	archive := filepath.Join(dir, EmbeddedTier, "libyak.a")
	if err := os.MkdirAll(filepath.Dir(archive), 0755); err != nil {
		t.Fatal(err)
	}
	want := []byte("fresh rebuilt runtime archive")
	if err := os.WriteFile(archive, want, 0644); err != nil {
		t.Fatal(err)
	}
	rp, res, err := ReleaseTierTo(t.TempDir(), EmbeddedTier)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != archive || res.Used != EmbeddedTier {
		t.Fatalf("resolution: %+v", res)
	}
	got, err := os.ReadFile(rp.Libyak)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("stale embedded runtime won over rebuilt disk archive")
	}
	if err := os.WriteFile(rp.Libyak, []byte("caller patched private copy"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("release did not make a private archive copy")
	}
}
