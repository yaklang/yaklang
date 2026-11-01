package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDNSReadableNamesPreserveWireAndCanonicalIdentity(t *testing.T) {
	for _, c := range []struct{ label, display, escaped string }{
		{"hp laserjet", "hp laserjet", `hp\032laserjet`},
		{"Jägermeister", "Jägermeister", `J\195\164germeister`},
		{string([]byte{'J', 228, 'g'}), `J\228g`, `J\228g`},
		{"a.b", `a\.b`, `a\.b`},
		{"a\x00b", `a\000b`, `a\000b`},
		{"x\u202ey", `x\226\128\174y`, `x\226\128\174y`},
	} {
		t.Run(c.escaped, func(t *testing.T) {
			w := reliabilityDNS(c.label)
			f, err := DecodeDNSMessage(w, 64)
			require.NoError(t, err)
			q := f["Questions"].([]map[string]any)[0]
			require.Equal(t, c.escaped, q["Name"])
			require.Equal(t, w[12:len(w)-4], q["Name Wire"])
			e := &ProtocolEvent{Protocol: "mdns", Session: map[string]any{"DNS": f}}
			require.Equal(t, []any{c.display}, e.DisplayFields()["dns.qry.name"])
			require.Equal(t, []any{c.escaped}, e.DisplayFields()["dns.qry.name.escaped"])
			original := dnsQuestionKey(f)
			q["Display Name"] = "different display only"
			require.Equal(t, original, dnsQuestionKey(f))
			clear(w)
			require.NotEqual(t, make([]byte, len(q["Name Wire"].([]byte))), q["Name Wire"])
		})
	}
	invalid, _ := DecodeDNSMessage(reliabilityDNS(string([]byte{228})), 64)
	replacement, _ := DecodeDNSMessage(reliabilityDNS("�"), 64)
	require.NotEqual(t, dnsQuestionKey(invalid), dnsQuestionKey(replacement), "replacement display must never alias invalid wire octets")
}

func TestCompletedMessageByteFrameIsOwnedAndNotDissectorBatching(t *testing.T) {
	e := &ProtocolEvent{ID: 1, Status: "decoded", Protocol: "tls", Raw: bytes.Repeat([]byte{1}, 5), SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 12, Domain: CaptureDomain{Interface: 2}}, {Number: 7}}}}
	e.finalizeEvidence()
	require.Equal(t, uint64(12), e.SourceBytes.CompletedIn.Number)
	require.Equal(t, 2, e.SourceBytes.CompletedIn.Domain.Interface)
	copy := *e
	cloneEvidence(&copy)
	copy.SourceBytes.CompletedIn.Number = 99
	require.Equal(t, uint64(12), e.SourceBytes.CompletedIn.Number)
	s := e.DisplayFieldValues()
	require.Equal(t, []any{uint64(12)}, s.FieldMap()["pdu.completed_in"])
	s.Source.CompletedIn.Number = 100
	require.Equal(t, uint64(12), e.SourceBytes.CompletedIn.Number)
	e.Status = "incomplete"
	e.finalizeEvidence()
	require.Nil(t, e.SourceBytes.CompletedIn)
	e.Status = "decoded"
	e.Raw = nil
	e.finalizeEvidence()
	require.Nil(t, e.SourceBytes.CompletedIn)
}
