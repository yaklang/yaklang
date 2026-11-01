package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func versionedTestInitial(t *testing.T, version uint32, seed, dcid, scid, token []byte, pn byte, dir int, tokenWidths ...int) []byte {
	t.Helper()
	keys, err := quicInitialKeysVersion(seed, version)
	require.NoError(t, err)
	first := byte(0xc0)
	if version == quicVersion2 {
		first = 0xd0
	}
	header := binary.BigEndian.AppendUint32([]byte{first}, version)
	header = append(header, byte(len(dcid)))
	header = append(header, dcid...)
	header = append(header, byte(len(scid)))
	header = append(header, scid...)
	lengthWire := quicPutVarint(uint64(len(token)))
	if len(tokenWidths) > 0 {
		width := tokenWidths[0]
		prefix, ok := map[int]byte{1: 0, 2: 1, 4: 2, 8: 3}[width]
		require.True(t, ok)
		require.GreaterOrEqual(t, width, len(lengthWire))
		lengthWire = binary.BigEndian.AppendUint64(nil, uint64(len(token)))[8-width:]
		lengthWire[0] |= prefix << 6
	}
	header = append(header, lengthWire...)
	header = append(header, token...)
	header = append(header, 41)
	pnAt := len(header)
	header = append(header, pn)
	k := keys[dir]
	block, err := aes.NewCipher(k.key)
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	wire := append(bytes.Clone(header), aead.Seal(nil, quicNonce(k.iv, uint64(pn)), append([]byte{1}, make([]byte, 23)...), header)...)
	mask, err := quicHeaderMask(k, wire[pnAt+4:pnAt+20])
	require.NoError(t, err)
	wire[0] ^= mask[0] & 15
	wire[pnAt] ^= mask[1]
	return wire
}

func versionedTestRetry(version uint32, original, dcid, scid, token []byte) []byte {
	first := byte(0xf0)
	if version == quicVersion2 {
		first = 0xc0
	}
	wire := binary.BigEndian.AppendUint32([]byte{first}, version)
	wire = append(wire, byte(len(dcid)))
	wire = append(wire, dcid...)
	wire = append(wire, byte(len(scid)))
	wire = append(wire, scid...)
	wire = append(wire, token...)
	return append(wire, quicRetryTagVersion(original, wire, version)...)
}

func TestQUICRetryZeroCIDReusedTupleAndLostClientInitialRecovery(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, lost := range []bool{false, true} {
			c := NewDefaultConfig()
			require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
			require.NoError(t, c.prepareBinParser())
			a := c.binParser
			base := &ProtocolEvent{Source: "client", Destination: "server", Transport: "udp", Timestamp: time.Unix(1, 0)}
			send := func(dir int, wire []byte) *ProtocolEvent {
				base.Source, base.Destination = "client", "server"
				if dir == 1 {
					base.Source, base.Destination = base.Destination, base.Source
				}
				base.Timestamp = base.Timestamp.Add(time.Second)
				es, handled := a.decodeQUICDatagram(base, wire)
				require.True(t, handled)
				require.Len(t, es, 1)
				return es[0]
			}
			old, original, retryCID, token := []byte("old-seed"), []byte("new-seed"), []byte("retry-id"), []byte("token")
			oldEvent := send(0, versionedTestInitial(t, version, old, old, nil, nil, 0, 0))
			require.Equal(t, "decoded", send(1, versionedTestInitial(t, version, old, nil, nil, nil, 0, 1)).Status)
			fresh := send(0, versionedTestInitial(t, version, original, original, nil, nil, 0, 0))
			require.NotEqual(t, oldEvent.FlowID, fresh.FlowID)
			retry := send(1, versionedTestRetry(version, original, nil, retryCID, token))
			require.Equal(t, fresh.FlowID, retry.FlowID, "public Retry tag selects the original CID despite competing zero-CID tuples")
			require.Equal(t, true, retry.Session["Retry Integrity Verified"])
			require.Equal(t, false, retry.Session["Authentication Verified"], "public integrity is not packet protection")
			var q *binQUIC
			for _, el := range a.udpSessions.entries {
				f := el.Value.(*binUDPEntry).flow
				if f.id == fresh.FlowID {
					q = f.quic
				}
			}
			require.NotNil(t, q)
			if !lost {
				require.Equal(t, "decoded", send(0, versionedTestInitial(t, version, retryCID, retryCID, nil, token, 1, 0)).Status)
			}
			before := bytes.Clone(q.initialDCID)
			server := versionedTestInitial(t, version, retryCID, nil, nil, nil, 0, 1)
			require.True(t, q.authenticatesExistingWire(1, server), "ownership probe must share post-Retry key recovery")
			require.Equal(t, before, q.initialDCID, "probe never commits replacement Initial keys")
			reply := send(1, server)
			require.Equal(t, fresh.FlowID, reply.FlowID)
			require.Equal(t, true, reply.Session["Authentication Verified"])
			require.Equal(t, retryCID, q.initialDCID)
			ack := send(0, versionedTestInitial(t, version, retryCID, nil, nil, token, 2, 0))
			require.Equal(t, "decoded", ack.Status, "observed zero-length server CID is a valid post-Retry destination")
			require.Equal(t, fresh.FlowID, ack.FlowID)
			require.Equal(t, true, ack.Session["Authentication Verified"])
			require.Equal(t, quicNativeProfile(version), ack.Profile)
			a.closeUDPSessions()
		}
	}
}
