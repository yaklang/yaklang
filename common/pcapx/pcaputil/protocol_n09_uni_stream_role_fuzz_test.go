package pcaputil

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
)

// Exercise the RFC role oracle through routing, AEAD, native preflight and
// publication. The seeded owner is not represented as a new live handshake.
func FuzzQUICNativeUniStreamRoleTransactions(f *testing.F) {
	for _, v := range [][4]byte{{0, 0xc0, 0, 0}, {1, 0xc5, 1, 7}, {2, 0xcb, 2, 31}, {0, 0x40, 3, 0}, {1, 0x45, 4, 255}, {2, 0, 5, 17}} {
		f.Add(v[0], v[1], v[2], v[3])
	}
	f.Fuzz(func(t *testing.T, cipher, flags, stream, number byte) {
		version := quicVersion1
		if flags&1 != 0 {
			version = quicVersion2
		}
		algorithm := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[int(cipher)%3]
		dir, phase := int((flags>>1)&1), int((flags>>2)&1)
		kind := []string{"STREAM", "RESET_STREAM", "STOP_SENDING"}[int(flags>>3)%3]
		uni, bad := flags&0x40 != 0, flags&0xc0 == 0xc0
		initiator := dir
		if kind == "STOP_SENDING" {
			initiator = 1 - dir
		}
		if bad {
			initiator = 1 - initiator
		}
		sid := uint64(stream)*4 + uint64(initiator)
		if uni {
			sid |= 2
		}
		a, q := n09AuditOwner(t, version, algorithm)
		opened := n09AuditDatagram(t, a, q, initiator, 0, 11, quicStreamFrame(sid, 0, false, nil))
		require.Equal(t, "decoded", opened.Status, opened.Error)
		before := *q.streams[sid]
		keys := cloneQUICKeyring(q.keys)
		peer := q.spaces[1-dir][quicSpaceApplication]
		peer.seen = append([]uint64(nil), peer.seen...)
		peer.rejected = maps.Clone(peer.rejected)
		pn := uint64(number) + 12
		plain := append([]byte{1}, n09AuditFrame(kind, sid)...)
		e := n09AuditDatagram(t, a, q, dir, phase, pn, plain)
		if !bad {
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Equal(t, phase, q.keyPhase[dir])
			return
		}
		require.Equal(t, "malformed", e.Status)
		require.Equal(t, string(ErrMalformedMessage), e.ExpertCode)
		require.Equal(t, plain, e.Session["Rejected Packet Payload"])
		require.NotContains(t, e.Session, "Frames")
		require.Equal(t, before.maxOff, q.streams[sid].maxOff)
		require.Equal(t, before.fin, q.streams[sid].fin)
		require.Equal(t, before.reset, q.streams[sid].reset)
		require.Equal(t, before.stop, q.streams[sid].stop)
		require.Equal(t, keys, q.keys)
		require.Zero(t, q.keyPhase[dir])
		require.Equal(t, peer, q.spaces[1-dir][quicSpaceApplication])
		require.Contains(t, q.spaces[dir][quicSpaceApplication].rejected, pn)
		_, err := e.GetFields()
		require.Error(t, err)
		duplicate := n09AuditDatagram(t, a, q, dir, phase, pn, plain)
		require.Equal(t, "malformed", duplicate.Status)
		require.Equal(t, keys, q.keys)
		good := n09AuditDatagram(t, a, q, dir, phase, pn+1, []byte{1})
		require.Equal(t, "decoded", good.Status, good.Error)
		require.Equal(t, phase, q.keyPhase[dir])
	})
}
