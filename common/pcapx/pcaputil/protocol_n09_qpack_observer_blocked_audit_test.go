package pcaputil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Requires the separately frozen v1 native datagram helpers/test file.
// QPACK insertions come from an independent RFC fixture, not parser output.
// A capture reordering/drop is not evidence of endpoint delivery order.
func TestN09QPACKNativeObserverMissingInsertionsCannotUsePeerBlockedLimit(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, dir := range []int{0, 1} {
				for _, peerMax := range []uint64{0, 1} {
					t.Run(fmt.Sprintf("v%x/%s/dir%d/max%d", version, algorithm, dir, peerMax), func(t *testing.T) {
						q, key := quicConformanceContext(t, version, algorithm)
						n := n09QPACKOwner(t, q)
						capacity, encoder, field := n09QPACKFixture(dir)
						setting := n.send(t, 1-dir, 10, 1, "1-RTT", key, uint64(3-dir), 0, h3ControlSETTINGS(1, capacity, 7, peerMax))
						require.Equal(t, "decoded", setting.Status)
						ids := []uint64{0, 4}
						var originals []*ProtocolEvent
						for i, sid := range ids {
							e := n.send(t, dir, uint64(11+i), uint64(i+1), "1-RTT", key, sid, 0, h3Frame(1, field))
							require.Equal(t, "decoded", e.Status, "authenticated packet/framing observation; field section is not yet decoded")
							require.Empty(t, e.ExpertCode)
							require.Equal(t, true, e.Session["QPACK Blocked"])
							require.Contains(t, fmt.Sprint(e.Session["QPACK Blocked Scope"]), "observer")
							require.Equal(t, peerMax, e.Session["Peer QPACK Blocked Streams"])
							require.NotContains(t, e.Session, "Headers")
							require.NotContains(t, e.Session, "HTTP3 Frames")
							require.Same(t, q, n.f.quic)
							require.False(t, q.retired)
							require.False(t, q.closed)
							require.True(t, q.streams[sid].h3.blocked[dir])
							require.Zero(t, q.streams[sid].h3.headers[dir])
							require.Zero(t, q.streams[sid].h3.fieldFailure[dir])
							require.NotContains(t, q.spaces[dir][quicSpaceApplication].rejected, uint64(i+1))
							originals = append(originals, e)
						}
						// An independent later field can still be decoded while the
						// two dynamic sections are absent from this capture's table.
						static := h3RequestHeaders()
						if dir == 1 {
							static = h3ResponseHeaders()
						}
						independent := n.send(t, dir, 13, 3, "1-RTT", key, 8, 0, static)
						require.Equal(t, "decoded", independent.Status)
						require.Equal(t, 1, q.streams[8].h3.headers[dir])
						require.Zero(t, q.qpack.table[dir].insertCount)
						insert := n.send(t, dir, 14, 4, "1-RTT", key, uint64(6+dir), 0, h3EncoderStream(encoder))
						require.Equal(t, "decoded", insert.Status)
						require.Equal(t, uint64(2), q.qpack.table[dir].insertCount)
						resumed, ok := insert.Session["QPACK Resumed Streams"].([]map[string]any)
						require.True(t, ok)
						require.Len(t, resumed, 2)
						for i, sid := range ids {
							require.False(t, q.streams[sid].h3.blocked[dir])
							require.Equal(t, 1, q.streams[sid].h3.headers[dir])
							require.Equal(t, originals[i].ID, resumed[i]["Blocked PDU ID"])
							frames := resumed[i]["HTTP3 Frames"].([]map[string]any)
							require.Len(t, frames, 1)
							require.Equal(t, []uint64{originals[i].ID}, frames[0]["Byte Source"].(map[string]any)["Parent PDUs"])
							require.Equal(t, true, originals[i].Session["QPACK Blocked"], "published history remains immutable")
						}
					})
				}
			}
		}
	}
}

func TestN09QPACKNativeObserverUnobservedInsertionsStayBoundedAndOwnerUsable(t *testing.T) {
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	n := n09QPACKOwner(t, q)
	capacity, _, field := n09QPACKFixture(0)
	require.Equal(t, "decoded", n.send(t, 1, 10, 1, "1-RTT", key, 3, 0, h3ControlSETTINGS(1, capacity)).Status)
	wire := quicConformancePacket(t, key, "1-RTT", 1, 0, quicStreamFrame(0, 0, false, h3Frame(1, field)))
	first := n.dispatch(t, 0, 11, wire)
	require.Equal(t, "decoded", first.Status)
	require.Equal(t, true, first.Session["QPACK Blocked"])
	ownedBytes := q.initialMemory()
	for i := uint64(12); i < 20; i++ {
		again := n.dispatch(t, 0, i, wire)
		require.Equal(t, "decoded", again.Status)
		require.Equal(t, true, again.Session["Duplicate"])
		require.Equal(t, ownedBytes, q.initialMemory())
		require.Zero(t, q.qpack.table[0].insertCount)
		require.True(t, q.streams[0].h3.blocked[0])
		require.Zero(t, q.streams[0].h3.headers[0])
	}
	later := n.send(t, 0, 20, 2, "1-RTT", key, 4, 0, h3RequestHeaders())
	require.Equal(t, "decoded", later.Status)
	require.Same(t, q, n.f.quic)
	require.False(t, q.retired)
	require.Equal(t, 1, q.streams[4].h3.headers[0])
}

func TestN09QPACKNativeObserverKeepsKnownZeroCapacityContradiction(t *testing.T) {
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	n := n09QPACKOwner(t, q)
	_, _, field := n09QPACKFixture(0)
	require.Equal(t, "decoded", n.send(t, 1, 10, 1, "1-RTT", key, 3, 0, h3ControlSETTINGS(1, 0, 7, 8)).Status)
	bad := n.send(t, 0, 11, 1, "1-RTT", key, 0, 0, h3Frame(1, field))
	require.Equal(t, "malformed", bad.Status)
	require.Equal(t, string(ErrMalformedMessage), bad.ExpertCode)
	require.NotContains(t, bad.Session, "QPACK Blocked")
	require.NotContains(t, bad.Session, "Headers")
	require.False(t, q.streams[0].h3.blocked[0])
	require.Zero(t, q.qpack.table[0].insertCount)
}
