package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
	"time"
)

func TestRADIUSContextOwnershipAndRefusal(t *testing.T) {
	t.Run("udp-native-private-query", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, err)
		defer s.Close("cleanup")
		raw := agentRadiusRequest(7)
		o := s.Feed(0, time.Unix(100, 0), raw)
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		e := o.Events[0]
		want := cloneSession(e.Session)
		require.Equal(t, "conversation", want["Observation Scope"])
		f, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, want, f)
		for k := range f {
			delete(f, k)
		}
		e.Fields = map[string]any{"forged": true}
		e.Structured = map[string]any{"fields": map[string]any{"forged": true}}
		e.Session = map[string]any{"forged": true}
		e.Protocol = "http"
		clear(e.Raw)
		clear(raw)
		f, err = e.GetFields()
		require.NoError(t, err)
		require.Equal(t, want, f)
		tree, err := e.Decode()
		require.NoError(t, err)
		require.Equal(t, want, tree["session"])
	})
	t.Run("legacy-field-types-exact", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		defer s.Close("cleanup")
		o := s.Feed(0, time.Unix(100, 0), agentRadiusRequest(7))
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		expected := map[string]any{"Code": byte(1), "Identifier": byte(7), "Length": uint16(33), "Authenticator": []byte("0123456789abcdef"), "Attributes": []any{map[string]any{"Type": byte(1), "Length": byte(7), "User-Name": "alice"}, map[string]any{"Type": byte(4), "Length": byte(6), "NAS-IP-Address": []byte{192, 0, 2, 9}}}}
		got, err := o.Events[0].GetFields()
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("udp-refused-owner-preserves-neighbor", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxMessageBytes = 64
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, err)
		defer s.Close("cleanup")
		feed := func(dir int, at int64, w []byte) *ProtocolEvent {
			o := s.Feed(dir, time.Unix(at, 0), w)
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			return o.Events[0]
		}
		q1 := feed(0, 100, agentRadiusRequest(7))
		q2 := feed(0, 101, agentRadiusRequest(8))
		require.NotEqual(t, q1.ID, q2.ID)
		denied := radiusPkt(1, 7, radiusAttr(1, bytes.Repeat([]byte{'x'}, 44)))
		require.Greater(t, len(denied), 64)
		out := s.Feed(0, time.Unix(102, 0), denied)
		require.NotNil(t, out.Err)
		require.Equal(t, ErrResourceExceeded, out.Err.Kind)
		require.Len(t, out.Events, 1)
		require.Zero(t, out.Events[0].ResponseTo)
		require.Zero(t, out.Events[0].TransactionID)
		deniedEvent := out.Events[0]
		require.Equal(t, "limited", deniedEvent.Status)
		require.Equal(t, "limited", deniedEvent.Completeness)
		require.Equal(t, string(ErrResourceExceeded), deniedEvent.ExpertCode)
		require.Equal(t, len(denied), deniedEvent.Length)
		require.Equal(t, denied[:64], deniedEvent.Raw)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(denied[:64])), deniedEvent.SourceBytes.SHA256)
		require.Equal(t, len(denied), out.Consumed)
		f, err := deniedEvent.GetFields()
		require.Nil(t, f)
		rocTypedError(t, "ResourceExceeded", err)
		tlsDTLSPoison(deniedEvent, "http")
		f, err = deniedEvent.GetFields()
		require.Nil(t, f)
		rocTypedError(t, "ResourceExceeded", err)
		r1 := feed(1, 103, radiusPkt(2, 7, nil))
		require.Zero(t, r1.ResponseTo)
		require.Zero(t, r1.TransactionID)
		require.NotContains(t, r1.Session, "In Reply To")
		r2 := feed(1, 104, radiusPkt(2, 8, nil))
		require.Equal(t, q2.ID, r2.ResponseTo)
		require.Equal(t, q2.ID, r2.TransactionID)
		require.Equal(t, "Access-Request", r2.Session["In Reply To"])
		require.EqualValues(t, 172, s.Stats().InputBytes)
		require.EqualValues(t, 172, s.Stats().MessageBytes)
		require.EqualValues(t, 66, s.Stats().LimitedBytes)
		require.EqualValues(t, 5, s.Stats().Messages)
		require.Empty(t, s.Close("complete"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func agentRadiusFields(w []byte, reply bool) map[string]any {
	attrs := make([]map[string]any, 0)
	m := map[string]any{"Packet Name": "Access-Request", "Code": int(w[0]), "Identifier": int(w[1]), "Role": "request", "Authenticator": bytes.Clone(w[4:20]), "Packet Length": len(w), "Padding Bytes": 0, "Authentication": "not-verified", "Observation Scope": "conversation"}
	for at := 20; at < len(w); {
		typ, n := w[at], int(w[at+1])
		v := bytes.Clone(w[at+2 : at+n])
		attrs = append(attrs, map[string]any{"Type": typ, "Raw": v})
		if typ == 1 {
			m["User-Name"] = string(v)
		}
		if typ == 4 {
			m["NAS-IP-Address"] = fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
		}
		at += n
	}
	m["Attributes"], m["Attribute Count"] = attrs, len(attrs)
	if reply {
		m["Packet Name"], m["Role"], m["In Reply To"] = "Access-Accept", "response", "Access-Request"
	}
	return m
}
func TestRADIUSContextDomainsAndClose(t *testing.T) {
	for _, vlan := range []bool{false, true} {
		t.Run(fmt.Sprintf("vlan%t", vlan), func(t *testing.T) {
			q0 := agentRadiusRequest(7)
			q1 := agentRadiusRequest(7)
			q1[4] ^= 1
			copy(q1[22:27], []byte("other"))
			reply := radiusPkt(2, 7, nil)
			wires := [][]byte{q0, q1, reply, reply}
			domains := []int{0, 1, 1, 0}
			reverse := []bool{false, false, true, true}
			var events []*ProtocolEvent
			var stats ProtocolStats
			a, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
			require.NoError(t, err)
			defer a.Close()
			for i, w := range wires {
				src, dst := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
				sp, dp := layers.UDPPort(41000), layers.UDPPort(1812)
				if reverse[i] {
					src, dst = dst, src
					sp, dp = dp, sp
				}
				eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
				ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: src, DstIP: dst}
				u := &layers.UDP{SrcPort: sp, DstPort: dp}
				require.NoError(t, u.SetNetworkLayerForChecksum(ip))
				buf := gopacket.NewSerializeBuffer()
				opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
				iface := domains[i]
				if vlan {
					iface = 0
					eth.EthernetType = layers.EthernetTypeDot1Q
					tag := &layers.Dot1Q{VLANIdentifier: uint16(10 + domains[i]), Type: layers.EthernetTypeIPv4}
					require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, tag, ip, u, gopacket.Payload(w)))
				} else {
					require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, u, gopacket.Payload(w)))
				}
				b := buf.Bytes()
				require.NoError(t, a.Feed(b, gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+i), 0), CaptureLength: len(b), Length: len(b), InterfaceIndex: iface}, layers.LinkTypeEthernet))
				clear(b)
			}
			require.NoError(t, a.Close())
			require.NoError(t, a.Close())
			require.Len(t, events, 4)
			require.NotEqual(t, events[0].FlowID, events[1].FlowID)
			for i, e := range events {
				require.Equal(t, wires[i], e.Raw)
				require.Equal(t, agentRadiusFields(wires[i], reverse[i]), e.Session)
				got, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, agentRadiusFields(wires[i], reverse[i]), got)
				require.Equal(t, len(wires[i]), e.Length)
				require.Len(t, e.SourceBytes.PacketRefs, 1)
				require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
				require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
				if vlan {
					require.Equal(t, fmt.Sprintf("/vlan:%d", 10+domains[i]), e.Domain.Encapsulation)
				} else {
					require.Equal(t, domains[i], e.Domain.Interface)
				}
			}
			require.Equal(t, events[1].ID, events[2].ResponseTo)
			require.Equal(t, events[1].ID, events[2].TransactionID)
			require.Equal(t, events[0].ID, events[3].ResponseTo)
			require.Equal(t, events[0].ID, events[3].TransactionID)
			require.Zero(t, events[0].ResponseTo)
			require.Zero(t, events[1].ResponseTo)
			require.Zero(t, stats.BufferedBytes)
		})
	}
	t.Run("pending-close-owned-needmore", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, err)
		o := s.Feed(0, time.Unix(100, 0), agentRadiusRequest(7))
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		end := s.Close("done")
		require.Len(t, end, 1)
		require.Equal(t, "incomplete", end[0].Status)
		require.Equal(t, map[string]any{"Outstanding": 1}, end[0].Session)
		require.Zero(t, end[0].ResponseTo)
		require.Zero(t, end[0].TransactionID)
		f, err := end[0].GetFields()
		require.Nil(t, f)
		rocTypedError(t, "NeedMore", err)
		tlsDTLSPoison(end[0], "http")
		f, err = end[0].GetFields()
		require.Nil(t, f)
		rocTypedError(t, "NeedMore", err)
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
