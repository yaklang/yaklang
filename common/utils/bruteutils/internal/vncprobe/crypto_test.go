package vncprobe

import (
	"encoding/hex"
	"testing"
)

func TestVNCAuthResponseKnownVectors(t *testing.T) {
	challenge, err := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		password string
		want     string
	}{
		{"", "491e890de9ace932838a49792f2213f3"},
		{"pass", "5fb02f4e6ec9fda06c41df1f35015138"},
		{"password", "b866924125c8eebb9debc1db61c538e2"},
		{"VncPass123!", "bcaa4d80e2549f96f9531d07c24ede46"},
		{"12345678", "83dd2b4dbd04367f28578fdd5b142740"},
		{"toolongpassword", "047c82fb19899ed16db7a73b6c2e69f1"},
	} {
		got, err := vncAuthResponse(tc.password, challenge)
		if err != nil {
			t.Fatalf("password %q: %v", tc.password, err)
		}
		if hex.EncodeToString(got) != tc.want {
			t.Errorf("password %q: got %x want %s", tc.password, got, tc.want)
		}
	}
}

func TestVNCAuthResponseRejectsShortChallenge(t *testing.T) {
	if _, err := vncAuthResponse("pass", make([]byte, 15)); !isProtocolMismatch(err) {
		t.Fatalf("want protocol mismatch, got %v", err)
	}
}
