package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQUICStoppedPacketPayloadDoesNotClaimRollback(t *testing.T) {
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	q.byteLimit = 1 << 20
	q.tls = &binTLS{client: 0, version: 0x304, suite: 0x1301, app: [2]bool{true, true}}
	before := *q.tls
	// A valid CID frame is applied before a refused CRYPTO message. This is
	// not an all-frames transaction, so diagnostic text must not claim one.
	cid := []byte{0x71, 0x72, 0x73}
	plain := append([]byte{0x18, 1, 0, byte(len(cid))}, cid...)
	plain = append(plain, bytes.Repeat([]byte{0x61}, 16)...)
	wire := quicTLSLevelMessage(24, []byte{0})
	plain = append(plain, 6, 0, byte(len(wire)))
	plain = append(plain, wire...)
	packet := quicConformancePacket(t, key, "1-RTT", 100, 0, plain)
	info, err := q.consume(0, packet, 4096)
	require.Error(t, err)
	require.Equal(t, before, *q.tls, "the refused TLS batch remains atomic")
	owner, present := q.cids[string(cid)]
	require.True(t, present, "the earlier valid transport frame was applied")
	require.Equal(t, 0, owner)
	require.Equal(t, true, info["Authentication Verified"])
	require.Equal(t, plain, info["Rejected Packet Payload"])
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(plain)), info["Rejected Packet Payload SHA256"])
	require.Contains(t, info["Payload Delivery"], "parsing stopped")
	require.Contains(t, info["Payload Delivery"], "earlier validated frames may have been applied")
	require.NotContains(t, info["Payload Delivery"], "not delivered")
	info["Rejected Packet Payload"].([]byte)[0] ^= 0xff
	info, err = q.consume(0, packet, 4096)
	require.Error(t, err)
	require.Equal(t, true, info["Duplicate"])
	require.Equal(t, plain, info["Rejected Packet Payload"], "the new diagnostic owns independently decrypted bytes")
	require.Contains(t, info["Payload Delivery"], "parsing stopped")
	require.Equal(t, before, *q.tls)
}

func FuzzQUICAuthenticatedCRYPTORefusalPayload(f *testing.F) {
	f.Add(quicTLSLevelMessage(24, []byte{0}), uint8(0))
	f.Add(quicTLSLevelMessage(24, []byte{0}), uint8(7))
	f.Add(quicTLSLevelMessage(13, []byte{0, 0, 0}), uint8(9))
	f.Add(finishedTestMessage(48), uint8(4))
	f.Add([]byte{}, uint8(10))
	f.Fuzz(func(t *testing.T, payload []byte, flags uint8) {
		if len(payload) > 512 {
			return
		}
		version := quicVersion1
		if flags&2 != 0 {
			version = quicVersion2
		}
		algorithm := int(flags>>2) % 3
		q, key := quicConformanceContext(t, version, []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[algorithm])
		q.byteLimit = 1024
		q.tls = &binTLS{client: 0, version: 0x304, suite: []uint16{0x1301, 0x1302, 0x1303}[algorithm], app: [2]bool{true, true}}
		before := *q.tls
		original := bytes.Clone(payload)
		plain := append([]byte{6, 0}, quicPutVarint(uint64(len(payload)))...)
		plain = append(plain, payload...)
		packet := quicConformancePacket(t, key, "1-RTT", 100, 0, plain)
		info, err := q.consume(int(flags&1), packet, 128)
		require.Equal(t, original, payload)
		require.Equal(t, true, info["Authentication Verified"])
		if err == nil {
			require.NotContains(t, info, "Rejected Packet Payload")
			return
		}
		require.Equal(t, before, *q.tls)
		require.Equal(t, plain, info["Rejected Packet Payload"])
		require.Contains(t, info["Payload Delivery"], "parsing stopped")
		info["Rejected Packet Payload"].([]byte)[0] ^= 0xff
		duplicate, duplicateErr := q.consume(int(flags&1), packet, 128)
		require.Error(t, duplicateErr)
		require.Equal(t, err.Error(), duplicateErr.Error())
		require.Equal(t, true, duplicate["Duplicate"])
		require.Equal(t, plain, duplicate["Rejected Packet Payload"])
		require.Equal(t, before, *q.tls)
	})
}
