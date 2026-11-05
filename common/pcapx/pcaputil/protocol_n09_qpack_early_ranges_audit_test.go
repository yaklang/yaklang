package pcaputil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Uses v1's native AEAD dispatch+emit fixture. No test sets any early-range
// implementation field: packet type, accepted STREAM offsets and bytes are
// the only qualification inputs.
func TestN09QPACKNativeEarlyTailDoesNotQualifyCurrentPrefixGap(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
			for _, kind := range []string{"field", "encoder"} {
				t.Run(fmt.Sprintf("v%x/%x/%s", version, suite, kind), func(t *testing.T) {
					q, early, app, finish := earlyRecoveryFixture(t, version, suite, true, false)
					finish(true)
					n := n09QPACKOwner(t, q)
					capacity, encoder, field := n09QPACKFixture(0)
					sid := uint64(0)
					if kind == "encoder" {
						sid = 6
					}
					tail := n.send(t, 0, 11, 0, "0-RTT", early, sid, 100, []byte{0})
					require.Equal(t, "decoded", tail.Status)
					require.Len(t, q.streams[sid].reassembly[0].pending, 1)
					// Editable public observations cannot expand an actual early
					// byte interval to cover the never-captured preceding hole.
					tail.Session["Packet Name"] = "1-RTT"
					tail.Raw[0] ^= 1
					setting := n.send(t, 1, 12, 1, "1-RTT", q.keys.app[1][0], 3, 0, h3ControlSETTINGS(1, capacity, 7, 1))
					require.Equal(t, "decoded", setting.Status)
					insert := n.send(t, 0, 13, 2, "1-RTT", app, 6, 0, h3EncoderStream(encoder))
					require.Equal(t, "decoded", insert.Status)
					require.Equal(t, uint64(2), q.qpack.table[0].insertCount)
					message := n.send(t, 0, 14, 3, "1-RTT", app, 0, 0, h3Frame(1, field))
					require.Equal(t, "decoded", message.Status)
					require.NotContains(t, message.Session, "QPACK Field Context")
					require.Equal(t, 1, q.streams[0].h3.headers[0])
					frames := message.Session["HTTP3 Frames"].([]map[string]any)
					require.Len(t, frames, 1)
					require.Equal(t, []uint64{message.ID}, frames[0]["Byte Source"].(map[string]any)["Parent PDUs"])
					require.NotContains(t, frames[0]["Byte Source"].(map[string]any)["Parent PDUs"], tail.ID)
				})
			}
		}
	}
}

func TestN09QPACKNativeMixedFieldPrefixNeedsRememberedContext(t *testing.T) {
	for _, offset := range []uint64{2, 3} { // actual RIC first byte or Base byte
		for _, currentMax := range []uint64{0, 220} {
			t.Run(fmt.Sprintf("early%d/current%d", offset, currentMax), func(t *testing.T) {
				q, early, app, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
				finish(true)
				n := n09QPACKOwner(t, q)
				_, _, field := n09QPACKFixture(0)
				framed := h3Frame(1, field)
				part := n.send(t, 0, 11, 0, "0-RTT", early, 0, offset, framed[offset:offset+1])
				require.Equal(t, "decoded", part.Status)
				setting := n.send(t, 1, 12, 1, "1-RTT", q.keys.app[1][0], 3, 0, h3ControlSETTINGS(1, currentMax, 7, 1))
				require.Equal(t, "decoded", setting.Status)
				full := n.send(t, 0, 13, 2, "1-RTT", app, 0, 0, framed)
				n09QPACKRequireContext(t, full, "remembered SETTINGS")
				require.True(t, q.streams[0].h3.blocked[0])
				require.Zero(t, q.streams[0].h3.headers[0])
				require.NotContains(t, q.spaces[0][quicSpaceApplication].rejected, uint64(2))
				static := n.send(t, 0, 14, 3, "1-RTT", app, 4, 0, h3RequestHeaders())
				require.Equal(t, "decoded", static.Status)
				require.Equal(t, 1, q.streams[4].h3.headers[0])
			})
		}
	}
}

func TestN09QPACKNativeEarlyEncoderContinuationDoesNotBorrowCurrentMaximum(t *testing.T) {
	for _, offset := range []uint64{2, 4} {
		t.Run(fmt.Sprintf("early%d", offset), func(t *testing.T) {
			q, early, app, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
			finish(true)
			n := n09QPACKOwner(t, q)
			capacity, encoder, _ := n09QPACKFixture(0)
			framed := h3EncoderStream(encoder)
			// offset2 is a continuation of the SetCapacity integer whose
			// first byte is current; offset4 starts the first Insert.
			part := n.send(t, 0, 11, 0, "0-RTT", early, 6, offset, framed[offset:offset+1])
			require.Equal(t, "decoded", part.Status)
			setting := n.send(t, 1, 12, 1, "1-RTT", q.keys.app[1][0], 3, 0, h3ControlSETTINGS(1, capacity, 7, 1))
			require.Equal(t, "decoded", setting.Status)
			full := n.send(t, 0, 13, 2, "1-RTT", app, 6, 0, framed)
			n09QPACKRequireContext(t, full, "remembered SETTINGS")
			require.Zero(t, q.qpack.table[0].insertCount)
			if offset == 2 {
				require.Zero(t, q.qpack.table[0].capacity)
				require.Equal(t, 1, q.streams[6].h3.parsed[0])
			} else {
				require.Equal(t, capacity, q.qpack.table[0].capacity, "a wholly current capacity instruction prefix remains valid")
				require.Equal(t, 4, q.streams[6].h3.parsed[0])
				instructions := full.Session["QPACK Instructions"].([]map[string]any)
				require.Len(t, instructions, 1)
				require.Equal(t, "Set Dynamic Table Capacity", instructions[0]["Instruction"])
			}
			static := n.send(t, 0, 14, 3, "1-RTT", app, 0, 0, h3RequestHeaders())
			require.Equal(t, "decoded", static.Status)
			require.Equal(t, 1, q.streams[0].h3.headers[0])
		})
	}
}

func TestN09QPACKNativeEarlyFieldVarintContinuationStaysOpaque(t *testing.T) {
	q, early, app, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
	finish(true)
	n := n09QPACKOwner(t, q)
	// RIC encoded255 needs ff00; Base=0; relative dynamic index0. A large
	// current maximum cannot qualify an early continuation byte of that RIC.
	framed := h3Frame(1, []byte{0xff, 0, 0, 0x80})
	part := n.send(t, 0, 11, 0, "0-RTT", early, 0, 3, framed[3:4])
	require.Equal(t, "decoded", part.Status)
	setting := n.send(t, 1, 12, 1, "1-RTT", q.keys.app[1][0], 3, 0, h3ControlSETTINGS(1, 8192, 7, 1))
	require.Equal(t, "decoded", setting.Status)
	full := n.send(t, 0, 13, 2, "1-RTT", app, 0, 0, framed)
	n09QPACKRequireContext(t, full, "remembered SETTINGS")
	require.Zero(t, q.streams[0].h3.headers[0])
	require.Zero(t, q.qpack.table[0].insertCount)
}
