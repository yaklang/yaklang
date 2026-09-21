package digest

import "testing"

func TestParseDeclaredInvalidUTF8BeforeColon(t *testing.T) {
	raw := "000000000000000000000000000\x8f:"
	got := ParseDeclared(raw)
	if got.Original != raw {
		t.Fatalf("original %q", got.Original)
	}
	if got.Canonical != "" {
		t.Fatalf("invalid UTF-8 became canonical %q", got.Canonical)
	}
	if len(got.Issues) == 0 {
		t.Fatal("expected illegal/malformed issue")
	}
}

func TestParseDeclaredSRIStillWorks(t *testing.T) {
	got := ParseDeclared("sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=")
	if got.Canonical != "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("sri %q", got.Canonical)
	}
}
