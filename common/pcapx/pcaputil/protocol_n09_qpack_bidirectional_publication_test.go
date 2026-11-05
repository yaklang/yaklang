package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// One late datagram can deliver peer SETTINGS and this sender's encoder
// prefix. Both directions' recovered publications belong to that datagram,
// while their field bytes still belong to the earlier blocked observations.
func n09QPACKBidirectionalPublication(t *testing.T, version uint32, algorithm string, dir int, controlFirst bool, split int) {
	t.Helper()
	q, key := quicConformanceContext(t, version, algorithm)
	n := n09QPACKOwner(t, q)
	capacity, encoder, field := n09QPACKFixture(dir)
	otherCapacity, otherEncoder, otherField := n09QPACKFixture(1 - dir)
	require.Equal(t, "decoded", n.send(t, 1-dir, 10, 1, "1-RTT", key, uint64(3-dir), 0, h3ControlSETTINGS(1, capacity, 7, 4)).Status)
	blockedEncoder := n.send(t, 1-dir, 11, 2, "1-RTT", key, uint64(7-dir), 0, h3EncoderStream(otherEncoder))
	n09QPACKRequireContext(t, blockedEncoder, "peer SETTINGS not observed")
	a := n.send(t, dir, 12, 3, "1-RTT", key, 0, 0, h3Frame(1, field))
	b := n.send(t, 1-dir, 13, 4, "1-RTT", key, 0, 0, h3Frame(1, otherField))
	require.True(t, q.streams[0].h3.blocked[dir])
	require.True(t, q.streams[0].h3.blocked[1-dir])
	aBefore, bBefore := cloneSession(a.Session), cloneSession(b.Session)
	encoded := h3EncoderStream(encoder)
	if split > 0 {
		split = 1 + (split-1)%(len(encoded)-1)
		prefix := n.send(t, dir, 14, 5, "1-RTT", key, uint64(6+dir), 0, encoded[:split])
		require.Equal(t, "decoded", prefix.Status)
	}
	settings := quicStreamFrame(uint64(2+dir), 0, false, h3ControlSETTINGS(1, otherCapacity, 7, 4))
	own := quicStreamFrame(uint64(6+dir), uint64(split), false, encoded[split:])
	plain := append(bytes.Clone(own), settings...)
	if controlFirst {
		plain = append(bytes.Clone(settings), own...)
	}
	wire := quicConformancePacket(t, key, "1-RTT", 6, 0, plain)
	event := n.dispatch(t, dir, 15, wire)
	require.Equal(t, "decoded", event.Status)
	rows, ok := event.Session["QPACK Resumed Streams"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, rows, 2, "both directions survive one packet's recovery publication")
	seen := map[uint64]uint64{}
	for _, row := range rows {
		frames := row["HTTP3 Frames"].([]map[string]any)
		require.Len(t, frames, 1)
		frame := frames[0]
		require.Equal(t, "HEADERS", frame["Frame Type"])
		d := frame["Direction"].(uint64)
		id := a.ID
		if int(d) != dir {
			id = b.ID
		}
		require.Equal(t, id, row["Blocked PDU ID"])
		require.Equal(t, []uint64{id}, frame["Byte Source"].(map[string]any)["Parent PDUs"])
		seen[d] = id
	}
	require.Len(t, seen, 2)
	require.Equal(t, aBefore, a.Session)
	require.Equal(t, bBefore, b.Session)
	require.False(t, q.streams[0].h3.blocked[0])
	require.False(t, q.streams[0].h3.blocked[1])
	require.Equal(t, [2]int{1, 1}, q.streams[0].h3.headers)
	duplicate := n.dispatch(t, dir, 16, wire)
	require.NotContains(t, duplicate.Session, "QPACK Resumed Streams")
}

func TestN09QPACKBidirectionalResumedPublication(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for dir := 0; dir < 2; dir++ {
				for _, controlFirst := range []bool{false, true} {
					for _, split := range []int{0, 1, 3, 17} {
						t.Run(fmt.Sprintf("v%x/%s/dir%d/controlFirst%v/split%d", version, algorithm, dir, controlFirst, split), func(t *testing.T) {
							n09QPACKBidirectionalPublication(t, version, algorithm, dir, controlFirst, split)
						})
					}
				}
			}
		}
	}
}

func FuzzN09QPACKBidirectionalPublication(f *testing.F) {
	for _, seed := range [][2]uint8{{0, 0}, {1, 1}, {2, 3}, {3, 17}, {4, 31}, {5, 63}, {6, 127}, {7, 255}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, shape, split uint8) {
		version := quicVersion1
		if shape&1 != 0 {
			version = quicVersion2
		}
		algorithm := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[int(shape>>1)%3]
		n09QPACKBidirectionalPublication(t, version, algorithm, int(shape>>3)&1, shape&16 != 0, int(split))
	})
}
