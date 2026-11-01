package pcaputil

import (
	"context"
	"net"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestTCPAsymmetricCaptureReuseAfterObservedClose(t *testing.T) {
	for _, synack := range []bool{false, true} {
		t.Run(map[bool]string{false: "sender-FIN", true: "SYN-ACK-after-RST"}[synack], func(t *testing.T) {
			p := NewTrafficPool(context.Background())
			defer p.Close()
			var ids []uint64
			var bodies []string
			p.onFlowCreated = func(f *TrafficFlow) { ids = append(ids, f.Index) }
			p.onFlowFrameDataFrameArrived = append(p.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
				bodies = append(bodies, string(f.Payload))
			})
			ip := &layers.IPv4{SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
			feed := func(seq uint32, syn, fin, rst bool, body string) {
				tcp := &layers.TCP{SrcPort: 12345, DstPort: 389, Seq: seq, SYN: syn, ACK: synack || !syn, Ack: 1000, FIN: fin, RST: rst}
				tcp.Payload = []byte(body)
				p.Feed(nil, ip, tcp)
			}
			feed(99, true, false, false, "")
			feed(100, false, false, false, "old")
			feed(103, false, !synack, synack, "")
			feed(99, true, false, false, "") // Delayed original SYN is not a fresh generation.
			require.Len(t, ids, 1)
			feed(999, true, false, false, "")
			feed(1000, false, false, false, "new")
			require.Len(t, ids, 2)
			require.NotEqual(t, ids[0], ids[1])
			require.Equal(t, []string{"old", "new"}, bodies)
		})
	}
}
