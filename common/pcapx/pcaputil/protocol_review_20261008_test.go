package pcaputil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestReviewHTTP2DisplayFields(t *testing.T) {
	steps := dohExchangeH2Start()
	steps = append(steps, sessionStep{0, h2TestFrame(1, 5, 1, h2TestHeaders(t, ":method", "GET", ":scheme", "https", ":path", "/", ":authority", "example.test"))}, sessionStep{1, h2TestFrame(1, 5, 1, h2TestHeaders(t, ":status", "204"))})
	for _, deferred := range []bool{false, true} {
		events, _ := sessionTestFlow(t, "http2", steps, 1, deferred)
		request, err := CompileDisplayFilter(`http.request.method == "GET" and http2.streamid == 1`)
		require.NoError(t, err)
		response, err := CompileDisplayFilter(`http.response.code == 204 and http2.streamid == 1`)
		require.NoError(t, err)
		require.True(t, request.Match(events[len(events)-2]))
		require.False(t, response.Match(events[len(events)-2]))
		require.True(t, response.Match(events[len(events)-1]))
		require.False(t, request.Match(events[len(events)-1]))
	}
}

func TestReviewIPPCloseDelimitedFIN(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		session, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		s := session.(*captureSession)
		s.f.a.config.Deferred = deferred
		body := ippTestWire(0xb)
		r := s.Feed(0, time.Unix(1, 0), append([]byte(fmt.Sprintf("POST /ipp HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/ipp\r\nContent-Length: %d\r\n\r\n", len(body))), body...))
		require.Nil(t, r.Err)
		pending := s.Feed(1, time.Unix(2, 0), append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/ipp\r\nConnection: close\r\n\r\n"), ippTestWire(0)...))
		require.True(t, pending.NeedMore)
		require.Equal(t, ErrNeedMore, pending.Err.Kind)
		events := s.Close(string(TrafficFlowCloseReason_FIN))
		require.Len(t, events, 1)
		require.Equal(t, "ipp", events[0].Protocol)
		require.Empty(t, events[0].Error)
		require.Equal(t, true, events[0].Session["Matched"])
		require.Equal(t, r.Events[0].ID, events[0].ResponseTo)
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestReviewDoHEarlyFinalResponseAndLateUpload(t *testing.T) {
	for _, warm := range []bool{false, true} {
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{1, 7, 0} {
				t.Run(fmt.Sprintf("warm%v/deferred%v/chunk%d", warm, deferred, chunk), func(t *testing.T) {
					session, err := NewProtocolSession(DefaultParserBudget())
					require.NoError(t, err)
					s := session.(*captureSession)
					s.f.a.config.Deferred = deferred
					defer s.Close("FIN")
					post := dohPOST("dns.example.test", dnsWire(dnsQuery(0, "late.example", 1)))
					end := bytes.Index(post, []byte("\r\n\r\n")) + 4
					feed := func(dir int, wire []byte) (events []*ProtocolEvent) {
						for len(wire) > 0 {
							n := len(wire)
							if chunk > 0 {
								n = min(n, chunk)
							}
							r := s.Feed(dir, time.Unix(2, 0), wire[:n])
							if r.Err != nil {
								require.True(t, r.NeedMore)
								require.Equal(t, ErrNeedMore, r.Err.Kind)
							}
							events = append(events, r.Events...)
							wire = wire[n:]
						}
						return
					}
					if warm {
						require.Len(t, feed(0, []byte("GET /warm HTTP/1.1\r\nHost: dns.example.test\r\n\r\n")), 1)
						require.Len(t, feed(1, []byte("HTTP/1.1 204 No Content\r\n\r\n")), 1)
					}
					feed(0, post[:end])
					early := feed(1, dohHTTPResp(415, nil))
					late := feed(0, post[end:])
					request := feed(0, dohGET("dns.example.test", "next.example", 0))
					response := feed(1, dohHTTPResp(200, dnsWire(dnsAResponse(0, "next.example", [4]byte{1, 2, 3, 4}))))
					require.Len(t, early, 1)
					require.Len(t, late, 1)
					require.Len(t, request, 1)
					require.Len(t, response, 1)
					require.Equal(t, late[0].ID, early[0].ResponseTo)
					require.Equal(t, request[0].ID, response[0].ResponseTo)
					require.Equal(t, "next.example", response[0].Session["Matched Request"])
					require.Equal(t, false, late[0].Session["Outstanding"])
					require.Empty(t, s.f.doh.pending)
				})
			}
		}
	}
}

func TestReviewDoHCloseDelimitedFIN(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint(deferred), func(t *testing.T) {
			session, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			s := session.(*captureSession)
			s.f.a.config.Deferred = deferred
			request := s.Feed(0, time.Unix(1, 0), dohGET("dns.example.test", "fin.example", 0))
			require.Nil(t, request.Err)
			wire := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/dns-message\r\nConnection: close\r\n\r\n"), dnsWire(dnsAResponse(0, "fin.example", [4]byte{1, 2, 3, 4}))...)
			pending := s.Feed(1, time.Unix(2, 0), wire)
			require.True(t, pending.NeedMore)
			require.Equal(t, ErrNeedMore, pending.Err.Kind)
			events := s.Close(string(TrafficFlowCloseReason_FIN))
			require.Len(t, events, 1)
			require.Equal(t, "doh", events[0].Protocol)
			require.Empty(t, events[0].Error)
			require.Equal(t, request.Events[0].ID, events[0].ResponseTo)
			require.Equal(t, "fin.example", events[0].Session["Matched Request"])
			require.Equal(t, []string{"1.2.3.4"}, events[0].Session["A Records"])
			require.Zero(t, s.f.a.buffered.Load())
		})
	}
}

func TestReviewZooKeeperPendingBudgetAndClose(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(fmt.Sprint(limited), func(t *testing.T) {
			session, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			s := session.(*captureSession)
			for _, step := range zookeeperTestSteps()[:2] {
				require.Nil(t, s.Feed(step.dir, time.Unix(1, 0), step.wire).Err)
			}
			if limited {
				s.f.a.config.MaxBufferedBytes = int(s.f.a.buffered.Load()) + 512
			}
			path := "/" + strings.Repeat("z", 2048)
			r := s.Feed(0, time.Unix(2, 0), zookeeperTestRequest(100, zookeeperOpcodeGetChildren, path))
			if limited {
				require.NotNil(t, r.Err, "retained path must consume the shared budget")
				require.Equal(t, ErrResourceExceeded, r.Err.Kind)
				require.Nil(t, s.f.zookeeper)
			} else {
				require.Nil(t, r.Err)
				require.GreaterOrEqual(t, s.f.sessionBytes, int64(len(path)))
				require.Equal(t, path, s.f.zookeeper.pending[100].path)
			}
			s.Close("FIN")
			require.Nil(t, s.f.zookeeper, "close must discard retained pending paths")
			require.Zero(t, s.f.a.buffered.Load())
		})
	}
}

func TestReviewLiveWorkerCaptureDomains(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		for _, observer := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/observer=%v", workers, observer), func(t *testing.T) {
				var events []*ProtocolEvent
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })}
				if observer {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
				}
				a, err := NewPacketAnalyzer(opts...)
				require.NoError(t, err)
				var raws [][]byte
				for _, path := range []string{"/a", "/b"} {
					ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
					tcp := &layers.TCP{SrcPort: 40000, DstPort: 80, Seq: 1, ACK: true}
					require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
					raws = append(raws, m1Serialize(t, ip, tcp, gopacket.Payload("GET "+path+" HTTP/1.1\r\nHost: example.test\r\n\r\n")))
				}
				at := 0
				read := func() ([]byte, gopacket.CaptureInfo, error) {
					if at == len(raws) {
						return nil, gopacket.CaptureInfo{}, io.EOF
					}
					raw := raws[at]
					ci := gopacket.CaptureInfo{Timestamp: time.Unix(2, int64(at)), CaptureLength: len(raw), Length: len(raw), InterfaceIndex: 7 + at}
					at++
					return raw, ci, nil
				}
				require.NoError(t, readLiveWorkerPackets(a.conf, context.Background(), read, layers.LinkTypeRaw, !observer))
				require.NoError(t, a.Close())
				var requests []*ProtocolEvent
				for _, e := range events {
					if e.Length > 0 {
						requests = append(requests, e)
					}
				}
				require.Len(t, requests, 2)
				require.NotEqual(t, requests[0].FlowID, requests[1].FlowID)
				byInterface := map[int]*ProtocolEvent{}
				for _, e := range requests {
					byInterface[e.Domain.Interface] = e
				}
				for i := 0; i < 2; i++ {
					e := byInterface[7+i]
					require.NotNil(t, e)
					require.Equal(t, 7+i, e.Domain.Interface)
					require.Equal(t, time.Unix(2, int64(i)), e.Timestamp)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.Equal(t, uint64(i+1), e.SourceBytes.PacketRefs[0].Number)
					require.Contains(t, string(e.Raw), []string{"GET /a", "GET /b"}[i])
				}
			})
		}
	}
}

func TestReviewVXLANUDPObserverParity(t *testing.T) {
	inner := m1Serialize(t, &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{0, 1, 2, 3, 4, 6}, EthernetType: layers.EthernetTypeIPv4}, gopacket.Payload(m1UDP(t, m1DNSQuery())))
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(198, 51, 100, 1), DstIP: net.IPv4(198, 51, 100, 2)}
	udp := &layers.UDP{SrcPort: 40001, DstPort: 4789}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	raw := m1Serialize(t, ip, udp, &layers.VXLAN{ValidIDFlag: true, VNI: 42}, gopacket.Payload(inner))
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observer := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers=%d/deferred=%v/observer=%v", workers, deferred, observer), func(t *testing.T) {
					var events []*ProtocolEvent
					opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })}
					if observer {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
					}
					a, err := NewPacketAnalyzer(opts...)
					require.NoError(t, err)
					require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(raw), Length: len(raw), InterfaceIndex: 3}, layers.LinkTypeRaw))
					require.NoError(t, a.Close())
					var messages []*ProtocolEvent
					for _, e := range events {
						if e.Length > 0 {
							messages = append(messages, e)
						}
					}
					require.Len(t, messages, 1)
					e := messages[0]
					require.Equal(t, "dns", e.Protocol)
					require.Empty(t, e.Error)
					require.Equal(t, m1DNSQuery(), e.Raw)
					require.Equal(t, 3, e.Domain.Interface)
					require.Contains(t, e.Domain.Encapsulation, "/vxlan:")
					require.Contains(t, e.Domain.Encapsulation, ":42")
					require.Equal(t, "192.0.2.1:40000", e.Source)
					require.NotEmpty(t, e.Session)
					_, err = e.Decode()
					require.NoError(t, err)
					require.Zero(t, a.conf.binParser.buffered.Load())
				})
			}
		}
	}
}
