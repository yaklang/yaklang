package pcaputil

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// Synthetic RFC 8484 exchanges. HTTP success and DNS success are independent;
// HTTP exchange identity must not hide a contradictory DNS message.
func TestDoHDNSDirectionMatchesHTTP(t *testing.T) {
	for _, response := range []bool{false, true} {
		t.Run(map[bool]string{false: "request", true: "response"}[response], func(t *testing.T) {
			d := &binDoH{}
			wire := dnsWire(dnsQuery(0, "direction.example", 1))
			if !response {
				wire[2] |= 0x80
			}
			_, err := d.attachDNS(map[string]any{"Stream ID": uint32(1)}, wire, response, 64)
			require.ErrorContains(t, err, "QR")
			require.Empty(t, d.pending)
		})
	}
}

func TestDoHDNSResponseQuestionMustMatch(t *testing.T) {
	for _, kind := range []string{"name", "type", "class", "opcode"} {
		t.Run(kind, func(t *testing.T) {
			d := &binDoH{}
			_, err := d.attachDNS(map[string]any{"Stream ID": uint32(1)}, dnsWire(dnsQuery(0, "one.example", 1)), false, 64)
			require.NoError(t, err)
			name := "one.example"
			if kind == "name" {
				name = "two.example"
			}
			wire := dnsWire(dnsQuery(0, name, 1))
			wire[2] |= 0x80
			switch kind {
			case "type":
				binary.BigEndian.PutUint16(wire[len(wire)-4:], 28)
			case "class":
				binary.BigEndian.PutUint16(wire[len(wire)-2:], 3)
			case "opcode":
				wire[2] |= 0x08
			}
			info, err := d.attachDNS(map[string]any{"Stream ID": uint32(1)}, wire, true, 64)
			require.ErrorContains(t, err, "question")
			require.NotEqual(t, "matched", info["Association Status"])
			require.Empty(t, d.pending)
		})
	}
}

func TestDoHDNSEmptyErrorAndNegativeAnswers(t *testing.T) {
	for _, rcode := range []byte{0, 1, 2, 3, 5} {
		for _, empty := range []bool{false, true} {
			if rcode == 0 && empty {
				continue
			}
			d := &binDoH{}
			_, err := d.attachDNS(map[string]any{"Stream ID": uint32(1)}, dnsWire(dnsQuery(0, "MiXeD.example", 1)), false, 64)
			require.NoError(t, err)
			wire := dnsWire(dnsQuery(0, "mixed.example", 1))
			wire[2] |= 0x80
			wire[3] = rcode
			if empty {
				wire = wire[:12]
				binary.BigEndian.PutUint16(wire[4:], 0)
			}
			info, err := d.attachDNS(map[string]any{"Stream ID": uint32(1)}, wire, true, 64)
			require.NoError(t, err, "rcode=%d empty=%v", rcode, empty)
			require.Equal(t, "matched", info["Association Status"])
			require.Equal(t, "MiXeD.example", info["Matched Request"])
			require.Empty(t, d.pending)
		}
	}
}

func dohDNSSemanticsSteps(t *testing.T) []sessionStep {
	t.Helper()
	steps := dohExchangeH2Start()
	for _, id := range []uint32{1, 3, 5, 7} {
		steps = append(steps, dohExchangeH2GET(t, id, "question.example"))
		wire := dnsWire(dnsQuery(0, "question.example", 1))
		wire[2] |= 0x80
		switch id {
		case 1:
			wire[2] &= 0x7f // An HTTP answer containing a DNS query.
		case 3:
			binary.BigEndian.PutUint16(wire[len(wire)-2:], 3) // Wrong QCLASS.
		case 5:
			wire = wire[:12]
			binary.BigEndian.PutUint16(wire[4:], 0)
			wire[3] = 2 // SERVFAIL without question.
		}
		steps = append(steps, sessionStep{1, h2TestFrame(1, 4, id, h2TestHeaders(t, ":status", "200", "content-type", "application/dns-message"))}, sessionStep{1, h2TestFrame(0, 1, id, wire)})
	}
	return steps
}

func assertDoHDNSSemanticsEvents(t *testing.T, events []*ProtocolEvent) {
	t.Helper()
	malformed, responses := 0, 0
	for _, event := range events {
		if event.Error != "" {
			malformed++
			require.Equal(t, "malformed", event.Status)
			require.Equal(t, "stream", event.Session["Error Scope"])
			require.NotEqual(t, "matched", event.Session["Association Status"])
			continue
		}
		if event.Session["Packet Name"] == "Response" {
			responses++
			require.Equal(t, "matched", event.Session["Association Status"])
			require.Equal(t, "question.example", event.Session["Matched Request"])
			if event.Session["Stream ID"] == uint32(5) {
				require.Equal(t, "omitted-error-question", event.Session["Question Status"])
				require.Equal(t, "", event.Session["QNAME"])
			}
		}
	}
	require.Equal(t, 2, malformed)
	require.Equal(t, 2, responses)
}

func TestDoHHTTP2DNSSemanticsStreamIsolation(t *testing.T) {
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			events, _ := sessionTestFlow(t, "http2", dohDNSSemanticsSteps(t), chunk, deferred)
			assertDoHDNSSemanticsEvents(t, events)
		}
	}
}

func TestDoHHTTP1DNSDirectionRejected(t *testing.T) {
	for _, response := range []bool{false, true} {
		wire := dnsWire(dnsQuery(0, "direction.example", 1))
		var steps []sessionStep
		if response {
			steps = []sessionStep{{0, dohGET("dns.example.test", "direction.example", 0)}, {1, dohHTTPResp(200, wire)}}
		} else {
			wire[2] |= 0x80
			steps = []sessionStep{{0, dohPOST("dns.example.test", wire)}}
		}
		for _, chunk := range []int{0, 1, 7} {
			events, _ := sessionTestFlow(t, "http", steps, chunk, false)
			require.Contains(t, events[len(events)-1].Error, "QR")
			require.Equal(t, "malformed", events[len(events)-1].Status)
		}
	}
}

func TestDoHDNSEDNSExtendedErrorWithoutQuestion(t *testing.T) {
	// Synthetic OPT RR: BADVERS = extended RCODE 1, header RCODE 0.
	// Its high TTL octet belongs to the extended RCODE, not an RR lifetime.
	for _, extended := range []byte{0, 1} {
		d := &binDoH{}
		request := dnsWire(dnsQuery(0, "version.example", 1))
		binary.BigEndian.PutUint16(request[10:], 1)
		request = append(request, 0, 0, 41, 0x10, 0, 0, 1, 0, 0, 0, 0) // EDNS version 1.
		_, err := d.attachDNS(map[string]any{"Stream ID": uint32(1)}, request, false, 64)
		require.NoError(t, err)
		wire := make([]byte, 12)
		wire[2] = 0x80
		binary.BigEndian.PutUint16(wire[10:], 1)
		wire = append(wire, 0, 0, 41, 0x10, 0, extended, 0, 0, 0, 0, 0)
		info, err := d.attachDNS(map[string]any{"Stream ID": uint32(1)}, wire, true, 64)
		if extended == 0 {
			require.ErrorContains(t, err, "question")
			require.Equal(t, "question-mismatch", info["Association Status"])
		} else {
			require.NoError(t, err)
			require.Equal(t, uint16(16), info["RCODE"])
			require.Equal(t, uint16(16), info["DNS"].(map[string]any)["RCODE"])
			require.Equal(t, "matched", info["Association Status"])
			require.Equal(t, "omitted-error-question", info["Question Status"])
		}
		require.Empty(t, d.pending)
	}
}
