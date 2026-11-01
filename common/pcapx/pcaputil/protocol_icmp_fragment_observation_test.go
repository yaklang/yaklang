package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// RFC791/8200 fragmentation derivatives preserve all selected ICMP bytes from
// immutable independently answered controls. Expected fields never come from
// the parser or from the fragment construction code.
func icmpFragmentFrames(t *testing.T, frame []byte, split int, atomic bool) [][]byte {
	t.Helper()
	require.GreaterOrEqual(t, len(frame), 42)
	ether, ip := frame[:14], frame[14:]
	v6 := ip[0]>>4 == 6
	hlen := int(ip[0]&15) * 4
	if v6 {
		hlen = 40
	}
	header, body := ip[:hlen], ip[hlen:]
	if atomic {
		require.True(t, v6)
		split = len(body)
	} else {
		require.Greater(t, len(body), split)
		require.Zero(t, split%8)
	}
	var frames [][]byte
	for off := 0; off < len(body); {
		end := len(body)
		if off == 0 {
			end = min(split, len(body))
		}
		h := bytes.Clone(header)
		more := end < len(body)
		var part []byte
		if v6 {
			h[6] = 44
			binary.BigEndian.PutUint16(h[4:6], uint16(8+end-off))
			frag := make([]byte, 8)
			frag[0] = 58
			bits := uint16(off)
			if more {
				bits |= 1
			}
			binary.BigEndian.PutUint16(frag[2:4], bits)
			binary.BigEndian.PutUint32(frag[4:8], 0x11223344)
			part = append(h, frag...)
		} else {
			binary.BigEndian.PutUint16(h[2:4], uint16(hlen+end-off))
			bits := uint16(off / 8)
			if more {
				bits |= 0x2000
			}
			binary.BigEndian.PutUint16(h[6:8], bits)
			h[10], h[11] = 0, 0
			var sum uint32
			for at := 0; at < hlen; at += 2 {
				sum += uint32(binary.BigEndian.Uint16(h[at : at+2]))
			}
			for sum>>16 != 0 {
				sum = (sum & 65535) + (sum >> 16)
			}
			binary.BigEndian.PutUint16(h[10:12], ^uint16(sum))
			part = h
		}
		part = append(part, body[off:end]...)
		frames = append(frames, append(bytes.Clone(ether), part...))
		off = end
	}
	return frames
}
func icmpFragmentCapture(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := pcapgo.NewWriter(&b)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for i, f := range frames {
		ci := gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+i), 0), CaptureLength: len(f), Length: len(f)}
		require.NoError(t, w.WritePacket(ci, f))
	}
	return b.Bytes()
}
func testICMPFragmentObservations(t *testing.T) {
	raw, err := trafficfixture.ReadFile("network-query-ownership/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []networkOwnershipControl }
	require.NoError(t, json.Unmarshal(raw, &doc))
	count := 0
	for _, c := range doc.Cases {
		c.Fields = networkOwnershipFieldsCurrent(t, c)
		if c.Protocol == "arp" || c.Error != "" {
			continue
		}
		input, err := trafficfixture.ReadFile("network-query-ownership/" + c.Capture)
		require.NoError(t, err)
		require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(input)))
		r, err := pcapgo.NewReader(bytes.NewReader(input))
		require.NoError(t, err)
		frame, _, err := r.ReadPacketData()
		require.NoError(t, err)
		nd := c.ID == "icmp6-nd-options"
		split := 8
		if nd {
			split = 24
		}
		orders := []string{"ordered", "reverse"}
		if c.Protocol == "icmpv6" {
			orders = append(orders, "atomic")
		}
		for _, order := range orders {
			frames := icmpFragmentFrames(t, frame, split, order == "atomic")
			refs := []uint64{1, 2}
			if order == "reverse" {
				frames[0], frames[1] = frames[1], frames[0]
				refs = []uint64{2, 1}
			}
			if order == "atomic" {
				refs = []uint64{1}
			}
			capture := icmpFragmentCapture(t, frames)
			count++
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("icmp-fragments/%s/%s/w%d/deferred%t/observe%t", c.ID, order, workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen++ }))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(capture), opts...))
							require.Len(t, events, 1)
							e := events[0]
							require.Equal(t, "network", e.Transport)
							require.Zero(t, e.ResponseTo)
							require.Zero(t, e.FlowID)
							require.Zero(t, e.TransactionID)
							require.Equal(t, "reassembled", e.SourceBytes.Kind)
							require.Len(t, e.SourceBytes.PacketRefs, len(refs))
							for i, n := range refs {
								require.Equal(t, n, e.SourceBytes.PacketRefs[i].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[i].Domain)
							}
							require.Equal(t, time.Unix(int64(99+len(frames)), 0), e.Timestamp)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.Equal(t, len(frames), seen)
							}
							if nd {
								require.Equal(t, "ip", e.Protocol)
								require.Equal(t, "malformed", e.Status)
								require.Equal(t, "NDFragmentForbidden", e.ExpertCode)
								require.Equal(t, "NDFragmentForbidden", e.Error)
								require.Nil(t, e.Raw)
								require.Empty(t, e.Session)
								require.Empty(t, e.Fields)
								require.Empty(t, e.Structured)
								fields, err := e.GetFields()
								require.Nil(t, fields)
								require.EqualError(t, err, "protocol parser: no exact rule for this message")
								decoded, err := e.Decode()
								require.Nil(t, decoded)
								require.EqualError(t, err, "protocol parser: no exact rule for this message")
								require.Empty(t, e.Source)
								require.Empty(t, e.Destination)
								require.Zero(t, stats.Messages)
								require.EqualValues(t, 1, stats.Malformed)
								return
							}
							require.Equal(t, c.Protocol, e.Protocol)
							require.Equal(t, c.Source, e.Source)
							require.Equal(t, c.Destination, e.Destination)
							require.Equal(t, "decoded", e.Status)
							require.Equal(t, "message", e.Completeness)
							require.Empty(t, e.Error)
							require.Equal(t, frame[14:], e.Raw)
							require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(frame[14:])), e.SourceBytes.SHA256)
							f, err := e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, c.Fields, f)
							rocEqualFields(t, c.Fields, e.Session)
							d, err := e.Decode()
							require.NoError(t, err)
							rocEqualFields(t, c.Fields, d["fields"].(map[string]any))
							rocEqualFields(t, c.Fields, d["session"].(map[string]any))
							require.EqualValues(t, 1, stats.Messages)
							require.EqualValues(t, 1, stats.Decoded)
							corruptNetworkObservation(f)
							corruptNetworkObservation(e.Session)
							clear(e.Raw)
							f, err = e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, c.Fields, f)
							d, err = e.Decode()
							require.NoError(t, err)
							rocEqualFields(t, c.Fields, d["session"].(map[string]any))
						})
					}
				}
			}
		}
	}
	require.Equal(t, 17, count) // four ICMPv4 x2 plus three ICMPv6 x3; ND is negative.
}
