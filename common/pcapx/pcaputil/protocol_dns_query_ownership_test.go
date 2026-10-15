package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func dnsOwnedQueryInput(t *testing.T) []byte {
	t.Helper()
	raw, err := trafficfixture.ReadFile("testdata/protocol-sessions/reliability/dns-lan.pcap")
	require.NoError(t, err)
	r, err := pcapgo.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	packet, _, err := r.ReadPacketData()
	require.NoError(t, err)
	p := gopacket.NewPacket(packet, r.LinkType(), gopacket.Default)
	u, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
	require.True(t, ok)
	// Fixed historical first query: one label containing a literal dot, not
	// two labels. RFC1035 4.1.1/4.1.2 supplies the independent full answer.
	require.Equal(t, "12340000000100000000000003612e620000010001", fmt.Sprintf("%x", u.Payload))
	return bytes.Clone(u.Payload)
}

func dnsOwnedQueryFields(wire []byte) map[string]any {
	return map[string]any{
		"ID": uint16(4660), "Flags": uint16(0), "Response": false, "Opcode": uint16(0), "RCODE": uint16(0), "Truncated": false,
		"Questions": []map[string]any{{"Name": `a\.b`, "Name Wire": bytes.Clone(wire[12:17]), "Type": uint16(1), "Class": uint16(1), "Class Base": uint16(1), "QU": false}},
		"Answers":   []map[string]any{}, "Authority": []map[string]any{}, "Additional": []map[string]any{},
	}
}

func testDNSOwnedDatagramQueries(t *testing.T) {
	wire := dnsOwnedQueryInput(t)
	for port, protocol := range map[uint16]string{53: "dns", 5353: "mdns", 5355: "llmnr"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deferred%t", protocol, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, port))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				owned := bytes.Clone(wire)
				out := s.Feed(0, time.Unix(1, 0), owned)
				clear(owned)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Equal(t, protocol, e.Protocol)
				require.Equal(t, protocol+"-native", e.Profile)
				require.Equal(t, wire, e.Raw)
				require.Equal(t, "message", e.Completeness)
				require.Zero(t, e.ResponseTo)
				require.NotZero(t, e.ID)
				want := dnsOwnedQueryFields(wire)
				f, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, want, f)
				require.Equal(t, want, e.Session["DNS"])
				wantSession := cloneSession(e.Session)
				if protocol == "mdns" {
					require.Zero(t, e.TransactionID)
					require.Equal(t, "multicast-observation", e.Session["Association"])
				} else {
					require.Equal(t, e.ID, e.TransactionID)
				}
				f["ID"] = uint16(0)
				q := f["Questions"].([]map[string]any)[0]
				q["Name"] = "mutated"
				clear(q["Name Wire"].([]byte))
				tree, err := e.Decode()
				require.NoError(t, err)
				require.Equal(t, want, protocolFields(tree))
				protocolFields(tree)["Flags"] = uint16(1)
				// The session in a returned Decode envelope is itself owned.
				tree["session"].(map[string]any)["DNS"].(map[string]any)["RCODE"] = uint16(3)
				e.Session["DNS"].(map[string]any)["ID"] = uint16(1)
				if e.Fields != nil {
					e.Fields["ID"] = uint16(2)
				}
				if e.Structured != nil {
					protocolFields(e.Structured)["RCODE"] = uint16(4)
				}
				again, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, want, again)
				tree, err = e.Decode()
				require.NoError(t, err)
				require.Equal(t, wantSession, tree["session"])
				v, err := NewBinParserInspector(1, protocolHistoryBytes(e))
				require.NoError(t, err)
				v.OnEvent(e)
				details, err := v.Details(e.ID)
				require.NoError(t, err)
				tree, err = details.Decode()
				require.NoError(t, err)
				require.Equal(t, wantSession, tree["session"])
				mutateRTPSQuery(tree)
				details, err = v.Details(e.ID)
				require.NoError(t, err)
				tree, err = details.Decode()
				require.NoError(t, err)
				require.Equal(t, wantSession, tree["session"])
				require.Zero(t, v.Evicted())
				require.Nil(t, v.Rows("", 0)[0].semanticSession)
				bounded, err := NewBinParserInspector(1, protocolHistoryBytes(e)-1)
				require.NoError(t, err)
				bounded.OnEvent(e)
				require.EqualValues(t, 1, bounded.Evicted())
				require.Empty(t, bounded.Rows("", 0))
				// Refuse one missing QCLASS octet as a complete bad datagram;
				// keep a normal adjacent query on the same session afterwards.
				bad := s.Feed(0, time.Unix(2, 0), wire[:len(wire)-1])
				require.Len(t, bad.Events, 1)
				rocTypedError(t, "MalformedMessage", bad.Err)
				refusal := bad.Events[0]
				f, err = refusal.GetFields()
				rocTypedError(t, "MalformedMessage", err)
				require.Nil(t, f)
				require.Nil(t, refusal.Session)
				require.Equal(t, wire[:len(wire)-1], refusal.Raw)
				require.Zero(t, refusal.TransactionID)
				require.Zero(t, refusal.ResponseTo)
				assertOwnedProtocolRefusal(t, refusal, "MalformedMessage", err)
				next := s.Feed(0, time.Unix(3, 0), wire)
				require.Nil(t, next.Err)
				require.Len(t, next.Events, 1)
				f, err = next.Events[0].GetFields()
				require.NoError(t, err)
				require.Equal(t, want, f)
				require.Zero(t, next.Events[0].ResponseTo)
				if protocol != "mdns" {
					// A refused same-identity input is not an invisible exact retry.
					require.Zero(t, next.Events[0].TransactionID)
					require.Equal(t, "ambiguous", next.Events[0].Session["Association Status"])
					require.NotContains(t, next.Events[0].Session, "Retransmission")
				}
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
			t.Run(fmt.Sprintf("%s/deferred%t/record-budget", protocol, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(ParserBudget{MaxCollectionElements: 1}, WithSessionTransport("udp"), WithSessionPorts(40000, port))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				// A second copy of the historical question, with exact QDCOUNT.
				// This checks refusal propagation, not another framing algorithm.
				two := append(bytes.Clone(wire), wire[12:]...)
				two[5] = 2
				out := s.Feed(0, time.Unix(1, 0), two)
				require.Len(t, out.Events, 1)
				rocTypedError(t, "ResourceExceeded", out.Err)
				e := out.Events[0]
				f, err := e.GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Nil(t, e.Session)
				require.Equal(t, "limited", e.Status)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				assertOwnedProtocolRefusal(t, e, "ResourceExceeded", err)
				next := s.Feed(0, time.Unix(2, 0), wire)
				require.Nil(t, next.Err)
				require.Len(t, next.Events, 1)
				f, err = next.Events[0].GetFields()
				require.NoError(t, err)
				require.Equal(t, dnsOwnedQueryFields(wire), f)
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
