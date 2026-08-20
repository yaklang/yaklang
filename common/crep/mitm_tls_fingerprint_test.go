package crep

import (
	"testing"

	"github.com/yaklang/yaklang/common/netx"
)

func TestMITMDefaultTLSFingerprint(t *testing.T) {
	server, err := NewMITMServer()
	if err != nil {
		t.Fatal(err)
	}
	if got := server.tlsFingerprint; got != netx.DefaultTLSFingerprint {
		t.Fatalf("default MITM TLS fingerprint = %q, want %q", got, netx.DefaultTLSFingerprint)
	}
}

func TestMITMTLSFingerprintSelection(t *testing.T) {
	server := &MITMServer{tlsFingerprint: netx.DefaultTLSFingerprint}

	if err := MITM_RandomJA3(false)(server); err != nil {
		t.Fatal(err)
	}
	if server.tlsFingerprint != "" {
		t.Fatalf("legacy randomJA3(false) should select native TLS, got %q", server.tlsFingerprint)
	}

	if err := MITM_RandomJA3(true)(server); err != nil {
		t.Fatal(err)
	}
	if got := server.tlsFingerprint; got != netx.DefaultTLSFingerprint {
		t.Fatalf("legacy randomJA3(true) fingerprint = %q, want %q", got, netx.DefaultTLSFingerprint)
	}

	if err := MITM_TLSFingerprint(netx.TLSFingerprintChrome120)(server); err != nil {
		t.Fatal(err)
	}
	if got := server.tlsFingerprint; got != netx.TLSFingerprintChrome120 {
		t.Fatalf("selected MITM TLS fingerprint = %q", got)
	}

	if err := MITM_TLSFingerprint("does-not-exist")(server); err == nil {
		t.Fatal("unknown MITM TLS fingerprint should fail validation")
	}
}
