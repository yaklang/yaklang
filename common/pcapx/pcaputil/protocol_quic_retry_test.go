package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQUICRetryRFC9001Vector(t *testing.T) {
	// RFC 9001 Appendix A.4, independent of the packet generator below.
	wire := rfcHex("ff000000010008f067a5502a4262b5746f6b656e04a265ba2eff4d829058fb3f0f2496ba")
	require.Equal(t, wire[len(wire)-16:], quicRetryTag(rfcHex("8394c8f03e515708"), wire[:len(wire)-16]))
}

func retryTestInitial(t *testing.T, dcid, token []byte, pn byte) []byte {
	return retryTestProtectedInitial(t, dcid, dcid, nil, token, pn, 0)
}
func retryTestProtectedInitial(t *testing.T, seed, dcid, scid, token []byte, pn byte, dir int) []byte {
	return retryTestInitialFrames(t, seed, dcid, scid, token, pn, dir, append([]byte{1}, make([]byte, 23)...))
}
func retryTestInitialFrames(t *testing.T, seed, dcid, scid, token []byte, pn byte, dir int, plain []byte) []byte {
	t.Helper()
	keys, err := quicInitialKeys(seed)
	require.NoError(t, err)
	k := keys[dir]
	head := append([]byte{0xc0, 0, 0, 0, 1, byte(len(dcid))}, dcid...)
	head = append(head, byte(len(scid)))
	head = append(head, scid...)
	head = append(head, quicPutVarint(uint64(len(token)))...)
	head = append(head, token...)
	head = append(head, quicPutVarint(uint64(1+len(plain)+16))...)
	pnAt := len(head)
	head = append(head, pn)
	block, err := aes.NewCipher(k.key)
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	wire := append(bytes.Clone(head), aead.Seal(nil, quicNonce(k.iv, uint64(pn)), plain, head)...)
	mask, err := quicHeaderMask(k, wire[pnAt+4:pnAt+20])
	require.NoError(t, err)
	wire[0] ^= mask[0] & 15
	wire[pnAt] ^= mask[1]
	return wire
}

func TestQUICRetryKeyTransitionAndReordering(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		q := &binQUIC{native: true, byteLimit: 1 << 20}
		initial := rfc9001ClientInitial()
		_, err := q.consume(0, initial, 4096)
		require.NoError(t, err)
		original := bytes.Clone(q.initialDCID)
		retry := rfcHex("ff000000010008f067a5502a4262b5746f6b656e04a265ba2eff4d829058fb3f0f2496ba")
		if corrupt {
			retry[len(retry)-1] ^= 1
		}
		info, err := q.consume(1, retry, 4096)
		if corrupt {
			require.Error(t, err)
			require.False(t, q.retrySeen)
			require.Equal(t, original, q.initialDCID)
			require.NotEqual(t, true, info["Retry Integrity Verified"])
			continue
		}
		require.NoError(t, err)
		require.Equal(t, true, info["Retry Integrity Verified"])
		require.Equal(t, false, info["Authentication Verified"], "public integrity tag is not peer authentication")
		require.Equal(t, original, q.initialDCID, "Retry alone does not commit new Initial keys")
		_, err = q.consume(0, retryTestInitial(t, q.retrySCID, []byte("wrong"), 3), 4096)
		require.Error(t, err)
		require.Equal(t, original, q.initialDCID)
		info, err = q.consume(0, retryTestInitial(t, q.retrySCID, []byte("token"), 3), 4096)
		require.NoError(t, err)
		require.Equal(t, true, info["Authentication Verified"])
		require.Equal(t, uint64(3), info["Packet Number"], "Retry does not reset the PN space")
		require.Equal(t, q.retrySCID, q.initialDCID)
		active := bytes.Clone(q.initialDCID)
		info, err = q.consume(0, initial, 4096)
		require.NoError(t, err)
		require.Equal(t, true, info["Duplicate"])
		require.Equal(t, active, q.initialDCID, "late old Initial cannot roll active keys back")
		_, err = q.consume(1, retry, 4096)
		require.Error(t, err)
		require.Equal(t, active, q.initialDCID)
	}
}

func TestQUICRetryServerCIDChangeAndLostClientInitial(t *testing.T) {
	for _, lost := range []bool{false, true} {
		q := &binQUIC{native: true, byteLimit: 1 << 20}
		_, err := q.consume(0, rfc9001ClientInitial(), 4096)
		require.NoError(t, err)
		retry := rfcHex("ff000000010008f067a5502a4262b5746f6b656e04a265ba2eff4d829058fb3f0f2496ba")
		_, err = q.consume(1, retry, 4096)
		require.NoError(t, err)
		if !lost {
			_, err = q.consume(0, retryTestInitial(t, q.retrySCID, []byte("token"), 3), 4096)
			require.NoError(t, err)
		}
		serverCID := []byte("server02")
		info, err := q.consume(1, retryTestProtectedInitial(t, q.retrySCID, nil, serverCID, nil, 0, 1), 4096)
		require.NoError(t, err)
		require.Equal(t, true, info["Authentication Verified"])
		info, err = q.consume(0, retryTestProtectedInitial(t, q.retrySCID, serverCID, nil, []byte("token"), 4, 0), 4096)
		require.NoError(t, err)
		require.Equal(t, true, info["Authentication Verified"])
		require.Equal(t, q.retrySCID, q.initialDCID, "wire DCID change does not change the Initial salt input")
		require.Equal(t, uint64(4), q.spaces[0][quicSpaceInitial].largest)
	}
}
