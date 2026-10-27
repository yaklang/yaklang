package pcaputil

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func FuzzTCPOpeningObservationSourceIsolation(f *testing.F) {
	for _, base := range []uint32{0, ^uint32(0), 0xcfffffff} {
		for close := uint8(0); close < 4; close++ {
			f.Add(base, uint8(7), close, false, false)
			f.Add(base, uint8(2), close, true, true)
		}
	}
	f.Fuzz(func(t *testing.T, base uint32, repeats, close uint8, reversed, ipv6 bool) {
		c, s, nc, ns := base, base+0x10000000, base+0x30000000, base+0x50000000
		var steps []tcpStep
		var owners []int
		add := func(owner int, step tcpStep) { steps = append(steps, step); owners = append(owners, owner) }
		add(0, tcpStep{seq: c, syn: true})
		add(0, tcpStep{seq: s, ack: c + 1, syn: true, synack: true, reverse: true})
		add(0, tcpStep{seq: c + 1, ack: s + 1, data: "old"})
		for i := 0; i <= int(repeats%8); i++ {
			add(1, tcpStep{seq: nc, syn: true, reverse: reversed})
		}
		switch close % 4 {
		case 1:
			add(0, tcpStep{seq: c + 4, ack: s + 1, rst: true})
		case 2:
			add(0, tcpStep{seq: c + 4, ack: s + 1, fin: true})
			add(0, tcpStep{seq: s + 1, ack: c + 5, fin: true, reverse: true})
		case 3:
			add(0, tcpStep{seq: c + 4, ack: s + 1, fin: true})
			add(1, tcpStep{seq: nc, syn: true, reverse: reversed})
		}
		add(1, tcpStep{seq: ns, ack: nc + 1, syn: true, synack: true, reverse: !reversed})
		add(1, tcpStep{seq: nc + 1, ack: ns + 1, data: "new", reverse: reversed})
		add(1, tcpStep{seq: ns + 1, ack: nc + 4, data: "reply", reverse: !reversed})
		var mu sync.Mutex
		var ids []uint64
		var payloads []string
		observed := map[uint64]uint64{}
		pools := map[*TrafficPool]bool{}
		err := ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 135, ipv6, true)), WithTCPReassemblyStream(64),
			WithOnTrafficFlowCreated(func(f *TrafficFlow) { mu.Lock(); defer mu.Unlock(); ids = append(ids, f.Index); pools[f.pool] = true }),
			WithOnTrafficFlowPacket(func(f *TrafficFlow, _ *TrafficConnection, refs []PacketReference, _ time.Time) {
				mu.Lock()
				defer mu.Unlock()
				for _, ref := range refs {
					require.NotContains(t, observed, ref.Number)
					observed[ref.Number] = f.Index
				}
			}), WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, frame *TrafficFrame) {
				mu.Lock()
				defer mu.Unlock()
				payloads = append(payloads, string(frame.Payload))
			}))
		require.NoError(t, err)
		require.Len(t, ids, 2)
		require.Len(t, observed, len(steps))
		for i, owner := range owners {
			require.Equal(t, ids[owner], observed[uint64(i+1)], "original frame ownership")
		}
		require.Equal(t, []string{"old", "new", "reply"}, payloads, "control retries never replay application bytes")
		generationPoolsReleased(t, pools)
	})
}

func TestTCPOpeningMetadataSharesDirectionBudgetWithQueuedBytes(t *testing.T) {
	p := newTrafficPool(nil, TCPReassemblyOptions{MaxPendingSegments: 2, MaxTotalPendingSegments: 8})
	defer p.Close()
	var old *TrafficFlow
	var body string
	p.onFlowCreated = func(f *TrafficFlow) {
		if old == nil {
			old = f
		}
	}
	p.onFlowFrameDataFrameArrived = append(p.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { body += string(f.Payload) })
	openingBudgetFeed(p, false, 99, 0, true, false, false, "", 1)
	openingBudgetFeed(p, true, 499, 100, true, true, false, "", 2)
	openingBudgetFeed(p, false, 110, 500, false, false, false, "later", 3)
	openingBudgetFeed(p, false, 999, 0, true, false, false, "", 4)
	require.Equal(t, 2, p.pendingSegments)
	openingBudgetFeed(p, false, 999, 0, true, false, false, "", 5)
	require.Equal(t, 1, p.pendingSegments)
	require.False(t, old.IsClosed())
	require.False(t, old.reassemblyIncomplete)
	openingBudgetFeed(p, false, 100, 500, false, false, false, "0123456789", 6)
	require.Equal(t, "0123456789later", body)
	require.Zero(t, p.pendingSegments)
	require.Zero(t, p.pendingBytes)
}
