package pcaputil

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Minimal artificial controls use protocol facts independently checked against
// Scapy's valid DoIP flow. Original captures are retained outside the repo.
func TestDoIPNativeIngressMinimalControls(t *testing.T) {
	for _, chunk := range []int{1, 7, 1000} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("chunk%d-deferred%v", chunk, deferred), func(t *testing.T) {
				steps := []tcpStep{{seq: 99, syn: true}, {seq: 199, syn: true, reverse: true}}
				seq := [2]uint32{100, 200}
				wires := []string{
					"02fd00050000000b0e80000000000000000000",
					"02fd0006000000090e8040101000000000",
					"02fd8001000000060e8040101003",
					"02fd80020000000540100e8000",
					"02fd80010000000a40100e805003003201f4",
					"02fd000700000000", "02fd0008000000020e80",
				}
				dirs := []int{0, 1, 0, 1, 1, 1, 0}
				for i, h := range wires {
					w, err := hex.DecodeString(h)
					require.NoError(t, err)
					dir := dirs[i]
					for len(w) > 0 {
						n := min(len(w), chunk)
						steps = append(steps, tcpStep{seq: seq[dir], data: string(w[:n]), reverse: dir == 1})
						seq[dir] += uint32(n)
						w = w[n:]
					}
				}
				steps = append(steps, tcpStep{seq: seq[0], fin: true}, tcpStep{seq: seq[1], fin: true, reverse: true})
				var es []*ProtocolEvent
				var stats ProtocolStats
				require.NoError(t, ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 36964, false, true)), WithTCPReassemblyWorkers(1), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
				require.Zero(t, stats.BufferedBytes)
				var ds []*ProtocolEvent
				for _, e := range es {
					if e.Protocol == "doip" && (e.Status == "decoded" || e.Status == "deferred") {
						ds = append(ds, e)
					}
					if e.Protocol == "doip" {
						require.NotEqual(t, "malformed", e.Status, e.Error)
						require.NotEqual(t, "context-required", e.Status, e.Error)
					}
				}
				require.Len(t, ds, 7, "all complete DoIP messages must be natively decoded on nonstandard port")
				for i, e := range ds {
					require.NotEmpty(t, e.SourceBytes.PacketRefs)
					d, err := e.Decode()
					require.NoError(t, err)
					fields, ok := d["fields"].(map[string]any)
					require.True(t, ok)
					require.Equal(t, e.Session["Packet Name"], fields["Packet Name"])
					if i == 4 {
						require.Equal(t, 50, fields["P2 Server Max Milliseconds"])
						require.Equal(t, 5000, fields["P2 Star Server Max Milliseconds"])
						require.Equal(t, "matched", fields["Association"])
						require.Equal(t, true, fields["Diagnostic Acknowledged"])
					}
				}
				require.Equal(t, 0, ds[6].Session["Outstanding Requests"])
				ds[0].Raw[8] ^= 0xff
				require.Equal(t, 0x0e80, ds[1].Session["Tester Address"])
			})
		}
	}
}

func TestDoIPNativeUDPPortDoesNotGrantTCPContext(t *testing.T) {
	w, err := hex.DecodeString("02fd0005000000070e800000000000")
	require.NoError(t, err)
	var es []*ProtocolEvent
	pcap := t20UDPCapture(t, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), 41707, 13400, w)
	require.NoError(t, ReplayPcap(bytes.NewReader(pcap), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) })))
	for _, e := range es {
		require.NotEqual(t, "doip", e.Protocol, "TCP routing context must not be inferred for UDP")
	}
}

// The first 7-byte arrival is carrier evidence only. It must not be selected
// as length-prefixed DNS before the complete routing header/body arrives.
func TestDoIPRoutingAdmissionChunkParity(t *testing.T) {
	steps := []sessionStep{{0, []byte{2, 253, 0, 5, 0, 0, 0, 7, 14, 128, 0, 0, 0, 0, 0}}}
	for _, chunk := range []int{1, 7, 15} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
		require.NoError(t, err)
		var es []*ProtocolEvent
		for _, step := range steps {
			for at := 0; at < len(step.wire); {
				n := min(chunk, len(step.wire)-at)
				r := s.Feed(step.dir, time.Unix(1, 0), step.wire[at:at+n])
				es = append(es, r.Events...)
				at += n
			}
		}
		es = append(es, s.Close("test")...)
		require.Zero(t, s.Stats().BufferedBytes)
		require.NotEmpty(t, es)
		require.Equal(t, "doip", es[0].Protocol)
		require.Equal(t, "decoded", es[0].Status)
	}
}
