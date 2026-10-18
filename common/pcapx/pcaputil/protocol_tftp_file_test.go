package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
	"time"
)

type tftpPublicStep struct {
	dir    int
	sp, dp uint16
	wire   []byte
}

func tftpPublicPacketsObserved(t *testing.T, steps []tftpPublicStep, observe func(*ProtocolEvent), opts ...CaptureOption) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var events []*ProtocolEvent
	var stats ProtocolStats
	opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) {
		snap := *e
		snap.Raw = append([]byte(nil), e.Raw...)
		snap.Session = cloneSession(e.Session)
		snap.Fields = cloneSession(e.Fields)
		snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
		events = append(events, &snap)
		if observe != nil {
			observe(e)
		}
	}), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
	a, err := NewPacketAnalyzer(opts...)
	require.NoError(t, err)
	for i, s := range steps {
		src, dst := net.IP{192, 0, 2, 1}, net.IP{192, 0, 2, 2}
		if s.dir == 1 {
			src, dst = dst, src
		}
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: src, DstIP: dst}
		u := &layers.UDP{SrcPort: layers.UDPPort(s.sp), DstPort: layers.UDPPort(s.dp)}
		require.NoError(t, u.SetNetworkLayerForChecksum(ip))
		b := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, u, gopacket.Payload(s.wire)))
		raw := b.Bytes()
		require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Unix(int64(i+1), 0), CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
		clear(raw)
	}
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
	require.Zero(t, stats.BufferedBytes)
	return events, stats
}
func TestTFTPRejectedCandidateTID(t *testing.T) {
	for _, prior := range []bool{false, true} {
		name := "new-unseen-server-TID"
		if prior {
			name = "previously-rejected-TID-cannot-be-fresh"
		}
		t.Run(name, func(t *testing.T) {
			q1 := []byte("\x00\x01one.txt\x00octet\x00")
			q2 := []byte("\x00\x01two.txt\x00octet\x00")
			d1 := []byte("\x00\x03\x00\x01one")
			old := []byte("\x00\x03\x00\x01old")
			ack := []byte{0, 4, 0, 1}
			steps := []tftpPublicStep{{0, 40000, 69, q1}, {1, 50000, 40000, d1}}
			if prior {
				steps = append(steps, tftpPublicStep{1, 50001, 40000, old})
			}
			steps = append(steps, tftpPublicStep{0, 40000, 50000, ack}, tftpPublicStep{0, 40000, 69, q2}, tftpPublicStep{1, 50001, 40000, old})
			events, _ := tftpPublicPackets(t, steps)
			require.Len(t, events, len(steps))
			require.Empty(t, events[0].Error)
			require.Empty(t, events[1].Error)
			if prior {
				rocTypedError(t, "ContextRequired", events[2].sessionError)
			}
			e := events[len(events)-1]
			require.Equal(t, old, e.Raw)
			require.Equal(t, "192.0.2.2:50001", e.Source)
			require.Equal(t, "192.0.2.1:40000", e.Destination)
			if prior {
				require.Equal(t, "context-required", e.Status, "a wire-observed rejected candidate cannot later masquerade as an unseen generation")
				rocTypedError(t, "ContextRequired", e.sessionError)
			} else {
				require.Empty(t, e.Error)
				require.Equal(t, "DATA", e.Session["Packet Name"])
				require.Equal(t, "two.txt", e.Session["Filename"])
				require.Equal(t, "192.0.2.2:50001", e.Session["Server Transfer ID"])
			}
		})
	}
}

func tftpPublicPackets(t *testing.T, steps []tftpPublicStep, opts ...CaptureOption) ([]*ProtocolEvent, ProtocolStats) {
	return tftpPublicPacketsObserved(t, steps, nil, opts...)
}

func TestTFTPInitialServiceIsolation(t *testing.T) {
	for _, port := range []uint16{69, 70} {
		name := "same-service-retry"
		if port == 70 {
			name = "another-initial-service-is-not-a-retry"
		}
		t.Run(name, func(t *testing.T) {
			q := []byte("\x00\x01mvp.txt\x00octet\x00")
			events, _ := tftpPublicPackets(t, []tftpPublicStep{{0, 40000, 69, q}, {0, 40000, port, q}, {1, 50000, 40000, []byte("\x00\x03\x00\x01hello")}, {0, 40000, 50000, []byte{0, 4, 0, 1}}})
			require.Len(t, events, 4)
			require.Equal(t, "192.0.2.2:"+map[uint16]string{69: "69", 70: "70"}[port], events[1].Destination)
			if port == 69 {
				require.Empty(t, events[3].Error)
				require.Equal(t, true, events[3].Session["Transfer Complete"])
			} else {
				require.Equal(t, "context-required", events[1].Status, "identical wire sent to a different initial service is not an exact retry")
				require.Equal(t, "context-required", events[2].Status)
				require.Equal(t, "context-required", events[3].Status)
			}
		})
	}
}

func TestTFTPFirstRefusal(t *testing.T) {
	for _, lost := range []bool{false, true} {
		name := "ordinary"
		if lost {
			name = "denied-first-request-cannot-disappear"
		}
		t.Run(name, func(t *testing.T) {
			q := []byte("\x00\x01new.txt\x00octet\x00")
			data := []byte("\x00\x03\x00\x01old")
			ack := []byte{0, 4, 0, 1}
			var steps []tftpPublicStep
			if lost {
				large := append([]byte{0, 1}, bytes.Repeat([]byte{'x'}, 256)...)
				large = append(large, []byte("\x00octet\x00")...)
				steps = append(steps, tftpPublicStep{0, 40000, 69, large})
			}
			steps = append(steps, tftpPublicStep{0, 40000, 69, q}, tftpPublicStep{1, 50000, 40000, data}, tftpPublicStep{0, 40000, 50000, ack})
			events, _ := tftpPublicPackets(t, steps, WithProtocolBudget(128, 1<<20))
			require.Len(t, events, len(steps))
			if lost {
				require.Equal(t, "limited", events[0].Status)
				require.Equal(t, "context-required", events[len(events)-2].Status, "a refused initial RRQ cannot later manufacture ownership for delayed DATA")
				require.Equal(t, "context-required", events[len(events)-1].Status)
			} else {
				require.Empty(t, events[2].Error)
				require.Equal(t, true, events[2].Session["Transfer Complete"])
				require.Equal(t, "192.0.2.2:50000", events[2].Session["Server Transfer ID"])
			}
		})
	}
}

func TestTFTPQueryOwnership(t *testing.T) {
	for _, edit := range []bool{false, true} {
		name := "normal"
		if edit {
			name = "public-projection-edit"
		}
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 69))
			require.NoError(t, err)
			defer s.Close("cleanup")
			o := s.Feed(0, time.Unix(1, 0), []byte("\x00\x01mvp.txt\x00octet\x00"))
			require.Len(t, o.Events, 1)
			e := o.Events[0]
			v, err := e.Decode()
			require.NoError(t, err)
			want, err := json.Marshal(v)
			require.NoError(t, err)
			if edit {
				tlsDTLSPoison(e, "redis")
			}
			v, err = e.Decode()
			require.NoError(t, err)
			got, err := json.Marshal(v)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
			s.Close("done")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestTFTPRefusalOwnership(t *testing.T) {
	for _, edit := range []bool{false, true} {
		name := "normal"
		if edit {
			name = "public-projection-edit"
		}
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 69))
			require.NoError(t, err)
			defer s.Close("cleanup")
			o := s.Feed(0, time.Unix(1, 0), []byte("\x00\x01mvp.txt\x00octet\x00"))
			require.Nil(t, o.Err)
			o = s.Feed(1, time.Unix(2, 0), []byte{0, 3, 0})
			require.Len(t, o.Events, 1)
			require.NotNil(t, o.Err)
			require.Equal(t, ErrMalformedMessage, o.Err.Kind)
			e := o.Events[0]
			if edit {
				tlsDTLSPoison(e, "caller")
				e.Status = "decoded"
				e.Error = ""
			}
			f, err := e.GetFields()
			rocTypedError(t, "MalformedMessage", err)
			require.Nil(t, f)
			s.Close("done")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestTFTPOctetFileBoundaries(t *testing.T) {
	q := []byte("\x00\x01mvp.txt\x00octet\x00")
	wr := []byte("\x00\x02mvp.txt\x00octet\x00")
	d := func(n byte, b []byte) []byte { return append([]byte{0, 3, 0, n}, b...) }
	ack := func(n byte) []byte { return []byte{0, 4, 0, n} }
	rq := tftpPublicStep{0, 40000, 69, q}
	data := func(n byte, b []byte) tftpPublicStep { return tftpPublicStep{1, 50000, 40000, d(n, b)} }
	a := func(n byte) tftpPublicStep { return tftpPublicStep{0, 40000, 50000, ack(n)} }
	body := append(bytes.Repeat([]byte{'A'}, 512), []byte{0, 255, 'B'}...)
	for _, c := range []struct {
		name  string
		steps []tftpPublicStep
		body  []byte
		limit int
		files int
	}{
		{"empty", []tftpPublicStep{rq, data(1, nil), a(1)}, []byte{}, 4096, 1},
		{"binary-two-block", []tftpPublicStep{rq, data(1, body[:512]), a(1), data(2, body[512:]), a(2)}, body, 4096, 1},
		{"exact-retries-one-object", []tftpPublicStep{rq, data(1, body[:512]), data(1, body[:512]), a(1), data(2, body[512:]), a(2), a(2)}, body, 4096, 1},
		{"WRQ", []tftpPublicStep{{0, 40000, 69, wr}, {1, 50000, 40000, ack(0)}, {0, 40000, 50000, d(1, []byte("write"))}, {1, 50000, 40000, ack(1)}}, []byte("write"), 4096, 1},
		{"file-byte-limit", []tftpPublicStep{rq, data(1, []byte("hello")), a(1)}, nil, 4, 0},
		{"missing-final-ACK", []tftpPublicStep{rq, data(1, []byte("hello"))}, nil, 4096, 0},
		{"gap-not-a-file", []tftpPublicStep{rq, data(2, []byte("gap")), data(1, []byte("hello")), a(1)}, nil, 4096, 0},
	} {
		for _, poison := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/poison%t", c.name, poison), func(t *testing.T) {
				var originals []*ProtocolEvent
				observe := func(e *ProtocolEvent) {
					originals = append(originals, e)
					if poison {
						tlsDTLSPoison(e, "redis")
						e.ID = 999999
						e.FlowID = 999999
						if len(e.SourceBytes.PacketRefs) > 0 {
							e.SourceBytes.PacketRefs[0].Number = 999999
						}
					}
				}
				events, _ := tftpPublicPacketsObserved(t, c.steps, observe, WithProtocolTFTPFileAssembly(69, c.limit))
				require.Len(t, events, len(c.steps))
				count := 0
				for i, e := range events {
					f, ok := e.Session["File Bytes"]
					if !ok {
						continue
					}
					count++
					require.Equal(t, c.body, f)
					require.Equal(t, len(c.body), e.Session["File Byte Count"])
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(c.body)), e.Session["File SHA256"])
					require.Equal(t, events[0].ID, e.Session["File Request ID"])
					require.Equal(t, []uint64{1}, e.Session["File Request Packets"])
					out, err := originals[i].Decode()
					require.NoError(t, err)
					rocEqualFields(t, e.Session, out["session"].(map[string]any))
					mutateRTPSQuery(out)
					out, err = originals[i].Decode()
					require.NoError(t, err)
					rocEqualFields(t, e.Session, out["session"].(map[string]any))
					src := e.Session["File Sources"].([]map[string]any)
					offset := 0
					for _, x := range src {
						id := x["Event ID"].(uint64)
						require.Equal(t, offset, x["File Offset"])
						require.Equal(t, 4, x["Event Offset"])
						found := -1
						for j, v := range events {
							if v.ID == id {
								found = j
								break
							}
						}
						require.GreaterOrEqual(t, found, 0)
						w := c.steps[found].wire
						require.Equal(t, int(w[1]), 3)
						n := x["Length"].(int)
						require.Equal(t, len(w)-4, n)
						require.Equal(t, []uint64{uint64(found + 1)}, x["Packet Numbers"])
						require.Equal(t, c.body[offset:offset+n], w[4:])
						offset += n
					}
					require.Equal(t, len(c.body), offset)
				}
				require.Equal(t, c.files, count)
			})
		}
	}
}

// A completed file must release its retained data/evidence reservation without
// releasing the conversation identity needed to reject delayed datagrams.
func TestTFTPFileCompletionReleasesBytes(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 69), WithSessionTFTPFileAssembly(69, 4096))
	require.NoError(t, err)
	q := s.Feed(0, time.Unix(1, 0), []byte("\x00\x01mvp.bin\x00octet\x00"))
	require.Len(t, q.Events, 1)
	require.Nil(t, q.Err)
	base := s.Stats().BufferedBytes
	d := s.Feed(1, time.Unix(2, 0), []byte("\x00\x03\x00\x01hello"))
	require.Len(t, d.Events, 1)
	require.Nil(t, d.Err)
	retained := s.Stats().BufferedBytes
	require.Greater(t, retained, base)
	a := s.Feed(0, time.Unix(3, 0), []byte{0, 4, 0, 1})
	require.Len(t, a.Events, 1)
	require.Nil(t, a.Err)
	require.Equal(t, []byte("hello"), a.Events[0].Session["File Bytes"])
	require.Less(t, s.Stats().BufferedBytes, retained, "final ACK releases file-owned storage immediately")
	require.Greater(t, s.Stats().BufferedBytes, int64(0), "identity history remains charged")
	require.Empty(t, s.Close("done"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
