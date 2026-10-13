package pcaputil

import (
	"bytes"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func bacnetRoutedWire(invoke, peer byte, response bool) []byte {
	w := bacnetRPWire(invoke, response, []byte{0x44, 0x3f, 0x80, 0, 0})
	npdu := []byte{1, 36, 0, 5, 1, peer, 255}
	if response {
		npdu = []byte{1, 8, 0, 5, 1, peer}
	}
	return bacnetTestPacket(10, append(npdu, w[6:]...))
}
func bacnetRoutedOriginWire(invoke, peer byte, response bool) []byte {
	w := bacnetRPWire(invoke, response, []byte{0x44, 0x3f, 0x80, 0, 0})
	npdu := []byte{1, 12, 0, 5, 1, peer}
	if response {
		npdu = []byte{1, 32, 0, 5, 1, peer, 255}
	}
	return bacnetTestPacket(10, append(npdu, w[6:]...))
}
func bacnetRoutedSession(t *testing.T) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionBACnetRoutedReadProperty(), WithSessionPorts(40000, 47808))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("done"); require.Zero(t, s.Stats().BufferedBytes) })
	return s
}
func TestBACnetRoutedReadLogicalIsolation(t *testing.T) {
	s := bacnetRoutedSession(t)
	reqA := bacnetRPFeed(t, s, 0, 1, bacnetRoutedOriginWire(36, 12, false))
	require.Empty(t, reqA.Error)
	reqB := bacnetRPFeed(t, s, 0, 2, bacnetRoutedOriginWire(36, 13, false))
	require.Empty(t, reqB.Error)
	require.NotEqual(t, reqA.FlowID, reqB.FlowID)
	ackB := bacnetRPFeed(t, s, 1, 3, bacnetRoutedOriginWire(36, 13, true))
	ackA := bacnetRPFeed(t, s, 1, 4, bacnetRoutedOriginWire(36, 12, true))
	for _, pair := range [][2]*ProtocolEvent{{reqA, ackA}, {reqB, ackB}} {
		q, a := pair[0], pair[1]
		require.Empty(t, a.Error)
		require.Equal(t, q.ID, a.ResponseTo)
		require.Equal(t, q.FlowID, a.FlowID)
		require.Equal(t, uint64(36), a.TransactionID)
		for _, e := range pair {
			f, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, bacnetRoutedReadProfile, e.Profile)
			require.Equal(t, uint32(2), f["Object Type"])
			require.Equal(t, uint32(1), f["Object Instance"])
			require.Equal(t, uint32(85), f["Property Identifier"])
			require.Equal(t, false, f["Authentication Verified"])
		}
	}
	qf, _ := reqA.GetFields()
	af, _ := ackA.GetFields()
	require.Equal(t, map[string]any{"Network": uint16(5), "Address": []byte{12}}, qf["Source Route"])
	require.Equal(t, qf["Source Route"], af["Destination Route"])
	require.Equal(t, 10, qf["APDU Offset"])
	require.Equal(t, 11, af["APDU Offset"])
	require.Equal(t, uint8(255), af["Hop Count"])
	require.Equal(t, false, qf["Hop Count Present"])
	_, exists := qf["Hop Count"]
	require.False(t, exists)
	require.Equal(t, []byte{1, 12, 0, 5, 1, 12}, qf["NPDU Header"])
	require.Equal(t, []byte{1, 32, 0, 5, 1, 12, 255}, af["NPDU Header"])
	before := s.Stats()
	probe := s.Probe(bacnetRoutedOriginWire(99, 12, false))
	require.Equal(t, ProbeAccept, probe.Verdict)
	require.Equal(t, bacnetRoutedReadProfile, probe.Version)
	require.Equal(t, before, s.Stats())
}
func TestBACnetRoutedReadPhysicalRefusal(t *testing.T) {
	for _, reason := range []string{"bad-route", "oversize", "bad-value"} {
		t.Run(reason, func(t *testing.T) {
			s := bacnetRoutedSession(t)
			for i, peer := range []byte{12, 13} {
				e := bacnetRPFeed(t, s, 0, int64(peer), bacnetRoutedWire(byte(36+i), peer, false))
				require.Empty(t, e.Error)
			}
			w := bacnetRoutedWire(36, 12, true)
			kind := ErrMalformedMessage
			if reason == "bad-route" {
				w[8] = 255
			} else if reason == "bad-value" {
				w[len(w)-1] = 0x2f
			} else {
				s.(*captureSession).f.a.config.MaxMessageBytes = 64
				w = bacnetTestPacket(10, append([]byte{1, 8, 0, 5, 1, 12}, append([]byte{0x30, 36, 12}, bytes.Repeat([]byte{0}, 100)...)...))
				kind = ErrResourceExceeded
			}
			bacnetRPReject(t, bacnetRPFeed(t, s, 1, 20, w), kind)
			for i, peer := range []byte{12, 13} {
				bacnetRPReject(t, bacnetRPFeed(t, s, 1, int64(30+peer), bacnetRoutedWire(byte(36+i), peer, true)), ErrResourceExceeded)
			}
		})
	}
}
func TestBACnetRoutedReadOwnership(t *testing.T) {
	s := bacnetRoutedSession(t)
	q := bacnetRoutedWire(1, 12, false)
	e := bacnetRPFeed(t, s, 0, 1, q)
	clear(q)
	f, err := e.GetFields()
	require.NoError(t, err)
	want := cloneSession(f)
	f["Destination Route"].(map[string]any)["Address"].([]byte)[0] = 99
	e.Session["Destination Route"].(map[string]any)["Address"].([]byte)[0] = 99
	clear(e.Raw)
	e.Session = map[string]any{"forged": true}
	e.Fields = e.Session
	e.Structured = map[string]any{"fields": e.Session}
	e.Protocol = "http"
	f, err = e.GetFields()
	require.NoError(t, err)
	require.Equal(t, want, f)
	e2 := bacnetRPFeed(t, s, 1, 2, bacnetRoutedWire(1, 12, true))
	require.Empty(t, e2.Error)
	require.Equal(t, e.ID, e2.ResponseTo)
	s.Close("done")
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Empty(t, s.Feed(0, time.Unix(3, 0), bacnetRoutedWire(2, 12, false)).Events)
}

func TestBACnetReadForeignInvokeNamespace(t *testing.T) {
	for _, routed := range []bool{false, true} {
		for _, collision := range []bool{false, true} {
			t.Run(fmt.Sprintf("routed=%v/collision=%v", routed, collision), func(t *testing.T) {
				var s ProtocolSession
				if routed {
					s = bacnetRoutedSession(t)
				} else {
					s = bacnetRPSession(t, false)
				}
				wire := func(inv byte, response bool) []byte {
					if routed {
						return bacnetRoutedWire(inv, 12, response)
					}
					return bacnetRPWire(inv, response, []byte{0})
				}
				q := bacnetRPFeed(t, s, 0, 1, wire(36, false))
				require.Empty(t, q.Error)
				inv := byte(37)
				if collision {
					inv = 36
				}
				foreign := wire(inv, false)
				offset := 6
				if routed {
					offset = 11
				}
				foreign[offset+3] = 15
				bacnetRPReject(t, bacnetRPFeed(t, s, 0, 2, foreign), ErrUnsupportedFeature)
				resp := bacnetRPFeed(t, s, 1, 3, wire(36, true))
				if collision {
					bacnetRPReject(t, resp, ErrContextRequired)
				} else {
					require.Empty(t, resp.Error)
					require.Equal(t, q.ID, resp.ResponseTo)
				}
				bacnetRPReject(t, bacnetRPFeed(t, s, 0, 4, wire(inv, false)), ErrContextRequired)
			})
		}
	}
}

func TestBACnetRoutedReadBudgetAndDomain(t *testing.T) {
	_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionBACnetRoutedReadProperty(), WithSessionTransport("tcp"))
	require.Error(t, err)
	s := bacnetRoutedSession(t)
	a := s.(*captureSession).f.a
	for i, domain := range []CaptureDomain{{Interface: 0}, {Interface: 1}, {Section: 1}, {Encapsulation: "vxlan:1"}} {
		req := &ProtocolEvent{ID: uint64(i*2 + 1), Domain: domain, Source: "192.0.2.1:40000", Destination: "192.0.2.2:47808", Transport: "udp", Timestamp: time.Unix(100, 0)}
		a.decodeBACnetReadDatagram(req, bacnetRoutedWire(36, 12, false), true)
		require.Empty(t, req.Error)
		resp := &ProtocolEvent{ID: req.ID + 1, Domain: domain, Source: req.Destination, Destination: req.Source, Transport: "udp", Timestamp: time.Unix(101, 0)}
		a.decodeBACnetReadDatagram(resp, bacnetRoutedWire(36, 12, true), true)
		require.Empty(t, resp.Error)
		require.Equal(t, req.ID, resp.ResponseTo)
	}
	before := s.Stats()
	a.budget.MaxRecursionDepth = 1
	require.Equal(t, ProbeReject, s.Probe(bacnetRoutedWire(37, 12, false)).Verdict)
	require.Equal(t, before, s.Stats())
	bacnetRPReject(t, bacnetRPFeed(t, s, 0, 200, bacnetRoutedWire(37, 12, false)), ErrResourceExceeded)
	s.Close("done")
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, a.bacnetInvokeOrigins)
}

func TestBACnetSourceInvokeAcrossPeersExistingAPI(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reuse=%v", reuse), func(t *testing.T) {
			var cap bytes.Buffer
			writer := pcapgo.NewWriterNanos(&cap)
			require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
			q := bacnetRPWire(36, false, nil)
			ack := bacnetRPWire(36, true, []byte{0})
			qb, ab := bytes.Clone(q), bytes.Clone(ack)
			if !reuse {
				qb[8] = 37
				ab[7] = 37
			}
			steps := []struct {
				src, dst byte
				wire     []byte
			}{{1, 2, q}, {1, 3, qb}, {3, 1, ab}, {2, 1, ack}}
			for i, st := range steps {
				eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
				ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(192, 0, 2, st.src), DstIP: net.IPv4(192, 0, 2, st.dst)}
				sp, dp := layers.UDPPort(40000), layers.UDPPort(47808)
				if st.src != 1 {
					sp, dp = dp, sp
				}
				udp := &layers.UDP{SrcPort: sp, DstPort: dp}
				require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
				b := gopacket.NewSerializeBuffer()
				require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(st.wire)))
				require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(100, int64(i)*1000), CaptureLength: len(b.Bytes()), Length: len(b.Bytes())}, b.Bytes()))
			}
			var events []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(cap.Bytes()), WithProtocolDecodeAs("udp", 47808, "bacnet-read-property"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
			require.Len(t, events, 4)
			require.Empty(t, events[0].Error)
			if reuse {
				for _, e := range events[1:] {
					bacnetRPReject(t, e, ErrContextRequired)
				}
			} else {
				for _, e := range events {
					require.Empty(t, e.Error)
				}
				require.Equal(t, events[1].ID, events[2].ResponseTo)
				require.Equal(t, events[0].ID, events[3].ResponseTo)
			}
		})
	}
}
