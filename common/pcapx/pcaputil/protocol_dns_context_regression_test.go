package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
	"time"
)

func TestDNSForeignClockExistingAPI(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "adjacent", true: "foreign-future"}[foreign], func(t *testing.T) {
			q := dnsWire(dnsQuery(0x7123, "clock.example", 1))
			r := dnsWire(dnsAResponse(0x7123, "clock.example", [4]byte{192, 0, 2, 1}))
			makePacket := func(src, dst string, sp, dp uint16, ts uint32, w []byte) []byte {
				p := t20UDPCapture(t, net.ParseIP(src), net.ParseIP(dst), sp, dp, w)
				require.Equal(t, uint32(0xa1b2c3d4), binary.LittleEndian.Uint32(p))
				binary.LittleEndian.PutUint32(p[24:], ts)
				return p
			}
			data := makePacket("192.0.2.10", "192.0.2.53", 41707, 53, 100, q)
			if foreign {
				other := makePacket("198.51.100.10", "198.51.100.53", 41708, 53, 1000, dnsWire(dnsQuery(0x8423, "other.example", 1)))
				data = append(data, other[24:]...)
			}
			reply := makePacket("192.0.2.53", "192.0.2.10", 53, 41707, 101, r)
			data = append(data, reply[24:]...)
			var events []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(data), WithOnProtocolMessage(func(e *ProtocolEvent) {
				if e.Protocol == "dns" {
					events = append(events, e)
				}
			})))
			require.Len(t, events, 2+map[bool]int{false: 0, true: 1}[foreign])
			first, last := events[0], events[len(events)-1]
			require.Equal(t, first.ID, last.ResponseTo, "unrelated future endpoint must not expire this timely DNS response")
			require.Equal(t, first.ID, last.TransactionID)
			require.Equal(t, "matched", last.Session["Association Status"])
			f, e := last.GetFields()
			require.NoError(t, e)
			require.Equal(t, uint16(0x7123), f["ID"])
		})
	}
}
func TestDNSIdentityExistingAPI(t *testing.T) {
	for _, name := range []string{"ordinary", "reuse-after-complete", "changed-query", "retry-renews-idle", "opcode-mismatch"} {
		t.Run(name, func(t *testing.T) {
			q := dnsWire(dnsQuery(0x7123, "identity.example", 1))
			r := dnsWire(dnsAResponse(0x7123, "identity.example", [4]byte{192, 0, 2, 1}))
			var data []byte
			add := func(reverse bool, ts uint32, w []byte) {
				src, dst, sp, dp := "192.0.2.10", "192.0.2.53", uint16(41707), uint16(53)
				if reverse {
					src, dst, sp, dp = dst, src, dp, sp
				}
				p := t20UDPCapture(t, net.ParseIP(src), net.ParseIP(dst), sp, dp, w)
				binary.LittleEndian.PutUint32(p[24:], ts)
				if data == nil {
					data = append(data, p[:24]...)
				}
				data = append(data, p[24:]...)
			}
			add(false, 100, q)
			switch name {
			case "reuse-after-complete":
				add(true, 101, r)
				add(false, 102, q)
				add(true, 103, r)
			case "changed-query":
				changed := bytes.Clone(q)
				changed[2] &= 0xfe
				add(false, 101, changed)
				add(true, 102, r)
			case "retry-renews-idle":
				add(false, 129, q)
				add(true, 131, r)
			case "opcode-mismatch":
				bad := bytes.Clone(r)
				bad[2] |= 0x10
				add(true, 101, bad)
			default:
				add(true, 101, r)
			}
			var ev []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(data), WithOnProtocolMessage(func(e *ProtocolEvent) {
				if e.Protocol == "dns" {
					ev = append(ev, e)
				}
			})))
			require.GreaterOrEqual(t, len(ev), 2)
			last := ev[len(ev)-1]
			if name == "ordinary" || name == "retry-renews-idle" {
				require.Equal(t, ev[0].ID, last.ResponseTo)
				require.Equal(t, "matched", last.Session["Association Status"])
			} else {
				require.Zero(t, last.ResponseTo, "unproven DNS generation or incompatible opcode cannot gain a precise reply owner")
				require.Zero(t, last.TransactionID)
			}
		})
	}
}
func TestDNSRefusalExistingAPI(t *testing.T) {
	for _, first := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "first-refused"}[first], func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxMessageBytes, b.MaxFrameBytes, b.ProbeBytes = 64, 64, 64
			b.MaxBufferedBytes = 4096
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41707, 53))
			require.NoError(t, err)
			q := dnsWire(dnsQuery(0x7123, "refused.example", 1))
			r := dnsWire(dnsAResponse(0x7123, "refused.example", [4]byte{192, 0, 2, 1}))
			if !first {
				out := s.Feed(0, time.Unix(100, 0), q)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
			}
			denied := s.Feed(0, time.Unix(101, 0), append(bytes.Clone(q), make([]byte, 256)...))
			rocTypedError(t, "ResourceExceeded", denied.Err)
			if first {
				out := s.Feed(0, time.Unix(102, 0), q)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
			}
			out := s.Feed(1, time.Unix(103, 0), r)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			require.Zero(t, out.Events[0].ResponseTo, "denied same-identity input cannot disappear and give the next reply a precise owner")
			require.Zero(t, out.Events[0].TransactionID)
			s.Close("done")
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestDNSNativeProjectionExistingAPI(t *testing.T) {
	for _, protocol := range []string{"dns", "mdns", "llmnr"} {
		t.Run(protocol, func(t *testing.T) {
			port := map[string]uint16{"dns": 53, "mdns": 5353, "llmnr": 5355}[protocol]
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41707, port))
			require.NoError(t, err)
			defer s.Close("done")
			w := dnsOwnedQueryInput(t)
			o := s.Feed(0, time.Unix(1, 0), w)
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			e := o.Events[0]
			f, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, dnsOwnedQueryFields(w), f)
			before, err := e.Decode()
			require.NoError(t, err)
			want, err := json.Marshal(before)
			require.NoError(t, err)
			tlsDTLSPoison(e, "redis")
			after, err := e.Decode()
			require.NoError(t, err)
			got, err := json.Marshal(after)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
}
func TestDNSSemanticGapExistingAPI(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41707, 53))
	require.NoError(t, err)
	defer s.Close("done")
	q := dnsWire(dnsQuery(123, "gap.example", 1))
	r := dnsWire(dnsAResponse(123, "gap.example", [4]byte{192, 0, 2, 1}))
	require.Nil(t, s.Feed(0, time.Unix(1, 0), q).Err)
	bad := s.Feed(0, time.Unix(2, 0), q[:len(q)-1])
	rocTypedError(t, "MalformedMessage", bad.Err)
	require.Nil(t, s.Feed(0, time.Unix(3, 0), q).Err)
	o := s.Feed(1, time.Unix(4, 0), r)
	require.Nil(t, o.Err)
	require.Len(t, o.Events, 1)
	require.Zero(t, o.Events[0].ResponseTo)
	fresh := bytes.Clone(q)
	binary.BigEndian.PutUint16(fresh, 124)
	freshReply := bytes.Clone(r)
	binary.BigEndian.PutUint16(freshReply, 124)
	a := s.Feed(0, time.Unix(5, 0), fresh)
	require.Nil(t, a.Err)
	require.Len(t, a.Events, 1)
	b := s.Feed(1, time.Unix(6, 0), freshReply)
	require.Nil(t, b.Err)
	require.Len(t, b.Events, 1)
	require.Equal(t, a.Events[0].ID, b.Events[0].ResponseTo)
}
func TestDNSStreamProjectionExistingAPI(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			for _, serverPort := range []uint16{53, 0} {
				t.Logf("existing byte-stream projection port=%d", serverPort)
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(41707, serverPort), WithSessionClientDirection(0))
				require.NoError(t, err)
				defer s.Close("done")
				wire := dnsOwnedQueryInput(t)
				if bad && serverPort == 0 {
					// Unselected bytes cannot establish a malformed protocol by themselves.
					// First observe the valid compatibility stream, then refuse its bad frame.
					valid := append(binary.BigEndian.AppendUint16(nil, uint16(len(wire))), wire...)
					prior := s.Feed(0, time.Unix(0, 0), valid)
					require.Nil(t, prior.Err)
					require.Len(t, prior.Events, 1)
					require.Equal(t, dnsOwnedQueryFields(wire), prior.Events[0].Session["DNS"])
				}
				if bad {
					wire = wire[:len(wire)-1]
				}
				framed := binary.BigEndian.AppendUint16(nil, uint16(len(wire)))
				framed = append(framed, wire...)
				o := s.Feed(0, time.Unix(1, 0), framed)
				require.Len(t, o.Events, 1)
				e := o.Events[0]
				if bad {
					rocTypedError(t, "MalformedMessage", o.Err)
					_, er := e.GetFields()
					rocTypedError(t, "MalformedMessage", er)
					mutateRTPSQuery(e.Structured)
					tlsDTLSPoison(e, "redis")
					f, er := e.GetFields()
					rocTypedError(t, "MalformedMessage", er)
					require.Nil(t, f)
				} else {
					require.Nil(t, o.Err)
					require.Equal(t, dnsOwnedQueryFields(wire), e.Session["DNS"])
					before, er := e.Decode()
					require.NoError(t, er)
					want, er := json.Marshal(before)
					require.NoError(t, er)
					mutateRTPSQuery(e.Structured)
					tlsDTLSPoison(e, "redis")
					after, er := e.Decode()
					require.NoError(t, er)
					got, er := json.Marshal(after)
					require.NoError(t, er)
					require.JSONEq(t, string(want), string(got))
				}
			}
		})
	}
}
func TestDNSLANClockExistingAPI(t *testing.T) {
	_, dhcp := dhcpOwnedInputs(t)
	for _, first := range []string{"dns", "dhcp"} {
		t.Run(first, func(t *testing.T) {
			q := dnsWire(dnsQuery(0x7123, "lan-clock.example", 1))
			r := dnsWire(dnsAResponse(0x7123, "lan-clock.example", [4]byte{192, 0, 2, 1}))
			var data []byte
			add := func(src, dst string, sp, dp uint16, ts uint32, w []byte) {
				p := t20UDPCapture(t, net.ParseIP(src), net.ParseIP(dst), sp, dp, w)
				binary.LittleEndian.PutUint32(p[24:], ts)
				if data == nil {
					data = append(data, p[:24]...)
				}
				data = append(data, p[24:]...)
			}
			if first == "dns" {
				add("192.0.2.10", "192.0.2.53", 41707, 53, 100, q)
				add("192.0.2.11", "192.0.2.67", 68, 67, 1000, dhcp[0])
				add("192.0.2.53", "192.0.2.10", 53, 41707, 101, r)
			} else {
				add("192.0.2.11", "192.0.2.67", 68, 67, 100, dhcp[0])
				add("192.0.2.10", "192.0.2.53", 41707, 53, 1000, q)
				add("192.0.2.67", "192.0.2.11", 67, 68, 101, dhcp[1])
			}
			var events []*ProtocolEvent
			var stats ProtocolStats
			require.NoError(t, ReplayPcap(bytes.NewReader(data), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Len(t, events, 3)
			require.Equal(t, events[0].ID, events[2].ResponseTo, "a different protocol cannot expire the observed owner")
			require.Equal(t, events[0].ID, events[2].TransactionID)
			for _, e := range events {
				f, err := e.GetFields()
				require.NoError(t, err)
				if e.Protocol == "dhcp" {
					index := 0
					if f["Message Type"] == byte(2) {
						index = 1
					}
					require.Equal(t, dhcpOwnedFields(index), f)
					require.Equal(t, f, e.Session)
				}
			}
			require.Zero(t, stats.BufferedBytes)
		})
	}
}
func TestDNSObservedPortRefusalExistingAPI(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxMessageBytes, b.MaxFrameBytes, b.ProbeBytes = 64, 64, 64
	b.MaxBufferedBytes = 4096
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41707, 19555))
	require.NoError(t, err)
	defer s.Close("done")
	q := dnsWire(dnsQuery(0x7123, "state.example", 1))
	r := dnsWire(dnsAResponse(0x7123, "state.example", [4]byte{192, 0, 2, 1}))
	a := s.Feed(0, time.Unix(100, 0), q)
	require.Nil(t, a.Err)
	require.Len(t, a.Events, 1)
	require.Equal(t, "dns", a.Events[0].Protocol)
	denied := s.Feed(0, time.Unix(101, 0), append(bytes.Clone(q), make([]byte, 256)...))
	rocTypedError(t, "ResourceExceeded", denied.Err)
	o := s.Feed(1, time.Unix(102, 0), r)
	require.Nil(t, o.Err)
	require.Len(t, o.Events, 1)
	require.Zero(t, o.Events[0].ResponseTo)
	require.Zero(t, o.Events[0].TransactionID)
}
