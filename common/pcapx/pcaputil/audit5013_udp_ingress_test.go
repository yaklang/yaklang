package pcaputil

import (
	"github.com/gopacket/gopacket/layers"
	"testing"
)

// Real UDP transport, not the existing TCP-wrapped synthetic session fixture.
// This test requires current repository test helpers and has NOT been run here.
func TestAudit5013QUICRealUDPIngress(t *testing.T) {
	raw := sessionDatagramPCAP(t, []sessionStep{{0, rfc9001ClientInitial()}}, layers.UDPPort(14443))
	events, _, err := binReplay(t, raw, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Protocol == "quic" && e.Session["Decrypted"] == true {
			return
		}
	}
	t.Fatalf("RFC9001 Initial in a real UDP capture was not admitted and authenticated: %d events", len(events))
}
