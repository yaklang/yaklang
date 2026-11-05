package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzN09EarlyQPACKByteQualification(f *testing.F) {
	for _, seed := range [][4]byte{{0, 0, 100, 0}, {1, 0, 3, 0}, {2, 4, 2, 0}, {0, 8, 4, 0}, {1, 9, 100, 0}, {2, 13, 2, 1}} {
		f.Add(seed[0], seed[1], seed[2], seed[3])
	}
	f.Fuzz(func(t *testing.T, cipher, flags, offset, width byte) {
		version := quicVersion1
		if flags&1 != 0 {
			version = quicVersion2
		}
		suite := []uint16{0x1301, 0x1302, 0x1303}[int(cipher)%3]
		q, early, app, finish := earlyRecoveryFixture(t, version, suite, true, false)
		finish(true)
		n := n09QPACKOwner(t, q)
		capacity, encoder, field := n09QPACKFixture(0)
		encoderCase, zero := flags&8 != 0, flags&4 != 0
		if zero {
			capacity = 0
		}
		sid, data := uint64(0), h3Frame(1, field)
		if encoderCase {
			sid, data = 6, h3EncoderStream(encoder)
		}
		at, count := int(offset)%(len(data)+1), 1+int(width)%3
		part := []byte{0}
		if offset&0x40 != 0 {
			at = 100
		} else {
			if at == len(data) {
				at--
			}
			count = min(count, len(data)-at)
			part = data[at : at+count]
		}
		first := n.send(t, 0, 11, 0, "0-RTT", early, sid, uint64(at), part)
		require.Contains(t, []string{"decoded", "context-required"}, first.Status)
		setting := n.send(t, 1, 12, 1, "1-RTT", q.keys.app[1][0], 3, 0, h3ControlSETTINGS(1, capacity, 7, 1))
		require.Equal(t, "decoded", setting.Status)
		if !encoderCase && !zero {
			insert := n.send(t, 0, 13, 2, "1-RTT", app, 6, 0, h3EncoderStream(encoder))
			require.Equal(t, "decoded", insert.Status)
		}
		full := n.send(t, 0, 14, 3, "1-RTT", app, sid, 0, data)
		if encoderCase {
			// Literal fixture boundaries: stream type [0,1), capacity
			// [1,4), first Insert [4,21), second Insert [21,35).
			earlyInstruction, priorInsertions := -1, uint64(0)
			for _, extent := range [][3]int{{1, 4, 0}, {4, 21, 0}, {21, 35, 1}} {
				if at < extent[1] && at+len(part) > extent[0] {
					earlyInstruction, priorInsertions = extent[0], uint64(extent[2])
					break
				}
			}
			if zero && earlyInstruction != 1 {
				// A wholly current SetCapacity(220) already violates the
				// observed zero maximum before a later early instruction.
				require.Equal(t, "malformed", full.Status)
			} else if earlyInstruction >= 0 {
				n09QPACKRequireContext(t, full, "remembered SETTINGS")
				require.Equal(t, priorInsertions, q.qpack.table[0].insertCount)
			} else {
				require.Equal(t, "decoded", full.Status)
				require.Equal(t, uint64(2), q.qpack.table[0].insertCount)
			}
		} else if at < 4 && at+len(part) > 2 {
			n09QPACKRequireContext(t, full, "remembered SETTINGS")
			require.Zero(t, q.streams[0].h3.headers[0])
		} else if zero {
			require.Equal(t, "malformed", full.Status)
			require.Equal(t, string(ErrMalformedMessage), full.ExpertCode)
		} else {
			require.Equal(t, "decoded", full.Status)
			require.Equal(t, 1, q.streams[0].h3.headers[0])
		}
	})
}
