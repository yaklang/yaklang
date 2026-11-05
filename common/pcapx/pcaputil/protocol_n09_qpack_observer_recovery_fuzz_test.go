package pcaputil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Reuses frozen v1 n09QPACKNative helpers; run together with the v1/v2 tests.
// Bounded 1-4 field streams, 0-3 duplicates, 2 versions and 3 AEADs. Low local
// collection limits are confined to this fresh test parser; no global budget,
// production limit or previously refused owner's allowance is reset.
func FuzzN09QPACKNativeObserverRecovery(f *testing.F) {
	for _, seed := range [][3]uint8{{0, 0, 0}, {24, 8, 3}, {5, 18, 2}, {9, 36, 1}, {16, 1, 2}, {8, 0x98, 3}, {19, 0x9a, 1}, {4, 0x84, 2}} {
		f.Add(seed[0], seed[1], seed[2])
	}
	f.Fuzz(func(t *testing.T, profile, shape, duplicates uint8) {
		version := quicVersion1
		if profile&1 != 0 {
			version = quicVersion2
		}
		algorithm := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[(profile>>1)%3]
		dir, peerMax := int((profile>>3)&1), uint64((profile>>4)&1)
		knownZero, settingsLate, dropEncoder := shape&1 != 0, shape&2 != 0, shape&4 != 0
		if knownZero {
			settingsLate = false // prove a known current zero, never invent it
		}
		count, localMax := 1+int((shape>>3)&3), 16
		if shape&0x80 != 0 {
			localMax = 4
		}
		q, key := quicConformanceContext(t, version, algorithm)
		n := n09QPACKOwner(t, q)
		n.a.budget.MaxCollectionElements = localMax
		capacity, encoder, field := n09QPACKFixture(dir)
		if knownZero {
			capacity = 0
		}
		// A real local-stream refusal remains ResourceExceeded and may retire
		// the owner. Inspect the saved owner, not a fatal-cleanup nil pointer.
		localRefusal := func(e *ProtocolEvent) {
			t.Helper()
			require.Equal(t, string(ErrResourceExceeded), e.ExpertCode)
			require.NotEqual(t, "decoded", e.Status)
			require.True(t, q.retired || n.f.quic == nil)
		}
		if !settingsLate {
			setting := n.send(t, 1-dir, 10, 1, "1-RTT", key, uint64(3-dir), 0, h3ControlSETTINGS(1, capacity, 7, peerMax))
			require.Equal(t, "decoded", setting.Status)
		}
		var original []*ProtocolEvent
		var firstWire []byte
		for i := 0; i < count; i++ {
			beforeStreams := i
			if !settingsLate {
				beforeStreams++
			}
			wire := quicConformancePacket(t, key, "1-RTT", uint64(i+1), 0, quicStreamFrame(uint64(i*4), 0, false, h3Frame(1, field)))
			e := n.dispatch(t, dir, uint64(11+i), wire)
			if beforeStreams >= localMax {
				localRefusal(e)
				return
			}
			if knownZero {
				require.Equal(t, "malformed", e.Status)
				require.Equal(t, string(ErrMalformedMessage), e.ExpertCode)
				require.NotContains(t, e.Session, "Headers")
				require.False(t, q.streams[0].h3.blocked[dir])
				return
			}
			if settingsLate {
				n09QPACKRequireContext(t, e, "peer SETTINGS not observed")
			} else {
				require.Equal(t, "decoded", e.Status)
				require.Equal(t, true, e.Session["QPACK Blocked"])
				require.Contains(t, fmt.Sprint(e.Session["QPACK Blocked Scope"]), "observer")
			}
			require.NotEqual(t, string(ErrResourceExceeded), e.ExpertCode)
			require.NotContains(t, e.Session, "Headers")
			require.True(t, q.streams[uint64(i*4)].h3.blocked[dir])
			require.Zero(t, q.streams[uint64(i*4)].h3.headers[dir])
			require.NotContains(t, q.spaces[dir][quicSpaceApplication].rejected, uint64(i+1))
			original = append(original, e)
			if i == 0 {
				firstWire = wire
			}
		}
		memory := q.initialMemory()
		for i := uint8(0); i < duplicates%4; i++ {
			again := n.dispatch(t, dir, uint64(30+i), firstWire)
			require.Equal(t, true, again.Session["Duplicate"])
			require.Equal(t, memory, q.initialMemory())
			require.Zero(t, q.qpack.table[dir].insertCount)
		}
		if settingsLate {
			setting := n.send(t, 1-dir, 40, 1, "1-RTT", key, uint64(3-dir), 0, h3ControlSETTINGS(1, capacity, 7, peerMax))
			if count >= localMax {
				localRefusal(setting)
				return
			}
			require.Equal(t, "decoded", setting.Status)
			rows, ok := setting.Session["QPACK Resumed Field Contexts"].([]map[string]any)
			require.True(t, ok)
			require.Len(t, rows, count)
			for i, row := range rows {
				require.Equal(t, "context-required", row["Status"])
				require.Equal(t, original[i].ID, row["Reprocessed From PDU"])
				require.Equal(t, h3Frame(1, field), row["Field Section Frame Bytes"])
				require.Equal(t, []uint64{original[i].ID}, row["Byte Source"].(map[string]any)["Parent PDUs"])
			}
		}
		if !dropEncoder {
			insert := n.send(t, dir, 50, 20, "1-RTT", key, uint64(6+dir), 0, h3EncoderStream(encoder))
			if count+1 >= localMax {
				localRefusal(insert)
				return
			}
			require.Equal(t, "decoded", insert.Status)
			require.Equal(t, uint64(2), q.qpack.table[dir].insertCount)
			resumed, ok := insert.Session["QPACK Resumed Streams"].([]map[string]any)
			require.True(t, ok)
			require.Len(t, resumed, count)
			for i, row := range resumed {
				require.Equal(t, original[i].ID, row["Blocked PDU ID"])
				require.Equal(t, 1, q.streams[uint64(i*4)].h3.headers[dir])
				frames := row["HTTP3 Frames"].([]map[string]any)
				require.Len(t, frames, 1)
				require.Equal(t, []uint64{original[i].ID}, frames[0]["Byte Source"].(map[string]any)["Parent PDUs"])
			}
		}
		static, streamsBefore := h3RequestHeaders(), count+1
		if dir == 1 {
			static = h3ResponseHeaders()
		}
		if !dropEncoder {
			streamsBefore++
		}
		last := n.send(t, dir, 60, 21, "1-RTT", key, 16, 0, static)
		if streamsBefore >= localMax {
			localRefusal(last)
			return
		}
		require.Equal(t, "decoded", last.Status)
		require.False(t, q.retired)
		require.Equal(t, 1, q.streams[16].h3.headers[dir])
		if dropEncoder {
			for i := 0; i < count; i++ {
				require.True(t, q.streams[uint64(i*4)].h3.blocked[dir])
				require.Zero(t, q.streams[uint64(i*4)].h3.headers[dir])
			}
		}
	})
}
