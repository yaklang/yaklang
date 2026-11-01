package pcaputil

import (
	"bytes"
	"container/list"
	"crypto/aes"
	"crypto/cipher"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/chacha20poly1305"
	"testing"
	"time"
)

func epochQUICPacket(t *testing.T, k *quicTrafficKeys, cid []byte, pn uint64, phase, pnLen int) []byte {
	t.Helper()
	head := append([]byte{0x40 | byte(phase<<2) | byte(pnLen-1)}, cid...)
	for n := pnLen - 1; n >= 0; n-- {
		head = append(head, byte(pn>>uint(8*n)))
	}
	var a cipher.AEAD
	var err error
	if k.aead == quicAEADChaCha20Poly1305 {
		a, err = chacha20poly1305.New(k.key)
	} else {
		b, e := aes.NewCipher(k.key)
		require.NoError(t, e)
		a, err = cipher.NewGCM(b)
	}
	require.NoError(t, err)
	w := append(bytes.Clone(head), a.Seal(nil, quicNonce(k.iv, pn), append([]byte{1}, make([]byte, 23)...), head)...)
	mask, err := quicHeaderMask(k, w[1+len(cid)+4:1+len(cid)+20])
	require.NoError(t, err)
	w[0] ^= mask[0] & 0x1f
	for n := 0; n < pnLen; n++ {
		w[1+len(cid)+n] ^= mask[1+n]
	}
	return w
}

func TestQUICRetiredCIDLatePacketsRequireAuthentication(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	defer a.closeUDPSessions()
	k, err := quicDeriveTraffic(bytes.Repeat([]byte{3}, 32), quicAEADAES128GCM)
	require.NoError(t, err)
	next, err := quicNextPhase(k)
	require.NoError(t, err)
	cid := []byte("retired!")
	q := &binQUIC{native: true, byteLimit: 4096, keys: &quicKeyring{}, cids: map[string]int{string(cid): 1}}
	q.keys.app[0] = [2]*quicTrafficKeys{k, next}
	q.cidSequences[1] = map[uint64]string{0: string(cid)}
	require.NoError(t, q.applyFrames(0, 2, []map[string]any{{"Frame Type": "RETIRE_CONNECTION_ID", "Sequence": uint64(0)}}, map[string]any{}, 8))
	require.NotContains(t, q.cids, string(cid))
	f := &binFlow{a: a, id: 91, protocol: "quic", quic: q}
	key := binUDPKey{"old-client", "old-server", CaptureDomain{}}
	s := &binUDPStore{entries: map[binUDPKey]*list.Element{}}
	s.entries[key] = s.lru.PushBack(&binUDPEntry{key: key, flow: f, touched: time.Unix(1, 0)})
	a.udpSessions = s
	w := epochQUICPacket(t, k, cid, 8, 0, 2)
	bad := bytes.Clone(w)
	bad[len(bad)-1] ^= 1
	base := &ProtocolEvent{Source: "new-client", Destination: "new-server", Timestamp: time.Unix(2, 0)}
	events, handled := a.decodeQUICDatagram(base, bad)
	require.False(t, handled)
	require.Empty(t, events)
	require.False(t, q.spaces[0][quicSpaceApplication].init)
	events, handled = a.decodeQUICDatagram(base, w)
	require.True(t, handled)
	require.Len(t, events, 1)
	require.EqualValues(t, 91, events[0].FlowID)
	require.Equal(t, true, events[0].Session["Retired Destination CID"])
	require.Equal(t, true, events[0].Session["Authentication Verified"])
	require.NotContains(t, q.cids, string(cid), "late packets must not reactivate the retired route")
	base.Domain.Interface = 1
	_, handled = a.decodeQUICDatagram(base, w)
	require.False(t, handled)
	for seq := uint64(1); seq < 10; seq++ {
		q.rememberRetiredCID(1, seq, string([]byte{byte(seq)}), 3)
	}
	require.Len(t, q.cidRetired[1], 3)
}

func TestQUICAuthenticatedPNWindowAndKeyEpochRecovery(t *testing.T) {
	for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
		t.Run(algorithm, func(t *testing.T) {
			n := 32
			if algorithm == quicAEADAES256GCM {
				n = 48
			}
			k, err := quicDeriveTraffic(bytes.Repeat([]byte{2}, n), algorithm)
			require.NoError(t, err)
			next, err := quicNextPhase(k)
			require.NoError(t, err)
			cid := []byte("serverid")
			q := &binQUIC{native: true, keys: &quicKeyring{}, cids: map[string]int{string(cid): 1}}
			q.keys.app[0] = [2]*quicTrafficKeys{k, next}
			// One-byte PN is ambiguous after loss of 768 packets. Only verified
			// authentication, not the unauthenticated header, resolves that window.
			w := epochQUICPacket(t, k, cid, 769, 0, 1)
			bad := bytes.Clone(w)
			bad[len(bad)-1] ^= 1
			_, err = q.consume(0, bad, 64)
			require.Error(t, err)
			require.False(t, q.spaces[0][quicSpaceApplication].init)
			require.Zero(t, q.keyPhase[0])
			info, err := q.consume(0, w, 64)
			require.NoError(t, err)
			require.EqualValues(t, 769, info["Packet Number"])
			info, err = q.consume(0, epochQUICPacket(t, next, cid, 771, 1, 2), 64)
			require.NoError(t, err)
			require.Equal(t, 1, info["Key Phase"])
			// Reordered valid previous-phase packet cannot roll keys backwards.
			info, err = q.consume(0, epochQUICPacket(t, k, cid, 770, 0, 2), 64)
			require.NoError(t, err)
			require.Equal(t, 1, q.keyPhase[0])
			require.EqualValues(t, 771, q.spaces[0][quicSpaceApplication].largest)
			info, err = q.consume(0, w, 64)
			require.NoError(t, err)
			require.Equal(t, true, info["Duplicate"])
			second, err := quicNextPhase(next)
			require.NoError(t, err)
			info, err = q.consume(0, epochQUICPacket(t, second, cid, 772, 0, 2), 64)
			require.NoError(t, err)
			require.Equal(t, 0, q.keyPhase[0])
			require.Equal(t, true, info["Authentication Verified"])
		})
	}
}
