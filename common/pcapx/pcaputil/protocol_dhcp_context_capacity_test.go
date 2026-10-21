package pcaputil

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
)

func TestDHCPContextCapacity(t *testing.T) {
	_, wires := dhcpOwnedInputs(t)
	other := bytes.Clone(wires[0])
	other[33] ^= 1
	capture := dhcpClockCapture(t, []dhcpClockPacket{{false, false, 100, wires[0]}, {false, false, 1000, other}, {false, true, 101, wires[1]}})
	for _, deferred := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "deferred"}[deferred], func(t *testing.T) {
			var events []*ProtocolEvent
			a, err := NewPacketAnalyzer(WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
			require.NoError(t, err)
			a.conf.binParser.budget.MaxCollectionElements = 1
			r, err := NewCaptureReader(bytes.NewReader(capture))
			require.NoError(t, err)
			for {
				raw, ci, err := r.ReadPacketData()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				require.NoError(t, a.Feed(raw, ci, r.LinkType()))
				clear(raw)
				a.conf.binParser.dnsMu.Lock()
				count := len(a.conf.binParser.lan.pending) + len(a.conf.binParser.dns.pending)
				a.conf.binParser.dnsMu.Unlock()
				require.LessOrEqual(t, count, 1)
			}
			require.Len(t, events, 3)
			for i, e := range events {
				want := dhcpOwnedFields(0)
				wire := wires[0]
				if i == 1 {
					want["CHADDR"].([]byte)[5] ^= 1
					wire = other
				}
				if i == 2 {
					want = dhcpOwnedFields(1)
					wire = wires[1]
				}
				require.Equal(t, wire, e.Raw)
				require.Equal(t, want, e.Session)
				f, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, want, f)
			}
			require.Equal(t, events[0].ID, events[0].TransactionID)
			require.Zero(t, events[1].TransactionID)
			require.Zero(t, events[1].ResponseTo)
			require.Equal(t, events[0].ID, events[2].ResponseTo)
			require.Equal(t, events[0].ID, events[2].TransactionID)
			require.NoError(t, a.Close())
			require.NoError(t, a.Close())
			require.Len(t, events, 3)
			require.Zero(t, a.conf.binParser.buffered.Load())
		})
	}
}
